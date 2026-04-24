// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package mtailreceiver

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	mtaildatum "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/mtailreceiver/internal/mtail/metrics/datum"
	mtailmetrics "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/mtailreceiver/internal/mtail/metrics"
)

func TestConvertStoreToMetrics(t *testing.T) {
	store := mtailmetrics.NewStore()

	counter := mtailmetrics.NewMetric("lines_total", "example.mtail", mtailmetrics.Counter, mtailmetrics.Int)
	mtaildatum.SetInt(mustGetDatum(t, counter), 3, time.Unix(10, 0))
	require.NoError(t, store.Add(counter))

	gauge := mtailmetrics.NewMetric("queue_depth", "example.mtail", mtailmetrics.Gauge, mtailmetrics.Float, "host")
	mtaildatum.SetFloat(mustGetDatum(t, gauge, "web-1"), 2.5, time.Unix(11, 0))
	require.NoError(t, store.Add(gauge))

	histogram := mtailmetrics.NewMetric("latency", "example.mtail", mtailmetrics.Histogram, mtailmetrics.Buckets)
	histogram.Buckets = []mtaildatum.Range{{Min: 0, Max: 1}, {Min: 1, Max: 5}, {Min: 5, Max: math.Inf(+1)}}
	mtaildatum.Observe(mustGetDatum(t, histogram), 0.5, time.Unix(12, 0))
	mtaildatum.Observe(mustGetDatum(t, histogram), 3, time.Unix(13, 0))
	require.NoError(t, store.Add(histogram))

	md, err := convertStoreToMetrics(store, time.Unix(1, 0))
	require.NoError(t, err)
	require.Equal(t, 3, md.MetricCount())

	scopeMetrics := md.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics()
	require.Equal(t, "example.mtail", md.ResourceMetrics().At(0).ScopeMetrics().At(0).Scope().Name())
	require.Equal(t, "latency", scopeMetrics.At(0).Name())
	require.Equal(t, "lines_total", scopeMetrics.At(1).Name())
	require.Equal(t, "queue_depth", scopeMetrics.At(2).Name())
}

func mustGetDatum(t *testing.T, metric *mtailmetrics.Metric, labels ...string) mtaildatum.Datum {
	t.Helper()
	d, err := metric.GetDatum(labels...)
	require.NoError(t, err)
	return d
}
