// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repo_test

import (
	"testing"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFollowSubject(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	assert.False(t, repo_model.IsFollowingSubject(ctx, 5, 1))
	count, err := repo_model.CountSubjectFollowers(ctx, 1)
	require.NoError(t, err)
	assert.Zero(t, count)

	// following twice keeps one follow
	require.NoError(t, repo_model.FollowSubject(ctx, 5, 1, true))
	require.NoError(t, repo_model.FollowSubject(ctx, 5, 1, true))
	assert.True(t, repo_model.IsFollowingSubject(ctx, 5, 1))
	assert.False(t, repo_model.IsFollowingSubject(ctx, 5, 2), "only the subject followed")
	assert.False(t, repo_model.IsFollowingSubject(ctx, 4, 1), "only by the user who follows it")
	require.NoError(t, repo_model.FollowSubject(ctx, 4, 1, true))
	count, err = repo_model.CountSubjectFollowers(ctx, 1)
	require.NoError(t, err)
	assert.EqualValues(t, 2, count)

	// unfollowing, twice, leaves the other follower
	require.NoError(t, repo_model.FollowSubject(ctx, 5, 1, false))
	require.NoError(t, repo_model.FollowSubject(ctx, 5, 1, false))
	assert.False(t, repo_model.IsFollowingSubject(ctx, 5, 1))
	assert.True(t, repo_model.IsFollowingSubject(ctx, 4, 1))

	// no user or no subject follows nothing
	assert.False(t, repo_model.IsFollowingSubject(ctx, 0, 1))
	assert.False(t, repo_model.IsFollowingSubject(ctx, 4, 0))
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
