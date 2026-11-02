// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package v1_25_custom

import (
	"testing"

	"code.gitea.io/gitea/models/migrations/base"
	repo_model "code.gitea.io/gitea/models/repo"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// subjectWithSlug is the subject table as migration 326 left it
type subjectWithSlug struct {
	ID   int64  `xorm:"pk autoincr"`
	Name string `xorm:"VARCHAR(255) NOT NULL"`
	Slug string `xorm:"VARCHAR(255) UNIQUE NOT NULL"`
}

func (*subjectWithSlug) TableName() string {
	return "subject"
}

// Test_ResluggifySymbolSubjects tests the v332 migration (#401): subjects without any ASCII
// letter or digit get a slug of their own instead of "subject"/"subject-N", and the others
// keep theirs.
func Test_ResluggifySymbolSubjects(t *testing.T) {
	x, deferable := base.PrepareTestEnv(t, 0)
	defer deferable()
	if x == nil || t.Failed() {
		return
	}

	_, _ = x.Exec("DROP TABLE IF EXISTS subject")
	require.NoError(t, x.Sync(new(subjectWithSlug)))

	sixteen := "''''''''''''''''"
	// the slugs migration 326 gave them: every name without an ASCII letter or digit was
	// "subject", the collisions got a numeric suffix
	legacy := []*subjectWithSlug{
		{Name: sixteen, Slug: "subject"},
		{Name: "東京", Slug: "subject-2"},
		{Name: "Subject", Slug: "subject-3"},
		{Name: "The Moon", Slug: "the-moon"},
		{Name: "the moon!", Slug: "the-moon-2"},
		// a non-Latin letter was dropped: these shared the slug of "1980"
		{Name: "Москва 1980", Slug: "1980"},
		{Name: "1980", Slug: "1980-2"},
		{Name: "Ελλάδα 2004", Slug: "2004"},
	}
	for _, s := range legacy {
		_, err := x.Insert(s)
		require.NoError(t, err)
	}

	require.NoError(t, ResluggifySymbolSubjects(x))

	slugs := map[string]string{}
	var results []subjectWithSlug
	require.NoError(t, x.Table("subject").Find(&results))
	for _, s := range results {
		slugs[s.Name] = s.Slug
	}
	assert.Equal(t, map[string]string{
		sixteen: repo_model.GenerateSlugFromName(sixteen),
		"東京":    repo_model.GenerateSlugFromName("東京"),
		// "subject" was freed by the apostrophes
		"Subject":  "subject",
		"The Moon": "the-moon",
		// its slug is taken by an older subject: it keeps its own
		"the moon!":   "the-moon-2",
		"Москва 1980": repo_model.GenerateSlugFromName("Москва 1980"),
		// "1980" was freed by "Москва 1980"
		"1980":        "1980",
		"Ελλάδα 2004": repo_model.GenerateSlugFromName("Ελλάδα 2004"),
	}, slugs)
	assert.NotEqual(t, "2004", slugs["Ελλάδα 2004"])
	assert.NotEqual(t, slugs[sixteen], slugs["東京"])
	assert.NotEqual(t, repo_model.GenerateSlugFromName("'''"), slugs[sixteen])

	// running it again is a no-op
	require.NoError(t, ResluggifySymbolSubjects(x))
	results = nil
	require.NoError(t, x.Table("subject").Find(&results))
	for _, s := range results {
		assert.Equal(t, slugs[s.Name], s.Slug)
	}
}
