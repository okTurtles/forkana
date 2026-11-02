// Copyright 2026 The Forkana Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package explore

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	summaryFrontMatterRE   = regexp.MustCompile(`^---[^\S\n]*\r?\n[\s\S]*?\r?\n---[^\S\n]*(?:\r?\n|$)`)
	summaryFenceRE         = regexp.MustCompile("^(```|~~~)")
	summarySkippedLineRE   = regexp.MustCompile(`^(#{1,6}\s|!\[|\||<)`)
	summaryThematicBreakRE = regexp.MustCompile(`^(?:[-*_][ \t]*){3,}$`)
	summaryLinkRE          = regexp.MustCompile(`!?\[([^\]]*)\]\([^)]*\)`)
	summaryMarkerRE        = regexp.MustCompile("[*_`]")
	summarySpacesRE        = regexp.MustCompile(`\s+`)
	summaryLineSplitRE     = regexp.MustCompile(`\r?\n`)
)

// articleSummaryMaxLength clamps a pathological first paragraph, as the Bubble and
// Table views do.
const articleSummaryMaxLength = 400

// articleSummary is the summary of an article without a description: its first real
// paragraph, as the Bubble and Table views show it. It is the same rule as
// extractArticleSummary in web_src/js/components/graph/article-summary.ts, so that an
// article has one summary in every view (#478): front matter, headings, images,
// tables, HTML, thematic breaks and code fences are skipped, inline markup is stripped
// and the text is clamped to 400 characters.
func articleSummary(markdown string) string {
	text := strings.TrimPrefix(markdown, "\ufeff")
	if fm := summaryFrontMatterRE.FindString(text); fm != "" {
		text = text[len(fm):]
	}
	var paragraph []string
	inFence := false
	for _, rawLine := range summaryLineSplitRE.Split(text, -1) {
		line := strings.TrimSpace(rawLine)
		if summaryFenceRE.MatchString(line) {
			if len(paragraph) > 0 {
				break
			}
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if line == "" {
			if len(paragraph) > 0 {
				break
			}
			continue
		}
		if summarySkippedLineRE.MatchString(line) || summaryThematicBreakRE.MatchString(line) {
			if len(paragraph) > 0 {
				break
			}
			continue
		}
		paragraph = append(paragraph, line)
	}
	summary := strings.Join(paragraph, " ")
	summary = summaryLinkRE.ReplaceAllString(summary, "$1")
	summary = summaryMarkerRE.ReplaceAllString(summary, "")
	summary = strings.TrimSpace(summarySpacesRE.ReplaceAllString(summary, " "))
	// (the TS counts UTF-16 units; runes are close enough for a clamp)
	if utf8.RuneCountInString(summary) > articleSummaryMaxLength {
		runes := []rune(summary)
		summary = strings.TrimRight(string(runes[:articleSummaryMaxLength-1]), " \t\r\n") + "…"
	}
	return summary
}
