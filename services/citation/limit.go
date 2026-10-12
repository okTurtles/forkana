// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package citation

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
)

// A lookup makes the server fetch a page the user names: each user may only make a few per
// minute, and the details found are cached, so citing the same source again (or another
// writer citing it) fetches nothing.

// ErrRateLimited is returned when a user made too many lookups in the last minute.
var ErrRateLimited = errors.New("too many citation lookups, try again in a minute")

const (
	lookupsPerMinute = 20
	cacheSize        = 512
	cacheTTL         = 6 * time.Hour
)

var (
	cache = expirable.NewLRU[string, *Source](cacheSize, nil, cacheTTL)

	limitMu     sync.Mutex
	limitWindow time.Time
	limitCounts = map[int64]int{}
)

// allowLookup counts a lookup of the user in the current minute and reports whether the user
// is under the limit.
func allowLookup(userID int64, now time.Time) bool {
	limitMu.Lock()
	defer limitMu.Unlock()
	if now.Sub(limitWindow) >= time.Minute {
		limitWindow = now
		clear(limitCounts)
	}
	if limitCounts[userID] >= lookupsPerMinute {
		return false
	}
	limitCounts[userID]++
	return true
}

// cacheKey is the query in a canonical form: the DOI, the ISBN or the URL.
func cacheKey(query string) string {
	query = strings.TrimSpace(query)
	if doi := NormalizeDOI(query); doi != "" {
		return "doi:" + strings.ToLower(doi)
	}
	if isbn := NormalizeISBN(query); isbn != "" {
		return "isbn:" + isbn
	}
	return "url:" + query
}

// LookupFor is Lookup for a user: answered from the cache when possible, and rate-limited.
func LookupFor(ctx context.Context, userID int64, query string) (*Source, error) {
	key := cacheKey(query)
	if src, ok := cache.Get(key); ok {
		return src, nil
	}
	if !allowLookup(userID, time.Now()) {
		return nil, ErrRateLimited
	}
	src, err := Lookup(ctx, query)
	if err != nil {
		return nil, err
	}
	cache.Add(key, src)
	return src, nil
}
