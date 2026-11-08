// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repo_test

import (
	"testing"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func isFollowingSubject(t *testing.T, userID, subjectID int64) bool {
	t.Helper()
	following, err := repo_model.IsFollowingSubject(t.Context(), userID, subjectID)
	require.NoError(t, err)
	return following
}

func TestFollowSubject(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	assert.False(t, isFollowingSubject(t, 5, 1))

	// following twice keeps one follow
	require.NoError(t, repo_model.FollowSubject(ctx, 5, 1, true))
	require.NoError(t, repo_model.FollowSubject(ctx, 5, 1, true))
	assert.True(t, isFollowingSubject(t, 5, 1))
	assert.Equal(t, 1, unittest.GetCount(t, &repo_model.SubjectFollow{UserID: 5, SubjectID: 1}))
	assert.False(t, isFollowingSubject(t, 5, 2), "only the subject followed")
	assert.False(t, isFollowingSubject(t, 4, 1), "only by the user who follows it")
	require.NoError(t, repo_model.FollowSubject(ctx, 4, 1, true))
	assert.Equal(t, 2, unittest.GetCount(t, &repo_model.SubjectFollow{SubjectID: 1}))

	// unfollowing, twice, leaves the other follower
	require.NoError(t, repo_model.FollowSubject(ctx, 5, 1, false))
	require.NoError(t, repo_model.FollowSubject(ctx, 5, 1, false))
	assert.False(t, isFollowingSubject(t, 5, 1))
	assert.True(t, isFollowingSubject(t, 4, 1))

	// no user or no subject follows nothing
	assert.False(t, isFollowingSubject(t, 0, 1))
	assert.False(t, isFollowingSubject(t, 4, 0))
}

// FindFollowedSubjectReadableRepoIDs: the articles of the followed subjects, only those the
// user can read.
func TestFindFollowedSubjectReadableRepoIDs(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()
	user5 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 5})

	ids, err := repo_model.FindFollowedSubjectReadableRepoIDs(ctx, user5, 50)
	require.NoError(t, err)
	assert.Empty(t, ids)

	require.NoError(t, repo_model.FollowSubject(ctx, user5.ID, 1, true))
	ids, err = repo_model.FindFollowedSubjectReadableRepoIDs(ctx, user5, 50)
	require.NoError(t, err)
	assert.Equal(t, []int64{1}, ids)

	// user2's private repo2 joins the subject: user5 cannot read it
	repo2 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 2})
	require.True(t, repo2.IsPrivate)
	repo2.SubjectID = 1
	require.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(ctx, repo2, "subject_id"))
	ids, err = repo_model.FindFollowedSubjectReadableRepoIDs(ctx, user5, 50)
	require.NoError(t, err)
	assert.Equal(t, []int64{1}, ids)
}

// GetSubjectByID with no id finds nothing (xorm ignores a zero ID, and would find any
// subject).
func TestGetSubjectByIDZero(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())
	for _, id := range []int64{0, -1} {
		subject, err := repo_model.GetSubjectByID(t.Context(), id)
		assert.True(t, repo_model.IsErrSubjectNotExist(err), "id %d", id)
		assert.Nil(t, subject)
	}
}

// A deleted subject takes its follows with it, so that a later subject is not followed by
// its readers.
func TestDeleteSubjectDeletesItsFollows(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	subject, err := repo_model.GetOrCreateSubject(ctx, "A subject nobody writes about")
	require.NoError(t, err)
	require.NoError(t, repo_model.FollowSubject(ctx, 5, subject.ID, true))
	deleted, err := repo_model.DeleteSubjectIfOrphaned(ctx, subject.ID)
	require.NoError(t, err)
	require.True(t, deleted)
	unittest.AssertNotExistsBean(t, &repo_model.SubjectFollow{SubjectID: subject.ID})

	subject, err = repo_model.GetOrCreateSubject(ctx, "Another subject nobody writes about")
	require.NoError(t, err)
	require.NoError(t, repo_model.FollowSubject(ctx, 5, subject.ID, true))
	require.NoError(t, repo_model.DeleteSubject(ctx, subject.ID))
	unittest.AssertNotExistsBean(t, &repo_model.SubjectFollow{SubjectID: subject.ID})
}
