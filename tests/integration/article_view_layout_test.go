// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/tests"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestArticleViewLayout covers the Article view's Read, Edit and History tabs as the
// layout draws them (#433, #434, #435).
func TestArticleViewLayout(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	owner, repo, subjectName := loadArticleRepo(t, 1)
	articleURL := fmt.Sprintf("/subject/%s/%s", subjectName, owner.Name)

	get := func(t *testing.T, session *TestSession, query string) *HTMLDoc {
		t.Helper()
		req := NewRequest(t, "GET", articleURL+"?"+query)
		var resp *httptest.ResponseRecorder
		if session != nil {
			resp = session.MakeRequest(t, req, http.StatusOK)
		} else {
			resp = MakeRequest(t, req, http.StatusOK)
		}
		return NewHTMLParser(t, resp.Body)
	}

	t.Run("ReadSignedOut", func(t *testing.T) {
		doc := get(t, nil, "mode=read")
		section := doc.Find(".history-view-section--article")
		require.Equal(t, 1, section.Length())

		// #433 items 5 and 9: no placeholder line, no back link
		assert.NotContains(t, section.Text(), "marked this article as correct")
		assert.Equal(t, 0, section.Find(`a[href*="view=bubble"]`).Length())

		// item 2: "N Contributors" and "N Last updated: …"
		tokens := strings.Join(strings.Fields(section.Find(`[data-role="article-tokens"]`).Text()), " ")
		assert.Regexp(t, `\d+ Contributors?`, tokens)
		assert.Regexp(t, `\d+ Last updated:`, tokens)
		assert.NotContains(t, tokens, "Updated ")

		// item 3: Watch, which a signed-out reader is sent to sign in for
		watch := section.Find(`#article-tabs [data-role="article-watch"]`)
		require.Equal(t, 1, watch.Length())
		assert.Contains(t, watch.AttrOr("href", ""), "/user/login?redirect_to=")

		// item 4: the path names the article by its subject's name, both parts linked
		crumbs := section.Find(`[data-role="article-breadcrumb"] a`)
		require.Equal(t, 2, crumbs.Length())
		assert.Equal(t, owner.Name, strings.TrimSpace(crumbs.First().Text()))
		assert.Equal(t, subjectName, strings.TrimSpace(crumbs.Last().Text()))

		// CopyRef in the Read tab
		assert.Equal(t, 1, section.Find(".article-copy-ref").Length())
	})

	t.Run("ReadShowsSummary", func(t *testing.T) {
		repo.Description = "The summary of this article"
		require.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(t.Context(), repo, "description"))
		defer func() {
			repo.Description = ""
			_ = repo_model.UpdateRepositoryColsNoAutoTime(t.Context(), repo, "description")
		}()

		doc := get(t, nil, "mode=read")
		assert.Equal(t, "The summary of this article", strings.TrimSpace(doc.Find(`p[data-role="article-summary"]`).Text()))
	})

	t.Run("EditSignedOut", func(t *testing.T) {
		doc := get(t, nil, "mode=edit")

		// #434 item 13: the banner (shown by CSS while the Edit tab is open)
		banner := doc.Find("#article-signin-banner")
		require.Equal(t, 1, banner.Length())
		assert.Contains(t, banner.Text(), "You are not logged in.")

		// items 8 and 9: Fork and Submit changes, both leading to sign in
		tabs := doc.Find("#article-tabs")
		assert.NotContains(t, tabs.Text(), "Sign in to Edit")
		for _, id := range []string{"#signed-out-fork-button", "#signed-out-submit-button"} {
			assert.Contains(t, tabs.Find(id).AttrOr("href", ""), "/user/login?redirect_to=", id)
		}

		// items 5 and 12: the summary block and the contribution tip
		assert.Equal(t, 1, doc.Find(`.article-summary-field[data-role="article-summary"]`).Length())
		tip := strings.Join(strings.Fields(doc.Find(`[data-role="article-contribution-tip"]`).Text()), " ")
		assert.Equal(t, "Remember, contributions to this article should follow its contributing guidelines and code of conduct.", tip)
	})

	t.Run("HistoryChanges", func(t *testing.T) {
		doc := get(t, nil, "mode=history")
		table := doc.Find(`[data-role="article-history"][data-sort="changes"]`)
		require.Equal(t, 1, table.Length())

		// #435 item 7: no CopyRef, item 6: the Sort menu
		assert.Equal(t, 0, doc.Find(".article-copy-ref").Length())
		require.Equal(t, 1, doc.Find(`[data-role="article-history-sort"]`).Length())

		rows := table.Find("#commits-table tbody tr")
		require.Positive(t, rows.Length())
		// item 12: every message expands to its full commit id
		assert.Equal(t, rows.Length(), table.Find("details.article-history-details .article-history-sha").Length())
		// item 13: the footer counts the rows
		assert.Equal(t, fmt.Sprintf("%d of %d", rows.Length(), rows.Length()),
			strings.TrimSpace(table.Find(".article-history-count").Text()))
		assert.Equal(t, 1, table.Find(".article-history-pager").Length())
	})

	t.Run("HistoryForks", func(t *testing.T) {
		doc := get(t, nil, "mode=history&history=forks")
		// the menu marks Forks as chosen
		active := doc.Find(`[data-role="article-history-sort"] .menu a.item.active`)
		require.Equal(t, 1, active.Length())
		assert.Contains(t, active.AttrOr("href", ""), "history=forks")
		assert.Equal(t, 0, doc.Find("#commits-table").Length())
	})

	t.Run("WatchButtonTogglesItself", func(t *testing.T) {
		session := loginUser(t, "user4")
		doc := get(t, session, "mode=read")
		form := doc.Find("#article-tabs form.article-watch-form")
		require.Equal(t, 1, form.Length())
		action := form.AttrOr("action", "")
		require.Regexp(t, `/action/(un)?watch\?style=button$`, action)
		next := "/action/unwatch?style=button" // what the button offers once it has been pressed
		if strings.Contains(action, "/action/unwatch") {
			next = "/action/watch?style=button"
		}

		req := NewRequestWithValues(t, "POST", action, map[string]string{"_csrf": GetUserCSRFToken(t, session)})
		resp := session.MakeRequest(t, req, http.StatusOK)
		answer := NewHTMLParser(t, resp.Body)
		// the answer is the Watch button again, now offering the other action, not the
		// subject header's Follow star
		require.Equal(t, 1, answer.Find("form.article-watch-form").Length())
		assert.Contains(t, answer.Find("form.article-watch-form").AttrOr("action", ""), next)
		assert.Equal(t, 0, answer.Find(".follow-article-button").Length())
	})
}
