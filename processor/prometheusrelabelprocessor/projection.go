// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package prometheusrelabelprocessor

import (
	"math"
	"strconv"
	"strings"

	"github.com/prometheus/common/model"
	"github.com/prometheus/otlptranslator"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/prometheus/relabelconfig"
)

type projection struct {
	rules   []*relabel.Config
	scratch labels.ScratchBuilder
	builder *labels.Builder
	namer   otlptranslator.LabelNamer
	keys    map[string]string
	names   map[string]string
}

type projected struct {
	name    string
	initial labels.Labels
	final   labels.Labels
	changed bool
}

func newProjection(rules []*relabel.Config) *projection {
	return &projection{
		rules: rules, scratch: labels.NewScratchBuilder(0), builder: labels.NewBuilder(labels.EmptyLabels()),
		namer: otlptranslator.LabelNamer{PreserveMultipleUnderscores: true},
		keys:  make(map[string]string), names: make(map[string]string),
	}
}

func (p *projection) base(attrs pcommon.Map, typ pmetric.MetricType, name string) (labels.Labels, dropReason) {
	p.scratch.Reset()
	clear(p.keys)
	for key, value := range attrs.All() {
		label, ok := p.names[key]
		if !ok {
			var err error
			label, err = p.namer.Build(key)
			if err != nil || !model.LegacyValidation.IsValidLabelName(label) {
				return labels.EmptyLabels(), invalidInput
			}
			p.names[key] = label
		}
		if _, exists := p.keys[label]; exists ||
			label == model.MetricNameLabel ||
			(typ == pmetric.MetricTypeHistogram && label == "le") ||
			(typ == pmetric.MetricTypeSummary && label == "quantile") {
			return labels.EmptyLabels(), ambiguousLabels
		}
		p.keys[label] = key
		if str := value.AsString(); str != "" {
			p.scratch.Add(label, str)
		}
	}
	p.scratch.Add(model.MetricNameLabel, name)
	p.scratch.Sort()
	return p.scratch.Labels(), accepted
}

func (p *projection) run(base labels.Labels, family string, pt point) (projected, dropReason) {
	result := projected{name: family, initial: base}
	total, kept := 0, 0
	reason := accepted
	if !pt.constituents(func(suffix, structural, value string) {
		total++
		p.builder.Reset(base)
		if suffix != "" {
			p.builder.Set(model.MetricNameLabel, family+suffix)
		}
		if structural != "" {
			p.builder.Set(structural, value)
		}
		keep, valid := relabelconfig.Apply(p.builder, p.rules, model.LegacyValidation)
		if !keep {
			return
		}
		if !valid {
			reason = invalidOutput
			return
		}

		name := p.builder.Get(model.MetricNameLabel)
		rename, ok := strings.CutSuffix(name, suffix)
		if !ok || !model.LegacyValidation.IsValidMetricName(rename) {
			reason = incompleteFamily
			return
		}
		if structural != "" {
			if p.builder.Get(structural) != value {
				reason = incompleteFamily
				return
			}
			p.builder.Del(structural)
		}
		if (pt.kind == pmetric.MetricTypeHistogram && p.builder.Get("le") != "") ||
			(pt.kind == pmetric.MetricTypeSummary && p.builder.Get("quantile") != "") {
			reason = incompleteFamily
			return
		}
		p.builder.Set(model.MetricNameLabel, rename)
		final := p.builder.Labels()
		if kept == 0 {
			result.name, result.final = rename, final
		} else if !labels.Equal(result.final, final) && reason == accepted {
			reason = incompleteFamily
		}
		kept++
	}) {
		return result, invalidInput
	}
	if reason != accepted {
		return result, reason
	}
	if kept == 0 {
		return result, ruleDrop
	}
	if kept != total {
		return result, incompleteFamily
	}
	result.changed = !labels.Equal(result.initial, result.final)
	return result, accepted
}

type attributeEdit struct {
	key    string
	value  string
	remove bool
}

func (p *projection) edits(result projected, attrs pcommon.Map) ([]attributeEdit, dropReason) {
	if !result.changed {
		return nil, accepted
	}
	var edits []attributeEdit
	result.initial.Range(func(l labels.Label) {
		if l.Name == model.MetricNameLabel || result.final.Get(l.Name) == l.Value {
			return
		}
		edits = append(edits, attributeEdit{key: p.keys[l.Name], remove: true})
	})
	reason := accepted
	result.final.Range(func(l labels.Label) {
		if l.Name == model.MetricNameLabel || result.initial.Get(l.Name) == l.Value {
			return
		}
		if _, exists := attrs.Get(l.Name); exists && p.keys[l.Name] != l.Name {
			reason = ambiguousLabels
		}
		edits = append(edits, attributeEdit{key: l.Name, value: l.Value})
	})
	return edits, reason
}

func (pt point) constituents(visit func(suffix, structural, value string)) bool {
	switch pt.kind {
	case pmetric.MetricTypeHistogram:
		bounds := pt.histogram.ExplicitBounds()
		counts := pt.histogram.BucketCounts()
		if counts.Len() != bounds.Len()+1 &&
			!(pt.histogram.Flags().NoRecordedValue() && bounds.Len() == 0 && counts.Len() == 0) {
			return false
		}
		prev := math.Inf(-1)
		for i := range bounds.Len() {
			bound := bounds.At(i)
			if math.IsNaN(bound) || math.IsInf(bound, 0) || bound <= prev {
				return false
			}
			prev = bound
			visit("_bucket", "le", strconv.FormatFloat(bound, 'f', -1, 64))
		}
		visit("_bucket", "le", "+Inf")
		visit("_count", "", "")
		if pt.histogram.HasSum() {
			visit("_sum", "", "")
		}
	case pmetric.MetricTypeSummary:
		quantiles := pt.summary.QuantileValues()
		prev := -1.0
		for i := range quantiles.Len() {
			q := quantiles.At(i).Quantile()
			if math.IsNaN(q) || q < 0 || q > 1 || q <= prev {
				return false
			}
			prev = q
			visit("", "quantile", strconv.FormatFloat(q, 'f', -1, 64))
		}
		visit("_count", "", "")
		visit("_sum", "", "")
	default:
		visit("", "", "")
	}
	return true
}
