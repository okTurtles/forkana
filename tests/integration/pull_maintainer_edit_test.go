// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	auth_model "code.gitea.io/gitea/models/auth"
	issues_model "code.gitea.io/gitea/models/issues"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	api "code.gitea.io/gitea/modules/structs"

	"github.com/stretchr/testify/require"
)

// TestPullMaintainerEdit checks that "allow maintainer edits" only grants a base-repo
// maintainer write access to the PR's head branch when the PR's poster can write to the
// head repo themselves (backport of go-gitea/gitea#37479).
//
// The PR rows are created through the model layer: the web compare form stores
// allow_maintainer_edit without checking the poster's head-repo permission, so this is
// state a user can produce. The writes go through the API contents endpoint, which is
// gated only by CanMaintainerWriteToBranch. It needs a running server because the commit is
// pushed through the pre-receive hook, which calls back into the internal API.
func TestPullMaintainerEdit(t *testing.T) {
	onGiteaRun(t, testPullMaintainerEdit)
}

func testPullMaintainerEdit(t *testing.T, _ *url.URL) {
	// Fixtures: user12 owns repo10; user13 owns repo11, a fork of repo10.
	upstream := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 10})
	fork := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 11})
	upstreamOwner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: upstream.OwnerID})
	forkOwner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: fork.OwnerID})

	createAllowEditsPR := func(t *testing.T, poster *user_model.User, baseRepo, headRepo *repo_model.Repository) {
		// IsPull must be set: a push to the head branch notifies the PR's watchers, and the
		// mail notifier dereferences Issue.PullRequest, which only loads for pull issues. A
		// panic there shuts the graceful manager down and fails every later test.
		pr := &issues_model.PullRequest{
			Issue: &issues_model.Issue{
				RepoID:   baseRepo.ID,
				PosterID: poster.ID,
				Poster:   poster,
				Title:    "maintainer edit probe",
				IsPull:   true,
			},
			HeadRepoID:          headRepo.ID,
			BaseRepoID:          baseRepo.ID,
			HeadBranch:          headRepo.DefaultBranch,
			BaseBranch:          baseRepo.DefaultBranch,
			Type:                issues_model.PullRequestGitea,
			AllowMaintainerEdit: true,
		}
		require.NoError(t, issues_model.NewPullRequest(t.Context(), baseRepo, pr.Issue, nil, nil, pr))
	}

	createFileAs := func(t *testing.T, doer *user_model.User, repo *repo_model.Repository, treePath string, expectedStatus int) {
		token := getTokenForLoggedInUser(t, loginUser(t, doer.Name), auth_model.AccessTokenScopeWriteRepository)
		opts := api.CreateFileOptions{
			FileOptions:   api.FileOptions{BranchName: repo.DefaultBranch, Message: "maintainer edit probe"},
			ContentBase64: base64.StdEncoding.EncodeToString([]byte("maintainer edit probe\n")),
		}
		req := NewRequestWithJSON(t, "POST", fmt.Sprintf("/api/v1/repos/%s/%s/contents/%s", repo.OwnerName, repo.Name, treePath), &opts).
			AddTokenAuth(token)
		MakeRequest(t, req, expectedStatus)
	}

	t.Run("PosterCanWriteHead", func(t *testing.T) {
		// The fork owner opens a PR into upstream and allows edits: the upstream owner may push to the fork's branch.
		createAllowEditsPR(t, forkOwner, upstream, fork)
		createFileAs(t, upstreamOwner, fork, "maintainer-edit-allowed.txt", http.StatusCreated)
	})

	t.Run("PosterCannotWriteHead", func(t *testing.T) {
		// The fork owner opens a PR into their own fork with upstream's branch as head and allows edits.
		// As a maintainer of the base (their fork) they must not gain write access to upstream's branch.
		createAllowEditsPR(t, forkOwner, fork, upstream)
		createFileAs(t, forkOwner, upstream, "maintainer-edit-denied.txt", http.StatusForbidden)
	})
}
