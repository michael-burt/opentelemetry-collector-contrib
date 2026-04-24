// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package mtailreceiver // import "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/mtailreceiver"

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/receiver"
	"go.uber.org/zap"

	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/mtailreceiver/internal/mtail/logline"
	mtailmetrics "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/mtailreceiver/internal/mtail/metrics"
	mtailruntime "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/mtailreceiver/internal/mtail/runtime"
	mtailtailer "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/mtailreceiver/internal/mtail/tailer"
	mtailwaker "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/mtailreceiver/internal/mtail/waker"
)

var _ receiver.Metrics = (*metricsReceiver)(nil)

type metricsReceiver struct {
	logger *zap.Logger
	cfg    *Config
	next   consumer.Metrics

	mu        sync.Mutex
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	store     *mtailmetrics.Store
	lines     chan *logline.LogLine
	runtime   *mtailruntime.Runtime
	tailer    *mtailtailer.Tailer
	startTime time.Time
}

func newMetricsReceiver(set receiver.Settings, cfg *Config, next consumer.Metrics) *metricsReceiver {
	return &metricsReceiver{
		logger: set.Logger,
		cfg:    cfg,
		next:   next,
	}
}

func (r *metricsReceiver) Start(ctx context.Context, _ component.Host) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.cancel != nil {
		return nil
	}
	if err := r.cfg.Validate(); err != nil {
		return err
	}

	runCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	r.startTime = time.Now().UTC()
	r.store = mtailmetrics.NewStore()
	r.lines = make(chan *logline.LogLine)

	runtimeOpts := make([]mtailruntime.Option, 0, 2)
	if r.cfg.OmitMetricSource {
		runtimeOpts = append(runtimeOpts, mtailruntime.OmitMetricSource())
	}
	if r.cfg.SyslogUseCurrentYear {
		runtimeOpts = append(runtimeOpts, mtailruntime.SyslogUseCurrentYear())
	}

	var err error
	r.runtime, err = mtailruntime.New(r.lines, &r.wg, r.cfg.Programs, r.store, runtimeOpts...)
	if err != nil {
		cancel()
		r.cancel = nil
		return err
	}

	patternWaker := mtailwaker.NewTimed(runCtx, r.cfg.PollInterval)
	streamWaker := mtailwaker.NewTimed(runCtx, r.cfg.PollInterval)
	tailerOpts := []mtailtailer.Option{
		mtailtailer.LogPatterns(r.cfg.Logs),
		mtailtailer.LogPatternPollWaker(patternWaker),
		mtailtailer.LogstreamPollWaker(streamWaker),
	}
	if r.cfg.IgnoreRegex != "" {
		tailerOpts = append(tailerOpts, mtailtailer.IgnoreRegex(r.cfg.IgnoreRegex))
	}

	r.tailer, err = mtailtailer.New(runCtx, &r.wg, r.lines, tailerOpts...)
	if err != nil {
		cancel()
		r.cancel = nil
		return err
	}

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		ticker := time.NewTicker(r.cfg.CollectionInterval)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				if err := r.emitMetrics(runCtx); err != nil {
					r.logger.Warn("failed to emit mtail metrics", zap.Error(err))
				}
			}
		}
	}()

	return nil
}

func (r *metricsReceiver) Shutdown(_ context.Context) error {
	r.mu.Lock()
	cancel := r.cancel
	r.cancel = nil
	r.mu.Unlock()

	if cancel == nil {
		return nil
	}
	cancel()
	r.wg.Wait()
	return nil
}

func (r *metricsReceiver) emitMetrics(ctx context.Context) error {
	md, err := convertStoreToMetrics(r.store, r.startTime)
	if err != nil {
		return err
	}
	if md.DataPointCount() == 0 {
		return nil
	}
	return r.next.ConsumeMetrics(ctx, md)
}
