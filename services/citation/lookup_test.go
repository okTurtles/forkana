// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package citation

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"code.gitea.io/gitea/modules/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeDOI(t *testing.T) {
	for in, want := range map[string]string{
		"10.1126/science.167.3918.449":                 "10.1126/science.167.3918.449",
		"doi:10.1126/science.167.3918.449":             "10.1126/science.167.3918.449",
		"https://doi.org/10.1126/science.167.3918.449": "10.1126/science.167.3918.449",
		"https://dx.doi.org/10.1038/nature12373.":      "10.1038/nature12373",
		"https://example.com/10.1126/x":                "",
		"978-0-670-81446-6":                            "",
		"":                                             "",
	} {
		assert.Equal(t, want, NormalizeDOI(in), in)
	}
}

func TestNormalizeISBN(t *testing.T) {
	for in, want := range map[string]string{
		"978-0-670-81446-6":      "9780670814466",
		"ISBN 978-0-670-81446-6": "9780670814466",
		"0-670-81446-6":          "0670814466",
		"0-8044-2957-X":          "080442957X",
		"978-0-670-81446-7":      "", // bad check digit
		"10.1126/science":        "",
		"https://example.com/":   "",
	} {
		assert.Equal(t, want, NormalizeISBN(in), in)
	}
}

func parse(t *testing.T, pageURL, page string) *Source {
	t.Helper()
	u, err := url.Parse(pageURL)
	require.NoError(t, err)
	return ParsePage(strings.NewReader(page), u)
}

func TestParsePage_Website(t *testing.T) {
	src := parse(t, "https://nssdc.gsfc.nasa.gov/nmc/spacecraft/display.action?id=1959-014A", `<!doctype html>
<html><head><title>
  Luna 2
</title></head><body><h1>Luna 2</h1></body></html>`)
	assert.Equal(t, &Source{Type: TypeWebsite, Title: "Luna 2", Site: "nssdc.gsfc.nasa.gov"}, src)
}

func TestParsePage_NewsOpenGraph(t *testing.T) {
	src := parse(t, "https://www.theguardian.com/science/2023/aug/23/india-moon-landing", `<html><head>
<meta property="og:title" content="India lands spacecraft near south pole of moon in world first">
<meta property="og:site_name" content="The Guardian">
<meta property="og:type" content="article">
<meta property="article:published_time" content="2023-08-23T13:05:41.000Z">
<meta name="author" content="Amrit Dhillon">
<meta property="article:author" content="https://www.theguardian.com/profile/amritdhillon">
<title>India lands spacecraft | The Guardian</title>
</head><body></body></html>`)
	assert.Equal(t, &Source{
		Type:      TypeNews,
		Title:     "India lands spacecraft near south pole of moon in world first",
		Site:      "The Guardian",
		Authors:   []string{"Dhillon, Amrit"},
		Published: "2023-08-23",
	}, src)
}

func TestParsePage_NewsJSONLD(t *testing.T) {
	src := parse(t, "https://news.example.org/2024/slim", `<html><head>
<title>SLIM</title>
<script type="application/ld+json">{"@context":"https://schema.org","@graph":[{"@type":"WebSite","name":"Example"},
{"@type":"NewsArticle","headline":"Japan announces successful SLIM lunar landing","datePublished":"2024-01-19T09:00:00Z",
"author":[{"@type":"Person","name":"Michael Sheetz"}],"publisher":{"@type":"Organization","name":"CNBC"}}]}</script>
</head><body></body></html>`)
	assert.Equal(t, &Source{
		Type:      TypeNews,
		Title:     "Japan announces successful SLIM lunar landing",
		Site:      "CNBC",
		Authors:   []string{"Sheetz, Michael"},
		Published: "2024-01-19",
	}, src)
}

func TestParsePage_Journal(t *testing.T) {
	src := parse(t, "https://www.science.org/doi/10.1126/science.167.3918.449", `<html><head>
<meta name="citation_title" content="Preliminary examination of lunar samples from Apollo 11">
<meta name="citation_author" content="Lunar Sample Preliminary Examination Team">
<meta name="citation_journal_title" content="Science">
<meta name="citation_publication_date" content="1970/01/30">
<meta name="citation_volume" content="167">
<meta name="citation_issue" content="3918">
<meta name="citation_firstpage" content="449">
<meta name="citation_lastpage" content="458">
<meta name="citation_doi" content="10.1126/science.167.3918.449">
</head></html>`)
	assert.Equal(t, &Source{
		Type:      TypeJournal,
		Title:     "Preliminary examination of lunar samples from Apollo 11",
		Authors:   []string{"Lunar Sample Preliminary Examination Team"},
		Journal:   "Science",
		Published: "1970-01-30",
		Volume:    "167",
		Issue:     "3918",
		Pages:     "449–458",
		DOI:       "10.1126/science.167.3918.449",
	}, src)
}

func TestIsNewsHost(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://www.nytimes.com/2023/08/23/world/asia/india-moon-landing.html": true,
		"https://apnews.com/article/x":                                          true,
		"https://edition.cnn.com/x":                                             true,
		"https://www.nasa.gov/":                                                 false,
		"https://notnytimes.com/":                                               false,
	} {
		u, _ := url.Parse(raw)
		assert.Equal(t, want, IsNewsHost(u), raw)
	}
}

func TestLookup_DOIAndISBN(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/doi/10.1126/science.167.3918.449":
			assert.Equal(t, "application/vnd.citationstyles.csl+json", r.Header.Get("Accept"))
			_, _ = w.Write([]byte(`{"type":"article-journal","title":"Preliminary examination of lunar samples from Apollo 11",
"container-title":"Science","author":[{"literal":"Lunar Sample Preliminary Examination Team"}],
"issued":{"date-parts":[[1970,1,30]]},"volume":"167","issue":"3918","page":"449-458",
"DOI":"10.1126/science.167.3918.449","URL":"https://doi.org/10.1126/science.167.3918.449"}`))
		case r.URL.Path == "/books":
			assert.Equal(t, "ISBN:9780670814466", r.URL.Query().Get("bibkeys"))
			_, _ = w.Write([]byte(`{"ISBN:9780670814466":{"title":"A Man on the Moon","publish_date":"1994",
"authors":[{"name":"Andrew Chaikin"}],"publishers":[{"name":"Viking"}],"url":"https://openlibrary.org/books/OL1M"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	defer test.MockVariableValue(&doiAPI, srv.URL+"/doi/")()
	defer test.MockVariableValue(&openLibraryAPI, srv.URL+"/books")()
	defer test.MockVariableValue(&client, func() *http.Client { return srv.Client() })()

	src, err := Lookup(t.Context(), "https://doi.org/10.1126/science.167.3918.449")
	require.NoError(t, err)
	assert.Equal(t, &Source{
		Type:      TypeJournal,
		URL:       "https://doi.org/10.1126/science.167.3918.449",
		Title:     "Preliminary examination of lunar samples from Apollo 11",
		Authors:   []string{"Lunar Sample Preliminary Examination Team"},
		Published: "1970-01-30",
		Journal:   "Science",
		Volume:    "167",
		Issue:     "3918",
		Pages:     "449–458",
		DOI:       "10.1126/science.167.3918.449",
	}, src)

	src, err = Lookup(t.Context(), "ISBN 978-0-670-81446-6")
	require.NoError(t, err)
	assert.Equal(t, &Source{
		Type:      TypeBook,
		URL:       "https://openlibrary.org/books/OL1M",
		Title:     "A Man on the Moon",
		Authors:   []string{"Chaikin, Andrew"},
		Published: "1994",
		ISBN:      "9780670814466",
		Publisher: "Viking",
	}, src)
}

func TestLookup_Unsupported(t *testing.T) {
	_, err := Lookup(t.Context(), "just some words")
	assert.ErrorIs(t, err, ErrUnsupported)
	_, err = Lookup(t.Context(), "ftp://example.com/file")
	assert.ErrorIs(t, err, ErrUnsupported)
}

func TestLookup_RefusesPrivateHosts(t *testing.T) {
	// The default client only reaches public internet hosts, on the standard ports: a user
	// must not be able to make the server fetch an address on its own network.
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`<title>secret</title>`))
	}))
	defer srv.Close()

	_, err := Lookup(t.Context(), srv.URL) // a loopback address on another port
	require.ErrorIs(t, err, ErrUnsupported)
	for _, target := range []string{"http://127.0.0.1/", "http://localhost/", "http://100.100.100.200/latest/meta-data/", "http://[::1]/"} {
		_, err = Lookup(t.Context(), target)
		require.Error(t, err, target)
		assert.Contains(t, err.Error(), "deny", target) // refused by the dialer, before connecting
	}
	assert.Zero(t, hits.Load())

	// a redirect may not go to another port either
	redirect := &http.Request{URL: &url.URL{Scheme: "http", Host: "example.com:8080", Path: "/"}}
	assert.ErrorIs(t, client().CheckRedirect(redirect, nil), ErrUnsupported)
	assert.NoError(t, client().CheckRedirect(&http.Request{URL: &url.URL{Scheme: "https", Host: "example.com", Path: "/"}}, nil))
}

func TestParsePage_IgnoresTitlesAfterHead(t *testing.T) {
	src := parse(t, "https://example.com/page", `<html><head></head><body><svg><title>Close</title></svg><h1>Page</h1></body></html>`)
	assert.Empty(t, src.Title)
}

func TestLookupFor_CacheAndRateLimit(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`{"type":"article-journal","title":"T"}`))
	}))
	defer srv.Close()
	defer test.MockVariableValue(&doiAPI, srv.URL+"/")()
	defer test.MockVariableValue(&client, func() *http.Client { return srv.Client() })()

	// the same DOI, written two ways, is fetched once
	src, err := LookupFor(t.Context(), 1, "10.1234/cache-test")
	require.NoError(t, err)
	assert.Equal(t, "T", src.Title)
	_, err = LookupFor(t.Context(), 1, "https://doi.org/10.1234/CACHE-test")
	require.NoError(t, err)
	assert.EqualValues(t, 1, hits.Load())

	now := time.Now()
	for range lookupsPerMinute {
		assert.True(t, allowLookup(42, now))
	}
	assert.False(t, allowLookup(42, now), "over the limit")
	assert.True(t, allowLookup(43, now), "other users are not limited")
	assert.True(t, allowLookup(42, now.Add(time.Minute)), "a new minute")
}
