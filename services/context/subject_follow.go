// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package context

import (
	"strconv"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/modules/setting"
)

// SubjectFollowLink is where the subject header's Follow button posts, followed by
// "/follow" or "/unfollow" (routers/web/repo.ActionSubjectFollow).
func SubjectFollowLink(subjectID int64) string {
	return setting.AppSubURL + "/-/subject/" + strconv.FormatInt(subjectID, 10)
}

// SetSubjectFollowData gives the subject header's Follow button (repo/subject_follow,
// #165) the subject it follows and whether the reader follows it. A repository without
// a subject has no Follow.
func SetSubjectFollowData(ctx *Context, subjectID int64) {
	if subjectID <= 0 {
		return
	}
	ctx.Data["SubjectFollowLink"] = SubjectFollowLink(subjectID)
	if ctx.IsSigned {
		ctx.Data["IsFollowingSubject"] = repo_model.IsFollowingSubject(ctx, ctx.Doer.ID, subjectID)
	}
}
