// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package explore

import (
	"testing"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/modules/timeutil"
	repo_service "code.gitea.io/gitea/services/repository"

	"github.com/stretchr/testify/assert"
)

func TestSortHistoryTableEntries(t *testing.T) {
	entry := func(name string, count int64, updated timeutil.TimeStamp) *historyTableEntry {
		return &historyTableEntry{Repo: &repo_model.Repository{Name: name}, ContributorCount: count, Updated: updated}
	}
	names := func(entries []*historyTableEntry) []string {
		out := make([]string, 0, len(entries))
		for _, e := range entries {
			out = append(out, e.Repo.Name)
		}
		return out
	}
	rows := func() []*historyTableEntry {
		// the graph's own order
		return []*historyTableEntry{entry("a", 3, 10), entry("b", -1, 40), entry("c", 7, 20), entry("d", 3, 30)}
	}

	most := rows()
	sortHistoryTableEntries(most, "most_contrib")
	assert.Equal(t, []string{"c", "a", "d", "b"}, names(most), "most first, ties in graph order, unknown last")

	least := rows()
	sortHistoryTableEntries(least, "least_contrib")
	assert.Equal(t, []string{"a", "d", "c", "b"}, names(least), "least first, unknown still last")

	latest := rows()
	sortHistoryTableEntries(latest, "latest")
	assert.Equal(t, []string{"b", "d", "c", "a"}, names(latest))

	unsorted := rows()
	sortHistoryTableEntries(unsorted, "")
	assert.Equal(t, []string{"a", "b", "c", "d"}, names(unsorted), "anything else keeps the graph's order")
}

func TestLiveArticleCount(t *testing.T) {
	live := &repo_service.ForkGraphEntry{Repo: &repo_model.Repository{}}
	tombstone := &repo_service.ForkGraphEntry{Repo: &repo_model.Repository{IsTombstoned: true}}
	assert.Equal(t, 0, liveArticleCount(nil))
	assert.Equal(t, 1, liveArticleCount([]*repo_service.ForkGraphEntry{tombstone, live}))
	assert.Equal(t, 2, liveArticleCount([]*repo_service.ForkGraphEntry{live, tombstone, live}))
}
