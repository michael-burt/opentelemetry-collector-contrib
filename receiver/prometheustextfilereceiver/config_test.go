// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package prometheustextfilereceiver

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidConfig(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig().(*Config)
	cfg.Directories = []string{"/path/to/metrics"}

	assert.NoError(t, cfg.Validate())
}

func TestInvalidConfig(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig().(*Config)
	cfg.Directories = []string{}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least one directory must be specified")
}

func TestConfigWithMultipleDirectories(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig().(*Config)
	cfg.Directories = []string{"/path/to/metrics1", "/path/to/metrics2"}

	assert.NoError(t, cfg.Validate())
	assert.Equal(t, 3, len(cfg.Directories))
}
