// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package prometheusrelabelprocessor

import (
	"math"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/prometheus/relabelconfig"
)

func testMetric() (pmetric.Metrics, pmetric.Metric) {
	input := pmetric.NewMetrics()
	rm := input.ResourceMetrics().AppendEmpty()
	rm.SetSchemaUrl("resource-schema")
	rm.Resource().Attributes().PutStr("service.name", "example")
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.SetSchemaUrl("scope-schema")
	sm.Scope().SetName("instrumentation")
	m := sm.Metrics().AppendEmpty()
	m.SetName("source.metric")
	m.SetDescription("metric description")
	return input, m
}

func TestScalarPrecisionAndMetadata(t *testing.T) {
	input, m := testMetric()
	m.SetUnit("By")
	m.Metadata().PutStr("metadata", "kept")
	p := m.SetEmptyGauge().DataPoints().AppendEmpty()
	p.SetIntValue(1<<53 + 1)
	p.SetTimestamp(7)
	p.SetStartTimestamp(2)
	p.SetFlags(pmetric.DefaultDataPointFlags.WithNoRecordedValue(true))
	p.Attributes().PutInt("keep.typed", math.MinInt64)
	p.Attributes().PutBool("old", true)
	p.Attributes().PutEmptyBytes("empty")
	p.Exemplars().AppendEmpty().SetIntValue(math.MaxInt64)
	rules, err := (relabelconfig.Rules{
		{"source_labels": []string{"old"}, "target_label": "__tmp_capture"},
		{"source_labels": []string{"__tmp_capture"}, "target_label": "new"},
		{"action": "labeldrop", "regex": "old"},
		{"source_labels": []string{"__name__"}, "target_label": "__name__", "regex": "source_metric_bytes", "replacement": "renamed"},
	}).Compile(model.LegacyValidation)
	require.NoError(t, err)
	out, drops, err := relabelMetrics(t.Context(), input, rules)
	require.NoError(t, err)
	require.Equal(t, dropCounts{}, drops)
	require.Equal(t, "resource-schema", out.ResourceMetrics().At(0).SchemaUrl())
	scope := out.ResourceMetrics().At(0).ScopeMetrics().At(0)
	require.Equal(t, "scope-schema", scope.SchemaUrl())
	o := scope.Metrics().At(0)
	require.Equal(t, "renamed", o.Name())
	require.Equal(t, m.Unit(), o.Unit())
	require.Equal(t, m.Metadata().AsRaw(), o.Metadata().AsRaw())
	q := o.Gauge().DataPoints().At(0)
	require.Equal(t, p.IntValue(), q.IntValue())
	require.Equal(t, p.Flags(), q.Flags())
	require.Equal(t, p.Timestamp(), q.Timestamp())
	require.Equal(t, p.StartTimestamp(), q.StartTimestamp())
	require.Equal(t, p.Exemplars().At(0).IntValue(), q.Exemplars().At(0).IntValue())
	require.Equal(t, map[string]any{"keep.typed": int64(math.MinInt64), "new": "true", "empty": []byte(nil)}, q.Attributes().AsRaw())
	require.Equal(t, "source.metric", m.Name())
	require.True(t, p.Attributes().AsRaw()["old"].(bool))
}

func TestIncompleteHistogramAndAbsentSum(t *testing.T) {
	input, m := testMetric()
	h := m.SetEmptyHistogram()
	h.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	p := h.DataPoints().AppendEmpty()
	p.ExplicitBounds().FromRaw([]float64{.2})
	p.BucketCounts().FromRaw([]uint64{0, 1})
	p.SetCount(1)
	for _, tc := range []struct {
		pattern string
		drops   dropCounts
		want    int
	}{
		{"source_metric_count", dropCounts{incompleteFamily: 1}, 0},
		{"source_metric_sum", dropCounts{}, 1},
		{"source_metric_.*", dropCounts{ruleDrop: 1}, 0},
	} {
		t.Run(tc.pattern, func(t *testing.T) {
			rules, err := (relabelconfig.Rules{{"source_labels": []string{"__name__"}, "regex": tc.pattern, "action": "drop"}}).Compile(model.LegacyValidation)
			require.NoError(t, err)
			out, drops, err := relabelMetrics(t.Context(), input, rules)
			require.NoError(t, err)
			require.Equal(t, tc.drops, drops)
			require.Equal(t, tc.want, out.DataPointCount())
		})
	}
	require.False(t, p.HasSum())
}

func TestLabelRemovalCannotMergeIndependentStreams(t *testing.T) {
	input, m := testMetric()
	points := m.SetEmptyGauge().DataPoints()
	for _, item := range []struct {
		label string
		ts    uint64
	}{{"a", 1}, {"b", 2}, {"a", 3}} {
		pt := points.AppendEmpty()
		pt.Attributes().PutStr("stream", item.label)
		pt.SetTimestamp(pcommon.Timestamp(item.ts))
	}
	rules, err := (relabelconfig.Rules{{"action": "labeldrop", "regex": "stream"}}).Compile(model.LegacyValidation)
	require.NoError(t, err)
	out, drops, err := relabelMetrics(t.Context(), input, rules)
	require.NoError(t, err)
	require.Zero(t, out.DataPointCount())
	require.Equal(t, dropCounts{identityCollision: 3}, drops)
}
