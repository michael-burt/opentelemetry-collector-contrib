// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Package relabelconfig adapts Collector configuration to Prometheus relabeling.
package relabelconfig

import (
	"fmt"
	"strings"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
	"go.yaml.in/yaml/v2"
)

// Rules is the map representation produced by Collector configuration decoding.
// Compile uses Prometheus's YAML unmarshaler for defaults and regular expressions.
type Rules []map[string]any

func (r Rules) Compile(scheme model.ValidationScheme) ([]*relabel.Config, error) {
	if len(r) == 0 {
		return nil, nil
	}
	raw, err := yaml.Marshal(r)
	if err != nil {
		return nil, err
	}
	var rules []*relabel.Config
	if err := yaml.UnmarshalStrict(raw, &rules); err != nil {
		return nil, err
	}
	for i, rule := range rules {
		if rule == nil {
			return nil, fmt.Errorf("rule %d is null", i+1)
		}
		if err := rule.Validate(scheme); err != nil {
			return nil, fmt.Errorf("rule %d: %w", i+1, err)
		}
	}
	return rules, nil
}

// Apply leaves the final labels in builder. An invalid result is distinct from a rule drop.
func Apply(builder *labels.Builder, rules []*relabel.Config, scheme model.ValidationScheme) (keep, valid bool) {
	if !relabel.ProcessBuilder(builder, rules...) {
		return false, true
	}
	valid = scheme.IsValidMetricName(builder.Get(model.MetricNameLabel))
	builder.Range(func(label labels.Label) {
		if strings.HasPrefix(label.Name, "__tmp") {
			builder.Del(label.Name)
			return
		}
		valid = valid && scheme.IsValidLabelName(label.Name) && model.LabelValue(label.Value).IsValid()
	})
	return true, valid
}
