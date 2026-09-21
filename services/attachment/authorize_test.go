// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package attachment

import (
	"testing"

	"code.gitea.io/gitea/models/db"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestAttachment inserts an article attachment owned by repoID and uploaded
// by uploaderID, which is the shape a pending editor upload has.
func newTestAttachment(t *testing.T, repoID, uploaderID int64) *repo_model.Attachment {
	t.Helper()
	attach := &repo_model.Attachment{
		UUID:       uuid.New().String(),
		RepoID:     repoID,
		UploaderID: uploaderID,
		Name:       "image.png",
		Purpose:    repo_model.AttachmentPurposeArticle,
	}
	require.NoError(t, db.Insert(t.Context(), attach))
	return attach
}

func TestCanAssociate(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())

	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	user13 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 13})
	repo1 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	repo2 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 2})
	// repo11 is a fork of repo10 in the fixtures.
	repo10 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 10})
	repo11 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 11})

	t.Run("NilArguments", func(t *testing.T) {
		attach := newTestAttachment(t, repo1.ID, user2.ID)

		ok, err := CanAssociate(t.Context(), user2, nil, attach)
		assert.NoError(t, err)
		assert.False(t, ok)

		ok, err = CanAssociate(t.Context(), user2, repo1, nil)
		assert.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("OriginRepository", func(t *testing.T) {
		attach := newTestAttachment(t, repo1.ID, user2.ID)

		ok, err := CanAssociate(t.Context(), nil, repo1, attach)
		assert.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("AlreadyAssociated", func(t *testing.T) {
		attach := newTestAttachment(t, repo1.ID, user2.ID)
		require.NoError(t, repo_model.AddArticleAttachments(t.Context(), repo2.ID, []int64{attach.ID}))

		ok, err := CanAssociate(t.Context(), nil, repo2, attach)
		assert.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("PendingUploadByDoer", func(t *testing.T) {
		attach := newTestAttachment(t, repo1.ID, user2.ID)

		ok, err := CanAssociate(t.Context(), user2, repo2, attach)
		assert.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("PendingUploadByAnotherUser", func(t *testing.T) {
		attach := newTestAttachment(t, repo1.ID, user2.ID)

		ok, err := CanAssociate(t.Context(), user13, repo2, attach)
		assert.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("UploaderCannotMoveALinkedAttachment", func(t *testing.T) {
		// Fixture attachment 12 belongs to repo2 and is linked to a release.
		attach := unittest.AssertExistsAndLoadBean(t, &repo_model.Attachment{ID: 12})
		require.Equal(t, user2.ID, attach.UploaderID)

		ok, err := CanAssociate(t.Context(), user2, repo1, attach)
		assert.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("UploaderCannotReuseAnAlreadyAssociatedAttachment", func(t *testing.T) {
		attach := newTestAttachment(t, repo1.ID, user2.ID)
		require.NoError(t, repo_model.AddArticleAttachments(t.Context(), repo1.ID, []int64{attach.ID}))

		ok, err := CanAssociate(t.Context(), user2, repo2, attach)
		assert.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("ForkInheritsFromOriginRepository", func(t *testing.T) {
		attach := newTestAttachment(t, repo10.ID, user13.ID)

		ok, err := CanAssociate(t.Context(), nil, repo11, attach)
		assert.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("ForkInheritsFromAssociatedParent", func(t *testing.T) {
		attach := newTestAttachment(t, repo1.ID, user2.ID)
		require.NoError(t, repo_model.AddArticleAttachments(t.Context(), repo10.ID, []int64{attach.ID}))

		ok, err := CanAssociate(t.Context(), nil, repo11, attach)
		assert.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("UnrelatedRepositoryIsRefused", func(t *testing.T) {
		attach := newTestAttachment(t, repo10.ID, user13.ID)

		ok, err := CanAssociate(t.Context(), user2, repo2, attach)
		assert.NoError(t, err)
		assert.False(t, ok)
	})

	// Knowing the UUID of an attachment held by a private repository must not
	// let a writer of an unrelated public repository republish it.
	t.Run("PrivateToPublicAttackIsRefused", func(t *testing.T) {
		private := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 3})
		require.True(t, private.IsPrivate)
		attach := newTestAttachment(t, private.ID, private.OwnerID)

		public := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
		require.False(t, public.IsPrivate)

		ok, err := CanAssociate(t.Context(), user2, public, attach)
		assert.NoError(t, err)
		assert.False(t, ok)
	})
}
