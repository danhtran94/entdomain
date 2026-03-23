package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema"
	"github.com/danhtran94/entdomain"
)

// User holds the schema definition for the User entity.
type User struct {
	ent.Schema
}

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("name"),
		field.Int("age").Optional(),
	}
}

func (User) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entdomain.Entity(),
	}
}
