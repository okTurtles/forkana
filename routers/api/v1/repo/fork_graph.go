// Copyright 2025 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"code.gitea.io/gitea/modules/cache"
	"code.gitea.io/gitea/services/context"
	"code.gitea.io/gitea/services/repository"
)

// forkGraphCacheVersion should be incremented whenever the fork graph
// logic, data structure, or API response format changes.
// This allows automatic cache invalidation across deployments.
//
// Version History:
//   - v1: Initial implementation with basic fork graph traversal
//   - v2: Added cycle detection error handling (ErrCycleDetected)
//   - v3: Changed GetPublicRepositoryBySubject to prioritize non-empty repositories
//   - v4: Contributor counts come from ArticleContributorCount and are no longer cached in
//     the response (graphs with include_contributors are never cached); recent_count is 0
const forkGraphCacheVersion = "v4"

// ForkGraphParams represents the query parameters for fork graph endpoint. The subject
// page requests the graph it embeds with the same names (services/repository
// SubjectForkGraphQuery); TestSubjectForkGraphQueryContract keeps the two in step.
type ForkGraphParams struct {
	IncludeContributors bool   `form:"include_contributors"`
	ContributorDays     int    `form:"contributor_days"`
	MaxDepth            int    `form:"max_depth"`
	IncludePrivate      bool   `form:"include_private"`
	Sort                string `form:"sort"`
	Page                int    `form:"page"`
	Limit               int    `form:"limit"`
}

// setDefaults sets default values for parameters
func (p *ForkGraphParams) setDefaults() {
	if p.ContributorDays == 0 {
		p.ContributorDays = 90
	}
	if p.MaxDepth == 0 {
		p.MaxDepth = 10
	}
	if p.Sort == "" {
		p.Sort = "updated"
	}
	if p.Page == 0 {
		p.Page = 1
	}
	if p.Limit == 0 {
		p.Limit = 50
	}
}

// validate validates the parameters
func (p *ForkGraphParams) validate() error {
	if p.ContributorDays < 1 || p.ContributorDays > 365 {
		return errors.New("contributor_days must be between 1 and 365")
	}
	if p.MaxDepth < 1 || p.MaxDepth > 20 {
		return errors.New("max_depth must be between 1 and 20")
	}
	if p.Limit < 1 || p.Limit > 100 {
		return errors.New("limit must be between 1 and 100")
	}
	if p.Page < 1 {
		return errors.New("page must be at least 1")
	}
	validSorts := map[string]bool{"updated": true, "created": true, "stars": true, "forks": true}
	if !validSorts[p.Sort] {
		return errors.New("sort must be one of: updated, created, stars, forks")
	}
	return nil
}

// getCacheKey generates a versioned cache key for fork graph data.
// The key includes:
// - forkGraphCacheVersion: Incremented when logic changes (for cache invalidation)
// - repoID: The repository being queried
// - isEmpty: Whether the repository is empty (changes when first content is added)
// - numForks: Number of forks (changes when forks are created, invalidating cache)
// - paramsHash: Hash of query parameters (depth, filters, etc.)
// - userID: User-specific permissions may affect the graph
func getCacheKey(repoID int64, isEmpty bool, numForks int, params ForkGraphParams, userID int64) string {
	paramsHash := hashParams(params)
	emptyStr := "0"
	if isEmpty {
		emptyStr = "1"
	}
	return fmt.Sprintf("fork_graph:%s:%d:%s:%d:%s:%d",
		forkGraphCacheVersion, repoID, emptyStr, numForks, paramsHash, userID)
}

// hashParams creates a hash of the parameters
// Only a graph without contributors is cached, and contributor_days does not change
// the graph, so neither is part of the hash.
func hashParams(params ForkGraphParams) string {
	data := fmt.Sprintf("%d:%t:%s:%d:%d",
		params.MaxDepth, params.IncludePrivate, params.Sort, params.Page, params.Limit)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:8]) // First 8 bytes for brevity
}

// getCacheTTL returns the cache TTL based on repository and parameters
func getCacheTTL(isPrivate bool) time.Duration {
	if isPrivate {
		return 5 * time.Minute
	}
	return 30 * time.Minute
}

// GetForkGraph returns the fork graph for a repository
func GetForkGraph(ctx *context.APIContext) {
	// swagger:operation GET /repos/{owner}/{repo}/forks/graph repository getForkGraph
	// ---
	// summary: Get repository fork graph
	// description: Returns a hierarchical tree structure of all forks with optional contributor statistics
	// produces:
	// - application/json
	// parameters:
	// - name: owner
	//   in: path
	//   description: owner of the repo
	//   type: string
	//   required: true
	// - name: repo
	//   in: path
	//   description: name of the repo
	//   type: string
	//   required: true
	// - name: include_contributors
	//   in: query
	//   description: Include contributor count for each fork
	//   type: boolean
	//   default: false
	// - name: contributor_days
	//   in: query
	//   description: "Accepted and validated (1-365) for compatibility and echoed as metadata.contributor_window_days; it no longer affects the counts"
	//   type: integer
	//   default: 90
	// - name: max_depth
	//   in: query
	//   description: Maximum depth of fork tree traversal (1-20)
	//   type: integer
	//   default: 10
	// - name: include_private
	//   in: query
	//   description: Include private forks (requires appropriate permissions)
	//   type: boolean
	//   default: false
	// - name: sort
	//   in: query
	//   description: Sort order for child nodes (updated, created, stars, forks)
	//   type: string
	//   default: updated
	// - name: page
	//   in: query
	//   description: Page number for pagination
	//   type: integer
	//   default: 1
	// - name: limit
	//   in: query
	//   description: Number of forks per level per page (1-100)
	//   type: integer
	//   default: 50
	// responses:
	//   "200":
	//     "$ref": "#/responses/ForkGraph"
	//   "400":
	//     "$ref": "#/responses/error"
	//   "403":
	//     "$ref": "#/responses/forbidden"
	//   "404":
	//     "$ref": "#/responses/notFound"

	params, err := parseForkGraphParams(ctx.Req.URL.Query())
	if err != nil {
		ctx.APIError(http.StatusBadRequest, err)
		return
	}

	// Check repository access
	if !ctx.Repo.Permission.HasAnyUnitAccessOrPublicAccess() {
		ctx.APIErrorNotFound()
		return
	}

	// Get user ID for cache key
	var userID int64
	if ctx.Doer != nil {
		userID = ctx.Doer.ID
	}

	// Try cache first. A graph with contributor counts is never served from the response
	// cache: its key only changes with the root's own fork count, so it kept serving the
	// counts (and the forks of forks) of up to 15 minutes ago while the table and the
	// article view, which are rendered live, already showed the new ones (#405). Each
	// node's count is cached by its branch head instead (ArticleContributorCount), so a
	// fresh graph stays cheap and is never stale. The trade-off: every such request
	// rebuilds the graph, a few database queries per level, while the counts themselves
	// come from that cache (heads read in one query) under one bounded budget.
	cacheGraph := !params.IncludeContributors
	cacheKey := ""
	c := cache.GetCache()
	if cacheGraph && c != nil {
		cacheKey = getCacheKey(ctx.Repo.Repository.ID, ctx.Repo.Repository.IsEmpty, ctx.Repo.Repository.NumForks, params, userID)
		var cachedResponse repository.ForkGraphResponse
		found, err := c.GetJSON(cacheKey, &cachedResponse)
		if err == nil && found {
			cachedResponse.Metadata.CacheStatus = "hit"
			ctx.JSON(http.StatusOK, cachedResponse)
			return
		}
	}

	// Generate graph
	graph, err := repository.BuildForkGraph(ctx, ctx.Repo.Repository, params.serviceParams(), ctx.Doer)
	if err != nil {
		handleForkGraphError(ctx, err)
		return
	}

	// Set cache status
	graph.Metadata.CacheStatus = "miss"

	// Cache result
	if cacheKey != "" {
		ttl := getCacheTTL(ctx.Repo.Repository.IsPrivate)
		_ = c.PutJSON(cacheKey, graph, int64(ttl.Seconds()))
	}

	ctx.JSON(http.StatusOK, graph)
}

// parseForkGraphParams reads the endpoint's query parameters, with their defaults, and
// validates them. A parameter that does not parse counts as given with its zero value
// (as ctx.FormBool and ctx.FormInt read it), which validation then refuses.
func parseForkGraphParams(query url.Values) (ForkGraphParams, error) {
	parseBool := func(name string) bool {
		s := query.Get(name)
		v, _ := strconv.ParseBool(s)
		return v || strings.EqualFold(s, "on") // as ctx.FormBool reads it
	}
	parseInt := func(name string, def int) int {
		if query.Get(name) == "" {
			return def
		}
		v, _ := strconv.Atoi(query.Get(name))
		return v
	}
	params := ForkGraphParams{
		IncludeContributors: parseBool("include_contributors"),
		ContributorDays:     parseInt("contributor_days", 90),
		MaxDepth:            parseInt("max_depth", 10),
		IncludePrivate:      parseBool("include_private"),
		Sort:                query.Get("sort"),
		Page:                parseInt("page", 1),
		Limit:               parseInt("limit", 50),
	}
	if params.Sort == "" {
		params.Sort = "updated"
	}
	return params, params.validate()
}

// serviceParams converts the endpoint's parameters for BuildForkGraph.
func (p ForkGraphParams) serviceParams() repository.ForkGraphParams {
	return repository.ForkGraphParams{
		IncludeContributors: p.IncludeContributors,
		ContributorDays:     p.ContributorDays,
		MaxDepth:            p.MaxDepth,
		IncludePrivate:      p.IncludePrivate,
		Sort:                p.Sort,
		Page:                p.Page,
		Limit:               p.Limit,
	}
}

// handleForkGraphError handles errors from fork graph generation
func handleForkGraphError(ctx *context.APIContext, err error) {
	switch {
	case repository.IsErrMaxDepthExceeded(err):
		ctx.APIError(http.StatusBadRequest, err)
	case repository.IsErrTooManyNodes(err):
		ctx.APIError(http.StatusBadRequest, err)
	case repository.IsErrProcessingTimeout(err):
		ctx.APIError(http.StatusRequestTimeout, err)
	default:
		ctx.APIErrorInternal(err)
	}
}
