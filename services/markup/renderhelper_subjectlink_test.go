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

	exists := FormalRenderHelperFuncs().IsSubjectLinkExisting
	assert.True(t, exists(ctx, "Solar system", ""))
	assert.True(t, exists(ctx, "solar  system", ""), "names are normalized like the subject routes do")
	assert.True(t, exists(ctx, "Solar system", "user5"))
	assert.True(t, exists(ctx, "Solar system", "USER5"), "user names are case-insensitive")
	assert.False(t, exists(ctx, "Solar system", "user2"), "user2 has no article on the subject")
	assert.False(t, exists(ctx, "Chang'e 4", ""), "no such subject")
	assert.False(t, exists(ctx, "Deleted subject", ""), "every article of the subject was deleted")
}
