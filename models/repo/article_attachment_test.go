// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repo_test

import (
	"testing"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"

	"github.com/stretchr/testify/assert"
)

func TestAddArticleAttachments(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	// a new association plus one that already exists, and a duplicate input
	assert.NoError(t, repo_model.AddArticleAttachments(t.Context(), 1, []int64{1, 2, 2}))

	unittest.AssertExistsAndLoadBean(t, &repo_model.ArticleAttachment{RepoID: 1, AttachmentID: 2})
	unittest.AssertCount(t, &repo_model.ArticleAttachment{RepoID: 1, AttachmentID: 2}, 1)
	unittest.AssertCount(t, &repo_model.ArticleAttachment{RepoID: 1, AttachmentID: 1}, 1)

	// re-adding the very same set must stay idempotent
	assert.NoError(t, repo_model.AddArticleAttachments(t.Context(), 1, []int64{1, 2}))
	unittest.AssertCount(t, &repo_model.ArticleAttachment{RepoID: 1}, 3)

	// nothing to do
	assert.NoError(t, repo_model.AddArticleAttachments(t.Context(), 1, nil))
	assert.NoError(t, repo_model.AddArticleAttachments(t.Context(), 0, []int64{1}))
	unittest.AssertCount(t, &repo_model.ArticleAttachment{RepoID: 1}, 3)
}

func TestHasArticleAttachment(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	has, err := repo_model.HasArticleAttachment(t.Context(), 1, 1)
	assert.NoError(t, err)
	assert.True(t, has)

	has, err = repo_model.HasArticleAttachment(t.Context(), 3, 1)
	assert.NoError(t, err)
	assert.False(t, has)

	has, err = repo_model.HasArticleAttachment(t.Context(), 0, 0)
	assert.NoError(t, err)
	assert.False(t, has)
}

func TestArticleAttachmentReferences(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	repoIDs, err := repo_model.GetArticleAttachmentRepoIDs(t.Context(), 1)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []int64{1, 2}, repoIDs)

	count, err := repo_model.CountArticleAttachmentRepos(t.Context(), 1)
	assert.NoError(t, err)
	assert.EqualValues(t, 2, count)

	count, err = repo_model.CountArticleAttachmentRepos(t.Context(), 2)
	assert.NoError(t, err)
	assert.EqualValues(t, 0, count)

	attachmentIDs, err := repo_model.GetRepoArticleAttachmentIDs(t.Context(), 1)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []int64{1, 3}, attachmentIDs)
}

func TestCopyArticleAttachments(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	// the fork shares the same attachment rows, it does not mint new ones
	assert.NoError(t, repo_model.CopyArticleAttachments(t.Context(), 3, 1))

	attachmentIDs, err := repo_model.GetRepoArticleAttachmentIDs(t.Context(), 3)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []int64{1, 3}, attachmentIDs)

	count, err := repo_model.CountArticleAttachmentRepos(t.Context(), 1)
	assert.NoError(t, err)
	assert.EqualValues(t, 3, count)

	// copying twice must not duplicate rows
	assert.NoError(t, repo_model.CopyArticleAttachments(t.Context(), 3, 1))
	unittest.AssertCount(t, &repo_model.ArticleAttachment{RepoID: 3}, 2)
}

func TestDeleteArticleAttachmentsByRepoID(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	assert.NoError(t, repo_model.DeleteArticleAttachmentsByRepoID(t.Context(), 1))
	unittest.AssertNotExistsBean(t, &repo_model.ArticleAttachment{RepoID: 1})

	// the attachment row itself survives, the other repository still keeps it alive
	unittest.AssertExistsAndLoadBean(t, &repo_model.Attachment{ID: 1})
	count, err := repo_model.CountArticleAttachmentRepos(t.Context(), 1)
	assert.NoError(t, err)
	assert.EqualValues(t, 1, count)

	assert.NoError(t, repo_model.DeleteArticleAttachmentsByRepoID(t.Context(), 0))
	unittest.AssertCount(t, &repo_model.ArticleAttachment{RepoID: 2}, 1)
}
