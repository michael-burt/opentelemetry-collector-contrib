// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package prometheustextfilereceiver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/prometheus/common/expfmt"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/receiver"
	"go.uber.org/zap"

	dto "github.com/prometheus/client_model/go"

	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/prometheustextfilereceiver/internal/metadata"
)

type textfileScraper struct {
	cfg       *Config
	settings  component.TelemetrySettings
	logger    *zap.Logger
	mb        *metadata.MetricsBuilder
	buildInfo component.BuildInfo
}

// newTextfileScraper creates a new textfile scraper
func newTextfileScraper(settings receiver.Settings, cfg *Config) *textfileScraper {
	return &textfileScraper{
		cfg:       cfg,
		settings:  settings.TelemetrySettings,
		logger:    settings.TelemetrySettings.Logger,
		mb:        metadata.NewMetricsBuilder(cfg.MetricsBuilderConfig, settings),
		buildInfo: settings.BuildInfo,
	}
}

// start starts the scraper
func (s *textfileScraper) start(context.Context, component.Host) error {
	return nil
}

// scrape collects metrics from prometheus text files
func (s *textfileScraper) scrape(ctx context.Context) (pmetric.Metrics, error) {
	metrics := pmetric.NewMetrics()

	var scrapeErrors []error
	successfulFiles := make(map[string]time.Time)

	// Generate list of all files to process
	allFiles := []string{}
	for _, directory := range s.cfg.Directories {
		files, err := filepath.Glob(filepath.Join(directory, "*.prom"))
		if err != nil {
			s.logger.Error("Error processing directory", zap.String("directory", directory), zap.Error(err))
			scrapeErrors = append(scrapeErrors, err)
		} else if len(files) == 0 {
			files = []string{}
			s.logger.Info("No .prom files found in directory", zap.String("directory", directory))
		}
		allFiles = append(allFiles, files...)
		for _, file := range allFiles {
			s.logger.Info("file found", zap.String("file", file))
		}
	}

	// Check that files exist
	for _, filePath := range allFiles {
		// unable to locate file
		fileInfo, err := os.Stat(filePath)
		if err != nil {
			scrapeErrors = append(scrapeErrors, fmt.Errorf("failed to locate file %s: %w", filePath, err))
			continue
		}
		// file is actually directory
		if fileInfo.IsDir() {
			scrapeErrors = append(scrapeErrors, fmt.Errorf("file is actually directory: %s", filePath))
		}

		t, mf, err := s.processFile(filePath)
		if err != nil {
			scrapeErrors = append(scrapeErrors, fmt.Errorf("error processing file %s: %w", filePath, err))
			continue
		}
		for _, family := range mf {
			s.convertMetricFamily(family, metrics)
		}
		successfulFiles[filePath] = t
	}

	s.addMtimeMetrics(successfulFiles, metrics)

	// Add error metric if any errors occurred
	if len(scrapeErrors) > 0 {
		s.addScrapeErrorMetric(metrics)
		// Log errors but continue with partial success
		for _, err := range scrapeErrors {
			s.logger.Error("Error during prometheus textfile scraping", zap.Error(err))
		}
	}

	return metrics, nil
}

// processFile reads and parses a prometheus textfile and returns prometheus protos of the parsed metrics
func (s *textfileScraper) processFile(path string) (time.Time, map[string]*dto.MetricFamily, error) {
	f, err := os.Open(path)
	if err != nil {
		return time.Time{}, nil, fmt.Errorf("failed to open file %q: %w", path, err)
	}
	defer f.Close()

	var parser expfmt.TextParser
	families, err := parser.TextToMetricFamilies(f)
	if err != nil {
		return time.Time{}, nil, fmt.Errorf("failed to parse metrics from %q: %w", path, err)
	}

	if s.hasTimestamps(families) {
		return time.Time{}, nil, fmt.Errorf("file %q contains unsupported client-side timestamps, skipping", path)
	}

	// Only stat the file once it has been parsed and validated
	stat, err := f.Stat()
	if err != nil {
		return time.Time{}, families, fmt.Errorf("failed to stat %q: %w", path, err)
	}

	return stat.ModTime(), families, nil
}

// hasTimestamps returns true when metrics contain unsupported timestamps
func (s *textfileScraper) hasTimestamps(parsedFamilies map[string]*dto.MetricFamily) bool {
	for _, mf := range parsedFamilies {
		for _, m := range mf.Metric {
			if m.TimestampMs != nil {
				return true
			}
		}
	}
	return false
}

// convertMetricFamily converts Prometheus metric family to OpenTelemetry metric
func (s *textfileScraper) convertMetricFamily(family *dto.MetricFamily, metrics pmetric.Metrics) {
	// Prometheus to OTLP metrics conversion is handled by the prometheus translator package
	// This is a simplified version - in a real implementation you would use the full translator

	timestamp := pcommon.NewTimestampFromTime(time.Now())

	for _, metric := range family.Metric {
		rm := metrics.ResourceMetrics().AppendEmpty()
		sm := rm.ScopeMetrics().AppendEmpty()
		sm.Scope().SetName("prometheustextfilereceiver")
		sm.Scope().SetVersion(s.buildInfo.Version)

		m := sm.Metrics().AppendEmpty()
		m.SetName(*family.Name)
		if family.Help != nil {
			m.SetDescription(*family.Help)
		}

		// Convert based on metric type
		switch family.GetType() {
		case dto.MetricType_COUNTER:
			counter := m.SetEmptySum()
			counter.SetIsMonotonic(true)
			counter.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
			dp := counter.DataPoints().AppendEmpty()
			dp.SetDoubleValue(metric.Counter.GetValue())
			dp.SetTimestamp(timestamp)
			s.setDataPointAttributes(dp.Attributes(), metric.Label)

		case dto.MetricType_GAUGE:
			gauge := m.SetEmptyGauge()
			dp := gauge.DataPoints().AppendEmpty()
			dp.SetDoubleValue(metric.Gauge.GetValue())
			dp.SetTimestamp(timestamp)
			s.setDataPointAttributes(dp.Attributes(), metric.Label)

		case dto.MetricType_SUMMARY:
			summary := m.SetEmptySummary()
			dp := summary.DataPoints().AppendEmpty()
			dp.SetCount(metric.Summary.GetSampleCount())
			dp.SetSum(metric.Summary.GetSampleSum())
			dp.SetTimestamp(timestamp)
			s.setDataPointAttributes(dp.Attributes(), metric.Label)

			for _, q := range metric.Summary.Quantile {
				qv := dp.QuantileValues().AppendEmpty()
				qv.SetQuantile(q.GetQuantile())
				qv.SetValue(q.GetValue())
			}

		case dto.MetricType_HISTOGRAM:
			histogram := m.SetEmptyHistogram()
			histogram.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
			dp := histogram.DataPoints().AppendEmpty()
			dp.SetCount(metric.Histogram.GetSampleCount())
			dp.SetSum(metric.Histogram.GetSampleSum())
			dp.SetTimestamp(timestamp)
			s.setDataPointAttributes(dp.Attributes(), metric.Label)

			// Convert buckets
			for _, bucket := range metric.Histogram.Bucket {
				bound := bucket.GetUpperBound()
				bucketCount := bucket.GetCumulativeCount()

				dp.ExplicitBounds().Append(bound)
				dp.BucketCounts().Append(bucketCount)
			}

		case dto.MetricType_UNTYPED:
			// Treat untyped as gauge
			gauge := m.SetEmptyGauge()
			dp := gauge.DataPoints().AppendEmpty()
			dp.SetDoubleValue(metric.Untyped.GetValue())
			dp.SetTimestamp(timestamp)
			s.setDataPointAttributes(dp.Attributes(), metric.Label)
		}
	}
}

// setDataPointAttributes sets the attributes of a data point from Prometheus labels
func (s *textfileScraper) setDataPointAttributes(attributes pcommon.Map, labels []*dto.LabelPair) {
	for _, label := range labels {
		attributes.PutStr(label.GetName(), label.GetValue())
	}
}

// addMtimeMetrics adds mtime metrics for files
func (s *textfileScraper) addMtimeMetrics(mtimes map[string]time.Time, metrics pmetric.Metrics) {
	if len(mtimes) == 0 {
		return
	}

	// Sort file paths for consistent output
	files := make([]string, 0, len(mtimes))
	for file := range mtimes {
		files = append(files, file)
	}
	sort.Strings(files)

	rm := metrics.ResourceMetrics().AppendEmpty()
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("prometheustextfilereceiver")
	sm.Scope().SetVersion(s.buildInfo.Version)

	m := sm.Metrics().AppendEmpty()
	m.SetName("textfile_mtime_seconds")
	m.SetDescription("Unixtime mtime of textfiles successfully read.")

	gauge := m.SetEmptyGauge()

	for _, file := range files {
		mtime := mtimes[file]
		dp := gauge.DataPoints().AppendEmpty()
		dp.SetDoubleValue(float64(mtime.Unix()))
		dp.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))
		dp.Attributes().PutStr("file", file)
	}
}

// addScrapeErrorMetric adds the scrape error metric
func (s *textfileScraper) addScrapeErrorMetric(metrics pmetric.Metrics) {
	rm := metrics.ResourceMetrics().AppendEmpty()
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("prometheustextfilereceiver")
	sm.Scope().SetVersion(s.buildInfo.Version)

	m := sm.Metrics().AppendEmpty()
	m.SetName("textfile_scrape_error")
	m.SetDescription("1 if there was an error opening or reading a file, 0 otherwise")

	gauge := m.SetEmptyGauge()
	dp := gauge.DataPoints().AppendEmpty()
	dp.SetDoubleValue(1.0)
	dp.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))
}
