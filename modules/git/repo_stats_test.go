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
	assert.Positive(t, count, "Expected at least one contributor")

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
	assert.Positive(t, countWithPastSince, "Expected contributors with past since date")

	// Test with empty branch (should default to HEAD)
	countEmptyBranch, err := bareRepo1.GetContributorCount("", time.Time{})
	assert.NoError(t, err)
	assert.Positive(t, countEmptyBranch, "Expected at least one contributor with empty branch")
}

func TestParseContributorAuthorEmails(t *testing.T) {
	stat := " 1 file changed, 1 insertion(+)\n"
	out := "\x1eAlice\x1falice@example.com\n\n" + stat +
		// the same name with another email is another contributor
		"\x1eAlice\x1falice@work.example.com\n\n" + stat +
		// the same email under another name is the same contributor
		"\x1eAlice Smith\x1falice@example.com\n\n" + stat +
		// case is kept, as the contributors graph keeps it
		"\x1eBob\x1fBob@Example.com\n\n" + stat +
		"\x1eBob\x1fbob@example.com\n\n" + stat +
		// skipped: no email, no name, no file changed
		"\x1eNobody\x1f\n\n" + stat +
		"\x1e\x1fanon@example.com\n\n" + stat +
		"\x1eEmpty\x1fempty@example.com\n"
	assert.Equal(t, []string{"alice@example.com", "alice@work.example.com", "Bob@Example.com", "bob@example.com"}, parseContributorAuthorEmails(out))
	assert.Empty(t, parseContributorAuthorEmails(""))
}

func TestRepository_GetContributorAuthorEmails(t *testing.T) {
	bareRepo1, err := OpenRepository(t.Context(), filepath.Join(testReposDir, "repo1_bare"))
	assert.NoError(t, err)
	defer bareRepo1.Close()

	emails, err := bareRepo1.GetContributorAuthorEmails("master", time.Time{})
	assert.NoError(t, err)
	assert.NotEmpty(t, emails)

	emails, err = bareRepo1.GetContributorAuthorEmails("master", time.Now().Add(24*time.Hour))
	assert.NoError(t, err)
	assert.Empty(t, emails)
}
