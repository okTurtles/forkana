// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"strconv"

	issues_model "code.gitea.io/gitea/models/issues"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unit"
	user_model "code.gitea.io/gitea/models/user"
	issue_indexer "code.gitea.io/gitea/modules/indexer/issues"
	"code.gitea.io/gitea/modules/optional"

	"xorm.io/builder"
)

// The three tabs of the change requests hub (#94, Figma "09 · Changes hub" 7368:274515).
const (
	// ChangeRequestsReceived lists the change requests to the articles the user owns or maintains.
	ChangeRequestsReceived = "received"
	// ChangeRequestsSent lists the change requests the user opened.
	ChangeRequestsSent = "sent"
	// ChangeRequestsMentions lists the change requests that @-mention the user.
	ChangeRequestsMentions = "mentions"
)

// ChangeRequestsTabs are the hub's tabs in the order they are shown.
var ChangeRequestsTabs = []string{ChangeRequestsReceived, ChangeRequestsSent, ChangeRequestsMentions}

// changeRequestsRepoCond selects the articles the user owns or maintains (collaborates
// on), the repositories of the dashboard's "In your repositories" filter
// (routers/web/user/home.go buildIssueOverview).
func changeRequestsRepoCond(doer *user_model.User) builder.Cond {
	return repo_model.SearchRepositoryCondition(repo_model.SearchRepoOptions{
		Actor:       doer,
		OwnerID:     doer.ID,
		Private:     true,
		Collaborate: optional.None[bool](),
		UnitType:    unit.TypePullRequests,
		Archived:    optional.Some(false),
	})
}

// ChangeRequestsRepoIDs returns the IDs of the articles the user owns or maintains, to
// look up once per request and pass to ChangeRequestsOptions for every tab.
func ChangeRequestsRepoIDs(ctx context.Context, doer *user_model.User) ([]int64, error) {
	ids, err := repo_model.SearchRepositoryIDsByCondition(ctx, changeRequestsRepoCond(doer))
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		// no repos found, don't let the indexer return all repos
		ids = []int64{0}
	}
	return ids, nil
}

// ChangeRequestsOptions returns the search options of one tab of the change requests
// hub, in every state, built the way the dashboard's change request overview builds them
// (routers/web/user/home.go buildIssueOverview): Received is its "In your repositories"
// filter, Sent its "Created by you" and Mentions its "Mentioning you". repoIDs comes from
// ChangeRequestsRepoIDs.
func ChangeRequestsOptions(doer *user_model.User, repoIDs []int64, tab string) *issues_model.IssuesOptions {
	opts := &issues_model.IssuesOptions{
		IsPull:     optional.Some(true),
		IsArchived: optional.Some(false),
		Doer:       doer,
		RepoIDs:    repoIDs,
	}
	switch tab {
	case ChangeRequestsSent:
		// a change request can be opened (or the user mentioned) on any public article
		opts.AllPublic = true
		opts.PosterID = strconv.FormatInt(doer.ID, 10)
	case ChangeRequestsMentions:
		opts.AllPublic = true
		opts.MentionedID = doer.ID
	}
	return opts
}

// CountOpenChangeRequests counts the open change requests matching the options of a tab.
func CountOpenChangeRequests(ctx context.Context, opts *issues_model.IssuesOptions) (int64, error) {
	open := *opts
	open.IsClosed = optional.Some(false)
	open.Paginator = nil
	return issue_indexer.CountIssues(ctx, issue_indexer.ToSearchOptions("", &open))
}

// CountOpenReceivedChangeRequests counts the open change requests to the articles the
// user owns or maintains (the hub's Received tab, open only) in a single query: the
// navbar's counter, shown on every page, so the articles are a subquery rather than a
// list of IDs looked up first.
func CountOpenReceivedChangeRequests(ctx context.Context, doer *user_model.User) (int64, error) {
	return issues_model.CountIssues(ctx, &issues_model.IssuesOptions{
		IsPull:     optional.Some(true),
		IsClosed:   optional.Some(false),
		IsArchived: optional.Some(false),
		Doer:       doer,
		RepoCond:   builder.In("issue.repo_id", builder.Select("id").From("repository").Where(changeRequestsRepoCond(doer))),
	})
}
