// Copyright 2026 The Forkana Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package explore

import (
	"testing"
	"time"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/timeutil"

	"github.com/stretchr/testify/assert"
)

func TestArticleChangeTimes(t *testing.T) {
	created := time.Date(2025, 11, 11, 14, 1, 15, 0, time.UTC)
	repo := &repo_model.Repository{CreatedUnix: timeutil.TimeStamp(created.Unix())}
	times := newArticleChangeTimes(nil, repo) // (a root article: no lookup)

	edited := time.Date(2025, 11, 11, 17, 53, 53, 0, time.UTC)
	assert.True(t, edited.Equal(times.of(&git.Commit{Committer: &git.Signature{When: edited}})), "a dated change keeps its date")

	imported := times.of(&git.Commit{Committer: &git.Signature{When: gitZeroTime}})
	assert.True(t, created.Equal(imported), "an import made without a date is dated by the article's creation, got %v", imported)

	assert.True(t, edited.Equal(times.of(&git.Commit{Author: &git.Signature{When: edited}})), "the author's date without a committer")
}
