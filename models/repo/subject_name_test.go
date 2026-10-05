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
	"code.gitea.io/gitea/modules/util"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func isTooLongErr(err error) bool {
	var invalid repo_model.ErrSubjectNameInvalid
	return errors.As(err, &invalid) && invalid.TooLong
}

func TestValidateSubjectName(t *testing.T) {
	name, err := repo_model.ValidateSubjectName("  Antoni   Gaudí ")
	require.NoError(t, err)
	assert.Equal(t, "Antoni Gaudí", name)

	_, err = repo_model.ValidateSubjectName("   ")
	assert.True(t, repo_model.IsErrSubjectNameInvalid(err))

	_, err = repo_model.ValidateSubjectName(";alskdjf")
	assert.True(t, repo_model.IsErrSubjectNameInvalid(err))
	assert.False(t, isTooLongErr(err))
	assert.ErrorIs(t, err, util.ErrInvalidArgument)

	// the limit counts characters, not bytes: 255 "é" (510 bytes) fit, 256 do not
	_, err = repo_model.ValidateSubjectName(strings.Repeat("é", repo_model.MaxSubjectNameLength))
	require.NoError(t, err)
	_, err = repo_model.ValidateSubjectName(strings.Repeat("é", repo_model.MaxSubjectNameLength+1))
	assert.True(t, repo_model.IsErrSubjectNameInvalid(err))
	assert.True(t, isTooLongErr(err))
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

	// 200 "é" is 400 bytes but only 200 characters: it is a valid new subject (#401 review)
	long := strings.Repeat("é", 200)
	subject, err := repo_model.GetOrCreateSubject(ctx, long)
	require.NoError(t, err)
	assert.Equal(t, long, subject.Name)
	reloaded := unittest.AssertExistsAndLoadBean(t, &repo_model.Subject{ID: subject.ID})
	assert.Equal(t, long, reloaded.Name)

	_, err = repo_model.GetOrCreateSubject(ctx, strings.Repeat("é", repo_model.MaxSubjectNameLength+1))
	assert.True(t, isTooLongErr(err))

	// the name is normalized before being stored
	subject, err = repo_model.GetOrCreateSubject(ctx, "  Antoni   Gaudí  ")
	require.NoError(t, err)
	assert.Equal(t, "Antoni Gaudí", subject.Name)
	assert.Equal(t, "antoni-gaudi", subject.Slug)

	// a legacy subject that breaks the rule is still found by slug (no new subject is created)
	legacy := &repo_model.Subject{Name: "Test: Legacy Gaudí", Slug: repo_model.GenerateSlugFromName("Test: Legacy Gaudí")}
	require.NoError(t, db.Insert(ctx, legacy))
	found, err := repo_model.GetOrCreateSubject(ctx, "Test: Legacy Gaudí")
	require.NoError(t, err)
	assert.Equal(t, legacy.ID, found.ID)
}

func TestResolveSubjectName(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	moon, err := repo_model.CreateSubject(ctx, "Moon")
	require.NoError(t, err)
	legacy := &repo_model.Subject{Name: "Test: Legacy", Slug: repo_model.GenerateSlugFromName("Test: Legacy")}
	require.NoError(t, db.Insert(ctx, legacy))

	cases := []struct {
		in, want string
		invalid  bool
	}{
		// an existing subject wins, whatever the spelling
		{"Moon!", moon.Name, false},
		{"  moon ", moon.Name, false},
		{"Test: Legacy", legacy.Name, false},
		{"test legacy?", legacy.Name, false},
		// no existing subject: the normalized name must follow the rule
		{" Brand   New ", "Brand New", false},
		{"Test: Brand New", "", true},
		{";alskdjf", "", true},
		{"   ", "", true},
	}
	for _, c := range cases {
		got, err := repo_model.ResolveSubjectName(ctx, c.in)
		if c.invalid {
			assert.True(t, repo_model.IsErrSubjectNameInvalid(err), "ResolveSubjectName(%q): %v", c.in, err)
			continue
		}
		require.NoError(t, err, "ResolveSubjectName(%q)", c.in)
		assert.Equal(t, c.want, got, "ResolveSubjectName(%q)", c.in)
	}

	_, err = repo_model.ResolveSubjectName(ctx, strings.Repeat("é", repo_model.MaxSubjectNameLength+1))
	assert.True(t, isTooLongErr(err))

	// resolving never creates a subject
	unittest.AssertNotExistsBean(t, &repo_model.Subject{Slug: "brand-new"})
}
