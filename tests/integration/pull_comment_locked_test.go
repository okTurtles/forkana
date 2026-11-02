// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"fmt"
	"net/http"
	"strconv"
	"testing"

	auth_model "code.gitea.io/gitea/models/auth"
	issues_model "code.gitea.io/gitea/models/issues"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	api "code.gitea.io/gitea/modules/structs"
	issue_service "code.gitea.io/gitea/services/issue"
	"code.gitea.io/gitea/tests"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestChangeRequestContentLocked covers #146: the description and comments of a change request
// (pull request) can't be edited, deleted or referenced in a new issue, not even by an admin or
// by their poster. The page offers none of these actions and the endpoints refuse them.
func TestChangeRequestContentLocked(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	repo1 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	pull := unittest.AssertExistsAndLoadBean(t, &issues_model.Issue{RepoID: repo1.ID, Index: 3}) // /user2/repo1/pulls/3, posted by user1
	require.True(t, pull.IsPull)
	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2}) // repo owner
	comment, err := issue_service.CreateIssueComment(t.Context(), user2, repo1, pull, "a change request comment", nil)
	require.NoError(t, err)

	pullURL := fmt.Sprintf("/user2/repo1/pulls/%d", pull.Index)

	// user1 is a site admin and the poster of the change request, user2 owns the article and
	// posted the comment: between them they cover every reason the actions used to be offered.
	for _, user := range []string{"user1", "user2"} {
		t.Run("page/"+user, func(t *testing.T) {
			resp := loginUser(t, user).MakeRequest(t, NewRequest(t, "GET", pullURL), http.StatusOK)
			htmlDoc := NewHTMLParser(t, resp.Body)
			menus := htmlDoc.Find(".comment .context-dropdown .menu")
			assert.Positive(t, menus.Length(), "the comments should still have their menu")
			assert.Positive(t, menus.Find(".quote-reply").Length(), "Quote reply is kept")
			assert.Zero(t, menus.Find(".edit-content").Length(), "no Edit")
			assert.Zero(t, menus.Find(".delete-comment").Length(), "no Delete")
			assert.Zero(t, menus.Find(".reference-issue").Length(), "no Reference in new issue")
			assert.Zero(t, htmlDoc.Find("#reference-issue-modal").Length())
			assert.Zero(t, htmlDoc.Find(".render-content[data-can-edit]").Length(), "task lists can't be ticked either")
		})
	}

	t.Run("web", func(t *testing.T) {
		for _, user := range []string{"user1", "user2"} {
			session := loginUser(t, user)
			csrf := GetUserCSRFToken(t, session)
			session.MakeRequest(t, NewRequestWithValues(t, "POST", fmt.Sprintf("/user2/repo1/comments/%d", comment.ID), map[string]string{
				"_csrf":           csrf,
				"content":         "edited",
				"content_version": strconv.Itoa(comment.ContentVersion),
			}), http.StatusForbidden)
			session.MakeRequest(t, NewRequestWithValues(t, "POST", fmt.Sprintf("/user2/repo1/comments/%d/delete", comment.ID), map[string]string{
				"_csrf": csrf,
			}), http.StatusForbidden)
			session.MakeRequest(t, NewRequestWithValues(t, "POST", pullURL+"/content", map[string]string{
				"_csrf":           csrf,
				"content":         "edited",
				"content_version": strconv.Itoa(pull.ContentVersion),
			}), http.StatusForbidden)
		}
	})

	t.Run("api", func(t *testing.T) {
		token := getUserToken(t, "user1", auth_model.AccessTokenScopeWriteIssue)
		commentURL := fmt.Sprintf("/api/v1/repos/user2/repo1/issues/comments/%d", comment.ID)
		MakeRequest(t, NewRequestWithJSON(t, "PATCH", commentURL, &api.EditIssueCommentOption{Body: "edited"}).AddTokenAuth(token), http.StatusForbidden)
		MakeRequest(t, NewRequest(t, "DELETE", commentURL).AddTokenAuth(token), http.StatusForbidden)
	})

	unittest.AssertExistsAndLoadBean(t, &issues_model.Comment{ID: comment.ID, Content: "a change request comment"})
	unittest.AssertExistsAndLoadBean(t, &issues_model.Issue{ID: pull.ID, Content: pull.Content})
}

// TestIssueContentNotLocked makes sure #146 stays scoped to change requests: a plain issue's
// comments can still be edited and deleted by their poster.
func TestIssueContentNotLocked(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	repo1 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	issue := unittest.AssertExistsAndLoadBean(t, &issues_model.Issue{RepoID: repo1.ID, Index: 1})
	require.False(t, issue.IsPull)
	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	comment, err := issue_service.CreateIssueComment(t.Context(), user2, repo1, issue, "an issue comment", nil)
	require.NoError(t, err)

	session := loginUser(t, "user2")
	csrf := GetUserCSRFToken(t, session)
	session.MakeRequest(t, NewRequestWithValues(t, "POST", fmt.Sprintf("/user2/repo1/comments/%d", comment.ID), map[string]string{
		"_csrf":           csrf,
		"content":         "edited",
		"content_version": strconv.Itoa(comment.ContentVersion),
	}), http.StatusOK)
	unittest.AssertExistsAndLoadBean(t, &issues_model.Comment{ID: comment.ID, Content: "edited"})
	session.MakeRequest(t, NewRequestWithValues(t, "POST", fmt.Sprintf("/user2/repo1/comments/%d/delete", comment.ID), map[string]string{
		"_csrf": csrf,
	}), http.StatusOK)
	unittest.AssertNotExistsBean(t, &issues_model.Comment{ID: comment.ID})
}
