// Copyright 2026-present Danh Tran Thanh
// SPDX-License-Identifier: Apache-2.0
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

package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/danhtran94/entdomain"
)

// Post holds the schema definition for the Post entity.
type Post struct {
	ent.Schema
}

// Fields of the Post.
func (Post) Fields() []ent.Field {
	return []ent.Field{
		field.String("title"),
		field.Bool("published").Default(false),
	}
}

// Edges of the Post.
func (Post) Edges() []ent.Edge {
	return []ent.Edge{
		// singular From: IDs → OwnerID int
		edge.From("owner", User.Type).
			Ref("posts").
			Unique().
			Annotations(entdomain.Edge(entdomain.IDs())),
		// plural From: IDs → PinnerIDs []int
		edge.From("pinners", User.Type).
			Ref("pinned_post").
			Annotations(entdomain.Edge(entdomain.IDs())),
	}
}

// Annotations of the Post.
func (Post) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entdomain.Entity(),
	}
}
