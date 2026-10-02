// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package v1_25_custom

import (
	"code.gitea.io/gitea/modules/timeutil"

	"xorm.io/xorm"
)

// AddArticleAttachmentTable creates the article_attachment table, which records
// that an article repository keeps an attachment alive, and adds the purpose
// column that tells article-editor uploads apart from issue/release drafts.
func AddArticleAttachmentTable(x *xorm.Engine) error {
	type ArticleAttachment struct {
		ID           int64              `xorm:"pk autoincr"`
		RepoID       int64              `xorm:"UNIQUE(s) INDEX NOT NULL"`
		AttachmentID int64              `xorm:"UNIQUE(s) INDEX NOT NULL"`
		CreatedUnix  timeutil.TimeStamp `xorm:"created"`
	}

	type Attachment struct {
		Purpose int `xorm:"NOT NULL DEFAULT 0"`
	}

	return x.Sync(new(ArticleAttachment), new(Attachment))
}
