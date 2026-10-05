// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repo_test

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	"code.gitea.io/gitea/models/db"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	"code.gitea.io/gitea/modules/util"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var subjectNameCases = []struct {
	name  string
	valid bool
}{
	// valid
	{"Moon", true},
	{"The Moon", true},
	{"Gaudí", true},
	{"Antoni Gaudí", true},
	{"Zalg'o", true},
	{"O’Brien", true},
	{"Jean-Paul Sartre", true},
	{"1984", true},
	{"2001 A Space Odyssey", true},
	{"Ελλάδα", true},
	{"東京", true},
	{"Москва", true},
	{"हिन्दी", true}, // Devanagari needs combining marks after the first letter
	{"a", true},
	{"Rock 'n' Roll", true},

	// invalid characters
	{"Test: Gaudí", false},
	{"Test: The Gaudí Question", false},
	{"Moon!", false},
	{"C++", false},
	{"AT&T", false},
	{"Foo/Bar", false},
	{"Foo_Bar", false},
	{"Foo.Bar", false},
	{"<script>", false},
	{"Hello 😀", false},
	{"Foo\tBar", false}, // not normalized: tabs are not allowed as such

	// must start with a letter or digit
	{";alskdjf", false},
	{"-Moon", false},
	{"'Moon", false},
	{"’Moon", false},
	{" Moon", false},
	{"́Moon", false}, // a combining mark cannot come first
	{"", false},
}

func TestIsValidSubjectName(t *testing.T) {
	for _, c := range subjectNameCases {
		assert.Equal(t, c.valid, repo_model.IsValidSubjectName(c.name), "IsValidSubjectName(%q)", c.name)
	}
}

// TestSubjectNameHTMLPattern checks that the HTML pattern used by the forms agrees with the
// server-side rule. Go's RE2 has the same Unicode classes as the browser's `v` flag.
func TestSubjectNameHTMLPattern(t *testing.T) {
	re := regexp.MustCompile(`^(?:` + repo_model.SubjectNameHTMLPattern + `)$`)
	for _, c := range subjectNameCases {
		normalized := repo_model.NormalizeSubjectName(c.name)
		if c.name != normalized {
			// the pattern tolerates whitespace that the server normalizes away
			continue
		}
		assert.Equal(t, c.valid, re.MatchString(c.name), "pattern match %q", c.name)
	}
	assert.True(t, re.MatchString("  The   Moon  "), "surrounding and repeated spaces are normalized by the server")
}

func TestNormalizeSubjectName(t *testing.T) {
	cases := map[string]string{
		"Moon":              "Moon",
		"  The Moon  ":      "The Moon",
		"The    Moon":       "The Moon",
		"The \t\n Moon":     "The Moon",
		"   ":               "",
		"Gaudí":            "Gaudí", // NFD → NFC
		" Jean - Paul ":     "Jean - Paul",
		"Antoni  Gaudí   x": "Antoni Gaudí x",
	}
	for in, want := range cases {
		assert.Equal(t, want, repo_model.NormalizeSubjectName(in), "NormalizeSubjectName(%q)", in)
	}
}

func TestValidateSubjectName(t *testing.T) {
	name, err := repo_model.ValidateSubjectName("  Antoni   Gaudí ")
	require.NoError(t, err)
	assert.Equal(t, "Antoni Gaudí", name)

	_, err = repo_model.ValidateSubjectName("   ")
	assert.True(t, repo_model.IsErrSubjectNameInvalid(err))

	_, err = repo_model.ValidateSubjectName(strings.Repeat("a", repo_model.MaxSubjectNameLength+1))
	assert.Error(t, err)
	assert.False(t, repo_model.IsErrSubjectNameInvalid(err))

	_, err = repo_model.ValidateSubjectName(";alskdjf")
	assert.True(t, repo_model.IsErrSubjectNameInvalid(err))
	assert.ErrorIs(t, err, util.ErrInvalidArgument)
}

func TestGetOrCreateSubject_TitleRule(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	// new subjects must follow the rule
	for _, name := range []string{";alskdjf", "Test: Gaudí", "Moon?"} {
		_, err := repo_model.GetOrCreateSubject(ctx, name)
		assert.True(t, repo_model.IsErrSubjectNameInvalid(err), "GetOrCreateSubject(%q): %v", name, err)
		// the error survives wrapping by callers
		assert.True(t, repo_model.IsErrSubjectNameInvalid(errors.Join(errors.New("wrapped"), err)))
	}

	// the name is normalized before being stored
	subject, err := repo_model.GetOrCreateSubject(ctx, "  Antoni   Gaudí  ")
	require.NoError(t, err)
	assert.Equal(t, "Antoni Gaudí", subject.Name)
	assert.Equal(t, "antoni-gaudi", subject.Slug)

	// a legacy subject that breaks the rule is still found by slug (no new subject is created)
	legacy := &repo_model.Subject{Name: "Test: Legacy Gaudí", Slug: repo_model.GenerateSlugFromName("Test: Legacy Gaudí")}
	require.NoError(t, db.Insert(ctx, legacy))
	found, err := repo_model.GetOrCreateSubject(ctx, "Test: Legacy Gaudí")
	require.NoError(t, err)
	assert.Equal(t, legacy.ID, found.ID)

	normalized, err := repo_model.CheckSubjectNameForCreate(ctx, "Test: Legacy Gaudí")
	require.NoError(t, err)
	assert.Equal(t, "Test: Legacy Gaudí", normalized)

	// a valid name with the legacy subject's slug links to it
	normalized, err = repo_model.CheckSubjectNameForCreate(ctx, "Test Legacy Gaudí")
	require.NoError(t, err)
	assert.Equal(t, "Test Legacy Gaudí", normalized)

	// an invalid name that is not exactly the legacy name is rejected
	_, err = repo_model.CheckSubjectNameForCreate(ctx, "test: legacy gaudí!")
	assert.True(t, repo_model.IsErrSubjectNameInvalid(err))

	_, err = repo_model.CheckSubjectNameForCreate(ctx, "   ")
	assert.True(t, repo_model.IsErrSubjectNameInvalid(err))

	_, err = repo_model.CheckSubjectNameForCreate(ctx, "Test: Brand New")
	assert.True(t, repo_model.IsErrSubjectNameInvalid(err))

	normalized, err = repo_model.CheckSubjectNameForCreate(ctx, " Brand   New ")
	require.NoError(t, err)
	assert.Equal(t, "Brand New", normalized)
}
