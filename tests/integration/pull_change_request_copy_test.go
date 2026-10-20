// Copyright 2025 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	issues_model "code.gitea.io/gitea/models/issues"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	repo_service "code.gitea.io/gitea/services/repository"
	files_service "code.gitea.io/gitea/services/repository/files"
	"code.gitea.io/gitea/tests"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPullViewChangeRequestTitleDesc checks that the change request header line is viewer-aware:
// the change author reads it in the first person, the article owner keeps the possessive form,
// and any other viewer gets the neutral form.
func TestPullViewChangeRequestTitleDesc(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	// user1 is the poster of /user2/repo1/pulls/3, user2 owns the article, user4 is neither.
	cases := []struct {
		user     string
		contains string
		absent   []string
	}{
		{user: "user1", contains: "You want to make", absent: []string{"wants to make"}},
		{user: "user2", contains: "changes to your article", absent: []string{"You want to make", "changes to this article"}},
		{user: "user4", contains: "changes to this article", absent: []string{"You want to make", "changes to your article"}},
	}

	for _, c := range cases {
		t.Run(c.user, func(t *testing.T) {
			session := loginUser(t, c.user)
			req := NewRequest(t, "GET", "/user2/repo1/pulls/3")
			resp := session.MakeRequest(t, req, http.StatusOK)
			// Scope the assertions to the header element so unrelated page copy cannot affect them.
			htmlDoc := NewHTMLParser(t, resp.Body)
			desc := htmlDoc.Find("#pull-desc-display").Text()
			assert.Contains(t, desc, c.contains)
			for _, absent := range c.absent {
				assert.NotContains(t, desc, absent)
			}
			// #326: the line no longer names the article, the title above links to it instead
			assert.NotContains(t, desc, "user2/repo1")
			assert.Equal(t, 0, htmlDoc.Find("#pull-desc-display .article-ref-pill").Length())
			title := htmlDoc.Find(".pr-repo-title a#pr-article-link")
			assert.Equal(t, "user2 / example-subject", strings.TrimSpace(title.Text()))
			assert.Equal(t, "/subject/example-subject/user2", title.AttrOr("href", ""))
		})
	}
}

// TestPullViewChangeRequestTitleDescPlural checks that a single-change request is not described
// as "1 changes".
func TestPullViewChangeRequestTitleDescPlural(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	// /user2/repo1/pulls/5 carries a single commit; user2 owns the article and user1 posted it.
	cases := []struct {
		user     string
		contains string
	}{
		{user: "user2", contains: "1 change to your article"},
		{user: "user1", contains: "You want to make 1 change to this article"},
		{user: "user4", contains: "1 change to this article"},
	}

	for _, c := range cases {
		t.Run(c.user, func(t *testing.T) {
			session := loginUser(t, c.user)
			req := NewRequest(t, "GET", "/user2/repo1/pulls/5")
			resp := session.MakeRequest(t, req, http.StatusOK)
			desc := strings.Join(strings.Fields(NewHTMLParser(t, resp.Body).Find("#pull-desc-display").Text()), " ")
			assert.Contains(t, desc, c.contains)
			assert.NotContains(t, desc, "1 changes")
		})
	}
}

// TestPullViewChangeRequestForkOf checks the "Fork of: <owner> / <subject>" line under the change
// request title (#326): it names the article the target article was forked from, and links to it.
// It describes the target article, never the head repository of the change request.
func TestPullViewChangeRequestForkOf(t *testing.T) {
	// a running server is needed for the git hooks of the commit pushed to the fork
	onGiteaRun(t, func(t *testing.T, _ *url.URL) {
		testPullViewChangeRequestForkOf(t)
	})
}

func testPullViewChangeRequestForkOf(t *testing.T) {
	t.Run("target article is a fork", func(t *testing.T) {
		repo1 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
		user4 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
		fork, err := repo_service.ForkRepository(t.Context(), user4, user4, repo_service.ForkRepoOptions{
			BaseRepo:     repo1,
			Name:         "fork-of-repo1",
			SingleBranch: repo1.DefaultBranch,
		})
		require.NoError(t, err)
		defer func() {
			_ = repo_service.DeleteRepositoryDirectly(t.Context(), fork.ID)
		}()
		_, err = files_service.ChangeRepoFiles(t.Context(), fork, user4, &files_service.ChangeRepoFilesOptions{
			NewBranch: "fork-of-change",
			Files: []*files_service.ChangeRepoFile{
				{Operation: "update", TreePath: "README.md", ContentReader: strings.NewReader("# Changed in the fork\n")},
			},
		})
		require.NoError(t, err)

		session := loginUser(t, "user4")
		testPullCreateDirectly(t, session, "user4", fork.Name, fork.DefaultBranch, "", "", "fork-of-change", "Change in a forked article")
		pr := unittest.AssertExistsAndLoadBean(t, &issues_model.PullRequest{BaseRepoID: fork.ID, HeadBranch: "fork-of-change"})

		for _, user := range []string{"user4", "user2"} {
			resp := loginUser(t, user).MakeRequest(t, NewRequestf(t, "GET", "/user4/%s/pulls/%d", fork.Name, pr.Index), http.StatusOK)
			htmlDoc := NewHTMLParser(t, resp.Body)
			title := htmlDoc.Find(".pr-repo-title a#pr-article-link")
			assert.Equal(t, "user4 / example-subject", strings.TrimSpace(title.Text()), user)
			forkOf := htmlDoc.Find(".pr-repo-header #pr-article-fork-of a")
			assert.Equal(t, 1, forkOf.Length(), user)
			assert.Equal(t, "Fork of: user2 / example-subject", strings.Join(strings.Fields(forkOf.Text()), " "), user)
			assert.Equal(t, "/subject/example-subject/user2", forkOf.AttrOr("href", ""), user)
		}
	})

	t.Run("target article is not a fork", func(t *testing.T) {
		// /user2/repo1/pulls/3 is a same-repository change request on an original article
		resp := loginUser(t, "user2").MakeRequest(t, NewRequest(t, "GET", "/user2/repo1/pulls/3"), http.StatusOK)
		assert.Equal(t, 0, NewHTMLParser(t, resp.Body).Find(".pr-repo-header .fork-flag").Length())
	})

	t.Run("change request opened from a fork", func(t *testing.T) {
		// /user12/repo10/pulls/1 comes from user13/repo11, a fork of repo10: the head's base is
		// the article itself, which is not a fork, so there is nothing to show
		resp := loginUser(t, "user12").MakeRequest(t, NewRequest(t, "GET", "/user12/repo10/pulls/1"), http.StatusOK)
		assert.Equal(t, 0, NewHTMLParser(t, resp.Body).Find(".pr-repo-header .fork-flag").Length())
	})

	t.Run("head repository is gone", func(t *testing.T) {
		// deleting user13/repo11 leaves /user12/repo10/pulls/1 without a head repository:
		// the title must still lead back to the article
		require.NoError(t, repo_service.DeleteRepositoryDirectly(t.Context(), 11))
		resp := loginUser(t, "user12").MakeRequest(t, NewRequest(t, "GET", "/user12/repo10/pulls/1"), http.StatusOK)
		htmlDoc := NewHTMLParser(t, resp.Body)
		repo10 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 10})
		title := htmlDoc.Find(".pr-repo-header #pr-article-link")
		assert.Equal(t, 1, title.Length())
		assert.Equal(t, repo10.Link(), title.AttrOr("href", ""))
		assert.Equal(t, 1, htmlDoc.Find(".pr-repo-header .pr-back-link").Length())
	})
}

// TestPullViewChangeRequestTitleDescOtherLocale checks that the change request header line renders
// cleanly in a non-English locale. The upstream translations (e.g. de-DE) still define Gitea's
// pulls.title_desc / pulls.merged_title_desc with Gitea's branch arguments, so the fork's copy must
// use keys of its own: a reused key resolves to the upstream translation, which formats the fork's
// arguments into "%!s(MISSING)" / "%!s(BADINDEX)" garbage.
func TestPullViewChangeRequestTitleDescOtherLocale(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	// /user2/repo1/pulls/3 is open (user1 posted it, user2 owns the article, user4 is neither),
	// /user2/repo1/pulls/2 is merged.
	cases := []struct {
		user string
		pull string
	}{
		{user: "user1", pull: "3"},
		{user: "user2", pull: "3"},
		{user: "user4", pull: "3"},
		{user: "user2", pull: "2"},
		{user: "user4", pull: "2"},
	}

	for _, c := range cases {
		t.Run(c.user+"/pulls/"+c.pull, func(t *testing.T) {
			session := loginUser(t, c.user)
			req := NewRequest(t, "GET", "/user2/repo1/pulls/"+c.pull+"?lang=de-DE")
			resp := session.MakeRequest(t, req, http.StatusOK)
			htmlDoc := NewHTMLParser(t, resp.Body)
			assert.Equal(t, "de-DE", htmlDoc.Find("html").AttrOr("lang", ""))
			desc := strings.Join(strings.Fields(htmlDoc.Find(".issue-title-meta .pull-desc").Text()), " ")
			assert.NotEmpty(t, desc)
			assert.NotContains(t, desc, "%!")
		})
	}
}

// TestPullViewMergePermissionHint checks that the upstream "only those with write access ... can
// merge change requests" hint is gone: merging is limited to the article owner and its maintainers
// in Forkana, so that copy contradicted the message shown right below it to every other viewer.
func TestPullViewMergePermissionHint(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	const writeAccessHint = "can merge change requests"

	// user2 owns repo1 and can merge, user40 has write access but is not the owner,
	// user4 has neither.
	cases := []struct {
		user   string
		absent []string
		// contains is the accurate, Forkana-specific message the viewer should get instead.
		contains string
	}{
		{user: "user2", absent: []string{writeAccessHint, "Only the article owner and its maintainers can accept change requests."}},
		{user: "user40", absent: []string{writeAccessHint}, contains: "Only the article owner and its maintainers can accept change requests."},
		{user: "user4", absent: []string{writeAccessHint}, contains: "You are not authorized to merge this change request."},
	}

	for _, c := range cases {
		t.Run(c.user, func(t *testing.T) {
			session := loginUser(t, c.user)
			req := NewRequest(t, "GET", "/user2/repo1/pulls/3")
			resp := session.MakeRequest(t, req, http.StatusOK)
			body := resp.Body.String()
			for _, absent := range c.absent {
				assert.NotContains(t, body, absent)
			}
			if c.contains != "" {
				assert.Contains(t, body, c.contains)
			}
		})
	}
}
