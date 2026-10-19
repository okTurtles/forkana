// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package citation

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	"golang.org/x/sync/singleflight"
	"golang.org/x/time/rate"
)

// A lookup makes the server fetch a page the user names: each user may only make a few per
// minute, and what a lookup found (or that it found nothing) is cached, so citing the same
// source again (or another writer citing it) fetches nothing. Concurrent lookups of the
// same source share one fetch.

// ErrRateLimited is returned when a user made too many lookups in the last minute.
var ErrRateLimited = errors.New("too many citation lookups, try again in a minute")

const (
	lookupsPerMinute = 20
	cacheSize        = 512
	cacheTTL         = 6 * time.Hour
	failureTTL       = 5 * time.Minute
	limiterCacheSize = 10000
)

var (
	cache    = expirable.NewLRU[string, *Source](cacheSize, nil, cacheTTL)
	failures = expirable.NewLRU[string, error](cacheSize, nil, failureTTL)
	lookups  singleflight.Group

	// limiters holds a token bucket per user: lookupsPerMinute at once, then one every
	// 60/lookupsPerMinute seconds. Each use keeps the bucket a minute longer; a bucket unused
	// for a minute is full again, so it may be dropped then (or earlier, when too many users
	// look up at once).
	limitersMu sync.Mutex
	limiters   = expirable.NewLRU[int64, *rate.Limiter](limiterCacheSize, nil, time.Minute)
)

// allowLookup takes a lookup from the user's bucket and reports whether there was one left.
func allowLookup(userID int64, now time.Time) bool {
	limitersMu.Lock()
	defer limitersMu.Unlock()
	limiter, ok := limiters.Get(userID)
	if !ok {
		limiter = rate.NewLimiter(rate.Every(time.Minute/lookupsPerMinute), lookupsPerMinute)
	}
	limiters.Add(userID, limiter) // (again): expires a minute after its last use
	return limiter.AllowN(now, 1)
}

// copySource returns a copy of a cached source, which callers may then change.
func copySource(src *Source) *Source {
	c := *src
	c.Authors = slices.Clone(src.Authors)
	return &c
}

// LookupFor is Lookup for a user: answered from the cache when possible, and rate-limited
// (a query that is not a URL, DOI or ISBN fetches nothing, so it is not counted).
func LookupFor(ctx context.Context, userID int64, raw string) (*Source, error) {
	q, err := parseQuery(raw)
	if err != nil {
		return nil, err
	}
	key := q.cacheKey()
	if src, ok := cache.Get(key); ok {
		return copySource(src), nil
	}
	if err, ok := failures.Get(key); ok {
		return nil, err
	}
	if !allowLookup(userID, time.Now()) {
		return nil, ErrRateLimited
	}
	v, err, _ := lookups.Do(key, func() (any, error) {
		// the fetch is shared: it must not stop when the first caller goes away
		src, err := q.lookup(context.WithoutCancel(ctx))
		if err != nil {
			failures.Add(key, err)
			return nil, err
		}
		cache.Add(key, src)
		return src, nil
	})
	if err != nil {
		return nil, err
	}
	return copySource(v.(*Source)), nil
}
