// Copyright 2021 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package explore

import (
	"code.gitea.io/gitea/models/db"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/container"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/structs"
	"code.gitea.io/gitea/modules/util"
	"code.gitea.io/gitea/services/context"
)

// Organizations render explore organizations page
func Organizations(ctx *context.Context) {
	if setting.Service.Explore.DisableOrganizationsPage {
		ctx.Redirect(setting.AppSubURL + "/explore")
		return
	}

	ctx.Data["UsersPageIsDisabled"] = setting.Service.Explore.DisableUsersPage
	ctx.Data["CodePageIsDisabled"] = setting.Service.Explore.DisableCodePage
	ctx.Data["Title"] = ctx.Tr("explore")
	ctx.Data["PageIsExplore"] = true
	ctx.Data["PageIsExploreOrganizations"] = true
	ctx.Data["IsRepoIndexerEnabled"] = setting.Indexer.RepoIndexerEnabled

	visibleTypes := []structs.VisibleType{structs.VisibleTypePublic}
	if ctx.Doer != nil {
		visibleTypes = append(visibleTypes, structs.VisibleTypeLimited, structs.VisibleTypePrivate)
	}

	supportedSortOrders := container.SetOf(
		"newest",
		"oldest",
		"alphabetically",
		"reversealphabetically",
	)
	// Remember what the request actually asked for before the form is rewritten below, so
	// RenderUserSearch can tell a real user selection from the default filled in here: the
	// page shares the users tab's sort dropdown, which must show nothing as selected until
	// the user selects it (#292).
	ctx.Data["RequestedSortType"] = ctx.FormString("sort")

	sortOrder := ctx.FormString("sort")
	if sortOrder == "" {
		sortOrder = util.Iif(supportedSortOrders.Contains(setting.UI.ExploreDefaultSort), setting.UI.ExploreDefaultSort, "newest")
		ctx.SetFormString("sort", sortOrder)
	}

	RenderUserSearch(ctx, user_model.SearchUserOptions{
		Actor:       ctx.Doer,
		Type:        user_model.UserTypeOrganization,
		ListOptions: db.ListOptions{PageSize: setting.UI.ExplorePagingNum},
		Visible:     visibleTypes,

		SupportedSortOrders: supportedSortOrders,
	}, tplExploreUsers)
}
