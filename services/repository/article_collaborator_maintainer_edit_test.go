// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repository

import (
	"testing"

	issues_model "code.gitea.io/gitea/models/issues"
	access_model "code.gitea.io/gitea/models/perm/access"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestArticleCollaboratorsMaintainerEdit checks that accepted article maintainers count as
// writers for "allow edits from maintainers" (issues_model.CanMaintainerWriteToBranch), on
// both sides of a change request: as maintainers of the base article, and as the poster
// whose head-repo write the edits are delegated from. Pending invitees and removed
// maintainers must not count.
func TestArticleCollaboratorsMaintainerEdit(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	// repo10 (user12) is the upstream article, repo11 (user13) a fork of it
	upstream := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 10})
	fork := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 11})
	upstreamOwner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: upstream.OwnerID})
	forkOwner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: fork.OwnerID})
	upstreamMaintainer := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
	forkMaintainer := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 5})
	forkInvitee := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 8})

	// openChangeRequest opens a change request from the fork's headBranch into upstream with
	// edits from maintainers allowed
	openChangeRequest := func(t *testing.T, poster *user_model.User, headBranch string) {
		pr := &issues_model.PullRequest{
			Issue: &issues_model.Issue{
				RepoID:   upstream.ID,
				PosterID: poster.ID,
				Poster:   poster,
				Title:    "change request",
				IsPull:   true,
			},
			HeadRepoID:          fork.ID,
			BaseRepoID:          upstream.ID,
			HeadBranch:          headBranch,
			BaseBranch:          upstream.DefaultBranch,
			Type:                issues_model.PullRequestGitea,
			AllowMaintainerEdit: true,
		}
		require.NoError(t, issues_model.NewPullRequest(ctx, upstream, pr.Issue, nil, nil, pr))
	}
	canWriteForkBranch := func(t *testing.T, doer *user_model.User, headBranch string) bool {
		headPerm, err := access_model.GetUserRepoPermission(ctx, fork, doer)
		require.NoError(t, err)
		return issues_model.CanMaintainerWriteToBranch(ctx, headPerm, headBranch, doer)
	}

	t.Run("UpstreamMaintainerEditsChangeRequest", func(t *testing.T) {
		const branch = "cr-from-fork-owner"
		openChangeRequest(t, forkOwner, branch)
		assert.True(t, canWriteForkBranch(t, upstreamOwner, branch))

		require.NoError(t, InviteArticleCollaborator(ctx, upstreamOwner, upstream, upstreamMaintainer, repo_model.ArticleCollaboratorRoleMaintainer))
		assert.False(t, canWriteForkBranch(t, upstreamMaintainer, branch), "a pending invitee is not a maintainer yet")

		require.NoError(t, AcceptArticleCollaboratorInvite(ctx, upstreamMaintainer, upstream))
		assert.True(t, canWriteForkBranch(t, upstreamMaintainer, branch))

		require.NoError(t, RemoveArticleCollaborators(ctx, upstreamOwner, upstream, []int64{upstreamMaintainer.ID}))
		assert.False(t, canWriteForkBranch(t, upstreamMaintainer, branch), "a removed maintainer loses the delegated write")
	})

	t.Run("ForkMaintainerPostsChangeRequest", func(t *testing.T) {
		const branch = "cr-from-fork-maintainer"
		require.NoError(t, InviteArticleCollaborator(ctx, forkOwner, fork, forkMaintainer, repo_model.ArticleCollaboratorRoleMaintainer))
		require.NoError(t, AcceptArticleCollaboratorInvite(ctx, forkMaintainer, fork))

		openChangeRequest(t, forkMaintainer, branch)
		assert.True(t, canWriteForkBranch(t, upstreamOwner, branch), "the poster maintains the fork, so they can delegate their write")

		require.NoError(t, RemoveArticleCollaborators(ctx, forkOwner, fork, []int64{forkMaintainer.ID}))
		assert.False(t, canWriteForkBranch(t, upstreamOwner, branch), "the poster no longer maintains the fork, so nothing is delegated")
	})

	t.Run("ForkInviteePostsChangeRequest", func(t *testing.T) {
		const branch = "cr-from-fork-invitee"
		require.NoError(t, InviteArticleCollaborator(ctx, forkOwner, fork, forkInvitee, repo_model.ArticleCollaboratorRoleMaintainer))

		openChangeRequest(t, forkInvitee, branch)
		assert.False(t, canWriteForkBranch(t, upstreamOwner, branch), "a pending invitee has no write on the fork to delegate")
	})
}
