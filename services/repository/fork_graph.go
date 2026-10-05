// Copyright 2025 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"time"

	"code.gitea.io/gitea/models/db"
	perm_model "code.gitea.io/gitea/models/perm"
	access_model "code.gitea.io/gitea/models/perm/access"
	repo_model "code.gitea.io/gitea/models/repo"
	user_model "code.gitea.io/gitea/models/user"
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

// SubjectForkGraphParams are the parameters the subject page builds its fork graph
// with: the graph is embedded in the page and drawn by the Bubble view, the Table view
// is built from it on the server, and it decides the Article view's article, so all
// three always list the same articles (#405). The Bubble view's API fallback (used for
// retries) requests the same graph: custom/templates/shared/repo/bubble.tmpl builds its
// url from SubjectForkGraphQuery, which is derived from these.
func SubjectForkGraphParams() ForkGraphParams {
	return ForkGraphParams{
		IncludeContributors: true,
		ContributorDays:     90,
		MaxDepth:            10,
		Sort:                "updated",
		Page:                1,
		Limit:               50,
	}
}

// SubjectForkGraphQuery is SubjectForkGraphParams as the fork-graph API's query string.
func SubjectForkGraphQuery() string {
	p := SubjectForkGraphParams()
	q := url.Values{}
	q.Set("include_contributors", strconv.FormatBool(p.IncludeContributors))
	q.Set("contributor_days", strconv.Itoa(p.ContributorDays))
	q.Set("max_depth", strconv.Itoa(p.MaxDepth))
	q.Set("sort", p.Sort)
	q.Set("page", strconv.Itoa(p.Page))
	q.Set("limit", strconv.Itoa(p.Limit))
	return q.Encode()
}

// ForkGraphEntry is one article of a fork graph, as a flat list entry.
type ForkGraphEntry struct {
	Repo *repo_model.Repository
	// ContributorCount is -1 when the count could not be computed.
	ContributorCount int64
}

// FlattenForkGraph lists the articles of a fork graph in depth-first order: the root,
// then each fork followed by its own forks, in the graph's sort order.
func FlattenForkGraph(root *ForkNode) []*ForkGraphEntry {
	var entries []*ForkGraphEntry
	var visit func(*ForkNode)
	visit = func(n *ForkNode) {
		if n == nil || n.repo == nil {
			return
		}
		count := int64(-1)
		if n.Contributors != nil {
			count = int64(n.Contributors.TotalCount)
		}
		entries = append(entries, &ForkGraphEntry{Repo: n.repo, ContributorCount: count})
		for _, child := range n.Children {
			visit(child)
		}
	}
	visit(root)
	return entries
}

// ForkGraphResponse represents the complete fork graph response
type ForkGraphResponse struct {
	Root       *ForkNode       `json:"root"`
	Metadata   GraphMetadata   `json:"metadata"`
	Pagination *PaginationInfo `json:"pagination,omitempty"`

	// articles is the graph as a flat list (see Articles); never serialized
	articles []*ForkGraphEntry
}

// Articles lists the articles of the graph in depth-first order (see FlattenForkGraph).
// Only a freshly built graph has them: the nodes give their repositories up when they
// are converted to the API format, and a response read back from a cache has none.
func (r *ForkGraphResponse) Articles() []*ForkGraphEntry {
	return r.articles
}

// ForkNode represents a node in the fork tree
type ForkNode struct {
	ID           string            `json:"id"`
	Repository   *api.Repository   `json:"repository"`
	Contributors *ContributorStats `json:"contributors,omitempty"`
	Level        int               `json:"level"`
	Children     []*ForkNode       `json:"children"`
	// IsTombstoned marks an article that its author deleted. The node stays in the
	// graph because its descendants need the ancestry, but the article itself is no
	// longer readable, so the client must not present it as a live one.
	IsTombstoned bool `json:"is_tombstoned"`

	// Internal field for batch processing (not exported to JSON)
	repo *repo_model.Repository `json:"-"`
}

// ContributorStats represents contributor statistics
type ContributorStats struct {
	// TotalCount is the article's contributor count: the distinct authors on its default
	// branch, counted from the fork's creation for a fork. It is the count the subject's
	// Bubble, Table and Article views all show.
	TotalCount int `json:"total_count"`
	// RecentCount is no longer computed and is always 0. It is kept so the response
	// keeps its shape for existing API clients.
	RecentCount int `json:"recent_count"`
}

// GraphMetadata represents metadata about the fork graph
type GraphMetadata struct {
	TotalForks      int       `json:"total_forks"`
	VisibleForks    int       `json:"visible_forks"`
	MaxDepthReached bool      `json:"max_depth_reached"`
	CacheStatus     string    `json:"cache_status"`
	GeneratedAt     time.Time `json:"generated_at"`
	// ContributorWindowDays echoes the request's contributor_days parameter. It no
	// longer windows the counts: total_count is measured from the fork's creation (see
	// ContributorStats). Kept so the response keeps its shape.
	ContributorWindowDays int `json:"contributor_window_days,omitempty"`
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

	// The flat list of the articles, taken while the nodes still hold their repositories
	articles := FlattenForkGraph(rootNode)

	// Convert all nodes to API format (using preloaded data)
	convertNodesToAPI(ctx, rootNode)

	// Count total and visible forks (use root repository's fork count)
	totalForks := rootRepo.NumForks
	visibleForks := countVisibleForks(rootNode)

	// Build response
	response := &ForkGraphResponse{
		Root:     rootNode,
		articles: articles,
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
		return createLeafNode(ctx, repo, level, params), nil
	}

	// Get direct forks
	forks, err := getDirectForks(ctx, repo.ID, doer, params)
	if err != nil {
		log.Error("Failed to get forks for repo %d: %v", repo.ID, err)
		return createLeafNode(ctx, repo, level, params), nil
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

	return newForkNode(ctx, repo, level, children, params), nil
}

// newForkNode builds the node of repo with its children, and its contributor stats when
// they are requested.
func newForkNode(ctx context.Context, repo *repo_model.Repository, level int, children []*ForkNode, params ForkGraphParams) *ForkNode {
	node := &ForkNode{
		ID:       fmt.Sprintf("repo_%d", repo.ID),
		Level:    level,
		Children: children,
		repo:     repo, // Store for batch processing
	}
	if params.IncludeContributors {
		node.Contributors = nodeContributorStats(ctx, repo)
	}
	return node
}

// createLeafNode creates a leaf node without children
func createLeafNode(ctx context.Context, repo *repo_model.Repository, level int, params ForkGraphParams) *ForkNode {
	return newForkNode(ctx, repo, level, []*ForkNode{}, params)
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
		node.IsTombstoned = node.repo.IsTombstone()
		// Clear the internal repo reference to free memory
		node.repo = nil
	}

	// Recursively convert children
	for _, child := range node.Children {
		convertNodesToAPI(ctx, child)
	}
}
