// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"code.gitea.io/gitea/models/db"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/gitrepo"
	"code.gitea.io/gitea/modules/setting"
	repo_service "code.gitea.io/gitea/services/repository"
	files_service "code.gitea.io/gitea/services/repository/files"
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
	// the Bubble view draws the embedded graph; a page opened on the Article view does
	// not embed it (the Bubble view is not on screen there)
	if strings.Contains(query, "view=article") {
		assert.NotContains(t, body, "subjectForkGraph", "no fork graph is embedded on the Article view")
	} else {
		assert.Contains(t, body, "subjectForkGraph", "the fork graph is embedded for the Bubble view")
	}
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

	// the legacy history route names its article in the path: that one is rendered
	t.Run("LegacyHistoryRouteRendersItsArticle", func(t *testing.T) {
		resp := MakeRequest(t, NewRequest(t, "GET", "/explore/articles/history/"+fork+"?view=article"), http.StatusOK)
		app := NewHTMLParser(t, resp.Body).Find("#repo-history-app")
		chosen, _ := app.Attr("data-initial-article")
		initialOwner, _ := app.Attr("data-initial-owner")
		assert.Equal(t, "true", chosen)
		assert.Equal(t, "user4", initialOwner)
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

// The subject root is found by subject, not through the access-checked fork listing: a
// root made private after it was forked must not reach the readers who may not read it,
// neither in the table nor in the embedded graph. It stays in the graph as a nameless
// node, so its forks stay connected.
func TestSubjectPagePrivateRoot(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	owner, repo, subjectName := loadArticleRepo(t, 1)
	forkArticle(t, repo)
	repo.IsPrivate = true
	require.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(t.Context(), repo, "is_private"))
	rootJSON := `"full_name":"` + repo.FullName() + `"`

	check := func(t *testing.T, session *TestSession, visible bool) {
		t.Helper()
		got, body := getSubjectPage(t, session, subjectName, "view=table")
		rows := 1
		if visible {
			rows = 2
		}
		assert.Equal(t, rows, got.rows, "table rows")
		assert.Equal(t, visible, strings.Contains(body, rootJSON), "the private root in the embedded graph")
		assert.Equal(t, !visible, strings.Contains(body, `"id":"hidden_root"`), "the root's place in the graph")
		assert.Contains(t, body, "user4-fork-of-repo1", "its fork stays in the graph")
	}

	t.Run("Owner", func(t *testing.T) { check(t, loginUser(t, owner.Name), true) })
	t.Run("Anonymous", func(t *testing.T) { check(t, nil, false) })
	t.Run("OtherUser", func(t *testing.T) { check(t, loginUser(t, "user5"), false) })
}

// Follow is offered on every subject, to every reader (#421 item 5): it used to be
// hidden from the owner of the article shown, so a signed-in reader lost it on every
// subject they had created.
func TestSubjectPageFollowButton(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	owner, _, subjectName := loadArticleRepo(t, 1)
	for _, reader := range []string{"", owner.Name, "user4"} {
		var session *TestSession
		if reader != "" {
			session = loginUser(t, reader)
		}
		for _, view := range []string{"bubble", "table", "article"} {
			_, body := getSubjectPage(t, session, subjectName, "view="+view)
			doc := NewHTMLParser(t, bytes.NewBufferString(body))
			assert.Equal(t, 1, doc.Find(".repo-header-follow .follow-article-button").Length(), "Follow for %q on the %s view", reader, view)
		}
	}
}

// A brand-new subject: its first article was just created and is still empty (the
// "create first article" flow makes an empty repository and opens the editor). Every
// page of it must render as a whole, not with a server error page appended to it.
func TestSubjectPageBrandNewSubject(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	subjectName := "brand-new-subject-probe"
	repo, err := repo_service.CreateRepository(t.Context(), user2, user2, repo_service.CreateRepoOptions{
		Name:          subjectName,
		Subject:       subjectName,
		DefaultBranch: setting.Repository.DefaultBranch,
		AutoInit:      false,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		// t.Context() is already cancelled when cleanups run
		assert.NoError(t, repo_service.DeleteRepositoryDirectly(context.Background(), repo.ID))
	})

	paths := []string{
		"/user2/" + repo.Name,
		"/subject/" + url.PathEscape(subjectName),
		"/subject/" + url.PathEscape(subjectName) + "?view=table",
		"/subject/" + url.PathEscape(subjectName) + "?view=article",
		"/subject/" + url.PathEscape(subjectName) + "/user2",
	}
	for _, session := range []*TestSession{nil, loginUser(t, "user2"), loginUser(t, "user4")} {
		for _, path := range paths {
			req := NewRequest(t, "GET", path)
			var body string
			if session != nil {
				body = session.MakeRequest(t, req, http.StatusOK).Body.String()
			} else {
				body = MakeRequest(t, req, http.StatusOK).Body.String()
			}
			assert.NotContains(t, body, "Internal Server Error", path)
			assert.Equal(t, 1, strings.Count(body, "<title>"), "%s renders one page", path)
		}
	}
}

// The branch table is updated asynchronously after a push (and not at all for commits
// written straight into a repository), so its head can be behind the repository's. The
// counts must still come from the real branch tip: counted on a stale head (a fork's
// fork point), a fork with contributors of its own showed 0 in every bubble and row.
func TestSubjectPageCountsFromTheBranchNotAStaleHead(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	_, repo, subjectName := loadArticleRepo(t, 1)
	fork := forkArticle(t, repo)
	user4 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})

	gitRepo, err := gitrepo.OpenRepository(t.Context(), fork)
	require.NoError(t, err)
	forkPoint, err := gitRepo.GetBranchCommitID(fork.DefaultBranch)
	gitRepo.Close()
	require.NoError(t, err)

	// user4 edits the fork after it was created
	later := time.Now().Add(time.Hour)
	_, err = files_service.ChangeRepoFiles(t.Context(), fork, user4, &files_service.ChangeRepoFilesOptions{
		Files: []*files_service.ChangeRepoFile{{
			Operation:     "create",
			TreePath:      "notes.md",
			ContentReader: strings.NewReader("user4's notes"),
		}},
		Message:   "notes",
		OldBranch: fork.DefaultBranch,
		Author:    &files_service.IdentityOptions{GitUserName: user4.Name, GitUserEmail: user4.Email},
		Committer: &files_service.IdentityOptions{GitUserName: user4.Name, GitUserEmail: user4.Email},
		Dates:     &files_service.CommitDateOptions{Author: later, Committer: later},
		// no hooks: like a commit written straight into the repository, the branch
		// table does not learn about it (and no server is running to run them)
		InternalPush: true,
	})
	require.NoError(t, err)

	// ...but the branch table still has the fork point as the head
	_, err = db.GetEngine(t.Context()).Exec("UPDATE branch SET commit_id = ? WHERE repo_id = ? AND name = ?", forkPoint, fork.ID, fork.DefaultBranch)
	require.NoError(t, err)

	want, err := repo_service.ArticleContributorCount(t.Context(), fork)
	require.NoError(t, err)
	require.Equal(t, int64(1), want, "user4 is the fork's one contributor since it was created")

	resp := MakeRequest(t, NewRequest(t, "GET", "/subject/"+url.PathEscape(subjectName)+"?view=table"), http.StatusOK)
	row := NewHTMLParser(t, resp.Body).Find(`#articles-table tr.article-row[data-owner="user4"]`)
	require.Equal(t, 1, row.Length())
	assert.Equal(t, strconv.FormatInt(want, 10), strings.TrimSpace(row.Find("td.tw-font-semibold").First().Text()))
}
