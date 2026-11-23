// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Package workload defines shared synthetic metrics and rules for both benchmarks.
package workload

import (
	"strconv"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/prometheus/relabelconfig"
)

func Metrics(points int, mixed bool) pmetric.Metrics {
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "relabel-benchmark")
	ms := rm.ScopeMetrics().AppendEmpty().Metrics()
	n := ms.AppendEmpty()
	n.SetName("requests.inflight")
	gauges := n.SetEmptyGauge().DataPoints()
	var histograms pmetric.HistogramDataPointSlice
	var summaries pmetric.SummaryDataPointSlice
	if mixed {
		h := ms.AppendEmpty()
		h.SetName("request.duration")
		h.SetUnit("s")
		hist := h.SetEmptyHistogram()
		hist.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
		histograms = hist.DataPoints()
		s := ms.AppendEmpty()
		s.SetName("job.duration")
		s.SetUnit("s")
		summaries = s.SetEmptySummary().DataPoints()
	}
	timestamp := pcommon.NewTimestampFromTime(time.Unix(1800000000, 0))
	for i := range points {
		g := gauges.AppendEmpty()
		g.SetIntValue(42)
		g.SetTimestamp(timestamp)
		setAttributes(g.Attributes(), i)
		if mixed {
			h := histograms.AppendEmpty()
			h.SetTimestamp(timestamp)
			h.ExplicitBounds().FromRaw([]float64{.001, .005, .01, .05, .1, .5, 1, 2})
			h.BucketCounts().FromRaw([]uint64{0, 1, 3, 5, 7, 8, 2, 1, 1})
			h.SetCount(28)
			h.SetSum(4.5)
			setAttributes(h.Attributes(), i)
			s := summaries.AppendEmpty()
			s.SetTimestamp(timestamp)
			s.SetCount(3)
			s.SetSum(1.8)
			for _, q := range []float64{.5, .95, .99} {
				item := s.QuantileValues().AppendEmpty()
				item.SetQuantile(q)
				item.SetValue(q)
			}
			setAttributes(s.Attributes(), i)
		}
	}
	return md
}

func setAttributes(attrs pcommon.Map, i int) {
	attrs.PutStr("cluster.name", "test")
	attrs.PutInt("shard", int64(i))
	attrs.PutStr("instance_id", "benchmark-"+strconv.Itoa(i))
	attrs.PutBool("drop_me", i%2 == 0)
}

func Rules(policy string) relabelconfig.Rules {
	switch policy {
	case "no-match":
		return relabelconfig.Rules{{"source_labels": []string{"cluster_name"}, "regex": "never", "action": "drop"}}
	case "drop":
		return relabelconfig.Rules{{"source_labels": []string{"drop_me"}, "regex": "true", "action": "drop"}}
	case "rename":
		return relabelconfig.Rules{
			{"source_labels": []string{"cluster_name"}, "target_label": "cluster", "replacement": "translated"},
			{"regex": "cluster_name", "action": "labeldrop"},
		}
	}
	return relabelconfig.Rules{}
}
