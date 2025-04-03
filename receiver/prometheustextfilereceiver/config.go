// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package prometheustextfilereceiver

import (
	"fmt"

	"go.opentelemetry.io/collector/scraper/scraperhelper"

	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/prometheustextfilereceiver/internal/metadata"
)

// Config defines configuration for the PrometheusTextfile receiver.
type Config struct {
	scraperhelper.ControllerConfig `mapstructure:",squash"`
	metadata.MetricsBuilderConfig  `mapstructure:",squash"`

	// Directories is a list of directories to search for .prom files.
	// Multiple glob patterns can be specified.
	Directories []string `mapstructure:"directories"`
}

// Validate checks if the receiver configuration is valid
func (cfg *Config) Validate() error {
	if len(cfg.Directories) == 0 {
		return fmt.Errorf("at least one directory must be specified")
	}
	return nil
}
