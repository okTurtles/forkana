// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package feed

import (
	"fmt"
	"testing"
	"time"

	activities_model "code.gitea.io/gitea/models/activities"
	"code.gitea.io/gitea/models/db"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetFeedsFollowedSubjects: a user who follows a subject (#165) has, in their feed, the
// activity on every article of the subject they can read, once each, whether or not they
// also watch the article; and nothing more once they stop following it.
func TestGetFeedsFollowedSubjects(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})       // the actor
	user4 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})       // watches repo1
	user5 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 5})       // does not
	repo1 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1}) // public, subject 1
	repo2 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 2}) // user2's private one
	repo4 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 4}) // public, no subject
	require.EqualValues(t, 1, repo1.SubjectID)
	require.True(t, repo2.IsPrivate)
	require.Zero(t, repo4.SubjectID)
	require.True(t, repo_model.IsWatching(ctx, user4.ID, repo1.ID))
	require.False(t, repo_model.IsWatching(ctx, user5.ID, repo1.ID))

	// the private article is in the subject too: it must stay out of the feed of a reader
	// who cannot read it
	_, err := db.GetEngine(ctx).ID(repo2.ID).Cols("subject_id").Update(&repo_model.Repository{SubjectID: 1})
	require.NoError(t, err)

	since := time.Now().Unix() - 1
	for _, repo := range []*repo_model.Repository{repo1, repo2, repo4} {
		require.NoError(t, NotifyWatchers(ctx, &activities_model.Action{
			ActUserID: user2.ID,
			OpType:    activities_model.ActionCreatePullRequest,
			RepoID:    repo.ID,
			IsPrivate: repo.IsPrivate,
			Content:   "1|subject follow",
		}))
	}

	feed := func(t *testing.T, user, actor *user_model.User, followed bool) []int64 {
		return feedRepoIDs(t, activities_model.GetFeedsOptions{
			RequestedUser:           user,
			Actor:                   actor,
			IncludePrivate:          true,
			SinceUnix:               since,
			IncludeFollowedSubjects: followed,
		})
	}

	// not following: user5 has nothing, user4 has repo1 as a watcher
	assert.Empty(t, feed(t, user5, user5, true))
	assert.Equal(t, []int64{repo1.ID}, feed(t, user4, user4, true))

	require.NoError(t, repo_model.FollowSubject(ctx, user5.ID, 1, true))
	require.NoError(t, repo_model.FollowSubject(ctx, user4.ID, 1, true))

	// following: repo1 once, neither the private article nor the one outside the subject
	assert.Equal(t, []int64{repo1.ID}, feed(t, user5, user5, true))
	assert.Equal(t, []int64{repo1.ID}, feed(t, user4, user4, true), "a watcher's own copy is not listed twice")
	// ...with the permission checks of another reader (ActivityQueryCondition) too
	assert.Equal(t, []int64{repo1.ID}, feed(t, user5, user4, true))
	// ...and only where it is asked for
	assert.Empty(t, feed(t, user5, user5, false))

	// the actor's own feed is unchanged: their actions, once each
	assert.ElementsMatch(t, []int64{repo1.ID, repo2.ID, repo4.ID}, feed(t, user2, user2, true))

	require.NoError(t, repo_model.FollowSubject(ctx, user5.ID, 1, false))
	assert.Empty(t, feed(t, user5, user5, true))
}

// TestGetFeedsFollowedSubjectsManyArticles: past db.DefaultMaxInSize followed articles, the
// feed takes them from a subquery rather than a list of ids, with the same result.
func TestGetFeedsFollowedSubjectsManyArticles(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()
	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	user4 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4}) // watches repo1
	repo1 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})

	var last *repo_model.Repository
	for i := range db.DefaultMaxInSize {
		name := fmt.Sprintf("follow-many-%d", i)
		last = &repo_model.Repository{OwnerID: user2.ID, OwnerName: user2.Name, SubjectID: 1, Name: name, LowerName: name}
		require.NoError(t, db.Insert(ctx, last))
	}
	require.NoError(t, repo_model.FollowSubject(ctx, user4.ID, 1, true))
	ids, err := repo_model.FindFollowedSubjectReadableRepoIDs(ctx, user4, db.DefaultMaxInSize+1)
	require.NoError(t, err)
	require.Len(t, ids, db.DefaultMaxInSize+1, "more than an IN list takes")

	since := time.Now().Unix() - 1
	for _, repo := range []*repo_model.Repository{repo1, last} {
		require.NoError(t, NotifyWatchers(ctx, &activities_model.Action{
			ActUserID: user2.ID,
			OpType:    activities_model.ActionCreatePullRequest,
			RepoID:    repo.ID,
			Content:   "1|subject follow",
		}))
	}
	repoIDs := feedRepoIDs(t, activities_model.GetFeedsOptions{
		RequestedUser:           user4,
		Actor:                   user4,
		IncludePrivate:          true,
		SinceUnix:               since,
		IncludeFollowedSubjects: true,
	})
	assert.ElementsMatch(t, []int64{repo1.ID, last.ID}, repoIDs, "each once, repo1's watcher copy left out")
}

// feedRepoIDs returns the article of each action of the feed, in its order.
func feedRepoIDs(t *testing.T, opts activities_model.GetFeedsOptions) []int64 {
	t.Helper()
	actions, _, err := GetFeeds(t.Context(), opts)
	require.NoError(t, err)
	repoIDs := make([]int64, 0, len(actions))
	for _, act := range actions {
		repoIDs = append(repoIDs, act.RepoID)
	}
	return repoIDs
}
