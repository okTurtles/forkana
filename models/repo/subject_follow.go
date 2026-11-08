// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"context"

	"code.gitea.io/gitea/models/db"
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
func IsFollowingSubject(ctx context.Context, userID, subjectID int64) bool {
	if userID <= 0 || subjectID <= 0 {
		return false
	}
	has, _ := db.GetEngine(ctx).Exist(&SubjectFollow{UserID: userID, SubjectID: subjectID})
	return has
}

// FollowSubject makes the user follow the subject (follow true) or stop following it.
// Following a subject twice, or unfollowing one that is not followed, changes nothing.
func FollowSubject(ctx context.Context, userID, subjectID int64, follow bool) error {
	return db.WithTx(ctx, func(ctx context.Context) error {
		if !follow {
			_, err := db.GetEngine(ctx).Delete(&SubjectFollow{UserID: userID, SubjectID: subjectID})
			return err
		}
		if IsFollowingSubject(ctx, userID, subjectID) {
			return nil
		}
		return db.Insert(ctx, &SubjectFollow{UserID: userID, SubjectID: subjectID})
	})
}

// CountSubjectFollowers returns the number of users following the subject.
func CountSubjectFollowers(ctx context.Context, subjectID int64) (int64, error) {
	return db.GetEngine(ctx).Where("subject_id = ?", subjectID).Count(new(SubjectFollow))
}

// FollowedSubjectRepoIDsQuery selects the ids of the repositories (articles, forks
// included) of every subject the user follows.
func FollowedSubjectRepoIDsQuery(userID int64) *builder.Builder {
	return builder.Select("`repository`.id").From("`repository`").Where(
		builder.In("`repository`.subject_id",
			builder.Select("`subject_follow`.subject_id").From("`subject_follow`").
				Where(builder.Eq{"`subject_follow`.user_id": userID}),
		),
	)
}

// deleteSubjectFollows removes every follow of the subject, once it is deleted, so that a
// later subject that got the same id is not followed by the readers of the deleted one.
func deleteSubjectFollows(ctx context.Context, subjectID int64) error {
	_, err := db.GetEngine(ctx).Where("subject_id = ?", subjectID).Delete(new(SubjectFollow))
	return err
}
