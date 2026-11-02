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
	changeRequestOpen   = "open"
	changeRequestMerged = "merged"
	changeRequestClosed = "closed"
)

// maxChangeRequestPage bounds the page asked for: past it the offsets would be absurd
// (and `?page=9223372036854775807` would overflow them).
const maxChangeRequestPage = 10_000

// maxFilteredChangeRequests bounds the search that tells merged from closed, which
// gathers every candidate to filter them here: past it, the total says "N+".
var maxFilteredChangeRequests = 5_000

var changeRequestStates = []string{changeRequestOpen, changeRequestMerged, changeRequestClosed}

// changeRequestSorts are the Sort menu's entries (Figma 7368:275446), in its order, as
// the issue search's sort types; the first is the default.
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
		return changeRequestMerged
	case issue.IsClosed:
		return changeRequestClosed
	default:
		return changeRequestOpen
	}
}

// parseChangeRequestStates reads the comma-separated state filter, in the Filter menu's
// order. Unknown states are dropped; no state, like every state, means no filter (the
// list is kept as it is, so every ticked box stays ticked).
func parseChangeRequestStates(s string) []string {
	var states []string
	parts := strings.Split(s, ",")
	for _, state := range changeRequestStates {
		if slices.Contains(parts, state) {
			states = append(states, state)
		}
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
// keyword in the given states, how many there are in all, and whether that total stopped
// at maxFilteredChangeRequests. The issue search only tells open from closed, so when
// merged and closed ones are told apart it gathers the first maxFilteredChangeRequests
// candidates and filters them here.
func searchChangeRequests(ctx *context.Context, keyword string, opts *issues_model.IssuesOptions, states []string, page, pageSize int) ([]int64, int64, bool, error) {
	open := slices.Contains(states, changeRequestOpen)
	merged := slices.Contains(states, changeRequestMerged)
	closed := slices.Contains(states, changeRequestClosed)
	if len(states) > 0 && !open {
		opts.IsClosed = optional.Some(true)
	} else if open && !merged && !closed {
		opts.IsClosed = optional.Some(false)
	}

	if merged == closed {
		opts.Paginator = &db.ListOptions{Page: page, PageSize: pageSize}
		ids, total, err := issue_indexer.SearchIssues(ctx, issue_indexer.ToSearchOptions(keyword, opts))
		return ids, total, false, err
	}

	opts.Paginator = &db.ListOptions{Page: 1, PageSize: maxFilteredChangeRequests}
	ids, _, err := issue_indexer.SearchIssues(ctx, issue_indexer.ToSearchOptions(keyword, opts))
	if err != nil {
		return nil, 0, false, err
	}
	capped := len(ids) >= maxFilteredChangeRequests
	// only the two flags, read in chunks: not the issues themselves in one IN (...)
	mergedIDs, err := issues_model.GetMergedPullIssueIDs(ctx, ids)
	if err != nil {
		return nil, 0, false, err
	}
	closedIDs, err := issues_model.GetClosedIssueIDs(ctx, ids)
	if err != nil {
		return nil, 0, false, err
	}
	matching := make([]int64, 0, len(ids))
	for _, id := range ids { // in the search's order
		state := changeRequestOpen
		if mergedIDs.Contains(id) {
			state = changeRequestMerged
		} else if closedIDs.Contains(id) {
			state = changeRequestClosed
		}
		if slices.Contains(states, state) {
			matching = append(matching, id)
		}
	}
	start := min((page-1)*pageSize, len(matching))
	end := min(start+pageSize, len(matching))
	return matching[start:end], int64(len(matching)), capped, nil
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
	page := min(max(ctx.FormInt("page"), 1), maxChangeRequestPage)
	pageSize := setting.UI.IssuePagingNum

	// the articles the user owns or maintains, looked up once for the list and every counter
	repoIDs, err := common.ChangeRequestsRepoIDs(ctx, ctx.Doer)
	if err != nil {
		ctx.ServerError("ChangeRequestsRepoIDs", err)
		return
	}
	opts := common.ChangeRequestsOptions(ctx.Doer, repoIDs, tab)
	opts.SortType = sortType
	ids, total, totalCapped, err := searchChangeRequests(ctx, keyword, opts, states, page, pageSize)
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
		row.HasConflicts = row.State == changeRequestOpen && issue.PullRequest != nil && issue.PullRequest.Status == issues_model.PullRequestStatusConflict
		switch tab {
		case common.ChangeRequestsSent:
			row.Who = issue.Repo.Owner
		case common.ChangeRequestsMentions:
			// the latest comment, review or code comment that mentions the user; else the
			// description, if it does; else (the mention was edited away) no quote
			text := ""
			row.When = issue.CreatedUnix
			if c, ok := mentions[issue.ID]; ok {
				row.Who, text, row.When = c.Poster, c.Content, c.CreatedUnix
			} else if issues_model.MentionsUser(issue.Content, ctx.Doer.Name) {
				text = issue.Content
			}
			if text != "" {
				stripped, _ := mdstripper.StripMarkdown([]byte(text))
				row.Quote = util.EllipsisDisplayString(strings.Join(strings.Fields(stripped), " "), mentionQuoteLength)
			}
		}
		rows = append(rows, row)
	}

	counts := make(map[string]int64, len(common.ChangeRequestsTabs))
	for _, t := range common.ChangeRequestsTabs {
		if counts[t], err = common.CountOpenChangeRequests(ctx, common.ChangeRequestsOptions(ctx.Doer, repoIDs, t)); err != nil {
			ctx.ServerError("CountOpenChangeRequests", err)
			return
		}
	}

	stateLinks := make(map[string]string, len(changeRequestStates))
	for _, s := range changeRequestStates {
		stateLinks[s] = toggleChangeRequestState(states, s)
	}

	pager := context.NewPagination(int(total), pageSize, page, 5)

	ctx.Data["Title"] = ctx.Tr("change_requests_hub.title")
	ctx.Data["PageIsChangeRequestsHub"] = true
	ctx.Data["Tab"] = tab
	ctx.Data["TabParam"] = util.Iif(tab == common.ChangeRequestsReceived, "", tab)
	ctx.Data["Tabs"] = common.ChangeRequestsTabs
	ctx.Data["TabCounts"] = counts
	// the navbar shows this count rather than counting it again
	ctx.Data["ChangeRequestsHubReceivedCount"] = counts[common.ChangeRequestsReceived]
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
	ctx.Data["TotalCapped"] = totalCapped
	ctx.Data["ShownUpTo"] = (page-1)*pageSize + len(rows)
	ctx.Data["Page"] = pager

	ctx.HTML(http.StatusOK, tplChangeRequests)
}
