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
	"github.com/google/uuid"
)

// RingTag has a required, mutable foreign key bound to its edge. The key is
// FIQL-filterable and the edge is also nested, so ToDomain returns both RingID
// and Ring.
type RingTag struct {
	ent.Schema
}

// Fields of the RingTag.
func (RingTag) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("ring_id", uuid.UUID{}).
			Annotations(entdomain.Field(entdomain.FIQL(entdomain.EQ, entdomain.NEQ))),
		field.String("name").Unique(),
	}
}

// Edges of the RingTag.
func (RingTag) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("ring", Ring.Type).
			Ref("tags").
			Field("ring_id").
			Unique().
			Required().
			Annotations(entdomain.Edge(entdomain.Nest())),
	}
}

// Annotations of the RingTag.
func (RingTag) Annotations() []schema.Annotation {
	return []schema.Annotation{entdomain.Entity()}
}
