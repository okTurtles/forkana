// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"context"
	"strings"

	"code.gitea.io/gitea/models/db"
	"code.gitea.io/gitea/models/unit"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/structs"

	"xorm.io/builder"
)

// readableArticleCond selects the articles of a subject the doer (nil when signed out) can
// open: repositories they can access (AccessibleRepositoryCondition, which hides the public
// repositories of private and, signed out, limited organizations), whose owner is not a
// private user (nor, signed out, a limited one), that are not deleted (tombstones) and have
// content.
func readableArticleCond(doer *user_model.User) builder.Cond {
	cond := AccessibleRepositoryCondition(doer, unit.TypeInvalid).And(builder.Eq{
		"`repository`.is_tombstoned": false,
		"`repository`.is_empty":      false,
	})
	if doer == nil || !doer.IsAdmin {
		// signed out, only public owners are visible (HasOrgOrUserVisible)
		hiddenVisibilities := []structs.VisibleType{structs.VisibleTypePrivate}
		if doer == nil {
			hiddenVisibilities = append(hiddenVisibilities, structs.VisibleTypeLimited)
		}
		hiddenUsers := builder.Select("id").From("`user`").Where(builder.Eq{
			"type": user_model.UserTypeIndividual,
		}.And(builder.In("visibility", hiddenVisibilities)))
		ownerVisible := builder.NotIn("`repository`.owner_id", hiddenUsers)
		if doer != nil {
			ownerVisible = ownerVisible.Or(builder.Eq{"`repository`.owner_id": doer.ID})
		}
		cond = cond.And(ownerVisible)
	}
	return cond
}

// CountReadableArticlesBySubjectIDs returns, for each of the subjects, how many articles the
// doer can open in it (see readableArticleCond); subjects without any are left out.
func CountReadableArticlesBySubjectIDs(ctx context.Context, subjectIDs []int64, doer *user_model.User) (map[int64]int64, error) {
	counts := make(map[int64]int64, len(subjectIDs))
	if len(subjectIDs) == 0 {
		return counts, nil
	}
	var rows []struct {
		SubjectID int64
		Count     int64
	}
	err := db.GetEngine(ctx).Table("repository").
		Select("`repository`.subject_id, COUNT(*) AS count").
		Where(builder.In("`repository`.subject_id", subjectIDs).And(readableArticleCond(doer))).
		GroupBy("`repository`.subject_id").
		Find(&rows)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		counts[r.SubjectID] = r.Count
	}
	return counts, nil
}

// maxArticleOwners bounds the authors listed for a subject.
const maxArticleOwners = 50

// FindReadableArticleOwners returns the owners of the articles the doer can open in the
// subject (see readableArticleCond), the most recently updated first, each once. Owners whose
// articles there are all archived come last. The grouping is done by the database, so the
// limit counts owners, not articles: an owner with many articles on the subject cannot push
// the others out.
func FindReadableArticleOwners(ctx context.Context, subjectID int64, doer *user_model.User) ([]string, error) {
	return findReadableArticleOwners(ctx, subjectID, doer, maxArticleOwners)
}

func findReadableArticleOwners(ctx context.Context, subjectID int64, doer *user_model.User, limit int) ([]string, error) {
	var rows []struct {
		OwnerID   int64
		OwnerName string
	}
	err := db.GetEngine(ctx).Table("repository").
		Select("`repository`.owner_id, `repository`.owner_name").
		Where(builder.Eq{"`repository`.subject_id": subjectID}.And(readableArticleCond(doer))).
		GroupBy("`repository`.owner_id, `repository`.owner_name").
		// MIN over a CASE rather than over is_archived: PostgreSQL has no MIN(boolean)
		OrderBy("MIN(CASE WHEN `repository`.is_archived = ? THEN 1 ELSE 0 END) ASC, "+
			"MAX(`repository`.updated_unix) DESC, `repository`.owner_id DESC", true).
		Limit(limit).
		Find(&rows)
	if err != nil {
		return nil, err
	}
	owners := make([]string, 0, len(rows))
	for _, r := range rows {
		owners = append(owners, r.OwnerName)
	}
	return owners, nil
}

// HasReadableArticle reports whether the subject has an article the doer can open (see
// readableArticleCond), by ownerName when it is not empty. The owner is matched by its
// current name, as the link resolves it.
func HasReadableArticle(ctx context.Context, subjectID int64, ownerName string, doer *user_model.User) (bool, error) {
	cond := builder.Eq{"`repository`.subject_id": subjectID}.And(readableArticleCond(doer))
	if ownerName != "" {
		cond = cond.And(builder.In("`repository`.owner_id",
			builder.Select("id").From("`user`").Where(builder.Eq{"lower_name": strings.ToLower(ownerName)})))
	}
	return db.GetEngine(ctx).Table("repository").Where(cond).Exist()
}
