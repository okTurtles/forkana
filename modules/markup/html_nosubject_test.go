// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package markup

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestRender_RepoWithoutSubjectReferences covers #274: a repository without a subject has
// no article url, so its own issue and commit references link to its repository route,
// while an article's keep linking to its subject url.
func TestRender_RepoWithoutSubjectReferences(t *testing.T) {
	const sha = "65f1bf27bc3bf70f64657658635e66094edbcb4d"
	render := func(metas map[string]string, input string) string {
		var buf strings.Builder
		processors := []processor{issueIndexPatternProcessor, commitCrossReferencePatternProcessor, hashCurrentPatternProcessor}
		err := postProcess(NewTestRenderContext(metas), processors, strings.NewReader(input), &buf)
		assert.NoError(t, err)
		return buf.String()
	}

	noSubject := map[string]string{
		"user":                         "user2",
		"repo":                         "plain", // the subject name falls back to the repository name
		"reponame":                     "plain",
		"nosubject":                    "true",
		"markupAllowShortIssuePattern": "true",
	}
	assert.Contains(t, render(noSubject, "see #12"), `href="/user2/plain/issues/12"`)
	assert.Contains(t, render(noSubject, "see user2/plain#12"), `href="/user2/plain/issues/12"`)
	assert.Contains(t, render(noSubject, "see "+sha), `href="/user2/plain/commit/`+sha+`"`)
	assert.Contains(t, render(noSubject, "see user2/plain@"+sha), `href="/user2/plain/commit/`+sha+`"`)

	article := map[string]string{
		"user":                         "user2",
		"repo":                         "Solar System",
		"reponame":                     "solar-system",
		"markupAllowShortIssuePattern": "true",
	}
	assert.Contains(t, render(article, "see #12"), `href="/subject/Solar%20System/user2/issues/12"`)
	assert.Contains(t, render(article, "see user2/solar-system#12"), `href="/subject/Solar%20System/user2/issues/12"`)
	assert.Contains(t, render(article, "see "+sha), `href="/subject/Solar%20System/user2?version=`+sha+`"`)
	assert.Contains(t, render(article, "see user2/solar-system@"+sha), `href="/subject/Solar%20System/user2?version=`+sha+`"`)
}
