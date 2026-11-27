// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

// Package help serves Forkana's help pages, under /help.
package help

import (
	"fmt"
	"net/http"

	moderation_model "code.gitea.io/gitea/models/moderation"
	"code.gitea.io/gitea/modules/httplib"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/templates"
	"code.gitea.io/gitea/services/context"
)

const tplWhatCanBeReported templates.TplName = "help/what_can_be_reported"

// navItem is one entry of the Help sidebar (Figma "Help nav" 8159:880). Only "What can be
// reported" exists so far; the other entries link to the pages the design plans under /help.
type navItem struct {
	Slug string // its page is /help/<Slug>
	Key  string // help.<Key> is its name
	Icon string
}

type navGroup struct {
	Key   string
	Items []navItem
}

var navGroups = []navGroup{
	{"using_forkana", []navItem{
		{"getting-started", "getting_started", "octicon-book"},
		{"how-to-use-forkana", "how_to_use_forkana", "octicon-question"},
		{"communities", "communities", "octicon-people"},
		{"contact-support", "contact_support", "octicon-mail"},
	}},
	{"guidelines", []navItem{
		{"community-guidelines", "community_guidelines", "octicon-info"},
		{"contributing-guidelines", "contributing_guidelines", "octicon-shield-check"},
		{"what-can-be-reported", "what_can_be_reported", "octicon-report"},
		{"location-restrictions", "location_restrictions", "octicon-circle-slash"},
	}},
}

// reportReason is one reason of the report form, as "What can be reported" explains it.
type reportReason struct {
	Slug string // its anchor: the reason's value in the report form (models/moderation.Reason)
	Key  string // its strings are help.report.<Key>.*
	// Severe reasons have a red label, like their moderation label (child safety).
	Severe     bool
	NotAllowed []string // locale keys of the "Not allowed" examples
	Allowed    []string // locale keys of the "Allowed" examples
	LinkSlug   string   // a help page the reason links to, if any
	LinkKey    string   // the link's text
	Example    bool     // the content warning example (graphic violence)
}

type reasonGroup struct {
	Key     string
	Reasons []reportReason
}

// reason builds a reportReason whose examples are help.report.<key>.not_allowed_1…notAllowed
// and help.report.<key>.allowed_1…allowed.
func reason(slug moderation_model.Reason, key string, notAllowed, allowed int) reportReason {
	r := reportReason{Slug: string(slug), Key: key}
	for i := 1; i <= notAllowed; i++ {
		r.NotAllowed = append(r.NotAllowed, fmt.Sprintf("help.report.%s.not_allowed_%d", key, i))
	}
	for i := 1; i <= allowed; i++ {
		r.Allowed = append(r.Allowed, fmt.Sprintf("help.report.%s.allowed_%d", key, i))
	}
	return r
}

// reasonGroups lists the reasons in the report form's groups and order. "Something else" has
// nothing to explain, so the page leaves it out, as the design does.
func reasonGroups() []reasonGroup {
	childSafety := reason(moderation_model.ReasonChildSafety, "child_safety", 3, 2)
	childSafety.Severe = true
	graphicViolence := reason(moderation_model.ReasonGraphicViolence, "graphic_violence", 2, 2)
	graphicViolence.Example = true
	illegalInRegion := reason(moderation_model.ReasonIllegalInRegion, "illegal_in_region", 1, 1)
	illegalInRegion.LinkSlug, illegalInRegion.LinkKey = "location-restrictions", "help.location_restrictions"
	noReliableSource := reason(moderation_model.ReasonNoSource, "no_reliable_source", 2, 2)
	noReliableSource.LinkSlug, noReliableSource.LinkKey = "contributing-guidelines", "help.report.sourcing_rules"

	return []reasonGroup{
		{"group_harmful", []reportReason{
			childSafety,
			reason(moderation_model.ReasonHateOrHarassment, "hate_or_harassment", 4, 3),
			reason(moderation_model.ReasonDoxxing, "doxxing", 2, 2),
			graphicViolence,
			reason(moderation_model.ReasonAdultSexualContent, "adult_sexual_content", 2, 2),
			illegalInRegion,
		}},
		{"group_accuracy", []reportReason{
			noReliableSource,
			reason(moderation_model.ReasonPlagiarism, "plagiarism_or_copyright", 2, 2),
			reason(moderation_model.ReasonOffTopicOrSpam, "off_topic_or_spam", 3, 2),
		}},
	}
}

// backLink is where a help page's Back leads: the page that linked to it with
// ?redirect_to=<url> (the report form), if that is a page of this site, or else the home page.
// The page can't use the referrer: Gitea's pages send none (templates/base/head.tmpl).
func backLink(ctx *context.Context) string {
	if redirectTo := ctx.FormString("redirect_to"); redirectTo != "" && httplib.IsCurrentGiteaSiteURL(ctx, redirectTo) {
		return redirectTo
	}
	return setting.AppSubURL + "/"
}

// WhatCanBeReported renders the guidance page linked from the report form (#541).
func WhatCanBeReported(ctx *context.Context) {
	ctx.Data["BackLink"] = backLink(ctx)
	ctx.Data["Title"] = ctx.Locale.TrString("help.what_can_be_reported")
	ctx.Data["HelpNavGroups"] = navGroups
	ctx.Data["HelpActive"] = "what-can-be-reported"
	ctx.Data["ReasonGroups"] = reasonGroups()
	ctx.HTML(http.StatusOK, tplWhatCanBeReported)
}
