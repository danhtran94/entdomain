// Copyright 2019-present Facebook
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package entdomain

import "reflect"

// ApplyConfig holds the runtime configuration for domain apply operations.
type ApplyConfig struct {
	omitZeroVal bool
	omitNil     bool
	omitFields  map[string]bool
	onlyFields  map[string]bool
	appendEdges map[string]bool
}

// ApplyOption is a functional option for configuring ApplyConfig.
type ApplyOption func(*ApplyConfig)

// NewApplyConfig creates a new ApplyConfig with the given options applied.
func NewApplyConfig(opts ...ApplyOption) *ApplyConfig {
	c := &ApplyConfig{}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// OmitZeroVal returns an ApplyOption that skips fields with zero values.
func OmitZeroVal() ApplyOption {
	return func(c *ApplyConfig) {
		c.omitZeroVal = true
	}
}

// OmitNil returns an ApplyOption that skips pointer fields that are nil.
func OmitNil() ApplyOption {
	return func(c *ApplyConfig) {
		c.omitNil = true
	}
}

// OmitFields returns an ApplyOption that skips specific fields by name.
func OmitFields(fields ...string) ApplyOption {
	return func(c *ApplyConfig) {
		if c.omitFields == nil {
			c.omitFields = make(map[string]bool)
		}
		for _, f := range fields {
			c.omitFields[f] = true
		}
	}
}

// OnlyFields returns an ApplyOption that allowlists specific fields by name.
func OnlyFields(fields ...string) ApplyOption {
	return func(c *ApplyConfig) {
		if c.onlyFields == nil {
			c.onlyFields = make(map[string]bool)
		}
		for _, f := range fields {
			c.onlyFields[f] = true
		}
	}
}

// AppendEdge returns an ApplyOption that makes the given edge field use append
// instead of the default replace semantics.
func AppendEdge(field string) ApplyOption {
	return func(c *ApplyConfig) {
		if c.appendEdges == nil {
			c.appendEdges = make(map[string]bool)
		}
		c.appendEdges[field] = true
	}
}

// ShouldApply reports whether a non-pointer field should be applied.
// It checks onlyFields, omitFields, and OmitZeroVal (via reflect.Value.IsZero).
func (c *ApplyConfig) ShouldApply(field string, val any) bool {
	if len(c.onlyFields) > 0 && !c.onlyFields[field] {
		return false
	}
	if c.omitFields[field] {
		return false
	}
	if c.omitZeroVal && val != nil {
		rv := reflect.ValueOf(val)
		if rv.IsValid() && rv.IsZero() {
			return false
		}
	}
	return true
}

// ShouldApplyPtr reports whether a pointer field should be applied.
// It checks onlyFields, omitFields, and OmitNil (val == nil).
func (c *ApplyConfig) ShouldApplyPtr(field string, val any) bool {
	if len(c.onlyFields) > 0 && !c.onlyFields[field] {
		return false
	}
	if c.omitFields[field] {
		return false
	}
	if c.omitNil {
		rv := reflect.ValueOf(val)
		if !rv.IsValid() || rv.IsNil() {
			return false
		}
	}
	return true
}

// IsAppendEdge reports whether the given edge field should use append semantics.
func (c *ApplyConfig) IsAppendEdge(field string) bool {
	return c.appendEdges[field]
}
