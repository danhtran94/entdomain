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
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"entgo.io/ent/entc/gen"
)

//go:embed template/proto_messages.tmpl
var protoMessagesTmpl string

var protoTemplate = template.Must(template.New("proto_messages").Parse(protoMessagesTmpl))

// protoFileData holds all data needed to render a .proto file.
type protoFileData struct {
	PkgName   string
	GoPackage string
	Imports   []string
	Enums     []protoEnumData
	Messages  []protoMessageData
}

type protoEnumData struct {
	Name   string
	Values []protoEnumValue
}

type protoEnumValue struct {
	Name   string
	Number int
}

type protoMessageData struct {
	Name   string
	Fields []protoMessageField
}

type protoMessageField struct {
	Optional  bool
	Repeated  bool
	TypeName  string
	SnakeName string
	FieldNum  int
}

// generateProtoFiles generates .proto file(s) for all entities with EntityAnnotation.
// outDir is the module root. The .proto files are written to {outDir}/{cfg.dir}/{cfg.pkgName}/.
func generateProtoFiles(g *gen.Graph, cfg *ProtoConfig, outDir string) error {
	protoOutDir := filepath.Join(outDir, cfg.dir, cfg.pkgName)
	if err := os.MkdirAll(protoOutDir, 0755); err != nil {
		return fmt.Errorf("entdomain proto: create proto dir: %w", err)
	}

	lockPath := filepath.Join(protoOutDir, ".entdomain.lock.json")
	lock, err := loadLockFile(lockPath)
	if err != nil {
		return fmt.Errorf("entdomain proto: load lock file: %w", err)
	}

	var allEnums []protoEnumData
	var allMessages []protoMessageData
	var allSkipped []skippedField
	allImports := map[string]bool{}

	for _, t := range g.Nodes {
		ant, err := extractEntityAnnotation(t.Annotations)
		if err != nil {
			return fmt.Errorf("entdomain proto: extract annotation for %s: %w", t.Name, err)
		}
		if ant == nil {
			continue
		}

		enums, msg, imports, skipped, allocErr := buildProtoMessageData(t, ant, lock)
		if allocErr != nil {
			return fmt.Errorf("entdomain proto: build proto data for %s: %w", t.Name, allocErr)
		}

		allEnums = append(allEnums, enums...)
		allMessages = append(allMessages, msg)
		allSkipped = append(allSkipped, skipped...)
		for imp := range imports {
			allImports[imp] = true
		}
	}

	if err := saveLockFile(lockPath, lock); err != nil {
		return fmt.Errorf("entdomain proto: save lock file: %w", err)
	}

	skippedPath := filepath.Join(protoOutDir, ".entdomain.skipped.json")
	if err := saveSkippedFile(skippedPath, allSkipped); err != nil {
		return fmt.Errorf("entdomain proto: save skipped summary: %w", err)
	}

	// Sort imports for deterministic output.
	var sortedImports []string
	for imp := range allImports {
		sortedImports = append(sortedImports, imp)
	}
	sort.Strings(sortedImports)

	pkgDecl := cfg.pkgName
	if cfg.fullPkgName != "" {
		pkgDecl = cfg.fullPkgName
	}

	fd := protoFileData{
		PkgName:   pkgDecl,
		GoPackage: cfg.goPackage,
		Imports:   sortedImports,
		Enums:     allEnums,
		Messages:  allMessages,
	}

	protoFile := filepath.Join(protoOutDir, "ent_messages.proto")
	if err := renderProtoFile(protoFile, fd); err != nil {
		return fmt.Errorf("entdomain proto: render proto file: %w", err)
	}

	return nil
}

// skippedField records a field that was excluded from the generated proto
// message. The collected slice is written to .entdomain.skipped.json so
// silently-dropped fields surface diagnostically without changing the
// generated proto / Go output.
type skippedField struct {
	Message string `json:"message"`
	Field   string `json:"field"`
	Reason  string `json:"reason"`
}

// buildProtoMessageData constructs the proto enum and message data for one entity.
// Returns skipped to surface silently-excluded fields in the .entdomain.skipped.json artifact.
func buildProtoMessageData(t *gen.Type, ant *EntityAnnotation, lock *ProtoLockFile) (enums []protoEnumData, msg protoMessageData, imports map[string]bool, skipped []skippedField, err error) {
	imports = map[string]bool{}
	msg.Name = t.Name

	// Collect all candidate field names for lock allocation.
	// We process in schema order: id, fields, edges, virtual fields.
	type pendingField struct {
		snakeName string
		spec      ProtoFieldSpec
		// For enums, additional data.
		enumData *protoEnumData
	}

	var pending []pendingField

	// ID field (always int64 in proto for non-UUID IDs).
	idSpec := resolveEntFieldProtoSpec(t.Name, t.ID, nil)
	if idSpec.IsExcluded {
		// Custom-GoType UUID IDs hit this branch via the resolver gate; the
		// message is unrepresentable in proto without an ID.
		skipped = append(skipped, skippedField{Message: t.Name, Field: "id", Reason: idSpec.ExcludedReason})
	} else {
		// IDs are non-optional in proto.
		idSpec.IsOptional = false
		pending = append(pending, pendingField{snakeName: "id", spec: idSpec})
	}

	// Regular fields.
	for _, f := range t.Fields {
		if f.IsEdgeField() {
			continue
		}

		// Check SkipProto field annotation.
		fa, faErr := extractFieldAnnotation(f.Annotations)
		if faErr != nil {
			err = fmt.Errorf("extract field annotation for %s.%s: %w", t.Name, f.Name, faErr)
			return
		}
		if fa != nil && fa.SkipProto {
			skipped = append(skipped, skippedField{Message: t.Name, Field: f.Name, Reason: "field has SkipProto annotation"})
			continue
		}

		spec := resolveEntFieldProtoSpec(t.Name, f, fa)
		// Symmetric exclusion handling: skip immediately like edges/virtuals
		// rather than carrying excluded specs through pending and re-checking.
		if spec.IsExcluded {
			skipped = append(skipped, skippedField{Message: t.Name, Field: f.Name, Reason: spec.ExcludedReason})
			continue
		}
		pf := pendingField{snakeName: f.Name, spec: spec}

		if spec.IsEnum {
			// Build the enum data: ENTITY_FIELD_VALUE naming.
			prefix := strings.ToUpper(snake(t.Name)) + "_" + strings.ToUpper(snake(f.StructField()))
			ed := &protoEnumData{Name: spec.EnumTypeName}
			// Unspecified = 0 sentinel.
			ed.Values = append(ed.Values, protoEnumValue{
				Name:   prefix + "_UNSPECIFIED",
				Number: 0,
			})
			for i, v := range f.Enums {
				ed.Values = append(ed.Values, protoEnumValue{
					Name:   prefix + "_" + strings.ToUpper(snake(v.Value)),
					Number: i + 1,
				})
			}
			pf.enumData = ed
		}

		pending = append(pending, pf)
	}

	// Edge fields.
	for _, e := range t.Edges {
		ea, eaErr := extractEdgeAnnotation(e.Annotations)
		if eaErr != nil {
			err = fmt.Errorf("extract edge annotation for %s.%s: %w", t.Name, e.Name, eaErr)
			return
		}
		if ea == nil {
			continue
		}
		fa, faErr := extractFieldAnnotation(e.Annotations)
		if faErr != nil {
			err = fmt.Errorf("extract field annotation for edge %s.%s: %w", t.Name, e.Name, faErr)
			return
		}

		specs := resolveEdgeProtoSpec(e, ea, fa)
		specIdx := 0

		if ea.HasIDs() && specIdx < len(specs) {
			spec := specs[specIdx]
			specIdx++
			// Compute field name: {singular_edge}_id or {singular_edge}_ids.
			var fieldName string
			if e.Unique {
				fieldName = snake(e.Name) + "_id"
			} else {
				fieldName = snake(singular(e.Name)) + "_ids"
			}
			if spec.IsExcluded {
				skipped = append(skipped, skippedField{Message: t.Name, Field: fieldName, Reason: spec.ExcludedReason})
			} else {
				pending = append(pending, pendingField{snakeName: fieldName, spec: spec})
			}
		}
		if ea.HasNest() && specIdx < len(specs) {
			spec := specs[specIdx]
			specIdx++
			fieldName := snake(e.Name)
			if spec.IsExcluded {
				skipped = append(skipped, skippedField{Message: t.Name, Field: fieldName, Reason: spec.ExcludedReason})
			} else {
				pending = append(pending, pendingField{snakeName: fieldName, spec: spec})
			}
		}
	}

	// Virtual fields.
	for _, vf := range ant.VirtualFields {
		spec := resolveVirtualFieldProtoSpec(vf)
		fieldName := snake(vf.Name)
		if spec.IsExcluded {
			skipped = append(skipped, skippedField{Message: t.Name, Field: fieldName, Reason: spec.ExcludedReason})
			continue
		}
		pending = append(pending, pendingField{
			snakeName: fieldName,
			spec:      spec,
		})
	}

	// Collect field names for lock allocation. By construction every
	// pending entry is non-excluded — exclusion sites short-circuit to
	// the skipped slice before reaching here.
	fieldNames := make([]string, 0, len(pending))
	for _, pf := range pending {
		fieldNames = append(fieldNames, pf.snakeName)
	}

	// Allocate field numbers (stable across generations).
	fieldNums := allocateFieldNumbers(lock, t.Name, fieldNames)

	// Build the message fields and collect enums/imports. By construction
	// pending now contains only non-excluded fields (every exclusion site
	// short-circuits to the skipped slice above).
	for _, pf := range pending {
		num, ok := fieldNums[pf.snakeName]
		if !ok {
			continue
		}
		if pf.spec.ImportPath != "" {
			imports[pf.spec.ImportPath] = true
		}
		if pf.enumData != nil {
			enums = append(enums, *pf.enumData)
		}
		msg.Fields = append(msg.Fields, protoMessageField{
			Optional:  pf.spec.IsOptional,
			Repeated:  pf.spec.IsRepeated,
			TypeName:  pf.spec.ProtoType,
			SnakeName: pf.snakeName,
			FieldNum:  num,
		})
	}

	// Sort fields by field number for deterministic, readable proto output.
	sort.Slice(msg.Fields, func(i, j int) bool {
		return msg.Fields[i].FieldNum < msg.Fields[j].FieldNum
	})

	return
}

// renderProtoFile renders the proto file template and writes the result.
func renderProtoFile(path string, fd protoFileData) error {
	var buf bytes.Buffer
	if err := protoTemplate.Execute(&buf, fd); err != nil {
		return fmt.Errorf("execute proto template: %w", err)
	}
	return os.WriteFile(path, buf.Bytes(), 0644)
}

// saveSkippedFile writes the per-message skipped-field summary to a sibling
// of the lock file. Always written (even when empty) for deterministic
// output. The file is for diagnostics only — the proto and Go generators
// do not consume it.
func saveSkippedFile(path string, skipped []skippedField) error {
	if skipped == nil {
		skipped = []skippedField{}
	}
	sort.SliceStable(skipped, func(i, j int) bool {
		if skipped[i].Message != skipped[j].Message {
			return skipped[i].Message < skipped[j].Message
		}
		if skipped[i].Field != skipped[j].Field {
			return skipped[i].Field < skipped[j].Field
		}
		return skipped[i].Reason < skipped[j].Reason
	})
	data, err := json.MarshalIndent(skipped, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal skipped: %w", err)
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}
