// Copyright 2026 The Forkana Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	auth_model "code.gitea.io/gitea/models/auth"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/gitrepo"
	api "code.gitea.io/gitea/modules/structs"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A file created through the API without "dates" is committed at the time of the request
// (#478). The defaults used to be set after the options were copied, so the commit got
// Go's zero time, which git reads as 2001-01-01T00:00:00Z: every article imported by the
// first article-creator was dated "26 years ago" in its History tab.
func TestAPICreateFileWithoutDates(t *testing.T) {
	onGiteaRun(t, func(t *testing.T, u *url.URL) {
		user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
		repo1 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
		session := loginUser(t, user2.Name)
		token := getTokenForLoggedInUser(t, session, auth_model.AccessTokenScopeWriteRepository)

		before := time.Now().Add(-time.Minute)
		opts := api.CreateFileOptions{
			FileOptions:   api.FileOptions{Message: "No dates"},
			ContentBase64: base64.StdEncoding.EncodeToString([]byte("text")),
		}
		treePath := "no-dates/file.txt"
		req := NewRequestWithJSON(t, "POST", fmt.Sprintf("/api/v1/repos/%s/%s/contents/%s", user2.Name, repo1.Name, treePath), &opts).
			AddTokenAuth(token)
		MakeRequest(t, req, http.StatusCreated)

		gitRepo, err := gitrepo.OpenRepository(t.Context(), repo1)
		require.NoError(t, err)
		defer gitRepo.Close()
		commit, err := gitRepo.GetCommitByPath(treePath)
		require.NoError(t, err)
		assert.True(t, commit.Author.When.After(before), "author date %v", commit.Author.When)
		assert.True(t, commit.Committer.When.After(before), "committer date %v", commit.Committer.When)
	})
}
