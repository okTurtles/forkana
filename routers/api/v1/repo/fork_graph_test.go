// Copyright 2025 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"net/http"
	"net/url"
	"testing"

	"code.gitea.io/gitea/models/unittest"
	"code.gitea.io/gitea/services/contexttest"
	repo_service "code.gitea.io/gitea/services/repository"

	"github.com/stretchr/testify/assert"
)

func TestAPIForkGraph(t *testing.T) {
	unittest.PrepareTestEnv(t)

	ctx, _ := contexttest.MockAPIContext(t, "GET /user2/repo1/forks/graph")
	contexttest.LoadRepo(t, ctx, 1)
	contexttest.LoadUser(t, ctx, 2)

	GetForkGraph(ctx)

	assert.Equal(t, http.StatusOK, ctx.Resp.WrittenStatus())
}

func TestAPIForkGraphWithContributors(t *testing.T) {
	unittest.PrepareTestEnv(t)

	ctx, _ := contexttest.MockAPIContext(t, "GET /user2/repo1/forks/graph?include_contributors=true&contributor_days=30")
	contexttest.LoadRepo(t, ctx, 1)
	contexttest.LoadUser(t, ctx, 2)

	GetForkGraph(ctx)

	assert.Equal(t, http.StatusOK, ctx.Resp.WrittenStatus())
}

func TestAPIForkGraphInvalidParams(t *testing.T) {
	unittest.PrepareTestEnv(t)

	// Test with invalid max_depth
	ctx, _ := contexttest.MockAPIContext(t, "GET /user2/repo1/forks/graph?max_depth=100")
	contexttest.LoadRepo(t, ctx, 1)
	contexttest.LoadUser(t, ctx, 2)

	GetForkGraph(ctx)
	assert.Equal(t, http.StatusBadRequest, ctx.Resp.WrittenStatus())

	// Test with invalid contributor_days
	ctx, _ = contexttest.MockAPIContext(t, "GET /user2/repo1/forks/graph?contributor_days=500")
	contexttest.LoadRepo(t, ctx, 1)
	contexttest.LoadUser(t, ctx, 2)

	GetForkGraph(ctx)
	assert.Equal(t, http.StatusBadRequest, ctx.Resp.WrittenStatus())

	// Test with invalid sort
	ctx, _ = contexttest.MockAPIContext(t, "GET /user2/repo1/forks/graph?sort=invalid")
	contexttest.LoadRepo(t, ctx, 1)
	contexttest.LoadUser(t, ctx, 2)

	GetForkGraph(ctx)
	assert.Equal(t, http.StatusBadRequest, ctx.Resp.WrittenStatus())
}

func TestAPIForkGraphPermissions(t *testing.T) {
	unittest.PrepareTestEnv(t)

	// Test accessing repository without authentication (public repo should work)
	ctx, _ := contexttest.MockAPIContext(t, "GET /user2/repo1/forks/graph")
	contexttest.LoadRepo(t, ctx, 1)
	// Don't load user - simulating unauthenticated access
	ctx.Doer = nil

	GetForkGraph(ctx)
	// Public repo should be accessible without auth
	assert.Equal(t, http.StatusOK, ctx.Resp.WrittenStatus())
}

func TestForkGraphParamsValidation(t *testing.T) {
	tests := []struct {
		name    string
		params  ForkGraphParams
		wantErr bool
	}{
		{
			name: "valid params",
			params: ForkGraphParams{
				ContributorDays: 90,
				MaxDepth:        10,
				Sort:            "updated",
				Page:            1,
				Limit:           50,
			},
			wantErr: false,
		},
		{
			name: "invalid contributor_days too low",
			params: ForkGraphParams{
				ContributorDays: 0,
				MaxDepth:        10,
				Sort:            "updated",
				Page:            1,
				Limit:           50,
			},
			wantErr: true,
		},
		{
			name: "invalid contributor_days too high",
			params: ForkGraphParams{
				ContributorDays: 400,
				MaxDepth:        10,
				Sort:            "updated",
				Page:            1,
				Limit:           50,
			},
			wantErr: true,
		},
		{
			name: "invalid max_depth",
			params: ForkGraphParams{
				ContributorDays: 90,
				MaxDepth:        25,
				Sort:            "updated",
				Page:            1,
				Limit:           50,
			},
			wantErr: true,
		},
		{
			name: "invalid sort",
			params: ForkGraphParams{
				ContributorDays: 90,
				MaxDepth:        10,
				Sort:            "invalid",
				Page:            1,
				Limit:           50,
			},
			wantErr: true,
		},
		{
			name: "invalid limit",
			params: ForkGraphParams{
				ContributorDays: 90,
				MaxDepth:        10,
				Sort:            "updated",
				Page:            1,
				Limit:           150,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.params.validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestForkGraphCacheKey(t *testing.T) {
	params1 := ForkGraphParams{
		IncludeContributors: true,
		ContributorDays:     90,
		MaxDepth:            10,
		Sort:                "updated",
		Page:                1,
		Limit:               50,
	}

	params2 := ForkGraphParams{
		IncludeContributors: true,
		ContributorDays:     90,
		MaxDepth:            10,
		Sort:                "updated",
		Page:                1,
		Limit:               50,
	}

	params3 := ForkGraphParams{
		IncludeContributors: true,
		ContributorDays:     90,
		MaxDepth:            10,
		IncludePrivate:      true,
		Sort:                "updated",
		Page:                1,
		Limit:               50,
	}

	// contributor_days does not change the graph, and only graphs without contributors
	// are cached, so neither field is part of the key
	params4 := params1
	params4.ContributorDays = 30
	params4.IncludeContributors = false
	assert.Equal(t, getCacheKey(1, false, 0, params1, 1, false), getCacheKey(1, false, 0, params4, 1, false))

	key1 := getCacheKey(1, false, 0, params1, 1, false)
	key2 := getCacheKey(1, false, 0, params2, 1, false)
	key3 := getCacheKey(1, false, 0, params3, 1, false)

	// Same params should generate same key
	assert.Equal(t, key1, key2)

	// Different params should generate different key
	assert.NotEqual(t, key1, key3)

	// nor may a graph cached while its root was public be served once it is private
	assert.NotEqual(t, key1, getCacheKey(1, false, 0, params1, 1, true))
}

func TestForkGraphCacheKeyIncludesVersion(t *testing.T) {
	params := ForkGraphParams{
		IncludeContributors: true,
		ContributorDays:     90,
		MaxDepth:            10,
		Sort:                "updated",
		Page:                1,
		Limit:               50,
	}

	key := getCacheKey(1, false, 0, params, 1, false)

	// Verify cache key includes the version
	assert.Contains(t, key, forkGraphCacheVersion, "Cache key should include version for cache invalidation")

	// Verify cache key format: fork_graph:{version}:{repoID}:{isEmpty}:{numForks}:{paramsHash}:{userID}
	assert.Contains(t, key, "fork_graph:"+forkGraphCacheVersion+":", "Cache key should start with fork_graph:{version}:")
}

// The subject page embeds the graph it builds with SubjectForkGraphParams and points the
// Bubble view's API fallback at SubjectForkGraphQuery: read by the API, that query must
// give back the very same parameters, or a retry would draw a different graph.
func TestSubjectForkGraphQueryContract(t *testing.T) {
	query, err := url.ParseQuery(repo_service.SubjectForkGraphQuery())
	assert.NoError(t, err)
	params, err := parseForkGraphParams(query)
	assert.NoError(t, err)
	assert.Equal(t, repo_service.SubjectForkGraphParams(), params.serviceParams())
}

func TestParseForkGraphParams(t *testing.T) {
	params, err := parseForkGraphParams(url.Values{})
	assert.NoError(t, err)
	assert.Equal(t, ForkGraphParams{ContributorDays: 90, MaxDepth: 10, Sort: "updated", Page: 1, Limit: 50}, params)

	_, err = parseForkGraphParams(url.Values{"limit": {"abc"}})
	assert.Error(t, err)

	// "on", as an HTML checkbox sends it, is true, as ctx.FormBool reads it
	params, err = parseForkGraphParams(url.Values{"include_contributors": {"on"}, "include_private": {"true"}})
	assert.NoError(t, err)
	assert.True(t, params.IncludeContributors)
	assert.True(t, params.IncludePrivate)
}
