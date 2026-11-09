// Copyright 2026 The Forkana Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package explore

import (
	"testing"
	"time"

	"code.gitea.io/gitea/models/db"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	"code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/timeutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArticleChangeTimes(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	// fixtures: repo 27 is a root article, repo 29 a fork of it
	created := time.Date(2025, 11, 11, 14, 1, 15, 0, time.UTC)
	root := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 27})
	setCreated := func(repo *repo_model.Repository, at time.Time) {
		repo.CreatedUnix = timeutil.TimeStamp(at.Unix())
		// (created_unix is xorm's "created" column: no Cols update writes it)
		_, err := db.GetEngine(ctx).Exec("UPDATE repository SET created_unix = ? WHERE id = ?", repo.CreatedUnix, repo.ID)
		require.NoError(t, err)
	}
	setCreated(root, created)
	fork := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 29})
	require.True(t, fork.IsFork)
	require.Equal(t, root.ID, fork.ForkID)
	setCreated(fork, created.Add(30*24*time.Hour))

	edited := time.Date(2025, 11, 11, 17, 53, 53, 0, time.UTC)
	importedCommit := &git.Commit{Committer: &git.Signature{When: gitZeroTime}}

	for _, repo := range []*repo_model.Repository{root, fork} {
		times := newArticleChangeTimes(ctx, repo)
		assert.True(t, edited.Equal(times.of(&git.Commit{Committer: &git.Signature{When: edited}})), "repo %d: a dated change keeps its date", repo.ID)
		assert.True(t, edited.Equal(times.of(&git.Commit{Author: &git.Signature{When: edited}})), "repo %d: the author's date without a committer", repo.ID)
		// an import made without a date is dated by the creation of the root article, the
		// one it was imported into, also in a fork (created later)
		imported := times.of(importedCommit)
		assert.True(t, created.Equal(imported), "repo %d: got %v", repo.ID, imported)
	}

	// a fork whose parent is gone is its own root
	orphan := &repo_model.Repository{ID: 999999, IsFork: true, ForkID: 999998, CreatedUnix: timeutil.TimeStamp(created.Unix())}
	assert.True(t, created.Equal(articleRootCreated(ctx, orphan)))
}
