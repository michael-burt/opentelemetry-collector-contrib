module github.com/open-telemetry/opentelemetry-collector-contrib/processor/prometheusrelabelprocessor

go 1.26.7

require (
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/prometheus v0.165.1
	github.com/prometheus/common v0.74.0
	github.com/prometheus/otlptranslator v1.0.0
	github.com/prometheus/prometheus v0.316.0
	github.com/stretchr/testify v1.12.1
	go.opentelemetry.io/collector/component v1.71.1-0.20261119222305-0c7a74cd0a4a
	go.opentelemetry.io/collector/consumer v1.71.1-0.20261119222305-0c7a74cd0a4a
	go.opentelemetry.io/collector/pdata v1.71.1-0.20261119222305-0c7a74cd0a4a
	go.opentelemetry.io/collector/processor v1.71.1-0.20261119222305-0c7a74cd0a4a
	go.opentelemetry.io/collector/processor/processorhelper v0.165.1-0.20261119222305-0c7a74cd0a4a
	go.opentelemetry.io/otel v1.47.0
	go.opentelemetry.io/otel/metric v1.47.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/grafana/regexp v0.0.0-20250905093917-f7b3be9d1853 // indirect
	github.com/hashicorp/go-version v1.9.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	github.com/prometheus/client_model v0.6.3 // indirect
	go.opentelemetry.io/collector/featuregate v1.71.1-0.20261119222305-0c7a74cd0a4a // indirect
	go.opentelemetry.io/collector/internal/componentalias v0.165.0 // indirect
	go.opentelemetry.io/collector/pipeline v1.71.0 // indirect
	go.opentelemetry.io/otel/trace v1.47.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.28.0 // indirect
	go.yaml.in/yaml/v2 v2.4.5 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/text v0.43.0 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
)

replace github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/prometheus => ../../pkg/translator/prometheus
