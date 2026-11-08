// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"context"

	"code.gitea.io/gitea/models/db"
	"code.gitea.io/gitea/models/unit"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/timeutil"

	"xorm.io/builder"
)

// SubjectFollow is a user following a subject (#165). Following a subject only feeds the
// user's dashboard: the activity on every article of the subject, new articles and forks
// included, shows in their feed (see activities.GetFeedsOptions.IncludeFollowedSubjects).
// It sends no notification and no email: those come from watching an article (Watch).
type SubjectFollow struct {
	ID          int64              `xorm:"pk autoincr"`
	UserID      int64              `xorm:"UNIQUE(subject_follow) NOT NULL"`
	SubjectID   int64              `xorm:"UNIQUE(subject_follow) INDEX NOT NULL"`
	CreatedUnix timeutil.TimeStamp `xorm:"INDEX created"`
}

func init() {
	db.RegisterModel(new(SubjectFollow))
}

// TableName returns the table name for SubjectFollow
func (s *SubjectFollow) TableName() string {
	return "subject_follow"
}

// IsFollowingSubject reports whether the user follows the subject.
func IsFollowingSubject(ctx context.Context, userID, subjectID int64) (bool, error) {
	if userID <= 0 || subjectID <= 0 {
		return false, nil
	}
	return db.GetEngine(ctx).Exist(&SubjectFollow{UserID: userID, SubjectID: subjectID})
}

// FollowSubject makes the user follow the subject (follow true) or stop following it.
// Following a subject twice, or unfollowing one that is not followed, changes nothing; so
// does a follow that loses a race with the same follow (a double click), which the unique
// index rejects.
func FollowSubject(ctx context.Context, userID, subjectID int64, follow bool) error {
	if !follow {
		_, err := db.GetEngine(ctx).Delete(&SubjectFollow{UserID: userID, SubjectID: subjectID})
		return err
	}
	has, err := IsFollowingSubject(ctx, userID, subjectID)
	if err != nil || has {
		return err
	}
	if err = db.Insert(ctx, &SubjectFollow{UserID: userID, SubjectID: subjectID}); err != nil {
		// the same follow, inserted in the meantime
		if has, _ = IsFollowingSubject(ctx, userID, subjectID); has {
			return nil
		}
		return err
	}
	return nil
}

// FollowedSubjectReadableRepoCond selects, in the repository table, the articles (forks
// included) of every subject the user follows that the user can read.
func FollowedSubjectReadableRepoCond(user *user_model.User) builder.Cond {
	return builder.In("`repository`.subject_id",
		builder.Select("`subject_follow`.subject_id").From("`subject_follow`").
			Where(builder.Eq{"`subject_follow`.user_id": user.ID}),
	).And(AccessibleRepositoryCondition(user, unit.TypeInvalid))
}

// FindFollowedSubjectReadableRepoIDs returns the ids of up to limit repositories of
// FollowedSubjectReadableRepoCond.
func FindFollowedSubjectReadableRepoIDs(ctx context.Context, user *user_model.User, limit int) ([]int64, error) {
	ids := make([]int64, 0, limit)
	err := db.GetEngine(ctx).Table("repository").Where(FollowedSubjectReadableRepoCond(user)).
		Limit(limit).Cols("`repository`.id").Find(&ids)
	return ids, err
}

// deleteSubjectFollows removes every follow of the subject, once it is deleted, so that a
// later subject that got the same id is not followed by the readers of the deleted one.
func deleteSubjectFollows(ctx context.Context, subjectID int64) error {
	_, err := db.GetEngine(ctx).Where("subject_id = ?", subjectID).Delete(new(SubjectFollow))
	return err
}
