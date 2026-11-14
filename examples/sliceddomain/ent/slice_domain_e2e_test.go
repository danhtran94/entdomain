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

package ent_test

import (
	"context"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danhtran94/entdomain/examples/sliceddomain/ent"
	"github.com/danhtran94/entdomain/examples/sliceddomain/ent/enttest"
	"github.com/danhtran94/entdomain/examples/sliceddomain/internal/domain/billing"
	"github.com/danhtran94/entdomain/examples/sliceddomain/internal/domain/identity"
	"github.com/danhtran94/entdomain/examples/sliceddomain/proto/entpb"
)

func newClient(t *testing.T) *ent.Client {
	t.Helper()
	c := enttest.Open(t, "sqlite3", "file:"+t.Name()+"?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// Slices combined with WithProtoHelpersInDomain: the proto mappers are methods
// and functions of the slice's own domain package (no pbmap subpackage), so a
// domain value converts with d.ToProto() and billing.InvoiceFromProto.
func TestSlicedDomainModeE2E_ProtoRoundTrip(t *testing.T) {
	c := newClient(t)
	ctx := context.Background()

	acc := c.Account.Create().SetEmail("a@example.com").SaveX(ctx)
	inv := c.Invoice.Create().SetNumber("inv-001").SetTotal(150).SetAccountID(acc.ID).SaveX(ctx)
	c.LineItem.Create().SetDescription("widget").SetQty(2).SetInvoice(inv).SaveX(ctx)
	c.LineItem.Create().SetDescription("gadget").SetQty(1).SetInvoice(inv).SaveX(ctx)

	d := c.Invoice.Query().WithLineItems().WithAccount().OnlyX(ctx).ToDomain()
	require.Len(t, d.LineItems, 2)

	p := d.ToProto()
	require.NotNil(t, p)
	assert.Equal(t, int64(inv.ID), p.Id)
	assert.Equal(t, "inv-001", p.Number)
	assert.Equal(t, int64(acc.ID), p.AccountId, "cross-slice edge is a plain int64")
	require.Len(t, p.LineItems, 2, "same-slice Nest converts through the slice's own LineItem mapper")
	assert.Equal(t, "widget", p.LineItems[0].Description)

	back := billing.InvoiceFromProto(&entpb.Invoice{Id: p.Id, AccountId: p.AccountId, Number: "inv-009", Total: 7})
	assert.Equal(t, acc.ID, back.AccountID)
	assert.Equal(t, "inv-009", back.Number)

	list := billing.InvoiceList{d}.ToProto()
	require.Len(t, list, 1)
	assert.Equal(t, p.Id, list[0].Id)

	ap := c.Account.Query().WithInvoices().OnlyX(ctx).ToDomain().ToProto()
	assert.Equal(t, []int64{int64(inv.ID)}, ap.InvoiceIds, "identity's helper-using mapper works in its own package")
	assert.Equal(t, []int{inv.ID}, identity.AccountFromProto(ap).InvoiceIDs)
}
