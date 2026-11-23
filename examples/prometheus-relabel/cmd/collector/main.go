// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"log"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/provider/envprovider"
	"go.opentelemetry.io/collector/confmap/provider/fileprovider"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
	"go.opentelemetry.io/collector/service/telemetry/otelconftelemetry"

	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/prometheusremotewriteexporter"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/prometheusrelabelprocessor"
)

func main() {
	opts := otelcol.CollectorSettings{
		BuildInfo: component.BuildInfo{
			Command: "relabel-collector", Description: "Prometheus relabel prototype", Version: "0.165.1-dev",
		},
		Factories: func() (otelcol.Factories, error) {
			input := otlpreceiver.NewFactory()
			filter := prometheusrelabelprocessor.NewFactory()
			output := prometheusremotewriteexporter.NewFactory()
			return otelcol.Factories{
				Telemetry:  otelconftelemetry.NewFactory(),
				Receivers:  map[component.Type]receiver.Factory{input.Type(): input},
				Processors: map[component.Type]processor.Factory{filter.Type(): filter},
				Exporters:  map[component.Type]exporter.Factory{output.Type(): output},
			}, nil
		},
		ConfigProviderSettings: otelcol.ConfigProviderSettings{
			ResolverSettings: confmap.ResolverSettings{
				ProviderFactories: []confmap.ProviderFactory{fileprovider.NewFactory(), envprovider.NewFactory()},
			},
		},
	}
	if err := otelcol.NewCommand(opts).Execute(); err != nil {
		log.Fatal(err)
	}
}
