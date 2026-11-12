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
	"code.gitea.io/gitea/modules/timeutil"

	"xorm.io/builder"
)

// readableArticleCond selects the articles of a subject the doer (nil when signed out) can
// open and that have content (see openableArticleCond): those the listings show.
func readableArticleCond(doer *user_model.User) builder.Cond {
	return openableArticleCond(doer).And(builder.Eq{"`repository`.is_empty": false})
}

// openableArticleCond selects the articles of a subject the doer (nil when signed out) can
// open: repositories they can access (AccessibleRepositoryCondition, which hides the public
// repositories of private and, signed out, limited organizations), whose owner is not a
// private user (nor, signed out, a limited one), and that are not deleted (tombstones). A
// brand-new article, still empty, is one: its subject's pages render.
func openableArticleCond(doer *user_model.User) builder.Cond {
	cond := AccessibleRepositoryCondition(doer, unit.TypeInvalid).And(builder.Eq{
		"`repository`.is_tombstoned": false,
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

// SubjectListing is what a row of the subjects list shows besides the subject's name (#248):
// the summary (description) of the subject's original article, and when an article of the
// subject was last updated.
type SubjectListing struct {
	// Summary is the description of the subject's root article: the oldest non-fork,
	// non-empty article among those the doer can open, ties broken by id. Unlike
	// GetSubjectRootRepository it skips deleted (tombstoned) articles, so a deleted article
	// never lends its summary to the row. It is empty when that article has no summary or the
	// doer cannot open any non-fork article.
	Summary string
	// UpdatedUnix is the latest update of an article of the subject the doer can open.
	UpdatedUnix timeutil.TimeStamp
}

// FindSubjectListings loads, for each of the subjects, its SubjectListing as the doer (nil when
// signed out) sees it (see readableArticleCond), in two queries whatever the number of
// subjects. Subjects without any article the doer can open are left out.
func FindSubjectListings(ctx context.Context, subjectIDs []int64, doer *user_model.User) (map[int64]*SubjectListing, error) {
	listings := make(map[int64]*SubjectListing, len(subjectIDs))
	if len(subjectIDs) == 0 {
		return listings, nil
	}
	readable := builder.In("`repository`.subject_id", subjectIDs).And(readableArticleCond(doer))

	var updated []struct {
		SubjectID   int64
		UpdatedUnix timeutil.TimeStamp
	}
	if err := db.GetEngine(ctx).Table("repository").
		Select("`repository`.subject_id, MAX(`repository`.updated_unix) AS updated_unix").
		Where(readable).
		GroupBy("`repository`.subject_id").
		Find(&updated); err != nil {
		return nil, err
	}
	for _, u := range updated {
		listings[u.SubjectID] = &SubjectListing{UpdatedUnix: u.UpdatedUnix}
	}

	// The root articles: the readable non-fork articles of these subjects, oldest first, of
	// which the first of each subject is kept. A subject has one such article, a few at most.
	var roots []struct {
		SubjectID   int64
		Description string
	}
	if err := db.GetEngine(ctx).Table("repository").
		Select("`repository`.subject_id, `repository`.description").
		Where(readable.And(builder.Eq{"`repository`.is_fork": false})).
		OrderBy("`repository`.subject_id ASC, `repository`.created_unix ASC, `repository`.id ASC").
		Find(&roots); err != nil {
		return nil, err
	}
	seen := make(map[int64]bool, len(roots))
	for _, r := range roots {
		if seen[r.SubjectID] {
			continue
		}
		seen[r.SubjectID] = true
		if l := listings[r.SubjectID]; l != nil {
			l.Summary = strings.TrimSpace(r.Description)
		}
	}
	return listings, nil
}

// LoadSubjectListings sets the Listing of each of the subjects (see FindSubjectListings).
func LoadSubjectListings(ctx context.Context, subjects []*Subject, doer *user_model.User) error {
	ids := make([]int64, 0, len(subjects))
	for _, s := range subjects {
		ids = append(ids, s.ID)
	}
	listings, err := FindSubjectListings(ctx, ids, doer)
	if err != nil {
		return err
	}
	for _, s := range subjects {
		s.Listing = listings[s.ID]
	}
	return nil
}
