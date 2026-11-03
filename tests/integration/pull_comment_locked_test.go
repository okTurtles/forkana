// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"testing"

	auth_model "code.gitea.io/gitea/models/auth"
	"code.gitea.io/gitea/models/db"
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

		// the description can't be rewritten through the pull or the issue route either, and a
		// rejected request doesn't change the other fields it carries
		token = getUserToken(t, "user1", auth_model.AccessTokenScopeWriteIssue, auth_model.AccessTokenScopeWriteRepository)
		body := "edited"
		MakeRequest(t, NewRequestWithJSON(t, "PATCH", fmt.Sprintf("/api/v1/repos/user2/repo1/pulls/%d", pull.Index),
			&api.EditPullRequestOption{Title: "edited title", Body: &body}).AddTokenAuth(token), http.StatusForbidden)
		MakeRequest(t, NewRequestWithJSON(t, "PATCH", fmt.Sprintf("/api/v1/repos/user2/repo1/issues/%d", pull.Index),
			&api.EditIssueOption{Title: "edited title", Body: &body}).AddTokenAuth(token), http.StatusForbidden)
	})

	unittest.AssertExistsAndLoadBean(t, &issues_model.Comment{ID: comment.ID, Content: "a change request comment"})
	unittest.AssertExistsAndLoadBean(t, &issues_model.Issue{ID: pull.ID, Title: pull.Title, Content: pull.Content})
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

// TestChangeRequestAssetsLocked covers the API asset routes of #146: adding, renaming or
// removing a file of a change request's description or comment edits that content, and the
// create routes would even rewrite it to record the association, so they are refused too.
func TestChangeRequestAssetsLocked(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	repo1 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	pull := unittest.AssertExistsAndLoadBean(t, &issues_model.Issue{RepoID: repo1.ID, Index: 3})
	require.True(t, pull.IsPull)
	user1 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 1}) // site admin, poster of the change request
	comment, err := issue_service.CreateIssueComment(t.Context(), user1, repo1, pull, "a change request comment", nil)
	require.NoError(t, err)

	issueAsset := &repo_model.Attachment{UUID: "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380146", RepoID: repo1.ID, IssueID: pull.ID, UploaderID: user1.ID, Name: "description.png"}
	commentAsset := &repo_model.Attachment{UUID: "a0eebc99-9c0b-4ef8-bb6d-6bb9bd381146", RepoID: repo1.ID, IssueID: pull.ID, CommentID: comment.ID, UploaderID: user1.ID, Name: "comment.png"}
	require.NoError(t, db.Insert(t.Context(), issueAsset))
	require.NoError(t, db.Insert(t.Context(), commentAsset))
	assetCount := unittest.GetCount(t, &repo_model.Attachment{IssueID: pull.ID})

	token := getUserToken(t, "user1", auth_model.AccessTokenScopeWriteIssue)
	upload := func(url string) *RequestWrapper {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		part, err := writer.CreateFormFile("attachment", "image.png")
		require.NoError(t, err)
		buff := generateImg()
		_, err = io.Copy(part, &buff)
		require.NoError(t, err)
		require.NoError(t, writer.Close())
		req := NewRequestWithBody(t, "POST", url, body).AddTokenAuth(token)
		req.Header.Add("Content-Type", writer.FormDataContentType())
		return req
	}

	for _, c := range []struct {
		name   string
		assets string
		asset  *repo_model.Attachment
	}{
		{"description", fmt.Sprintf("/api/v1/repos/user2/repo1/issues/%d/assets", pull.Index), issueAsset},
		{"comment", fmt.Sprintf("/api/v1/repos/user2/repo1/issues/comments/%d/assets", comment.ID), commentAsset},
	} {
		t.Run(c.name, func(t *testing.T) {
			assetURL := fmt.Sprintf("%s/%d", c.assets, c.asset.ID)
			MakeRequest(t, upload(c.assets), http.StatusForbidden)
			MakeRequest(t, NewRequestWithJSON(t, "PATCH", assetURL, &api.EditAttachmentOptions{Name: "renamed.png"}).AddTokenAuth(token), http.StatusForbidden)
			MakeRequest(t, NewRequest(t, "DELETE", assetURL).AddTokenAuth(token), http.StatusForbidden)
			unittest.AssertExistsAndLoadBean(t, &repo_model.Attachment{ID: c.asset.ID, Name: c.asset.Name})
		})
	}

	// nothing was added, and neither the description nor the comment was touched
	assert.Equal(t, assetCount, unittest.GetCount(t, &repo_model.Attachment{IssueID: pull.ID}))
	assert.Equal(t, pull.ContentVersion, unittest.AssertExistsAndLoadBean(t, &issues_model.Issue{ID: pull.ID}).ContentVersion)
	assert.Equal(t, comment.ContentVersion, unittest.AssertExistsAndLoadBean(t, &issues_model.Comment{ID: comment.ID}).ContentVersion)
	unittest.AssertNotExistsBean(t, &issues_model.ContentHistory{IssueID: pull.ID})
}
