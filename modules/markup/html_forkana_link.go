// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package markup

import (
	"net/url"
	"strings"

	"code.gitea.io/gitea/modules/container"
	"code.gitea.io/gitea/modules/setting"

	"golang.org/x/net/html"
)

// Forkana internal links (issue #399).
//
// An article links to another Forkana page with a path relative to the instance root:
//
//	[Solar System](subject/Solar System)         the subject
//	[Solar System](subject/Solar System/pierre)  pierre's article on the subject
//	[guidelines](community-notes)                a special Forkana page
//
// The full URL of the page on this instance (https://<instance>/subject/Solar%20System) is an
// internal link too. Such links resolve to the instance root, never relative to the article
// they are written in, and render as internal links ("internal-link" class). A link to a
// subject or an article that does not exist (yet) renders as a missing link
// ("internal-link-missing", Wikipedia's red link) and leads to the creation of the subject.
//
// When the render marks external links (RenderOptions.MarkExternalLinks, set for articles),
// every other http(s) link, which leaves Forkana, renders as an external link
// ("external-link"), so readers know they are leaving.

const (
	internalLinkClass        = "internal-link"
	internalLinkMissingClass = "internal-link internal-link-missing"
	externalLinkClass        = "external-link"
)

// ForkanaInternalLinkRoots are the first path segments of the Forkana pages an article can
// link to with a root-relative path: the subject routes and the special pages linked from the
// site footer (see routers/web/web.go and custom/templates/base/footer_content.tmpl).
var ForkanaInternalLinkRoots = container.SetOf(
	"subject", "explore",
	"about", "terms", "privacy", "security", "community-notes", "contact",
)

// ForkanaInternalLink is a parsed Forkana internal link.
type ForkanaInternalLink struct {
	// Path is the escaped path of the page from the instance root, without AppSubURL and
	// without a leading slash, e.g. "subject/Solar%20System/pierre"; Suffix is its query and
	// fragment as written ("?…", "#…"), if any.
	Path   string
	Suffix string
	// Subject and Owner are the unescaped subject name and article owner of a subject link
	// (Owner is empty for a link to the subject itself); both are empty for other pages.
	Subject string
	Owner   string
}

// ParseForkanaInternalLink parses href as a Forkana internal link: a root-relative path whose
// first segment is in ForkanaInternalLinkRoots ("subject/Solar System"), the same path with
// a leading AppSubURL ("/subject/Solar%20System"), or the full URL of such a page on this
// instance (AppURL + "subject/Solar%20System"). Segments may be written escaped or not.
func ParseForkanaInternalLink(href string) (link ForkanaInternalLink, ok bool) {
	var rest string
	switch {
	case setting.AppURL != "" && strings.HasPrefix(href, setting.AppURL):
		rest = href[len(setting.AppURL):]
	case strings.HasPrefix(href, setting.AppSubURL+"/"):
		rest = href[len(setting.AppSubURL)+1:]
	case IsFullURLString(href) || strings.HasPrefix(href, "/") || strings.HasPrefix(href, "#") || strings.HasPrefix(href, "?"):
		return link, false
	default:
		rest = href
	}

	rest, link.Suffix = splitFragment(rest)
	if i := strings.IndexByte(rest, '?'); i >= 0 && !strings.HasPrefix(rest, "subject/") {
		// a query on a special page is kept as is; subject names may contain "?" (it is
		// escaped below), so a subject link has no query
		rest, link.Suffix = rest[:i], rest[i:]+link.Suffix
	}
	segments := strings.Split(strings.TrimSuffix(rest, "/"), "/")
	if len(segments) == 0 || !ForkanaInternalLinkRoots.Contains(segments[0]) {
		return link, false
	}

	escaped := make([]string, 0, len(segments))
	for i, seg := range segments {
		unescaped, err := url.PathUnescape(seg)
		if err != nil {
			unescaped = seg // a stray "%" in the text: take it literally
		}
		if i > 0 && unescaped == "" {
			return link, false // "subject//x": not a valid page
		}
		if segments[0] == "subject" {
			switch i {
			case 1:
				link.Subject = unescaped
			case 2:
				link.Owner = unescaped
			}
		}
		escaped = append(escaped, url.PathEscape(unescaped))
	}
	if segments[0] == "subject" && link.Subject == "" {
		return link, false // "subject" alone is not a page
	}
	link.Path = strings.Join(escaped, "/")
	return link, true
}

func splitFragment(s string) (rest, fragment string) {
	if i := strings.IndexByte(s, '#'); i >= 0 {
		return s[:i], s[i:]
	}
	return s, ""
}

// isExternalLink reports whether href leaves this Forkana instance.
func isExternalLink(href string) bool {
	if !IsFullURLString(href) {
		return false
	}
	if setting.AppURL != "" && strings.HasPrefix(href, setting.AppURL) {
		return false
	}
	lower := strings.ToLower(href)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

// isForkanaSubjectLinkExisting reports whether the subject (or the owner's article on it) a
// link points to exists, asking the render helper and caching the answer for the render.
func (ctx *RenderContext) isForkanaSubjectLinkExisting(subject, owner string) bool {
	if DefaultRenderHelperFuncs == nil || DefaultRenderHelperFuncs.IsSubjectLinkExisting == nil {
		return true
	}
	key := subject + "\x00" + owner
	if exists, ok := ctx.subjectLinkExists[key]; ok {
		return exists
	}
	exists := DefaultRenderHelperFuncs.IsSubjectLinkExisting(ctx, subject, owner)
	if ctx.subjectLinkExists == nil {
		ctx.subjectLinkExists = map[string]bool{}
	}
	ctx.subjectLinkExists[key] = exists
	return exists
}

// anchorContainsImage reports whether the <a> node wraps an image (a linked image is not
// marked as an external link: the icon would trail the image).
func anchorContainsImage(node *html.Node) bool {
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == "img" {
			return true
		}
	}
	return false
}

// processForkanaLink resolves a Forkana internal link to the instance root and marks it, or
// marks an external link. It returns false when href is neither (the caller resolves it).
func processForkanaLink(ctx *RenderContext, node *html.Node, idx int, href string) bool {
	if link, ok := ParseForkanaInternalLink(href); ok {
		class := internalLinkClass
		if link.Subject != "" && !ctx.isForkanaSubjectLinkExisting(link.Subject, link.Owner) {
			class = internalLinkMissingClass
			if link.Owner == "" {
				// Wikipedia's red link leads to the creation of the page
				node.Attr[idx].Val = ctx.ResolveLinkRoot("repo/create?subject=" + url.QueryEscape(link.Subject))
			} else {
				node.Attr[idx].Val = ctx.ResolveLinkRoot(link.Path) + link.Suffix
			}
		} else if IsFullURLString(href) {
			node.Attr[idx].Val = href // already the page's URL on this instance
		} else {
			node.Attr[idx].Val = ctx.ResolveLinkRoot(link.Path) + link.Suffix
		}
		node.Attr = append(node.Attr, ctx.RenderInternal.NodeSafeAttr("class", class))
		return true
	}
	if ctx.RenderOptions.MarkExternalLinks && isExternalLink(href) && !anchorContainsImage(node) {
		node.Attr = append(node.Attr, ctx.RenderInternal.NodeSafeAttr("class", externalLinkClass))
	}
	return false
}
