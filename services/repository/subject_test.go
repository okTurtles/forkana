// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repository

import (
	"testing"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepareSubjectAndRepoName(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	_, err := repo_model.CreateSubject(ctx, "Prepare Moon")
	require.NoError(t, err)

	cases := []struct {
		subject, repoName         string
		wantSubject, wantRepoName string
		invalid                   bool
	}{
		// no subject: nothing to do
		{"", "custom", "", "custom", false},
		// an existing subject is used as-is, whatever the spelling
		{"Prepare Moon!", "", "Prepare Moon", "prepare-moon", false},
		{"Prepare Moon!", "prepare-moon", "Prepare Moon", "prepare-moon", false},
		// a new subject is normalized, its first letter capitalized
		{"  brand_New ", "", "Brand New", "brand-new", false},
		// punctuation is allowed
		{"AC/DC", "", "AC/DC", "acdc", false},
		// a repo name chosen by the user is kept
		{"Brand New", "my-article", "Brand New", "my-article", false},
		// a new subject must follow the rule
		{"a#b", "", "", "", true},
		{":Colon", "", "", "", true},
		{"   ", "", "", "", true},
	}
	for _, c := range cases {
		subject, repoName, err := PrepareSubjectAndRepoName(ctx, c.subject, c.repoName)
		if c.invalid {
			assert.True(t, repo_model.IsErrSubjectNameInvalid(err), "PrepareSubjectAndRepoName(%q): %v", c.subject, err)
			continue
		}
		require.NoError(t, err, "PrepareSubjectAndRepoName(%q)", c.subject)
		assert.Equal(t, c.wantSubject, subject, "subject for %q", c.subject)
		assert.Equal(t, c.wantRepoName, repoName, "repo name for %q", c.subject)
	}
}
