// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package mtailreceiver // import "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/mtailreceiver"

import (
	"errors"
	"fmt"
	"os"
	"time"
)

type Config struct {
	Programs             string        `mapstructure:"programs"`
	Logs                 []string      `mapstructure:"logs"`
	IgnoreRegex          string        `mapstructure:"ignore_regex"`
	CollectionInterval   time.Duration `mapstructure:"collection_interval"`
	PollInterval         time.Duration `mapstructure:"poll_interval"`
	OmitMetricSource     bool          `mapstructure:"omit_metric_source"`
	SyslogUseCurrentYear bool          `mapstructure:"syslog_use_current_year"`
}

func (c *Config) Validate() error {
	if c.Programs == "" {
		return errors.New("programs must be set")
	}
	if _, err := os.Stat(c.Programs); err != nil {
		return fmt.Errorf("stat programs: %w", err)
	}
	if len(c.Logs) == 0 {
		return errors.New("logs must contain at least one path pattern")
	}
	if c.CollectionInterval <= 0 {
		return errors.New("collection_interval must be greater than zero")
	}
	if c.PollInterval <= 0 {
		return errors.New("poll_interval must be greater than zero")
	}
	return nil
}
