// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package markup

import (
	"testing"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderHelperSubjectLink(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	subject, err := repo_model.GetOrCreateSubject(ctx, "Solar system")
	require.NoError(t, err)
	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 4}) // user5, public
	repo.SubjectID = subject.ID
	require.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(ctx, repo, "subject_id"))

	deleted, err := repo_model.GetOrCreateSubject(ctx, "Deleted subject")
	require.NoError(t, err)
	tomb := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 10}) // user12, public
	tomb.SubjectID = deleted.ID
	tomb.IsTombstoned = true
	require.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(ctx, tomb, "subject_id", "is_tombstoned"))

	hidden, err := repo_model.GetOrCreateSubject(ctx, "Hidden subject")
	require.NoError(t, err)
	private := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 40}) // privated_org's public repo
	private.SubjectID = hidden.ID
	require.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(ctx, private, "subject_id"))

	emptySubject, err := repo_model.GetOrCreateSubject(ctx, "Empty subject")
	require.NoError(t, err)
	empty := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 11}) // user13, public
	empty.SubjectID = emptySubject.ID
	empty.IsEmpty = true
	require.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(ctx, empty, "subject_id", "is_empty"))

	check := FormalRenderHelperFuncs().IsSubjectLinkExisting
	exists := func(subject, owner string) bool {
		_, ownerExists := check(ctx, subject, owner)
		return ownerExists
	}
	assert.False(t, exists("Hidden subject", ""), "its only article belongs to a private organization")
	assert.False(t, exists("Hidden subject", "privated_org"))
	assert.False(t, exists("Empty subject", ""), "its only article has no content")
	assert.True(t, exists("Solar system", ""))
	assert.True(t, exists("solar  system", ""), "names are normalized like the subject routes do")
	assert.True(t, exists("Solar system", "user5"))
	assert.True(t, exists("Solar system", "USER5"), "user names are case-insensitive")
	assert.False(t, exists("Solar system", "user2"), "user2 has no article on the subject")
	assert.False(t, exists("Chang'e 4", ""), "no such subject")
	assert.False(t, exists("Deleted subject", ""), "every article of the subject was deleted")

	// one call answers both questions, for a link to a missing article (review M1)
	subjectExists, ownerExists := check(ctx, "Solar system", "user2")
	assert.True(t, subjectExists)
	assert.False(t, ownerExists)
	subjectExists, ownerExists = check(ctx, "Chang'e 4", "user2")
	assert.False(t, subjectExists)
	assert.False(t, ownerExists)
}
