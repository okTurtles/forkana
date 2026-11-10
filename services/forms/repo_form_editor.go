// Copyright 2025 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package forms

import (
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"code.gitea.io/gitea/modules/optional"
	"code.gitea.io/gitea/modules/web/middleware"
	"code.gitea.io/gitea/services/context"

	"gitea.com/go-chi/binding"
)

type CommitCommonForm struct {
	TreePath      string `binding:"MaxSize(500)"`
	CommitSummary string `binding:"MaxSize(100)"`
	CommitMessage string
	CommitChoice  string `binding:"Required;MaxSize(50)"`
	NewBranchName string `binding:"GitRefName;MaxSize(100)"`
	LastCommit    string
	Signoff       bool
	CommitEmail   string
}

func (f *CommitCommonForm) Validate(req *http.Request, errs binding.Errors) binding.Errors {
	ctx := context.GetValidateContext(req)
	return middleware.Validate(errs, ctx.Data, f, ctx.Locale)
}

type CommitCommonFormInterface interface {
	GetCommitCommonForm() *CommitCommonForm
}

func (f *CommitCommonForm) GetCommitCommonForm() *CommitCommonForm {
	return f
}

type EditRepoFileForm struct {
	CommitCommonForm
	Content                  optional.Option[string]
	ForkAndEdit              bool   // If true, fork the repository first and commit to the fork
	SubmitChangeRequest      bool   // If true, fork + create branch + commit + create CR back to original
	ChangeRequestTitle       string // Optional custom title for the Change Request
	ChangeRequestDescription string // Optional custom description for the Change Request
	// Summary is the article's summary (the repository's description), which the article
	// editors submit with the article (#475). Other editor forms do not send it.
	Summary optional.Option[string]
}

// MaxArticleSummaryLength is the most characters an article's summary may have (#475), as the
// Summary field's caption says.
const MaxArticleSummaryLength = 200

// ErrArticleSummaryRequired and ErrArticleSummaryTooLong are why ArticleSummary rejects a summary.
var (
	ErrArticleSummaryRequired = errors.New("the article summary is required")
	ErrArticleSummaryTooLong  = errors.New("the article summary is too long")
)

// NormalizeArticleSummary trims a summary and makes its line breaks and runs of blanks single
// spaces, as ArticleSummary does. A stored description is compared in this form, so that one
// saved with line breaks (in the settings, or through the API) does not count as changed.
func NormalizeArticleSummary(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// ArticleSummary returns the summary submitted with the article, its blanks trimmed and its
// line breaks made spaces (it is shown as one paragraph). It is required whenever the form
// comes from an article editor (required, or a Summary was sent) and then must be from 1 to
// MaxArticleSummaryLength characters. hasSummary tells whether the form is an article editor's,
// which a summary goes with: it is true even when err says that summary is invalid.
func (f *EditRepoFileForm) ArticleSummary(required bool) (summary string, hasSummary bool, err error) {
	if !f.Summary.Has() && !required {
		return "", false, nil
	}
	summary = NormalizeArticleSummary(f.Summary.Value())
	if summary == "" {
		return "", true, ErrArticleSummaryRequired
	}
	if utf8.RuneCountInString(summary) > MaxArticleSummaryLength {
		return "", true, ErrArticleSummaryTooLong
	}
	return summary, true, nil
}

// EditorWorkflowForm exposes the editor workflow flags of a bound form.
// The permission middleware and the editor handlers read the flags through this
// interface instead of re-parsing the raw request, so that every gate sees the
// value the binder produced: the binder accepts only "on" and the values
// strconv.ParseBool understands, and web.Bind silently binds anything else to
// false, whereas Base.FormBool also accepts a case-insensitive "on".
type EditorWorkflowForm interface {
	IsForkAndEdit() bool
	IsSubmitChangeRequest() bool
}

func (f *EditRepoFileForm) IsForkAndEdit() bool {
	return f.ForkAndEdit
}

func (f *EditRepoFileForm) IsSubmitChangeRequest() bool {
	return f.SubmitChangeRequest
}

type DeleteRepoFileForm struct {
	CommitCommonForm
}

type UploadRepoFileForm struct {
	CommitCommonForm
	Files []string
}

type CherryPickForm struct {
	CommitCommonForm
	Revert bool
}
