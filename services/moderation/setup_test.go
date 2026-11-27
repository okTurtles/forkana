// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package moderation

import (
	"testing"

	"code.gitea.io/gitea/models/db"
	issues_model "code.gitea.io/gitea/models/issues"
	moderation_model "code.gitea.io/gitea/models/moderation"
	"code.gitea.io/gitea/models/organization"
	"code.gitea.io/gitea/models/perm"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unit"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/structs"
	"code.gitea.io/gitea/modules/test"
	org_service "code.gitea.io/gitea/services/org"
	repo_service "code.gitea.io/gitea/services/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	unittest.MainTest(m)
}

func countCreated(r *SetupResult) int {
	n := 0
	for _, it := range r.Items {
		if it.Created {
			n++
		}
	}
	return n
}

func TestEnsureSetup(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())
	defer test.MockVariableValue(&setting.Moderation.Org, "test-moderation")()
	defer test.MockVariableValue(&setting.Moderation.Repo, "cases")()
	defer test.MockVariableValue(&setting.Moderation.Team, "moderators")()
	defer test.MockVariableValue(&setting.Service.AutoWatchNewRepos, true)()

	admin := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 1})
	require.True(t, admin.IsAdmin)
	nLabels := len(moderation_model.CoreLabels())
	// organization, team, repository, repository units and team repository, then the labels
	const nNonLabels = 5

	t.Run("NeedsAnAdminOwner", func(t *testing.T) {
		_, err := EnsureSetup(t.Context(), SetupOptions{})
		require.Error(t, err)

		user := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
		require.False(t, user.IsAdmin)
		_, err = EnsureSetup(t.Context(), SetupOptions{Owner: user})
		require.Error(t, err)
		unittest.AssertNotExistsBean(t, &user_model.User{LowerName: "test-moderation"})
	})

	t.Run("DryRunWritesNothing", func(t *testing.T) {
		for _, owner := range []*user_model.User{admin, nil} {
			result, err := EnsureSetup(t.Context(), SetupOptions{Owner: owner, DryRun: true})
			require.NoError(t, err, "a dry run needs no owner")
			assert.Equal(t, nNonLabels+nLabels, countCreated(result))
			unittest.AssertNotExistsBean(t, &user_model.User{LowerName: "test-moderation"})
		}
	})

	t.Run("CreatesEverything", func(t *testing.T) {
		result, err := EnsureSetup(t.Context(), SetupOptions{Owner: admin})
		require.NoError(t, err)
		assert.Equal(t, nNonLabels+nLabels, countCreated(result))
		assert.Zero(t, result.Unwatched, "the creator doesn't auto-watch the cases repository")

		org, err := organization.GetOrgByName(t.Context(), "test-moderation")
		require.NoError(t, err)
		assert.Equal(t, structs.VisibleTypePrivate, org.Visibility)

		team, err := organization.GetTeam(t.Context(), org.ID, "moderators")
		require.NoError(t, err)
		assert.False(t, team.IncludesAllRepositories)
		require.NoError(t, team.LoadUnits(t.Context()))
		assert.Equal(t, perm.AccessModeWrite, team.UnitAccessMode(t.Context(), unit.TypeIssues))
		assert.Equal(t, perm.AccessModeNone, team.UnitAccessMode(t.Context(), unit.TypeCode))

		repo, err := repo_model.GetRepositoryByName(t.Context(), org.ID, "cases")
		require.NoError(t, err)
		assert.True(t, repo.IsPrivate)
		assert.Zero(t, repo.SubjectID)
		require.NoError(t, repo.LoadUnits(t.Context()))
		require.Len(t, repo.Units, 1)
		assert.Equal(t, unit.TypeIssues, repo.Units[0].Type)
		assert.True(t, repo.Units[0].IssuesConfig().EnableDependencies)
		assert.True(t, repo_service.HasRepository(t.Context(), team, repo.ID))
		assert.False(t, repo_model.IsWatching(t.Context(), admin.ID, repo.ID))

		labels, err := issues_model.GetLabelsByOrgID(t.Context(), org.ID, "", db.ListOptionsAll)
		require.NoError(t, err)
		assert.Len(t, labels, nLabels)
		byName := map[string]*issues_model.Label{}
		for _, l := range labels {
			byName[l.Name] = l
		}
		assert.True(t, byName[moderation_model.LabelTypeCase].Exclusive)
		assert.Equal(t, "type", byName[moderation_model.LabelTypeCase].ExclusiveScope())
		assert.True(t, byName[moderation_model.ReasonDoxxing.LabelName()].Exclusive)
		assert.Equal(t, "reason", byName[moderation_model.ReasonDoxxing.LabelName()].ExclusiveScope())
	})

	t.Run("SecondRunCreatesNothing", func(t *testing.T) {
		org, err := organization.GetOrgByName(t.Context(), "test-moderation")
		require.NoError(t, err)
		repo, err := repo_model.GetRepositoryByName(t.Context(), org.ID, "cases")
		require.NoError(t, err)

		// A label deleted by hand comes back; a watch added since is removed again.
		label, err := issues_model.GetLabelInOrgByName(t.Context(), org.ID, moderation_model.LabelOutcomeUpheld)
		require.NoError(t, err)
		require.NoError(t, issues_model.DeleteLabel(t.Context(), org.ID, label.ID))
		require.NoError(t, repo_model.WatchRepo(t.Context(), admin, repo, true))

		result, err := EnsureSetup(t.Context(), SetupOptions{Owner: admin})
		require.NoError(t, err)
		require.Equal(t, 1, countCreated(result))
		assert.Equal(t, 1, result.Unwatched)

		result, err = EnsureSetup(t.Context(), SetupOptions{})
		require.NoError(t, err, "nothing to create: no owner needed")
		assert.Zero(t, countCreated(result))
		assert.Zero(t, result.Unwatched)
	})

	t.Run("RepairsTheRepositoryUnits", func(t *testing.T) {
		// As a run that failed after creating the repository leaves it: the default units.
		org, err := organization.GetOrgByName(t.Context(), "test-moderation")
		require.NoError(t, err)
		repo, err := repo_model.GetRepositoryByName(t.Context(), org.ID, "cases")
		require.NoError(t, err)
		require.NoError(t, repo_service.UpdateRepositoryUnits(t.Context(), repo, []repo_model.RepoUnit{
			{RepoID: repo.ID, Type: unit.TypeIssues, Config: &repo_model.IssuesConfig{}},
			{RepoID: repo.ID, Type: unit.TypeWiki, Config: new(repo_model.UnitConfig)},
		}, nil))

		result, err := EnsureSetup(t.Context(), SetupOptions{})
		require.NoError(t, err)
		assert.Equal(t, 1, countCreated(result))
		repo, err = repo_model.GetRepositoryByName(t.Context(), org.ID, "cases")
		require.NoError(t, err)
		require.NoError(t, repo.LoadUnits(t.Context()))
		require.Len(t, repo.Units, 1)
		assert.True(t, repo.Units[0].IssuesConfig().EnableDependencies)
	})

	t.Run("RefusesOwnersWhoArentAdmins", func(t *testing.T) {
		org, err := organization.GetOrgByName(t.Context(), "test-moderation")
		require.NoError(t, err)
		owners, err := org.GetOwnerTeam(t.Context())
		require.NoError(t, err)
		user := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
		require.False(t, user.IsAdmin)
		require.NoError(t, org_service.AddTeamMember(t.Context(), owners, user))

		for _, dryRun := range []bool{false, true} {
			_, err = EnsureSetup(t.Context(), SetupOptions{DryRun: dryRun})
			require.ErrorContains(t, err, "aren't site administrators (user2)")
		}

		require.NoError(t, org_service.RemoveTeamMember(t.Context(), owners, user))
		_, err = EnsureSetup(t.Context(), SetupOptions{})
		require.NoError(t, err)
	})

	t.Run("RefusesAPublicRepository", func(t *testing.T) {
		org, err := organization.GetOrgByName(t.Context(), "test-moderation")
		require.NoError(t, err)
		repo, err := repo_model.GetRepositoryByName(t.Context(), org.ID, "cases")
		require.NoError(t, err)
		repo.IsPrivate = false
		require.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(t.Context(), repo, "is_private"))

		_, err = EnsureSetup(t.Context(), SetupOptions{})
		require.ErrorContains(t, err, "public")
	})
}
