// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package prometheusrelabelprocessor

import (
	"encoding/binary"
	"math"
	"slices"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

// The keys encode typed, order-independent attribute values. Length prefixes
// separate fields, including arbitrary keys and strings, without hash collisions.
func appendString(key []byte, value string) []byte {
	return append(binary.AppendUvarint(key, uint64(len(value))), value...)
}

func appendMap(key []byte, attrs pcommon.Map) []byte {
	key = binary.AppendUvarint(key, uint64(attrs.Len()))
	for _, name := range slices.Sorted(mapKeys(attrs)) {
		value, _ := attrs.Get(name)
		key = appendValue(appendString(key, name), value)
	}
	return key
}

func mapKeys(attrs pcommon.Map) func(func(string) bool) {
	return func(yield func(string) bool) {
		for name := range attrs.All() {
			if !yield(name) {
				return
			}
		}
	}
}

func appendValue(key []byte, value pcommon.Value) []byte {
	key = append(key, byte(value.Type()))
	switch value.Type() {
	case pcommon.ValueTypeStr:
		return appendString(key, value.Str())
	case pcommon.ValueTypeInt:
		return binary.AppendVarint(key, value.Int())
	case pcommon.ValueTypeDouble:
		return binary.LittleEndian.AppendUint64(key, math.Float64bits(value.Double()))
	case pcommon.ValueTypeBool:
		if value.Bool() {
			return append(key, 1)
		}
		return append(key, 0)
	case pcommon.ValueTypeMap:
		return appendMap(key, value.Map())
	case pcommon.ValueTypeSlice:
		items := value.Slice()
		key = binary.AppendUvarint(key, uint64(items.Len()))
		for i := range items.Len() {
			key = appendValue(key, items.At(i))
		}
		return key
	case pcommon.ValueTypeBytes:
		b := value.Bytes()
		key = binary.AppendUvarint(key, uint64(b.Len()))
		return append(key, b.AsRaw()...)
	default:
		return key
	}
}

func addFields(key []byte, values ...string) []byte {
	for _, value := range values {
		key = appendString(key, value)
	}
	return key
}

type stream struct {
	first      int
	source     string
	collided   bool
	timestamps map[pcommon.Timestamp]int
}

func (s *stream) add(candidates []candidate, index int, original string, ts pcommon.Timestamp) {
	if s.source != original {
		s.collided = true
	}
	if s.timestamps == nil {
		if ts == candidates[s.first].time {
			candidates[s.first].reason = identityCollision
			candidates[index].reason = identityCollision
			return
		}
		s.timestamps = map[pcommon.Timestamp]int{candidates[s.first].time: s.first}
	}
	if previous, exists := s.timestamps[ts]; exists {
		candidates[previous].reason = identityCollision
		candidates[index].reason = identityCollision
		return
	}
	s.timestamps[ts] = index
}
