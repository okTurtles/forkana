// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

// Package citation looks up the details of a source an article cites (issue #399): the Cite
// dialog of the article editor takes one field, a URL, a DOI or an ISBN, and "Forkana fills
// in the details" (title, authors, publication, date…), the way Wikipedia's Citoid does.
//
//   - a DOI is looked up through doi.org's content negotiation (CSL JSON, which Crossref,
//     DataCite and mEDRA all serve), and the source is a journal article;
//   - an ISBN is looked up in Open Library, and the source is a book;
//   - any other URL is fetched and read: the citation_* meta tags (Google Scholar's, used by
//     most journals), Open Graph, schema.org JSON-LD and the page title (all in the <head>).
//     The source is a news article when its JSON-LD says so (NewsArticle) or it is on a
//     well-known news site (news_hosts.json), and a website otherwise.
//
// The URL comes from the user, so: only public internet hosts are contacted (hostmatcher's
// "external", minus the shared and translation ranges below), on the standard ports, without
// going through a proxy (a proxy would hide the target from that check); the responses are
// size-limited and the whole lookup has a timeout.
package citation

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"code.gitea.io/gitea/modules/hostmatcher"
	"code.gitea.io/gitea/modules/json"

	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

// Source types, shared with the editor (web_src/js/features/citations/citation-format.ts).
const (
	TypeWebsite = "website"
	TypeNews    = "news"
	TypeJournal = "journal"
	TypeBook    = "book"
)

// Source is what a lookup found about a source. Empty fields were not found.
type Source struct {
	Type      string   `json:"type"`
	URL       string   `json:"url,omitempty"`
	Title     string   `json:"title,omitempty"`
	Site      string   `json:"site,omitempty"`      // website name or news publication
	Authors   []string `json:"authors,omitempty"`   // "Last, First" when known
	Published string   `json:"published,omitempty"` // YYYY-MM-DD, YYYY-MM or YYYY
	Journal   string   `json:"journal,omitempty"`
	Volume    string   `json:"volume,omitempty"`
	Issue     string   `json:"issue,omitempty"`
	Pages     string   `json:"pages,omitempty"`
	DOI       string   `json:"doi,omitempty"`
	ISBN      string   `json:"isbn,omitempty"`
	Publisher string   `json:"publisher,omitempty"`
}

// ErrUnsupported is returned when the query is neither a URL, a DOI nor an ISBN.
var ErrUnsupported = errors.New("not a URL, DOI or ISBN")

const (
	lookupTimeout   = 10 * time.Second
	maxResponseSize = 2 << 20 // a page's <head> is far smaller; enough for the JSON APIs too
	userAgent       = "Forkana citation lookup (+https://forkana.org)"
)

var (
	doiRe  = regexp.MustCompile(`(?i)\b(10\.\d{4,9}/[^\s"<>]+)`)
	isbnRe = regexp.MustCompile(`^(?i:isbn(?:-1[03])?:?\s*)?([0-9][0-9\- ]{8,15}[0-9Xx])$`)
)

// doiAPI and openLibraryAPI are variables so the tests can point them at a local server.
var (
	doiAPI         = "https://doi.org/"
	openLibraryAPI = "https://openlibrary.org/api/books"
)

// ErrRedirectRefused is returned when a page redirects somewhere a lookup may not go.
var ErrRedirectRefused = errors.New("redirect refused")

// client is the HTTP client of the lookups, which may only reach public internet hosts
// (the tests replace it to reach their local server).
var client = sync.OnceValue(func() *http.Client {
	allow := hostmatcher.ParseHostMatchList("citation lookup", hostmatcher.MatchBuiltinExternal)
	// "external" still lets through carrier-grade NAT (where some clouds put their
	// metadata service), benchmarking and IPv6 translation ranges
	block := hostmatcher.ParseHostMatchList("citation lookup", "100.64.0.0/10,198.18.0.0/15,64:ff9b::/96,64:ff9b:1::/48,2002::/16")
	return &http.Client{
		Timeout: lookupTimeout,
		Transport: &http.Transport{
			// no proxy: through one, the dialer would only see the proxy's address
			Proxy:       nil,
			DialContext: hostmatcher.NewDialContext("citation lookup", allow, block, nil),
			// the hosts are user-chosen and rarely reused: keep no idle connection to them
			DisableKeepAlives:     true,
			TLSHandshakeTimeout:   lookupTimeout,
			ResponseHeaderTimeout: lookupTimeout,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("%w: too many redirects", ErrRedirectRefused)
			}
			if err := checkURL(req.URL); err != nil {
				// not ErrUnsupported: the query itself was fine
				return fmt.Errorf("%w: %s", ErrRedirectRefused, err.Error())
			}
			return nil
		},
	}
})

// checkURL allows only http(s) URLs on the standard ports.
func checkURL(u *url.URL) error {
	if u.Scheme != "http" && u.Scheme != "https" {
		return ErrUnsupported
	}
	if port := u.Port(); port != "" && port != "80" && port != "443" {
		return fmt.Errorf("%w: port %s", ErrUnsupported, port)
	}
	return nil
}

// NormalizeDOI returns the DOI in s (a bare DOI, "doi:…" or a doi.org URL), or "".
func NormalizeDOI(s string) string {
	s = strings.TrimSpace(s)
	if u, err := url.Parse(s); err == nil && (u.Host == "doi.org" || u.Host == "dx.doi.org") {
		s, _ = url.PathUnescape(strings.TrimPrefix(u.Path, "/"))
	}
	s = strings.TrimPrefix(strings.TrimPrefix(s, "doi:"), "DOI:")
	m := doiRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil || !strings.HasPrefix(strings.TrimSpace(s), m[1]) {
		return ""
	}
	return strings.TrimRight(m[1], ".,;")
}

// NormalizeISBN returns the ISBN-10 or ISBN-13 in s, without hyphens or spaces, if its check
// digit is valid; or "".
func NormalizeISBN(s string) string {
	m := isbnRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return ""
	}
	isbn := strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(m[1]))
	switch len(isbn) {
	case 10:
		sum := 0
		for i, c := range isbn {
			v := int(c - '0')
			if c == 'X' {
				if i != 9 {
					return ""
				}
				v = 10
			}
			sum += (10 - i) * v
		}
		if sum%11 == 0 {
			return isbn
		}
	case 13:
		if strings.ContainsRune(isbn, 'X') {
			return ""
		}
		sum := 0
		for i, c := range isbn {
			w := 1
			if i%2 == 1 {
				w = 3
			}
			sum += w * int(c-'0')
		}
		if sum%10 == 0 {
			return isbn
		}
	}
	return ""
}

// query is a parsed lookup query: one of doi, isbn and pageURL is set.
type query struct {
	doi, isbn, pageURL string
}

// parseQuery reads a DOI, an ISBN or an http(s) URL on a standard port, or returns
// ErrUnsupported.
func parseQuery(raw string) (query, error) {
	raw = strings.TrimSpace(raw)
	if doi := NormalizeDOI(raw); doi != "" {
		return query{doi: doi}, nil
	}
	if isbn := NormalizeISBN(raw); isbn != "" {
		return query{isbn: isbn}, nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return query{}, ErrUnsupported
	}
	if err := checkURL(u); err != nil {
		return query{}, err
	}
	return query{pageURL: u.String()}, nil
}

// Lookup finds the details of the source query names: a DOI, an ISBN or an http(s) URL.
func Lookup(ctx context.Context, raw string) (*Source, error) {
	q, err := parseQuery(raw)
	if err != nil {
		return nil, err
	}
	return q.lookup(ctx)
}

func (q query) lookup(ctx context.Context) (*Source, error) {
	ctx, cancel := context.WithTimeout(ctx, lookupTimeout)
	defer cancel()
	switch {
	case q.doi != "":
		return lookupDOI(ctx, q.doi)
	case q.isbn != "":
		return lookupISBN(ctx, q.isbn)
	default:
		return lookupPage(ctx, q.pageURL)
	}
}

// cacheKey is the query in a canonical form.
func (q query) cacheKey() string {
	switch {
	case q.doi != "":
		return "doi:" + strings.ToLower(q.doi)
	case q.isbn != "":
		return "isbn:" + q.isbn
	default:
		return "url:" + q.pageURL
	}
}

func get(ctx context.Context, rawURL, accept string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", accept)
	resp, err := client().Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("%s: HTTP %d", rawURL, resp.StatusCode)
	}
	return resp, nil
}

// cslItem is the part of a CSL JSON item the lookup reads.
type cslItem struct {
	Title          json.RawMessage `json:"title"`
	ContainerTitle json.RawMessage `json:"container-title"`
	Author         []struct {
		Family  string `json:"family"`
		Given   string `json:"given"`
		Literal string `json:"literal"`
		Name    string `json:"name"`
	} `json:"author"`
	Issued struct {
		DateParts [][]json.Number `json:"date-parts"`
	} `json:"issued"`
	Volume    json.RawMessage `json:"volume"`
	Issue     json.RawMessage `json:"issue"`
	Page      json.RawMessage `json:"page"`
	Publisher string          `json:"publisher"`
	URL       string          `json:"URL"`
	DOI       string          `json:"DOI"`
	Type      string          `json:"type"`
}

// cslString reads a CSL field that is a string, a number or a list of strings.
func cslString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
	}
	var list []string
	if json.Unmarshal(raw, &list) == nil && len(list) > 0 {
		return strings.TrimSpace(list[0])
	}
	var n json.Number
	if json.Unmarshal(raw, &n) == nil {
		return n.String()
	}
	return ""
}

func cslDate(parts [][]json.Number) string {
	if len(parts) == 0 || len(parts[0]) == 0 {
		return ""
	}
	p := parts[0]
	out := make([]string, 0, 3)
	for i, n := range p {
		v, err := n.Int64()
		if err != nil || i > 2 {
			break
		}
		if i == 0 {
			out = append(out, strconv.FormatInt(v, 10))
		} else {
			out = append(out, fmt.Sprintf("%02d", v))
		}
	}
	return strings.Join(out, "-")
}

// escapeDOI escapes a DOI for a doi.org URL: each "/"-separated part is a path segment.
func escapeDOI(doi string) string {
	parts := strings.Split(doi, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

func lookupDOI(ctx context.Context, doi string) (*Source, error) {
	resp, err := get(ctx, doiAPI+escapeDOI(doi), "application/vnd.citationstyles.csl+json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var item cslItem
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseSize)).Decode(&item); err != nil {
		return nil, fmt.Errorf("doi %s: %w", doi, err)
	}
	src := &Source{
		Type:      TypeJournal,
		Title:     cslString(item.Title),
		Journal:   cslString(item.ContainerTitle),
		Published: cslDate(item.Issued.DateParts),
		Volume:    cslString(item.Volume),
		Issue:     cslString(item.Issue),
		Pages:     strings.ReplaceAll(cslString(item.Page), "-", "–"),
		Publisher: item.Publisher,
		DOI:       doi,
		URL:       item.URL,
	}
	if item.Type == "book" || item.Type == "monograph" {
		src.Type = TypeBook
	}
	for _, a := range item.Author {
		switch {
		case a.Family != "" && a.Given != "":
			src.Authors = append(src.Authors, a.Family+", "+a.Given)
		case a.Family != "":
			src.Authors = append(src.Authors, a.Family)
		case a.Literal != "":
			src.Authors = append(src.Authors, a.Literal)
		case a.Name != "":
			src.Authors = append(src.Authors, a.Name)
		}
	}
	return src, nil
}

func lookupISBN(ctx context.Context, isbn string) (*Source, error) {
	q := url.Values{"bibkeys": {"ISBN:" + isbn}, "format": {"json"}, "jscmd": {"data"}}
	resp, err := get(ctx, openLibraryAPI+"?"+q.Encode(), "application/json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var books map[string]struct {
		Title       string `json:"title"`
		URL         string `json:"url"`
		PublishDate string `json:"publish_date"`
		Authors     []struct {
			Name string `json:"name"`
		} `json:"authors"`
		Publishers []struct {
			Name string `json:"name"`
		} `json:"publishers"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseSize)).Decode(&books); err != nil {
		return nil, fmt.Errorf("isbn %s: %w", isbn, err)
	}
	book, ok := books["ISBN:"+isbn]
	if !ok {
		return nil, fmt.Errorf("isbn %s: not found", isbn)
	}
	src := &Source{Type: TypeBook, Title: book.Title, ISBN: isbn, URL: book.URL, Published: yearOf(book.PublishDate)}
	for _, a := range book.Authors {
		src.Authors = append(src.Authors, invertName(a.Name))
	}
	if len(book.Publishers) > 0 {
		src.Publisher = book.Publishers[0].Name
	}
	return src, nil
}

var yearRe = regexp.MustCompile(`\b(1[5-9]\d\d|2\d\d\d)\b`)

func yearOf(s string) string {
	return yearRe.FindString(s)
}

// invertName turns "Andrew Chaikin" into "Chaikin, Andrew", the order of a citation; a name
// that already has a comma, or a single word, is kept.
func invertName(name string) string {
	name = strings.Join(strings.Fields(name), " ")
	if name == "" || strings.Contains(name, ",") {
		return name
	}
	i := strings.LastIndexByte(name, ' ')
	if i < 0 {
		return name
	}
	return name[i+1:] + ", " + name[:i]
}

func lookupPage(ctx context.Context, rawURL string) (*Source, error) {
	resp, err := get(ctx, rawURL, "text/html,application/xhtml+xml")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	contentType := resp.Header.Get("Content-Type")
	if contentType != "" && !strings.Contains(contentType, "html") {
		// a PDF or an image: nothing to read, the URL alone makes the citation
		return &Source{Type: TypeWebsite, URL: rawURL, Site: siteName(resp.Request.URL)}, nil
	}
	body, err := charset.NewReader(io.LimitReader(resp.Body, maxResponseSize), contentType)
	if err != nil {
		return nil, err
	}
	src := ParsePage(body, resp.Request.URL)
	src.URL = rawURL
	return src, nil
}

// siteName is the host of u without "www.", the website of a page that does not name itself.
func siteName(u *url.URL) string {
	if u == nil {
		return ""
	}
	return strings.TrimPrefix(u.Hostname(), "www.")
}

type pageMeta struct {
	values map[string][]string // lower-cased meta name/property → contents, in order
	title  string
	jsonLD []string
}

func (m *pageMeta) first(keys ...string) string {
	for _, k := range keys {
		for _, v := range m.values[k] {
			if v = strings.TrimSpace(v); v != "" {
				return v
			}
		}
	}
	return ""
}

// pageTitle is the page's <title>, its white space collapsed.
func (m *pageMeta) pageTitle() string {
	return strings.Join(strings.Fields(m.title), " ")
}

func (m *pageMeta) all(key string) []string {
	var out []string
	for _, v := range m.values[key] {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func readPageMeta(r io.Reader) *pageMeta {
	m := &pageMeta{values: map[string][]string{}}
	z := html.NewTokenizer(r)
	inTitle, inJSONLD := false, false
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			return m
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			switch string(name) {
			case "meta":
				var key, content string
				for hasAttr {
					var k, v []byte
					k, v, hasAttr = z.TagAttr()
					switch string(k) {
					case "name", "property", "itemprop":
						if key == "" {
							key = strings.ToLower(string(v))
						}
					case "content":
						content = string(v)
					}
				}
				if key != "" {
					m.values[key] = append(m.values[key], content)
				}
			case "title":
				inTitle = m.title == ""
			case "script":
				for hasAttr {
					var k, v []byte
					k, v, hasAttr = z.TagAttr()
					if string(k) == "type" && strings.EqualFold(string(v), "application/ld+json") {
						inJSONLD = true
					}
				}
			case "body", "svg":
				// everything the lookup reads is in <head>; a <title> after it is an icon's
				return m
			}
		case html.TextToken:
			if inTitle {
				m.title += string(z.Text())
			} else if inJSONLD {
				m.jsonLD = append(m.jsonLD, string(z.Text()))
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			switch string(name) {
			case "title":
				inTitle = false
			case "script":
				inJSONLD = false
			}
		}
	}
}

// ldObject is the part of a schema.org JSON-LD object the lookup reads.
type ldObject struct {
	Type          json.RawMessage `json:"@type"`
	Headline      string          `json:"headline"`
	Name          string          `json:"name"`
	DatePublished string          `json:"datePublished"`
	Author        json.RawMessage `json:"author"`
	Publisher     json.RawMessage `json:"publisher"`
	Graph         []ldObject      `json:"@graph"`
}

func ldTypes(raw json.RawMessage) []string {
	var one string
	if json.Unmarshal(raw, &one) == nil {
		return []string{one}
	}
	var many []string
	_ = json.Unmarshal(raw, &many)
	return many
}

func ldNames(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var one struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(raw, &one) == nil && one.Name != "" {
		return []string{one.Name}
	}
	var many []struct {
		Name string `json:"name"`
	}
	var out []string
	if json.Unmarshal(raw, &many) == nil {
		for _, a := range many {
			if a.Name != "" {
				out = append(out, a.Name)
			}
		}
	}
	var s string
	if len(out) == 0 && json.Unmarshal(raw, &s) == nil && s != "" {
		out = append(out, s)
	}
	return out
}

// articleLD returns the first article-like object of the page's JSON-LD.
func articleLD(blocks []string) *ldObject {
	var candidates []ldObject
	for _, b := range blocks {
		var one ldObject
		if json.Unmarshal([]byte(b), &one) == nil {
			candidates = append(candidates, one)
			candidates = append(candidates, one.Graph...)
			continue
		}
		var many []ldObject
		if json.Unmarshal([]byte(b), &many) == nil {
			for _, item := range many {
				candidates = append(candidates, item)
				candidates = append(candidates, item.Graph...)
			}
		}
	}
	for i := range candidates {
		for _, t := range ldTypes(candidates[i].Type) {
			if strings.HasSuffix(t, "Article") || t == "BlogPosting" || t == "Report" {
				return &candidates[i]
			}
		}
	}
	return nil
}

func isNewsType(types []string) bool {
	for _, t := range types {
		if t == "NewsArticle" || t == "ReportageNewsArticle" || t == "AnalysisNewsArticle" {
			return true
		}
	}
	return false
}

// isoDate keeps the date part of an ISO 8601 date-time ("2023-08-23T10:00:00Z" → "2023-08-23").
func isoDate(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 10 && s[4] == '-' && s[7] == '-' {
		return s[:10]
	}
	if len(s) >= 10 && s[4] == '/' && s[7] == '/' {
		return strings.ReplaceAll(s[:10], "/", "-")
	}
	return yearOf(s)
}

// ParsePage reads the citation details of an HTML page at pageURL.
func ParsePage(r io.Reader, pageURL *url.URL) *Source {
	m := readPageMeta(r)
	ld := articleLD(m.jsonLD)
	src := &Source{Type: TypeWebsite}

	// Scholarly pages (Google Scholar's citation_* tags) are journal articles.
	if doi := NormalizeDOI(m.first("citation_doi", "dc.identifier", "prism.doi")); doi != "" || m.first("citation_journal_title") != "" {
		src.Type = TypeJournal
		src.DOI = doi
		src.Title = m.first("citation_title", "dc.title")
		src.Journal = m.first("citation_journal_title")
		src.Volume = m.first("citation_volume")
		src.Issue = m.first("citation_issue")
		if first, last := m.first("citation_firstpage"), m.first("citation_lastpage"); first != "" && last != "" {
			src.Pages = first + "–" + last
		} else {
			src.Pages = first
		}
		src.Published = isoDate(m.first("citation_publication_date", "citation_date", "citation_online_date"))
		src.Authors = m.all("citation_author")
		src.Publisher = m.first("citation_publisher", "dc.publisher")
		if src.Title == "" {
			src.Title = m.pageTitle()
		}
		return src
	}

	src.Title = m.first("citation_title", "og:title", "twitter:title")
	if src.Title == "" && ld != nil {
		src.Title = strings.TrimSpace(ld.Headline)
	}
	if src.Title == "" {
		src.Title = m.pageTitle()
	}
	src.Site = m.first("og:site_name", "application-name")
	if src.Site == "" && ld != nil {
		if names := ldNames(ld.Publisher); len(names) > 0 {
			src.Site = names[0]
		}
	}
	if src.Site == "" {
		src.Site = siteName(pageURL)
	}
	src.Published = isoDate(m.first("article:published_time", "citation_publication_date", "date", "dc.date", "pubdate", "datepublished"))
	if src.Published == "" && ld != nil {
		src.Published = isoDate(ld.DatePublished)
	}

	var authors []string
	for _, a := range append(m.all("author"), m.all("article:author")...) {
		if strings.HasPrefix(a, "http://") || strings.HasPrefix(a, "https://") {
			continue // a profile URL, not a name
		}
		authors = append(authors, a)
	}
	if len(authors) == 0 && ld != nil {
		authors = ldNames(ld.Author)
	}
	seen := map[string]bool{}
	for _, a := range authors {
		for name := range strings.SplitSeq(a, " and ") {
			name = invertName(name)
			if name != "" && !seen[name] && name != src.Site {
				seen[name] = true
				src.Authors = append(src.Authors, name)
			}
		}
	}

	if (ld != nil && isNewsType(ldTypes(ld.Type))) || IsNewsHost(pageURL) {
		src.Type = TypeNews
	}
	return src
}
