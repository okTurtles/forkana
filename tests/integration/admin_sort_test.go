// Copyright 2025 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"net/http"
	"testing"

	"code.gitea.io/gitea/tests"

	"github.com/stretchr/testify/assert"
)

// TestAdminListsKeepDefaultSortIndicators pins the admin listings against the fix for #292,
// which stops the Explore sort dropdown reporting a selection the user never made by leaving
// "SortType" empty until the request names a sort. The admin tables are rendered by the very
// same handlers and read the same value -- for the column sort arrow and for the sort
// dropdown they inherit -- so they must keep showing which order they are actually in, with
// no sort parameter in the URL.
func TestAdminListsKeepDefaultSortIndicators(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	session := loginUser(t, "user1")

	// The admin user list sorts alphabetically by default and marks the Name column with an
	// ascending arrow. sortArrow() renders nothing at all for an empty sort, so an empty
	// SortType would silently strip every arrow from this table.
	req := NewRequest(t, "GET", "/-/admin/users")
	h := NewHTMLParser(t, session.MakeRequest(t, req, http.StatusOK).Body)
	assert.Positive(t, h.Find(`table thead th .svg.octicon-triangle-up`).Length(), "the default-sorted column keeps its arrow")

	// The admin repository list shares the Explore search partial, sort dropdown included,
	// and keeps marking the order it is really in.
	req = NewRequest(t, "GET", "/-/admin/repos")
	h = NewHTMLParser(t, session.MakeRequest(t, req, http.StatusOK).Body)
	active := h.Find(`.menu label.active.item`)
	assert.Equal(t, 1, active.Length())
	value, exists := active.Find(`input[name="sort"]`).Attr("value")
	assert.True(t, exists)
	assert.Equal(t, "recentupdate", value)
}
