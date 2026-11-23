// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package relabelconfig

import (
	"testing"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/stretchr/testify/require"
)

func TestCompileAndApply(t *testing.T) {
	rules, err := (Rules{
		{"source_labels": []string{"a", "b"}, "target_label": "__tmp_combined"},
		{"source_labels": []string{"__tmp_combined"}, "target_label": "result", "regex": "(.+);(.+)", "replacement": "$2-$1"},
	}).Compile(model.LegacyValidation)
	require.NoError(t, err)
	require.Equal(t, relabel.Replace, rules[0].Action)
	require.Equal(t, ";", rules[0].Separator)
	builder := labels.NewBuilder(labels.FromStrings("__name__", "example", "a", "left", "b", "right"))
	keep, valid := Apply(builder, rules, model.LegacyValidation)
	require.True(t, keep)
	require.True(t, valid)
	require.Equal(t, "right-left", builder.Get("result"))
	require.Empty(t, builder.Get("__tmp_combined"))
}

func TestRejectInvalidRules(t *testing.T) {
	for _, rule := range (Rules{
		nil,
		{"action": "unknown"},
		{"action": "drop", "regex": "["},
		{"action": "hashmod", "target_label": "hash"},
		{"action": "drop", "extra": true},
	}) {
		_, err := (Rules{rule}).Compile(model.LegacyValidation)
		require.Error(t, err)
	}
}
