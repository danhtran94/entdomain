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

package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/danhtran94/entdomain"
	"github.com/danhtran94/entdomain/internal/testdata/domain"
	"github.com/google/uuid"
)

// User holds the schema definition for the User entity.
type User struct {
	ent.Schema
}

// Fields of the User.
func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("name"),
		field.String("bio").Optional(),
		field.Enum("status").Values("active", "inactive").Default("active"),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.String("username").Immutable().
			Annotations(entdomain.Field(entdomain.SkipProto())),
		field.Int("score").Optional(),
		field.UUID("external_id", uuid.UUID{}),
		field.Time("updated_at").Optional(),
		field.JSON("settings", map[string]any{}).Optional(),
		// Typed JSON field opted into proto via explicit ProtoType annotation.
		// Demonstrates entdomain.Field(entdomain.ProtoType(...)) usage.
		field.JSON("labels", map[string]any{}).
			Annotations(entdomain.Field(entdomain.ProtoType("google.protobuf.Struct", "google/protobuf/struct.proto"))),
		// []string JSON field: auto-infers repeated from slice type.
		field.JSON("tag_names", []string{}).
			Annotations(entdomain.Field(entdomain.ProtoType("string"))),
		// Custom struct JSON field mapped to a custom proto message via explicit annotation.
		field.JSON("metadata", domain.UserMetadata{}).
			Annotations(entdomain.Field(
				entdomain.ProtoType("UserMetadata", "entpb/user_metadata.proto").
					WithConversion("UserMetadataToProto(%s)", "UserMetadataFromProto(%s)"),
			)),
	}
}

// Edges of the User.
func (User) Edges() []ent.Edge {
	return []ent.Edge{
		// O2M To: IDs + Nest → PostIDs []int, Posts PostList
		edge.To("posts", Post.Type).
			Annotations(entdomain.Edge(entdomain.IDs(), entdomain.Nest())),
		// O2O To: IDs + Nest → PinnedPostID int, PinnedPost Post
		edge.To("pinned_post", Post.Type).
			Unique().
			Annotations(entdomain.Edge(entdomain.IDs(), entdomain.Nest())),
		// M2M To: IDs + Nest → TagIDs []int, Tags TagList
		edge.To("tags", Tag.Type).
			Annotations(entdomain.Edge(entdomain.IDs(), entdomain.Nest())),
	}
}

// Annotations of the User.
func (User) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entdomain.Entity(
			entdomain.VirtualField("full_name", entdomain.String),
			entdomain.VirtualField("is_premium", entdomain.Bool),
			entdomain.VirtualField("expires_at", entdomain.GoType("time", "Time"),
				entdomain.ProtoType("google.protobuf.Timestamp", "google/protobuf/timestamp.proto")),
			entdomain.VirtualField("subscription_duration", entdomain.GoType("time", "Duration")),
		),
	}
}
