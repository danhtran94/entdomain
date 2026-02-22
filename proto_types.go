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
	"path"
	"strings"

	"entgo.io/ent/entc/gen"
	"entgo.io/ent/schema/field"
)

// ProtoFieldSpec describes how a single domain field maps to a proto field.
type ProtoFieldSpec struct {
	// ProtoType is the proto field type string (e.g., "string", "int64", "google.protobuf.Timestamp").
	ProtoType string
	// ImportPath is the proto import path needed (e.g., "google/protobuf/timestamp.proto"), or "".
	ImportPath string
	// IsOptional means the proto field should use the proto3 "optional" keyword.
	IsOptional bool
	// IsRepeated means the proto field is "repeated".
	IsRepeated bool
	// IsEnum means the field is a proto enum.
	IsEnum bool
	// IsExcluded means this field should be omitted from proto output.
	IsExcluded bool
	// ToProtoExpr is a Go fmt format string (with %s for the source expression) that converts
	// the domain value to the proto value. Empty string means direct assignment.
	ToProtoExpr string
	// FromProtoExpr is a Go fmt format string (with %s for the source expression) that converts
	// the proto value to the domain value. Empty string means direct assignment.
	FromProtoExpr string
	// EnumTypeName is the proto enum type name (e.g., "UserStatus"). Only set when IsEnum=true.
	EnumTypeName string
	// NestEntityName is set for Nest edge specs and holds the referenced entity type name
	// (e.g., "Post"). The mapper uses it to build mode-appropriate converter calls
	// (e.g., PostToProto / PostFromProto in subpackage mode).
	NestEntityName string
}

// wellKnownGoTypeMap maps (pkgPath, typeName) → proto type + proto import.
var wellKnownGoTypeMap = map[[2]string][2]string{
	{"time", "Time"}:                   {"google.protobuf.Timestamp", "google/protobuf/timestamp.proto"},
	{"time", "Duration"}:               {"google.protobuf.Duration", "google/protobuf/duration.proto"},
	{"github.com/google/uuid", "UUID"}: {"string", ""},
}

// resolveEntFieldProtoSpec resolves the proto spec for a regular ent schema field.
// Resolution order:
//  1. TypeUUID → explicit string with conversion expressions
//  2. TypeJSON / TypeBytes / TypeOther → IsExcluded
//  3. Custom GoType → attempt well-known map or exclude
//  4. TypeEnum → proto enum
//  5. Primitive types → direct mapping
func resolveEntFieldProtoSpec(entityName string, f *gen.Field) ProtoFieldSpec {
	optional := f.Optional || f.Nillable

	// UUID fields: always use string representation with explicit converters.
	// Check this BEFORE HasGoType since ent represents uuid fields as a GoType.
	if f.Type.Type == field.TypeUUID {
		spec := ProtoFieldSpec{ProtoType: "string", IsOptional: optional}
		if optional {
			spec.ToProtoExpr = "uuidPtrToProtoString(%s)"
			spec.FromProtoExpr = "protoStringToUUIDPtr(%s)"
		} else {
			spec.ToProtoExpr = "%s.String()"
			spec.FromProtoExpr = "uuid.MustParse(%s)"
		}
		return spec
	}

	// Custom Go type: try well-known map, else exclude.
	if f.HasGoType() && f.Type.RType != nil {
		rt := f.Type.RType
		pkgPath := rt.PkgPath
		typeName := rt.Ident
		if pkgPath != "" {
			pkg := path.Base(pkgPath)
			if strings.HasPrefix(typeName, pkg+".") {
				typeName = typeName[len(pkg)+1:]
			}
		}
		return resolveWellKnownGoType(pkgPath, typeName, optional)
	}

	switch f.Type.Type {
	case field.TypeJSON, field.TypeBytes, field.TypeOther:
		return ProtoFieldSpec{IsExcluded: true}

	case field.TypeString:
		return ProtoFieldSpec{ProtoType: "string", IsOptional: optional}

	case field.TypeBool:
		return ProtoFieldSpec{ProtoType: "bool", IsOptional: optional}

	case field.TypeFloat32:
		return ProtoFieldSpec{ProtoType: "float", IsOptional: optional}

	case field.TypeFloat64:
		return ProtoFieldSpec{ProtoType: "double", IsOptional: optional}

	case field.TypeInt32:
		return ProtoFieldSpec{ProtoType: "int32", IsOptional: optional}

	case field.TypeInt64:
		return ProtoFieldSpec{ProtoType: "int64", IsOptional: optional}

	case field.TypeUint32:
		return ProtoFieldSpec{ProtoType: "uint32", IsOptional: optional}

	case field.TypeUint64:
		return ProtoFieldSpec{ProtoType: "uint64", IsOptional: optional}

	case field.TypeInt, field.TypeInt8, field.TypeInt16:
		// Map small int types to int64 in proto for safety.
		spec := ProtoFieldSpec{ProtoType: "int64", IsOptional: optional}
		goType := "int"
		if f.Type.Type == field.TypeInt8 {
			goType = "int8"
		} else if f.Type.Type == field.TypeInt16 {
			goType = "int16"
		}
		if optional {
			spec.ToProtoExpr = "toInt64Ptr(%s)"
			spec.FromProtoExpr = "fromInt64Ptr(%s)"
		} else {
			spec.ToProtoExpr = "int64(%s)"
			spec.FromProtoExpr = goType + "(%s)"
		}
		return spec

	case field.TypeUint, field.TypeUint8, field.TypeUint16:
		spec := ProtoFieldSpec{ProtoType: "uint64", IsOptional: optional}
		goType := "uint"
		if f.Type.Type == field.TypeUint8 {
			goType = "uint8"
		} else if f.Type.Type == field.TypeUint16 {
			goType = "uint16"
		}
		if optional {
			spec.ToProtoExpr = "toUint64Ptr(%s)"
			spec.FromProtoExpr = "fromUint64Ptr(%s)"
		} else {
			spec.ToProtoExpr = "uint64(%s)"
			spec.FromProtoExpr = goType + "(%s)"
		}
		return spec

	case field.TypeTime:
		spec := ProtoFieldSpec{
			ProtoType:  "google.protobuf.Timestamp",
			ImportPath: "google/protobuf/timestamp.proto",
		}
		if optional {
			spec.ToProtoExpr = "timeToTimestampProto(%s)"
			spec.FromProtoExpr = "timestampProtoToTime(%s)"
		} else {
			spec.ToProtoExpr = "timestamppb.New(%s)"
			spec.FromProtoExpr = "%s.AsTime()"
		}
		return spec

	case field.TypeUUID:
		spec := ProtoFieldSpec{ProtoType: "string", IsOptional: optional}
		if optional {
			spec.ToProtoExpr = "uuidPtrToProtoString(%s)"
			spec.FromProtoExpr = "protoStringToUUIDPtr(%s)"
		} else {
			spec.ToProtoExpr = "%s.String()"
			spec.FromProtoExpr = "uuid.MustParse(%s)"
		}
		return spec

	case field.TypeEnum:
		enumTypeName := entityName + f.StructField()
		spec := ProtoFieldSpec{
			ProtoType:    enumTypeName,
			IsEnum:       true,
			IsOptional:   optional,
			EnumTypeName: enumTypeName,
		}
		return spec
	}

	return ProtoFieldSpec{IsExcluded: true}
}

// resolveVirtualFieldProtoSpec resolves the proto spec for a virtual field.
// Resolution order:
//  1. Explicit ProtoType config → use as-is
//  2. GoType matching well-known map → auto-map
//  3. GoType with no match → excluded
//  4. Primitive FieldType (String/Bool/Int/Float64) → direct map
func resolveVirtualFieldProtoSpec(vf VirtualFieldConfig) ProtoFieldSpec {
	ft := vf.FieldType

	// Rule 1: explicit ProtoType override.
	if vf.ProtoType != nil {
		optional := strings.HasPrefix(ft.TypeName, "*")
		spec := ProtoFieldSpec{
			ProtoType:  vf.ProtoType.TypeName,
			ImportPath: vf.ProtoType.ImportPath,
			IsOptional: optional,
		}
		// Add conversion based on the proto type.
		inferConversionExprs(vf.ProtoType.TypeName, optional, &spec)
		return spec
	}

	// Rule 2+3: GoType with package path → check well-known map.
	if ft.PkgPath != "" {
		typeName := ft.TypeName
		pointer := strings.HasPrefix(typeName, "*")
		if pointer {
			typeName = typeName[1:]
		}
		// Strip pkg prefix if present (e.g., "time.Time" → "Time").
		if pkg := path.Base(ft.PkgPath); strings.HasPrefix(typeName, pkg+".") {
			typeName = typeName[len(pkg)+1:]
		}
		return resolveWellKnownGoType(ft.PkgPath, typeName, pointer)
	}

	// Rule 4: primitive FieldType (empty PkgPath).
	switch ft.TypeName {
	case "string":
		return ProtoFieldSpec{ProtoType: "string"}
	case "*string":
		return ProtoFieldSpec{ProtoType: "string", IsOptional: true}
	case "bool":
		return ProtoFieldSpec{ProtoType: "bool"}
	case "*bool":
		return ProtoFieldSpec{ProtoType: "bool", IsOptional: true}
	case "int":
		return ProtoFieldSpec{ProtoType: "int64", ToProtoExpr: "int64(%s)", FromProtoExpr: "int(%s)"}
	case "*int":
		return ProtoFieldSpec{ProtoType: "int64", IsOptional: true, ToProtoExpr: "toInt64Ptr(%s)", FromProtoExpr: "fromInt64Ptr(%s)"}
	case "float64":
		return ProtoFieldSpec{ProtoType: "double"}
	case "*float64":
		return ProtoFieldSpec{ProtoType: "double", IsOptional: true}
	case "float32":
		return ProtoFieldSpec{ProtoType: "float"}
	case "*float32":
		return ProtoFieldSpec{ProtoType: "float", IsOptional: true}
	case "int64":
		return ProtoFieldSpec{ProtoType: "int64"}
	case "int32":
		return ProtoFieldSpec{ProtoType: "int32"}
	}

	// Unknown type → exclude.
	return ProtoFieldSpec{IsExcluded: true}
}

// resolveEdgeProtoSpec resolves proto specs for all domain fields contributed by an edge.
// The returned slice follows the same order as buildDomainFileData: IDs field first (if HasIDs),
// then Nest field (if HasNest). Nest fields are always excluded from proto.
// Returns nil if the edge has no EdgeAnnotation.
func resolveEdgeProtoSpec(e *gen.Edge, ea *EdgeAnnotation, fa *FieldAnnotation) []ProtoFieldSpec {
	if ea == nil {
		return nil
	}

	var specs []ProtoFieldSpec

	skipAll := fa != nil && fa.SkipProto

	if ea.HasIDs() {
		if skipAll {
			specs = append(specs, ProtoFieldSpec{IsExcluded: true})
		} else {
			idTypeStr, _ := fieldToDomainType(e.Type.Name, e.Type.ID)
			spec := resolveIDTypeSpec(idTypeStr, e.Unique)
			specs = append(specs, spec)
		}
	}

	if ea.HasNest() {
		if skipAll {
			specs = append(specs, ProtoFieldSpec{IsExcluded: true})
		} else if e.Unique {
			specs = append(specs, ProtoFieldSpec{
				ProtoType:      e.Type.Name,
				NestEntityName: e.Type.Name,
			})
		} else {
			specs = append(specs, ProtoFieldSpec{
				ProtoType:      e.Type.Name,
				IsRepeated:     true,
				NestEntityName: e.Type.Name,
			})
		}
	}

	return specs
}

// resolveWellKnownGoType maps (pkgPath, typeName) to a ProtoFieldSpec using the well-known table.
// If not found, returns an excluded spec.
func resolveWellKnownGoType(pkgPath, typeName string, optional bool) ProtoFieldSpec {
	key := [2]string{pkgPath, typeName}
	mapping, ok := wellKnownGoTypeMap[key]
	if !ok {
		return ProtoFieldSpec{IsExcluded: true}
	}
	spec := ProtoFieldSpec{
		ProtoType:  mapping[0],
		ImportPath: mapping[1],
		IsOptional: optional,
	}
	inferConversionExprs(mapping[0], optional, &spec)
	return spec
}

// inferConversionExprs populates ToProtoExpr and FromProtoExpr based on the proto type.
func inferConversionExprs(protoType string, optional bool, spec *ProtoFieldSpec) {
	switch protoType {
	case "google.protobuf.Timestamp":
		if optional {
			spec.ToProtoExpr = "timeToTimestampProto(%s)"
			spec.FromProtoExpr = "timestampProtoToTime(%s)"
		} else {
			spec.ToProtoExpr = "timestamppb.New(%s)"
			spec.FromProtoExpr = "%s.AsTime()"
		}
	case "google.protobuf.Duration":
		if optional {
			spec.ToProtoExpr = "durationToDurationProto(%s)"
			spec.FromProtoExpr = "durationProtoToDuration(%s)"
		} else {
			spec.ToProtoExpr = "durationpb.New(%s)"
			spec.FromProtoExpr = "%s.AsDuration()"
		}
	}
	// For "string" from UUID mapping etc., no conversion needed (direct copy).
}

// resolveIDTypeSpec builds a ProtoFieldSpec for an edge ID field.
func resolveIDTypeSpec(idTypeStr string, unique bool) ProtoFieldSpec {
	// Strip pointer prefix if any (edge IDs are typically non-optional).
	typeStr := strings.TrimPrefix(idTypeStr, "*")

	var protoType, toExpr, fromExpr string
	switch typeStr {
	case "int", "int8", "int16":
		protoType = "int64"
		toExpr = "int64(%s)"
		fromExpr = "int(%s)"
	case "int32":
		protoType = "int32"
	case "int64":
		protoType = "int64"
	case "uint", "uint8", "uint16":
		protoType = "uint64"
		toExpr = "uint64(%s)"
		fromExpr = "uint(%s)"
	case "uint32":
		protoType = "uint32"
	case "uint64":
		protoType = "uint64"
	case "string":
		protoType = "string"
	default:
		// UUID or other types → string representation.
		protoType = "string"
		toExpr = "%s.String()"
		fromExpr = "uuid.MustParse(%s)"
	}

	return ProtoFieldSpec{
		ProtoType:     protoType,
		IsRepeated:    !unique,
		ToProtoExpr:   toExpr,
		FromProtoExpr: fromExpr,
	}
}
