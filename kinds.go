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

package entdomain

import (
	"fmt"

	"entgo.io/ent/entc/gen"
	"entgo.io/ent/schema/field"
)

// FieldKind is the canonical broad classification of an ent field for the FIQL
// pipeline. Each generator (FIQL template, proto, domain) dispatches on this
// kind for the cases that diverge between ent types but converge into one
// behaviour from FIQL's perspective. Narrow type granularity (e.g. int32 vs
// int64) stays in each generator's sub-switch; the kind exists to centralise
// the outer ent-type-to-FIQL-kind mapping and the UUID/GoType gate.
type FieldKind int

const (
	// KindUnsupported indicates the field has no FIQL representation. The
	// accompanying reason string from resolveFieldKind explains why.
	KindUnsupported FieldKind = iota
	KindString
	KindInt
	KindFloat
	KindBool
	KindTime
	KindEnum
	KindUUID
)

// String returns the FIQL template kind tag (e.g. "String", "UUID") used by
// the FIQL template's else-branch to instantiate the matching FIQL{kind} type.
// KindUnsupported returns "" — preserved as the existing skip sentinel.
func (k FieldKind) String() string {
	switch k {
	case KindString:
		return "String"
	case KindInt:
		return "Int"
	case KindFloat:
		return "Float"
	case KindBool:
		return "Bool"
	case KindTime:
		return "Time"
	case KindEnum:
		return "Enum"
	case KindUUID:
		return "UUID"
	default:
		return ""
	}
}

// resolveFieldKind classifies an ent field for the FIQL pipeline. Returns the
// kind plus a non-empty reason string when KindUnsupported is returned —
// proto generation surfaces these reasons in the .skipped.json artifact so
// silently-dropped fields are diagnosable.
//
// UUID fields are classified KindUUID only when the underlying Go type is the
// canonical github.com/google/uuid.UUID. Custom GoType-overridden UUID fields
// fall back to KindUnsupported (ent generates predicate methods using the
// custom type, so wiring them into FIQLUUID would produce a signature
// mismatch — see ENTD-001).
func resolveFieldKind(f *gen.Field) (FieldKind, string) {
	switch f.Type.Type {
	case field.TypeString:
		return KindString, ""
	case field.TypeInt, field.TypeInt8, field.TypeInt16, field.TypeInt32, field.TypeInt64,
		field.TypeUint, field.TypeUint8, field.TypeUint16, field.TypeUint32, field.TypeUint64:
		return KindInt, ""
	case field.TypeFloat32, field.TypeFloat64:
		return KindFloat, ""
	case field.TypeBool:
		return KindBool, ""
	case field.TypeTime:
		return KindTime, ""
	case field.TypeEnum:
		return KindEnum, ""
	case field.TypeUUID:
		if f.Type.RType != nil &&
			f.Type.RType.PkgPath == "github.com/google/uuid" &&
			f.Type.RType.Ident == "uuid.UUID" {
			return KindUUID, ""
		}
		return KindUnsupported, "custom UUID GoType not supported (only github.com/google/uuid.UUID)"
	case field.TypeJSON:
		return KindUnsupported, "JSON fields not supported via FIQL"
	case field.TypeBytes:
		return KindUnsupported, "bytes fields not supported via FIQL/proto"
	case field.TypeOther:
		return KindUnsupported, "ent TypeOther fields not supported via FIQL/proto"
	default:
		return KindUnsupported, fmt.Sprintf("ent field type %v not supported", f.Type.Type)
	}
}
