// Copyright 2026-present Danh Tran Thanh
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/danhtran94/entdomain"
)

// LineItem lives in the "billing" slice and nests Invoice back (mutual Nest).
type LineItem struct {
	ent.Schema
}

// Fields of the LineItem.
func (LineItem) Fields() []ent.Field {
	return []ent.Field{
		field.String("description"),
		field.Int("qty"),
	}
}

// Edges of the LineItem.
func (LineItem) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("invoice", Invoice.Type).
			Ref("line_items").
			Unique().
			Annotations(entdomain.Edge(entdomain.Nest())),
	}
}

// Annotations of the LineItem.
func (LineItem) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entdomain.Entity(entdomain.Slice("billing")),
	}
}
