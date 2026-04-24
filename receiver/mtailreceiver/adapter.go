// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package mtailreceiver // import "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/mtailreceiver"

import (
	"fmt"
	"math"
	"sort"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"

	mtaildatum "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/mtailreceiver/internal/mtail/metrics/datum"
	mtailmetrics "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/mtailreceiver/internal/mtail/metrics"
)

func convertStoreToMetrics(store *mtailmetrics.Store, startTime time.Time) (pmetric.Metrics, error) {
	md := pmetric.NewMetrics()
	if store == nil {
		return md, nil
	}

	startTs := pcommon.NewTimestampFromTime(startTime)
	resourceMetrics := md.ResourceMetrics().AppendEmpty()

	byProgram := make(map[string][]*mtailmetrics.Metric)
	if err := store.Range(func(m *mtailmetrics.Metric) error {
		if m == nil || m.Hidden || m.Kind == mtailmetrics.Text {
			return nil
		}
		byProgram[m.Program] = append(byProgram[m.Program], m)
		return nil
	}); err != nil {
		return md, err
	}

	programs := make([]string, 0, len(byProgram))
	for program := range byProgram {
		programs = append(programs, program)
	}
	sort.Strings(programs)

	for _, program := range programs {
		scopeMetrics := resourceMetrics.ScopeMetrics().AppendEmpty()
		scopeMetrics.Scope().SetName(program)
		metrics := byProgram[program]
		sort.Slice(metrics, func(i, j int) bool {
			if metrics[i].Name == metrics[j].Name {
				return metrics[i].Source < metrics[j].Source
			}
			return metrics[i].Name < metrics[j].Name
		})
		for _, metric := range metrics {
			if err := appendMetric(scopeMetrics.Metrics().AppendEmpty(), metric, startTs); err != nil {
				return md, err
			}
		}
	}

	return md, nil
}

func appendMetric(dest pmetric.Metric, metric *mtailmetrics.Metric, startTs pcommon.Timestamp) error {
	metric.RLock()
	defer metric.RUnlock()

	dest.SetName(metric.Name)
	if metric.Source != "" {
		dest.SetDescription(fmt.Sprintf("%s defined at %s", metric.Name, metric.Source))
	}

	switch metric.Kind {
	case mtailmetrics.Counter:
		return appendSumMetric(dest, metric, startTs, true)
	case mtailmetrics.Gauge, mtailmetrics.Timer:
		return appendGaugeMetric(dest, metric)
	case mtailmetrics.Histogram:
		return appendHistogramMetric(dest, metric, startTs)
	default:
		return nil
	}
}

func appendSumMetric(dest pmetric.Metric, metric *mtailmetrics.Metric, startTs pcommon.Timestamp, monotonic bool) error {
	sum := dest.SetEmptySum()
	sum.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	sum.SetIsMonotonic(monotonic)

	for _, labelValue := range metric.LabelValues {
		dp := sum.DataPoints().AppendEmpty()
		dp.SetStartTimestamp(startTs)
		dp.SetTimestamp(pcommon.NewTimestampFromTime(labelValue.Value.TimeUTC()))
		fillAttributes(dp.Attributes(), metric.Keys, labelValue.Labels)
		switch metric.Type {
		case mtailmetrics.Int:
			dp.SetIntValue(mtaildatum.GetInt(labelValue.Value))
		case mtailmetrics.Float:
			dp.SetDoubleValue(mtaildatum.GetFloat(labelValue.Value))
		default:
			return fmt.Errorf("unsupported sum type %s for metric %s", metric.Type, metric.Name)
		}
	}

	return nil
}

func appendGaugeMetric(dest pmetric.Metric, metric *mtailmetrics.Metric) error {
	gauge := dest.SetEmptyGauge()

	for _, labelValue := range metric.LabelValues {
		dp := gauge.DataPoints().AppendEmpty()
		dp.SetTimestamp(pcommon.NewTimestampFromTime(labelValue.Value.TimeUTC()))
		fillAttributes(dp.Attributes(), metric.Keys, labelValue.Labels)
		switch metric.Type {
		case mtailmetrics.Int:
			dp.SetIntValue(mtaildatum.GetInt(labelValue.Value))
		case mtailmetrics.Float:
			dp.SetDoubleValue(mtaildatum.GetFloat(labelValue.Value))
		default:
			return fmt.Errorf("unsupported gauge type %s for metric %s", metric.Type, metric.Name)
		}
	}

	return nil
}

func appendHistogramMetric(dest pmetric.Metric, metric *mtailmetrics.Metric, startTs pcommon.Timestamp) error {
	histogram := dest.SetEmptyHistogram()
	histogram.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)

	for _, labelValue := range metric.LabelValues {
		dp := histogram.DataPoints().AppendEmpty()
		dp.SetStartTimestamp(startTs)
		dp.SetTimestamp(pcommon.NewTimestampFromTime(labelValue.Value.TimeUTC()))
		fillAttributes(dp.Attributes(), metric.Keys, labelValue.Labels)

		buckets := mtaildatum.GetBuckets(labelValue.Value)
		bounds, counts := histogramBoundsAndCounts(buckets)
		dp.SetCount(mtaildatum.GetBucketsCount(labelValue.Value))
		dp.SetSum(mtaildatum.GetBucketsSum(labelValue.Value))
		dp.ExplicitBounds().FromRaw(bounds)
		dp.BucketCounts().FromRaw(counts)
	}

	return nil
}

func histogramBoundsAndCounts(buckets *mtaildatum.Buckets) ([]float64, []uint64) {
	if buckets == nil || len(buckets.Buckets) == 0 {
		return nil, nil
	}

	counts := make([]uint64, len(buckets.Buckets))
	bounds := make([]float64, 0, len(buckets.Buckets)-1)
	counts = make([]uint64, len(buckets.Buckets))
	for i, bucket := range buckets.Buckets {
		counts[i] = bucket.Count
		if !math.IsInf(bucket.Range.Max, +1) {
			bounds = append(bounds, bucket.Range.Max)
		}
	}
	return bounds, counts
}

func fillAttributes(dest pcommon.Map, keys, values []string) {
	for i := range keys {
		if i < len(values) {
			dest.PutStr(keys[i], values[i])
		}
	}
}
