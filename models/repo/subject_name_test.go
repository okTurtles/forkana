// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repo_test

import (
	"errors"
	"strings"
	"testing"

	"code.gitea.io/gitea/models/db"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	"code.gitea.io/gitea/modules/subjecttitle"
	"code.gitea.io/gitea/modules/util"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func subjectNameProblem(err error) subjecttitle.Problem {
	var invalid repo_model.ErrSubjectNameInvalid
	if errors.As(err, &invalid) {
		return invalid.Problem
	}
	return subjecttitle.ProblemNone
}

func TestValidateSubjectName(t *testing.T) {
	name, err := repo_model.ValidateSubjectName("  antoni_Gaudí   ")
	require.NoError(t, err)
	assert.Equal(t, "Antoni Gaudí", name, "underscores become spaces and the first letter is capitalized")

	for in, want := range map[string]string{
		";alskdjf":                      ";alskdjf",
		"Test: Gaudí":                   "Test: Gaudí",
		"AC/DC":                         "AC/DC",
		"C++":                           "C++",
		"iPhone":                        "IPhone",
		"Python (programming language)": "Python (programming language)",
	} {
		got, err := repo_model.ValidateSubjectName(in)
		require.NoError(t, err, "ValidateSubjectName(%q)", in)
		assert.Equal(t, want, got)
	}

	for in, problem := range map[string]subjecttitle.Problem{
		"   ":      subjecttitle.ProblemEmpty,
		"a#b":      subjecttitle.ProblemForbiddenChar,
		"%41":      subjecttitle.ProblemPercentEncoding,
		"AT&amp;T": subjecttitle.ProblemHTMLEntity,
		"~~~":      subjecttitle.ProblemTildes,
		"../Foo":   subjecttitle.ProblemRelativePath,
		":Foo":     subjecttitle.ProblemLeadingColon,
	} {
		_, err := repo_model.ValidateSubjectName(in)
		assert.True(t, repo_model.IsErrSubjectNameInvalid(err), "ValidateSubjectName(%q): %v", in, err)
		assert.Equal(t, problem, subjectNameProblem(err), "ValidateSubjectName(%q)", in)
		assert.ErrorIs(t, err, util.ErrInvalidArgument)
	}

	// the limit counts bytes, as on Wikipedia: 127 "É" (254 bytes) fit, 128 (256 bytes) do not
	_, err = repo_model.ValidateSubjectName(strings.Repeat("É", 127))
	require.NoError(t, err)
	_, err = repo_model.ValidateSubjectName(strings.Repeat("É", 128))
	assert.Equal(t, subjecttitle.ProblemTooLong, subjectNameProblem(err))
	_, err = repo_model.ValidateSubjectName(strings.Repeat("a", repo_model.MaxSubjectNameLength))
	require.NoError(t, err)
}

func TestGetOrCreateSubject_TitleRule(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	// new subjects must follow the rule
	for _, name := range []string{"a#b", "%41", "Moon~~~", ":Moon", "."} {
		_, err := repo_model.GetOrCreateSubject(ctx, name)
		assert.True(t, repo_model.IsErrSubjectNameInvalid(err), "GetOrCreateSubject(%q): %v", name, err)
		// the error survives wrapping by callers
		assert.True(t, repo_model.IsErrSubjectNameInvalid(errors.Join(errors.New("wrapped"), err)))
	}

	// punctuation is allowed, and the first letter is capitalized
	for in, want := range map[string]string{
		";alskdjf":    ";alskdjf",
		"Test: Gaudí": "Test: Gaudí",
		"AC/DC":       "AC/DC",
		"iPhone":      "IPhone",
	} {
		subject, err := repo_model.GetOrCreateSubject(ctx, in)
		require.NoError(t, err, "GetOrCreateSubject(%q)", in)
		assert.Equal(t, want, subject.Name)
	}

	// 200 "é" is 400 bytes: too long for Wikipedia, so too long for a new subject
	_, err := repo_model.GetOrCreateSubject(ctx, strings.Repeat("é", 200))
	assert.Equal(t, subjecttitle.ProblemTooLong, subjectNameProblem(err))

	// the name is normalized before being stored
	subject, err := repo_model.GetOrCreateSubject(ctx, "  antoni_  Gaudí  ")
	require.NoError(t, err)
	assert.Equal(t, "Antoni Gaudí", subject.Name)
	assert.Equal(t, "antoni-gaudi", subject.Slug)

	// a legacy subject that breaks the rule is still found by slug (no new subject is created)
	legacy := &repo_model.Subject{Name: "Legacy #Gaudí", Slug: repo_model.GenerateSlugFromName("Legacy #Gaudí")}
	require.NoError(t, db.Insert(ctx, legacy))
	found, err := repo_model.GetOrCreateSubject(ctx, "Legacy #Gaudí")
	require.NoError(t, err)
	assert.Equal(t, legacy.ID, found.ID)

	// ... even when it is longer than 255 bytes, or starts with a lowercase letter
	longLegacy := &repo_model.Subject{Name: strings.Repeat("é", 200), Slug: repo_model.GenerateSlugFromName(strings.Repeat("é", 200))}
	require.NoError(t, db.Insert(ctx, longLegacy))
	found, err = repo_model.GetOrCreateSubject(ctx, strings.Repeat("é", 200))
	require.NoError(t, err)
	assert.Equal(t, longLegacy.ID, found.ID)
	lowerLegacy := &repo_model.Subject{Name: "eBay legacy", Slug: repo_model.GenerateSlugFromName("eBay legacy")}
	require.NoError(t, db.Insert(ctx, lowerLegacy))
	found, err = repo_model.GetOrCreateSubject(ctx, "eBay legacy")
	require.NoError(t, err)
	assert.Equal(t, "eBay legacy", found.Name, "an existing subject keeps its name")
}

func TestResolveSubjectName(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	moon, err := repo_model.CreateSubject(ctx, "Moon")
	require.NoError(t, err)
	legacy := &repo_model.Subject{Name: "Test [Legacy]", Slug: repo_model.GenerateSlugFromName("Test [Legacy]")}
	require.NoError(t, db.Insert(ctx, legacy))
	const legacyUnderscoreName = "Foo_Bar\u00a0Baz" // with a no-break space
	legacyUnderscore := &repo_model.Subject{Name: legacyUnderscoreName, Slug: repo_model.GenerateSlugFromName(legacyUnderscoreName)}
	require.NoError(t, db.Insert(ctx, legacyUnderscore))

	cases := []struct {
		in, want string
		problem  subjecttitle.Problem
	}{
		// an existing subject wins, whatever the spelling
		{"Moon~~~", moon.Name, ""},
		{"  moon ", moon.Name, ""},
		{"Test [Legacy]", legacy.Name, ""},
		{"test legacy?", legacy.Name, ""},
		// the slug of the name as typed is tried first, so normalization does not hide a
		// legacy subject (its slug is "foo-barbaz", "Foo Bar Baz" would give "foo-bar-baz")
		{legacyUnderscoreName, legacyUnderscore.Name, ""},
		// no existing subject: the normalized name must follow the rule
		{" brand_new  Thing ", "Brand new Thing", ""},
		{"Test: Brand New", "Test: Brand New", ""},
		{"a#b", "", subjecttitle.ProblemForbiddenChar},
		{":Brand New", "", subjecttitle.ProblemLeadingColon},
		{"   ", "", subjecttitle.ProblemEmpty},
	}
	for _, c := range cases {
		got, err := repo_model.ResolveSubjectName(ctx, c.in)
		if c.problem != "" {
			assert.True(t, repo_model.IsErrSubjectNameInvalid(err), "ResolveSubjectName(%q): %v", c.in, err)
			assert.Equal(t, c.problem, subjectNameProblem(err), "ResolveSubjectName(%q)", c.in)
			continue
		}
		require.NoError(t, err, "ResolveSubjectName(%q)", c.in)
		assert.Equal(t, c.want, got, "ResolveSubjectName(%q)", c.in)
	}

	_, err = repo_model.ResolveSubjectName(ctx, strings.Repeat("É", 128))
	assert.Equal(t, subjecttitle.ProblemTooLong, subjectNameProblem(err))

	// the length applies to the normalized name, so long input that normalizes to a valid
	// title, or that resolves to an existing subject, is not "too long"
	got, err := repo_model.ResolveSubjectName(ctx, strings.Repeat(" ", 1500)+"Brand Moon"+strings.Repeat("_", 1500))
	require.NoError(t, err)
	assert.Equal(t, "Brand Moon", got)
	got, err = repo_model.ResolveSubjectName(ctx, "Moon"+strings.Repeat("~", 2000))
	require.NoError(t, err)
	assert.Equal(t, moon.Name, got)
	// beyond the abuse guard, the input is too long without any lookup
	_, err = repo_model.ResolveSubjectName(ctx, "Moon"+strings.Repeat("~", 9000))
	assert.Equal(t, subjecttitle.ProblemTooLong, subjectNameProblem(err))

	// resolving never creates a subject
	unittest.AssertNotExistsBean(t, &repo_model.Subject{Slug: "brand-new-thing"})
}
