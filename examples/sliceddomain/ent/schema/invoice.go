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

// Invoice lives in the "billing" slice (mappers generated into the slice package).
type Invoice struct {
	ent.Schema
}

// Fields of the Invoice.
func (Invoice) Fields() []ent.Field {
	return []ent.Field{
		field.String("number"),
		field.Int("total"),
	}
}

// Edges of the Invoice.
func (Invoice) Edges() []ent.Edge {
	return []ent.Edge{
		// Same slice (billing → billing): nested structs, as without slices.
		edge.To("line_items", LineItem.Type).
			Annotations(entdomain.Edge(entdomain.Nest())),
		// Cross slice (billing → identity): IDs only, no identity import.
		edge.From("account", Account.Type).
			Ref("invoices").
			Unique().
			Annotations(entdomain.Edge(entdomain.IDs())),
	}
}

// Annotations of the Invoice.
func (Invoice) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entdomain.Entity(entdomain.Slice("billing")),
	}
}
