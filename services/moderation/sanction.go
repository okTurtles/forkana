// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package moderation

import (
	"context"
	"fmt"
	"time"

	"code.gitea.io/gitea/models/db"
	moderation_model "code.gitea.io/gitea/models/moderation"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/globallock"
	"code.gitea.io/gitea/modules/timeutil"
	"code.gitea.io/gitea/modules/util"
)

// WarningMonths is how long a warning stays in force, in calendar months.
const WarningMonths = 12

// WarningOptions is a warning given from a case.
type WarningOptions struct {
	Doer        *user_model.User
	CaseIssueID int64
	// ConversationIssueID is the conversation the warning message went to; 0 until
	// conversations land (1.9).
	ConversationIssueID int64
}

// IssueWarning warns the owner of a case's target: a warning sanction in force for
// WarningMonths calendar months, and its "warned" audit entry, in one transaction
// (foundations decisions 6–7). A warning only records: it never changes the user.
// It refuses doers who aren't moderators, and a second warning in force from the same case.
func IssueWarning(ctx context.Context, opts WarningOptions) (*moderation_model.Sanction, error) {
	isModerator, err := IsModerator(ctx, opts.Doer)
	if err != nil {
		return nil, fmt.Errorf("check moderator: %w", err)
	}
	if !isModerator {
		return nil, util.NewPermissionDeniedErrorf("only moderators can warn")
	}
	c, err := moderation_model.GetCase(ctx, opts.CaseIssueID)
	if err != nil {
		return nil, fmt.Errorf("get case %d: %w", opts.CaseIssueID, err)
	}
	if c == nil || c.TargetUserID == 0 {
		return nil, util.NewNotExistErrorf("case %d has no user to warn", opts.CaseIssueID)
	}

	// Two moderators warning from one case at once: one warning.
	release, err := globallock.Lock(ctx, fmt.Sprintf("moderation:sanction:case:%d", c.IssueID))
	if err != nil {
		return nil, fmt.Errorf("lock the case's sanctions: %w", err)
	}
	defer release()

	now := time.Now()
	sanction := &moderation_model.Sanction{
		UserID:              c.TargetUserID,
		Kind:                moderation_model.SanctionWarning,
		Status:              moderation_model.SanctionActive,
		EndsUnix:            timeutil.TimeStamp(now.AddDate(0, WarningMonths, 0).Unix()),
		CaseIssueID:         c.IssueID,
		ConversationIssueID: opts.ConversationIssueID,
		CreatedBy:           opts.Doer.ID,
	}
	role := moderation_model.ActorModerator
	if opts.Doer.IsAdmin {
		role = moderation_model.ActorAdmin
	}
	err = db.WithTx(ctx, func(ctx context.Context) error {
		has, err := moderation_model.HasActiveWarningForCase(ctx, c.TargetUserID, c.IssueID, timeutil.TimeStamp(now.Unix()))
		if err != nil {
			return fmt.Errorf("check earlier warning: %w", err)
		}
		if has {
			return util.NewAlreadyExistErrorf("the case already gave a warning in force")
		}
		if err := moderation_model.InsertSanction(ctx, sanction); err != nil {
			return fmt.Errorf("insert warning: %w", err)
		}
		return moderation_model.InsertAudit(ctx, &moderation_model.Audit{
			ActorID:             opts.Doer.ID,
			ActorRole:           role,
			Action:              moderation_model.AuditWarned,
			CaseIssueID:         c.IssueID,
			ConversationIssueID: opts.ConversationIssueID,
			TargetUserID:        c.TargetUserID,
			TargetRepoID:        c.TargetRepoID,
			Summary:             "until:" + sanction.EndsUnix.AsTime().UTC().Format(time.DateOnly),
		})
	})
	if err != nil {
		return nil, err
	}
	return sanction, nil
}
