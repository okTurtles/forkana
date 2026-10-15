// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package setting

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"code.gitea.io/gitea/models/db"
	repo_model "code.gitea.io/gitea/models/repo"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/htmlutil"
	"code.gitea.io/gitea/modules/optional"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/util"
	"code.gitea.io/gitea/services/context"
	convert_service "code.gitea.io/gitea/services/convert"
	repo_service "code.gitea.io/gitea/services/repository"
)

// articleCollaboratorsURL is where the collaborator actions report back, the
// collaborators section of the article settings.
func articleCollaboratorsURL(ctx *context.Context) string {
	return articleSettingsURL(ctx) + "&section=collaborators"
}

// requireArticleCollaboratorManager answers 404 unless the doer may manage the collaborators
// of the current article: the article settings are not reachable for them either. Each
// action then checks the finer rules (which role, which collaborator) in the service.
func requireArticleCollaboratorManager(ctx *context.Context) bool {
	p, err := repo_service.GetArticleCollaboratorPermission(ctx, ctx.Doer, ctx.Repo.Repository)
	if err != nil {
		ctx.ServerError("GetArticleCollaboratorPermission", err)
		return false
	}
	if !p.CanManage {
		ctx.NotFound(nil)
		return false
	}
	return true
}

// ArticleCollaboratorCandidates searches the users who can be invited to the current
// article, leaving out its owner, the users already listed and everyone who owns an
// active article on the same subject.
func ArticleCollaboratorCandidates(ctx *context.Context) {
	if !requireArticleCollaboratorManager(ctx) {
		return
	}

	repo := ctx.Repo.Repository
	listedIDs, err := repo_model.GetArticleCollaboratorUserIDs(ctx, repo.ID)
	if err != nil {
		ctx.ServerError("GetArticleCollaboratorUserIDs", err)
		return
	}

	users, _, err := user_model.SearchUsers(ctx, user_model.SearchUserOptions{
		Actor:                    ctx.Doer,
		Keyword:                  ctx.FormTrim("q"),
		Type:                     user_model.UserTypeIndividual,
		IsActive:                 optional.Some(true),
		ExcludeUserIDs:           append(listedIDs, repo.OwnerID),
		ExcludeOwnersOfSubjectID: repo.SubjectID,
		ListOptions:              db.ListOptions{PageSize: setting.UI.MembersPagingNum},
	})
	if err != nil {
		ctx.ServerError("SearchUsers", err)
		return
	}
	ctx.JSON(http.StatusOK, map[string]any{"data": convert_service.ToUsers(ctx, ctx.Doer, users)})
}

// flashArticleCollaboratorError reports a refused collaborator action through a flash
// message. It returns false when the error is unexpected and the response was written.
func flashArticleCollaboratorError(ctx *context.Context, err error) bool {
	switch {
	case errors.Is(err, repo_service.ErrArticleArchived):
		repo := ctx.Repo.Repository
		archivedAt := util.Iif(repo.ArchivedUnix.IsZero(), repo.UpdatedUnix, repo.ArchivedUnix)
		ctx.Flash.Error(ctx.Tr("repo.settings.article_archived_notice", archivedAt.FormatDate()))
	case errors.Is(err, repo_service.ErrArticleCollaboratorExists):
		ctx.Flash.Error(ctx.Tr("repo.settings.article_collaborators_already_listed"))
	case errors.Is(err, repo_service.ErrArticleCollaboratorOwnsSubject):
		ctx.Flash.Error(ctx.Tr("repo.settings.article_collaborators_owns_subject"))
	case errors.Is(err, repo_service.ErrArticleCollaboratorNotEligible), errors.Is(err, user_model.ErrBlockedUser):
		ctx.Flash.Error(ctx.Tr("repo.settings.article_collaborators_not_eligible"))
	case errors.Is(err, repo_service.ErrArticleCollaboratorNoInvite):
		ctx.Flash.Error(ctx.Tr("repo.settings.article_collaborators_no_invite"))
	case errors.Is(err, util.ErrPermissionDenied):
		ctx.Flash.Error(ctx.Tr("repo.settings.article_collaborators_no_permission"))
	case errors.Is(err, util.ErrNotExist), errors.Is(err, util.ErrInvalidArgument):
		ctx.Flash.Error(ctx.Tr("repo.settings.article_collaborators_user_not_found"))
	default:
		ctx.ServerError("ArticleCollaborator", err)
		return false
	}
	return true
}

// resolveArticleCollaboratorInvitee finds the invitee by the username the invite modal
// submits. Only users the doer can see are accepted, so that a crafted request cannot
// reach a hidden account or confirm that it exists. It reports the failure through a flash
// message and returns nil when the name is unusable.
func resolveArticleCollaboratorInvitee(ctx *context.Context, name string) *user_model.User {
	name = strings.TrimSpace(name)
	if name == "" {
		ctx.Flash.Error(ctx.Tr("repo.settings.article_collaborators_user_not_found"))
		return nil
	}

	invitee, err := user_model.GetUserByName(ctx, name)
	if err != nil {
		if !user_model.IsErrUserNotExist(err) {
			ctx.ServerError("GetUserByName", err)
			return nil
		}
		ctx.Flash.Error(ctx.Tr("repo.settings.article_collaborators_user_not_found"))
		return nil
	}
	if !user_model.IsUserVisibleToViewer(ctx, invitee, ctx.Doer) {
		ctx.Flash.Error(ctx.Tr("repo.settings.article_collaborators_user_not_found"))
		return nil
	}
	return invitee
}

// ArticleCollaboratorInvite invites a user to maintain the current article.
func ArticleCollaboratorInvite(ctx *context.Context) {
	if !requireArticleCollaboratorManager(ctx) {
		return
	}
	redirectURL := articleCollaboratorsURL(ctx)

	invitee := resolveArticleCollaboratorInvitee(ctx, ctx.FormString("invitee_name"))
	if invitee == nil {
		if !ctx.Written() {
			ctx.Redirect(redirectURL)
		}
		return
	}

	role := repo_model.ArticleCollaboratorRole(ctx.FormInt("role"))
	err := repo_service.InviteArticleCollaborator(ctx, ctx.Doer, ctx.Repo.Repository, invitee, role)
	if err != nil {
		if flashArticleCollaboratorError(ctx, err) {
			ctx.Redirect(redirectURL)
		}
		return
	}

	inviteeLink := htmlutil.HTMLFormat(`<a href="%s">%s</a>`, invitee.HomeLink(), invitee.DisplayName())
	ctx.Flash.Success(ctx.Tr("repo.settings.article_collaborators_invite_success", inviteeLink))
	ctx.Redirect(redirectURL)
}

// ArticleCollaboratorChangeRole changes the role of a collaborator, it answers the role
// dropdown's fetch request with JSON.
func ArticleCollaboratorChangeRole(ctx *context.Context) {
	if !requireArticleCollaboratorManager(ctx) {
		return
	}

	role := repo_model.ArticleCollaboratorRole(ctx.FormInt("role"))
	uid := ctx.FormInt64("uid")
	err := repo_service.ChangeArticleCollaboratorRole(ctx, ctx.Doer, ctx.Repo.Repository, uid, role)
	switch {
	case err == nil:
		collaborator, err := user_model.GetUserByID(ctx, uid)
		if err != nil {
			ctx.ServerError("GetUserByID", err)
			return
		}
		// Tr escapes the name, the message is HTML
		ctx.JSON(http.StatusOK, map[string]any{
			"message": ctx.Tr("repo.settings.article_collaborators_role_success", collaborator.DisplayName()),
		})
	case errors.Is(err, util.ErrPermissionDenied):
		ctx.JSONError(ctx.Tr("repo.settings.article_collaborators_no_permission"))
	case errors.Is(err, util.ErrNotExist), errors.Is(err, util.ErrInvalidArgument):
		ctx.JSONError(ctx.Tr("repo.settings.article_collaborators_user_not_found"))
	default:
		ctx.ServerError("ChangeArticleCollaboratorRole", err)
	}
}

// ArticleCollaboratorsRemove removes the selected collaborators, one from a row's button
// or several from the "Remove selected" action.
func ArticleCollaboratorsRemove(ctx *context.Context) {
	if !requireArticleCollaboratorManager(ctx) {
		return
	}
	redirectURL := articleCollaboratorsURL(ctx)

	values := ctx.FormStrings("uid")
	userIDs := make([]int64, 0, len(values))
	for _, value := range values {
		if id, err := strconv.ParseInt(value, 10, 64); err == nil && id > 0 {
			userIDs = append(userIDs, id)
		}
	}

	if err := repo_service.RemoveArticleCollaborators(ctx, ctx.Doer, ctx.Repo.Repository, userIDs); err != nil {
		if flashArticleCollaboratorError(ctx, err) {
			ctx.Redirect(redirectURL)
		}
		return
	}
	if len(userIDs) > 0 {
		ctx.Flash.Success(ctx.Tr("repo.settings.article_collaborators_remove_success"))
	}
	ctx.Redirect(redirectURL)
}

// ArticleCollaboratorAccept accepts the doer's pending invitation to the current article.
func ArticleCollaboratorAccept(ctx *context.Context) {
	redirectURL := ctx.Repo.Repository.Link()
	err := repo_service.AcceptArticleCollaboratorInvite(ctx, ctx.Doer, ctx.Repo.Repository)
	if err != nil {
		if errors.Is(err, repo_service.ErrArticleCollaboratorOwnsSubject) {
			ctx.Flash.Error(ctx.Tr("repo.settings.article_collaborators_accept_owns_subject"))
			ctx.Redirect(redirectURL)
		} else if flashArticleCollaboratorError(ctx, err) {
			ctx.Redirect(redirectURL)
		}
		return
	}
	ctx.Flash.Success(ctx.Tr("repo.settings.article_collaborators_invite_accepted"))
	ctx.Redirect(redirectURL)
}

// ArticleCollaboratorDecline declines the doer's pending invitation to the current article.
func ArticleCollaboratorDecline(ctx *context.Context) {
	redirectURL := ctx.Repo.Repository.Link()
	if err := repo_service.DeclineArticleCollaboratorInvite(ctx, ctx.Doer, ctx.Repo.Repository); err != nil {
		if flashArticleCollaboratorError(ctx, err) {
			ctx.Redirect(redirectURL)
		}
		return
	}
	ctx.Flash.Success(ctx.Tr("repo.settings.article_collaborators_invite_declined"))
	ctx.Redirect(redirectURL)
}
