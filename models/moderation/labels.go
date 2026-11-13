// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

// Package moderation holds the data of Forkana's moderation: the cases repository it lives in,
// the labels its issues carry and, later, its own tables.
package moderation

// CoreLabel is one of the org labels the moderation issues carry. A label whose scope is
// exclusive uses Gitea's scoped-label behaviour: an issue carries at most one value of it.
//
// They are defined here rather than as a template in options/label, because a template there
// would show up in every repository's label template picker.
type CoreLabel struct {
	Name        string
	Exclusive   bool
	Color       string
	Description string
}

// Label names used by the moderation services. Each one is in CoreLabels.
const (
	LabelTypeCase         = "type/case"
	LabelTypeReport       = "type/report"
	LabelTypeConversation = "type/conversation"

	LabelKindArticle = "kind/article"
	LabelKindFork    = "kind/fork"
	LabelKindUser    = "kind/user"
	LabelKindSubject = "kind/subject"

	LabelSourceReader  = "source/reader"
	LabelSourceForkana = "source/forkana"

	LabelDecisionDismissed        = "decision/dismissed"
	LabelDecisionSourcesRequested = "decision/sources-requested"
	LabelDecisionRemoved          = "decision/removed"
	LabelDecisionEscalated        = "decision/escalated"

	LabelResolutionSourcesAdded         = "resolution/sources-added"
	LabelResolutionRemovedAfterDeadline = "resolution/removed-after-deadline"
	LabelResolutionOverturned           = "resolution/overturned"

	LabelStatusNeedsAdmin = "status/needs-admin"

	LabelOutcomeUpheld    = "outcome/upheld"
	LabelOutcomeDismissed = "outcome/dismissed"

	// ReasonLabelPrefix starts the name of every reason label: ReasonLabelPrefix + Reason.
	ReasonLabelPrefix = "reason/"
)

// Reason is one of the reasons a reader can report an article for.
type Reason string

// The report reasons, in the order the report form lists them.
const (
	ReasonChildSafety        Reason = "child-safety"
	ReasonHateOrHarassment   Reason = "hate-or-harassment"
	ReasonDoxxing            Reason = "doxxing"
	ReasonGraphicViolence    Reason = "graphic-violence"
	ReasonAdultSexualContent Reason = "adult-sexual-content"
	ReasonIllegalInRegion    Reason = "illegal-in-region"
	ReasonNoReliableSource   Reason = "no-reliable-source"
	ReasonPlagiarism         Reason = "plagiarism-or-copyright"
	ReasonOffTopicOrSpam     Reason = "off-topic-or-spam"
	ReasonSomethingElse      Reason = "something-else"
)

// reasonDescriptions holds every reason, in form order, with the one-line copy the report form
// shows under it.
var reasonDescriptions = []struct {
	Reason      Reason
	Description string
}{
	{ReasonChildSafety, "Sexualises, exploits or endangers minors. Reviewed first."},
	{ReasonHateOrHarassment, "Attacks people for who they are, or targets a specific person."},
	{ReasonDoxxing, "Shares someone's private details, like a home address or phone number."},
	{ReasonGraphicViolence, "Shows real gore or violence."},
	{ReasonAdultSexualContent, "Sexually explicit text or images."},
	{ReasonIllegalInRegion, "Breaks the law where you live. It may be hidden only there."},
	{ReasonNoReliableSource, "A factual claim nobody else could go and check."},
	{ReasonPlagiarism, "Copied from somewhere else without credit or permission."},
	{ReasonOffTopicOrSpam, "Unrelated to the subject, advertising, or repeated content."},
	{ReasonSomethingElse, "Anything not listed above."},
}

// Reasons returns every report reason, in form order.
func Reasons() []Reason {
	reasons := make([]Reason, len(reasonDescriptions))
	for i, r := range reasonDescriptions {
		reasons[i] = r.Reason
	}
	return reasons
}

// IsValid reports whether r is one of the report reasons.
func (r Reason) IsValid() bool {
	for _, d := range reasonDescriptions {
		if d.Reason == r {
			return true
		}
	}
	return false
}

// LabelName returns the name of the reason's label.
func (r Reason) LabelName() string {
	return ReasonLabelPrefix + string(r)
}

const (
	colorRed     = "#d73a4a"
	colorNeutral = "#6e7781"
	colorBlue    = "#0969da"
	colorPurple  = "#8250df"
	colorGreen   = "#1a7f37"
	colorOrange  = "#bc4c00"
	colorYellow  = "#9a6700"
)

// CoreLabels returns the org labels moderation setup creates. The flag/* labels are left to
// the automatic flags that set them (Story 2).
func CoreLabels() []CoreLabel {
	labels := []CoreLabel{
		{LabelTypeCase, true, colorBlue, "All the reports on one target, and the moderator's decision on them"},
		{LabelTypeReport, true, colorBlue, "One reason a reader ticked in one report"},
		{LabelTypeConversation, true, colorBlue, "Messages between moderators and one participant of a case"},

		{LabelKindArticle, true, colorPurple, "The case is about an article"},
		{LabelKindFork, true, colorPurple, "The case is about a fork of an article"},
		{LabelKindUser, true, colorPurple, "The case is about a user"},
		{LabelKindSubject, true, colorPurple, "The case is about a subject"},
	}
	for _, r := range reasonDescriptions {
		color := colorNeutral
		if r.Reason == ReasonChildSafety {
			color = colorRed
		}
		labels = append(labels, CoreLabel{r.Reason.LabelName(), false, color, r.Description})
	}
	return append(labels,
		CoreLabel{LabelSourceReader, false, colorNeutral, "Reported by a reader"},
		CoreLabel{LabelSourceForkana, false, colorNeutral, "Flagged by Forkana"},

		CoreLabel{LabelDecisionDismissed, true, colorGreen, "No violation: the article stays as it is"},
		CoreLabel{LabelDecisionSourcesRequested, true, colorYellow, "The owner was asked to add sources before a deadline"},
		CoreLabel{LabelDecisionRemoved, true, colorRed, "The article was removed"},
		CoreLabel{LabelDecisionEscalated, true, colorOrange, "Sent to a site administrator"},

		CoreLabel{LabelResolutionSourcesAdded, true, colorGreen, "The owner added the requested sources"},
		CoreLabel{LabelResolutionRemovedAfterDeadline, true, colorRed, "Removed automatically when the sources deadline passed"},
		CoreLabel{LabelResolutionOverturned, true, colorOrange, "The decision was overturned"},

		CoreLabel{LabelStatusNeedsAdmin, false, colorOrange, "Waiting for a site administrator"},

		CoreLabel{LabelOutcomeUpheld, true, colorRed, "The moderator upheld this reason"},
		CoreLabel{LabelOutcomeDismissed, true, colorGreen, "The moderator dismissed this reason"},
	)
}
