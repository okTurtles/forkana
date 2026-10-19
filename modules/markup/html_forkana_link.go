// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package markup

import (
	"net/url"
	"strings"
	"sync/atomic"

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
// When the render marks external links (RenderOptions.ForkanaArticle, set for articles),
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
	// A link to one of the owner's numbered articles ("subject/X/pierre/2") is checked as a
	// link to the owner's articles on the subject: the number is not checked.
	Subject string
	Owner   string
}

type parsedAppURL struct {
	raw string
	u   *url.URL
}

var appURLCache atomic.Pointer[parsedAppURL]

// appURL returns setting.AppURL parsed, parsing it again only when the setting changed (the
// tests change it).
func appURL() *url.URL {
	if c := appURLCache.Load(); c != nil && c.raw == setting.AppURL {
		return c.u
	}
	u, err := url.Parse(setting.AppURL)
	if err != nil {
		u = nil
	}
	appURLCache.Store(&parsedAppURL{raw: setting.AppURL, u: u})
	return u
}

// instanceRelativePath returns the part of href after this instance's root when href is the
// full URL of a page on this instance (the scheme and the host's case may differ from
// AppURL's), the instance root included, or a path starting with AppSubURL + "/".
func instanceRelativePath(href string) (rest string, ok bool) {
	if strings.HasPrefix(href, setting.AppSubURL+"/") && !strings.HasPrefix(href, "//") {
		return href[len(setting.AppSubURL)+1:], true
	}
	if !IsFullURLString(href) || setting.AppURL == "" {
		return "", false
	}
	u, err := url.Parse(href)
	// with user info, the host is not right after "://" (and such a link is not the app's)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
		return "", false
	}
	app := appURL()
	if app == nil || !strings.EqualFold(u.Host, app.Host) {
		return "", false
	}
	// keep the path as written (escaped), with its query and fragment
	afterHost := href[strings.Index(href, "://")+3+len(u.Host):]
	root := strings.TrimSuffix(app.Path, "/")
	if rest, ok := strings.CutPrefix(afterHost, root); ok && (rest == "" || rest[0] == '?' || rest[0] == '#') {
		return rest, true // the instance root itself
	}
	prefix := root + "/"
	if !strings.HasPrefix(afterHost, prefix) {
		return "", false
	}
	return afterHost[len(prefix):], true
}

// ParseForkanaInternalLink parses href as a Forkana internal link: a root-relative path whose
// first segment is in ForkanaInternalLinkRoots ("subject/Solar System"), the same path with
// a leading AppSubURL ("/subject/Solar%20System"), or the full URL of such a page on this
// instance (AppURL + "subject/Solar%20System"). Segments may be written escaped or not.
// Unless specialPages is set (articles), only "subject" links are internal links: elsewhere a
// relative "about" or "security/README.md" is a link to a file of the repository.
func ParseForkanaInternalLink(href string, specialPages bool) (link ForkanaInternalLink, ok bool) {
	rest, urlForm := instanceRelativePath(href)
	return parseForkanaInternalLink(href, specialPages, rest, urlForm)
}

// parseForkanaInternalLink is ParseForkanaInternalLink with instanceRelativePath(href)
// already known.
func parseForkanaInternalLink(href string, specialPages bool, rest string, urlForm bool) (link ForkanaInternalLink, ok bool) {
	if !urlForm {
		if IsFullURLString(href) || strings.HasPrefix(href, "/") || strings.HasPrefix(href, "#") || strings.HasPrefix(href, "?") {
			return link, false
		}
		rest = href
	}

	rest, link.Suffix = splitFragment(rest)
	// In a URL or an absolute path, "?" starts the query (a "?" in a subject name is written
	// "%3F"); only the hand-written "subject/Name" form may have a "?" in the name.
	if i := strings.IndexByte(rest, '?'); i >= 0 && (urlForm || !strings.HasPrefix(rest, "subject/")) {
		rest, link.Suffix = rest[:i], rest[i:]+link.Suffix
	}
	segments := strings.Split(strings.TrimSuffix(rest, "/"), "/")
	if !ForkanaInternalLinkRoots.Contains(segments[0]) {
		return link, false
	}
	if segments[0] != "subject" && !specialPages {
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
		if unescaped == "." || unescaped == ".." {
			// the link would be resolved to another page than the one checked
			return link, false
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
	if len(segments) > 2 && segments[0] == "subject" && segments[2] == "compare" {
		link.Owner = "" // "subject/X/compare/…" is a page of the subject, not an article
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
	_, onInstance := instanceRelativePath(href)
	return isExternalLinkOf(href, onInstance)
}

// isExternalLinkOf is isExternalLink with instanceRelativePath(href) already known.
func isExternalLinkOf(href string, onInstance bool) bool {
	if onInstance || !IsFullURLString(href) {
		return false
	}
	lower := strings.ToLower(href)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

// maxSubjectLinkLookups bounds the existence checks of a render (each costs two or three
// database queries): the links after that many distinct ones are taken as existing.
const maxSubjectLinkLookups = 100

// checkForkanaSubjectLink reports whether the subject a link points to has an article the
// viewer can open, and whether the owner has one (ownerExists is subjectExists when owner is
// empty). It asks the render helper once per subject and owner, and caches the answers for
// the render: a subject without articles answers for all its owners.
func (ctx *RenderContext) checkForkanaSubjectLink(subject, owner string) (subjectExists, ownerExists bool) {
	if DefaultRenderHelperFuncs == nil || DefaultRenderHelperFuncs.IsSubjectLinkExisting == nil {
		return true, true
	}
	subjectKey := subject + "\x00"
	subjectCached, hasSubject := ctx.subjectLinkExists[subjectKey]
	if hasSubject && (!subjectCached || owner == "") {
		return subjectCached, subjectCached
	}
	ownerKey := subjectKey + owner
	if ownerCached, ok := ctx.subjectLinkExists[ownerKey]; ok && hasSubject {
		return subjectCached, ownerCached
	}
	if ctx.subjectLinkLookups >= maxSubjectLinkLookups {
		return true, true
	}
	ctx.subjectLinkLookups++
	subjectExists, ownerExists = DefaultRenderHelperFuncs.IsSubjectLinkExisting(ctx, subject, owner)
	if ctx.subjectLinkExists == nil {
		ctx.subjectLinkExists = map[string]bool{}
	}
	ctx.subjectLinkExists[subjectKey] = subjectExists
	if owner != "" {
		ctx.subjectLinkExists[ownerKey] = ownerExists
	}
	return subjectExists, ownerExists
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

// addClass adds a class to the node, after the safe class it may already have.
func addClass(ctx *RenderContext, node *html.Node, class string) {
	key := ctx.RenderInternal.SafeAttr("class")
	for i, attr := range node.Attr {
		if attr.Key == key {
			node.Attr[i].Val += " " + class
			return
		}
	}
	node.Attr = append(node.Attr, ctx.RenderInternal.NodeSafeAttr("class", class))
}

// processForkanaLink resolves a Forkana internal link to the instance root and marks it, or
// marks an external link. It returns false when href is neither (the caller resolves it).
func processForkanaLink(ctx *RenderContext, node *html.Node, idx int, href string) bool {
	rest, onInstance := instanceRelativePath(href)
	if link, ok := parseForkanaInternalLink(href, ctx.RenderOptions.ForkanaArticle, rest, onInstance); ok {
		class := internalLinkClass
		subjectExists, ownerExists := true, true
		if link.Subject != "" {
			subjectExists, ownerExists = ctx.checkForkanaSubjectLink(link.Subject, link.Owner)
		}
		switch {
		case !ownerExists:
			class = internalLinkMissingClass
			if link.Owner != "" && subjectExists {
				// the author has no article on the subject: show the subject's articles
				node.Attr[idx].Val = ctx.ResolveLinkRoot("subject/" + url.PathEscape(link.Subject))
			} else {
				// Wikipedia's red link leads to the creation of the page
				node.Attr[idx].Val = ctx.ResolveLinkRoot("repo/create?subject=" + url.QueryEscape(link.Subject))
			}
		case IsFullURLString(href):
			node.Attr[idx].Val = href // already the page's URL on this instance
		default:
			node.Attr[idx].Val = ctx.ResolveLinkRoot(link.Path) + link.Suffix
		}
		addClass(ctx, node, class)
		return true
	}
	if ctx.RenderOptions.ForkanaArticle && isExternalLinkOf(href, onInstance) && !anchorContainsImage(node) {
		addClass(ctx, node, externalLinkClass)
	}
	return false
}
