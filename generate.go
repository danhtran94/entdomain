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
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"entgo.io/ent/entc/gen"
	"entgo.io/ent/schema/field"
)

// moduleRoot returns the module root directory (parent of the ent target dir).
// g.Config.Target may be a relative path (e.g., "."), so we resolve it to an
// absolute path using the working directory before calling filepath.Dir.
func moduleRoot(g *gen.Graph) string {
	target := g.Config.Target
	if !filepath.IsAbs(target) {
		if wd, err := os.Getwd(); err == nil {
			target = filepath.Join(wd, target)
		}
	}
	return filepath.Dir(target)
}

// domainImportPath computes the full Go import path for the domain package.
// It strips the last path segment ("ent") from g.Config.Package and appends pkgPath.
func domainImportPath(g *gen.Graph, pkgPath string) string {
	base := path.Dir(g.Config.Package)
	return base + "/" + pkgPath
}

// generateDomainFiles writes one {entity_lower}.go per entity with EntityAnnotation
// to filepath.Join(moduleRoot(g), pkgPath).
func generateDomainFiles(g *gen.Graph, pkgPath, pkgName string) error {
	outDir := filepath.Join(moduleRoot(g), pkgPath)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("entdomain: create domain dir: %w", err)
	}
	for _, t := range g.Nodes {
		ant, err := extractEntityAnnotation(t.Annotations)
		if err != nil {
			return fmt.Errorf("entdomain: extract annotation for %s: %w", t.Name, err)
		}
		if ant == nil {
			continue
		}
		data, err := buildDomainFileData(g, t, ant, pkgName)
		if err != nil {
			return fmt.Errorf("entdomain: build domain data for %s: %w", t.Name, err)
		}
		src, err := renderDomainFile(data)
		if err != nil {
			return fmt.Errorf("entdomain: render domain file for %s: %w", t.Name, err)
		}
		outPath := filepath.Join(outDir, strings.ToLower(t.Name)+"_gen.go")
		if err := os.WriteFile(outPath, src, 0644); err != nil {
			return fmt.Errorf("entdomain: write domain file %s: %w", outPath, err)
		}
	}
	return nil
}

// domainFileData holds all data needed to render a single domain entity file.
type domainFileData struct {
	PkgName       string
	EntityName    string
	ListName      string // e.g. "UserList"
	NoBulk        bool
	IDField       domainField
	Fields        []domainField
	EnumTypes     []domainEnumType
	EdgeFields    []domainEdgeField
	VirtualFields []domainVirtualField
	Imports       []string
}

type domainField struct {
	// SnakeName is the snake_case field name used as the constant value.
	SnakeName string
	// StructName is the PascalCase struct field name.
	StructName string
	// TypeStr is the Go type string (e.g., "string", "*time.Time").
	TypeStr string
}

type domainEnumType struct {
	TypeName string
	Values   []domainEnumValue
}

type domainEnumValue struct {
	ConstName string
	Value     string
}

type domainEdgeField struct {
	StructName string
	TypeStr    string
}

type domainVirtualField struct {
	StructName string
	TypeStr    string
}

// buildDomainFileData constructs the template data for a single entity's domain file.
func buildDomainFileData(g *gen.Graph, t *gen.Type, ant *EntityAnnotation, pkgName string) (*domainFileData, error) {
	imports := map[string]bool{}
	data := &domainFileData{
		PkgName:    pkgName,
		EntityName: t.Name,
		ListName:   t.Name + "List",
		NoBulk:     ant.NoBulk,
	}

	// ID field
	idTypeStr, idImport := fieldToDomainType(t.Name, t.ID)
	data.IDField = domainField{
		SnakeName:  "id",
		StructName: "ID",
		TypeStr:    idTypeStr,
	}
	if idImport != "" {
		imports[idImport] = true
	}

	// Regular fields
	for _, f := range t.Fields {
		if f.IsEdgeField() {
			continue
		}
		typeStr, importPath, enumType := fieldToDomainTypeWithEnum(t.Name, f)
		if importPath != "" {
			imports[importPath] = true
		}
		if enumType != nil {
			data.EnumTypes = append(data.EnumTypes, *enumType)
		}
		data.Fields = append(data.Fields, domainField{
			SnakeName:  f.Name,
			StructName: f.StructField(),
			TypeStr:    typeStr,
		})
	}

	// Edge fields
	for _, e := range t.Edges {
		edgeAnt, err := extractEdgeAnnotation(e.Annotations)
		if err != nil {
			return nil, fmt.Errorf("extractEdgeAnnotation for edge %s: %w", e.Name, err)
		}
		if edgeAnt == nil {
			continue
		}
		// IDs fields
		if edgeAnt.HasIDs() {
			idTypeStr, idImport := fieldToDomainType(e.Type.Name, e.Type.ID)
			if idImport != "" {
				imports[idImport] = true
			}
			if e.Unique {
				data.EdgeFields = append(data.EdgeFields, domainEdgeField{
					StructName: pascal(e.Name) + "ID",
					TypeStr:    idTypeStr,
				})
			} else {
				data.EdgeFields = append(data.EdgeFields, domainEdgeField{
					StructName: singular(pascal(e.Name)) + "IDs",
					TypeStr:    "[]" + idTypeStr,
				})
			}
		}
		// Nest fields
		if edgeAnt.HasNest() {
			if e.Unique {
				data.EdgeFields = append(data.EdgeFields, domainEdgeField{
					StructName: pascal(e.Name),
					TypeStr:    e.Type.Name,
				})
			} else {
				data.EdgeFields = append(data.EdgeFields, domainEdgeField{
					StructName: pascal(e.Name),
					TypeStr:    e.Type.Name + "List",
				})
			}
		}
	}

	// Virtual fields
	for _, vf := range ant.VirtualFields {
		typeStr, importPath := resolveGoTypeDomainStr(vf.FieldType)
		if importPath != "" {
			imports[importPath] = true
		}
		data.VirtualFields = append(data.VirtualFields, domainVirtualField{
			StructName: pascal(vf.Name),
			TypeStr:    typeStr,
		})
	}

	// Deduplicate and sort imports
	for imp := range imports {
		data.Imports = append(data.Imports, imp)
	}
	sort.Strings(data.Imports)

	return data, nil
}

// fieldToDomainType returns the Go type string and import path for a field (no enum handling).
func fieldToDomainType(entityName string, f *gen.Field) (typeStr, importPath string) {
	typeStr, importPath, _ = fieldToDomainTypeWithEnum(entityName, f)
	return
}

// fieldToDomainTypeWithEnum returns the Go type string, import path, and optional enum type data.
func fieldToDomainTypeWithEnum(entityName string, f *gen.Field) (typeStr, importPath string, enumType *domainEnumType) {
	optional := f.Optional || f.Nillable

	// Custom Go type (HasGoType)
	if f.HasGoType() && f.Type.RType != nil {
		rt := f.Type.RType
		ident := rt.Ident
		pkgPath := rt.PkgPath
		if pkgPath != "" {
			pkg := path.Base(pkgPath)
			// RType.Ident may already include the package prefix (e.g., "uuid.UUID").
			// Strip it to avoid double-qualification ("uuid.uuid.UUID").
			if strings.HasPrefix(ident, pkg+".") {
				ident = ident[len(pkg)+1:]
			}
			typeStr = pkg + "." + ident
			importPath = pkgPath
		} else {
			typeStr = ident
		}
		if optional && !strings.HasPrefix(typeStr, "*") {
			typeStr = "*" + typeStr
		}
		return
	}

	switch f.Type.Type {
	case field.TypeString:
		typeStr = "string"
	case field.TypeBool:
		typeStr = "bool"
	case field.TypeInt:
		typeStr = "int"
	case field.TypeInt8:
		typeStr = "int8"
	case field.TypeInt16:
		typeStr = "int16"
	case field.TypeInt32:
		typeStr = "int32"
	case field.TypeInt64:
		typeStr = "int64"
	case field.TypeUint:
		typeStr = "uint"
	case field.TypeUint8:
		typeStr = "uint8"
	case field.TypeUint16:
		typeStr = "uint16"
	case field.TypeUint32:
		typeStr = "uint32"
	case field.TypeUint64:
		typeStr = "uint64"
	case field.TypeFloat32:
		typeStr = "float32"
	case field.TypeFloat64:
		typeStr = "float64"
	case field.TypeTime:
		typeStr = "time.Time"
		importPath = "time"
	case field.TypeUUID:
		typeStr = "uuid.UUID"
		importPath = "github.com/google/uuid"
	case field.TypeEnum:
		typeName := entityName + f.StructField()
		typeStr = typeName
		et := &domainEnumType{TypeName: typeName}
		for _, v := range f.Enums {
			// v.Name in ent includes the field prefix (e.g., "StatusActive" for "active").
			// Use pascal(v.Value) to derive only the suffix part (e.g., "Active").
			et.Values = append(et.Values, domainEnumValue{
				ConstName: typeName + pascal(v.Value),
				Value:     v.Value,
			})
		}
		enumType = et
	case field.TypeJSON, field.TypeBytes:
		if f.Type.RType != nil {
			typeStr = f.Type.RType.Ident
			if f.Type.RType.PkgPath != "" {
				importPath = f.Type.RType.PkgPath
			}
		} else {
			typeStr = "json.RawMessage"
			importPath = "encoding/json"
		}
	default:
		typeStr = "interface{}"
	}

	if optional && typeStr != "" && !strings.HasPrefix(typeStr, "*") && !strings.HasPrefix(typeStr, "[]") && !strings.HasPrefix(typeStr, "map[") && !strings.HasPrefix(typeStr, "interface") {
		typeStr = "*" + typeStr
	}
	return
}

// resolveGoTypeDomainStr resolves a FieldType to its Go type string and import path.
// GoType("github.com/shopspring/decimal", "*Decimal") → "*decimal.Decimal", "github.com/shopspring/decimal"
// GoType("", "*Money") → "*Money", ""
func resolveGoTypeDomainStr(ft FieldType) (typeStr, importPath string) {
	typeName := ft.TypeName
	pkgPath := ft.PkgPath

	if pkgPath == "" {
		return typeName, ""
	}

	// Has package path — need to qualify the type name with package alias
	pointer := strings.HasPrefix(typeName, "*")
	if pointer {
		typeName = typeName[1:]
	}
	pkg := path.Base(pkgPath)
	typeStr = pkg + "." + typeName
	if pointer {
		typeStr = "*" + typeStr
	}
	importPath = pkgPath
	return
}

var domainStructTemplate = template.Must(template.New("domain_struct").Parse(`// Code generated by entdomain, DO NOT EDIT.

package {{ .PkgName }}
{{ if .Imports }}
import (
{{ range .Imports }}	"{{ . }}"
{{ end }})
{{ end }}
{{- range .EnumTypes }}{{- $enumTypeName := .TypeName }}
type {{ .TypeName }} string

const (
{{- range .Values }}
	{{ .ConstName }} {{ $enumTypeName }} = "{{ .Value }}"
{{- end }}
)
{{ end }}
type {{ .EntityName }} struct {
	ID {{ .IDField.TypeStr }}
{{- range .Fields }}
	{{ .StructName }} {{ .TypeStr }}
{{- end }}
{{- range .EdgeFields }}
	{{ .StructName }} {{ .TypeStr }}
{{- end }}
{{- range .VirtualFields }}
	{{ .StructName }} {{ .TypeStr }}
{{- end }}
}

// {{ .ListName }} is a slice of {{ .EntityName }}.
type {{ .ListName }} []*{{ .EntityName }}

// ToList wraps the {{ .EntityName }} in a {{ .ListName }}.
func (e *{{ .EntityName }}) ToList() {{ .ListName }} {
	return {{ .ListName }}{e}
}

// GetIDs returns the ID of each item in the list.
func (ds {{ .ListName }}) GetIDs() []{{ .IDField.TypeStr }} {
	ids := make([]{{ .IDField.TypeStr }}, len(ds))
	for i, d := range ds {
		ids[i] = d.ID
	}
	return ids
}
`))

// renderDomainFile renders the domain struct template and formats it with go/format.
func renderDomainFile(data *domainFileData) ([]byte, error) {
	var buf bytes.Buffer
	if err := domainStructTemplate.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("execute domain struct template: %w", err)
	}
	src, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("format domain source (raw: %s): %w", buf.String(), err)
	}
	return src, nil
}

// pascal converts a string to PascalCase using ent's built-in function.
func pascal(s string) string {
	return gen.Funcs["pascal"].(func(string) string)(s)
}

// singular converts a string to singular form using ent's built-in function.
func singular(s string) string {
	return gen.Funcs["singular"].(func(string) string)(s)
}

// snake converts a string to snake_case using ent's built-in function.
func snake(s string) string {
	return gen.Funcs["snake"].(func(string) string)(s)
}
