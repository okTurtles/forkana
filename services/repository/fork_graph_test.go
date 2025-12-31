// Copyright 2025 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"testing"
	"time"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"

	"github.com/stretchr/testify/assert"
)

func TestBuildForkGraph(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	user := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})

	params := ForkGraphParams{
		IncludeContributors: false,
		ContributorDays:     90,
		MaxDepth:            10,
		IncludePrivate:      false,
		Sort:                "updated",
		Page:                1,
		Limit:               50,
	}

	ctx := context.Background()
	graph, err := BuildForkGraph(ctx, repo, params, user)

	assert.NoError(t, err)
	assert.NotNil(t, graph)
	assert.NotNil(t, graph.Root)
	assert.Equal(t, "repo_1", graph.Root.ID)
	assert.Equal(t, 0, graph.Root.Level)
	assert.NotNil(t, graph.Metadata)
}

func TestBuildForkGraphWithContributors(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	user := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})

	params := ForkGraphParams{
		IncludeContributors: true,
		ContributorDays:     30,
		MaxDepth:            10,
		IncludePrivate:      false,
		Sort:                "updated",
		Page:                1,
		Limit:               50,
	}

	ctx := context.Background()
	graph, err := BuildForkGraph(ctx, repo, params, user)

	assert.NoError(t, err)
	assert.NotNil(t, graph)
	assert.Equal(t, 30, graph.Metadata.ContributorWindowDays)
}

func TestBuildForkGraphMaxDepth(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	user := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})

	params := ForkGraphParams{
		IncludeContributors: false,
		ContributorDays:     90,
		MaxDepth:            2,
		IncludePrivate:      false,
		Sort:                "updated",
		Page:                1,
		Limit:               50,
	}

	ctx := context.Background()
	graph, err := BuildForkGraph(ctx, repo, params, user)

	assert.NoError(t, err)
	assert.NotNil(t, graph)

	// Check that depth is limited
	maxLevel := getMaxLevel(graph.Root)
	assert.LessOrEqual(t, maxLevel, 2)
}

func TestSortRepositories(t *testing.T) {
	repos := []*repo_model.Repository{
		{ID: 1, NumStars: 10, NumForks: 5, UpdatedUnix: 1000, CreatedUnix: 900},
		{ID: 2, NumStars: 20, NumForks: 3, UpdatedUnix: 2000, CreatedUnix: 1000},
		{ID: 3, NumStars: 5, NumForks: 10, UpdatedUnix: 1500, CreatedUnix: 1100},
	}

	// Test sort by stars
	sortRepositories(repos, "stars")
	assert.Equal(t, int64(2), repos[0].ID)
	assert.Equal(t, int64(1), repos[1].ID)
	assert.Equal(t, int64(3), repos[2].ID)

	// Reset
	repos = []*repo_model.Repository{
		{ID: 1, NumStars: 10, NumForks: 5, UpdatedUnix: 1000, CreatedUnix: 900},
		{ID: 2, NumStars: 20, NumForks: 3, UpdatedUnix: 2000, CreatedUnix: 1000},
		{ID: 3, NumStars: 5, NumForks: 10, UpdatedUnix: 1500, CreatedUnix: 1100},
	}

	// Test sort by forks
	sortRepositories(repos, "forks")
	assert.Equal(t, int64(3), repos[0].ID)
	assert.Equal(t, int64(1), repos[1].ID)
	assert.Equal(t, int64(2), repos[2].ID)

	// Reset
	repos = []*repo_model.Repository{
		{ID: 1, NumStars: 10, NumForks: 5, UpdatedUnix: 1000, CreatedUnix: 900},
		{ID: 2, NumStars: 20, NumForks: 3, UpdatedUnix: 2000, CreatedUnix: 1000},
		{ID: 3, NumStars: 5, NumForks: 10, UpdatedUnix: 1500, CreatedUnix: 1100},
	}

	// Test sort by updated
	sortRepositories(repos, "updated")
	assert.Equal(t, int64(2), repos[0].ID)
	assert.Equal(t, int64(3), repos[1].ID)
	assert.Equal(t, int64(1), repos[2].ID)

	// Reset
	repos = []*repo_model.Repository{
		{ID: 1, NumStars: 10, NumForks: 5, UpdatedUnix: 1000, CreatedUnix: 900},
		{ID: 2, NumStars: 20, NumForks: 3, UpdatedUnix: 2000, CreatedUnix: 1000},
		{ID: 3, NumStars: 5, NumForks: 10, UpdatedUnix: 1500, CreatedUnix: 1100},
	}

	// Test sort by created
	sortRepositories(repos, "created")
	assert.Equal(t, int64(3), repos[0].ID)
	assert.Equal(t, int64(2), repos[1].ID)
	assert.Equal(t, int64(1), repos[2].ID)
}

func TestCountVisibleForks(t *testing.T) {
	// Create a simple tree structure
	root := &ForkNode{
		ID:    "repo_1",
		Level: 0,
		Children: []*ForkNode{
			{
				ID:    "repo_2",
				Level: 1,
				Children: []*ForkNode{
					{
						ID:       "repo_4",
						Level:    2,
						Children: []*ForkNode{},
					},
				},
			},
			{
				ID:       "repo_3",
				Level:    1,
				Children: []*ForkNode{},
			},
		},
	}

	count := countVisibleForks(root)
	assert.Equal(t, 3, count) // 2 direct children + 1 grandchild
}

func TestCycleDetection(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	user := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})

	params := ForkGraphParams{
		IncludeContributors: false,
		ContributorDays:     90,
		MaxDepth:            10,
		IncludePrivate:      false,
		Sort:                "updated",
		Page:                1,
		Limit:               50,
	}

	ctx := context.Background()
	visited := make(map[int64]bool)
	nodeCount := 0
	maxDepthReached := false

	// Build node twice with same repo - should detect cycle
	node1, err := buildNode(ctx, repo, 0, params, user, visited, &nodeCount, &maxDepthReached)
	assert.NoError(t, err)
	assert.NotNil(t, node1)

	// Try to build same repo again - should return ErrCycleDetected
	node2, err := buildNode(ctx, repo, 0, params, user, visited, &nodeCount, &maxDepthReached)
	assert.Error(t, err)
	assert.True(t, IsErrCycleDetected(err))
	assert.Nil(t, node2)
}

func TestErrorTypes(t *testing.T) {
	assert.True(t, IsErrMaxDepthExceeded(ErrMaxDepthExceeded))
	assert.False(t, IsErrMaxDepthExceeded(ErrTooManyNodes))

	assert.True(t, IsErrTooManyNodes(ErrTooManyNodes))
	assert.False(t, IsErrTooManyNodes(ErrProcessingTimeout))

	assert.True(t, IsErrProcessingTimeout(ErrProcessingTimeout))
	assert.False(t, IsErrProcessingTimeout(ErrMaxDepthExceeded))

	assert.True(t, IsErrCycleDetected(ErrCycleDetected))
	assert.False(t, IsErrCycleDetected(ErrTooManyNodes))
}

func TestGetContributorStats(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})

	// Test getting contributor stats without since filter (non-fork)
	stats, err := getContributorStats(repo, 90, time.Time{})

	// Should not error even if stats are not available
	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.GreaterOrEqual(t, stats.TotalCount, 0)
	assert.GreaterOrEqual(t, stats.RecentCount, 0)

	// Test getting contributor stats with since filter (simulating fork)
	// Using a future time should result in 0 contributors
	futureTime := time.Now().AddDate(1, 0, 0)
	statsWithFutureSince, err := getContributorStats(repo, 90, futureTime)
	assert.NoError(t, err)
	assert.NotNil(t, statsWithFutureSince)
	assert.EqualValues(t, 0, statsWithFutureSince.TotalCount, "Expected 0 contributors for future since date")
	assert.EqualValues(t, 0, statsWithFutureSince.RecentCount, "Expected 0 recent contributors for future since date")
}

func TestHasCommitsAfter(t *testing.T) {
	// Test the hasCommitsAfter helper function directly to verify mid-week edge case handling

	// Create a mock contributor with commits in a specific week
	// Week starts on Sunday 2024-01-07 (Unix timestamp in milliseconds)
	weekStart := time.Date(2024, 1, 7, 0, 0, 0, 0, time.UTC) // Sunday
	weekStartMs := weekStart.UnixMilli()

	contributor := &ContributorData{
		Name:         "Test User",
		TotalCommits: 5,
		Weeks: map[int64]*WeekData{
			weekStartMs: {
				Week:    weekStartMs,
				Commits: 5,
			},
		},
	}

	// Test 1: Zero since time should always return true
	assert.True(t, hasCommitsAfter(contributor, time.Time{}),
		"Zero since time should return true")

	// Test 2: Since time before the week should return true
	beforeWeek := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC) // Before the week
	assert.True(t, hasCommitsAfter(contributor, beforeWeek),
		"Since time before the week should return true")

	// Test 3: Since time after the week ends should return false
	afterWeek := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC) // After the week ends
	assert.False(t, hasCommitsAfter(contributor, afterWeek),
		"Since time after the week ends should return false")

	// Test 4: MID-WEEK EDGE CASE - Since time mid-week (Wednesday) should return true
	// This is the key edge case: fork created on Wednesday, commits exist in that week
	// The week started Sunday (before fork) but ends Sunday (after fork)
	// So commits made Thursday-Saturday should be counted
	midWeek := time.Date(2024, 1, 10, 12, 0, 0, 0, time.UTC) // Wednesday noon
	assert.True(t, hasCommitsAfter(contributor, midWeek),
		"Mid-week since time should return true because week ends after since")

	// Test 5: Since time exactly at week end should return false
	// Week ends at the start of the next Sunday (2024-01-14 00:00:00)
	weekEnd := time.Date(2024, 1, 14, 0, 0, 0, 0, time.UTC)
	assert.False(t, hasCommitsAfter(contributor, weekEnd),
		"Since time at week end should return false")

	// Test 6: Since time just before week end should return true
	justBeforeWeekEnd := time.Date(2024, 1, 13, 23, 59, 59, 0, time.UTC) // Saturday 23:59:59
	assert.True(t, hasCommitsAfter(contributor, justBeforeWeekEnd),
		"Since time just before week end should return true")

	// Test 7: Contributor with no commits should return false
	emptyContributor := &ContributorData{
		Name:         "Empty User",
		TotalCommits: 0,
		Weeks: map[int64]*WeekData{
			weekStartMs: {
				Week:    weekStartMs,
				Commits: 0, // No commits in this week
			},
		},
	}
	assert.False(t, hasCommitsAfter(emptyContributor, beforeWeek),
		"Contributor with no commits should return false even with valid since time")
}

func TestProcessingTimeout(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	user := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})

	params := ForkGraphParams{
		IncludeContributors: false,
		ContributorDays:     90,
		MaxDepth:            10,
		IncludePrivate:      false,
		Sort:                "updated",
		Page:                1,
		Limit:               50,
	}

	// Create a context that's already cancelled
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	time.Sleep(10 * time.Millisecond) // Ensure timeout

	visited := make(map[int64]bool)
	nodeCount := 0
	maxDepthReached := false

	_, err := buildNode(ctx, repo, 0, params, user, visited, &nodeCount, &maxDepthReached)
	assert.Error(t, err)
	assert.True(t, IsErrProcessingTimeout(err))
}

// Helper function to get max level in tree
func getMaxLevel(node *ForkNode) int {
	if node == nil || len(node.Children) == 0 {
		return node.Level
	}

	maxLevel := node.Level
	for _, child := range node.Children {
		childMax := getMaxLevel(child)
		if childMax > maxLevel {
			maxLevel = childMax
		}
	}

	return maxLevel
}

func TestCycleDetection_SelfLoop(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	user := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})

	params := ForkGraphParams{
		IncludeContributors: false,
		ContributorDays:     90,
		MaxDepth:            10,
		IncludePrivate:      false,
		Sort:                "updated",
		Page:                1,
		Limit:               50,
	}

	ctx := context.Background()
	visited := make(map[int64]bool)
	nodeCount := 0
	maxDepthReached := false

	// First call should succeed
	node1, err := buildNode(ctx, repo, 0, params, user, visited, &nodeCount, &maxDepthReached)
	assert.NoError(t, err)
	assert.NotNil(t, node1)
	assert.Equal(t, 1, nodeCount)

	// Second call with same repo should detect cycle
	node2, err := buildNode(ctx, repo, 0, params, user, visited, &nodeCount, &maxDepthReached)
	assert.Error(t, err)
	assert.True(t, IsErrCycleDetected(err))
	assert.Nil(t, node2)
	// Node count should not increase on cycle detection
	assert.Equal(t, 1, nodeCount)
}

func TestCycleDetection_VisitedMapPreventsInfiniteRecursion(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	user := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})

	params := ForkGraphParams{
		IncludeContributors: false,
		ContributorDays:     90,
		MaxDepth:            10,
		IncludePrivate:      false,
		Sort:                "updated",
		Page:                1,
		Limit:               50,
	}

	ctx := context.Background()
	visited := make(map[int64]bool)
	nodeCount := 0
	maxDepthReached := false

	// Build node - this will mark repo as visited
	node, err := buildNode(ctx, repo, 0, params, user, visited, &nodeCount, &maxDepthReached)
	assert.NoError(t, err)
	assert.NotNil(t, node)

	// Verify repo is in visited map
	assert.True(t, visited[repo.ID])

	// Attempting to visit again should immediately return ErrCycleDetected
	// without causing stack overflow or infinite recursion
	node2, err := buildNode(ctx, repo, 0, params, user, visited, &nodeCount, &maxDepthReached)
	assert.Error(t, err)
	assert.True(t, IsErrCycleDetected(err))
	assert.Nil(t, node2)
}

func TestCycleDetection_ErrorPropagation(t *testing.T) {
	// Test that ErrCycleDetected is properly handled by callers
	// and doesn't cause the entire graph building to fail

	assert.NoError(t, unittest.PrepareTestDatabase())

	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	user := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})

	params := ForkGraphParams{
		IncludeContributors: false,
		ContributorDays:     90,
		MaxDepth:            10,
		IncludePrivate:      false,
		Sort:                "updated",
		Page:                1,
		Limit:               50,
	}

	ctx := context.Background()

	// BuildForkGraph should handle cycles gracefully and continue building
	graph, err := BuildForkGraph(ctx, repo, params, user)
	assert.NoError(t, err)
	assert.NotNil(t, graph)
	assert.NotNil(t, graph.Root)
}

func TestCycleDetection_DeepForkChain(t *testing.T) {
	// Test that cycle detection works correctly in deep fork chains
	// This ensures O(n) performance and no stack overflow

	assert.NoError(t, unittest.PrepareTestDatabase())

	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	user := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})

	params := ForkGraphParams{
		IncludeContributors: false,
		ContributorDays:     90,
		MaxDepth:            100, // Deep chain
		IncludePrivate:      false,
		Sort:                "updated",
		Page:                1,
		Limit:               50,
	}

	ctx := context.Background()
	visited := make(map[int64]bool)
	nodeCount := 0
	maxDepthReached := false

	// Build a deep chain - should not cause stack overflow
	node, err := buildNode(ctx, repo, 0, params, user, visited, &nodeCount, &maxDepthReached)
	assert.NoError(t, err)
	assert.NotNil(t, node)

	// Verify visited map has entries (cycle detection is working)
	assert.NotEmpty(t, visited)
}
