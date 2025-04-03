// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

//go:generate mdatagen metadata.yaml

// Package prometheustextfilereceiver implements a receiver that can be used to
// scrape metrics from Prometheus text files in a directory, similar to the Node
// Exporter textfile collector.
package prometheustextfilereceiver // import "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/prometheustextfilereceiver"
