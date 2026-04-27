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

import "context"

// domainKey is a type-parameterized empty struct used as a context key.
// Distinct instantiations of domainKey[T] produce distinct keys, so domain
// structs of different types coexist in the same context without collision.
type domainKey[T any] struct{}

// WithDomain returns a derived context carrying the domain struct d keyed by
// its Go type. It is an escape-hatch helper for users who want ent mutation
// hooks (client.X.Use) to read the full domain struct — including virtual
// fields — during Save.
//
// The primary path for domain-to-ent encoding remains the typed transformer
// slots generated in ent/domain.go (see ADR-005). Use WithDomain only when
// the work must happen at ent-hook time rather than ApplyDomain time, e.g.
// outbox event emission, cross-cutting audit logging, or tenant-isolation
// checks that depend on virtual-field values.
//
// Typical use is at the repository boundary, immediately before Save:
//
//	ctx = entdomain.WithDomain(ctx, d)
//	builder, err := client.User.Create().ApplyDomain(ctx, d)
//	// ... check err ...
//	created, err := builder.Save(ctx) // hooks see d via DomainFrom[*domain.User](ctx)
//
// Caveats:
//
//   - This uses context.Value, which Go's standard guidance reserves for
//     request-scoped data. Static analyzers may flag call sites. That is
//     expected: the smell lives at the opt-in site, not inside the library.
//   - Hook authors must handle the absent case via the ok return from
//     DomainFrom. Do not assume presence — callers can bypass WithDomain.
//   - Do not auto-wrap ApplyDomain to call WithDomain internally. The ctx
//     chain must stay visible to the caller.
func WithDomain[T any](ctx context.Context, d T) context.Context {
	return context.WithValue(ctx, domainKey[T]{}, d)
}

// DomainFrom retrieves a domain struct of type T previously stashed via
// WithDomain. Returns the zero value of T and false when no value of that
// type is present. Always check ok — bare .Save(ctx) callers (who never
// called WithDomain) will see ok == false.
//
// Example in an ent hook:
//
//	client.User.Use(func(next ent.Mutator) ent.Mutator {
//	    return hook.UserFunc(func(ctx context.Context, m *ent.UserMutation) (ent.Value, error) {
//	        d, ok := entdomain.DomainFrom[*domain.User](ctx)
//	        if !ok {
//	            // Not an entdomain-originated mutation; run default path.
//	            return next.Mutate(ctx, m)
//	        }
//	        // Use d (including virtual fields) here.
//	        return next.Mutate(ctx, m)
//	    })
//	})
func DomainFrom[T any](ctx context.Context) (T, bool) {
	v, ok := ctx.Value(domainKey[T]{}).(T)
	return v, ok
}
