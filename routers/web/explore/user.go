// Copyright 2021 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package explore

import (
	"bytes"
	"net/http"

	"code.gitea.io/gitea/models/db"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/container"
	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/optional"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/sitemap"
	"code.gitea.io/gitea/modules/structs"
	"code.gitea.io/gitea/modules/templates"
	"code.gitea.io/gitea/modules/util"
	"code.gitea.io/gitea/services/context"
)

const (
	// tplExploreUsers explore users page template
	tplExploreUsers templates.TplName = "explore/users"
)

var nullByte = []byte{0x00}

func isKeywordValid(keyword string) bool {
	return !bytes.Contains([]byte(keyword), nullByte)
}

// maxExactUserMatches bounds the exact-match lookup. Usernames are unique, so this only
// ever matters for a full name several accounts share; a page listing more than this many
// identically named accounts is not a page the split can usefully organise anyway.
const maxExactUserMatches = 50

// splitExactUserMatch separates the users the search keyword actually names -- those whose
// username or full name is exactly the keyword -- from the rest of the current result page.
//
// The exact matches get their own query instead of being picked out of "users" because the
// list is paginated and ordered by name or sign-up date, not relevance: the user named
// exactly like the keyword can sit on any page, and someone searching for "anastasia"
// should find her on the first one. Reusing "opts" makes the lookup inherit every filter
// the page applies (user type, active, visibility to the viewer, repo role), so it can
// never surface a user the paginated list would have hidden.
//
// All exact matches are returned, not just the first: "lower_name" is unique but
// "full_name" is not, so a name two accounts share would otherwise promote one of them
// arbitrarily and file its twin under "Similar", as though it were a near miss.
//
// Any exact match that also falls on the current page is dropped from the returned
// "similar" list so it is not rendered twice. That leaves the page a row shorter; the
// pagination total is deliberately left untouched so page boundaries stay stable while
// browsing.
func splitExactUserMatch(ctx *context.Context, opts user_model.SearchUserOptions, users []*user_model.User) ([]*user_model.User, []*user_model.User, error) {
	// Same guard as the paginated search above: a keyword it refused to run is not
	// worth a second query either.
	if opts.Keyword == "" || !isKeywordValid(opts.Keyword) {
		return nil, users, nil
	}

	exactOpts := opts
	exactOpts.ExactMatchOnly = true
	exactOpts.ListOptions = db.ListOptions{Page: 1, PageSize: maxExactUserMatches}
	exactMatches, _, err := user_model.SearchUsers(ctx, exactOpts)
	if err != nil {
		return nil, nil, err
	}
	if len(exactMatches) == 0 {
		return nil, users, nil
	}

	exactIDs := make(container.Set[int64], len(exactMatches))
	for _, u := range exactMatches {
		exactIDs.Add(u.ID)
	}
	similar := make([]*user_model.User, 0, len(users))
	for _, u := range users {
		if !exactIDs.Contains(u.ID) {
			similar = append(similar, u)
		}
	}
	return exactMatches, similar, nil
}

// RenderUserSearch render user search page
func RenderUserSearch(ctx *context.Context, opts user_model.SearchUserOptions, tplName templates.TplName) {
	// Sitemap index for sitemap paths
	opts.Page = int(ctx.PathParamInt64("idx"))
	isSitemap := ctx.PathParam("idx") != ""
	if opts.Page <= 1 {
		opts.Page = ctx.FormInt("page")
	}
	if opts.Page <= 1 {
		opts.Page = 1
	}

	if isSitemap {
		opts.PageSize = setting.UI.SitemapPagingNum
	}

	var (
		users   []*user_model.User
		count   int64
		err     error
		orderBy db.SearchOrderBy
	)

	// we can not set orderBy to `models.SearchOrderByXxx`, because there may be a JOIN in the statement, different tables may have the same name columns

	sortOrder := ctx.FormString("sort")
	if sortOrder == "" {
		sortOrder = setting.UI.ExploreDefaultSort
	}
	ctx.Data["SortType"] = sortOrder

	switch sortOrder {
	case "newest":
		orderBy = "`user`.id DESC"
	case "oldest":
		orderBy = "`user`.id ASC"
	case "leastupdate":
		orderBy = "`user`.updated_unix ASC"
	case "reversealphabetically":
		orderBy = "`user`.name DESC"
	case "lastlogin":
		orderBy = "`user`.last_login_unix ASC"
	case "reverselastlogin":
		orderBy = "`user`.last_login_unix DESC"
	case "alphabetically":
		orderBy = "`user`.name ASC"
	case "recentupdate":
		fallthrough
	default:
		// in case the sortType is not valid, we set it to recentupdate
		sortOrder = "recentupdate"
		ctx.Data["SortType"] = "recentupdate"
		orderBy = "`user`.updated_unix DESC"
	}

	if opts.SupportedSortOrders != nil && !opts.SupportedSortOrders.Contains(sortOrder) {
		ctx.NotFound(nil)
		return
	}

	opts.Keyword = ctx.FormTrim("q")
	opts.OrderBy = orderBy
	if len(opts.Keyword) == 0 || isKeywordValid(opts.Keyword) {
		users, count, err = user_model.SearchUsers(ctx, opts)
		if err != nil {
			ctx.ServerError("SearchUsers", err)
			return
		}
	}
	if isSitemap {
		m := sitemap.NewSitemap()
		for _, item := range users {
			m.Add(sitemap.URL{URL: item.HTMLURL(ctx), LastMod: item.UpdatedUnix.AsTimePtr()})
		}
		ctx.Resp.Header().Set("Content-Type", "text/xml")
		if _, err := m.WriteTo(ctx.Resp); err != nil {
			log.Error("Failed writing sitemap: %v", err)
		}
		return
	}

	// Split the results the way the subjects tab does: the users the keyword actually
	// names go into their own "Search results for ..." section, everything else is
	// listed under "Similar" (#276).
	//
	// Only the users tab gets this. The organizations page renders the very same
	// template (explore.Organizations calls this function with tplExploreUsers), so
	// without the guard an org listing would answer a search with "No user named
	// exactly ...". The admin pages share the handler too, but their own templates
	// ignore these values either way.
	if ctx.Data["PageIsExploreUsers"] == true {
		exactMatches, similarUsers, err := splitExactUserMatch(ctx, opts, users)
		if err != nil {
			ctx.ServerError("SearchUsers (exact)", err)
			return
		}
		ctx.Data["HasSearchKeyword"] = opts.Keyword != ""
		ctx.Data["ExactMatches"] = exactMatches
		ctx.Data["SimilarUsers"] = similarUsers
	}

	ctx.Data["Keyword"] = opts.Keyword
	ctx.Data["Total"] = count
	ctx.Data["Users"] = users
	ctx.Data["UsersTwoFaStatus"] = user_model.UserList(users).GetTwoFaStatus(ctx)
	ctx.Data["ShowUserEmail"] = setting.UI.ShowUserEmail
	ctx.Data["IsRepoIndexerEnabled"] = setting.Indexer.RepoIndexerEnabled

	pager := context.NewPagination(int(count), opts.PageSize, opts.Page, 5)
	pager.AddParamFromRequest(ctx.Req)
	ctx.Data["Page"] = pager

	ctx.HTML(http.StatusOK, tplName)
}

// Users render explore users page
func Users(ctx *context.Context) {
	if setting.Service.Explore.DisableUsersPage {
		ctx.Redirect(setting.AppSubURL + "/explore")
		return
	}
	ctx.Data["OrganizationsPageIsDisabled"] = setting.Service.Explore.DisableOrganizationsPage
	ctx.Data["CodePageIsDisabled"] = setting.Service.Explore.DisableCodePage
	ctx.Data["Title"] = ctx.Tr("explore")
	ctx.Data["PageIsExplore"] = true
	ctx.Data["PageIsExploreUsers"] = true
	ctx.Data["IsRepoIndexerEnabled"] = setting.Indexer.RepoIndexerEnabled

	supportedSortOrders := container.SetOf(
		"newest",
		"oldest",
		"alphabetically",
		"reversealphabetically",
	)
	sortOrder := ctx.FormString("sort")
	if sortOrder == "" {
		sortOrder = util.Iif(supportedSortOrders.Contains(setting.UI.ExploreDefaultSort), setting.UI.ExploreDefaultSort, "newest")
		ctx.SetFormString("sort", sortOrder)
	}

	repoRole := user_model.RepoRole(ctx.FormString("repo_role"))
	switch repoRole {
	case user_model.RepoRoleOwner, user_model.RepoRoleContributor, user_model.RepoRoleNeither:
		// valid, keep as-is
	default:
		repoRole = ""
	}
	ctx.Data["RepoRole"] = string(repoRole)

	RenderUserSearch(ctx, user_model.SearchUserOptions{
		Actor:       ctx.Doer,
		Type:        user_model.UserTypeIndividual,
		ListOptions: db.ListOptions{PageSize: setting.UI.ExplorePagingNum},
		IsActive:    optional.Some(true),
		Visible:     []structs.VisibleType{structs.VisibleTypePublic, structs.VisibleTypeLimited, structs.VisibleTypePrivate},
		RepoRole:    repoRole,

		SupportedSortOrders: supportedSortOrders,
	}, tplExploreUsers)
}
