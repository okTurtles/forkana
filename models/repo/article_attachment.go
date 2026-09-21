// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"context"

	"code.gitea.io/gitea/models/db"
	"code.gitea.io/gitea/modules/container"
	"code.gitea.io/gitea/modules/timeutil"

	"xorm.io/builder"
)

// ArticleAttachment states that an article repository keeps an attachment alive
// and may use it for article delivery. Several repositories may reference the
// same attachment, so the stored object outlives any single repository.
//
// The effective reference count of an attachment is the number of rows here; it
// is deliberately not stored on the attachment row.
type ArticleAttachment struct {
	ID           int64              `xorm:"pk autoincr"`
	RepoID       int64              `xorm:"UNIQUE(s) INDEX NOT NULL"`
	AttachmentID int64              `xorm:"UNIQUE(s) INDEX NOT NULL"`
	CreatedUnix  timeutil.TimeStamp `xorm:"created"`
}

func init() {
	db.RegisterModel(new(ArticleAttachment))
}

// AddArticleAttachments associates the given attachments with a repository.
// It is idempotent: already existing associations are left untouched, and
// duplicates within attachmentIDs are collapsed.
func AddArticleAttachments(ctx context.Context, repoID int64, attachmentIDs []int64) error {
	if repoID == 0 || len(attachmentIDs) == 0 {
		return nil
	}

	return db.WithTx(ctx, func(ctx context.Context) error {
		var known []int64
		if err := db.GetEngine(ctx).Table("article_attachment").
			Where(builder.Eq{"repo_id": repoID}).
			In("attachment_id", attachmentIDs).
			Cols("attachment_id").
			Find(&known); err != nil {
			return err
		}

		seen := container.SetOf(known...)
		rows := make([]*ArticleAttachment, 0, len(attachmentIDs))
		for _, attachmentID := range attachmentIDs {
			if attachmentID == 0 || !seen.Add(attachmentID) {
				continue
			}
			rows = append(rows, &ArticleAttachment{RepoID: repoID, AttachmentID: attachmentID})
		}
		if len(rows) == 0 {
			return nil
		}
		return db.Insert(ctx, rows)
	})
}

// HasArticleAttachment reports whether the repository is associated with the attachment.
func HasArticleAttachment(ctx context.Context, repoID, attachmentID int64) (bool, error) {
	if repoID == 0 || attachmentID == 0 {
		return false, nil
	}
	return db.GetEngine(ctx).Exist(&ArticleAttachment{RepoID: repoID, AttachmentID: attachmentID})
}

// GetArticleAttachmentRepoIDs returns the IDs of every repository associated
// with the attachment.
func GetArticleAttachmentRepoIDs(ctx context.Context, attachmentID int64) ([]int64, error) {
	repoIDs := make([]int64, 0, 4)
	if attachmentID == 0 {
		return repoIDs, nil
	}
	return repoIDs, db.GetEngine(ctx).Table("article_attachment").
		Where(builder.Eq{"attachment_id": attachmentID}).
		Cols("repo_id").
		Find(&repoIDs)
}

// CountArticleAttachmentRepos returns how many repositories keep the attachment alive.
func CountArticleAttachmentRepos(ctx context.Context, attachmentID int64) (int64, error) {
	if attachmentID == 0 {
		return 0, nil
	}
	return db.GetEngine(ctx).Count(&ArticleAttachment{AttachmentID: attachmentID})
}

// GetRepoArticleAttachmentIDs returns the IDs of every attachment associated
// with the repository.
func GetRepoArticleAttachmentIDs(ctx context.Context, repoID int64) ([]int64, error) {
	attachmentIDs := make([]int64, 0, 8)
	if repoID == 0 {
		return attachmentIDs, nil
	}
	return attachmentIDs, db.GetEngine(ctx).Table("article_attachment").
		Where(builder.Eq{"repo_id": repoID}).
		Cols("attachment_id").
		Find(&attachmentIDs)
}

// CopyArticleAttachments gives the new repository the associations of the old
// one, sharing the very same attachment rows and stored objects. It must run
// inside the fork transaction.
func CopyArticleAttachments(ctx context.Context, newRepoID, oldRepoID int64) error {
	attachmentIDs, err := GetRepoArticleAttachmentIDs(ctx, oldRepoID)
	if err != nil {
		return err
	}
	return AddArticleAttachments(ctx, newRepoID, attachmentIDs)
}

// DeleteArticleAttachmentsByRepoID drops every association of a repository.
// The attachment rows and stored objects stay behind for the garbage collector,
// which reclaims them only once no repository references them anymore.
func DeleteArticleAttachmentsByRepoID(ctx context.Context, repoID int64) error {
	if repoID == 0 {
		return nil
	}
	_, err := db.GetEngine(ctx).Delete(&ArticleAttachment{RepoID: repoID})
	return err
}
