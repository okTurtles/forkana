// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"code.gitea.io/gitea/tests"

	"github.com/stretchr/testify/assert"
)

// The subject page's Article view renders the article it is asked for, on the server, so
// the reader never sees another article first (#405), and the Bubble view gets the fork
// graph the Table view was built from, so it does not request it again.
func TestSubjectPageArticleViewSelection(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	owner, repo, subjectName := loadArticleRepo(t, 1)
	subjectURL := "/subject/" + url.PathEscape(subjectName)

	type rendered struct {
		owner     string
		chosen    bool
		hasReader bool
		rows      int
	}
	get := func(t *testing.T, query string) rendered {
		t.Helper()
		resp := MakeRequest(t, NewRequest(t, "GET", subjectURL+"?"+query), http.StatusOK)
		assert.Contains(t, resp.Body.String(), "subjectForkGraph", "the fork graph is embedded for the Bubble view")
		doc := NewHTMLParser(t, resp.Body)
		app := doc.Find("#repo-history-app")
		initialOwner, _ := app.Attr("data-initial-owner")
		chosen, _ := app.Attr("data-initial-article")
		return rendered{
			owner:     initialOwner,
			chosen:    chosen == "true",
			hasReader: doc.Find("#article-view-root").Length() == 1,
			rows:      doc.Find("#articles-table tr.article-row").Length(),
		}
	}

	t.Run("SingleArticleIsChosen", func(t *testing.T) {
		got := get(t, "view=article")
		assert.Equal(t, rendered{owner: owner.Name, chosen: true, hasReader: true, rows: 1}, got)
	})

	forkArticle(t, repo)
	fork := "user4/user4-fork-of-repo1"

	t.Run("NoSelectionRendersNoArticle", func(t *testing.T) {
		got := get(t, "view=article")
		assert.False(t, got.chosen)
		assert.False(t, got.hasReader, "the main article must not be rendered when none is chosen")
		assert.Equal(t, 2, got.rows)
	})

	t.Run("SelectedForkIsRenderedDirectly", func(t *testing.T) {
		got := get(t, "view=article&selected="+url.QueryEscape(fork))
		assert.Equal(t, rendered{owner: "user4", chosen: true, hasReader: true, rows: 2}, got)
	})

	t.Run("SelectedMainArticle", func(t *testing.T) {
		got := get(t, "view=article&selected="+url.QueryEscape(fmt.Sprintf("%s/%s", owner.Name, repo.Name)))
		assert.Equal(t, rendered{owner: owner.Name, chosen: true, hasReader: true, rows: 2}, got)
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
