// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package context

import (
	"net/http"
	"net/url"
	"strings"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/subjecttitle"
)

// RedirectToCanonicalSubject handles a "/subject/{subjectname}/…" request whose subject does
// not exist under that exact name. New subject titles are stored normalized, with their first
// letter capitalized (modules/subjecttitle), so, like MediaWiki does for "/wiki/iPhone", the
// request is redirected to the subject under its normalized title when that one exists. The
// rest of the path and the query string are kept. It reports whether it redirected; only GET
// and HEAD requests are redirected.
func RedirectToCanonicalSubject(ctx *Context, subjectName string) bool {
	if ctx.Req.Method != http.MethodGet && ctx.Req.Method != http.MethodHead {
		return false
	}
	canonical := subjecttitle.Normalize(subjectName)
	if canonical == "" || canonical == subjectName {
		return false
	}
	if _, err := repo_model.GetSubjectByName(ctx, canonical); err != nil {
		return false
	}

	prefix := setting.AppSubURL + "/subject/"
	escapedPath := ctx.Req.URL.EscapedPath()
	if !strings.HasPrefix(escapedPath, prefix) {
		return false
	}
	rest := escapedPath[len(prefix):]
	tail := ""
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		tail = rest[i:]
	}
	link := prefix + url.PathEscape(canonical) + tail
	if ctx.Req.URL.RawQuery != "" {
		link += "?" + ctx.Req.URL.RawQuery
	}
	ctx.Redirect(link, http.StatusMovedPermanently)
	return true
}
