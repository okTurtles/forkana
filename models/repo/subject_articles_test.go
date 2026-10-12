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

func TestReadableArticlesBySubject(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	subject, err := repo_model.GetOrCreateSubject(ctx, "Apollo 11")
	require.NoError(t, err)
	other, err := repo_model.GetOrCreateSubject(ctx, "Apollo 17")
	require.NoError(t, err)

	assign := func(repoID, subjectID int64, set func(*repo_model.Repository) []string) {
		repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: repoID})
		repo.SubjectID = subjectID
		cols := []string{}
		if set != nil {
			cols = set(repo)
		}
		require.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(ctx, repo, "subject_id", cols...))
	}
	assign(1, subject.ID, nil)                                       // user2, public
	assign(4, subject.ID, nil)                                       // user5, public
	assign(2, subject.ID, nil)                                       // user2, private: not readable by others
	assign(10, subject.ID, func(r *repo_model.Repository) []string { // user12, deleted
		r.IsTombstoned = true
		return []string{"is_tombstoned"}
	})
	assign(11, subject.ID, func(r *repo_model.Repository) []string { // user13, empty
		r.IsEmpty = true
		return []string{"is_empty"}
	})
	assign(38, subject.ID, nil) // limited_org: only for signed-in users
	assign(40, subject.ID, nil) // privated_org: hidden
	assign(16, other.ID, nil)   // user2, private

	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})

	// signed out: user2 and user5
	counts, err := repo_model.CountReadableArticlesBySubjectIDs(ctx, []int64{subject.ID, other.ID, 9999}, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(2), counts[subject.ID])
	_, has := counts[9999]
	assert.False(t, has)

	owners, err := repo_model.FindReadableArticleOwners(ctx, subject.ID, nil)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"user2", "user5"}, owners)

	// signed in as user2: the limited org too, and user2 once although it has two articles
	owners, err = repo_model.FindReadableArticleOwners(ctx, subject.ID, user2)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"user2", "user5", "limited_org"}, owners)

	for _, c := range []struct {
		owner string
		doer  *user_model.User
		want  bool
	}{
		{"", nil, true},
		{"user5", nil, true},
		{"USER5", nil, true},
		{"user12", nil, false},       // tombstone
		{"user13", nil, false},       // empty
		{"privated_org", nil, false}, // private owner
		{"limited_org", nil, false},
		{"limited_org", user2, true},
	} {
		got, err := repo_model.HasReadableArticle(ctx, subject.ID, c.owner, c.doer)
		require.NoError(t, err)
		assert.Equal(t, c.want, got, "%s, signed in: %v", c.owner, c.doer != nil)
	}

	counts, err = repo_model.CountReadableArticlesBySubjectIDs(ctx, nil, nil)
	require.NoError(t, err)
	assert.Empty(t, counts)
}
