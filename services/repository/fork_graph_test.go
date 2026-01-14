// Copyright 2025 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/cache"
	"code.gitea.io/gitea/modules/setting"

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
	assert.Equal(t, 0, statsWithFutureSince.TotalCount, "Expected 0 contributors for future since date")
	assert.Equal(t, 0, statsWithFutureSince.RecentCount, "Expected 0 recent contributors for future since date")
}

func TestHasCommitsAfter(t *testing.T) {
	// Test the hasCommitsAfter helper function directly to verify conservative filtering
	// The function uses week START time for comparison (not week end) to avoid over-counting
	// contributors who only have pre-fork commits in a week that overlaps with fork creation.

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
	// Week starts Jan 7, since is Jan 1 -> week starts after since
	beforeWeek := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC) // Before the week
	assert.True(t, hasCommitsAfter(contributor, beforeWeek),
		"Since time before the week should return true")

	// Test 3: Since time after the week ends should return false
	afterWeek := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC) // After the week ends
	assert.False(t, hasCommitsAfter(contributor, afterWeek),
		"Since time after the week ends should return false")

	// Test 4: MID-WEEK EDGE CASE - Since time mid-week (Wednesday) should return FALSE
	// This is the key edge case: fork created on Wednesday, commits exist in that week
	// The week started Sunday (BEFORE fork), so we conservatively exclude this contributor
	// to avoid over-counting contributors who may only have pre-fork commits.
	// Trade-off: We may under-count contributors who have post-fork commits in this week.
	midWeek := time.Date(2024, 1, 10, 12, 0, 0, 0, time.UTC) // Wednesday noon
	assert.False(t, hasCommitsAfter(contributor, midWeek),
		"Mid-week since time should return false because week started before since (conservative)")

	// Test 5: Since time exactly at week end should return false
	// Week starts Jan 7, since is Jan 14 -> week starts before since
	weekEnd := time.Date(2024, 1, 14, 0, 0, 0, 0, time.UTC)
	assert.False(t, hasCommitsAfter(contributor, weekEnd),
		"Since time at week end should return false")

	// Test 6: Since time just before week end should return false (conservative)
	// Week starts Jan 7, since is Jan 13 23:59:59 -> week starts before since
	justBeforeWeekEnd := time.Date(2024, 1, 13, 23, 59, 59, 0, time.UTC) // Saturday 23:59:59
	assert.False(t, hasCommitsAfter(contributor, justBeforeWeekEnd),
		"Since time just before week end should return false (week started before since)")

	// Test 7: Since time exactly at week start should return true
	// Week starts Jan 7, since is Jan 7 -> !weekTime.Before(since) = true
	assert.True(t, hasCommitsAfter(contributor, weekStart),
		"Since time exactly at week start should return true")

	// Test 8: Contributor with no commits should return false
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

	// Test 9: Contributor with commits in a later week should return true
	// even if they also have commits in an earlier week
	laterWeekStart := time.Date(2024, 1, 14, 0, 0, 0, 0, time.UTC) // Next Sunday
	laterWeekStartMs := laterWeekStart.UnixMilli()
	multiWeekContributor := &ContributorData{
		Name:         "Multi-week User",
		TotalCommits: 10,
		Weeks: map[int64]*WeekData{
			weekStartMs: {
				Week:    weekStartMs,
				Commits: 5, // Pre-fork week
			},
			laterWeekStartMs: {
				Week:    laterWeekStartMs,
				Commits: 5, // Post-fork week
			},
		},
	}
	// Fork created mid-week Jan 10 -> later week (Jan 14) starts after fork
	assert.True(t, hasCommitsAfter(multiWeekContributor, midWeek),
		"Contributor with commits in a later week should return true")
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

// TestRegisterForkStatsCacheKey tests the registerForkStatsCacheKey function
func TestRegisterForkStatsCacheKey(t *testing.T) {
	// Clean up before and after test
	clearForkStatsCacheKeysForTesting()
	defer clearForkStatsCacheKeysForTesting()

	// Test 1: Register a single key for a repository
	registerForkStatsCacheKey(1, "key1")
	keys := getForkStatsCacheKeysForTesting(1)
	assert.NotNil(t, keys)
	assert.Len(t, keys, 1)
	_, exists := keys["key1"]
	assert.True(t, exists, "key1 should be registered")

	// Test 2: Register multiple keys for the same repository
	registerForkStatsCacheKey(1, "key2")
	registerForkStatsCacheKey(1, "key3")
	keys = getForkStatsCacheKeysForTesting(1)
	assert.Len(t, keys, 3)
	_, exists = keys["key2"]
	assert.True(t, exists, "key2 should be registered")
	_, exists = keys["key3"]
	assert.True(t, exists, "key3 should be registered")

	// Test 3: Register keys for different repositories
	registerForkStatsCacheKey(2, "key_repo2")
	keys1 := getForkStatsCacheKeysForTesting(1)
	keys2 := getForkStatsCacheKeysForTesting(2)
	assert.Len(t, keys1, 3, "repo 1 should still have 3 keys")
	assert.Len(t, keys2, 1, "repo 2 should have 1 key")
	_, exists = keys2["key_repo2"]
	assert.True(t, exists, "key_repo2 should be registered for repo 2")

	// Test 4: Registering the same key twice should not duplicate
	registerForkStatsCacheKey(1, "key1")
	keys = getForkStatsCacheKeysForTesting(1)
	assert.Len(t, keys, 3, "duplicate key should not increase count")

	// Test 5: Non-existent repository should return nil
	keys = getForkStatsCacheKeysForTesting(999)
	assert.Nil(t, keys, "non-existent repo should return nil")
}

// TestRegisterForkStatsCacheKeyConcurrent tests thread-safety of registerForkStatsCacheKey
func TestRegisterForkStatsCacheKeyConcurrent(t *testing.T) {
	// Clean up before and after test
	clearForkStatsCacheKeysForTesting()
	defer clearForkStatsCacheKeysForTesting()

	const numGoroutines = 100
	const keysPerGoroutine = 10
	repoID := int64(1)

	// Use a WaitGroup to synchronize goroutines
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Launch concurrent goroutines to register keys
	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < keysPerGoroutine; j++ {
				key := fmt.Sprintf("key_%d_%d", goroutineID, j)
				registerForkStatsCacheKey(repoID, key)
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// Verify all keys were registered
	keys := getForkStatsCacheKeysForTesting(repoID)
	assert.NotNil(t, keys)
	expectedKeys := numGoroutines * keysPerGoroutine
	assert.Len(t, keys, expectedKeys, "all unique keys should be registered")
}

// TestInvalidateForkContributorStatsCache tests the InvalidateForkContributorStatsCache function
func TestInvalidateForkContributorStatsCache(t *testing.T) {
	// Clean up before and after test
	clearForkStatsCacheKeysForTesting()
	defer clearForkStatsCacheKeysForTesting()

	// Create a mock cache for testing
	mockCache, err := cache.NewStringCache(setting.Cache{})
	assert.NoError(t, err)

	// Save original cache and restore after test
	originalCache := cache.GetCache()
	cache.SetDefaultCache(mockCache)
	defer cache.SetDefaultCache(originalCache)

	// Test 1: Invalidate with no registered keys (should not panic)
	InvalidateForkContributorStatsCache(999)
	// No assertion needed - just verify no panic

	// Test 2: Register keys and add data to cache, then invalidate
	repoID := int64(1)
	cacheKey1 := "ForkContributorStats/1/1234567890/90"
	cacheKey2 := "ForkContributorStats/1/1234567890/30"

	// Add data to cache
	assert.NoError(t, mockCache.Put(cacheKey1, "test_data_1", 300))
	assert.NoError(t, mockCache.Put(cacheKey2, "test_data_2", 300))

	// Verify data exists in cache
	_, exists := mockCache.Get(cacheKey1)
	assert.True(t, exists, "cacheKey1 should exist before invalidation")
	_, exists = mockCache.Get(cacheKey2)
	assert.True(t, exists, "cacheKey2 should exist before invalidation")

	// Register the cache keys
	registerForkStatsCacheKey(repoID, cacheKey1)
	registerForkStatsCacheKey(repoID, cacheKey2)

	// Verify keys are registered
	keys := getForkStatsCacheKeysForTesting(repoID)
	assert.Len(t, keys, 2)

	// Invalidate the cache
	InvalidateForkContributorStatsCache(repoID)

	// Verify cache entries are deleted
	_, exists = mockCache.Get(cacheKey1)
	assert.False(t, exists, "cacheKey1 should be deleted after invalidation")
	_, exists = mockCache.Get(cacheKey2)
	assert.False(t, exists, "cacheKey2 should be deleted after invalidation")

	// Verify keys are cleared from tracking map
	keys = getForkStatsCacheKeysForTesting(repoID)
	assert.Nil(t, keys, "keys should be cleared after invalidation")

	// Test 3: Verify other repositories' keys are not affected
	otherRepoID := int64(2)
	otherCacheKey := "ForkContributorStats/2/1234567890/90"
	assert.NoError(t, mockCache.Put(otherCacheKey, "other_data", 300))
	registerForkStatsCacheKey(otherRepoID, otherCacheKey)

	// Invalidate repo 1 again (should be no-op now)
	InvalidateForkContributorStatsCache(repoID)

	// Verify repo 2's data is still intact
	_, exists = mockCache.Get(otherCacheKey)
	assert.True(t, exists, "other repo's cache should not be affected")
	keys = getForkStatsCacheKeysForTesting(otherRepoID)
	assert.Len(t, keys, 1, "other repo's keys should not be affected")
}

// TestInvalidateForkContributorStatsCacheNilCache tests behavior when cache is nil
func TestInvalidateForkContributorStatsCacheNilCache(t *testing.T) {
	// Clean up before and after test
	clearForkStatsCacheKeysForTesting()
	defer clearForkStatsCacheKeysForTesting()

	// Save original cache and set to nil
	originalCache := cache.GetCache()
	cache.SetDefaultCache(nil)
	defer cache.SetDefaultCache(originalCache)

	// Register some keys
	registerForkStatsCacheKey(1, "some_key")

	// This should return early without panic
	InvalidateForkContributorStatsCache(1)

	// Keys should still be registered (not cleared because cache was nil)
	// Note: The function returns early when cache is nil, so keys remain
	keys := getForkStatsCacheKeysForTesting(1)
	assert.Len(t, keys, 1, "keys should remain when cache is nil")
}

// TestForkContributorStatsCacheIntegration tests the complete cache lifecycle
func TestForkContributorStatsCacheIntegration(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	// Clean up before and after test
	clearForkStatsCacheKeysForTesting()
	defer clearForkStatsCacheKeysForTesting()

	// Create a mock cache for testing
	mockCache, err := cache.NewStringCache(setting.Cache{})
	assert.NoError(t, err)

	// Save original cache and restore after test
	originalCache := cache.GetCache()
	cache.SetDefaultCache(mockCache)
	defer cache.SetDefaultCache(originalCache)

	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})

	// Step 1: Call getContributorStats to populate the cache
	// Using a non-zero since time to simulate fork behavior
	sinceTime := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	stats, err := getContributorStats(repo, 90, sinceTime)
	assert.NoError(t, err)
	assert.NotNil(t, stats)

	// Step 2: Note about cache key registration
	// The cache key format is: ForkContributorStats/{repoID}/{sinceUnix}/{days}
	// The cache key may or may not be registered depending on whether
	// the primary cache (GetContributorStats) returns data. If it returns
	// ErrAwaitGeneration, the secondary cache won't be populated.
	// We test the registration mechanism directly instead by manually
	// registering and populating the cache.

	// Step 3: Manually register and populate cache to test invalidation
	testCacheKey := fmt.Sprintf("ForkContributorStats/%d/1234567890/90", repo.ID)
	testStats := ContributorStats{TotalCount: 5, RecentCount: 3}
	assert.NoError(t, mockCache.PutJSON(testCacheKey, testStats, 300))
	registerForkStatsCacheKey(repo.ID, testCacheKey)

	// Verify cache entry exists
	var cachedStats ContributorStats
	exists, cacheErr := mockCache.GetJSON(testCacheKey, &cachedStats)
	assert.True(t, exists, "cache entry should exist")
	assert.Nil(t, cacheErr)
	assert.Equal(t, 5, cachedStats.TotalCount)
	assert.Equal(t, 3, cachedStats.RecentCount)

	// Verify key is registered
	keys := getForkStatsCacheKeysForTesting(repo.ID)
	assert.NotNil(t, keys)
	_, keyExists := keys[testCacheKey]
	assert.True(t, keyExists, "cache key should be registered")

	// Step 4: Invalidate the cache (simulating a push event)
	InvalidateForkContributorStatsCache(repo.ID)

	// Step 5: Verify cache entry is deleted
	exists, _ = mockCache.GetJSON(testCacheKey, &cachedStats)
	assert.False(t, exists, "cache entry should be deleted after invalidation")

	// Step 6: Verify keys are cleared
	keys = getForkStatsCacheKeysForTesting(repo.ID)
	assert.Nil(t, keys, "keys should be cleared after invalidation")

	// Step 7: Verify getContributorStats still works after invalidation
	stats, err = getContributorStats(repo, 90, sinceTime)
	assert.NoError(t, err)
	assert.NotNil(t, stats)
}

// TestInvalidateForkContributorStatsCacheMultipleKeys tests invalidation with many keys
func TestInvalidateForkContributorStatsCacheMultipleKeys(t *testing.T) {
	// Clean up before and after test
	clearForkStatsCacheKeysForTesting()
	defer clearForkStatsCacheKeysForTesting()

	// Create a mock cache for testing
	mockCache, err := cache.NewStringCache(setting.Cache{})
	assert.NoError(t, err)

	// Save original cache and restore after test
	originalCache := cache.GetCache()
	cache.SetDefaultCache(mockCache)
	defer cache.SetDefaultCache(originalCache)

	repoID := int64(1)
	numKeys := 50

	// Register many keys and add data to cache
	for i := 0; i < numKeys; i++ {
		cacheKey := fmt.Sprintf("ForkContributorStats/%d/%d/%d", repoID, i, 90)
		assert.NoError(t, mockCache.Put(cacheKey, fmt.Sprintf("data_%d", i), 300))
		registerForkStatsCacheKey(repoID, cacheKey)
	}

	// Verify all keys are registered
	keys := getForkStatsCacheKeysForTesting(repoID)
	assert.Len(t, keys, numKeys)

	// Verify all cache entries exist
	for i := 0; i < numKeys; i++ {
		cacheKey := fmt.Sprintf("ForkContributorStats/%d/%d/%d", repoID, i, 90)
		_, exists := mockCache.Get(cacheKey)
		assert.True(t, exists, "cache entry %d should exist", i)
	}

	// Invalidate all at once
	InvalidateForkContributorStatsCache(repoID)

	// Verify all cache entries are deleted
	for i := 0; i < numKeys; i++ {
		cacheKey := fmt.Sprintf("ForkContributorStats/%d/%d/%d", repoID, i, 90)
		_, exists := mockCache.Get(cacheKey)
		assert.False(t, exists, "cache entry %d should be deleted", i)
	}

	// Verify keys are cleared
	keys = getForkStatsCacheKeysForTesting(repoID)
	assert.Nil(t, keys)
}
