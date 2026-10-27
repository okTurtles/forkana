// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"code.gitea.io/gitea/models/organization"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unit"
	"code.gitea.io/gitea/models/unittest"
	"code.gitea.io/gitea/modules/setting"
	repo_service "code.gitea.io/gitea/services/repository"
	"code.gitea.io/gitea/tests"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// shareTags are the og: and twitter: tags of a page, by property or name.
func shareTags(t *testing.T, link string) map[string]string {
	t.Helper()
	return sessionShareTags(t, nil, link)
}

// sessionShareTags are the share tags of a page as the session's user (signed out when nil) sees it.
func sessionShareTags(t *testing.T, session *TestSession, link string) map[string]string {
	t.Helper()
	var resp *httptest.ResponseRecorder
	if session != nil {
		resp = session.MakeRequest(t, NewRequest(t, "GET", link), http.StatusOK)
	} else {
		resp = MakeRequest(t, NewRequest(t, "GET", link), http.StatusOK)
	}
	doc := NewHTMLParser(t, resp.Body)
	tags := map[string]string{}
	doc.Find(`head meta`).Each(func(_ int, s *goquery.Selection) {
		key := s.AttrOr("property", s.AttrOr("name", ""))
		if strings.HasPrefix(key, "og:") || strings.HasPrefix(key, "twitter:") {
			_, dup := tags[key]
			assert.False(t, dup, "%s: %s is set twice", link, key)
			tags[key] = s.AttrOr("content", "")
		}
	})
	return tags
}

// assertSiteShareTags checks what every page shares: Forkana's name and image, a large
// card, and nothing of Gitea's.
func assertSiteShareTags(t *testing.T, tags map[string]string) {
	t.Helper()
	assert.Equal(t, "Forkana", tags["og:site_name"])
	assert.Equal(t, setting.AppURL+"assets/img/forkana-share.png", tags["og:image"])
	assert.Equal(t, tags["og:image"], tags["twitter:image"])
	assert.Equal(t, "1200", tags["og:image:width"])
	assert.Equal(t, "630", tags["og:image:height"])
	assert.NotEmpty(t, tags["og:image:alt"])
	assert.Equal(t, "summary_large_image", tags["twitter:card"])
	assert.NotEmpty(t, tags["og:description"])
	assert.Equal(t, tags["og:title"], tags["twitter:title"])
	assert.Equal(t, tags["og:description"], tags["twitter:description"])
	assert.LessOrEqual(t, len([]rune(tags["og:description"])), 200)
	for key, value := range tags {
		assert.NotContains(t, strings.ToLower(value), "gitea", "%s must not mention Gitea", key)
		assert.NotContains(t, value, "avatar", "%s must not be an avatar", key)
	}
}

// TestShareMeta covers what a link to Forkana shows when it is shared (#450).
func TestShareMeta(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	// the test configuration keeps Gitea's APP_NAME, which shared links must not show
	require.Contains(t, setting.AppName, "Gitea")

	owner, repo, subjectName := loadArticleRepo(t, 1)
	require.Empty(t, repo.Description, "repo1's summary comes from its README")
	subjectPath := "/subject/" + url.PathEscape(subjectName)
	articlePath := subjectPath + "/" + url.PathEscape(owner.Name)

	t.Run("Home", func(t *testing.T) {
		tags := shareTags(t, "/")
		assertSiteShareTags(t, tags)
		assert.Equal(t, "Forkana", tags["og:title"])
		assert.Equal(t, "website", tags["og:type"])
		assert.Equal(t, setting.ForkanaDescription, tags["og:description"])
		assert.Equal(t, setting.AppURL, tags["og:url"])
	})

	t.Run("Subject", func(t *testing.T) {
		for _, query := range []string{"", "?view=table", "?view=article"} {
			tags := shareTags(t, subjectPath+query)
			assertSiteShareTags(t, tags)
			assert.Equal(t, subjectName+" · Forkana", tags["og:title"], query)
			assert.Equal(t, "website", tags["og:type"], query)
			assert.Equal(t, setting.AppURL+strings.TrimPrefix(subjectPath, "/"), tags["og:url"], query)
			assert.Regexp(t, `^\d+ articles? on Forkana\. Description for repo1$`, tags["og:description"], query)
		}
	})

	t.Run("Article", func(t *testing.T) {
		tags := shareTags(t, articlePath)
		assertSiteShareTags(t, tags)
		assert.Equal(t, subjectName+" by "+owner.Name+" · Forkana", tags["og:title"])
		assert.Equal(t, "article", tags["og:type"])
		assert.Equal(t, setting.AppURL+strings.TrimPrefix(articlePath, "/"), tags["og:url"])
		// no description: the first paragraph of the README, its heading skipped
		assert.Equal(t, "Description for repo1", tags["og:description"])
		assert.NotContains(t, tags["og:title"], repo.Name+" ")
	})

	t.Run("ArticleSummary", func(t *testing.T) {
		original := repo.Description
		repo.Description = strings.Repeat("A long summary of the article. ", 10)
		require.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(t.Context(), repo, "description"))
		defer func() {
			repo.Description = original
			require.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(t.Context(), repo, "description"))
		}()

		tags := shareTags(t, articlePath)
		assertSiteShareTags(t, tags)
		assert.True(t, strings.HasPrefix(tags["og:description"], "A long summary of the article."), tags["og:description"])
		assert.True(t, strings.HasSuffix(tags["og:description"], "…"), tags["og:description"])
	})
}

// TestShareMetaWithoutCodeAccess checks that a viewer who may not read an article's code
// is not shared its README: org3/repo3 is private, its README's first paragraph is
// "Description for repo3", and user15 is only given access to its issues (team 7).
func TestShareMetaWithoutCodeAccess(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 3})
	require.True(t, repo.IsPrivate)
	require.Empty(t, repo.Description)
	team := unittest.AssertExistsAndLoadBean(t, &organization.Team{ID: 7})
	require.NoError(t, team.LoadUnits(t.Context()))
	require.False(t, team.UnitEnabled(t.Context(), unit.TypeCode), "team 7 must not read code")
	require.True(t, team.UnitEnabled(t.Context(), unit.TypeIssues))
	require.NoError(t, repo_service.TeamAddRepository(t.Context(), team, repo))

	const readme = "Description for repo3"
	issuesLink := "/" + repo.OwnerName + "/" + repo.Name + "/issues"
	issueLink := issuesLink + "/1"

	issuesOnly := loginUser(t, "user15")
	tags := sessionShareTags(t, issuesOnly, issuesLink)
	assertSiteShareTags(t, tags)
	assert.Equal(t, setting.ForkanaDescription, tags["og:description"])
	tags = sessionShareTags(t, issuesOnly, issueLink)
	assertSiteShareTags(t, tags)
	assert.NotContains(t, tags["og:description"], readme)
	// the issue's own content, which the viewer reads on the page anyway
	assert.Equal(t, "content6", tags["og:description"])

	// a reader of the code is shared the README
	owner := loginUser(t, "user2")
	tags = sessionShareTags(t, owner, issuesLink)
	assert.Equal(t, readme, tags["og:description"])
}
