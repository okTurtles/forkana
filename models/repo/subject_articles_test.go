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

func TestPublicArticlesBySubject(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	subject, err := repo_model.GetOrCreateSubject(ctx, "Apollo 11")
	require.NoError(t, err)
	other, err := repo_model.GetOrCreateSubject(ctx, "Apollo 17")
	require.NoError(t, err)

	assign := func(repoID, subjectID int64, cols ...func(*repo_model.Repository) string) {
		repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: repoID})
		repo.SubjectID = subjectID
		var names []string
		for _, set := range cols {
			names = append(names, set(repo))
		}
		require.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(ctx, repo, "subject_id", names...))
	}
	assign(1, subject.ID)                                          // user2, public
	assign(4, subject.ID)                                          // user5, public
	assign(2, subject.ID)                                          // user2, private: not counted
	assign(10, subject.ID, func(r *repo_model.Repository) string { // user12, deleted
		r.IsTombstoned = true
		return "is_tombstoned"
	})
	assign(11, other.ID) // user13, public

	counts, err := repo_model.CountPublicArticlesBySubjectIDs(ctx, []int64{subject.ID, other.ID, 9999})
	require.NoError(t, err)
	assert.Equal(t, map[int64]int64{subject.ID: 2, other.ID: 1}, counts)

	articles, err := repo_model.FindPublicArticlesBySubjectID(ctx, subject.ID)
	require.NoError(t, err)
	owners := make([]string, 0, len(articles))
	for _, a := range articles {
		owners = append(owners, a.OwnerName)
	}
	assert.ElementsMatch(t, []string{"user2", "user5"}, owners)

	counts, err = repo_model.CountPublicArticlesBySubjectIDs(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, counts)
}
