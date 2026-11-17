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

// E2E test for ENTD-010: the README "HTTP Handler Usage" handler, as
// written, behind a real HTTP server. FIQL's `;` (AND) must reach the
// parser even though net/url rejects it as a query separator.

package ent_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danhtran94/entdomain"
	"github.com/danhtran94/entdomain/examples/basic/ent"
	"github.com/danhtran94/entdomain/examples/basic/ent/enttest"
)

// userListHandler is the README "HTTP Handler Usage" handler; keep the two
// identical (scope success criterion 9).
func userListHandler(client *ent.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := client.User.Query()
		// Not r.URL.Query().Get: net/url drops any pair containing ';' (FIQL AND).
		expr, ok, err := entdomain.FIQLQueryParam(r.URL.RawQuery, "filter")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if ok { // present, even if empty: an empty filter is an error, not "no filter"
			pred, err := ent.UserFIQL(expr)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			q = q.Where(pred)
		}
		users, err := q.All(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(len(users))
	}
}

func TestReadmeFIQLHandler(t *testing.T) {
	ctx := context.Background()
	withUserTransformer(t, nil)
	client := enttest.Open(t, "sqlite3", "file:"+t.Name()+"?mode=memory&_fk=1")
	t.Cleanup(func() { _ = client.Close() })

	alice, bob := sampleUser(), sampleUser() // both score 100
	bob.Name, bob.Username = "bob", "bob"
	client.User.Create().ApplyDomainX(ctx, alice).ExecX(ctx)
	client.User.Create().ApplyDomainX(ctx, bob).ExecX(ctx)

	srv := httptest.NewServer(userListHandler(client))
	t.Cleanup(srv.Close)

	get := func(rawQuery string) (status, rows int) {
		t.Helper()
		resp, err := http.Get(srv.URL + "/users?" + rawQuery)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode == http.StatusOK {
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&rows))
		}
		return resp.StatusCode, rows
	}

	cases := []struct {
		name       string
		rawQuery   string
		wantStatus int
		wantRows   int
	}{
		{"no filter", "", http.StatusOK, 2},
		{"single term", "filter=name==alice", http.StatusOK, 1},
		{"AND with raw semicolon", "filter=name==alice;score=gt=10", http.StatusOK, 1},
		{"AND, no match", "filter=name==alice;score=gt=1000", http.StatusOK, 0},
		{"percent-encoded semicolon", "filter=name==alice%3Bscore=gt=10", http.StatusOK, 1},
		{"present but empty", "filter=", http.StatusBadRequest, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, rows := get(tc.rawQuery)
			assert.Equal(t, tc.wantStatus, status)
			assert.Equal(t, tc.wantRows, rows)
		})
	}
}
