// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package prometheusrelabelprocessor

import (
	"context"

	"github.com/prometheus/common/model"
	"github.com/prometheus/otlptranslator"
	"github.com/prometheus/prometheus/model/relabel"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/otel/metric"
)

type dropReason uint8

const (
	accepted dropReason = iota
	ruleDrop
	unsupportedType
	unsupportedTemporality
	invalidInput
	ambiguousLabels
	invalidOutput
	incompleteFamily
	identityCollision
	reasonLimit
)

var reasonNames = [...]string{
	ruleDrop: "rule_drop", unsupportedType: "unsupported_type",
	unsupportedTemporality: "unsupported_temporality", invalidInput: "invalid_input",
	ambiguousLabels: "ambiguous_labels", invalidOutput: "invalid_output",
	incompleteFamily: "incomplete_family", identityCollision: "identity_collision",
}

type metricsProcessor struct {
	rules      []*relabel.Config
	dropped    metric.Int64Counter
	attributes [reasonLimit]metric.AddOption
}

func (p *metricsProcessor) process(ctx context.Context, md pmetric.Metrics) (pmetric.Metrics, error) {
	out, drops, err := relabelMetrics(ctx, md, p.rules)
	if err == nil {
		for reason := ruleDrop; reason < reasonLimit; reason++ {
			if drops[reason] != 0 {
				p.dropped.Add(ctx, drops[reason], p.attributes[reason])
			}
		}
	}
	return out, err
}

type point struct {
	kind      pmetric.MetricType
	number    pmetric.NumberDataPoint
	histogram pmetric.HistogramDataPoint
	summary   pmetric.SummaryDataPoint
}

func (pt point) attributes() (pcommon.Map, pcommon.Timestamp) {
	switch pt.kind {
	case pmetric.MetricTypeHistogram:
		return pt.histogram.Attributes(), pt.histogram.Timestamp()
	case pmetric.MetricTypeSummary:
		return pt.summary.Attributes(), pt.summary.Timestamp()
	default:
		return pt.number.Attributes(), pt.number.Timestamp()
	}
}

func visitPoints(m pmetric.Metric, visit func(point)) (int, dropReason) {
	switch m.Type() {
	case pmetric.MetricTypeGauge, pmetric.MetricTypeSum:
		var points pmetric.NumberDataPointSlice
		if m.Type() == pmetric.MetricTypeGauge {
			points = m.Gauge().DataPoints()
		} else {
			sum := m.Sum()
			points = sum.DataPoints()
			if !sum.IsMonotonic() {
				return points.Len(), unsupportedType
			}
			if sum.AggregationTemporality() != pmetric.AggregationTemporalityCumulative {
				return points.Len(), unsupportedTemporality
			}
		}
		for index := range points.Len() {
			visit(point{kind: m.Type(), number: points.At(index)})
		}
	case pmetric.MetricTypeHistogram:
		h := m.Histogram()
		if h.AggregationTemporality() != pmetric.AggregationTemporalityCumulative {
			return h.DataPoints().Len(), unsupportedTemporality
		}
		for index := range h.DataPoints().Len() {
			visit(point{kind: m.Type(), histogram: h.DataPoints().At(index)})
		}
	case pmetric.MetricTypeSummary:
		points := m.Summary().DataPoints()
		for index := range points.Len() {
			visit(point{kind: m.Type(), summary: points.At(index)})
		}
	case pmetric.MetricTypeExponentialHistogram:
		return m.ExponentialHistogram().DataPoints().Len(), unsupportedType
	default:
		return 0, unsupportedType
	}
	return 0, accepted
}

func descriptor(m pmetric.Metric) string {
	key := appendMap(addFields(nil, m.Unit(), m.Description()), m.Metadata())
	key = append(key, byte(m.Type()))
	switch m.Type() {
	case pmetric.MetricTypeSum:
		key = append(key, byte(m.Sum().AggregationTemporality()))
		if m.Sum().IsMonotonic() {
			key = append(key, 1)
		}
	case pmetric.MetricTypeHistogram:
		key = append(key, byte(m.Histogram().AggregationTemporality()))
	}
	return string(key)
}

func metricName(m pmetric.Metric, namer otlptranslator.MetricNamer) (string, error) {
	typ := otlptranslator.MetricType(otlptranslator.MetricTypeGauge)
	switch m.Type() {
	case pmetric.MetricTypeSum:
		typ = otlptranslator.MetricTypeMonotonicCounter
	case pmetric.MetricTypeHistogram:
		typ = otlptranslator.MetricTypeHistogram
	case pmetric.MetricTypeSummary:
		typ = otlptranslator.MetricTypeSummary
	}
	return namer.Build(otlptranslator.Metric{Name: m.Name(), Unit: m.Unit(), Type: typ})
}

type candidate struct {
	source               point
	metric               pmetric.Metric
	resource, scope      int
	time                 pcommon.Timestamp
	name, metricIdentity string
	edits                []attributeEdit
	stream               *stream
	reason               dropReason
}

type dropCounts [reasonLimit]int64

func relabelMetrics(ctx context.Context, md pmetric.Metrics, rules []*relabel.Config) (pmetric.Metrics, dropCounts, error) {
	var drops dropCounts
	if len(rules) == 0 {
		return md, drops, nil
	}

	projector := newProjection(rules)
	namer := otlptranslator.MetricNamer{WithMetricSuffixes: true}
	descriptors := make(map[string]string)
	conflicts := make(map[string]bool)
	streams := make(map[string]*stream)
	var candidates []candidate
	changed := false
	resources := md.ResourceMetrics()
	for ri := range resources.Len() {
		rm := resources.At(ri)
		resourceKey := appendMap(addFields(nil, rm.SchemaUrl()), rm.Resource().Attributes())
		scopes := rm.ScopeMetrics()
		for si := range scopes.Len() {
			sm := scopes.At(si)
			scope := sm.Scope()
			scopeKey := appendMap(addFields(resourceKey, sm.SchemaUrl(), scope.Name(), scope.Version()), scope.Attributes())
			metrics := sm.Metrics()
			for mi := range metrics.Len() {
				if err := ctx.Err(); err != nil {
					return pmetric.Metrics{}, drops, err
				}
				m := metrics.At(mi)
				family, nameErr := metricName(m, namer)
				header := descriptor(m)
				count, reason := visitPoints(m, func(pt point) {
					if ctx.Err() != nil {
						return
					}
					if nameErr != nil || !model.LegacyValidation.IsValidMetricName(family) {
						drops[invalidInput]++
						return
					}
					attrs, ts := pt.attributes()
					base, rejected := projector.base(attrs, pt.kind, family)
					if rejected != accepted {
						drops[rejected]++
						return
					}
					result, rejected := projector.run(base, family, pt)
					if rejected != accepted {
						drops[rejected]++
						return
					}
					edits, rejected := projector.edits(result, attrs)
					if rejected != accepted {
						drops[rejected]++
						return
					}
					name := m.Name()
					if result.name != family {
						name = result.name
					}
					changed = changed || result.changed
					metricKey := string(addFields(scopeKey, name))
					if existing, ok := descriptors[metricKey]; !ok {
						descriptors[metricKey] = header
					} else if existing != header {
						conflicts[metricKey] = true
					}
					originalAttrs := appendMap(nil, attrs)
					outAttrs := originalAttrs
					if len(edits) != 0 {
						copyAttrs := pcommon.NewMap()
						attrs.CopyTo(copyAttrs)
						applyEdits(copyAttrs, edits)
						outAttrs = appendMap(nil, copyAttrs)
					}
					original := string(addFields(scopeKey, m.Name(), header, string(originalAttrs)))
					key := string(append([]byte(metricKey), outAttrs...))
					index := len(candidates)
					st := streams[key]
					if st == nil {
						st = &stream{first: index, source: original}
						streams[key] = st
					}
					candidates = append(candidates, candidate{
						source: pt, metric: m, resource: ri, scope: si, name: name, time: ts,
						metricIdentity: metricKey, edits: edits, stream: st,
					})
					if st.first != index {
						st.add(candidates, index, original, ts)
					}
				})
				if reason != accepted {
					drops[reason] += int64(count)
				}
			}
		}
	}

	if err := ctx.Err(); err != nil {
		return pmetric.Metrics{}, drops, err
	}
	for i := range candidates {
		c := &candidates[i]
		if c.stream.collided || conflicts[c.metricIdentity] {
			c.reason = identityCollision
		}
		if c.reason != accepted {
			drops[c.reason]++
		}
	}
	for reason := ruleDrop; reason < reasonLimit; reason++ {
		changed = changed || drops[reason] != 0
	}
	if !changed {
		return md, drops, nil
	}
	out, err := rebuild(ctx, resources, candidates)
	return out, drops, err
}

func applyEdits(attrs pcommon.Map, edits []attributeEdit) {
	for _, edit := range edits {
		if edit.remove {
			attrs.Remove(edit.key)
		} else {
			attrs.PutStr(edit.key, edit.value)
		}
	}
}

func rebuild(ctx context.Context, input pmetric.ResourceMetricsSlice, candidates []candidate) (pmetric.Metrics, error) {
	out := pmetric.NewMetrics()
	ri, si := -1, -1
	var rm pmetric.ResourceMetrics
	var sm pmetric.ScopeMetrics
	metrics := make(map[string]pmetric.Metric)

	for i, candidate := range candidates {
		if i%128 == 0 {
			if err := ctx.Err(); err != nil {
				return pmetric.Metrics{}, err
			}
		}
		if candidate.reason != accepted {
			continue
		}
		inResource := input.At(candidate.resource)
		if candidate.resource != ri {
			ri, si = candidate.resource, -1
			rm = out.ResourceMetrics().AppendEmpty()
			rm.SetSchemaUrl(inResource.SchemaUrl())
			inResource.Resource().CopyTo(rm.Resource())
		}
		if candidate.scope != si {
			si = candidate.scope
			inScope := inResource.ScopeMetrics().At(si)
			sm = rm.ScopeMetrics().AppendEmpty()
			sm.SetSchemaUrl(inScope.SchemaUrl())
			inScope.Scope().CopyTo(sm.Scope())
			clear(metrics)
		}
		dest, exists := metrics[candidate.name]
		if !exists {
			dest = sm.Metrics().AppendEmpty()
			dest.SetName(candidate.name)
			dest.SetDescription(candidate.metric.Description())
			dest.SetUnit(candidate.metric.Unit())
			candidate.metric.Metadata().CopyTo(dest.Metadata())
			switch candidate.source.kind {
			case pmetric.MetricTypeGauge:
				dest.SetEmptyGauge()
			case pmetric.MetricTypeSum:
				sum := dest.SetEmptySum()
				sum.SetAggregationTemporality(candidate.metric.Sum().AggregationTemporality())
				sum.SetIsMonotonic(candidate.metric.Sum().IsMonotonic())
			case pmetric.MetricTypeHistogram:
				dest.SetEmptyHistogram().SetAggregationTemporality(candidate.metric.Histogram().AggregationTemporality())
			case pmetric.MetricTypeSummary:
				dest.SetEmptySummary()
			}
			metrics[candidate.name] = dest
		}
		var attrs pcommon.Map
		switch candidate.source.kind {
		case pmetric.MetricTypeGauge:
			pt := dest.Gauge().DataPoints().AppendEmpty()
			candidate.source.number.CopyTo(pt)
			attrs = pt.Attributes()
		case pmetric.MetricTypeSum:
			pt := dest.Sum().DataPoints().AppendEmpty()
			candidate.source.number.CopyTo(pt)
			attrs = pt.Attributes()
		case pmetric.MetricTypeHistogram:
			pt := dest.Histogram().DataPoints().AppendEmpty()
			candidate.source.histogram.CopyTo(pt)
			attrs = pt.Attributes()
		case pmetric.MetricTypeSummary:
			pt := dest.Summary().DataPoints().AppendEmpty()
			candidate.source.summary.CopyTo(pt)
			attrs = pt.Attributes()
		}
		applyEdits(attrs, candidate.edits)
	}
	return out, nil
}
