// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repository

import (
	"strings"
	"testing"
	"time"

	"code.gitea.io/gitea/models/db"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	"code.gitea.io/gitea/modules/storage"
	"code.gitea.io/gitea/modules/timeutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// addStoredAttachment inserts an article attachment with a controlled creation time
// and writes its stored object, so the collector has something to reclaim.
func addStoredAttachment(t *testing.T, uuid string, createdUnix timeutil.TimeStamp) *repo_model.Attachment {
	t.Helper()
	attach := &repo_model.Attachment{
		UUID:       uuid,
		RepoID:     1,
		UploaderID: 2,
		Purpose:    repo_model.AttachmentPurposeArticle,
		Name:       "image.png",
	}
	require.NoError(t, db.Insert(t.Context(), attach))
	_, err := db.GetEngine(t.Context()).Exec("UPDATE `attachment` SET created_unix = ? WHERE id = ?", createdUnix, attach.ID)
	require.NoError(t, err)

	content := "attachment content"
	_, err = storage.Attachments.Save(attach.RelativePath(), strings.NewReader(content), int64(len(content)))
	require.NoError(t, err)
	t.Cleanup(func() { _ = storage.Attachments.Delete(attach.RelativePath()) })
	return attach
}

func TestGarbageCollectArticleAttachments(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())

	cutoff := time.Unix(2000, 0)
	old, recent := timeutil.TimeStamp(1000), timeutil.TimeStamp(3000)

	abandoned := addStoredAttachment(t, "7f2c9a31-0000-4000-8000-00000000d001", old)
	pending := addStoredAttachment(t, "7f2c9a31-0000-4000-8000-00000000d002", recent)
	associated := addStoredAttachment(t, "7f2c9a31-0000-4000-8000-00000000d003", old)
	require.NoError(t, repo_model.AddArticleAttachments(t.Context(), 2, []int64{associated.ID}))

	// a dry run reports without touching anything
	collected, err := GarbageCollectArticleAttachments(t.Context(), GarbageCollectArticleAttachmentsOptions{OlderThan: cutoff, DryRun: true})
	require.NoError(t, err)
	assert.Equal(t, 0, collected)
	unittest.AssertExistsAndLoadBean(t, &repo_model.Attachment{ID: abandoned.ID})

	collected, err = GarbageCollectArticleAttachments(t.Context(), GarbageCollectArticleAttachmentsOptions{OlderThan: cutoff})
	require.NoError(t, err)
	assert.Equal(t, 1, collected)

	// only the abandoned upload is gone, object included
	unittest.AssertNotExistsBean(t, &repo_model.Attachment{ID: abandoned.ID})
	_, err = storage.Attachments.Stat(abandoned.RelativePath())
	assert.Error(t, err)

	unittest.AssertExistsAndLoadBean(t, &repo_model.Attachment{ID: pending.ID})
	unittest.AssertExistsAndLoadBean(t, &repo_model.Attachment{ID: associated.ID})
	_, err = storage.Attachments.Stat(associated.RelativePath())
	assert.NoError(t, err)

	// a second run has nothing left to do
	collected, err = GarbageCollectArticleAttachments(t.Context(), GarbageCollectArticleAttachmentsOptions{OlderThan: cutoff})
	require.NoError(t, err)
	assert.Equal(t, 0, collected)
}

func TestGarbageCollectArticleAttachmentsLimit(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())

	cutoff := time.Unix(2000, 0)
	addStoredAttachment(t, "7f2c9a31-0000-4000-8000-00000000e001", timeutil.TimeStamp(1000))
	addStoredAttachment(t, "7f2c9a31-0000-4000-8000-00000000e002", timeutil.TimeStamp(1000))

	collected, err := GarbageCollectArticleAttachments(t.Context(), GarbageCollectArticleAttachmentsOptions{OlderThan: cutoff, Limit: 1})
	require.NoError(t, err)
	assert.Equal(t, 1, collected)

	collected, err = GarbageCollectArticleAttachments(t.Context(), GarbageCollectArticleAttachmentsOptions{OlderThan: cutoff, Limit: 1})
	require.NoError(t, err)
	assert.Equal(t, 1, collected)
}
