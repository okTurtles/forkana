// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	"code.gitea.io/gitea/tests"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// subjectPage is what the tests read from a rendered subject page.
type subjectPage struct {
	owner     string
	chosen    bool
	hasReader bool
	rows      int
}

// getSubjectPage requests the subject page with query, as session (nil: anonymous). The
// body is returned too, for checks on the embedded fork graph.
func getSubjectPage(t *testing.T, session *TestSession, subjectName, query string) (subjectPage, string) {
	t.Helper()
	req := NewRequest(t, "GET", "/subject/"+url.PathEscape(subjectName)+"?"+query)
	var body string
	if session != nil {
		body = session.MakeRequest(t, req, http.StatusOK).Body.String()
	} else {
		body = MakeRequest(t, req, http.StatusOK).Body.String()
	}
	assert.Contains(t, body, "subjectForkGraph", "the fork graph is embedded for the Bubble view")
	doc := NewHTMLParser(t, bytes.NewBufferString(body))
	app := doc.Find("#repo-history-app")
	initialOwner, _ := app.Attr("data-initial-owner")
	chosen, _ := app.Attr("data-initial-article")
	return subjectPage{
		owner:     initialOwner,
		chosen:    chosen == "true",
		hasReader: doc.Find("#article-view-root").Length() == 1,
		rows:      doc.Find("#articles-table tr.article-row").Length(),
	}, body
}

// The subject page's Article view renders the article it is asked for, on the server, so
// the reader never sees another article first (#405), and the Bubble view gets the fork
// graph the Table view was built from, so it does not request it again.
func TestSubjectPageArticleViewSelection(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	owner, repo, subjectName := loadArticleRepo(t, 1)
	get := func(t *testing.T, query string) subjectPage {
		t.Helper()
		page, _ := getSubjectPage(t, nil, subjectName, query)
		return page
	}

	t.Run("SingleArticleIsChosen", func(t *testing.T) {
		got := get(t, "view=article")
		assert.Equal(t, subjectPage{owner: owner.Name, chosen: true, hasReader: true, rows: 1}, got)
	})

	fork := forkArticle(t, repo).FullName()

	t.Run("NoSelectionRendersNoArticle", func(t *testing.T) {
		got := get(t, "view=article")
		assert.False(t, got.chosen)
		assert.False(t, got.hasReader, "the main article must not be rendered when none is chosen")
		assert.Equal(t, 2, got.rows)
	})

	t.Run("SelectedForkIsRenderedDirectly", func(t *testing.T) {
		got := get(t, "view=article&selected="+url.QueryEscape(fork))
		assert.Equal(t, subjectPage{owner: "user4", chosen: true, hasReader: true, rows: 2}, got)
	})

	t.Run("SelectedMainArticle", func(t *testing.T) {
		got := get(t, "view=article&selected="+url.QueryEscape(fmt.Sprintf("%s/%s", owner.Name, repo.Name)))
		assert.Equal(t, subjectPage{owner: owner.Name, chosen: true, hasReader: true, rows: 2}, got)
	})

	t.Run("ForeignSelectionIsIgnored", func(t *testing.T) {
		// repo 3 belongs to another subject, so it is not one of this subject's articles
		got := get(t, "view=article&selected="+url.QueryEscape("org3/repo3"))
		assert.Equal(t, owner.Name, got.owner)
		assert.False(t, got.chosen)
		assert.False(t, got.hasReader)
	})

	t.Run("OtherViewsRenderNoArticle", func(t *testing.T) {
		got := get(t, "view=table&selected="+url.QueryEscape(fork))
		assert.Equal(t, owner.Name, got.owner)
		assert.False(t, got.chosen)
		assert.Equal(t, 2, got.rows)
	})
}

// A subject with one live article and a tombstone has a single article to show: the
// tombstone stays in the graph for its forks' ancestry, but is not one the reader can
// choose, so the Article view still opens the live one by itself.
func TestSubjectPageOnlyLiveArticleIsChosen(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	owner, repo, subjectName := loadArticleRepo(t, 1)
	forkArticle(t, repo)

	session := loginUser(t, owner.Name)
	req := NewRequestWithValues(t, "POST", fmt.Sprintf("/%s/%s/settings", owner.Name, repo.Name),
		deleteForm(GetUserCSRFToken(t, session), owner.Name, subjectName))
	session.MakeRequest(t, req, http.StatusSeeOther)
	require.True(t, unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: repo.ID}).IsTombstoned)

	got, _ := getSubjectPage(t, nil, subjectName, "view=article")
	assert.True(t, got.chosen, "the only live article is chosen")
	assert.True(t, got.hasReader)

	// the tombstone itself is never chosen, even when it is asked for
	got, _ = getSubjectPage(t, nil, subjectName, "view=article&selected="+url.QueryEscape(repo.FullName()))
	assert.True(t, got.chosen, "an ignored selection falls back to the only live article")
	assert.Equal(t, "user4", got.owner)
}

// A private article is listed for the readers who may see it, and only for them: the
// subject page builds its graph with private forks included, and FindForks keeps only
// the forks the reader can access.
func TestSubjectPagePrivateFork(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	_, repo, subjectName := loadArticleRepo(t, 1)
	fork := forkArticle(t, repo)
	fork.IsPrivate = true
	require.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(t.Context(), fork, "is_private"))
	forkName := fork.FullName()

	check := func(t *testing.T, session *TestSession, visible bool) {
		t.Helper()
		rows := 1
		if visible {
			rows = 2
		}
		for _, view := range []string{"table", "bubble"} {
			got, body := getSubjectPage(t, session, subjectName, "view="+view)
			if view == "table" {
				assert.Equal(t, rows, got.rows, "table rows")
			}
			assert.Equal(t, visible, strings.Contains(body, forkName), "the private fork in the %s view's page", view)
		}
		got, body := getSubjectPage(t, session, subjectName, "view=article&selected="+url.QueryEscape(forkName))
		// a reader who may not see it gets the subject's only other article instead
		assert.Equal(t, visible, got.owner == "user4", "selected= the private fork")
		assert.Equal(t, visible, strings.Contains(body, forkName), "the private fork in the Article view's page")
	}

	t.Run("Owner", func(t *testing.T) { check(t, loginUser(t, "user4"), true) })
	t.Run("Anonymous", func(t *testing.T) { check(t, nil, false) })
	t.Run("OtherUser", func(t *testing.T) { check(t, loginUser(t, "user5"), false) })
}
