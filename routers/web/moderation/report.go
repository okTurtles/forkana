// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package moderation

import (
	"errors"
	"io"
	"net/http"

	moderation_model "code.gitea.io/gitea/models/moderation"
	"code.gitea.io/gitea/modules/json"
	"code.gitea.io/gitea/modules/markup/markdown"
	"code.gitea.io/gitea/modules/util"
	"code.gitea.io/gitea/services/context"
	moderation_service "code.gitea.io/gitea/services/moderation"
)

// maxReportBody caps the request body: ten explanations of MaxExplanationLength runes and a quote.
const maxReportBody = 128 << 10

// ReportRequest is the body of POST {article}/-/report.
type ReportRequest struct {
	ClientToken string `json:"client_token"`
	// Anchor is "whole", "exact" (Range is the block's data-source-range, as read) or
	// "quote_only" (Quote, and optionally Section, for a block without a range).
	Anchor    string `json:"anchor"`
	CommitSHA string `json:"commit_sha"`
	Range     string `json:"range"`
	Quote     string `json:"quote"`
	Section   string `json:"section"`
	Reasons   []struct {
		Reason      string `json:"reason"`
		Explanation string `json:"explanation"`
	} `json:"reasons"`
}

// ReportResponse is the answer: Status is "filed", "existing" (the token was already used) or
// "already_reported" (every reason is a repeat; nothing was written).
type ReportResponse struct {
	Status       string   `json:"status"`
	SubmissionID int64    `json:"submission_id,omitempty"`
	Filed        []string `json:"filed"`
	Repeated     []string `json:"repeated"`
}

func reasonStrings(reasons []moderation_model.Reason) []string {
	s := make([]string, len(reasons))
	for i, r := range reasons {
		s[i] = string(r)
	}
	return s
}

// unprocessable answers 422 in the shape the frontend's fetch helpers render as an error.
func unprocessable(ctx *context.Context, msg string) {
	ctx.JSON(http.StatusUnprocessableEntity, map[string]any{"errorMessage": msg, "renderFormat": "text"})
}

// ReportArticle files a report on the article (POST {article}/-/report). There is no form
// for it yet: the report modal (feature 1.1) will post here.
func ReportArticle(ctx *context.Context) {
	var req ReportRequest
	body, err := io.ReadAll(io.LimitReader(ctx.Req.Body, maxReportBody+1))
	if err != nil {
		ctx.ServerError("read report", err)
		return
	}
	if len(body) > maxReportBody {
		ctx.JSONError("report too large")
		return
	}
	if err := json.Unmarshal(body, &req); err != nil {
		ctx.JSONError("invalid JSON")
		return
	}

	in := moderation_service.ReportInput{
		Reporter:    ctx.Doer,
		Repo:        ctx.Repo.Repository,
		ClientToken: req.ClientToken,
		Anchor:      moderation_model.AnchorStatus(req.Anchor),
		CommitSHA:   req.CommitSHA,
		Quote:       req.Quote,
		Section:     req.Section,
	}
	if in.Anchor == moderation_model.AnchorExact {
		start, end, ok := markdown.ParseSourceRange(req.Range)
		if !ok {
			unprocessable(ctx, "invalid range")
			return
		}
		in.SourceStart, in.SourceEnd = start, end
	}
	for _, r := range req.Reasons {
		in.Reasons = append(in.Reasons, moderation_service.ReasonInput{
			Reason:      moderation_model.Reason(r.Reason),
			Explanation: r.Explanation,
		})
	}

	result, err := moderation_service.SubmitReport(ctx, in)
	if err != nil {
		if errors.Is(err, util.ErrInvalidArgument) || errors.Is(err, moderation_service.ErrTokenReused) {
			unprocessable(ctx, err.Error())
			return
		}
		ctx.ServerError("SubmitReport", err)
		return
	}

	resp := ReportResponse{Filed: reasonStrings(result.Filed), Repeated: reasonStrings(result.Repeated)}
	switch {
	case result.Existing:
		resp.Status = "existing"
	case result.Submission == nil:
		resp.Status = "already_reported"
	default:
		resp.Status = "filed"
	}
	if result.Submission != nil {
		resp.SubmissionID = result.Submission.ID
	}
	ctx.JSON(http.StatusOK, resp)
}
