// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/prompb"
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
	"go.opentelemetry.io/collector/pdata/pmetric/pmetricotlp"

	"github.com/open-telemetry/opentelemetry-collector-contrib/examples/prometheus-relabel/internal/workload"
)

type result struct {
	Protocol    int     `json:"protocol"`
	Mode        string  `json:"mode"`
	Policy      string  `json:"policy"`
	Mixed       bool    `json:"mixed"`
	Repeat      int     `json:"repeat"`
	Batches     int     `json:"batches"`
	Input       int     `json:"input_points"`
	Samples     int     `json:"output_samples"`
	WallSeconds float64 `json:"wall_seconds"`
	CPUSeconds  float64 `json:"collector_cpu_seconds"`
	PeakKiB     int64   `json:"collector_peak_rss_kib"`
}

func main() {
	collector := flag.String("collector", "/relabel-collector", "Collector binary")
	repeats := flag.Int("repeats", 3, "repetitions per configuration")
	batches := flag.Int("batches", 120, "measured batches per run")
	points := flag.Int("points", 256, "points per input metric")
	flag.Parse()
	if *batches < 1 || *points < 2 || *points%2 != 0 || *repeats < 1 {
		log.Fatal("use positive batches/repeats and an even number of points")
	}

	for _, protocol := range []int{1, 2} {
		for _, mixed := range []bool{false, true} {
			md := workload.Metrics(*points, mixed)
			payload, err := pmetricotlp.NewExportRequestFromMetrics(md).MarshalProto()
			if err != nil {
				log.Fatal(err)
			}
			for repetition := range *repeats {
				for _, policy := range []string{"empty", "no-match", "drop", "rename"} {
					modes := []string{"baseline", "processor", "exporter"}
					wire := make(map[string][]string, 3)
					for shift := range modes {
						mode := modes[(shift+repetition)%len(modes)]
						r := result{Protocol: protocol, Mode: mode, Policy: policy, Mixed: mixed, Repeat: repetition + 1, Batches: *batches}
						outputPoints := *points
						if mode != "baseline" && policy == "drop" {
							outputPoints /= 2
						}
						seriesCount := outputPoints
						if mixed {
							seriesCount *= 17 // 1 gauge + 11 histogram series + 5 summary series.
						}
						r.Input = *batches * md.DataPointCount()
						if wire[mode], err = run(*collector, payload, seriesCount, &r); err != nil {
							log.Fatalf("%+v: %v", r, err)
						}
						if err := json.NewEncoder(os.Stdout).Encode(r); err != nil {
							log.Fatal(err)
						}
					}
					if !equal(wire["processor"], wire["exporter"]) ||
						((policy == "no-match" || policy == "empty") && !equal(wire["baseline"], wire["processor"])) {
						log.Fatalf("wire samples disagree for protocol %d, mixed=%t, policy %s", protocol, mixed, policy)
					}
				}
			}
		}
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func run(binary string, payload []byte, expected int, r *result) ([]string, error) {
	var requestCount atomic.Int64
	var verifying atomic.Bool
	verifying.Store(true)
	captured := make(chan []string, 2)
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if verifying.Load() {
			rows, err := readWire(request, r.Protocol)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			captured <- rows
		} else if _, err := io.Copy(io.Discard, request.Body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if r.Protocol == 2 {
			w.Header().Set("X-Prometheus-Remote-Write-Samples-Written", strconv.Itoa(expected))
		}
		requestCount.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer sink.Close()

	probe, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	endpoint := probe.Addr().String()
	probe.Close()

	dir, err := os.MkdirTemp("", "relabel-bench-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	config := filepath.Join(dir, "config.yaml")
	if err := writeConfig(config, sink.URL, endpoint, r); err != nil {
		return nil, err
	}
	logs, err := os.Create(filepath.Join(dir, "collector.log"))
	if err != nil {
		return nil, err
	}
	defer logs.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "--config="+config, "--feature-gates=exporter.prometheusremotewritexporter.enableSendingRW2")
	cmd.Stderr, cmd.Stdout = logs, logs
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	stop := func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		_ = cmd.Wait()
	}
	defer func() {
		if cmd.ProcessState == nil {
			stop()
		}
	}()

	client := &http.Client{Timeout: 10 * time.Second}
	defer client.CloseIdleConnections()
	url := "http://" + endpoint + "/v1/metrics"
	send := func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/x-protobuf")
		response, err := client.Do(req)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		if err != nil {
			return err
		}
		if response.StatusCode != http.StatusOK {
			return fmt.Errorf("OTLP response %s: %s", response.Status, body)
		}
		reply := pmetricotlp.NewExportResponse()
		if err := reply.UnmarshalProto(body); err != nil {
			return err
		}
		if partial := reply.PartialSuccess(); partial.RejectedDataPoints() != 0 || partial.ErrorMessage() != "" {
			return fmt.Errorf("OTLP partial success: %s", partial.ErrorMessage())
		}
		return nil
	}

	deadline := time.Now().Add(8 * time.Second)
	for {
		conn, dialErr := net.DialTimeout("tcp", endpoint, 100*time.Millisecond)
		if dialErr == nil {
			conn.Close()
			break
		}
		if time.Now().After(deadline) {
			content, _ := os.ReadFile(logs.Name())
			return nil, fmt.Errorf("collector startup failed: %w\n%s", dialErr, content)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err := send(); err != nil {
		content, _ := os.ReadFile(logs.Name())
		return nil, fmt.Errorf("wire verification input: %w\n%s", err, content)
	}
	var wire []string
	requestsPerBatch := 1
	if r.Protocol == 1 {
		requestsPerBatch++ // The exporter sends v1 metadata in a separate request.
	}
	for range requestsPerBatch {
		select {
		case rows := <-captured:
			wire = append(wire, rows...)
		default:
			return nil, errors.New("collector did not send the expected Remote Write requests")
		}
	}
	if len(wire) != expected {
		return nil, fmt.Errorf("wire verification got %d samples, want %d", len(wire), expected)
	}
	for _, row := range wire {
		if r.Mode != "baseline" && r.Policy == "rename" && (!strings.Contains(row, `cluster="translated"`) || strings.Contains(row, "cluster_name=")) {
			return nil, fmt.Errorf("label rename not present in %s", row)
		}
		if r.Mode != "baseline" && r.Policy == "drop" && !strings.Contains(row, `drop_me="false"`) {
			return nil, fmt.Errorf("drop retained %s", row)
		}
	}
	sort.Strings(wire)
	verifying.Store(false)
	for range 12 {
		if err := send(); err != nil {
			return nil, err
		}
	}
	startCount := requestCount.Load()
	start := time.Now()
	for range r.Batches {
		if err := send(); err != nil {
			return nil, err
		}
	}
	r.WallSeconds = time.Since(start).Seconds()
	if requestCount.Load()-startCount != int64(r.Batches*requestsPerBatch) {
		return nil, errors.New("unexpected number of Remote Write requests in measured interval")
	}
	r.Samples = r.Batches * expected
	stop()
	r.CPUSeconds = (cmd.ProcessState.UserTime() + cmd.ProcessState.SystemTime()).Seconds()
	r.PeakKiB = cmd.ProcessState.SysUsage().(*syscall.Rusage).Maxrss
	return wire, nil
}

func writeConfig(path, sink, listen string, r *result) error {
	exporter := map[string]any{
		"http": map[string]any{"endpoint": sink}, "remote_write_queue": map[string]any{"enabled": false},
		"retry_on_failure": map[string]any{"enabled": false}, "translation_strategy": "UnderscoreEscapingWithSuffixes",
		"send_metadata": true,
	}
	if r.Protocol == 2 {
		exporter["protobuf_message"] = "io.prometheus.write.v2.Request"
	}
	pipeline := map[string]any{"receivers": []string{"otlp"}, "exporters": []string{"prometheus_remote_write"}}
	config := map[string]any{
		"receivers": map[string]any{"otlp": map[string]any{"protocols": map[string]any{"http": map[string]any{"endpoint": listen}}}},
		"exporters": map[string]any{"prometheus_remote_write": exporter},
		"service": map[string]any{
			"pipelines": map[string]any{"metrics": pipeline},
			"telemetry": map[string]any{"logs": map[string]any{"level": "error"}, "metrics": map[string]any{"level": "none"}},
		},
	}
	switch r.Mode {
	case "processor":
		pipeline["processors"] = []string{"prometheus_relabel"}
		config["processors"] = map[string]any{"prometheus_relabel": map[string]any{"metric_relabel_configs": workload.Rules(r.Policy)}}
	case "exporter":
		exporter["write_relabel_configs"] = workload.Rules(r.Policy)
	}
	data, err := json.Marshal(config)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func readWire(req *http.Request, protocol int) ([]string, error) {
	raw, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	decoded, err := snappy.Decode(nil, raw)
	if err != nil {
		return nil, err
	}
	var rows []string
	if protocol == 1 {
		var request prompb.WriteRequest
		if err := request.Unmarshal(decoded); err != nil {
			return nil, err
		}
		builder := labels.NewScratchBuilder(0)
		for _, series := range request.Timeseries {
			ls := series.ToLabels(&builder, nil).String()
			for _, sample := range series.Samples {
				rows = append(rows, fmt.Sprintf("%s %g %d", ls, sample.Value, sample.Timestamp))
			}
		}
		return rows, nil
	}
	var request writev2.Request
	if err := request.Unmarshal(decoded); err != nil {
		return nil, err
	}
	builder := labels.NewScratchBuilder(0)
	for _, series := range request.Timeseries {
		ls, err := series.ToLabels(&builder, request.Symbols)
		if err != nil {
			return nil, err
		}
		for _, sample := range series.Samples {
			rows = append(rows, fmt.Sprintf("%s %g %d %d", ls.String(), sample.Value, sample.Timestamp, sample.StartTimestamp))
		}
	}
	return rows, nil
}
