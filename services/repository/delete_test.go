// Copyright 2017 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repository_test

import (
	"testing"

	"code.gitea.io/gitea/models/organization"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	repo_service "code.gitea.io/gitea/services/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTeam_HasRepository(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	test := func(teamID, repoID int64, expected bool) {
		team := unittest.AssertExistsAndLoadBean(t, &organization.Team{ID: teamID})
		assert.Equal(t, expected, repo_service.HasRepository(t.Context(), team, repoID))
	}
	test(1, 1, false)
	test(1, 3, true)
	test(1, 5, true)
	test(1, unittest.NonexistentID, false)

	test(2, 3, true)
	test(2, 5, false)
}

func TestTeam_RemoveRepository(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	testSuccess := func(teamID, repoID int64) {
		team := unittest.AssertExistsAndLoadBean(t, &organization.Team{ID: teamID})
		assert.NoError(t, repo_service.RemoveRepositoryFromTeam(t.Context(), team, repoID))
		unittest.AssertNotExistsBean(t, &organization.TeamRepo{TeamID: teamID, RepoID: repoID})
		unittest.CheckConsistencyFor(t, &organization.Team{ID: teamID}, &repo_model.Repository{ID: repoID})
	}
	testSuccess(2, 3)
	testSuccess(2, 5)
	testSuccess(1, unittest.NonexistentID)
}

func TestDeleteOwnerRepositoriesDirectly(t *testing.T) {
	unittest.PrepareTestEnv(t)

	user := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})

	assert.NoError(t, repo_service.DeleteOwnerRepositoriesDirectly(t.Context(), user))
}

func TestDeleteRepositoryDirectlyCleansUpSubject(t *testing.T) {
	unittest.PrepareTestEnv(t)

	t.Run("LastArticleDropsSubject", func(t *testing.T) {
		repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 2})
		subject, err := repo_model.GetOrCreateSubject(t.Context(), "Delete Subject Sole Article")
		require.NoError(t, err)
		repo.SubjectID = subject.ID
		require.NoError(t, repo_model.UpdateRepositoryColsWithAutoTime(t.Context(), repo, "subject_id"))

		require.NoError(t, repo_service.DeleteRepositoryDirectly(t.Context(), repo.ID))

		_, err = repo_model.GetSubjectByID(t.Context(), subject.ID)
		assert.True(t, repo_model.IsErrSubjectNotExist(err))
	})

	t.Run("RemainingArticleKeepsSubject", func(t *testing.T) {
		subject, err := repo_model.GetOrCreateSubject(t.Context(), "Delete Subject Shared")
		require.NoError(t, err)
		for _, id := range []int64{3, 4} {
			repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: id})
			repo.SubjectID = subject.ID
			require.NoError(t, repo_model.UpdateRepositoryColsWithAutoTime(t.Context(), repo, "subject_id"))
		}

		require.NoError(t, repo_service.DeleteRepositoryDirectly(t.Context(), 3))

		_, err = repo_model.GetSubjectByID(t.Context(), subject.ID)
		assert.NoError(t, err)
	})
}
