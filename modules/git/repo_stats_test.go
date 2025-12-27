// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package git

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRepository_GetCodeActivityStats(t *testing.T) {
	bareRepo1Path := filepath.Join(testReposDir, "repo1_bare")
	bareRepo1, err := OpenRepository(t.Context(), bareRepo1Path)
	assert.NoError(t, err)
	defer bareRepo1.Close()

	timeFrom, err := time.Parse(time.RFC3339, "2016-01-01T00:00:00+00:00")
	assert.NoError(t, err)

	code, err := bareRepo1.GetCodeActivityStats(timeFrom, "")
	assert.NoError(t, err)
	assert.NotNil(t, code)

	assert.EqualValues(t, 10, code.CommitCount)
	assert.EqualValues(t, 3, code.AuthorCount)
	assert.EqualValues(t, 10, code.CommitCountInAllBranches)
	assert.EqualValues(t, 10, code.Additions)
	assert.EqualValues(t, 1, code.Deletions)
	assert.Len(t, code.Authors, 3)
	assert.Equal(t, "tris.git@shoddynet.org", code.Authors[1].Email)
	assert.EqualValues(t, 3, code.Authors[1].Commits)
	assert.EqualValues(t, 5, code.Authors[0].Commits)
}

func TestRepository_GetContributorCount(t *testing.T) {
	bareRepo1Path := filepath.Join(testReposDir, "repo1_bare")
	bareRepo1, err := OpenRepository(t.Context(), bareRepo1Path)
	assert.NoError(t, err)
	defer bareRepo1.Close()

	// Test without since filter - should count all contributors
	count, err := bareRepo1.GetContributorCount("master", time.Time{})
	assert.NoError(t, err)
	assert.True(t, count > 0, "Expected at least one contributor")

	// Test with a future since date - should return 0 contributors
	futureTime := time.Now().AddDate(1, 0, 0) // 1 year in the future
	count, err = bareRepo1.GetContributorCount("master", futureTime)
	assert.NoError(t, err)
	assert.EqualValues(t, 0, count, "Expected 0 contributors for future since date")

	// Test with a past since date that includes all commits
	pastTime, err := time.Parse(time.RFC3339, "2016-01-01T00:00:00+00:00")
	assert.NoError(t, err)
	countWithPastSince, err := bareRepo1.GetContributorCount("master", pastTime)
	assert.NoError(t, err)
	assert.True(t, countWithPastSince > 0, "Expected contributors with past since date")
}
