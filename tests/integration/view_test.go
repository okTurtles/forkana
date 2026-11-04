// Copyright 2020 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/tests"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderFileSVGIsInImgTag(t *testing.T) {
	t.Skip("Skipping for Forkana - needs investigation")
	defer tests.PrepareTestEnv(t)()

	session := loginUser(t, "user2")

	req := NewRequest(t, "GET", "/user2/repo2/src/branch/master/line.svg")
	resp := session.MakeRequest(t, req, http.StatusOK)

	doc := NewHTMLParser(t, resp.Body)
	src, exists := doc.doc.Find(".file-view img").Attr("src")
	assert.True(t, exists, "The SVG image should be in an <img> tag so that scripts in the SVG are not run")
	assert.Equal(t, "/user2/repo2/raw/branch/master/line.svg", src)
}

func TestRenderFileMarkupErrorFallsBackToSource(t *testing.T) {
	onGiteaRun(t, func(t *testing.T, _ *url.URL) {
		user := unittest.AssertExistsAndLoadBean(t, &user_model.User{Name: "user2"})
		repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{OwnerName: "user2", Name: "repo1"})

		// html.Parse rejects input whose open element stack exceeds 512 nodes, so the markdown
		// renderer fails: the file view must fall back to the source instead of a server error
		require.NoError(t, createOrReplaceFileInBranch(user, repo, "deeply-nested.md", repo.DefaultBranch, strings.Repeat("<div>", 600)))

		session := loginUser(t, "user2")
		req := NewRequest(t, "GET", "/user2/repo1/src/branch/master/deeply-nested.md")
		session.MakeRequest(t, req, http.StatusOK)
	})
}

func TestCommitListActions(t *testing.T) {
	defer tests.PrepareTestEnv(t)()
	session := loginUser(t, "user2")

	t.Run("WikiRevisionList", func(t *testing.T) {
		defer tests.PrintCurrentTest(t)()

		req := NewRequest(t, "GET", "/user2/repo1/wiki/Home?action=_revision")
		resp := session.MakeRequest(t, req, http.StatusOK)
		htmlDoc := NewHTMLParser(t, resp.Body)
		AssertHTMLElement(t, htmlDoc, ".commit-list .copy-commit-id", true)
		AssertHTMLElement(t, htmlDoc, `.commit-list .view-single-diff`, false)
		AssertHTMLElement(t, htmlDoc, `.commit-list .view-commit-path`, false)
	})

	t.Run("RepoCommitList", func(t *testing.T) {
		defer tests.PrintCurrentTest(t)()

		req := NewRequest(t, "GET", "/user2/repo1/commits/branch/master")
		resp := session.MakeRequest(t, req, http.StatusOK)
		htmlDoc := NewHTMLParser(t, resp.Body)

		AssertHTMLElement(t, htmlDoc, `.commit-list .copy-commit-id`, true)
		AssertHTMLElement(t, htmlDoc, `.commit-list .view-single-diff`, false)
		AssertHTMLElement(t, htmlDoc, `.commit-list .view-commit-path`, true)
	})

	t.Run("RepoFileHistory", func(t *testing.T) {
		defer tests.PrintCurrentTest(t)()

		req := NewRequest(t, "GET", "/user2/repo1/commits/branch/master/README.md")
		resp := session.MakeRequest(t, req, http.StatusOK)
		htmlDoc := NewHTMLParser(t, resp.Body)

		AssertHTMLElement(t, htmlDoc, `.commit-list .copy-commit-id`, true)
		AssertHTMLElement(t, htmlDoc, `.commit-list .view-single-diff`, true)
		AssertHTMLElement(t, htmlDoc, `.commit-list .view-commit-path`, true)
	})
}
