// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package prometheustextfilereceiver // import "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/prometheustextfilereceiver"

import (
	"fmt"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/receiver/scraperhelper"
)

// Config defines configuration for the PrometheusTextfile receiver.
type Config struct {
	scraperhelper.ScraperControllerSettings `mapstructure:",squash"`

	// Directories is a list of directories to search for .prom files.
	// Multiple glob patterns can be specified.
	Directories []string `mapstructure:"directories"`

	// MetricsPath is the path where file metrics will be sent (default: "/metrics")
	MetricsPath string `mapstructure:"metrics_path"`

	// MetricsWaitTime is the time to wait until the metrics from the files are scraped.
	// Default: 0.5s
	MetricsWaitTime time.Duration `mapstructure:"metrics_wait_time"`
}

// Validate checks if the receiver configuration is valid
func (cfg *Config) Validate() error {
	if len(cfg.Directories) == 0 {
		return fmt.Errorf("at least one directory must be specified")
	}
	return nil
}

func createDefaultConfig() component.Config {
	return &Config{
		ScraperControllerSettings: scraperhelper.NewDefaultScraperControllerSettings(typeStr),
		Directories:               []string{},
		MetricsPath:               "/metrics",
		MetricsWaitTime:           500 * time.Millisecond,
	}
}
