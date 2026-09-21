// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repo_test

import (
	"testing"

	"code.gitea.io/gitea/models/db"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	"code.gitea.io/gitea/modules/timeutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// addAttachment inserts an attachment with a controlled creation time, which the
// `created` column would otherwise overwrite.
func addAttachment(t *testing.T, attach *repo_model.Attachment, createdUnix timeutil.TimeStamp) *repo_model.Attachment {
	t.Helper()
	require.NoError(t, db.Insert(t.Context(), attach))
	_, err := db.GetEngine(t.Context()).Exec("UPDATE `attachment` SET created_unix = ? WHERE id = ?", createdUnix, attach.ID)
	require.NoError(t, err)
	attach.CreatedUnix = createdUnix
	return attach
}

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

func TestFindUnreferencedArticleAttachments(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	const old, recent = timeutil.TimeStamp(1000), timeutil.TimeStamp(3000)
	cutoff := timeutil.TimeStamp(2000)

	abandoned := addAttachment(t, &repo_model.Attachment{UUID: "3d6b1f2c-0000-4000-8000-00000000a001", RepoID: 1, UploaderID: 2, Purpose: repo_model.AttachmentPurposeArticle, Name: "abandoned.png"}, old)
	// still inside the grace period: its commit may not have landed yet
	addAttachment(t, &repo_model.Attachment{UUID: "3d6b1f2c-0000-4000-8000-00000000a002", RepoID: 1, UploaderID: 2, Purpose: repo_model.AttachmentPurposeArticle, Name: "pending.png"}, recent)
	// referenced by a repository
	associated := addAttachment(t, &repo_model.Attachment{UUID: "3d6b1f2c-0000-4000-8000-00000000a003", RepoID: 1, UploaderID: 2, Purpose: repo_model.AttachmentPurposeArticle, Name: "live.png"}, old)
	require.NoError(t, repo_model.AddArticleAttachments(t.Context(), 1, []int64{associated.ID}))
	// a legacy row may be an article attachment whose association is not backfilled yet
	addAttachment(t, &repo_model.Attachment{UUID: "3d6b1f2c-0000-4000-8000-00000000a004", RepoID: 1, UploaderID: 2, Name: "legacy.png"}, old)
	// an article upload that ended up on an issue is that issue's attachment
	addAttachment(t, &repo_model.Attachment{UUID: "3d6b1f2c-0000-4000-8000-00000000a005", RepoID: 1, IssueID: 1, UploaderID: 2, Purpose: repo_model.AttachmentPurposeArticle, Name: "issue.png"}, old)

	found, err := repo_model.FindUnreferencedArticleAttachments(t.Context(), cutoff, 0)
	assert.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, abandoned.ID, found[0].ID)

	// the batch limit bounds a run
	orphan2 := addAttachment(t, &repo_model.Attachment{UUID: "3d6b1f2c-0000-4000-8000-00000000a006", RepoID: 1, UploaderID: 2, Purpose: repo_model.AttachmentPurposeArticle, Name: "abandoned2.png"}, old)
	found, err = repo_model.FindUnreferencedArticleAttachments(t.Context(), cutoff, 1)
	assert.NoError(t, err)
	assert.Len(t, found, 1)

	found, err = repo_model.FindUnreferencedArticleAttachments(t.Context(), cutoff, 10)
	assert.NoError(t, err)
	assert.Len(t, found, 2)
	assert.Equal(t, orphan2.ID, found[1].ID)
}

func TestDeleteUnreferencedArticleAttachment(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	abandoned := addAttachment(t, &repo_model.Attachment{UUID: "3d6b1f2c-0000-4000-8000-00000000b001", RepoID: 1, UploaderID: 2, Purpose: repo_model.AttachmentPurposeArticle, Name: "abandoned.png"}, timeutil.TimeStamp(1000))
	claimed := addAttachment(t, &repo_model.Attachment{UUID: "3d6b1f2c-0000-4000-8000-00000000b002", RepoID: 1, UploaderID: 2, Purpose: repo_model.AttachmentPurposeArticle, Name: "claimed.png"}, timeutil.TimeStamp(1000))
	require.NoError(t, repo_model.AddArticleAttachments(t.Context(), 2, []int64{claimed.ID}))

	deleted, err := repo_model.DeleteUnreferencedArticleAttachment(t.Context(), abandoned.ID)
	assert.NoError(t, err)
	assert.True(t, deleted)
	unittest.AssertNotExistsBean(t, &repo_model.Attachment{ID: abandoned.ID})

	// an association written while a collection run was in flight keeps the row
	deleted, err = repo_model.DeleteUnreferencedArticleAttachment(t.Context(), claimed.ID)
	assert.NoError(t, err)
	assert.False(t, deleted)
	unittest.AssertExistsAndLoadBean(t, &repo_model.Attachment{ID: claimed.ID})

	deleted, err = repo_model.DeleteUnreferencedArticleAttachment(t.Context(), 0)
	assert.NoError(t, err)
	assert.False(t, deleted)
}

func TestRetainedRepoAttachmentIDs(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	// an article upload is governed by the associations and the garbage collector,
	// even before a commit associates it
	article := addAttachment(t, &repo_model.Attachment{UUID: "3d6b1f2c-0000-4000-8000-00000000c001", RepoID: 1, UploaderID: 2, Purpose: repo_model.AttachmentPurposeArticle, Name: "article.png"}, timeutil.TimeStamp(1000))
	// a plain upload of this repository is nobody else's
	addAttachment(t, &repo_model.Attachment{UUID: "3d6b1f2c-0000-4000-8000-00000000c002", RepoID: 1, UploaderID: 2, Name: "plain.png"}, timeutil.TimeStamp(1000))

	ids, err := repo_model.RetainedRepoAttachmentIDs(t.Context(), 1)
	assert.NoError(t, err)
	// attachments 1 and 3 are associated by the fixtures
	assert.ElementsMatch(t, []int64{1, 3, article.ID}, ids)

	ids, err = repo_model.RetainedRepoAttachmentIDs(t.Context(), 0)
	assert.NoError(t, err)
	assert.Empty(t, ids)
}
