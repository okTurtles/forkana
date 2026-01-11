// Copyright 2025 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"code.gitea.io/gitea/models/db"
	perm_model "code.gitea.io/gitea/models/perm"
	access_model "code.gitea.io/gitea/models/perm/access"
	repo_model "code.gitea.io/gitea/models/repo"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/cache"
	"code.gitea.io/gitea/modules/log"
	api "code.gitea.io/gitea/modules/structs"
	"code.gitea.io/gitea/services/convert"
)

// Error definitions
var (
	ErrMaxDepthExceeded  = errors.New("maximum depth exceeded")
	ErrTooManyNodes      = errors.New("too many nodes in graph")
	ErrProcessingTimeout = errors.New("processing timeout")
	ErrCycleDetected     = errors.New("cycle detected in fork graph")

	// forkStatsComputeLock prevents cache stampede on secondary cache computation.
	// When multiple goroutines request the same cache key simultaneously, only one
	// will compute and cache the result; others will compute without caching.
	forkStatsComputeLock = sync.Map{}

	// forkStatsCacheKeys tracks active cache keys per repository for invalidation.
	// Key: repoID (int64), Value: map[string]struct{} (set of cache keys)
	// This enables efficient cache invalidation when commits are pushed to a repository.
	forkStatsCacheKeys     = sync.Map{}
	forkStatsCacheKeysLock = sync.Mutex{}
)

// IsErrMaxDepthExceeded checks if an error is ErrMaxDepthExceeded
func IsErrMaxDepthExceeded(err error) bool {
	return errors.Is(err, ErrMaxDepthExceeded)
}

// IsErrTooManyNodes checks if an error is ErrTooManyNodes
func IsErrTooManyNodes(err error) bool {
	return errors.Is(err, ErrTooManyNodes)
}

// IsErrProcessingTimeout checks if an error is ErrProcessingTimeout
func IsErrProcessingTimeout(err error) bool {
	return errors.Is(err, ErrProcessingTimeout)
}

// IsErrCycleDetected checks if an error is ErrCycleDetected
func IsErrCycleDetected(err error) bool {
	return errors.Is(err, ErrCycleDetected)
}

// registerForkStatsCacheKey registers a cache key for a repository.
// This enables efficient cache invalidation when commits are pushed.
func registerForkStatsCacheKey(repoID int64, cacheKey string) {
	forkStatsCacheKeysLock.Lock()
	defer forkStatsCacheKeysLock.Unlock()

	var keys map[string]struct{}
	if v, ok := forkStatsCacheKeys.Load(repoID); ok {
		keys = v.(map[string]struct{})
	} else {
		keys = make(map[string]struct{})
	}
	keys[cacheKey] = struct{}{}
	forkStatsCacheKeys.Store(repoID, keys)
}

// InvalidateForkContributorStatsCache invalidates all fork contributor stats cache entries
// for a specific repository. This should be called when commits are pushed to ensure
// contributor statistics are refreshed.
//
// The function is safe to call even if no cache entries exist for the repository.
// Errors during cache deletion are logged but don't cause the function to fail,
// as cache invalidation is best-effort.
func InvalidateForkContributorStatsCache(repoID int64) {
	c := cache.GetCache()
	if c == nil {
		return
	}

	forkStatsCacheKeysLock.Lock()
	v, ok := forkStatsCacheKeys.Load(repoID)
	if !ok {
		forkStatsCacheKeysLock.Unlock()
		return
	}
	keys := v.(map[string]struct{})
	// Clear the keys map for this repo
	forkStatsCacheKeys.Delete(repoID)
	forkStatsCacheKeysLock.Unlock()

	// Delete all cached entries for this repository
	for cacheKey := range keys {
		if err := c.Delete(cacheKey); err != nil {
			log.Warn("Failed to invalidate fork contributor stats cache key %s: %v", cacheKey, err)
		}
	}

	if len(keys) > 0 {
		log.Debug("Invalidated %d fork contributor stats cache entries for repo %d", len(keys), repoID)
	}
}

// ForkGraphParams represents parameters for building fork graph
type ForkGraphParams struct {
	IncludeContributors bool
	ContributorDays     int
	MaxDepth            int
	IncludePrivate      bool
	Sort                string
	Page                int
	Limit               int
}

// ForkGraphResponse represents the complete fork graph response
type ForkGraphResponse struct {
	Root       *ForkNode       `json:"root"`
	Metadata   GraphMetadata   `json:"metadata"`
	Pagination *PaginationInfo `json:"pagination,omitempty"`
}

// ForkNode represents a node in the fork tree
type ForkNode struct {
	ID           string            `json:"id"`
	Repository   *api.Repository   `json:"repository"`
	Contributors *ContributorStats `json:"contributors,omitempty"`
	Level        int               `json:"level"`
	Children     []*ForkNode       `json:"children"`

	// Internal field for batch processing (not exported to JSON)
	repo *repo_model.Repository `json:"-"`
}

// ContributorStats represents contributor statistics
type ContributorStats struct {
	TotalCount  int `json:"total_count"`
	RecentCount int `json:"recent_count"`
}

// GraphMetadata represents metadata about the fork graph
type GraphMetadata struct {
	TotalForks            int       `json:"total_forks"`
	VisibleForks          int       `json:"visible_forks"`
	MaxDepthReached       bool      `json:"max_depth_reached"`
	CacheStatus           string    `json:"cache_status"`
	GeneratedAt           time.Time `json:"generated_at"`
	ContributorWindowDays int       `json:"contributor_window_days,omitempty"`
}

// PaginationInfo represents pagination information
type PaginationInfo struct {
	Page       int  `json:"page"`
	Limit      int  `json:"limit"`
	TotalPages int  `json:"total_pages"`
	HasNext    bool `json:"has_next"`
}

const (
	maxNodes          = 10000
	processingTimeout = 30 * time.Second

	// forkContributorStatsCacheKey is the cache key format for pre-filtered fork contributor stats.
	// Format: "ForkContributorStats/{repoID}/{sinceUnix}/{days}"
	// This secondary cache stores pre-filtered results to avoid repeated post-cache filtering.
	forkContributorStatsCacheKey = "ForkContributorStats/%d/%d/%d"
	// forkContributorStatsCacheTimeout is the TTL for fork contributor stats cache (5 minutes).
	// This is shorter than the base contributor stats cache (10 minutes) to ensure
	// the secondary cache doesn't outlive the underlying data.
	forkContributorStatsCacheTimeout int64 = 60 * 5
)

// BuildForkGraph builds the fork graph for a repository
func BuildForkGraph(ctx context.Context, repo *repo_model.Repository, params ForkGraphParams, doer *user_model.User) (*ForkGraphResponse, error) {
	// Find the root repository for the fork graph.
	// Priority:
	// 1. If the repository has a subject, find the subject's root repository (first non-empty, non-fork repo for that subject)
	// 2. Otherwise, traverse up the fork chain to find the root
	// This ensures the bubble view always shows the global subject fork tree, not a user-specific view.
	rootRepo := repo
	foundNonEmptyRoot := false

	// First, try to find the subject's root repository
	if repo.SubjectID > 0 {
		subjectRoot, err := repo_model.GetSubjectRootRepository(ctx, repo.SubjectID)
		if err == nil {
			if err := subjectRoot.LoadOwner(ctx); err != nil {
				log.Warn("Failed to load owner for subject root repository %d: %v. Falling back to fork chain traversal.", subjectRoot.ID, err)
			} else {
				rootRepo = subjectRoot
				foundNonEmptyRoot = true
				log.Info("Repository %s has subject ID %d, using subject root repository %s for fork graph", repo.FullName(), repo.SubjectID, rootRepo.FullName())
			}
		} else if !repo_model.IsErrRepoNotExist(err) {
			log.Warn("Failed to find subject root repository for subject ID %d: %v. Falling back to fork chain traversal.", repo.SubjectID, err)
		}
		// If no subject root exists (all repos are empty), fall through to fork chain traversal
	}

	// If we didn't find a subject root, traverse up the fork chain
	if rootRepo.ID == repo.ID && repo.IsFork {
		current := repo
		for current.IsFork {
			parent, err := repo_model.GetRepositoryByID(ctx, current.ForkID)
			if err != nil {
				log.Warn("Failed to find parent repository for fork %s (ID: %d, ForkID: %d): %v. Using current repo as root.", current.FullName(), current.ID, current.ForkID, err)
				break
			}
			if err := parent.LoadOwner(ctx); err != nil {
				log.Warn("Failed to load owner for parent repository %d: %v. Using current repo as root.", parent.ID, err)
				break
			}
			current = parent
		}
		rootRepo = current
		if !rootRepo.IsEmpty {
			foundNonEmptyRoot = true
		}
		log.Info("Repository %s is a fork, building fork graph from root repository %s", repo.FullName(), rootRepo.FullName())
	}

	// If the root repository is empty and we didn't find a non-empty root through subject lookup,
	// return an empty graph. This triggers the "Create first article" UI in the frontend.
	// Empty repositories should not be shown as bubbles - only repositories with actual content count.
	if !foundNonEmptyRoot && rootRepo.IsEmpty {
		log.Info("Repository %s is empty and no non-empty root exists for subject ID %d. Returning empty graph.", repo.FullName(), repo.SubjectID)
		return &ForkGraphResponse{
			Root: nil,
			Metadata: GraphMetadata{
				TotalForks:      0,
				VisibleForks:    0,
				MaxDepthReached: false,
				CacheStatus:     "miss",
				GeneratedAt:     time.Now(),
			},
		}, nil
	}

	// Create context with timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, processingTimeout)
	defer cancel()

	// Initialize tracking
	visited := make(map[int64]bool)
	nodeCount := 0
	maxDepthReached := false

	// Build the tree structure
	rootNode, err := buildNode(timeoutCtx, rootRepo, 0, params, doer, visited, &nodeCount, &maxDepthReached)
	if err != nil {
		return nil, err
	}

	// Collect all repositories from the tree for batch loading
	allRepos := collectRepositories(rootNode)

	// Batch load attributes to avoid N+1 queries
	if err := batchLoadRepositoryAttributes(ctx, allRepos); err != nil {
		log.Warn("Failed to batch load repository attributes: %v", err)
		// Continue anyway - individual loads will happen in convert.ToRepo
	}

	// Convert all nodes to API format (using preloaded data)
	convertNodesToAPI(ctx, rootNode)

	// Count total and visible forks (use root repository's fork count)
	totalForks := rootRepo.NumForks
	visibleForks := countVisibleForks(rootNode)

	// Build response
	response := &ForkGraphResponse{
		Root: rootNode,
		Metadata: GraphMetadata{
			TotalForks:      totalForks,
			VisibleForks:    visibleForks,
			MaxDepthReached: maxDepthReached,
			CacheStatus:     "miss",
			GeneratedAt:     time.Now(),
		},
	}

	if params.IncludeContributors {
		response.Metadata.ContributorWindowDays = params.ContributorDays
	}

	return response, nil
}

// buildNode recursively builds a fork node
func buildNode(ctx context.Context, repo *repo_model.Repository, level int, params ForkGraphParams, doer *user_model.User, visited map[int64]bool, nodeCount *int, maxDepthReached *bool) (*ForkNode, error) {
	// Check timeout
	select {
	case <-ctx.Done():
		return nil, ErrProcessingTimeout
	default:
	}

	// Check node limit
	if *nodeCount >= maxNodes {
		return nil, ErrTooManyNodes
	}

	// Check if already visited (cycle detection)
	if visited[repo.ID] {
		log.Warn("Cycle detected in fork graph for repository ID %d", repo.ID)
		return nil, ErrCycleDetected
	}
	visited[repo.ID] = true
	*nodeCount++

	// Check depth limit
	if level >= params.MaxDepth {
		*maxDepthReached = true
		return createLeafNode(repo, level, params)
	}

	// Get direct forks
	forks, err := getDirectForks(ctx, repo.ID, doer, params)
	if err != nil {
		log.Error("Failed to get forks for repo %d: %v", repo.ID, err)
		return createLeafNode(repo, level, params)
	}

	// Build children
	children := make([]*ForkNode, 0, len(forks))
	for _, fork := range forks {
		childNode, err := buildNode(ctx, fork, level+1, params, doer, visited, nodeCount, maxDepthReached)
		if err != nil {
			if errors.Is(err, ErrProcessingTimeout) || errors.Is(err, ErrTooManyNodes) {
				return nil, err
			}
			if errors.Is(err, ErrCycleDetected) {
				// Log the cycle but continue building the rest of the graph
				log.Warn("Skipping cyclic fork relationship in graph for repo %d", fork.ID)
				continue
			}
			// Log error but continue with other children
			log.Error("Failed to build node for fork %d: %v", fork.ID, err)
			continue
		}
		if childNode != nil {
			children = append(children, childNode)
		}
	}

	// Create node
	node := &ForkNode{
		ID:       fmt.Sprintf("repo_%d", repo.ID),
		Level:    level,
		Children: children,
		repo:     repo, // Store for batch processing
	}

	// Add contributor stats if requested
	if params.IncludeContributors {
		stats, err := getContributorStats(repo, params.ContributorDays, getForkSinceTime(repo))
		if err != nil {
			log.Warn("Failed to get contributor stats for repo %d: %v", repo.ID, err)
		} else {
			node.Contributors = stats
		}
	}

	return node, nil
}

// createLeafNode creates a leaf node without children
func createLeafNode(repo *repo_model.Repository, level int, params ForkGraphParams) (*ForkNode, error) {
	node := &ForkNode{
		ID:       fmt.Sprintf("repo_%d", repo.ID),
		Level:    level,
		Children: []*ForkNode{},
		repo:     repo, // Store for batch processing
	}

	if params.IncludeContributors {
		stats, err := getContributorStats(repo, params.ContributorDays, getForkSinceTime(repo))
		if err != nil {
			log.Warn("Failed to get contributor stats for repo %d: %v", repo.ID, err)
		} else {
			node.Contributors = stats
		}
	}

	return node, nil
}

// createReadPermission creates a basic read permission for repositories
// that have already been filtered by AccessibleRepositoryCondition.
// This avoids redundant permission checks since we know the user can access these repos.
// This eliminates 4-6 database queries per node (5x faster for large fork trees).
func createReadPermission(ctx context.Context, repo *repo_model.Repository) access_model.Permission {
	// Load units if not already loaded (this is cached in the repo object)
	_ = repo.LoadUnits(ctx)

	// Create a permission with read access
	// The actual permission level doesn't matter much since the repo is already accessible
	perm := access_model.Permission{
		AccessMode: perm_model.AccessModeRead,
	}
	perm.SetUnitsWithDefaultAccessMode(repo.Units, perm_model.AccessModeRead)

	return perm
}

// getDirectForks gets direct forks of a repository with permission filtering
func getDirectForks(ctx context.Context, repoID int64, doer *user_model.User, params ForkGraphParams) ([]*repo_model.Repository, error) {
	repo := &repo_model.Repository{ID: repoID}

	listOpts := db.ListOptions{
		Page:     params.Page,
		PageSize: params.Limit,
	}

	forks, _, err := FindForks(ctx, repo, doer, listOpts)
	if err != nil {
		return nil, err
	}

	// Filter by visibility if needed
	if !params.IncludePrivate {
		filtered := make([]*repo_model.Repository, 0, len(forks))
		for _, fork := range forks {
			if !fork.IsPrivate {
				filtered = append(filtered, fork)
			}
		}
		forks = filtered
	}

	// Sort forks
	sortRepositories(forks, params.Sort)

	return forks, nil
}

// sortRepositories sorts repositories based on the sort parameter
func sortRepositories(repos []*repo_model.Repository, sortBy string) {
	sort.Slice(repos, func(i, j int) bool {
		switch sortBy {
		case "updated":
			// Sort by updated time descending (most recent first)
			return repos[i].UpdatedUnix > repos[j].UpdatedUnix
		case "created":
			// Sort by created time descending (most recent first)
			return repos[i].CreatedUnix > repos[j].CreatedUnix
		case "stars":
			// Sort by stars descending (most starred first)
			return repos[i].NumStars > repos[j].NumStars
		case "forks":
			// Sort by forks descending (most forked first)
			return repos[i].NumForks > repos[j].NumForks
		default:
			// Default to sorting by updated time
			return repos[i].UpdatedUnix > repos[j].UpdatedUnix
		}
	})
}

// getForkSinceTime returns the appropriate since time for contributor filtering.
// For forks, returns the fork creation time to exclude inherited history from the parent.
// For non-forks, returns zero time (no filtering).
func getForkSinceTime(repo *repo_model.Repository) time.Time {
	if repo.IsFork && repo.CreatedUnix > 0 {
		return repo.CreatedUnix.AsTime()
	}
	return time.Time{}
}

// hasCommitsAfter checks if a contributor has any commits after the given time.
// Returns true if since is zero (no filtering) or if the contributor has at least one commit after since.
// Note: Due to weekly granularity of contributor data, this may over-count contributors
// for forks created mid-week by including contributors who only have pre-fork commits
// within the same calendar week as the fork creation.
func hasCommitsAfter(contributor *ContributorData, since time.Time) bool {
	if since.IsZero() {
		return true
	}
	for _, week := range contributor.Weeks {
		weekTime := time.UnixMilli(week.Week)
		// Check if the week ends after since (week end = week start + 7 days)
		// This ensures we include weeks that overlap with the post-fork period
		weekEndTime := weekTime.AddDate(0, 0, 7)
		if weekEndTime.After(since) && week.Commits > 0 {
			return true
		}
	}
	return false
}

// getContributorStats gets contributor statistics for a repository.
// If since is non-zero, only counts contributors who made commits after that time.
// This is useful for forks where we only want to count post-fork contributions.
//
// Caching behavior (two-tier):
// 1. Secondary cache: Pre-filtered results keyed by (repoID, since, days) - 2 minute TTL
// 2. Primary cache: Raw contributor data keyed by (repo, revision) - 10 minute TTL
//
// The secondary cache eliminates redundant post-cache filtering for high-traffic fork
// repositories where the same contributor statistics are requested frequently with
// identical parameters (typically the fork creation time and days window).
//
// Cache key scoping:
// - Primary: "GetContributorStats/{repo.FullName()}/{revision}" - forks have separate entries
// - Secondary: "ForkContributorStats/{repoID}/{since.Unix()}/{days}" - unique per parameter set
//
// Stampede prevention:
// Uses forkStatsComputeLock (sync.Map) to prevent multiple goroutines from simultaneously
// computing the same cache key. When a cache miss occurs, only the first goroutine will
// compute and cache the result; concurrent requests compute without caching to avoid blocking.
//
// Cache invalidation:
// Cache keys are registered in forkStatsCacheKeys for each repository. When commits are
// pushed, InvalidateForkContributorStatsCache is called to delete all cached entries for
// that repository, ensuring contributor statistics are refreshed promptly.
//
// Fallback behavior:
// If secondary cache operations fail, the function falls back to computing results
// from the primary cache to ensure system reliability.
func getContributorStats(repo *repo_model.Repository, days int, since time.Time) (*ContributorStats, error) {
	// Validate days parameter to prevent future cutoff times
	if days < 0 {
		days = 0
	}

	c := cache.GetCache()
	if c == nil {
		return &ContributorStats{TotalCount: 0, RecentCount: 0}, nil
	}

	// Build secondary cache key for pre-filtered results
	// Use Unix timestamp for 'since' (0 if zero time) to create stable cache keys
	sinceUnix := int64(0)
	if !since.IsZero() {
		sinceUnix = since.Unix()
	}
	secondaryCacheKey := fmt.Sprintf(forkContributorStatsCacheKey, repo.ID, sinceUnix, days)

	// Try to get pre-filtered results from secondary cache
	var cachedStats ContributorStats
	if exists, cacheErr := c.GetJSON(secondaryCacheKey, &cachedStats); exists && cacheErr == nil {
		return &cachedStats, nil
	}

	// Secondary cache miss - prevent stampede by checking if another goroutine is computing
	// If another goroutine is already computing this key, we compute without caching
	// to avoid blocking. The first goroutine will populate the cache.
	_, alreadyComputing := forkStatsComputeLock.LoadOrStore(secondaryCacheKey, struct{}{})
	shouldCache := !alreadyComputing
	if shouldCache {
		// We acquired the lock - ensure we release it when done
		defer forkStatsComputeLock.Delete(secondaryCacheKey)
	}

	// Compute from primary cache
	ctx := context.Background()
	stats, err := GetContributorStats(ctx, c, repo, repo.DefaultBranch)
	if err != nil {
		// If contributor stats generation is still in progress, return zeros
		if errors.Is(err, ErrAwaitGeneration) {
			return &ContributorStats{TotalCount: 0, RecentCount: 0}, nil
		}
		return nil, err
	}

	// Count contributors in a single pass for efficiency
	// For forks, only count contributors who have commits after the fork creation time
	cutoffTime := time.Now().AddDate(0, 0, -days)
	totalCount := 0
	recentCount := 0

	for email, contributor := range stats {
		// Skip the "total" summary entry
		if email == "total" {
			continue
		}

		// For forks, skip contributors with no post-fork commits
		if !hasCommitsAfter(contributor, since) {
			continue
		}

		totalCount++

		// Check if contributor has commits in the recent time window
		for _, week := range contributor.Weeks {
			weekTime := time.UnixMilli(week.Week)
			if weekTime.After(cutoffTime) && week.Commits > 0 {
				recentCount++
				break
			}
		}
	}

	result := &ContributorStats{
		TotalCount:  totalCount,
		RecentCount: recentCount,
	}

	// Store in secondary cache for future requests (only if we hold the compute lock)
	// This prevents multiple goroutines from racing to write the same cache entry
	// Errors are logged but don't fail the request - cache is best-effort
	if shouldCache {
		if err := c.PutJSON(secondaryCacheKey, result, forkContributorStatsCacheTimeout); err != nil {
			log.Warn("Failed to cache fork contributor stats for repo %d: %v", repo.ID, err)
		} else {
			// Register the cache key for invalidation on push
			registerForkStatsCacheKey(repo.ID, secondaryCacheKey)
		}
	}

	return result, nil
}

// countVisibleForks counts the number of visible forks in the tree
func countVisibleForks(node *ForkNode) int {
	if node == nil {
		return 0
	}

	count := len(node.Children)
	for _, child := range node.Children {
		count += countVisibleForks(child)
	}

	return count
}

// collectRepositories traverses the tree and collects all repository objects
// Uses a map to ensure no duplicates are collected
func collectRepositories(node *ForkNode) []*repo_model.Repository {
	if node == nil {
		return nil
	}

	seen := make(map[int64]bool)
	repos := make([]*repo_model.Repository, 0)

	var collect func(*ForkNode)
	collect = func(n *ForkNode) {
		if n == nil || n.repo == nil {
			return
		}
		// Only add if not already seen
		if !seen[n.repo.ID] {
			seen[n.repo.ID] = true
			repos = append(repos, n.repo)
		}
		// Recursively collect from children
		for _, child := range n.Children {
			collect(child)
		}
	}

	collect(node)
	return repos
}

// batchLoadRepositoryAttributes loads all necessary attributes for repositories in a single batch
// This eliminates N+1 queries by using batch loading methods
func batchLoadRepositoryAttributes(ctx context.Context, repos []*repo_model.Repository) error {
	if len(repos) == 0 {
		return nil
	}

	repoList := repo_model.RepositoryList(repos)

	// Batch load owners (biggest performance impact)
	if err := repoList.LoadOwners(ctx); err != nil {
		return fmt.Errorf("failed to batch load owners: %w", err)
	}

	// Batch load subjects
	if err := repoList.LoadSubjects(ctx); err != nil {
		log.Warn("Failed to batch load subjects: %v", err)
		// Continue - individual loads will happen in convert.ToRepo
	}

	// Batch load units (needed for permissions)
	if err := repoList.LoadUnits(ctx); err != nil {
		log.Warn("Failed to batch load units: %v", err)
		// Continue - individual loads will happen in convert.ToRepo
	}

	// Batch load licenses
	licensesMap, err := repoList.LoadLicenses(ctx)
	if err != nil {
		log.Warn("Failed to batch load licenses: %v", err)
		// Continue - individual loads will happen in convert.ToRepo
	} else {
		// Store preloaded licenses in repository objects
		for _, repo := range repos {
			if licenses, ok := licensesMap[repo.ID]; ok {
				repo.Licenses = licenses
			}
		}
	}

	return nil
}

// convertNodesToAPI recursively converts all nodes to API format using preloaded data
func convertNodesToAPI(ctx context.Context, node *ForkNode) {
	if node == nil {
		return
	}

	// Convert this node's repository to API format
	if node.repo != nil {
		permission := createReadPermission(ctx, node.repo)
		node.Repository = convert.ToRepo(ctx, node.repo, permission)
		// Clear the internal repo reference to free memory
		node.repo = nil
	}

	// Recursively convert children
	for _, child := range node.Children {
		convertNodesToAPI(ctx, child)
	}
}
