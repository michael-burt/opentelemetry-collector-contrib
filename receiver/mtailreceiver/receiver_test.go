// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package mtailreceiver

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/receiver/receivertest"

	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/mtailreceiver/internal/metadata"
)

func TestReceiverEmitsMetricsFromLogLines(t *testing.T) {
	dir := t.TempDir()
	programPath := filepath.Join(dir, "simple.mtail")
	logPath := filepath.Join(dir, "simple.log")

	require.NoError(t, os.WriteFile(programPath, []byte("counter lines_total\n\n/$/ {\n  lines_total++\n}\n"), 0o600))
	require.NoError(t, os.WriteFile(logPath, nil, 0o600))

	sink := new(consumertest.MetricsSink)
	recv := newMetricsReceiver(receivertest.NewNopSettings(metadata.Type), &Config{
		Programs:           programPath,
		Logs:               []string{logPath},
		CollectionInterval: 20 * time.Millisecond,
		PollInterval:       10 * time.Millisecond,
	}, sink)

	require.NoError(t, recv.Start(t.Context(), componenttest.NewNopHost()))
	t.Cleanup(func() {
		require.NoError(t, recv.Shutdown(t.Context()))
	})

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o600)
	require.NoError(t, err)
	_, err = f.WriteString("hello world\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	require.Eventually(t, func() bool {
		all := sink.AllMetrics()
		if len(all) == 0 {
			return false
		}
		for _, md := range all {
			rms := md.ResourceMetrics()
			for i := 0; i < rms.Len(); i++ {
				sms := rms.At(i).ScopeMetrics()
				for j := 0; j < sms.Len(); j++ {
					metrics := sms.At(j).Metrics()
					for k := 0; k < metrics.Len(); k++ {
						if metrics.At(k).Name() == "lines_total" {
							return true
						}
					}
				}
			}
		}
		return false
	}, time.Second, 20*time.Millisecond)
}
