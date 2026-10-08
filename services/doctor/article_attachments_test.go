// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package doctor

import (
	"testing"

	"code.gitea.io/gitea/models/db"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	"code.gitea.io/gitea/modules/log"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckPendingArticleAttachments(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())
	logger := log.GetManager().GetLogger(log.DEFAULT)

	stale := &repo_model.Attachment{UUID: "7f2c9a31-0000-4000-8000-0000000a0001", RepoID: 1, UploaderID: 4, Purpose: repo_model.AttachmentPurposeArticle, Size: 42}
	require.NoError(t, db.Insert(t.Context(), stale))
	_, err := db.GetEngine(t.Context()).Table("attachment").Where("id = ?", stale.ID).
		Update(map[string]any{"created_unix": 1000})
	require.NoError(t, err)

	// reporting never deletes
	assert.NoError(t, checkPendingArticleAttachments(t.Context(), logger, false))
	unittest.AssertExistsAndLoadBean(t, &repo_model.Attachment{ID: stale.ID})

	// the test instance is not finalized, so --fix refuses to collect rather than failing
	assert.NoError(t, checkPendingArticleAttachments(t.Context(), logger, true))
	unittest.AssertExistsAndLoadBean(t, &repo_model.Attachment{ID: stale.ID})
}
