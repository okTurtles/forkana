// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"net/http"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/modules/templates"
	"code.gitea.io/gitea/services/context"
)

// tplSubjectFollow is the subject header's Follow button (#165).
const tplSubjectFollow templates.TplName = "repo/subject_follow"

// ActionSubjectFollow follows (action "follow") or stops following ("unfollow") the subject
// of the id in the path, and answers with the Follow button in its new state. Following a
// subject feeds the user's dashboard with the activity on its articles; it is independent
// of watching an article (ActionWatch), which sends notifications.
func ActionSubjectFollow(ctx *context.Context) {
	subject, err := repo_model.GetSubjectByID(ctx, ctx.PathParamInt64("id"))
	if err != nil {
		if repo_model.IsErrSubjectNotExist(err) {
			ctx.NotFound(err)
		} else {
			ctx.ServerError("GetSubjectByID", err)
		}
		return
	}

	if err := repo_model.FollowSubject(ctx, ctx.Doer.ID, subject.ID, ctx.PathParam("action") == "follow"); err != nil {
		ctx.ServerError("FollowSubject", err)
		return
	}

	context.SetSubjectFollowData(ctx, subject.ID)
	ctx.HTML(http.StatusOK, tplSubjectFollow)
}
