// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"testing"

	issues_model "code.gitea.io/gitea/models/issues"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/gitrepo"
	"code.gitea.io/gitea/modules/test"
	"code.gitea.io/gitea/modules/timeutil"
	"code.gitea.io/gitea/tests"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestArticleSummaryEdit covers #475: the article's Edit tab submits the summary with the
// article, it is required and up to 200 characters, and it is saved on the article that was
// edited (the article itself or the editor's fork), or proposed in a change request.
func TestArticleSummaryEdit(t *testing.T) {
	onGiteaRun(t, func(t *testing.T, _ *url.URL) {
		owner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
		repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
		editURL := path.Join(owner.Name, repo.Name, "_edit", repo.DefaultBranch, "README.md")

		// the form of the article's Edit tab
		articleForm := func(t *testing.T, session *TestSession, query string, values map[string]string) map[string]string {
			resp := session.MakeRequest(t, NewRequest(t, "GET", editURL+query), http.StatusOK)
			doc := NewHTMLParser(t, resp.Body)
			form := map[string]string{
				"_csrf":               doc.GetCSRF(),
				"last_commit":         doc.GetInputValueByName("last_commit"),
				"tree_path":           "README.md",
				"commit_choice":       "direct",
				"redirect_to_article": "true",
			}
			for k, v := range values {
				form[k] = v
			}
			return form
		}
		headCommit := func(t *testing.T, r *repo_model.Repository) string {
			id, err := gitrepo.GetBranchCommitID(t.Context(), r, r.DefaultBranch)
			require.NoError(t, err)
			return id
		}
		description := func(id int64) string {
			return unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: id}).Description
		}

		session := loginUser(t, owner.Name)

		t.Run("RequiredAndAtMost200Characters", func(t *testing.T) {
			before := description(repo.ID)
			for _, c := range []struct{ name, summary, message string }{
				{"missing", "", "Write a summary"},
				{"blank", " \n\t ", "Write a summary"},
				{"too long", strings.Repeat("a", 201), "up to 200 characters"},
			} {
				values := map[string]string{"content": "# Edited\n"}
				if c.name != "missing" {
					values["summary"] = c.summary
				}
				resp := session.MakeRequest(t, NewRequestWithValues(t, "POST", editURL, articleForm(t, session, "", values)), http.StatusBadRequest)
				assert.Contains(t, resp.Body.String(), c.message, c.name)
			}
			assert.Equal(t, before, description(repo.ID))
		})

		t.Run("OwnerSavesSummaryWithArticle", func(t *testing.T) {
			before := headCommit(t, repo)
			form := articleForm(t, session, "", map[string]string{"content": "# Edited with its summary\n", "summary": "  A new\nsummary  "})
			resp := session.MakeRequest(t, NewRequestWithValues(t, "POST", editURL, form), http.StatusOK)
			assert.Equal(t, "/subject/example-subject/user2", test.RedirectURL(resp))
			assert.Equal(t, "A new summary", description(repo.ID))
			assert.NotEqual(t, before, headCommit(t, repo))
		})

		t.Run("SummaryAloneAddsNoCommit", func(t *testing.T) {
			resp := session.MakeRequest(t, NewRequest(t, "GET", "/"+owner.Name+"/"+repo.Name+"/raw/branch/"+repo.DefaultBranch+"/README.md"), http.StatusOK)
			before := headCommit(t, repo)
			form := articleForm(t, session, "", map[string]string{"content": resp.Body.String(), "summary": "Only the summary changed"})
			session.MakeRequest(t, NewRequestWithValues(t, "POST", editURL, form), http.StatusOK)
			assert.Equal(t, "Only the summary changed", description(repo.ID))
			assert.Equal(t, before, headCommit(t, repo))
		})

		t.Run("OtherEditorsNeedNoSummary", func(t *testing.T) {
			// the stock file editor neither sends a summary nor redirects to the article
			before := description(repo.ID)
			form := articleForm(t, session, "", map[string]string{"content": "# Edited elsewhere\n"})
			delete(form, "redirect_to_article")
			session.MakeRequest(t, NewRequestWithValues(t, "POST", editURL, form), http.StatusOK)
			assert.Equal(t, before, description(repo.ID))
		})

		t.Run("CommitToAnotherBranchLeavesSummary", func(t *testing.T) {
			// the article's summary goes with its default branch: a commit to a new branch
			// (waiting for review) does not change it
			before := description(repo.ID)
			form := articleForm(t, session, "", map[string]string{
				"content":         "# On a branch\n",
				"summary":         "A summary for a branch",
				"commit_choice":   "commit-to-new-branch",
				"new_branch_name": "summary-branch",
			})
			session.MakeRequest(t, NewRequestWithValues(t, "POST", editURL, form), http.StatusOK)
			assert.Equal(t, before, description(repo.ID))
		})

		t.Run("UntouchedMultilineSummaryIsUnchanged", func(t *testing.T) {
			// a description saved elsewhere with line breaks is the same summary once its blanks
			// are collapsed, as the field submits it
			r := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: repo.ID})
			r.Description = "  Line one\n  line two "
			require.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(t.Context(), r, "description"))

			contributor := loginUser(t, "user4")
			form := articleForm(t, contributor, "?submit_change_request=true", map[string]string{
				"content":                    "# Proposed, summary untouched\n",
				"summary":                    "Line one line two",
				"submit_change_request":      "true",
				"change_request_title":       "Content only",
				"change_request_description": "Just the text",
			})
			resp := contributor.MakeRequest(t, NewRequestWithValues(t, "POST", editURL+"?submit_change_request=true", form), http.StatusOK)
			pr := pullFromRedirect(t, repo.ID, test.RedirectURL(resp))
			assert.Equal(t, "Just the text", pr.Issue.Content)

			// nor does the owner's save rewrite it
			form = articleForm(t, session, "", map[string]string{"content": "# Saved again\n", "summary": "Line one line two"})
			session.MakeRequest(t, NewRequestWithValues(t, "POST", editURL, form), http.StatusOK)
			assert.Equal(t, "  Line one\n  line two ", description(repo.ID))
		})

		t.Run("ChangeRequestProposesSummary", func(t *testing.T) {
			contributor := loginUser(t, "user4")
			form := articleForm(t, contributor, "?submit_change_request=true", map[string]string{
				"content":                    "# Proposed\n",
				"summary":                    "A proposed summary",
				"submit_change_request":      "true",
				"change_request_title":       "Better summary",
				"change_request_description": "Please",
			})
			before := description(repo.ID)
			resp := contributor.MakeRequest(t, NewRequestWithValues(t, "POST", editURL+"?submit_change_request=true", form), http.StatusOK)
			pr := pullFromRedirect(t, repo.ID, test.RedirectURL(resp))
			assert.Equal(t, "Please\n\n**Proposed summary:**\n\n> A proposed summary", pr.Issue.Content)
			// the article's own summary is the owner's to change
			assert.Equal(t, before, description(repo.ID))
		})

		t.Run("ForkAndEditSavesSummaryOnFork", func(t *testing.T) {
			contributor := loginUser(t, "user4")
			before := description(repo.ID)
			form := articleForm(t, contributor, "", map[string]string{
				"content":       "# In my fork\n",
				"summary":       "My fork's summary",
				"fork_and_edit": "true",
			})
			contributor.MakeRequest(t, NewRequestWithValues(t, "POST", editURL, form), http.StatusOK)
			fork := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{OwnerID: 4, ForkID: repo.ID})
			assert.Equal(t, "My fork's summary", fork.Description)
			assert.Equal(t, before, description(repo.ID))
		})
	})
}

// pullFromRedirect loads the change request a submit redirected to, with its issue.
func pullFromRedirect(t *testing.T, repoID int64, redirect string) *issues_model.PullRequest {
	require.Contains(t, redirect, "/pulls/")
	index, err := strconv.ParseInt(strings.TrimSuffix(redirect[strings.LastIndex(redirect, "/")+1:], "/"), 10, 64)
	require.NoError(t, err)
	pr, err := issues_model.GetPullRequestByIndex(t.Context(), repoID, index)
	require.NoError(t, err)
	require.NoError(t, pr.LoadIssue(t.Context()))
	return pr
}

// TestArticleSummaryField covers the Summary field of #475 on the Edit tab and in the
// first-article editor: an editable, required textarea with "(required)" in its label.
func TestArticleSummaryField(t *testing.T) {
	defer tests.PrepareTestEnv(t)()
	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	repo.Description = "The summary of this article"
	require.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(t.Context(), repo, "description"))

	session := loginUser(t, "user2")
	resp := session.MakeRequest(t, NewRequest(t, "GET", "/user2/repo1?mode=edit"), http.StatusOK)
	doc := NewHTMLParser(t, resp.Body)
	field := doc.Find(`.article-summary-field textarea#article-summary-input[name="summary"]`)
	require.Equal(t, 1, field.Length())
	assert.Equal(t, "The summary of this article", field.Text())
	assert.Equal(t, "article-edit-form", field.AttrOr("form", ""))
	_, required := field.Attr("required")
	assert.True(t, required)
	assert.Equal(t, "200", field.AttrOr("maxlength", ""))
	label := strings.Join(strings.Fields(doc.Find(".article-summary-field .article-summary-label").Text()), " ")
	assert.Equal(t, "Summary (required)", label)
	assert.NotContains(t, label, "*")

	// the first version of an article is written with its summary, in the editor's own form
	resp = session.MakeRequest(t, NewRequest(t, "GET", "/repo/create-first-article?subject="+url.QueryEscape("Summary first")), http.StatusSeeOther)
	editor := test.RedirectURL(resp)
	require.Contains(t, editor, "/_new/")
	resp = session.MakeRequest(t, NewRequest(t, "GET", editor), http.StatusOK)
	doc = NewHTMLParser(t, resp.Body)
	field = doc.Find(`#first-article-form .first-article-summary textarea#article-summary-input[name="summary"]`)
	require.Equal(t, 1, field.Length())
	assert.Empty(t, field.Text())
	_, required = field.Attr("required")
	assert.True(t, required)
}

// TestExploreSubjectRows covers #248: a subject row shows the summary of the subject's
// original article and when its articles were last updated.
func TestExploreSubjectRows(t *testing.T) {
	defer tests.PrepareTestEnv(t)()
	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1}) // example-subject
	repo.Description = "The original article's summary"
	repo.UpdatedUnix = timeutil.TimeStampNow() // the fixture has no update time
	require.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(t.Context(), repo, "description", "updated_unix"))

	resp := MakeRequest(t, NewRequest(t, "GET", "/explore/subjects?q=example-subject"), http.StatusOK)
	doc := NewHTMLParser(t, resp.Body)
	row := doc.Find(`.flex-list [data-role="subject-row"]`).First()
	require.Equal(t, 1, row.Length())
	assert.Equal(t, "example-subject", strings.TrimSpace(row.Find(".flex-item-title a").Text()))
	assert.Equal(t, "The original article's summary", strings.TrimSpace(row.Find(`[data-role="subject-summary"]`).Text()))
	updated := row.Find(`[data-role="subject-updated"]`)
	assert.True(t, strings.HasPrefix(strings.TrimSpace(updated.Text()), "Updated"))
	assert.Equal(t, 1, updated.Find("relative-time").Length())

	// without a search, too
	resp = MakeRequest(t, NewRequest(t, "GET", "/explore/subjects"), http.StatusOK)
	assert.Contains(t, resp.Body.String(), "The original article&#39;s summary")

	// no exact match: the small secondary button offers to create the subject (#248 item 3)
	resp = MakeRequest(t, NewRequest(t, "GET", "/explore/subjects?q=example"), http.StatusOK)
	doc = NewHTMLParser(t, resp.Body)
	create := doc.Find(`a[data-role="create-new-subject"]`)
	require.Equal(t, 1, create.Length())
	assert.True(t, create.HasClass("forkana-btn-secondary") && create.HasClass("forkana-btn-sm"))
	assert.Equal(t, "Create a new subject", strings.TrimSpace(create.Text()))
}
