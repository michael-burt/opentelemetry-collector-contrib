// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package prometheustextfilereceiver

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/receiver/receivertest"
)

var typ = component.MustNewType("prometheustextfile")

func TestCreateDefaultConfig(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig()

	assert.NotNil(t, cfg, "failed to create default configuration")
	assert.NoError(t, componenttest.CheckConfigStruct(cfg))

	promCfg, ok := cfg.(*Config)
	require.True(t, ok, "config is not of type *Config")
	
	assert.Equal(t, 60*time.Second, promCfg.CollectionInterval)
	assert.Equal(t, time.Second, promCfg.InitialDelay)
	assert.Empty(t, promCfg.Directories)
}

func TestFactoryType(t *testing.T) {
	factory := NewFactory()
	assert.Equal(t, component.MustNewType("prometheustextfile"), factory.Type())
}

func TestNewReceiver(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig().(*Config)
	cfg.Directories = []string{"/path/to/metrics"}
	
	consumer := consumertest.NewNop()
	receiver, err := newReceiver(
		context.Background(),
		receivertest.NewNopSettings(typ),
		cfg,
		consumer,
	)
	
	assert.NoError(t, err)
	assert.NotNil(t, receiver)
}

func TestNewReceiverWithInvalidConfig(t *testing.T) {
	invalidCfg := &struct{}{}
	
	consumer := consumertest.NewNop()
	receiver, err := newReceiver(
		context.Background(),
		receivertest.NewNopSettings(typ),
		invalidCfg,
		consumer,
	)
	
	assert.Error(t, err)
	assert.Nil(t, receiver)
	assert.Equal(t, errConfig, err)
}
