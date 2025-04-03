// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package prometheustextfilereceiver

import (
	"context"
	"errors"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/receiver"
	collectorscraper "go.opentelemetry.io/collector/scraper"
	"go.opentelemetry.io/collector/scraper/scraperhelper"

	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/prometheustextfilereceiver/internal/metadata"
)

const (
	typeStr = "prometheustextfile"
)

var errConfig = errors.New(`invalid config`)

// NewFactory creates a factory for PrometheusTextfile receiver
func NewFactory() receiver.Factory {
	return receiver.NewFactory(
		component.MustNewType(typeStr),
		createDefaultConfig,
		receiver.WithMetrics(newReceiver, metadata.MetricsStability),
	)
}

func createDefaultConfig() component.Config {
	cfg := scraperhelper.NewDefaultControllerConfig()
	cfg.CollectionInterval = 60 * time.Second
	cfg.InitialDelay = time.Second

	return &Config{
		ControllerConfig:     cfg,
		MetricsBuilderConfig: metadata.DefaultMetricsBuilderConfig(),
		Directories:          []string{},
	}
}

func newReceiver(
	_ context.Context,
	settings receiver.Settings,
	cfg component.Config,
	consumer consumer.Metrics,
) (receiver.Metrics, error) {
	promConfig, ok := cfg.(*Config)
	if !ok {
		return nil, errConfig
	}
	mp := newTextfileScraper(settings, promConfig)
	s, err := collectorscraper.NewMetrics(mp.scrape, collectorscraper.WithStart(mp.start))
	if err != nil {
		return nil, err
	}
	opt := scraperhelper.AddScraper(metadata.Type, s)

	return scraperhelper.NewMetricsController(
		&promConfig.ControllerConfig,
		settings,
		consumer,
		opt,
	)
}
