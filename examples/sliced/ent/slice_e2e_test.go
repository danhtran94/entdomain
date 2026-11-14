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

	"github.com/danhtran94/entdomain/examples/sliced/ent"
	"github.com/danhtran94/entdomain/examples/sliced/ent/enttest"
	"github.com/danhtran94/entdomain/examples/sliced/ent/invoice"
	"github.com/danhtran94/entdomain/examples/sliced/internal/domain/billing"
	billingpb "github.com/danhtran94/entdomain/examples/sliced/internal/domain/billing/pbmap"
	"github.com/danhtran94/entdomain/examples/sliced/internal/domain/identity"
	identitypb "github.com/danhtran94/entdomain/examples/sliced/internal/domain/identity/pbmap"
	"github.com/danhtran94/entdomain/examples/sliced/proto/entpb"
)

func newClient(t *testing.T) *ent.Client {
	t.Helper()
	c := enttest.Open(t, "sqlite3", "file:"+t.Name()+"?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// seed stores one account owning one invoice with two line items.
//
// Unique IDs edges (Invoice.account) are read-only in ApplyDomain, as for every
// entdomain entity, so the link is written with the ent builder.
func seed(t *testing.T, c *ent.Client) (accID, invID int) {
	t.Helper()
	ctx := context.Background()

	accCreate, err := c.Account.Create().ApplyDomain(ctx, &identity.Account{Email: "a@example.com"})
	require.NoError(t, err)
	acc := accCreate.SaveX(ctx)

	invCreate, err := c.Invoice.Create().ApplyDomain(ctx, &billing.Invoice{Number: "inv-001", Total: 150})
	require.NoError(t, err)
	inv := invCreate.SetAccountID(acc.ID).SaveX(ctx)

	for _, li := range []struct {
		desc string
		qty  int
	}{{"widget", 2}, {"gadget", 1}} {
		c.LineItem.Create().SetDescription(li.desc).SetQty(li.qty).SetInvoice(inv).SaveX(ctx)
	}
	return acc.ID, inv.ID
}

// ent → domain across two slices: nested inside billing, ID across slices.
func TestSlicedE2E_ToDomain(t *testing.T) {
	c := newClient(t)
	ctx := context.Background()
	accID, invID := seed(t, c)

	inv := c.Invoice.Query().WithLineItems().WithAccount().OnlyX(ctx)
	d := inv.ToDomain()
	assert.Equal(t, invID, d.ID)
	assert.Equal(t, "inv-001", d.Number)
	assert.Equal(t, 150, d.Total)
	assert.Equal(t, accID, d.AccountID, "cross-slice edge arrives as a bare ID")
	require.Len(t, d.LineItems, 2, "same-slice edge stays nested")
	assert.Equal(t, "widget", d.LineItems[0].Description)

	acc := c.Account.Query().WithInvoices().OnlyX(ctx).ToDomain()
	assert.Equal(t, []int{invID}, acc.InvoiceIDs)
	assert.Equal(t, "a@example.com", acc.Email)
}

// Bulk create and list conversion use the slice's own list type.
func TestSlicedE2E_ListAndBulk(t *testing.T) {
	c := newClient(t)
	ctx := context.Background()
	accID, _ := seed(t, c)

	bulk, err := c.Invoice.CreateBulkDomain(ctx, billing.InvoiceList{
		{Number: "inv-002", Total: 10},
		{Number: "inv-003", Total: 20},
	})
	require.NoError(t, err)
	created := bulk.SaveX(ctx)
	require.Len(t, created, 2)
	c.Account.UpdateOneID(accID).AddInvoiceIDs(created[0].ID, created[1].ID).ExecX(ctx)

	list := ent.Invoices(c.Invoice.Query().WithAccount().AllX(ctx)).ToDomain()
	assert.IsType(t, billing.InvoiceList{}, list, "the slice's own list type")
	require.Len(t, list, 3)
	for _, inv := range list {
		assert.Equal(t, accID, inv.AccountID)
	}

	// A nil element is rejected with an error, not a panic.
	_, err = c.Invoice.CreateBulkDomain(ctx, billing.InvoiceList{nil})
	assert.ErrorContains(t, err, "nil Invoice at index 0")
}

// A non-unique cross-slice IDs edge (Account.invoices) is written by
// ApplyDomain: the identity slice links billing rows by bare ID. (ent itself
// refuses to steal an invoice that already belongs to another account.)
func TestSlicedE2E_ApplyDomainWritesCrossSliceIDs(t *testing.T) {
	c := newClient(t)
	ctx := context.Background()
	seed(t, c)
	free := c.Invoice.Create().SetNumber("inv-free").SetTotal(5).SaveX(ctx)

	create, err := c.Account.Create().ApplyDomain(ctx, &identity.Account{
		Email: "b@example.com", InvoiceIDs: []int{free.ID},
	})
	require.NoError(t, err)
	other := create.SaveX(ctx)

	got := c.Invoice.Query().Where(invoice.ID(free.ID)).WithAccount().OnlyX(ctx).ToDomain()
	assert.Equal(t, other.ID, got.AccountID, "the invoice is linked to the new account")

	// ent's constraint still applies: an already-owned invoice cannot be taken.
	_, err = c.Account.Create().SetEmail("c@example.com").AddInvoiceIDs(free.ID).Save(ctx)
	assert.True(t, ent.IsConstraintError(err))
}

// The unique cross-slice edge is read-only in ApplyDomain: a domain struct with
// a different AccountID does not relink.
func TestSlicedE2E_UniqueIDsEdgeIsReadOnlyInApplyDomain(t *testing.T) {
	c := newClient(t)
	ctx := context.Background()
	accID, invID := seed(t, c)
	other := c.Account.Create().SetEmail("b@example.com").SaveX(ctx)

	upd, err := c.Invoice.UpdateOneID(invID).ApplyDomain(ctx, &billing.Invoice{
		ID: invID, Number: "inv-001", Total: 999, AccountID: other.ID,
	})
	require.NoError(t, err)
	upd.SaveX(ctx)

	got := c.Invoice.Query().WithAccount().OnlyX(ctx).ToDomain()
	assert.Equal(t, 999, got.Total, "scalar fields are applied")
	assert.Equal(t, accID, got.AccountID, "the unique IDs edge is left alone")
}

// domain ↔ proto through the per-slice pbmap packages, including the
// hand-written WithConversion functions in billing/pbmap.
func TestSlicedE2E_ProtoRoundTrip(t *testing.T) {
	c := newClient(t)
	ctx := context.Background()
	accID, invID := seed(t, c)

	d := c.Invoice.Query().WithLineItems().WithAccount().OnlyX(ctx).ToDomain()
	p := billingpb.InvoiceToProto(d)
	require.NotNil(t, p)
	assert.Equal(t, int64(invID), p.Id)
	assert.Equal(t, int64(accID), p.AccountId, "cross-slice ID is a plain int64")
	assert.Equal(t, "INV-001", p.Number, "WithConversion function from billing/pbmap ran")
	require.Len(t, p.LineItems, 2)
	assert.Equal(t, "widget", p.LineItems[0].Description)

	back := billingpb.InvoiceFromProto(&entpb.Invoice{Id: p.Id, AccountId: p.AccountId, Number: "inv-009", Total: 7})
	assert.Equal(t, accID, back.AccountID)
	assert.Equal(t, "INV-009", back.Number)

	ap := identitypb.AccountToProto(c.Account.Query().WithInvoices().OnlyX(ctx).ToDomain())
	assert.Equal(t, []int64{int64(invID)}, ap.InvoiceIds)
	assert.Equal(t, "a@example.com", ap.Email)
}
