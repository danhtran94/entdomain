package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"github.com/danhtran94/entdomain"
)

// Family is a regression fixture for irregular plurals: the generated
// ToDomain receiver must match ent core's `plural` (Families).
type Family struct {
	ent.Schema
}

func (Family) Fields() []ent.Field {
	return []ent.Field{field.String("name")}
}

func (Family) Annotations() []schema.Annotation {
	return []schema.Annotation{entdomain.Entity()}
}
