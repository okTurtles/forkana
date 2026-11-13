// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package moderation

import (
	"testing"
	"time"

	"code.gitea.io/gitea/models/db"
	"code.gitea.io/gitea/models/organization"
	access_model "code.gitea.io/gitea/models/perm/access"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unit"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/test"
	org_service "code.gitea.io/gitea/services/org"
	repo_service "code.gitea.io/gitea/services/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModerationAccess(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())
	defer test.MockVariableValue(&setting.Moderation.Enabled, true)()
	defer test.MockVariableValue(&setting.Moderation.Org, "access-moderation")()
	defer test.MockVariableValue(&setting.Moderation.Repo, "cases")()
	defer test.MockVariableValue(&setting.Moderation.Team, "moderators")()
	defer test.MockVariableValue(&setting.Service.AutoWatchNewRepos, true)()

	admin := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 1})
	outsider := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	moderator := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
	otherMember := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 5})

	_, err := EnsureSetup(t.Context(), SetupOptions{Owner: admin})
	require.NoError(t, err)
	org, err := organization.GetOrgByName(t.Context(), "access-moderation")
	require.NoError(t, err)
	team, err := organization.GetTeam(t.Context(), org.ID, "moderators")
	require.NoError(t, err)
	repo, err := repo_model.GetRepositoryByName(t.Context(), org.ID, "cases")
	require.NoError(t, err)
	require.True(t, repo.IsModerationCasesRepo())

	require.NoError(t, org_service.AddTeamMember(t.Context(), team, moderator))

	t.Run("JoiningTheTeamLeavesTheRepoUnwatched", func(t *testing.T) {
		// The team-join auto-watch runs in a goroutine: give it time to (not) happen.
		assert.Never(t, func() bool {
			return repo_model.IsWatching(t.Context(), moderator.ID, repo.ID)
		}, 300*time.Millisecond, 20*time.Millisecond)
	})

	t.Run("AddingTheRepoToATeamLeavesItUnwatched", func(t *testing.T) {
		other := &organization.Team{
			OrgID: org.ID, Name: "others", AccessMode: 1,
			Units: []*organization.TeamUnit{{OrgID: org.ID, Type: unit.TypeIssues, AccessMode: 1}},
		}
		require.NoError(t, org_service.NewTeam(t.Context(), other))
		require.NoError(t, org_service.AddTeamMember(t.Context(), other, otherMember))
		require.NoError(t, repo_service.TeamAddRepository(t.Context(), other, repo))
		assert.False(t, repo_model.IsWatching(t.Context(), otherMember.ID, repo.ID))
		require.NoError(t, repo_service.RemoveRepositoryFromTeam(t.Context(), other, repo.ID))
	})

	t.Run("IsModerator", func(t *testing.T) {
		for _, tc := range []struct {
			user *user_model.User
			want bool
		}{
			{nil, false},
			{admin, true},
			{moderator, true},
			{outsider, false},
			{otherMember, false},
		} {
			got, err := IsModerator(t.Context(), tc.user)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got, "user %v", tc.user)
		}

		defer test.MockVariableValue(&setting.Moderation.Enabled, false)()
		for _, u := range []*user_model.User{admin, moderator} {
			got, err := IsModerator(t.Context(), u)
			require.NoError(t, err)
			assert.False(t, got, "moderation off: nobody is a moderator")
		}
	})

	t.Run("RepositoryAccess", func(t *testing.T) {
		for _, tc := range []struct {
			user       *user_model.User
			readIssues bool
		}{
			{admin, true},
			{moderator, true},
			{outsider, false},
		} {
			p, err := access_model.GetUserRepoPermission(t.Context(), repo, tc.user)
			require.NoError(t, err)
			assert.Equal(t, tc.readIssues, p.CanRead(unit.TypeIssues), "user %s", tc.user.Name)
			assert.False(t, p.CanRead(unit.TypeCode) && !tc.user.IsAdmin, "no code unit for %s", tc.user.Name)
		}
		p, err := access_model.GetUserRepoPermission(t.Context(), repo, moderator)
		require.NoError(t, err)
		assert.True(t, p.CanWrite(unit.TypeIssues))
	})

	t.Run("HiddenFromListings", func(t *testing.T) {
		search := func(include bool) []int64 {
			repos, _, err := repo_model.SearchRepository(t.Context(), repo_model.SearchRepoOptions{
				ListOptions:                db.ListOptionsAll,
				Actor:                      admin,
				Private:                    true,
				IncludeModerationCasesRepo: include,
			})
			require.NoError(t, err)
			ids := make([]int64, len(repos))
			for i, r := range repos {
				ids[i] = r.ID
			}
			return ids
		}
		assert.NotContains(t, search(false), repo.ID)
		assert.Contains(t, search(true), repo.ID)

		repos, _, err := repo_model.SearchRepository(t.Context(), repo_model.SearchRepoOptions{
			ListOptions: db.ListOptionsAll,
			Actor:       moderator,
			Private:     true,
			OwnerID:     org.ID,
		})
		require.NoError(t, err)
		assert.Empty(t, repos, "not on the organization's page either")
	})
}
