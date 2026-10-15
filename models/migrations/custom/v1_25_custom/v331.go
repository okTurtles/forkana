// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package v1_25_custom

import (
	"code.gitea.io/gitea/modules/timeutil"

	"xorm.io/xorm"
)

// CreateArticleCollaboratorTable creates the article_collaborator table, which keeps
// the users invited to maintain an article, their role and whether they accepted.
func CreateArticleCollaboratorTable(x *xorm.Engine) error {
	type ArticleCollaborator struct {
		ID          int64              `xorm:"pk autoincr"`
		RepoID      int64              `xorm:"UNIQUE(s) INDEX NOT NULL"`
		UserID      int64              `xorm:"UNIQUE(s) INDEX NOT NULL"`
		InviterID   int64              `xorm:"NOT NULL DEFAULT 0"`
		Role        int                `xorm:"NOT NULL DEFAULT 1"`
		Status      int                `xorm:"INDEX NOT NULL DEFAULT 0"`
		CreatedUnix timeutil.TimeStamp `xorm:"created"`
		UpdatedUnix timeutil.TimeStamp `xorm:"updated"`
	}

	return x.Sync(new(ArticleCollaborator))
}
