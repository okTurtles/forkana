// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"net/http"
	"testing"

	activities_model "code.gitea.io/gitea/models/activities"
	"code.gitea.io/gitea/models/db"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	"code.gitea.io/gitea/modules/json"
	"code.gitea.io/gitea/modules/repository"
	"code.gitea.io/gitea/tests"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFeedCommitLinks covers the commit links of a push in the activity feeds: each one
// selects its version on the article the push went to.
func TestFeedCommitLinks(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	const sha = "65f1bf27bc3bf70f64657658635e66094edbcb4d"
	content, err := json.Marshal(&repository.PushCommits{
		Commits: []*repository.PushCommit{{Sha1: sha, Message: "feed commit"}},
		Len:     1,
	})
	require.NoError(t, err)
	// pushed by someone other than the owner, whose own article updates are not listed
	require.NoError(t, db.Insert(t.Context(), &activities_model.Action{
		UserID:    4,
		ActUserID: 4,
		OpType:    activities_model.ActionCommitRepo,
		RepoID:    repo.ID,
		RefName:   "refs/heads/master",
		Content:   string(content),
	}))

	session := loginUser(t, "user4")
	for _, page := range []string{"/feeds", "/user4?tab=activity"} {
		t.Run(page, func(t *testing.T) {
			resp := session.MakeRequest(t, NewRequest(t, "GET", page), http.StatusOK)
			link := NewHTMLParser(t, resp.Body).Find("a.ui.sha.label")
			require.Equal(t, 1, link.Length())
			assert.Equal(t, repo.Link()+"?version="+sha, link.AttrOr("href", ""))
		})
	}
}
