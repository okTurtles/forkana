// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repo_test

import (
	"testing"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/timeutil"

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
	// user33, a limited user: only for signed-in users
	assign(17, subject.ID, func(r *repo_model.Repository) []string {
		r.OwnerID, r.OwnerName, r.IsEmpty = 33, "user33", false
		return []string{"owner_id", "owner_name", "is_empty"}
	})

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
	assert.ElementsMatch(t, []string{"user2", "user5", "limited_org", "user33"}, owners)

	// the cap counts owners, the most recently updated first (review M2): user2's two
	// articles count once
	setUpdated := func(repoID, unix int64) {
		repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: repoID})
		repo.UpdatedUnix = timeutil.TimeStamp(unix)
		require.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(ctx, repo, "updated_unix"))
	}
	setUpdated(1, 300) // user2
	setUpdated(2, 400) // user2
	setUpdated(4, 350) // user5
	setUpdated(38, 100)
	setUpdated(17, 100)
	owners, err = repo_model.FindReadableArticleOwnersWithLimit(ctx, subject.ID, user2, 2)
	require.NoError(t, err)
	assert.Equal(t, []string{"user2", "user5"}, owners)

	// an owner whose articles on the subject are all archived comes last, however recent
	repo4 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 4})
	repo4.IsArchived = true
	repo4.UpdatedUnix = 500
	require.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(ctx, repo4, "is_archived", "updated_unix"))
	owners, err = repo_model.FindReadableArticleOwners(ctx, subject.ID, user2)
	require.NoError(t, err)
	require.Len(t, owners, 4)
	assert.Equal(t, "user2", owners[0])
	assert.Equal(t, "user5", owners[3])

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
		{"user33", nil, false}, // limited user
		{"user33", user2, true},
	} {
		got, err := repo_model.HasReadableArticle(ctx, subject.ID, c.owner, c.doer)
		require.NoError(t, err)
		assert.Equal(t, c.want, got, "%s, signed in: %v", c.owner, c.doer != nil)
	}

	counts, err = repo_model.CountReadableArticlesBySubjectIDs(ctx, nil, nil)
	require.NoError(t, err)
	assert.Empty(t, counts)
}

// #248: a subject row shows its original article's summary and its latest update
func TestFindSubjectListings(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	subject, err := repo_model.GetOrCreateSubject(ctx, "Apollo 11")
	require.NoError(t, err)
	other, err := repo_model.GetOrCreateSubject(ctx, "Apollo 17")
	require.NoError(t, err)
	unread, err := repo_model.GetOrCreateSubject(ctx, "Apollo 13")
	require.NoError(t, err)

	assign := func(repoID, subjectID, created, updated int64, description string, fork bool) {
		repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: repoID})
		repo.SubjectID, repo.Description = subjectID, description
		repo.CreatedUnix, repo.UpdatedUnix = timeutil.TimeStamp(created), timeutil.TimeStamp(updated)
		repo.IsFork = fork
		require.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(ctx, repo,
			"subject_id", "description", "created_unix", "updated_unix", "is_fork"))
	}
	assign(2, subject.ID, 50, 100, "The private one", false)   // user2, private: the oldest
	assign(1, subject.ID, 100, 200, "  The original  ", false) // user2, public
	assign(4, subject.ID, 200, 900, "A fork", true)            // user5, a fork: the latest update
	assign(33, other.ID, 300, 400, "", false)                  // user2, public, no summary
	assign(10, unread.ID, 10, 10, "Deleted", false)            // user12, deleted below
	tomb := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 10})
	tomb.IsTombstoned = true
	require.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(ctx, tomb, "is_tombstoned"))

	ids := []int64{subject.ID, other.ID, unread.ID}

	// signed out, the private article is neither the root nor the latest update; a fork is
	// never the root but its update counts
	listings, err := repo_model.FindSubjectListings(ctx, ids, nil)
	require.NoError(t, err)
	require.Contains(t, listings, subject.ID)
	assert.Equal(t, "The original", listings[subject.ID].Summary)
	assert.Equal(t, timeutil.TimeStamp(900), listings[subject.ID].UpdatedUnix)
	require.Contains(t, listings, other.ID)
	assert.Empty(t, listings[other.ID].Summary)
	assert.Equal(t, timeutil.TimeStamp(400), listings[other.ID].UpdatedUnix)
	assert.NotContains(t, listings, unread.ID)

	// its owner can open the private article, the oldest, so it is the root for them
	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	listings, err = repo_model.FindSubjectListings(ctx, ids, user2)
	require.NoError(t, err)
	assert.Equal(t, "The private one", listings[subject.ID].Summary)

	subjects := []*repo_model.Subject{subject, other, unread}
	require.NoError(t, repo_model.LoadSubjectListings(ctx, subjects, nil))
	require.NotNil(t, subject.Listing)
	assert.Equal(t, "The original", subject.Listing.Summary)
	assert.Nil(t, unread.Listing)

	listings, err = repo_model.FindSubjectListings(ctx, nil, nil)
	require.NoError(t, err)
	assert.Empty(t, listings)
}
