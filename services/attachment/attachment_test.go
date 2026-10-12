// Copyright 2021 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package attachment

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/test"
	"code.gitea.io/gitea/services/context/upload"

	_ "code.gitea.io/gitea/models/actions"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	unittest.MainTest(m)
}

func TestUploadAttachment(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	user := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 1})

	fPath := "./attachment_test.go"
	f, err := os.Open(fPath)
	assert.NoError(t, err)
	defer f.Close()

	attach, err := NewAttachment(t.Context(), &repo_model.Attachment{
		RepoID:     1,
		UploaderID: user.ID,
		Name:       filepath.Base(fPath),
	}, f, -1)
	assert.NoError(t, err)

	attachment, err := repo_model.GetAttachmentByUUID(t.Context(), attach.UUID)
	assert.NoError(t, err)
	assert.Equal(t, user.ID, attachment.UploaderID)
	assert.Equal(t, int64(0), attachment.DownloadCount)
}

func TestUploadAttachmentTooLargeWhileStreaming(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())
	defer test.MockVariableValue(&setting.Attachment.MaxSize, 1)()

	content := bytes.Repeat([]byte("a"), 1<<20+1)
	for _, declared := range []int64{-1, 10} {
		// an unknown or understated size passes the upfront check, so the limit is only hit
		// while reading, inside the storage write
		_, err := UploadAttachment(t.Context(), bytes.NewReader(content), "", declared, &repo_model.Attachment{
			Name:       "big.txt",
			UploaderID: 1,
			RepoID:     1,
		})
		assert.True(t, upload.IsErrFileTooLarge(err), "declared size %d: got %v", declared, err)
	}
}
