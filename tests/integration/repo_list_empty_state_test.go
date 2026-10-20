// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/tests"

	"github.com/stretchr/testify/assert"
)

// TestRepoListEmptyStateDoesNotOfferSubjectCreation covers #369: the article list shown on user
// profiles, org homes, forks and subscriptions used to answer an empty search with "Create new
// subject", even for a subject that already exists. Following that link silently attaches a fork
// to the existing subject, so these pages now only say that nothing matched.
func TestRepoListEmptyStateDoesNotOfferSubjectCreation(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	// example-subject exists (it is repo1's subject), but neither user4 nor org3 has an article on it.
	const keyword = "example-subject"
	createSelector := `a[href^="` + setting.AppSubURL + `/repo/create?subject="]`

	cases := map[string]string{
		"user profile": "/user4?tab=repositories&q=" + url.QueryEscape(keyword),
		"org home":     "/org3?q=" + url.QueryEscape(keyword),
	}
	for name, path := range cases {
		t.Run(name, func(t *testing.T) {
			resp := MakeRequest(t, NewRequest(t, "GET", path), http.StatusOK)
			h := NewHTMLParser(t, resp.Body)
			assert.Equal(t, 0, h.Find(createSelector).Length(), "must not offer to create a subject")
			text := strings.Join(strings.Fields(h.Find(".flex-list").Text()), " ")
			assert.Contains(t, text, "No articles match “example-subject”.")
			assert.NotContains(t, text, "Want to create it?")
		})
	}

	t.Run("no keyword", func(t *testing.T) {
		resp := MakeRequest(t, NewRequest(t, "GET", "/user4?tab=repositories"), http.StatusOK)
		h := NewHTMLParser(t, resp.Body)
		assert.Equal(t, 0, h.Find(createSelector).Length())
		text := strings.Join(strings.Fields(h.Find(".flex-list").Text()), " ")
		assert.Contains(t, text, "No articles.")
		assert.NotContains(t, text, "No articles match")
	})
}
