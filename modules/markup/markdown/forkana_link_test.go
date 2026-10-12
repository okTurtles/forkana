// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package markdown_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"code.gitea.io/gitea/modules/markup"
	"code.gitea.io/gitea/modules/markup/markdown"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withSubjects makes only the given subjects (and "subject\x00owner" articles) exist for the
// render helper, as services/markup does with the database.
func withSubjects(t *testing.T, existing ...string) {
	set := map[string]bool{}
	for _, s := range existing {
		set[s] = true
	}
	old := markup.DefaultRenderHelperFuncs
	funcs := markup.RenderHelperFuncs{}
	if old != nil {
		funcs = *old
	}
	funcs.IsSubjectLinkExisting = func(_ context.Context, subject, owner string) bool {
		if owner != "" {
			return set[subject+"\x00"+owner]
		}
		return set[subject]
	}
	markup.DefaultRenderHelperFuncs = &funcs
	t.Cleanup(func() { markup.DefaultRenderHelperFuncs = old })
}

func renderArticle(t *testing.T, input string) string {
	// the article's own base link: relative links that are not Forkana pages resolve against it
	rctx := markup.NewTestRenderContext("/subject/Moon%20landing/pierre/src/branch/main")
	rctx.RenderOptions.ForkanaArticle = true // as for every article
	res, err := markdown.RenderString(rctx, input)
	require.NoError(t, err)
	return strings.TrimSpace(string(res))
}

func TestRender_ForkanaInternalLinks(t *testing.T) {
	defer test.MockVariableValue(&setting.AppURL, "https://forkana.example/")()
	defer test.MockVariableValue(&setting.AppSubURL, "")()
	withSubjects(t, "Solar System", "Solar System\x00pierre", "Mercury (planet)")

	cases := []struct {
		name, input, expected string
	}{
		{
			"subject shorthand with spaces",
			"[Solar System](subject/Solar System)",
			`<p><a href="/subject/Solar%20System" class="internal-link" rel="nofollow">Solar System</a></p>`,
		},
		{
			"article in a subject",
			"See [Pierre's](subject/Solar System/pierre).",
			`<p>See <a href="/subject/Solar%20System/pierre" class="internal-link" rel="nofollow">Pierre&#39;s</a>.</p>`,
		},
		{
			"escaped spaces",
			"[Solar System](subject/Solar%20System)",
			`<p><a href="/subject/Solar%20System" class="internal-link" rel="nofollow">Solar System</a></p>`,
		},
		{
			"angle brackets",
			"[Solar System](<subject/Solar System>)",
			`<p><a href="/subject/Solar%20System" class="internal-link" rel="nofollow">Solar System</a></p>`,
		},
		{
			"parentheses in the subject name",
			"[Mercury](subject/Mercury (planet))",
			`<p><a href="/subject/Mercury%20%28planet%29" class="internal-link" rel="nofollow">Mercury</a></p>`,
		},
		{
			"full URL on this instance",
			"[Solar System](https://forkana.example/subject/Solar%20System)",
			`<p><a href="https://forkana.example/subject/Solar%20System" class="internal-link" rel="nofollow">Solar System</a></p>`,
		},
		{
			"special page",
			"Read the [guidelines](community-notes#sources).",
			`<p>Read the <a href="/community-notes#sources" class="internal-link" rel="nofollow">guidelines</a>.</p>`,
		},
		{
			"missing subject is a red link to its creation",
			"[Chang'e 4](subject/Chang'e 4)",
			`<p><a href="/repo/create?subject=Chang%27e+4" class="internal-link internal-link-missing" rel="nofollow">Chang&#39;e 4</a></p>`,
		},
		{
			"missing article in an existing subject leads to the subject",
			"[Tom's](subject/Solar System/tom)",
			`<p><a href="/subject/Solar%20System" class="internal-link internal-link-missing" rel="nofollow">Tom&#39;s</a></p>`,
		},
		{
			"a standard link with a title is not taken (review #7)",
			`[Solar System](subject/Solar System "the system")`,
			`<p>[Solar System](subject/Solar System &#34;the system&#34;)</p>`,
		},
		{
			"a standard link without spaces and a title",
			`[Solar](subject/Solar "the system")`,
			`<p><a href="/repo/create?subject=Solar" title="the system" class="internal-link internal-link-missing" rel="nofollow">Solar</a></p>`,
		},
		{
			"escaped parentheses in the name (review #25)",
			`[Mercury](subject/Mercury \(planet\))`,
			`<p><a href="/subject/Mercury%20%28planet%29" class="internal-link" rel="nofollow">Mercury</a></p>`,
		},
		{
			"the app's own links keep their query (review #2)",
			"[history](/subject/Solar%20System/pierre?version=abc)",
			`<p><a href="/subject/Solar%20System/pierre?version=abc" class="internal-link" rel="nofollow">history</a></p>`,
		},
		{
			"external link",
			"[NASA](https://www.nasa.gov/)",
			`<p><a href="https://www.nasa.gov/" class="external-link" rel="nofollow">NASA</a></p>`,
		},
		{
			"other relative links still resolve against the article",
			"[file](notes.md)",
			`<p><a href="/subject/Moon%20landing/pierre/src/branch/main/notes.md" rel="nofollow">file</a></p>`,
		},
		{
			"code spans are left alone",
			"`[Solar System](subject/Solar System)`",
			`<p><code>[Solar System](subject/Solar System)</code></p>`,
		},
		{
			"spaces in a destination that is not a Forkana page stay text",
			"[a](notes about.md)",
			`<p>[a](notes about.md)</p>`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.expected, renderArticle(t, c.input))
		})
	}
}

func TestRender_ForkanaSpecialPagesOnlyInArticles(t *testing.T) {
	defer test.MockVariableValue(&setting.AppURL, "https://forkana.example/")()
	defer test.MockVariableValue(&setting.AppSubURL, "")()
	withSubjects(t, "Moon")
	// a comment or README: "security/…" is a file of the repository (review #3)
	res, err := markdown.RenderString(markup.NewTestRenderContext("/user2/repo1/src/branch/main"), "[policy](security/README.md) [Moon](subject/Moon)")
	require.NoError(t, err)
	assert.Equal(t, `<p><a href="/user2/repo1/src/branch/main/security/README.md" rel="nofollow">policy</a> <a href="/subject/Moon" class="internal-link" rel="nofollow">Moon</a></p>`, strings.TrimSpace(string(res)))
}

// TestRender_InternalLinkParserIsLinear is the regression test of review #1: the parser runs at
// every "[", and an unbounded scan of each candidate made this line quadratic. It compares the
// render time of an input with that of an input 4 times longer (4x when linear, 16x when
// quadratic) rather than using a fixed budget, which CI's race detector makes unreliable.
func TestRender_InternalLinkParserIsLinear(t *testing.T) {
	render := func(input string) time.Duration {
		start := time.Now()
		_, err := markdown.RenderString(markup.NewTestRenderContext(), input)
		require.NoError(t, err)
		return time.Since(start)
	}
	for name, build := range map[string]func(n int) string{
		"empty destinations": func(n int) string { return strings.Repeat("[a]( ", n) },
		"unclosed subject":   func(n int) string { return strings.Repeat("[a](subject/x ", n) },
		"brackets":           func(n int) string { return strings.Repeat("[", 2*n) },
		"nested parentheses": func(n int) string { return "[a](subject/" + strings.Repeat("(", 2*n) },
	} {
		t.Run(name, func(t *testing.T) {
			small, large := build(12500), build(50000)
			render(small) // warm up
			base := max(render(small), 20*time.Millisecond)
			assert.Less(t, render(large), 10*base)
		})
	}
}

func BenchmarkInternalLinkParserPathological(b *testing.B) {
	input := strings.Repeat("[a]( ", 50000)
	for b.Loop() {
		_, _ = markdown.RenderString(markup.NewTestRenderContext(), input)
	}
}

func TestRender_ForkanaInternalLinksUnderSubURL(t *testing.T) {
	defer test.MockVariableValue(&setting.AppURL, "https://example.org/wiki/")()
	defer test.MockVariableValue(&setting.AppSubURL, "/wiki")()
	withSubjects(t, "Solar System")

	assert.Equal(t,
		`<p><a href="/wiki/subject/Solar%20System" class="internal-link" rel="nofollow">Solar System</a></p>`,
		renderArticle(t, "[Solar System](subject/Solar System)"))
	assert.Equal(t,
		`<p><a href="/wiki/subject/Solar%20System" class="internal-link" rel="nofollow">Solar System</a></p>`,
		renderArticle(t, "[Solar System](/wiki/subject/Solar%20System)"))
}

func TestRender_FootnotesAsReferences(t *testing.T) {
	// (the locales are not loaded in unit tests: texts render as their keys)
	input := `Apollo 11 landed in 1969.[^apollo] Luna 2 came first.[^luna] Apollo 11 again.[^apollo]

[^apollo]: ["Apollo 11"](https://www.nasa.gov/apollo-11). *NASA*.
[^luna]: "Luna 2". NASA–NSSDC.
`
	expected := `<p>Apollo 11 landed in 1969.<sup class="footnote-ref" id="fnref:user-content-apollo"><a href="#fn:user-content-apollo" rel="nofollow">[1]</a></sup> Luna 2 came first.<sup class="footnote-ref" id="fnref:user-content-luna"><a href="#fn:user-content-luna" rel="nofollow">[2]</a></sup> Apollo 11 again.<sup class="footnote-ref" id="fnref:user-content-apollo:2"><a href="#fn:user-content-apollo" rel="nofollow">[1]</a></sup></p>
<div class="footnotes">
<h2 class="footnotes-heading">citation.references</h2>
<ol>
<li id="fn:user-content-apollo">
<p><span class="footnote-backrefs">^ <a aria-label="citation.back_to_text a" class="footnote-backref" href="#fnref:user-content-apollo" rel="nofollow">a</a> <a aria-label="citation.back_to_text b" class="footnote-backref" href="#fnref:user-content-apollo:2" rel="nofollow">b</a></span> <a href="https://www.nasa.gov/apollo-11" class="external-link" rel="nofollow">&#34;Apollo 11&#34;</a>. <em>NASA</em>.</p>
</li>
<li id="fn:user-content-luna">
<p><span class="footnote-backrefs"><a aria-label="citation.back_to_text" class="footnote-backref" href="#fnref:user-content-luna" rel="nofollow">^</a></span> &#34;Luna 2&#34;. NASA–NSSDC.</p>
</li>
</ol>
</div>`
	assert.Equal(t, expected, renderArticle(t, input))
}

func TestRender_FootnotesContextID(t *testing.T) {
	// comments on the same page add a context id to every footnote id and link
	res, err := markdown.RenderString(markup.NewTestRenderContext(map[string]string{"footnoteContextId": "7"}), "a[^1] b[^1]\n\n[^1]: c\n")
	require.NoError(t, err)
	s := string(res)
	assert.Contains(t, s, `id="fnref:user-content-1-7"`)
	assert.Contains(t, s, `id="fnref:user-content-1:2-7"`)
	assert.Contains(t, s, `href="#fnref:user-content-1:2-7"`)
	assert.Contains(t, s, `<li id="fn:user-content-1-7">`)
}
