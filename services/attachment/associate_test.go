// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package attachment

import (
	"fmt"
	"testing"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func articleContent(uuids ...string) string {
	content := "# Article\n"
	for _, uuid := range uuids {
		content += fmt.Sprintf("![img](/attachments/%s)\n", uuid)
	}
	return content
}

func TestAssociateArticleAttachments(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())

	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	user13 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 13})
	repo1 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	repo2 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 2})

	associated := func(t *testing.T, repoID, attachmentID int64) bool {
		t.Helper()
		has, err := repo_model.HasArticleAttachment(t.Context(), repoID, attachmentID)
		require.NoError(t, err)
		return has
	}

	t.Run("NoReferences", func(t *testing.T) {
		attach := newTestAttachment(t, repo1.ID, user2.ID)

		require.NoError(t, AssociateArticleAttachments(t.Context(), user2, repo1, "plain article text"))
		assert.False(t, associated(t, repo1.ID, attach.ID))
	})

	t.Run("OriginRepository", func(t *testing.T) {
		attach := newTestAttachment(t, repo1.ID, user2.ID)

		require.NoError(t, AssociateArticleAttachments(t.Context(), user2, repo1, articleContent(attach.UUID)))
		assert.True(t, associated(t, repo1.ID, attach.ID))
	})

	// Repeated discovery is expected: the web commit path and the push path can both see the
	// same content, and reconciliation re-runs over it later.
	t.Run("IsIdempotent", func(t *testing.T) {
		attach := newTestAttachment(t, repo1.ID, user2.ID)
		content := articleContent(attach.UUID)

		require.NoError(t, AssociateArticleAttachments(t.Context(), user2, repo1, content))
		require.NoError(t, AssociateArticleAttachments(t.Context(), user2, repo1, content, content))

		repoIDs, err := repo_model.GetArticleAttachmentRepoIDs(t.Context(), attach.ID)
		require.NoError(t, err)
		assert.Equal(t, []int64{repo1.ID}, repoIDs)
	})

	// Pasting somebody else's UUID into an article must not hand the attachment over, and must
	// not fail the commit that already happened either.
	t.Run("UnauthorizedReferenceIsSkipped", func(t *testing.T) {
		attach := newTestAttachment(t, repo2.ID, user2.ID)

		require.NoError(t, AssociateArticleAttachments(t.Context(), user13, repo1, articleContent(attach.UUID)))
		assert.False(t, associated(t, repo1.ID, attach.ID))
	})

	t.Run("MixedReferencesAssociateTheAllowedOnes", func(t *testing.T) {
		allowed := newTestAttachment(t, repo1.ID, user2.ID)
		refused := newTestAttachment(t, repo2.ID, user2.ID)

		require.NoError(t, AssociateArticleAttachments(t.Context(), user13, repo1, articleContent(allowed.UUID, refused.UUID)))
		assert.True(t, associated(t, repo1.ID, allowed.ID))
		assert.False(t, associated(t, repo1.ID, refused.ID))
	})

	t.Run("UnknownUUIDIsIgnored", func(t *testing.T) {
		content := articleContent("11111111-2222-3333-4444-555555555555")
		assert.NoError(t, AssociateArticleAttachments(t.Context(), user2, repo1, content))
	})

	t.Run("NilRepository", func(t *testing.T) {
		attach := newTestAttachment(t, repo1.ID, user2.ID)
		assert.NoError(t, AssociateArticleAttachments(t.Context(), user2, nil, articleContent(attach.UUID)))
	})
}
