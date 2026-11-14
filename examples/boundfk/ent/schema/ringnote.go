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

// RingNote has an optional foreign key bound to its edge: NULL means no ring.
type RingNote struct {
	ent.Schema
}

// Fields of the RingNote.
func (RingNote) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("ring_id", uuid.UUID{}).Optional(),
		field.String("text").Unique(),
	}
}

// Edges of the RingNote.
func (RingNote) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("ring", Ring.Type).
			Ref("notes").
			Field("ring_id").
			Unique(),
	}
}

// Annotations of the RingNote.
func (RingNote) Annotations() []schema.Annotation {
	return []schema.Annotation{entdomain.Entity()}
}
