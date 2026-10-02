// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package attachment

import (
	"testing"

	"code.gitea.io/gitea/models/db"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReconcileArticleAttachments(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())

	const (
		referencedUUID = "7f2c9a31-0000-4000-8000-0000000e0001"
		droppedUUID    = "7f2c9a31-0000-4000-8000-0000000e0002"
	)
	repo := newArticleRepo(t, "reconcile-article")
	referenced := newLegacyAttachment(t, referencedUUID, repo.ID)
	// an attachment no revision of the article mentions any more, already
	// associated: the only way a run could break it is by removing the row
	dropped := newLegacyAttachment(t, droppedUUID, repo.ID)
	require.NoError(t, repo_model.AddArticleAttachments(t.Context(), repo.ID, []int64{dropped.ID}))
	commitArticle(t, repo.RepoPath(), articleContent(referencedUUID), "")

	opts := ReconcileOptions{StartRepoID: repo.ID, DryRun: true}
	result, err := ReconcileArticleAttachments(t.Context(), opts)
	require.NoError(t, err)
	assert.Equal(t, 1, result.ReposScanned)
	assert.Equal(t, 1, result.Outstanding)
	assert.Equal(t, 0, result.AssociationsInserted)
	unittest.AssertNotExistsBean(t, &repo_model.ArticleAttachment{RepoID: repo.ID, AttachmentID: referenced.ID})

	opts.DryRun = false
	result, err = ReconcileArticleAttachments(t.Context(), opts)
	require.NoError(t, err)
	assert.Equal(t, 1, result.AssociationsInserted)
	unittest.AssertExistsAndLoadBean(t, &repo_model.ArticleAttachment{RepoID: repo.ID, AttachmentID: referenced.ID})
	// add-only: the unreferenced association survives the run
	unittest.AssertExistsAndLoadBean(t, &repo_model.ArticleAttachment{RepoID: repo.ID, AttachmentID: dropped.ID})

	// a repaired instance needs no further repair
	result, err = ReconcileArticleAttachments(t.Context(), opts)
	require.NoError(t, err)
	assert.Equal(t, 0, result.AssociationsInserted)
}

func TestReconcileArticleAttachmentsReportsDangling(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())

	repo := newArticleRepo(t, "reconcile-dangling")
	before, err := repo_model.CountDanglingArticleAttachments(t.Context())
	require.NoError(t, err)

	orphan := &repo_model.ArticleAttachment{RepoID: repo.ID, AttachmentID: 9999999}
	require.NoError(t, db.Insert(t.Context(), orphan))

	result, err := ReconcileArticleAttachments(t.Context(), ReconcileOptions{StartRepoID: repo.ID})
	require.NoError(t, err)
	assert.Equal(t, before+1, result.DanglingAssociations)
	// reported, never repaired by deletion
	unittest.AssertExistsAndLoadBean(t, &repo_model.ArticleAttachment{ID: orphan.ID})
}
