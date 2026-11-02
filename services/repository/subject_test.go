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
		// #401: a title without any ASCII letter or digit is a subject of its own, not the
		// one of a longer run of apostrophes, and the form script's "subject" placeholder is
		// replaced by the generated repo name
		{"''''''''''''''''", "", "''''''''''''''''", repo_model.GenerateSlugFromName("''''''''''''''''"), false},
		{"'''", "subject", "'''", repo_model.GenerateSlugFromName("'''"), false},
		{"東京", "subject", "東京", repo_model.GenerateSlugFromName("東京"), false},
		// ... and so is "1980", what the script generates for a title with a non-Latin letter
		{"Москва 1980", "1980", "Москва 1980", repo_model.GenerateSlugFromName("Москва 1980"), false},
		{"1980", "1980", "1980", "1980", false},
		// "subject" is only a placeholder for such titles
		{"Brand New", "subject", "Brand New", "subject", false},
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
