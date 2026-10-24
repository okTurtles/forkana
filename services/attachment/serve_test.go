// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package attachment

import (
	"testing"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanServe(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())

	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	user13 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 13})
	repo1 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	repo2 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 2})
	// repo3 is private, readable by user2 but not by user13.
	repo3 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 3})
	require.True(t, repo3.IsPrivate)

	canServe := func(t *testing.T, doer *user_model.User, scope *repo_model.Repository, attach *repo_model.Attachment) bool {
		t.Helper()
		ok, err := CanServe(t.Context(), doer, scope, attach, false)
		require.NoError(t, err)
		return ok
	}

	// The editor previews an upload before any commit exists, so the uploader must
	// reach it while it is still referenced by nothing.
	t.Run("PendingUpload", func(t *testing.T) {
		attach := newTestAttachment(t, repo1.ID, user2.ID)

		assert.True(t, canServe(t, user2, nil, attach))
		assert.True(t, canServe(t, user2, repo1, attach))
		assert.False(t, canServe(t, user13, nil, attach))
		assert.False(t, canServe(t, nil, nil, attach))
	})

	// Access follows the associations, not the repository the attachment was
	// uploaded to: a fork's reader sees the inherited image even when the origin
	// repository is unreadable to them.
	t.Run("AssociatedWithAReadableRepository", func(t *testing.T) {
		attach := newTestAttachment(t, repo3.ID, user2.ID)
		require.NoError(t, repo_model.AddArticleAttachments(t.Context(), repo1.ID, []int64{attach.ID}))

		assert.True(t, canServe(t, nil, nil, attach))
		assert.True(t, canServe(t, user13, nil, attach))
	})

	t.Run("AssociatedOnlyWithAPrivateRepository", func(t *testing.T) {
		attach := newTestAttachment(t, repo3.ID, user2.ID)
		require.NoError(t, repo_model.AddArticleAttachments(t.Context(), repo3.ID, []int64{attach.ID}))

		assert.False(t, canServe(t, nil, nil, attach))
		assert.False(t, canServe(t, user13, nil, attach))
		assert.True(t, canServe(t, user2, nil, attach))
		assert.True(t, canServe(t, user2, repo3, attach))
	})

	// A repository that does not keep the attachment alive must not be usable as a
	// cover for serving it.
	t.Run("ScopeMustHoldTheAssociation", func(t *testing.T) {
		attach := newTestAttachment(t, repo1.ID, user2.ID)
		require.NoError(t, repo_model.AddArticleAttachments(t.Context(), repo1.ID, []int64{attach.ID}))

		assert.True(t, canServe(t, user13, repo1, attach))
		assert.False(t, canServe(t, user13, repo2, attach))
		assert.True(t, canServe(t, user13, nil, attach))
	})

	// An article upload records its purpose, so one without associations is a
	// pending upload and the legacy fallback must not expose it.
	t.Run("UnassociatedArticleUploadSkipsLegacyFallback", func(t *testing.T) {
		attach := newTestAttachment(t, repo1.ID, user2.ID)

		assert.False(t, canServe(t, user13, nil, attach))
		assert.False(t, canServe(t, user13, repo1, attach))
	})

	// Rows predating the association table carry no purpose; until the backfill is
	// finalized they stay readable through the repository they were uploaded to.
	t.Run("LegacyFallback", func(t *testing.T) {
		attach := newTestAttachment(t, repo1.ID, user2.ID)
		attach.Purpose = repo_model.AttachmentPurposeUnspecified
		require.NoError(t, repo_model.UpdateAttachment(t.Context(), attach))

		assert.True(t, canServe(t, user13, nil, attach))
		assert.True(t, canServe(t, user13, repo1, attach))

		private := newTestAttachment(t, repo3.ID, user2.ID)
		private.Purpose = repo_model.AttachmentPurposeUnspecified
		require.NoError(t, repo_model.UpdateAttachment(t.Context(), private))

		assert.False(t, canServe(t, user13, nil, private))
		assert.True(t, canServe(t, user2, nil, private))
	})

	// Once an association exists, it is the only rule: the fallback must not widen
	// access back to the origin repository's readers.
	t.Run("LegacyFallbackStopsAtTheFirstAssociation", func(t *testing.T) {
		attach := newTestAttachment(t, repo1.ID, user2.ID)
		attach.Purpose = repo_model.AttachmentPurposeUnspecified
		require.NoError(t, repo_model.UpdateAttachment(t.Context(), attach))
		require.NoError(t, repo_model.AddArticleAttachments(t.Context(), repo3.ID, []int64{attach.ID}))

		assert.False(t, canServe(t, user13, nil, attach))
		assert.False(t, canServe(t, user13, repo1, attach))
		assert.True(t, canServe(t, user2, nil, attach))
	})

	// A token limited to public resources only reaches an attachment through a
	// non-private repository associated with it, even when its user can read more.
	t.Run("PublicOnly", func(t *testing.T) {
		canServePublicOnly := func(t *testing.T, doer *user_model.User, scope *repo_model.Repository, attach *repo_model.Attachment) bool {
			t.Helper()
			ok, err := CanServe(t.Context(), doer, scope, attach, true)
			require.NoError(t, err)
			return ok
		}
		admin := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 1})

		private := newTestAttachment(t, repo3.ID, user2.ID)
		require.NoError(t, repo_model.AddArticleAttachments(t.Context(), repo3.ID, []int64{private.ID}))
		assert.False(t, canServePublicOnly(t, user2, nil, private))
		assert.False(t, canServePublicOnly(t, user2, repo3, private))
		assert.False(t, canServePublicOnly(t, admin, nil, private))

		shared := newTestAttachment(t, repo3.ID, user2.ID)
		require.NoError(t, repo_model.AddArticleAttachments(t.Context(), repo3.ID, []int64{shared.ID}))
		require.NoError(t, repo_model.AddArticleAttachments(t.Context(), repo1.ID, []int64{shared.ID}))
		assert.True(t, canServePublicOnly(t, user2, nil, shared))
		assert.True(t, canServePublicOnly(t, user2, repo1, shared))
		assert.False(t, canServePublicOnly(t, user2, repo3, shared))
		assert.True(t, canServePublicOnly(t, admin, nil, shared))

		pending := newTestAttachment(t, repo1.ID, user2.ID)
		assert.False(t, canServePublicOnly(t, user2, nil, pending))

		legacy := newTestAttachment(t, repo3.ID, user2.ID)
		legacy.Purpose = repo_model.AttachmentPurposeUnspecified
		require.NoError(t, repo_model.UpdateAttachment(t.Context(), legacy))
		assert.False(t, canServePublicOnly(t, user2, nil, legacy))
	})
}
