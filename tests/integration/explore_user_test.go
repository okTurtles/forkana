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
	// explore-search.ts) rather than links, so the "active"/current sort order is read
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
	tabHref := func(path, tabPrefix string) string {
		req := NewRequest(t, "GET", path)
		resp := MakeRequest(t, req, http.StatusOK)
		h := NewHTMLParser(t, resp.Body)
		href, exists := h.Find(`a[data-explore-tab-link][href^="` + tabPrefix + `"]`).Attr("href")
		assert.True(t, exists, "explore navbar should link to %s on %s", tabPrefix, path)
		return href
	}

	// the reported round trip: search on one tab, switch to the other, switch back
	assert.Equal(t, "/explore/subjects?q=user+two", tabHref("/explore/users?q=user%20two", "/explore/subjects"))
	assert.Equal(t, "/explore/users?q=user+two", tabHref("/explore/subjects?q=user+two", "/explore/users"))
	// an empty keyword must not add an empty "q" parameter
	assert.Equal(t, "/explore/subjects", tabHref("/explore/users", "/explore/subjects"))
	assert.Equal(t, "/explore/subjects", tabHref("/explore/users?q=", "/explore/subjects"))
	// the sort order is not carried over, it is tab specific and unsupported values are rejected
	assert.Equal(t, "/explore/subjects?q=user+two", tabHref("/explore/users?q=user%20two&sort=oldest", "/explore/subjects"))
	// the keyword lands in an href, so it must arrive query-escaped and never as raw markup
	assert.Equal(t, "/explore/users?q=%3Cscript%3E", tabHref("/explore/subjects?q=%3Cscript%3E", "/explore/users"))
	hostile := "%22%3E%3Cscript%3Ealert%281%29%3C%2Fscript%3E" // "><script>alert(1)</script>
	assert.Equal(t, "/explore/subjects?q="+hostile, tabHref("/explore/users?q="+hostile, "/explore/subjects"))
}
