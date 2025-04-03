// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package prometheustextfilereceiver

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap/confmaptest"
	"go.opentelemetry.io/collector/receiver/receivertest"
)

func TestLoadConfig(t *testing.T) {
	t.Parallel()

	factories, err := receivertest.NopFactories()
	assert.NoError(t, err)

	factory := NewFactory()
	factories.Receivers[typeStr] = factory
	cfg, err := confmaptest.LoadConf(filepath.Join("testdata", "config.yaml"))
	require.NoError(t, err)

	tests := []struct {
		id       component.ID
		expected component.Config
	}{
		{
			id: component.NewIDWithName(typeStr, ""),
			expected: &Config{
				Directories:     []string{},
				MetricsPath:     "/metrics",
				MetricsWaitTime: 500 * time.Millisecond,
			},
		},
		{
			id: component.NewIDWithName(typeStr, "customname"),
			expected: &Config{
				Directories:     []string{"/var/lib/node_exporter/textfile_collector"},
				MetricsPath:     "/metrics",
				MetricsWaitTime: 1 * time.Second,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.id.String(), func(t *testing.T) {
			factory := NewFactory()
			cfg := factory.CreateDefaultConfig()

			sub, err := cfg.Unmarshal(cfg.(*Config))
			require.NoError(t, err)
			assert.Equal(t, tt.expected, sub)
		})
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		desc        string
		cfg         *Config
		expectedErr string
	}{
		{
			desc: "Missing directories",
			cfg: &Config{
				Directories: []string{},
			},
			expectedErr: "at least one directory must be specified",
		},
		{
			desc: "Valid config",
			cfg: &Config{
				Directories: []string{"/var/lib/node_exporter/textfile_collector"},
			},
			expectedErr: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.expectedErr != "" {
				assert.EqualError(t, err, tc.expectedErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
