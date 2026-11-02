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

// ChangeRequestsOptions returns the search options of one tab of the change requests
// hub, in every state, built the way the dashboard's change request overview builds them
// (routers/web/user/home.go buildIssueOverview): Received is its "In your repositories"
// filter, Sent its "Created by you" and Mentions its "Mentioning you".
func ChangeRequestsOptions(ctx context.Context, doer *user_model.User, tab string) (*issues_model.IssuesOptions, error) {
	// The articles the user owns or maintains (collaborates on).
	repoIDs, _, err := repo_model.SearchRepositoryIDs(ctx, repo_model.SearchRepoOptions{
		Actor:       doer,
		OwnerID:     doer.ID,
		Private:     true,
		Collaborate: optional.None[bool](),
		UnitType:    unit.TypePullRequests,
		Archived:    optional.Some(false),
	})
	if err != nil {
		return nil, err
	}
	if len(repoIDs) == 0 {
		// no repos found, don't let the indexer return all repos
		repoIDs = []int64{0}
	}

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
	return opts, nil
}

// CountOpenChangeRequests counts the open change requests of one tab of the hub.
func CountOpenChangeRequests(ctx context.Context, doer *user_model.User, tab string) (int64, error) {
	opts, err := ChangeRequestsOptions(ctx, doer, tab)
	if err != nil {
		return 0, err
	}
	opts.IsClosed = optional.Some(false)
	return issue_indexer.CountIssues(ctx, issue_indexer.ToSearchOptions("", opts))
}
