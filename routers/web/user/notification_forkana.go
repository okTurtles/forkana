// Copyright 2026 The Forkana Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package user

import (
	"time"

	activities_model "code.gitea.io/gitea/models/activities"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/services/context"
)

// The Forkana notifications list (custom/templates/user/notification/notification_div.tmpl,
// Figma 7648:74304): a "Type" filter, the rows grouped by day, and who did it.

// notificationTypeSources maps the "Type" filter (?source=) to the notification sources it shows.
// An unknown or empty value shows every type.
var notificationTypeSources = map[string][]activities_model.NotificationSource{
	// no commit: Gitea never creates commit notifications, and the list has no row for them
	"change_requests": {activities_model.NotificationSourcePullRequest, activities_model.NotificationSourceIssue},
	"invites":         {activities_model.NotificationSourceArticleCollaboratorInvite},
	"transfers": {
		activities_model.NotificationSourceRepository,
		activities_model.NotificationSourceRepoTransferRejected,
	},
}

// notificationTypeKeys lists the filter's values in the order of its menu
var notificationTypeKeys = []string{"change_requests", "invites", "transfers"}

// notificationSourceFilter returns the filter's canonical value and its sources
func notificationSourceFilter(value string) (string, []activities_model.NotificationSource) {
	sources, ok := notificationTypeSources[value]
	if !ok {
		return "", nil
	}
	return value, sources
}

// NotificationGroup is one day of the list: "today", "yesterday" or "earlier"
type NotificationGroup struct {
	Key           string
	Notifications activities_model.NotificationList
}

// groupNotificationsByDay keeps the list's order and starts a group whenever the day changes
func groupNotificationsByDay(nls activities_model.NotificationList, now time.Time) []*NotificationGroup {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	yesterday := today.AddDate(0, 0, -1)
	var groups []*NotificationGroup
	for _, n := range nls {
		key := "earlier"
		switch t := n.UpdatedUnix.AsLocalTime(); {
		case !t.Before(today):
			key = "today"
		case !t.Before(yesterday):
			key = "yesterday"
		}
		if len(groups) == 0 || groups[len(groups)-1].Key != key {
			groups = append(groups, &NotificationGroup{Key: key})
		}
		last := groups[len(groups)-1]
		last.Notifications = append(last.Notifications, n)
	}
	return groups
}

// loadNotificationActors returns the users who caused the notifications, by ID
func loadNotificationActors(ctx *context.Context, nls activities_model.NotificationList) (map[int64]*user_model.User, error) {
	ids := make([]int64, 0, len(nls))
	for _, n := range nls {
		if n.UpdatedBy > 0 {
			ids = append(ids, n.UpdatedBy)
		}
	}
	return user_model.GetUsersMapByIDs(ctx, ids)
}
