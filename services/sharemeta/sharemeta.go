// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

// Package sharemeta builds what a link to a subject or an article shows when it is shared
// (Slack, Discord, X, iMessage...): the og: and twitter: tags rendered by
// custom/templates/base/head_opengraph.tmpl (#450).
package sharemeta

import (
	"context"
	"io"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"unicode"

	repo_model "code.gitea.io/gitea/models/repo"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/gitrepo"
	"code.gitea.io/gitea/modules/httplib"
	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/setting"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

// DataKey is the template data key holding the page's *Meta.
const DataKey = "ShareMeta"

// MaxDescriptionLength bounds a shared description, in characters.
const MaxDescriptionLength = 200

// maxReadmeBytes bounds how much of a README is read to find its first paragraph.
const maxReadmeBytes = 64 * 1024

// Meta is what a shared link to the page shows.
type Meta struct {
	Title string
	Type  string // og:type: "website" for a subject, "article" for an article
	URL   string // the absolute canonical url

	describe    func() string
	once        sync.Once
	description string
}

// Description is the shared description. It is computed when the page head is rendered
// (it may read the README), so that the routes that never render a page skip the work.
func (m *Meta) Description() string {
	m.once.Do(func() {
		if m.describe != nil {
			m.description = m.describe()
		}
		if m.description == "" {
			m.description = setting.ShareSiteDescription()
		}
	})
	return m.description
}

func titleWithSite(title string) string {
	return title + " · " + setting.ShareSiteName()
}

// ForSubject is the share data of a subject page: the subject's name, and how many
// articles the doer can read in it followed by the summary of its main article.
func ForSubject(ctx context.Context, subject *repo_model.Subject, mainArticle *repo_model.Repository, doer *user_model.User) *Meta {
	return &Meta{
		Title: titleWithSite(subject.Name),
		Type:  "website",
		URL:   httplib.MakeAbsoluteURL(ctx, setting.AppSubURL+"/subject/"+url.PathEscape(subject.Name)),
		describe: func() string {
			return subjectDescription(ctx, subject, mainArticle, doer)
		},
	}
}

func subjectDescription(ctx context.Context, subject *repo_model.Subject, mainArticle *repo_model.Repository, doer *user_model.User) string {
	counts, err := repo_model.CountReadableArticlesBySubjectIDs(ctx, []int64{subject.ID}, doer)
	if err != nil {
		log.Warn("CountReadableArticlesBySubjectIDs for subject %d: %v", subject.ID, err)
	}
	count := counts[subject.ID]
	if count < 1 {
		count = 1 // the page renders the main article, so there is at least that one
	}
	articles := strconv.FormatInt(count, 10) + " article"
	if count != 1 {
		articles += "s"
	}
	if summary := ArticleSummary(ctx, mainArticle); summary != "" {
		return Ellipsis(articles+" on "+setting.ShareSiteName()+". "+summary, MaxDescriptionLength)
	}
	return Ellipsis(articles+" about "+subject.Name+" on "+setting.ShareSiteName()+".", MaxDescriptionLength)
}

// ForArticle is the share data of an article page (any page of an article: its view, its
// editor, its change requests): its subject and owner, and its summary. link is the page
// requested, the canonical article url replaces it when it names the article itself.
// canReadCode tells whether the viewer may read the article's code: one who may not (a
// collaborator with issue access only, a repository whose code unit is disabled) gets
// its description alone, never its README.
func ForArticle(ctx context.Context, repo *repo_model.Repository, link string, canReadCode bool) *Meta {
	pageURL := httplib.MakeAbsoluteURL(ctx, link)
	if strings.EqualFold(strings.TrimSuffix(link, "/"), repo.LinkCtx(ctx)) {
		pageURL = repo.HTMLURL(ctx)
	}
	return &Meta{
		Title: titleWithSite(repo.GetSubject(ctx) + " by " + repo.OwnerName),
		Type:  "article",
		URL:   pageURL,
		describe: func() string {
			if !canReadCode {
				return articleDescription(repo)
			}
			return ArticleSummary(ctx, repo)
		},
	}
}

// articleDescription is the article's description (public repository metadata), cut to
// MaxDescriptionLength characters; none for a tombstone.
func articleDescription(repo *repo_model.Repository) string {
	if repo == nil || repo.IsTombstone() {
		return ""
	}
	return Ellipsis(collapseSpaces(repo.Description), MaxDescriptionLength)
}

// TextSummary is the plain text of the first paragraph of a markdown text (a change
// request's content, a profile's description), cut to MaxDescriptionLength characters.
func TextSummary(markdown string) string {
	return Ellipsis(FirstParagraph([]byte(markdown)), MaxDescriptionLength)
}

// ArticleSummary is the article's summary: its description, or else the first paragraph
// of its README stripped of markdown, at most MaxDescriptionLength characters. A deleted
// article (a tombstone) has none: its content must not be served.
func ArticleSummary(ctx context.Context, repo *repo_model.Repository) string {
	if desc := articleDescription(repo); desc != "" || repo == nil || repo.IsTombstone() {
		return desc
	}
	if repo.IsEmpty || repo.IsBroken() {
		return ""
	}
	readme, err := readReadme(ctx, repo)
	if err != nil {
		log.Warn("Reading the README of %s for its summary: %v", repo.FullName(), err)
		return ""
	}
	return Ellipsis(FirstParagraph(readme), MaxDescriptionLength)
}

func readReadme(ctx context.Context, repo *repo_model.Repository) ([]byte, error) {
	gitRepo, closer, err := gitrepo.RepositoryFromContextOrOpen(ctx, repo)
	if err != nil {
		return nil, err
	}
	defer closer.Close()

	commit, err := gitRepo.GetBranchCommit(repo.DefaultBranch)
	if err != nil {
		return nil, err
	}
	entries, err := commit.ListEntries()
	if err != nil {
		return nil, err
	}
	entry := FindReadme(entries)
	if entry == nil {
		return nil, nil
	}
	rc, err := entry.Blob().DataAsync()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, maxReadmeBytes))
}

// FindReadme finds the README among a tree's entries: the one the article view renders
// (routers/web/explore), and so the one its shared summary is taken from.
func FindReadme(entries []*git.TreeEntry) *git.TreeEntry {
	for _, entry := range entries {
		if entry.IsRegular() || entry.IsExecutable() {
			switch strings.ToLower(entry.Name()) {
			case "readme.md", "readme", "readme.txt":
				return entry
			}
		}
	}
	return nil
}

var markdownParser = goldmark.New(goldmark.WithExtensions(extension.GFM, extension.Footnote)).Parser()

// FirstParagraph is the plain text of the first paragraph of a markdown document:
// headings, lists, quotes, tables, code blocks and html are skipped, and so are the
// paragraphs holding nothing but images; links keep their text, images and footnote
// references are dropped.
func FirstParagraph(source []byte) string {
	doc := markdownParser.Parse(text.NewReader(source))
	for block := doc.FirstChild(); block != nil; block = block.NextSibling() {
		if block.Kind() != ast.KindParagraph {
			continue
		}
		if s := collapseSpaces(inlineText(block, source)); s != "" {
			return s
		}
	}
	return ""
}

func inlineText(node ast.Node, source []byte) string {
	var sb strings.Builder
	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.Image, *ast.RawHTML, *extast.FootnoteLink:
			return ast.WalkSkipChildren, nil
		case *ast.Text:
			sb.Write(n.Segment.Value(source))
			if n.SoftLineBreak() || n.HardLineBreak() {
				sb.WriteByte(' ')
			}
		case *ast.String:
			sb.Write(n.Value)
		case *ast.AutoLink:
			sb.Write(n.Label(source))
		}
		return ast.WalkContinue, nil
	})
	return sb.String()
}

func collapseSpaces(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// Ellipsis cuts s to at most limit characters, at a word boundary when there is one in
// its second half, and ends it with "…" when it is cut.
func Ellipsis(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	cut := runes[:limit-1]
	for i := len(cut) - 1; i > limit/2; i-- {
		if unicode.IsSpace(cut[i]) {
			cut = cut[:i]
			break
		}
	}
	return strings.TrimRightFunc(string(cut), func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r)
	}) + "…"
}
