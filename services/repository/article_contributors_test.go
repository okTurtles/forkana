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

	ctx := context.Background()
	graph, err := BuildForkGraph(ctx, repo, SubjectForkGraphParams(), user)
	require.NoError(t, err)
	require.NotNil(t, graph.Root)

	for _, entry := range graph.Articles() {
		want, err := ArticleContributorCount(ctx, entry.Repo)
		require.NoError(t, err, entry.Repo.FullName())
		assert.Equal(t, want, entry.ContributorCount, entry.Repo.FullName())
	}
	require.NotEmpty(t, graph.Articles())
	assert.Positive(t, graph.Articles()[0].ContributorCount)
}
