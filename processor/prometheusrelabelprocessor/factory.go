// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Package prometheusrelabelprocessor applies Prometheus relabeling to OTLP metrics.
package prometheusrelabelprocessor

import (
	"context"
	"fmt"

	"github.com/prometheus/common/model"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/processor/processorhelper"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/prometheus/relabelconfig"
)

type Config struct {
	MetricRelabelConfigs relabelconfig.Rules `mapstructure:"metric_relabel_configs"`
}

func (cfg *Config) Validate() error {
	_, err := cfg.MetricRelabelConfigs.Compile(model.LegacyValidation)
	if err != nil {
		return fmt.Errorf("metric_relabel_configs: %w", err)
	}
	return nil
}

func NewFactory() processor.Factory {
	return processor.NewFactory(
		component.MustNewType("prometheus_relabel"),
		func() component.Config { return &Config{} },
		processor.WithMetrics(createMetrics, component.StabilityLevelDevelopment),
	)
}

func createMetrics(ctx context.Context, set processor.Settings, cfg component.Config, next consumer.Metrics) (processor.Metrics, error) {
	rules, err := cfg.(*Config).MetricRelabelConfigs.Compile(model.LegacyValidation)
	if err != nil {
		return nil, fmt.Errorf("metric_relabel_configs: %w", err)
	}
	counter, err := set.MeterProvider.Meter("prometheus_relabel").Int64Counter(
		"otelcol_processor_prometheus_relabel_dropped_data_points",
		metric.WithUnit("{datapoint}"),
		metric.WithDescription("Input data points dropped during Prometheus relabeling"),
	)
	if err != nil {
		return nil, err
	}
	p := &metricsProcessor{rules: rules, dropped: counter}
	for reason := ruleDrop; reason < reasonLimit; reason++ {
		p.attributes[reason] = metric.WithAttributeSet(attribute.NewSet(attribute.String("reason", reasonNames[reason])))
	}
	return processorhelper.NewMetrics(ctx, set, cfg, next, p.process,
		processorhelper.WithCapabilities(consumer.Capabilities{MutatesData: false}))
}
