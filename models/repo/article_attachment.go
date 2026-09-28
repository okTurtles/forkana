// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"context"
	"fmt"

	"code.gitea.io/gitea/models/db"
	"code.gitea.io/gitea/models/unit"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/container"
	"code.gitea.io/gitea/modules/log"
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
//
// The read and the insert are not atomic against another writer — the push
// queue and the web commit path can discover the same reference at once — so
// the unique constraint may reject the insert. One retry then sees the other
// writer's rows and inserts only what is still missing; a second rejection
// means everything this call wanted is already there.
//
// Only a call that owns its transaction can retry: when the caller provides
// one, the failed insert has already rolled it back, so the error belongs to
// that caller.
func AddArticleAttachments(ctx context.Context, repoID int64, attachmentIDs []int64) error {
	if repoID == 0 || len(attachmentIDs) == 0 {
		return nil
	}

	err := addArticleAttachments(ctx, repoID, attachmentIDs)
	if err == nil || !db.IsErrDuplicateKey(err) || db.InTransaction(ctx) {
		return err
	}

	log.Debug("AddArticleAttachments: concurrent association of %d attachments to repo %d, retrying", len(attachmentIDs), repoID)
	err = addArticleAttachments(ctx, repoID, attachmentIDs)
	if err != nil && db.IsErrDuplicateKey(err) {
		// A concurrent writer associated the same pairs; the outcome is identical.
		log.Debug("AddArticleAttachments: duplicate association of %d attachments to repo %d ignored", len(attachmentIDs), repoID)
		return nil
	}
	return err
}

func addArticleAttachments(ctx context.Context, repoID int64, attachmentIDs []int64) error {
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

// CountArticleAttachmentRepos returns how many repositories keep the attachment alive.
func CountArticleAttachmentRepos(ctx context.Context, attachmentID int64) (int64, error) {
	if attachmentID == 0 {
		return 0, nil
	}
	return db.GetEngine(ctx).Count(&ArticleAttachment{AttachmentID: attachmentID})
}

// ArticleAttachmentAccessible reports whether the user may read the attachment
// through at least one of the repositories that keep it alive. An attachment is
// shared, so it outlives its origin repository and must not be bound to it:
// access follows the associations, exactly as LFS object access follows the
// repositories holding the object.
func ArticleAttachmentAccessible(ctx context.Context, user *user_model.User, attachmentID int64) (bool, error) {
	if attachmentID == 0 {
		return false, nil
	}
	if user != nil && user.IsAdmin {
		count, err := CountArticleAttachmentRepos(ctx, attachmentID)
		return count > 0, err
	}
	cond := AccessibleRepositoryCondition(user, unit.TypeCode)
	count, err := db.GetEngine(ctx).Where(cond).
		Join("INNER", "repository", "`article_attachment`.repo_id = `repository`.id").
		Count(&ArticleAttachment{AttachmentID: attachmentID})
	return count > 0, err
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

// noArticleAssociationCond matches attachments that no repository keeps alive.
func noArticleAssociationCond() builder.Cond {
	return builder.NotExists(
		builder.Select("1").From("article_attachment").
			Where(builder.Expr("article_attachment.attachment_id = attachment.id")),
	)
}

// unassociatedAttachmentCond matches attachments of the given purpose that no
// repository keeps alive and that no issue, comment or release links to.
func unassociatedAttachmentCond(purpose AttachmentPurpose) builder.Cond {
	return builder.Eq{
		"attachment.purpose":    purpose,
		"attachment.issue_id":   0,
		"attachment.comment_id": 0,
		"attachment.release_id": 0,
	}.And(noArticleAssociationCond())
}

// unreferencedArticleAttachmentCond matches attachments the garbage collector
// may reclaim: article uploads that no repository keeps alive any more and that
// no issue, comment or release links to.
//
// Only AttachmentPurposeArticle rows qualify. An unspecified-purpose row may be
// a legacy article attachment whose association has not been backfilled yet, so
// collecting it would destroy a live article image.
func unreferencedArticleAttachmentCond() builder.Cond {
	return unassociatedAttachmentCond(AttachmentPurposeArticle)
}

// FindUnreferencedArticleAttachments returns at most limit attachments that are
// no longer referenced by any repository and were created before olderThan.
//
// The age condition is the grace period: an attachment is created before the
// commit that associates it, and the association may still be on its way.
//
// A limit of 0 means no limit; a negative limit is a configuration error.
func FindUnreferencedArticleAttachments(ctx context.Context, olderThan timeutil.TimeStamp, limit int) ([]*Attachment, error) {
	if limit < 0 {
		return nil, fmt.Errorf("limit must not be negative: %d", limit)
	}
	attachments := make([]*Attachment, 0, min(limit, 64))
	sess := db.GetEngine(ctx).Table("attachment").
		Where(unreferencedArticleAttachmentCond()).
		And(builder.Lt{"attachment.created_unix": olderThan}).
		Asc("attachment.id")
	if limit > 0 {
		sess = sess.Limit(limit)
	}
	return attachments, sess.Find(&attachments)
}

// DeleteUnreferencedArticleAttachment deletes the attachment row only if it is
// still unreferenced, and reports whether it did. The condition is re-evaluated
// by the database as part of the delete, so an association committed in the
// meantime keeps the attachment alive.
func DeleteUnreferencedArticleAttachment(ctx context.Context, attachmentID int64) (bool, error) {
	if attachmentID == 0 {
		return false, nil
	}
	count, err := db.GetEngine(ctx).Table("attachment").
		Where(unreferencedArticleAttachmentCond()).
		And(builder.Eq{"attachment.id": attachmentID}).
		Delete(&Attachment{})
	return count > 0, err
}

// DeleteUnassociatedAttachment deletes the attachment row only if no repository
// keeps it alive through an article association, and reports whether it did.
//
// Unlike DeleteUnreferencedArticleAttachment it says nothing about the purpose
// or the linked unit of the row, so it also serves the uploader-initiated
// removal of an issue, release or still pending editor upload. The association
// condition is re-evaluated by the database as part of the delete, so a commit
// that associates the attachment between a caller's check and this call keeps
// it alive instead of losing its blob.
func DeleteUnassociatedAttachment(ctx context.Context, attachmentID int64) (bool, error) {
	if attachmentID == 0 {
		return false, nil
	}
	count, err := db.GetEngine(ctx).Table("attachment").
		Where(builder.Eq{"attachment.id": attachmentID}).
		And(noArticleAssociationCond()).
		Delete(&Attachment{})
	return count > 0, err
}

// MarkAttachmentsArticlePurpose records that attachments the backfill inferred
// to be article content are article uploads, so their lifetime follows the
// associations from now on and the garbage collector may eventually reclaim
// them. Rows that already carry a purpose, and rows an issue, comment or
// release claims, are left untouched.
func MarkAttachmentsArticlePurpose(ctx context.Context, attachmentIDs []int64) (int64, error) {
	if len(attachmentIDs) == 0 {
		return 0, nil
	}
	return db.GetEngine(ctx).Table("attachment").
		In("id", attachmentIDs).
		Where(builder.Eq{
			"purpose":    AttachmentPurposeUnspecified,
			"issue_id":   0,
			"comment_id": 0,
			"release_id": 0,
		}).
		Cols("purpose").
		Update(&Attachment{Purpose: AttachmentPurposeArticle})
}

// CountUnassociatedLegacyAttachments returns how many attachments of
// unspecified purpose no repository keeps alive. They are the rows that the
// legacy read fallback, and nothing else, still authorizes, so the count is
// what a backfill verification reports as remaining exposure.
func CountUnassociatedLegacyAttachments(ctx context.Context) (int64, error) {
	return db.GetEngine(ctx).Table("attachment").
		Where(unassociatedAttachmentCond(AttachmentPurposeUnspecified)).
		Count(new(Attachment))
}

// CountDanglingArticleAttachments returns how many associations point at an
// attachment row that no longer exists. Reconciliation reports them rather than
// deleting them: an association is only ever removed with its repository, and a
// dangling row means something deleted an attachment out from under it.
func CountDanglingArticleAttachments(ctx context.Context) (int64, error) {
	return db.GetEngine(ctx).Table("article_attachment").
		Where(builder.NotExists(
			builder.Select("1").From("attachment").
				Where(builder.Expr("attachment.id = article_attachment.attachment_id")),
		)).
		Count(new(ArticleAttachment))
}

// RetainedRepoAttachmentIDs returns the attachments uploaded to a repository
// that must survive its deletion: article uploads, whose lifetime is governed by
// the associations and the garbage collector, and anything another repository
// still keeps alive.
func RetainedRepoAttachmentIDs(ctx context.Context, repoID int64) ([]int64, error) {
	ids := make([]int64, 0, 8)
	if repoID == 0 {
		return ids, nil
	}
	return ids, db.GetEngine(ctx).Table("attachment").
		Where(builder.Eq{"attachment.repo_id": repoID}).
		And(builder.Eq{"attachment.purpose": AttachmentPurposeArticle}.Or(
			builder.Exists(
				builder.Select("1").From("article_attachment").
					Where(builder.Expr("article_attachment.attachment_id = attachment.id")),
			),
		)).
		Cols("attachment.id").
		Find(&ids)
}
