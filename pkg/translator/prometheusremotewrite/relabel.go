// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package prometheusremotewrite

import (
	"errors"
	"strings"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/prometheus/prometheus/prompb"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/prometheus/relabelconfig"
)

var errInvalidRelabeledSeries = errors.New("write relabeling produced an invalid metric name or label set")

type seriesRelabeler struct {
	rules   []*relabel.Config
	scheme  model.ValidationScheme
	scratch labels.ScratchBuilder
	builder *labels.Builder
	invalid bool
}

func newSeriesRelabeler(settings Settings) *seriesRelabeler {
	if len(settings.WriteRelabelConfigs) == 0 {
		return nil
	}
	_, utf8Allowed := getTranslationConfiguration(settings)
	scheme := model.LegacyValidation
	if utf8Allowed {
		scheme = model.UTF8Validation
	}
	return &seriesRelabeler{
		rules: settings.WriteRelabelConfigs, scheme: scheme,
		scratch: labels.NewScratchBuilder(0), builder: labels.NewBuilder(labels.EmptyLabels()),
	}
}

func (r *seriesRelabeler) apply(input []prompb.Label) []prompb.Label {
	if r == nil {
		return input
	}
	r.scratch.Reset()
	for _, label := range input {
		r.scratch.Add(label.Name, label.Value)
	}
	r.scratch.Sort()
	initial := r.scratch.Labels()
	r.builder.Reset(initial)
	keep, valid := relabelconfig.Apply(r.builder, r.rules, r.scheme)
	if !valid {
		r.invalid = true
	}
	if !keep || !valid {
		return nil
	}
	result := r.builder.Labels()
	if labels.Equal(initial, result) {
		return input
	}
	out := make([]prompb.Label, 0, result.Len())
	result.Range(func(label labels.Label) {
		out = append(out, prompb.Label{Name: label.Name, Value: label.Value})
	})
	return out
}

func (r *seriesRelabeler) err() error {
	if r != nil && r.invalid {
		return errInvalidRelabeledSeries
	}
	return nil
}

func seriesName(ls []prompb.Label) string {
	for _, label := range ls {
		if label.Name == model.MetricNameLabel {
			return label.Value
		}
	}
	return ""
}

// relabeledFamily returns the v1 metadata name for a surviving series. A
// flattened family member without its role suffix cannot carry family metadata.
func relabeledFamily(family, before, after string) string {
	if before == family {
		return after
	}
	suffix, ok := strings.CutPrefix(before, family)
	if !ok {
		return ""
	}
	name, ok := strings.CutSuffix(after, suffix)
	if !ok {
		return ""
	}
	return name
}
