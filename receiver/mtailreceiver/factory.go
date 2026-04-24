// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package mtailreceiver // import "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/mtailreceiver"

import (
	"context"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/receiver"

	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/mtailreceiver/internal/metadata"
)

func NewFactory() receiver.Factory {
	return receiver.NewFactory(
		metadata.Type,
		createDefaultConfig,
		receiver.WithMetrics(createMetricsReceiver, metadata.MetricsStability),
	)
}

func createDefaultConfig() component.Config {
	return &Config{
		CollectionInterval: 10 * time.Second,
		PollInterval:       250 * time.Millisecond,
	}
}

func createMetricsReceiver(
	_ context.Context,
	set receiver.Settings,
	cfg component.Config,
	next consumer.Metrics,
) (receiver.Metrics, error) {
	return newMetricsReceiver(set, cfg.(*Config), next), nil
}
