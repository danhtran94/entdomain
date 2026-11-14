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

// Ring owns three kinds of children that bind their foreign key to a field.
type Ring struct {
	ent.Schema
}

// Fields of the Ring.
func (Ring) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
	}
}

// Edges of the Ring.
func (Ring) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("versions", RingVersion.Type),
		edge.To("notes", RingNote.Type),
		edge.To("tags", RingTag.Type),
	}
}

// Annotations of the Ring.
func (Ring) Annotations() []schema.Annotation {
	return []schema.Annotation{entdomain.Entity()}
}
