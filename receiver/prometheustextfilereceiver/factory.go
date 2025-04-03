// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package prometheustextfilereceiver // import "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/prometheustextfilereceiver"

import (
	"context"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/scraperhelper"
)

const (
	typeStr   = "prometheustextfile"
	stability = component.StabilityLevelDevelopment
)

// NewFactory creates a factory for PrometheusTextfile receiver
func NewFactory() receiver.Factory {
	return receiver.NewFactory(
		typeStr,
		createDefaultConfig,
		receiver.WithMetrics(createMetricsReceiver, stability))
}

func createMetricsReceiver(
	ctx context.Context,
	settings receiver.CreateSettings,
	config component.Config,
	consumer consumer.Metrics,
) (receiver.Metrics, error) {
	cfg := config.(*Config)

	prometheusTextfileScraper := newTextfileScraper(settings, cfg)

	scraper, err := scraperhelper.NewScraper(
		typeStr,
		prometheusTextfileScraper.scrape,
		scraperhelper.WithStart(prometheusTextfileScraper.start),
		scraperhelper.WithShutdown(prometheusTextfileScraper.shutdown),
	)
	if err != nil {
		return nil, err
	}

	return scraperhelper.NewScraperControllerReceiver(
		&cfg.ScraperControllerSettings,
		settings,
		consumer,
		scraperhelper.AddScraper(scraper),
	)
} 