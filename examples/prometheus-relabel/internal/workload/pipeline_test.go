// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package workload

import (
	"context"
	"runtime"
	"testing"

	"github.com/prometheus/common/model"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/processor"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/prometheusremotewrite"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/prometheusrelabelprocessor"
)

func BenchmarkPipeline(b *testing.B) {
	for _, shape := range []struct {
		name  string
		mixed bool
	}{{"scalar", false}, {"mixed", true}} {
		for _, policy := range []string{"empty", "no-match", "drop", "rename"} {
			for _, mode := range []string{"baseline", "processor", "exporter"} {
				b.Run(shape.name+"/"+policy+"/"+mode, func(b *testing.B) {
					md := Metrics(256, shape.mixed)
					settings := prometheusremotewrite.Settings{
						DisableTargetInfo: true, TranslationStrategy: "UnderscoreEscapingWithSuffixes",
					}
					if mode == "exporter" {
						rules, err := Rules(policy).Compile(model.LegacyValidation)
						if err != nil {
							b.Fatal(err)
						}
						settings.WriteRelabelConfigs = rules
					}
					sink, err := consumer.NewMetrics(func(_ context.Context, metrics pmetric.Metrics) error {
						out, err := prometheusremotewrite.FromMetrics(metrics, settings)
						runtime.KeepAlive(out)
						return err
					})
					if err != nil {
						b.Fatal(err)
					}
					var pipeline consumer.Metrics = sink
					if mode == "processor" {
						factory := prometheusrelabelprocessor.NewFactory()
						instance, err := factory.CreateMetrics(b.Context(), processor.Settings{
							ID: component.NewID(factory.Type()), TelemetrySettings: componenttest.NewNopTelemetrySettings(),
						}, &prometheusrelabelprocessor.Config{MetricRelabelConfigs: Rules(policy)}, sink)
						if err != nil {
							b.Fatal(err)
						}
						pipeline = instance
					}
					b.ReportAllocs()
					for b.Loop() {
						if err := pipeline.ConsumeMetrics(b.Context(), md); err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		}
	}
}
