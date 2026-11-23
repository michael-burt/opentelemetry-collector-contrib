// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package prometheusremotewrite

import (
	"testing"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pmetric"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/prometheus/relabelconfig"
)

func TestWriteRelabelSeriesAndMetadata(t *testing.T) {
	md := pmetric.NewMetrics()
	ms := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics()
	m := ms.AppendEmpty()
	m.SetName("latency")
	m.SetDescription("request latency")
	h := m.SetEmptyHistogram()
	h.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	p := h.DataPoints().AppendEmpty()
	p.ExplicitBounds().FromRaw([]float64{1})
	p.BucketCounts().FromRaw([]uint64{1, 1})
	p.SetCount(2)
	p.SetSum(3)
	p.SetTimestamp(1000000)
	p.Exemplars().AppendEmpty().SetDoubleValue(.5)

	rules, err := (relabelconfig.Rules{
		{"source_labels": []string{"destination", "le"}, "regex": "test;1", "action": "drop"},
		{"source_labels": []string{"__name__"}, "regex": "latency_(.+)", "target_label": "__name__", "replacement": "renamed_$1"},
	}).Compile(model.LegacyValidation)
	require.NoError(t, err)
	settings := Settings{ExternalLabels: map[string]string{"destination": "test"}, WriteRelabelConfigs: rules}
	v1, metadata, err := FromMetricsWithMetadata(md, settings)
	require.NoError(t, err)
	require.Len(t, v1, 3)
	require.Len(t, metadata, 1)
	require.Equal(t, "renamed", metadata[0].MetricFamilyName)
	require.Equal(t, m.Description(), metadata[0].Help)
	names := make(map[string]bool)
	for _, ts := range v1 {
		require.Empty(t, ts.Exemplars, "exemplars from the dropped bucket must not move")
		names[seriesName(ts.Labels)] = true
	}
	require.Equal(t, map[string]bool{"renamed_bucket": true, "renamed_count": true, "renamed_sum": true}, names)

	v2, symbols, err := FromMetricsV2(md, settings)
	require.NoError(t, err)
	require.Len(t, v2, 3)
	builder := labels.NewScratchBuilder(0)
	for _, ts := range v2 {
		ls, decodeErr := ts.ToLabels(&builder, symbols.Symbols())
		require.NoError(t, decodeErr)
		require.True(t, names[ls.Get("__name__")])
		require.NotEqual(t, "1", ls.Get("le"))
		require.Empty(t, ts.Exemplars)
		require.Equal(t, m.Description(), symbols.Symbols()[ts.Metadata.HelpRef])
	}
}
