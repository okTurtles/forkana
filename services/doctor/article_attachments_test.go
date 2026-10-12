// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package doctor

import (
	"testing"
	"time"

	"code.gitea.io/gitea/models/db"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/test"
	repo_service "code.gitea.io/gitea/services/repository"

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

func TestArticleAttachmentGCSettings(t *testing.T) {
	cases := []struct {
		name          string
		ini           string
		wantGrace     time.Duration
		wantMaxPerRun int
	}{
		{
			name:          "defaults",
			wantGrace:     repo_service.DefaultArticleAttachmentGCGracePeriod,
			wantMaxPerRun: repo_service.DefaultArticleAttachmentGCMaxPerRun,
		},
		{
			name:          "configured",
			ini:           "[cron.gc_article_attachments]\nOLDER_THAN = 1h\nMAX_PER_RUN = 5\n",
			wantGrace:     time.Hour,
			wantMaxPerRun: 5,
		},
		{
			name:          "no cap",
			ini:           "[cron.gc_article_attachments]\nMAX_PER_RUN = 0\n",
			wantGrace:     repo_service.DefaultArticleAttachmentGCGracePeriod,
			wantMaxPerRun: 0,
		},
		{
			name:          "invalid values fall back to the defaults",
			ini:           "[cron.gc_article_attachments]\nOLDER_THAN = -1h\nMAX_PER_RUN = -1\n",
			wantGrace:     repo_service.DefaultArticleAttachmentGCGracePeriod,
			wantMaxPerRun: repo_service.DefaultArticleAttachmentGCMaxPerRun,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg, err := setting.NewConfigProviderFromData(c.ini)
			require.NoError(t, err)
			defer test.MockVariableValue(&setting.CfgProvider, cfg)()

			grace, maxPerRun := articleAttachmentGCSettings()
			assert.Equal(t, c.wantGrace, grace)
			assert.Equal(t, c.wantMaxPerRun, maxPerRun)
		})
	}
}
