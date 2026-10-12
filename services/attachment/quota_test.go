// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package attachment

import (
	"bytes"
	"sync"
	"testing"
	"time"

	"code.gitea.io/gitea/models/db"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockArticleLimits sets every article upload limit, so a test only trips the one it means to.
func mockArticleLimits(t *testing.T, files, sizeMB, rate int64) {
	t.Helper()
	t.Cleanup(test.MockVariableValue(&setting.Attachment.ArticleMaxPendingFiles, files))
	t.Cleanup(test.MockVariableValue(&setting.Attachment.ArticleMaxPendingSize, sizeMB))
	t.Cleanup(test.MockVariableValue(&setting.Attachment.ArticleUploadRateLimit, rate))
	t.Cleanup(test.MockVariableValue(&setting.Attachment.ArticleUploadRateWindow, time.Minute))
}

func uploadArticle(t *testing.T, doer *user_model.User, content []byte, declaredSize int64) (*repo_model.Attachment, error) {
	t.Helper()
	return UploadArticleAttachment(t.Context(), doer, bytes.NewReader(content), "", declaredSize, &repo_model.Attachment{
		Name:       "image.png",
		UploaderID: doer.ID,
		RepoID:     1,
	})
}

func pendingCount(t *testing.T, uploaderID int64) int64 {
	t.Helper()
	count, _, err := repo_model.GetPendingArticleAttachmentStats(t.Context(), uploaderID)
	require.NoError(t, err)
	return count
}

func TestUploadArticleAttachmentPendingFileQuota(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())
	mockArticleLimits(t, 2, 0, 0)
	user := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})

	first, err := uploadArticle(t, user, []byte("one"), 3)
	require.NoError(t, err)
	assert.Equal(t, repo_model.AttachmentPurposeArticle, first.Purpose)
	_, err = uploadArticle(t, user, []byte("two"), 3)
	require.NoError(t, err)

	_, err = uploadArticle(t, user, []byte("three"), 5)
	assert.True(t, IsErrPendingQuotaExceeded(err), "got %v", err)
	assert.EqualValues(t, 2, pendingCount(t, user.ID))

	// a committed upload no longer counts as pending, which frees a slot
	require.NoError(t, repo_model.AddArticleAttachments(t.Context(), 1, []int64{first.ID}))
	_, err = uploadArticle(t, user, []byte("three"), 5)
	assert.NoError(t, err)
}

func TestUploadArticleAttachmentPendingSizeQuota(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())
	mockArticleLimits(t, 0, 1, 0)
	user := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})

	half := bytes.Repeat([]byte("a"), 600<<10)
	_, err := uploadArticle(t, user, half, int64(len(half)))
	require.NoError(t, err)

	// rejected on the declared size, before anything is stored
	_, err = uploadArticle(t, user, half, int64(len(half)))
	assert.True(t, IsErrPendingQuotaExceeded(err), "got %v", err)

	// an unknown or understated size is caught while reading
	_, err = uploadArticle(t, user, half, -1)
	assert.True(t, IsErrPendingQuotaExceeded(err), "got %v", err)
	_, err = uploadArticle(t, user, half, 10)
	assert.True(t, IsErrPendingQuotaExceeded(err), "got %v", err)

	count, size, err := repo_model.GetPendingArticleAttachmentStats(t.Context(), user.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 1, count)
	assert.EqualValues(t, len(half), size)

	// what still fits is accepted
	_, err = uploadArticle(t, user, bytes.Repeat([]byte("b"), 100<<10), -1)
	assert.NoError(t, err)
}

func TestUploadArticleAttachmentRateLimit(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())
	mockArticleLimits(t, 0, 0, 2)
	user := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})

	first, err := uploadArticle(t, user, []byte("one"), 3)
	require.NoError(t, err)
	// committing does not refund the rate
	require.NoError(t, repo_model.AddArticleAttachments(t.Context(), 1, []int64{first.ID}))
	_, err = uploadArticle(t, user, []byte("two"), 3)
	require.NoError(t, err)

	_, err = uploadArticle(t, user, []byte("three"), 5)
	assert.True(t, IsErrUploadRateLimited(err), "got %v", err)

	// once the uploads fall out of the window, the uploader may continue
	_, err = db.GetEngine(t.Context()).Table("attachment").Where("uploader_id = ?", user.ID).
		Update(map[string]any{"created_unix": time.Now().Add(-2 * time.Minute).Unix()})
	require.NoError(t, err)
	_, err = uploadArticle(t, user, []byte("three"), 5)
	assert.NoError(t, err)
}

func TestUploadArticleAttachmentAdminExempt(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())
	mockArticleLimits(t, 1, 1, 1)
	admin := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 1})
	require.True(t, admin.IsAdmin)

	for range 3 {
		_, err := uploadArticle(t, admin, []byte("image"), 5)
		require.NoError(t, err)
	}
}

func TestUploadArticleAttachmentConcurrent(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())
	const quota = 3
	mockArticleLimits(t, quota, 0, 0)
	user := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})

	var wg sync.WaitGroup
	errs := make(chan error, quota+5)
	for range quota + 5 {
		wg.Go(func() {
			_, err := uploadArticle(t, user, []byte("image"), 5)
			errs <- err
		})
	}
	wg.Wait()
	close(errs)

	accepted := 0
	for err := range errs {
		if err == nil {
			accepted++
			continue
		}
		assert.True(t, IsErrPendingQuotaExceeded(err), "got %v", err)
	}
	assert.Equal(t, quota, accepted)
	assert.EqualValues(t, quota, pendingCount(t, user.ID))
}

func TestErrPendingQuotaExceededMessage(t *testing.T) {
	assert.Contains(t, ErrPendingQuotaExceeded{MaxFiles: 20}.Error(), "20")
	assert.Contains(t, ErrPendingQuotaExceeded{MaxMB: 100}.Error(), "100 MB")
	assert.Contains(t, ErrUploadRateLimited{Limit: 10, Window: time.Minute}.Error(), "10 per 1m0s")
}
