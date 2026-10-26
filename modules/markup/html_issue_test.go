// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package markup_test

import (
	"context"
	"html/template"
	"strings"
	"testing"

	"code.gitea.io/gitea/modules/htmlutil"
	"code.gitea.io/gitea/modules/markup"
	"code.gitea.io/gitea/modules/markup/markdown"
	testModule "code.gitea.io/gitea/modules/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRender_IssueList(t *testing.T) {
	defer testModule.MockVariableValue(&markup.RenderBehaviorForTesting.DisableAdditionalAttributes, true)()
	markup.Init(&markup.RenderHelperFuncs{
		RenderRepoIssueIconTitle: func(ctx context.Context, opts markup.RenderIssueIconTitleOptions) (template.HTML, error) {
			return htmlutil.HTMLFormat("<div>issue #%d</div>", opts.IssueIndex), nil
		},
	})

	test := func(input, expected string) {
		rctx := markup.NewTestRenderContext(markup.TestAppURL, map[string]string{
			"user": "test-user", "repo": "test-repo",
			"markupAllowShortIssuePattern": "true",
			"footnoteContextId":            "12345",
		})
		out, err := markdown.RenderString(rctx, input)
		require.NoError(t, err)
		assert.Equal(t, strings.TrimSpace(expected), strings.TrimSpace(string(out)))
	}

	t.Run("NormalIssueRef", func(t *testing.T) {
		test(
			"#12345",
			`<p><a href="/subject/test-repo/test-user/issues/12345" class="ref-issue" rel="nofollow">#12345</a></p>`,
		)
	})

	t.Run("ListIssueRef", func(t *testing.T) {
		test(
			"* #12345",
			`<ul>
<li><div>issue #12345</div></li>
</ul>`,
		)
	})

	t.Run("ListIssueRefNormal", func(t *testing.T) {
		test(
			"* foo #12345 bar",
			`<ul>
<li>foo <a href="/subject/test-repo/test-user/issues/12345" class="ref-issue" rel="nofollow">#12345</a> bar</li>
</ul>`,
		)
	})

	t.Run("ListTodoIssueRef", func(t *testing.T) {
		test(
			"* [ ] #12345",
			`<ul>
<li class="task-list-item"><input type="checkbox" disabled="" data-source-position="2"/><div>issue #12345</div></li>
</ul>`,
		)
	})

	t.Run("IssueFootnote", func(t *testing.T) {
		test(
			"foo[^1][^2]\n\n[^1]: bar\n[^2]: baz",
			`<p>foo<sup class="footnote-ref" id="fnref:user-content-1-12345"><a href="#fn:user-content-1-12345" rel="nofollow">[1]</a></sup><sup class="footnote-ref" id="fnref:user-content-2-12345"><a href="#fn:user-content-2-12345" rel="nofollow">[2]</a></sup></p>
<div class="footnotes">
<h2 class="footnotes-heading">citation.references</h2>
<ol>
<li id="fn:user-content-1-12345">
<p><span class="footnote-backrefs"><a aria-label="citation.back_to_text" class="footnote-backref" href="#fnref:user-content-1-12345" rel="nofollow">^</a></span> bar</p>
</li>
<li id="fn:user-content-2-12345">
<p><span class="footnote-backrefs"><a aria-label="citation.back_to_text" class="footnote-backref" href="#fnref:user-content-2-12345" rel="nofollow">^</a></span> baz</p>
</li>
</ol>
</div>`,
		)
	})
}
