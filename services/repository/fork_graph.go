// Copyright 2025 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"errors"
	"fmt"
	"sort"
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
		// For forks, only count contributors who made commits after the fork was created
		// to exclude inherited history from the parent repository
		var since time.Time
		if repo.IsFork {
			since = repo.CreatedUnix.AsTime()
		}
		stats, err := getContributorStats(repo, params.ContributorDays, since)
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
		// For forks, only count contributors who made commits after the fork was created
		// to exclude inherited history from the parent repository
		var since time.Time
		if repo.IsFork {
			since = repo.CreatedUnix.AsTime()
		}
		stats, err := getContributorStats(repo, params.ContributorDays, since)
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
func getContributorStats(repo *repo_model.Repository, days int, since time.Time) (*ContributorStats, error) {
	// Use existing contributor stats service
	c := cache.GetCache()
	if c == nil {
		return &ContributorStats{TotalCount: 0, RecentCount: 0}, nil
	}

	// Call GetContributorStats which handles cache and generation
	// This function will generate stats if not cached, or return cached stats if available
	ctx := context.Background()
	stats, err := GetContributorStats(ctx, c, repo, repo.DefaultBranch)
	if err != nil {
		// If contributor stats generation is still in progress, return zeros
		if errors.Is(err, ErrAwaitGeneration) {
			return &ContributorStats{TotalCount: 0, RecentCount: 0}, nil
		}
		return nil, err
	}

	// Count total contributors (exclude "total" summary entry)
	// For forks, only count contributors who have commits after the fork creation time
	totalCount := 0
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
	}

	// Count recent contributors (within the specified days window)
	cutoffTime := time.Now().AddDate(0, 0, -days)
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

		// Check if contributor has commits in the recent time window
		for _, week := range contributor.Weeks {
			weekTime := time.UnixMilli(week.Week)
			if weekTime.After(cutoffTime) && week.Commits > 0 {
				recentCount++
				break
			}
		}
	}

	return &ContributorStats{
		TotalCount:  totalCount,
		RecentCount: recentCount,
	}, nil
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
