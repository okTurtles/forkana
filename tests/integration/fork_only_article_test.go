// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"testing"

	auth_model "code.gitea.io/gitea/models/auth"
	"code.gitea.io/gitea/models/db"
	issues_model "code.gitea.io/gitea/models/issues"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	api "code.gitea.io/gitea/modules/structs"
	"code.gitea.io/gitea/modules/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestForkOnlyArticle covers #156: nobody reviews the change requests of an article whose
// owner is fork-only, so the Edit tab says so and offers a fork, and opening a change
// request against it is refused, from the editor and the API (the compare page runs the
// same repo_service.CheckArticleAcceptsChangeRequest check as the API).
func TestForkOnlyArticle(t *testing.T) {
	onGiteaRun(t, testForkOnlyArticle)
}

func testForkOnlyArticle(t *testing.T, _ *url.URL) {
	// user2 owns repo1, an article of subject 1; user4 owns nothing for it
	owner, repo, subjectName := loadArticleRepo(t, 1)
	editor := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
	articleURL := fmt.Sprintf("/subject/%s/%s", subjectName, owner.Name)
	editURL := path.Join(owner.Name, repo.Name, "_edit", repo.DefaultBranch, "README.md")
	session := loginUser(t, editor.Name)

	setForkOnly := func(t *testing.T, forkOnly bool) {
		owner.IsForkOnly = forkOnly
		_, err := db.GetEngine(t.Context()).ID(owner.ID).Cols("is_fork_only").Update(owner)
		require.NoError(t, err)
	}
	countPulls := func(t *testing.T) int64 {
		n, err := db.GetEngine(t.Context()).Where("base_repo_id = ?", repo.ID).Count(new(issues_model.PullRequest))
		require.NoError(t, err)
		return n
	}
	editTab := func(t *testing.T) *HTMLDoc {
		resp := session.MakeRequest(t, NewRequest(t, "GET", articleURL+"?mode=edit"), http.StatusOK)
		return NewHTMLParser(t, resp.Body)
	}

	t.Run("EditTabOfAnArticleAcceptingChangeRequests", func(t *testing.T) {
		doc := editTab(t)
		assert.Equal(t, 0, doc.Find(`[data-role="article-fork-only-notice"]`).Length())
		assert.Equal(t, 1, doc.Find("#pre-submit-changes-button").Length())
		assert.Equal(t, 0, doc.Find("#submit-changes-button").Length())
		assert.Equal(t, 1, doc.Find("#fork-article-button").Length())
	})

	setForkOnly(t, true)
	defer setForkOnly(t, false)

	t.Run("EditTabSaysItCreatesANewArticle", func(t *testing.T) {
		doc := editTab(t)
		notice := doc.Find(`[data-role="article-fork-only-notice"]`)
		require.Equal(t, 1, notice.Length())
		assert.Equal(t,
			fmt.Sprintf("This will create a new article because the user %s doesn't accept change requests", owner.Name),
			strings.Join(strings.Fields(notice.Text()), " "))

		// Submit changes forks instead of opening the change request modal, and is the
		// only action: no Fork button doing the same
		assert.Equal(t, 0, doc.Find("#pre-submit-changes-button").Length())
		assert.Equal(t, 0, doc.Find("#fork-article-button").Length())
		submit := doc.Find("#submit-changes-button")
		require.Equal(t, 1, submit.Length())
		assert.Equal(t, "true", submit.AttrOr("data-fork-and-edit", ""))
		assert.Equal(t, "true", doc.Find("#article-edit-form #fork_and_edit").AttrOr("value", ""))
	})

	t.Run("OwnerSeesNoNotice", func(t *testing.T) {
		resp := loginUser(t, owner.Name).MakeRequest(t, NewRequest(t, "GET", articleURL+"?mode=edit"), http.StatusOK)
		doc := NewHTMLParser(t, resp.Body)
		assert.Equal(t, 0, doc.Find(`[data-role="article-fork-only-notice"]`).Length())
	})

	t.Run("SubmitChangeRequestIsRefused", func(t *testing.T) {
		before := countPulls(t)
		resp := session.MakeRequest(t, NewRequest(t, "GET", editURL+"?submit_change_request=true"), http.StatusOK)
		doc := NewHTMLParser(t, resp.Body)
		req := NewRequestWithValues(t, "POST", editURL+"?submit_change_request=true", map[string]string{
			"_csrf":                 doc.GetCSRF(),
			"last_commit":           doc.GetInputValueByName("last_commit"),
			"tree_path":             "README.md",
			"content":               "# A change request nobody would read\n",
			"commit_choice":         "direct",
			"submit_change_request": "true",
			"change_request_title":  "Fork-only",
		})
		resp = session.MakeRequest(t, req, http.StatusBadRequest)
		assert.Contains(t, resp.Body.String(), "doesn't accept change requests")
		assert.Equal(t, before, countPulls(t), "no change request is opened")
	})

	t.Run("APIPullRequestIsRefused", func(t *testing.T) {
		token := getTokenForLoggedInUser(t, session, auth_model.AccessTokenScopeWriteRepository)
		req := NewRequestWithJSON(t, "POST", fmt.Sprintf("/api/v1/repos/%s/%s/pulls", owner.Name, repo.Name), &api.CreatePullRequestOption{
			Head:  "branch2",
			Base:  repo.DefaultBranch,
			Title: "Fork-only",
		}).AddTokenAuth(token)
		MakeRequest(t, req, http.StatusForbidden)
	})

	t.Run("SubmitChangesForksTheArticle", func(t *testing.T) {
		resp := session.MakeRequest(t, NewRequest(t, "GET", editURL), http.StatusOK)
		doc := NewHTMLParser(t, resp.Body)
		req := NewRequestWithValues(t, "POST", editURL, map[string]string{
			"_csrf":         doc.GetCSRF(),
			"last_commit":   doc.GetInputValueByName("last_commit"),
			"tree_path":     "README.md",
			"content":       "# My own version\n",
			"commit_choice": "direct",
			"fork_and_edit": "true",
		})
		resp = session.MakeRequest(t, req, http.StatusOK)
		fork, err := repo_model.GetForkedRepo(t.Context(), editor.ID, repo.ID)
		require.NoError(t, err)
		require.NotNil(t, fork, "the editor has their own article now")
		assert.Equal(t, fork.Link()+"?mode=read", test.RedirectURL(resp))
	})

	// an editor who already has a fork is sent to it, or told to unarchive it first
	t.Run("EditTabWithAFork", func(t *testing.T) {
		fork, err := repo_model.GetForkedRepo(t.Context(), editor.ID, repo.ID)
		require.NoError(t, err)
		require.NotNil(t, fork)
		noticeText := func(t *testing.T) string {
			doc := editTab(t)
			assert.Equal(t, 0, doc.Find("#submit-changes-button, #pre-submit-changes-button").Length())
			return strings.Join(strings.Fields(doc.Find(`[data-role="article-fork-only-notice"]`).Text()), " ")
		}

		assert.Equal(t,
			fmt.Sprintf("The user %s doesn't accept change requests: edit your fork of this article instead", owner.Name),
			noticeText(t))

		require.NoError(t, repo_model.SetArchiveRepoState(t.Context(), fork, true))
		defer func() { require.NoError(t, repo_model.SetArchiveRepoState(t.Context(), fork, false)) }()
		assert.Equal(t,
			fmt.Sprintf("The user %s doesn't accept change requests, and your fork of this article is archived: unarchive it in its settings to edit it", owner.Name),
			noticeText(t))
	})

	// the compare page and the change request list offer no change request either (the
	// editor has a fork now, which would otherwise let them open one)
	t.Run("CompareAndPullListOfferNoChangeRequest", func(t *testing.T) {
		// the editor's fork, with their own version, against the article
		fork, err := repo_model.GetForkedRepo(t.Context(), editor.ID, repo.ID)
		require.NoError(t, err)
		require.NotNil(t, fork)
		compareURL := fmt.Sprintf("/%s/%s/compare/%s...%s/%s:%s", owner.Name, repo.Name, repo.DefaultBranch, editor.Name, fork.Name, fork.DefaultBranch)
		pullsURL := fmt.Sprintf("/%s/%s/pulls", owner.Name, repo.Name)
		get := func(t *testing.T, link string) *HTMLDoc {
			resp := session.MakeRequest(t, NewRequest(t, "GET", link), http.StatusOK)
			return NewHTMLParser(t, resp.Body)
		}

		doc := get(t, compareURL)
		notice := doc.Find(`[data-role="compare-fork-only-notice"]`)
		require.Equal(t, 1, notice.Length())
		assert.Contains(t, strings.Join(strings.Fields(notice.Text()), " "),
			fmt.Sprintf("The user %s doesn't accept change requests", owner.Name))
		assert.Contains(t, notice.Find("a").AttrOr("href", ""), "mode=edit")
		assert.Equal(t, 0, doc.Find(".pullrequest-form").Length())
		assert.Equal(t, 0, get(t, pullsURL).Find(".new-pr-button").Length())

		// once the owner accepts change requests again, both are back
		setForkOnly(t, false)
		defer setForkOnly(t, true)
		doc = get(t, compareURL)
		assert.Equal(t, 0, doc.Find(`[data-role="compare-fork-only-notice"]`).Length())
		assert.Equal(t, 1, doc.Find(".pullrequest-form").Length())
		assert.Equal(t, 1, get(t, pullsURL).Find(".new-pr-button").Length())
	})
}
