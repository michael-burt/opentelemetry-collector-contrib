// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package mtailreceiver

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConfigValidate(t *testing.T) {
	cfg := &Config{
		Programs:           "testdata/lifecycle/simple.mtail",
		Logs:               []string{"testdata/lifecycle/simple.log"},
		CollectionInterval: time.Second,
		PollInterval:       100 * time.Millisecond,
	}
	require.NoError(t, cfg.Validate())
}

func TestConfigValidateRequiresPrograms(t *testing.T) {
	cfg := &Config{
		Logs:               []string{"testdata/lifecycle/simple.log"},
		CollectionInterval: time.Second,
		PollInterval:       100 * time.Millisecond,
	}
	require.Error(t, cfg.Validate())
}
