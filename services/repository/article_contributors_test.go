// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"testing"
	"time"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/timeutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArticleContributorSince(t *testing.T) {
	created := timeutil.TimeStamp(1700000000)
	assert.True(t, ArticleContributorSince(&repo_model.Repository{CreatedUnix: created}).IsZero())
	assert.True(t, ArticleContributorSince(&repo_model.Repository{IsFork: true}).IsZero())
	assert.Equal(t, time.Unix(1700000000, 0), ArticleContributorSince(&repo_model.Repository{IsFork: true, CreatedUnix: created}))
}

func TestFlattenForkGraph(t *testing.T) {
	root := &ForkNode{
		repo:         &repo_model.Repository{ID: 1},
		Contributors: &ContributorStats{TotalCount: 3},
		Children: []*ForkNode{
			{
				repo:         &repo_model.Repository{ID: 2},
				Contributors: &ContributorStats{TotalCount: 0},
				Children: []*ForkNode{
					{repo: &repo_model.Repository{ID: 4}, Contributors: &ContributorStats{TotalCount: 1}},
				},
			},
			{repo: &repo_model.Repository{ID: 3}},
		},
	}

	entries := FlattenForkGraph(root)
	ids := make([]int64, 0, len(entries))
	counts := make([]int64, 0, len(entries))
	for _, e := range entries {
		ids = append(ids, e.Repo.ID)
		counts = append(counts, e.ContributorCount)
	}
	// depth first, so a fork of a fork is listed (#405 item 3), right after its parent
	assert.Equal(t, []int64{1, 2, 4, 3}, ids)
	// a genuine 0 stays 0; a node without stats is unknown (-1)
	assert.Equal(t, []int64{3, 0, 1, -1}, counts)

	assert.Empty(t, FlattenForkGraph(nil))
}

// The bubble of an article carries the same contributor count as its table row and its
// article page, which all use ArticleContributorCount (#405 item 4).
func TestBuildForkGraphUsesArticleContributorCount(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())

	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	user := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})

	ctx := t.Context()
	graph, err := BuildForkGraph(ctx, repo, SubjectForkGraphParams(), user)
	require.NoError(t, err)
	require.NotNil(t, graph.Root)
	require.NotEmpty(t, graph.Articles())

	for _, entry := range graph.Articles() {
		want, err := ArticleContributorCount(ctx, entry.Repo)
		require.NoError(t, err, entry.Repo.FullName())
		assert.Equal(t, want, entry.ContributorCount, entry.Repo.FullName())
	}
	assert.Positive(t, graph.Articles()[0].ContributorCount)
}

// A count is cached under the branch head and the "since" time: answered from the cache
// without git while the head is the same, and counted again for a new head.
func TestArticleContributorCountCache(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()
	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})

	count, err := ArticleContributorCount(ctx, repo)
	require.NoError(t, err)

	heads := branchHeads(ctx, []*repo_model.Repository{repo})
	head := heads[repo.ID]
	require.NotEmpty(t, head, "the default branch head comes from the database")

	cached, ok := cachedArticleContributorCount(repo, head)
	require.True(t, ok, "the count is cached under its head")
	assert.Equal(t, count, cached)

	// a new head is a new key: nothing cached for it yet
	_, ok = cachedArticleContributorCount(repo, "0000000000000000000000000000000000000001")
	assert.False(t, ok)

	// "since" is part of the key: the same repository counted as a fork is another count
	fork := *repo
	fork.IsFork = true
	fork.CreatedUnix = timeutil.TimeStamp(1_700_000_000)
	assert.NotEqual(t,
		articleContributorCountCacheKey(repo.ID, head, ArticleContributorSince(repo)),
		articleContributorCountCacheKey(fork.ID, head, ArticleContributorSince(&fork)))

	// a node whose head is cached is answered without git, even once the counting
	// phase is over; one whose head is not cached is unknown then
	done, cancel := context.WithCancel(ctx)
	cancel()
	stats := nodeContributorStats(done, repo, head)
	require.NotNil(t, stats)
	assert.Equal(t, int(count), stats.TotalCount)
	assert.Nil(t, nodeContributorStats(done, repo, "0000000000000000000000000000000000000001"))
}
