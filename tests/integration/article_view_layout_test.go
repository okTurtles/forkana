// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	repo_service "code.gitea.io/gitea/services/repository"
	files_service "code.gitea.io/gitea/services/repository/files"
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

		// item 2: "N Contributors" and "Last updated: …", with no number before the latter
		tokens := strings.Join(strings.Fields(section.Find(`[data-role="article-tokens"]`).Text()), " ")
		assert.Regexp(t, `\d+ Contributors?`, tokens)
		assert.NotContains(t, tokens, "Updated ")
		lastUpdated := strings.Join(strings.Fields(section.Find(`[data-role="article-last-updated"]`).Text()), " ")
		assert.Regexp(t, `^Last updated: `, lastUpdated)

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
		original := repo.Description
		repo.Description = "The summary of this article"
		require.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(t.Context(), repo, "description"))
		defer func() {
			repo.Description = original
			_ = repo_model.UpdateRepositoryColsNoAutoTime(t.Context(), repo, "description")
		}()

		doc := get(t, nil, "mode=read")
		assert.Equal(t, "The summary of this article", strings.TrimSpace(doc.Find(`p[data-role="article-summary"]`).Text()))
	})

	t.Run("WatchInEveryTab", func(t *testing.T) {
		for _, mode := range []string{"read", "edit", "history"} {
			doc := get(t, nil, "mode="+mode)
			assert.Equal(t, 1, doc.Find(`#article-tabs [data-role="article-watch"]`).Length(), mode)
		}
	})

	t.Run("SignInLinksKeepThePage", func(t *testing.T) {
		doc := get(t, nil, "mode=edit")
		want := "redirect_to=" + url.QueryEscape(articleURL+"?mode=edit")
		assert.Contains(t, doc.Find("#signed-out-fork-button").AttrOr("href", ""), want)
		assert.Contains(t, doc.Find(`#article-tabs [data-role="article-watch"]`).AttrOr("href", ""), want)
		assert.Contains(t, doc.Find(`#article-signin-banner a[href*="/user/login"]`).AttrOr("href", ""), want)
	})

	t.Run("NoBannerForASignedInReader", func(t *testing.T) {
		doc := get(t, loginUser(t, "user4"), "mode=edit")
		require.Equal(t, 1, doc.Find(`#article-view-root[data-article-mode="edit"]`).Length())
		assert.Equal(t, 0, doc.Find("#article-signin-banner").Length())
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
		assert.Equal(t, "Up to 200 characters.", strings.TrimSpace(doc.Find(".article-summary-field .article-summary-caption").Text()))
		// Fork and its count are one control, the count split off (ButtonGroup I6961:75221)
		fork := tabs.Find("a#signed-out-fork-button.forkana-btn-group")
		require.Equal(t, 1, fork.Length())
		assert.Equal(t, 2, fork.Find(".forkana-btn-group-item").Length())
		assert.Equal(t, strconv.Itoa(repo.NumForks), strings.TrimSpace(fork.Find(`[data-role="article-fork-count"]`).Text()))
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
		// item 13: the footer gives the range of the rows shown
		assert.Equal(t, fmt.Sprintf("1-%d of %d", rows.Length(), rows.Length()),
			strings.TrimSpace(table.Find(".article-history-count").Text()))
		assert.Equal(t, 1, table.Find(".article-history-pager").Length())
		// every row of user2's own article is a change made in it
		assert.Equal(t, rows.Length(), table.Find(".col-icon .octicon-git-commit").Length())
	})

	t.Run("HistoryPastTheLastPage", func(t *testing.T) {
		doc := get(t, nil, "mode=history&page=99")
		table := doc.Find(`[data-role="article-history"][data-sort="changes"]`)
		require.Equal(t, 1, table.Length(), "a page past the last keeps the table and its pager")
		assert.Equal(t, 0, table.Find("#commits-table tbody tr").Length())
		assert.Equal(t, "Nothing on this page", strings.TrimSpace(table.Find(".article-history-count").Text()))
		// the pager leads back: every page is a link, the last (current) one included
		assert.Contains(t, table.Find(`.article-history-pager a.article-history-page`).First().AttrOr("href", ""), "page=1")
		assert.Equal(t, 0, table.Find(`.article-history-pager [aria-current="page"]`).Length())
	})

	t.Run("HistoryNoForks", func(t *testing.T) {
		doc := get(t, nil, "mode=history&history=forks")
		// the menu marks Forks as chosen
		active := doc.Find(`[data-role="article-history-sort"] .menu a.item.active`)
		require.Equal(t, 1, active.Length())
		assert.Contains(t, active.AttrOr("href", ""), "history=forks")
		assert.Equal(t, 0, doc.Find("#commits-table").Length())
		assert.Equal(t, 1, doc.Find(`[data-role="article-history-no-forks"]`).Length())
	})

	t.Run("Forks", func(t *testing.T) {
		user4 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
		user5 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 5})
		publicFork, err := repo_service.ForkRepository(t.Context(), user4, user4, repo_service.ForkRepoOptions{
			BaseRepo: repo, Name: repo.Name, SingleBranch: repo.DefaultBranch,
		})
		require.NoError(t, err)
		defer func() { _ = repo_service.DeleteRepositoryDirectly(t.Context(), publicFork.ID) }()
		privateFork, err := repo_service.ForkRepository(t.Context(), user5, user5, repo_service.ForkRepoOptions{
			BaseRepo: repo, Name: repo.Name, SingleBranch: repo.DefaultBranch,
		})
		require.NoError(t, err)
		defer func() { _ = repo_service.DeleteRepositoryDirectly(t.Context(), privateFork.ID) }()
		privateFork.IsPrivate = true
		require.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(t.Context(), privateFork, "is_private"))
		// a deleted fork that had forks of its own stays as a tombstone, which no listing shows
		user8 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 8})
		tombstone, err := repo_service.ForkRepository(t.Context(), user8, user8, repo_service.ForkRepoOptions{
			BaseRepo: repo, Name: repo.Name, SingleBranch: repo.DefaultBranch,
		})
		require.NoError(t, err)
		defer func() { _ = repo_service.DeleteRepositoryDirectly(t.Context(), tombstone.ID) }()
		tombstone.IsTombstoned = true
		require.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(t.Context(), tombstone, "is_tombstoned"))

		t.Run("ListingHidesThePrivateFork", func(t *testing.T) {
			doc := get(t, nil, "mode=history&history=forks")
			table := doc.Find(`[data-role="article-history"][data-sort="forks"]`)
			require.Equal(t, 1, table.Length())
			rows := table.Find("#forks-table tbody tr")
			require.Equal(t, 1, rows.Length(), "a signed-out reader only sees the public fork")
			assert.Equal(t, "user4", strings.TrimSpace(rows.Find(".author-wrapper").Text()))
			// the head of its default branch, as the branch table records it
			assert.Regexp(t, `^[0-9a-f]{10}$`, strings.TrimSpace(rows.Find(".col-code a").Text()))
			// no description: the fork is named by its owner and subject
			assert.Equal(t, "user4 / "+subjectName, strings.TrimSpace(rows.Find(".article-history-message").Text()))
			assert.Contains(t, rows.Find(".article-history-message").AttrOr("href", ""), "/subject/"+subjectName+"/user4")
			assert.Equal(t, "1-1 of 1", strings.TrimSpace(table.Find(".article-history-count").Text()))
		})

		t.Run("ListingShowsThePrivateForkToItsOwner", func(t *testing.T) {
			doc := get(t, loginUser(t, user5.Name), "mode=history&history=forks")
			assert.Equal(t, 2, doc.Find("#forks-table tbody tr").Length())
		})

		// #433 item 6 on both routes to a fork: its own url, and the subject page with
		// the fork selected (which used to leave the base repository unloaded)
		for _, u := range []string{
			fmt.Sprintf("/subject/%s/%s?mode=read", subjectName, user4.Name),
			fmt.Sprintf("/subject/%s?view=article&selected=%s/%s", subjectName, user4.Name, publicFork.Name),
		} {
			resp := MakeRequest(t, NewRequest(t, "GET", u), http.StatusOK)
			forkOf := NewHTMLParser(t, resp.Body).Find(`.history-view-section--article [data-role="article-fork-of"]`)
			require.Equal(t, 1, forkOf.Length(), u)
			assert.Equal(t, "Fork of: user2 / "+subjectName, strings.Join(strings.Fields(forkOf.Text()), " "), u)
			assert.Contains(t, forkOf.Find("a").AttrOr("href", ""), "/subject/"+subjectName+"/user2", u)
		}
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

		// with the star's article id (what hx-include sends), the answer also re-renders
		// the star, out of band; with another article's id, it does not
		back := answer.Find("form.article-watch-form").AttrOr("action", "")
		req = NewRequestWithValues(t, "POST", back, map[string]string{
			"_csrf":            GetUserCSRFToken(t, session),
			"sync_follow_repo": strconv.FormatInt(repo.ID, 10),
		})
		answer = NewHTMLParser(t, session.MakeRequest(t, req, http.StatusOK).Body)
		oob := answer.Find(`#repo-header-follow-slot[hx-swap-oob]`)
		require.Equal(t, 1, oob.Length())
		assert.Equal(t, 1, oob.Find(".follow-article-button").Length())

		req = NewRequestWithValues(t, "POST", answer.Find("form.article-watch-form").AttrOr("action", ""), map[string]string{
			"_csrf":            GetUserCSRFToken(t, session),
			"sync_follow_repo": "999999",
		})
		answer = NewHTMLParser(t, session.MakeRequest(t, req, http.StatusOK).Body)
		assert.Equal(t, 0, answer.Find(`[hx-swap-oob]`).Length())

		// the star's answer re-renders the Watch button of the same article
		starAction := strings.Replace(answer.Find("form.article-watch-form").AttrOr("action", ""), "?style=button", "", 1)
		req = NewRequestWithValues(t, "POST", starAction, map[string]string{
			"_csrf":            GetUserCSRFToken(t, session),
			"sync_button_repo": strconv.FormatInt(repo.ID, 10),
		})
		answer = NewHTMLParser(t, session.MakeRequest(t, req, http.StatusOK).Body)
		assert.Equal(t, 1, answer.Find(".follow-article-button").Length())
		assert.Equal(t, 1, answer.Find(`#article-watch-slot[hx-swap-oob] form.article-watch-form`).Length())

		// a crafted request with both ids is answered with one control and one out-of-band
		// fragment, whatever its style: the two templates must not include each other on
		for _, action := range []string{starAction, starAction + "?style=button"} {
			action = strings.Replace(action, "/action/watch", "/action/unwatch", 1)
			req = NewRequestWithValues(t, "POST", action, map[string]string{
				"_csrf":            GetUserCSRFToken(t, session),
				"sync_follow_repo": strconv.FormatInt(repo.ID, 10),
				"sync_button_repo": strconv.FormatInt(repo.ID, 10),
			})
			answer = NewHTMLParser(t, session.MakeRequest(t, req, http.StatusOK).Body)
			assert.Equal(t, 1, answer.Find(`[hx-swap-oob]`).Length(), action)
			assert.Equal(t, 2, answer.Find("form").Length(), action)
		}
	})
}

// TestArticleReadHidesTheFrontMatter: the Read tab does not show the article's front
// matter, as the figma's Read tab (641:65454) has nothing between the summary and the
// article (#433). (A commit needs the running server for its hooks, hence onGiteaRun.)
func TestArticleReadHidesTheFrontMatter(t *testing.T) {
	onGiteaRun(t, func(t *testing.T, _ *url.URL) {
		owner, repo, subjectName := loadArticleRepo(t, 1)
		_, err := files_service.ChangeRepoFiles(t.Context(), repo, owner, &files_service.ChangeRepoFilesOptions{
			Files: []*files_service.ChangeRepoFile{{
				Operation: "update",
				TreePath:  "README.md",
				ContentReader: strings.NewReader("---\ntitle: \"Prodromus\"\nsource: https://en.wikipedia.org/wiki/Prodromus\n" +
					"license: CC BY-SA 4.0\nattribution: Wikipedia contributors\nfetched_at: 2025-11-11T13:56:22Z\n---\n\nThe article itself.\n"),
			}},
			Message:   "front matter",
			OldBranch: repo.DefaultBranch,
			NewBranch: repo.DefaultBranch,
		})
		require.NoError(t, err)

		resp := MakeRequest(t, NewRequest(t, "GET", fmt.Sprintf("/subject/%s/%s?mode=read", subjectName, owner.Name)), http.StatusOK)
		content := NewHTMLParser(t, resp.Body).Find(".file-view.article-content")
		require.Equal(t, 1, content.Length())
		assert.Equal(t, "The article itself.", strings.TrimSpace(content.Text()))
		assert.Equal(t, 0, content.Find(".frontmatter-content, table, hr, h1, h2").Length())
	})
}
