package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"github.com/danhtran94/entdomain"
)

// Child is a regression fixture for irregular plurals: the generated
// ToDomain receiver must match ent core's `plural` (Children).
type Child struct {
	ent.Schema
}

func (Child) Fields() []ent.Field {
	return []ent.Field{field.String("name")}
}

func (Child) Annotations() []schema.Annotation {
	return []schema.Annotation{entdomain.Entity()}
}
