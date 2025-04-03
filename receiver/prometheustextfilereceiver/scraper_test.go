// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package prometheustextfilereceiver

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/receiver/receivertest"
	"google.golang.org/protobuf/proto"
)

func TestScraper(t *testing.T) {
	dir := t.TempDir()
	
	// Copy the sample.prom file to the temp dir
	samplePath := filepath.Join("testdata", "sample.prom")
	sampleContent, err := os.ReadFile(samplePath)
	require.NoError(t, err)
	
	destPath := filepath.Join(dir, "sample.prom")
	err = os.WriteFile(destPath, sampleContent, 0600)
	require.NoError(t, err)
	
	cfg := &Config{
		Directories: []string{dir},
	}
	
	scraper := newTextfileScraper(
		receivertest.NewNopCreateSettings(),
		cfg,
	)
	
	metrics, err := scraper.scrape(context.Background())
	require.NoError(t, err)
	
	// Check if metrics were collected
	assert.Greater(t, metrics.ResourceMetrics().Len(), 0)
	
	// Verify metrics details - we should have core metrics plus the textfile_mtime_seconds metric
	gaugeFound := false
	counterFound := false
	histogramFound := false
	summaryFound := false
	mtimeFound := false
	
	// Look for specific metrics
	for i := 0; i < metrics.ResourceMetrics().Len(); i++ {
		rm := metrics.ResourceMetrics().At(i)
		for j := 0; j < rm.ScopeMetrics().Len(); j++ {
			sm := rm.ScopeMetrics().At(j)
			for k := 0; k < sm.Metrics().Len(); k++ {
				m := sm.Metrics().At(k)
				
				switch m.Name() {
				case "test_metric_gauge":
					gaugeFound = true
					assert.Equal(t, "A test gauge metric", m.Description())
					assert.Equal(t, 1, m.Gauge().DataPoints().Len())
				case "test_metric_counter":
					counterFound = true
					assert.Equal(t, "A test counter metric", m.Description())
					assert.Equal(t, 1, m.Sum().DataPoints().Len())
				case "test_metric_histogram":
					histogramFound = true
					assert.Equal(t, "A test histogram metric", m.Description())
					assert.Equal(t, 1, m.Histogram().DataPoints().Len())
				case "test_metric_summary":
					summaryFound = true
					assert.Equal(t, "A test summary metric", m.Description())
					assert.Equal(t, 1, m.Summary().DataPoints().Len())
				case "textfile_mtime_seconds":
					mtimeFound = true
					assert.Equal(t, 1, m.Gauge().DataPoints().Len())
				}
			}
		}
	}
	
	assert.True(t, gaugeFound, "Gauge metric not found")
	assert.True(t, counterFound, "Counter metric not found")
	assert.True(t, histogramFound, "Histogram metric not found")
	assert.True(t, summaryFound, "Summary metric not found")
	assert.True(t, mtimeFound, "Mtime metric not found")
}

func TestProcessFile(t *testing.T) {
	cfg := &Config{}
	scraper := newTextfileScraper(
		receivertest.NewNopCreateSettings(),
		cfg,
	)
	
	// Test with a valid file
	samplePath := filepath.Join("testdata", "sample.prom")
	mtime, families, err := scraper.processFile(samplePath)
	require.NoError(t, err)
	assert.NotEmpty(t, mtime)
	assert.NotEmpty(t, families)
	assert.Equal(t, 4, len(families))
	
	// Test with a non-existent file
	_, _, err = scraper.processFile("nonexistent.prom")
	assert.Error(t, err)
}

func TestHasTimestamps(t *testing.T) {
	cfg := &Config{}
	scraper := newTextfileScraper(
		receivertest.NewNopCreateSettings(), 
		cfg,
	)
	
	// Sample without timestamps
	assert.False(t, scraper.hasTimestamps(map[string]*dto.MetricFamily{}))
	
	// Create a sample with a timestamp
	now := time.Now().UnixMilli()
	metricWithTimestamp := &dto.Metric{
		Label: []*dto.LabelPair{},
		Gauge: &dto.Gauge{
			Value: proto.Float64(42.0),
		},
		TimestampMs: proto.Int64(now),
	}
	
	family := &dto.MetricFamily{
		Name: proto.String("test_metric"),
		Help: proto.String("Test metric"),
		Type: dto.MetricType_GAUGE.Enum(),
		Metric: []*dto.Metric{
			metricWithTimestamp,
		},
	}
	
	familiesMap := map[string]*dto.MetricFamily{
		"test_metric": family,
	}
	
	// Should detect the timestamp
	assert.True(t, scraper.hasTimestamps(familiesMap))
} 