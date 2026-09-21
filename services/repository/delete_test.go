// Copyright 2017 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repository_test

import (
	"testing"

	"code.gitea.io/gitea/models/db"
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

	// repo 21 has a pending transfer in the fixtures and is not deleted by the
	// sibling subtests, which share this test's database state.
	t.Run("PendingTransferIsRemoved", func(t *testing.T) {
		transfer := unittest.AssertExistsAndLoadBean(t, &repo_model.RepoTransfer{RepoID: 21})

		require.NoError(t, repo_service.DeleteRepositoryDirectly(t.Context(), 21))

		unittest.AssertNotExistsBean(t, &repo_model.RepoTransfer{ID: transfer.ID})
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

func TestDeleteRepositoryDirectlyKeepsSharedAttachments(t *testing.T) {
	unittest.PrepareTestEnv(t)

	newAttachment := func(uuid string, purpose repo_model.AttachmentPurpose) *repo_model.Attachment {
		attach := &repo_model.Attachment{UUID: uuid, RepoID: 1, UploaderID: 2, Purpose: purpose, Name: "image.png"}
		require.NoError(t, db.Insert(t.Context(), attach))
		return attach
	}
	newLinkedAttachment := func(uuid string, issueID, releaseID int64) *repo_model.Attachment {
		attach := &repo_model.Attachment{UUID: uuid, RepoID: 1, UploaderID: 2, IssueID: issueID, ReleaseID: releaseID, Name: "image.png"}
		require.NoError(t, db.Insert(t.Context(), attach))
		return attach
	}

	// an article upload outlives the repository it was uploaded to: the associations
	// govern its lifetime, and the garbage collector reclaims it once they are gone
	article := newAttachment("5c1a7e40-0000-4000-8000-00000000f001", repo_model.AttachmentPurposeArticle)
	// a legacy row another repository still keeps alive
	shared := newAttachment("5c1a7e40-0000-4000-8000-00000000f002", repo_model.AttachmentPurposeUnspecified)
	require.NoError(t, repo_model.AddArticleAttachments(t.Context(), 2, []int64{shared.ID}))
	// nobody else's
	sole := newAttachment("5c1a7e40-0000-4000-8000-00000000f003", repo_model.AttachmentPurposeUnspecified)
	// an attachment linked to an issue or a release belongs to that unit alone, so its
	// lifetime is unchanged by the association work: it goes with the repository
	issueLinked := newLinkedAttachment("5c1a7e40-0000-4000-8000-00000000f004", 1, 0)
	releaseLinked := newLinkedAttachment("5c1a7e40-0000-4000-8000-00000000f005", 0, 1)

	require.NoError(t, repo_service.DeleteRepositoryDirectly(t.Context(), 1))

	unittest.AssertExistsAndLoadBean(t, &repo_model.Attachment{ID: article.ID})
	unittest.AssertExistsAndLoadBean(t, &repo_model.Attachment{ID: shared.ID})
	unittest.AssertNotExistsBean(t, &repo_model.Attachment{ID: sole.ID})
	unittest.AssertNotExistsBean(t, &repo_model.Attachment{ID: issueLinked.ID})
	unittest.AssertNotExistsBean(t, &repo_model.Attachment{ID: releaseLinked.ID})

	// the deleted repository keeps none of its own associations
	unittest.AssertNotExistsBean(t, &repo_model.ArticleAttachment{RepoID: 1})
}
