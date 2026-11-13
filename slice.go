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

package entdomain

import (
	"errors"
	"fmt"
	"go/token"
	"path"
	"regexp"
	"sort"

	"entgo.io/ent/entc/gen"
)

// pbmapDir is the mapper subpackage name. A slice cannot use it, or
// {pkgPath}/pbmap would be both a slice package and the root mapper package.
const pbmapDir = "pbmap"

// rootSliceLabel is how the root (un-sliced) domain package appears in errors.
const rootSliceLabel = "<root>"

var sliceNameRE = regexp.MustCompile(`^[a-z][a-z0-9]*$`)

// sliceGroup is the set of entities generated into one domain package.
type sliceGroup struct {
	// Slice is the annotation value; "" is the root package.
	Slice string
	// Dir is the output directory relative to the module root.
	Dir string
	// PkgName is the Go package name of the generated domain files.
	PkgName string
	// Alias is the import alias used in ent/domain.go and proto mappers.
	Alias string
	// Nodes are the annotated entities in this group, in graph order.
	Nodes []*gen.Type
}

// validateSliceName reports whether name is usable as a slice. The root
// package has no slice name, so the empty string is rejected here; callers
// skip validation for entities without a Slice.
func validateSliceName(name string) error {
	switch {
	case !sliceNameRE.MatchString(name):
		return fmt.Errorf("entdomain: invalid slice name %q: must match %s", name, sliceNameRE)
	case token.IsKeyword(name):
		return fmt.Errorf("entdomain: invalid slice name %q: Go keyword", name)
	case name == pbmapDir:
		return fmt.Errorf("entdomain: invalid slice name %q: reserved for the proto mapper subpackage", name)
	}
	return nil
}

// sliceOf returns the slice of an annotated entity, and whether t is a domain
// entity at all. An entity without entdomain.Entity() has no domain presence.
func sliceOf(t *gen.Type) (slice string, ok bool, err error) {
	ant, err := extractEntityAnnotation(t.Annotations)
	if err != nil {
		return "", false, fmt.Errorf("entdomain: extract annotation for %s: %w", t.Name, err)
	}
	if ant == nil {
		return "", false, nil
	}
	return ant.Slice, true, nil
}

func sliceLabel(slice string) string {
	if slice == "" {
		return rootSliceLabel
	}
	return fmt.Sprintf("%q", slice)
}

// validateSliceNames checks the Slice of every annotated entity.
func validateSliceNames(nodes []*gen.Type) error {
	var errs []error
	for _, t := range nodes {
		slice, ok, err := sliceOf(t)
		if err != nil {
			return err
		}
		if !ok || slice == "" {
			continue
		}
		if err := validateSliceName(slice); err != nil {
			errs = append(errs, fmt.Errorf("entity %s: %w", t.Name, err))
		}
	}
	return errors.Join(errs...)
}

// validateSliceEdges rejects Nest edges that cross a slice boundary.
//
// A nested edge types its field as the target's struct, which needs an import
// of the target's package; two slices nesting each other would form an import
// cycle. Edges that carry IDs only reference no type and are always allowed.
func validateSliceEdges(nodes []*gen.Type) error {
	var errs []error
	for _, t := range nodes {
		from, ok, err := sliceOf(t)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		for _, e := range t.Edges {
			ea, err := extractEdgeAnnotation(e.Annotations)
			if err != nil {
				return fmt.Errorf("entdomain: extract edge annotation for %s.%s: %w", t.Name, e.Name, err)
			}
			if ea == nil || !ea.HasNest() {
				continue
			}
			to, ok, err := sliceOf(e.Type)
			if err != nil {
				return err
			}
			if !ok || to == from {
				continue
			}
			errs = append(errs, fmt.Errorf(
				"entdomain: cross-slice Nest on edge %s.%s (%s in slice %s, %s in slice %s): "+
					"use entdomain.IDs() for this edge or move both entities into one slice",
				t.Name, e.Name, t.Name, sliceLabel(from), e.Type.Name, sliceLabel(to)))
		}
	}
	return errors.Join(errs...)
}

// groupBySlice partitions the annotated entities into slice groups: the root
// group first, then named slices sorted by name. The root group is omitted when
// it has no entities but a named slice exists. Graphs without any slice return
// exactly one root group, matching the pre-slice layout.
func groupBySlice(nodes []*gen.Type, pkgPath, pkgName string) ([]sliceGroup, error) {
	root := sliceGroup{Dir: pkgPath, PkgName: pkgName, Alias: pkgName}
	named := map[string]*sliceGroup{}
	for _, t := range nodes {
		slice, ok, err := sliceOf(t)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		if slice == "" {
			root.Nodes = append(root.Nodes, t)
			continue
		}
		g := named[slice]
		if g == nil {
			g = &sliceGroup{
				Slice:   slice,
				Dir:     path.Join(pkgPath, slice),
				PkgName: slice,
				Alias:   slice + "domain",
			}
			named[slice] = g
		}
		g.Nodes = append(g.Nodes, t)
	}

	var groups []sliceGroup
	if len(root.Nodes) > 0 || len(named) == 0 {
		groups = append(groups, root)
	}
	names := make([]string, 0, len(named))
	for n := range named {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		groups = append(groups, *named[n])
	}
	return groups, nil
}
