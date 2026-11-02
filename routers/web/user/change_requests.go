// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package user

import (
	"net/http"
	"slices"
	"strings"

	"code.gitea.io/gitea/models/db"
	issues_model "code.gitea.io/gitea/models/issues"
	"code.gitea.io/gitea/models/unit"
	user_model "code.gitea.io/gitea/models/user"
	issue_indexer "code.gitea.io/gitea/modules/indexer/issues"
	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/markup/mdstripper"
	"code.gitea.io/gitea/modules/optional"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/templates"
	"code.gitea.io/gitea/modules/timeutil"
	"code.gitea.io/gitea/modules/util"
	"code.gitea.io/gitea/routers/common"
	"code.gitea.io/gitea/services/context"
)

const tplChangeRequests templates.TplName = "user/dashboard/change_requests"

// The states a change request can be filtered by (the Filter menu, Figma 7368:275387).
const (
	ChangeRequestOpen   = "open"
	ChangeRequestMerged = "merged"
	ChangeRequestClosed = "closed"
)

var changeRequestStates = []string{ChangeRequestOpen, ChangeRequestMerged, ChangeRequestClosed}

// changeRequestSorts are the Sort menu's entries (Figma 7368:275446), in its order,
// with the sort types of the issue search they map to.
var changeRequestSorts = []string{"recentupdate", "leastupdate", "latest", "oldest", "mostcomment", "leastcomment", "nearduedate", "farduedate"}

// mentionQuoteLength caps the excerpt of the comment that mentions the user.
const mentionQuoteLength = 160

// ChangeRequestRow is one change request of the hub's list.
type ChangeRequestRow struct {
	Issue *issues_model.Issue
	// State is open, merged or closed.
	State string
	// HasConflicts marks an open change request that no longer merges cleanly.
	HasConflicts bool
	// Who leads the row (its avatar and bold name): the author on Received, the
	// article's owner on Sent, whoever mentioned the user on Mentions.
	Who *user_model.User
	// Article is the name of the article (the subject) the change request is for.
	Article string
	// Quote is, on Mentions, an excerpt of the text that mentions the user.
	Quote string
	// When is the time shown: the last update, or on Mentions when the user was mentioned.
	When timeutil.TimeStamp
}

// changeRequestState is open, merged or closed.
func changeRequestState(issue *issues_model.Issue) string {
	switch {
	case issue.PullRequest != nil && issue.PullRequest.HasMerged:
		return ChangeRequestMerged
	case issue.IsClosed:
		return ChangeRequestClosed
	default:
		return ChangeRequestOpen
	}
}

// parseChangeRequestStates reads the comma-separated state filter. Unknown states are
// dropped; no state, like every state, means no filter.
func parseChangeRequestStates(s string) []string {
	var states []string
	for _, state := range changeRequestStates {
		if slices.Contains(strings.Split(s, ","), state) {
			states = append(states, state)
		}
	}
	if len(states) == len(changeRequestStates) {
		return nil
	}
	return states
}

// toggleChangeRequestState returns the state filter with state switched on or off.
func toggleChangeRequestState(states []string, state string) string {
	var res []string
	for _, s := range changeRequestStates {
		if (s == state) != slices.Contains(states, s) {
			res = append(res, s)
		}
	}
	return strings.Join(res, ",")
}

// searchChangeRequests returns one page of the change requests matching opts and the
// keyword in the given states, and how many there are in all. The issue search only
// tells open from closed, so when merged and closed ones are told apart it searches
// every candidate and filters them here.
func searchChangeRequests(ctx *context.Context, keyword string, opts *issues_model.IssuesOptions, states []string, page, pageSize int) ([]int64, int64, error) {
	open := slices.Contains(states, ChangeRequestOpen)
	merged := slices.Contains(states, ChangeRequestMerged)
	closed := slices.Contains(states, ChangeRequestClosed)
	if len(states) > 0 && !open {
		opts.IsClosed = optional.Some(true)
	} else if open && !merged && !closed {
		opts.IsClosed = optional.Some(false)
	}

	if merged == closed {
		opts.Paginator = &db.ListOptions{Page: page, PageSize: pageSize}
		return issue_indexer.SearchIssues(ctx, issue_indexer.ToSearchOptions(keyword, opts))
	}

	// (a PageSize, because the database indexer reads a zero PageSize as "count only")
	opts.Paginator = &db.ListOptions{ListAll: true, PageSize: pageSize}
	ids, _, err := issue_indexer.SearchIssues(ctx, issue_indexer.ToSearchOptions(keyword, opts))
	if err != nil {
		return nil, 0, err
	}
	candidates, err := issues_model.GetIssuesByIDs(ctx, ids, true)
	if err != nil {
		return nil, 0, err
	}
	if err := candidates.LoadPullRequests(ctx); err != nil {
		return nil, 0, err
	}
	matching := make([]int64, 0, len(candidates))
	for _, issue := range candidates {
		if slices.Contains(states, changeRequestState(issue)) {
			matching = append(matching, issue.ID)
		}
	}
	start := min((page-1)*pageSize, len(matching))
	end := min(start+pageSize, len(matching))
	return matching[start:end], int64(len(matching)), nil
}

// ChangeRequests renders the change requests hub (#94): the change requests to the
// user's articles (Received), the ones the user opened (Sent) and the ones mentioning
// the user (Mentions).
func ChangeRequests(ctx *context.Context) {
	if unit.TypePullRequests.UnitGlobalDisabled() {
		log.Debug("Change requests hub not available as change requests are globally disabled.")
		ctx.Status(http.StatusNotFound)
		return
	}

	tab := ctx.FormString("tab")
	switch ctx.FormString("type") { // the filters of Gitea's dashboard, which this page replaces
	case "created_by":
		tab = common.ChangeRequestsSent
	case "mentioned":
		tab = common.ChangeRequestsMentions
	}
	if !slices.Contains(common.ChangeRequestsTabs, tab) {
		tab = common.ChangeRequestsReceived
	}
	sortType := ctx.FormString("sort")
	if !slices.Contains(changeRequestSorts, sortType) {
		sortType = changeRequestSorts[0]
	}
	states := parseChangeRequestStates(ctx.FormString("state"))
	keyword := strings.TrimSpace(ctx.FormString("q"))
	page := max(ctx.FormInt("page"), 1)
	pageSize := setting.UI.IssuePagingNum

	opts, err := common.ChangeRequestsOptions(ctx, ctx.Doer, tab)
	if err != nil {
		ctx.ServerError("ChangeRequestsOptions", err)
		return
	}
	opts.SortType = sortType
	ids, total, err := searchChangeRequests(ctx, keyword, opts, states, page, pageSize)
	if err != nil {
		ctx.ServerError("searchChangeRequests", err)
		return
	}
	issues, err := issues_model.GetIssuesByIDs(ctx, ids, true)
	if err != nil {
		ctx.ServerError("GetIssuesByIDs", err)
		return
	}
	if err := issues.LoadAttributes(ctx); err != nil {
		ctx.ServerError("LoadAttributes", err)
		return
	}

	var mentions map[int64]*issues_model.Comment
	if tab == common.ChangeRequestsMentions {
		if mentions, err = issues_model.GetLatestMentionComments(ctx, ids, ctx.Doer.Name); err != nil {
			ctx.ServerError("GetLatestMentionComments", err)
			return
		}
	}

	rows := make([]*ChangeRequestRow, 0, len(issues))
	for _, issue := range issues {
		if err := issue.Repo.LoadOwner(ctx); err != nil {
			ctx.ServerError("LoadOwner", err)
			return
		}
		if err := issue.Repo.LoadSubject(ctx); err != nil {
			ctx.ServerError("LoadSubject", err)
			return
		}
		row := &ChangeRequestRow{
			Issue:   issue,
			State:   changeRequestState(issue),
			Who:     issue.Poster,
			Article: issue.Repo.GetSubject(ctx),
			When:    issue.UpdatedUnix,
		}
		row.HasConflicts = row.State == ChangeRequestOpen && issue.PullRequest != nil && issue.PullRequest.Status == issues_model.PullRequestStatusConflict
		switch tab {
		case common.ChangeRequestsSent:
			row.Who = issue.Repo.Owner
		case common.ChangeRequestsMentions:
			text := issue.Content
			row.When = issue.CreatedUnix
			if c, ok := mentions[issue.ID]; ok {
				row.Who, text, row.When = c.Poster, c.Content, c.CreatedUnix
			}
			stripped, _ := mdstripper.StripMarkdown([]byte(text))
			row.Quote = util.EllipsisDisplayString(strings.Join(strings.Fields(stripped), " "), mentionQuoteLength)
		}
		rows = append(rows, row)
	}

	counts := make(map[string]int64, len(common.ChangeRequestsTabs))
	for _, t := range common.ChangeRequestsTabs {
		if counts[t], err = common.CountOpenChangeRequests(ctx, ctx.Doer, t); err != nil {
			ctx.ServerError("CountOpenChangeRequests", err)
			return
		}
	}

	stateLinks := make(map[string]string, len(changeRequestStates))
	for _, s := range changeRequestStates {
		stateLinks[s] = toggleChangeRequestState(states, s)
	}

	pager := context.NewPagination(int(total), pageSize, page, 5)
	pager.AddParamFromRequest(ctx.Req)

	ctx.Data["Title"] = ctx.Tr("change_requests_hub.title")
	ctx.Data["PageIsChangeRequestsHub"] = true
	ctx.Data["Tab"] = tab
	ctx.Data["TabParam"] = util.Iif(tab == common.ChangeRequestsReceived, "", tab)
	ctx.Data["Tabs"] = common.ChangeRequestsTabs
	ctx.Data["TabCounts"] = counts
	ctx.Data["SortType"] = sortType
	ctx.Data["SortParam"] = util.Iif(sortType == changeRequestSorts[0], "", sortType)
	ctx.Data["Sorts"] = changeRequestSorts
	ctx.Data["States"] = changeRequestStates
	ctx.Data["SelectedStates"] = states
	ctx.Data["StateFilter"] = strings.Join(states, ",")
	ctx.Data["StateToggles"] = stateLinks
	ctx.Data["Keyword"] = keyword
	ctx.Data["Rows"] = rows
	ctx.Data["Total"] = total
	ctx.Data["ShownUpTo"] = (page-1)*pageSize + len(rows)
	ctx.Data["Page"] = pager

	ctx.HTML(http.StatusOK, tplChangeRequests)
}
