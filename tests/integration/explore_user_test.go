// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"net/http"
	"testing"

	"code.gitea.io/gitea/tests"

	"github.com/stretchr/testify/assert"
)

func TestExploreUser(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	// The sort dropdown renders its options as radio inputs (JS-driven, see
	// repo-search.ts) rather than links, so the "active"/current sort order is read
	// off the checked radio's value instead of an anchor's href.
	cases := []struct{ sortOrder, expected string }{
		{"", "newest"},
		{"newest", "newest"},
		{"oldest", "oldest"},
		{"alphabetically", "alphabetically"},
		{"reversealphabetically", "reversealphabetically"},
	}
	for _, c := range cases {
		req := NewRequest(t, "GET", "/explore/users?sort="+c.sortOrder)
		resp := MakeRequest(t, req, http.StatusOK)
		h := NewHTMLParser(t, resp.Body)
		value, exists := h.Find(`.ui.dropdown .menu input[name="sort"][checked]`).Attr("value")
		assert.True(t, exists)
		assert.Equal(t, c.expected, value)
	}

	// these sort orders shouldn't be supported, to avoid leaking user activity
	cases404 := []string{
		"/explore/users?sort=lastlogin",
		"/explore/users?sort=reverselastlogin",
		"/explore/users?sort=leastupdate",
		"/explore/users?sort=reverseleastupdate",
	}
	for _, c := range cases404 {
		req := NewRequest(t, "GET", c).SetHeader("Accept", "text/html")
		MakeRequest(t, req, http.StatusNotFound)
	}
}
