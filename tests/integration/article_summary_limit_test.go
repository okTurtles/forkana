// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	auth_model "code.gitea.io/gitea/models/auth"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	api "code.gitea.io/gitea/modules/structs"
	"code.gitea.io/gitea/tests"

	"github.com/stretchr/testify/assert"
)

// TestArticleSummaryLimit: an article's summary (its repository description) is "Up to
// 200 characters" (#434, figma Textarea I6961:75226), in the web forms and the API alike.
func TestArticleSummaryLimit(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	user2, repo1, subjectName := loadArticleRepo(t, 1)
	session := loginUser(t, user2.Name)
	token := getTokenForLoggedInUser(t, session, auth_model.AccessTokenScopeWriteRepository, auth_model.AccessTokenScopeWriteUser)
	longest := strings.Repeat("a", 200)
	tooLong := strings.Repeat("b", 201)

	t.Run("APIEdit", func(t *testing.T) {
		url := fmt.Sprintf("/api/v1/repos/%s/%s", user2.Name, repo1.Name)
		MakeRequest(t, NewRequestWithJSON(t, "PATCH", url, &api.EditRepoOption{Description: &tooLong}).AddTokenAuth(token), http.StatusUnprocessableEntity)
		MakeRequest(t, NewRequestWithJSON(t, "PATCH", url, &api.EditRepoOption{Description: &longest}).AddTokenAuth(token), http.StatusOK)
		assert.Equal(t, longest, unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1}).Description)
	})

	t.Run("APICreate", func(t *testing.T) {
		MakeRequest(t, NewRequestWithJSON(t, "POST", "/api/v1/user/repos", &api.CreateRepoOption{
			Name: "summary-too-long", Description: tooLong,
		}).AddTokenAuth(token), http.StatusUnprocessableEntity)
		unittest.AssertNotExistsBean(t, &repo_model.Repository{OwnerID: user2.ID, LowerName: "summary-too-long"})
	})

	t.Run("WebSettings", func(t *testing.T) {
		before := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1}).Description
		req := NewRequestWithValues(t, "POST", fmt.Sprintf("/%s/%s/settings", user2.Name, repo1.Name), map[string]string{
			"_csrf":       GetUserCSRFToken(t, session),
			"action":      "update",
			"repo_name":   repo1.Name,
			"subject":     subjectName,
			"description": tooLong,
		})
		session.MakeRequest(t, req, NoExpectedStatus)
		assert.Equal(t, before, unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1}).Description,
			"a summary over 200 characters is refused")

		// while the same form with 200 characters goes through
		exact := strings.Repeat("c", 200)
		req = NewRequestWithValues(t, "POST", fmt.Sprintf("/%s/%s/settings", user2.Name, repo1.Name), map[string]string{
			"_csrf":       GetUserCSRFToken(t, session),
			"action":      "update",
			"repo_name":   repo1.Name,
			"subject":     subjectName,
			"description": exact,
		})
		session.MakeRequest(t, req, NoExpectedStatus)
		assert.Equal(t, exact, unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1}).Description)
	})
}
