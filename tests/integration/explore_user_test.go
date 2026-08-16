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

func TestExploreSearchKeywordAcrossTabs(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	// switching explore tabs must keep the search keyword, it is carried over by the "q" query
	// parameter of the tab links themselves (the navigation is a plain page load, so nothing can
	// be kept in memory, and anonymous users have no session to store it in)
	subjectsTabLink := func(path string) string {
		req := NewRequest(t, "GET", path)
		resp := MakeRequest(t, req, http.StatusOK)
		h := NewHTMLParser(t, resp.Body)
		href, exists := h.Find(`a[data-explore-tab-link][href^="/explore/subjects"]`).Attr("href")
		assert.True(t, exists, "explore navbar should link to the subjects tab on %s", path)
		return href
	}

	assert.Equal(t, "/explore/subjects?q=user+two", subjectsTabLink("/explore/users?q=user%20two"))
	// an empty keyword must not add an empty "q" parameter
	assert.Equal(t, "/explore/subjects", subjectsTabLink("/explore/users"))
	assert.Equal(t, "/explore/subjects", subjectsTabLink("/explore/users?q="))
	// the sort order is not carried over, it is tab specific and unsupported values are rejected
	assert.Equal(t, "/explore/subjects?q=user+two", subjectsTabLink("/explore/users?q=user%20two&sort=oldest"))
}
