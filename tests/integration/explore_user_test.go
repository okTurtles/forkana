// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"net/http"
	"strings"
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

func TestExploreUserSearchSplit(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	// NewHTMLParser drains resp.Body, so every assertion below reads the parsed document
	// rather than resp.Body.String() -- which would be empty by then, and quietly pass.
	listText := func(h *HTMLDoc) string {
		return h.Find(".explore-users-list-container").Text()
	}

	// A search on the users tab is answered in two parts: the user the keyword actually
	// names, then everything that merely contains it under "Similar" (#276). "user1" is a
	// real username; user10..user18 only contain it as a substring.
	req := NewRequest(t, "GET", "/explore/users?q=user1")
	resp := MakeRequest(t, req, http.StatusOK)
	h := NewHTMLParser(t, resp.Body)

	assert.Equal(t, "Search results for user1",
		strings.TrimSpace(h.Find(".explore-users-list-container .tw-font-bold").First().Text()))

	// the border is drawn per box, and only the rows go inside one
	boxes := h.Find(".explore-users-rows")
	assert.Equal(t, 2, boxes.Length(), "one box for the exact match, one for the similar users")
	assert.Equal(t, 1, boxes.Eq(0).Find(".flex-item").Length(), "the exact match stands alone")
	assert.Contains(t, boxes.Eq(0).Text(), "user1")
	assert.Positive(t, boxes.Eq(1).Find(".flex-item").Length(), "the substring matches follow")

	// a keyword that is only ever a substring names nobody, so it gets the note instead of
	// an exact-match box, and the similar list is the whole answer
	req = NewRequest(t, "GET", "/explore/users?q=ser1")
	resp = MakeRequest(t, req, http.StatusOK)
	h = NewHTMLParser(t, resp.Body)
	assert.Contains(t, listText(h), "No user named exactly")
	assert.Equal(t, 1, h.Find(".explore-users-rows").Length(), "only the similar box")

	// no keyword means no split at all, just the plain list
	req = NewRequest(t, "GET", "/explore/users")
	resp = MakeRequest(t, req, http.StatusOK)
	h = NewHTMLParser(t, resp.Body)
	assert.Equal(t, 1, h.Find(".explore-users-rows").Length())
	assert.NotContains(t, listText(h), "Search results for")
}

func TestExploreOrganizationsKeepsPlainList(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	// /explore/organizations is served by the same handler AND the same template as the
	// users tab (explore.Organizations calls RenderUserSearch with tplExploreUsers), so the
	// user-specific split must not leak onto it: an org listing answering a search with
	// "No user named exactly ..." would be plainly wrong (#276).
	req := NewRequest(t, "GET", "/explore/organizations?q=org")
	resp := MakeRequest(t, req, http.StatusOK)
	h := NewHTMLParser(t, resp.Body)
	listText := h.Find(".explore-users-list-container").Text()

	assert.NotContains(t, listText, "Search results for")
	assert.NotContains(t, listText, "No user named exactly")
	assert.Equal(t, 1, h.Find(".explore-users-rows").Length(), "a single plain list")
	assert.Positive(t, h.Find(".explore-users-rows .flex-item").Length(), "the orgs are still listed")
}
