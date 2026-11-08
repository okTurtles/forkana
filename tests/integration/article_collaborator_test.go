// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"strconv"
	"strings"
	"testing"

	issues_model "code.gitea.io/gitea/models/issues"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/test"
	repo_service "code.gitea.io/gitea/services/repository"
	"code.gitea.io/gitea/tests"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArticleCollaborator(t *testing.T) {
	// a running server is needed: the direct commit goes through the pre-receive hook,
	// which calls back into the internal API
	onGiteaRun(t, testArticleCollaborator)
}

func testArticleCollaborator(t *testing.T, _ *url.URL) {
	// user2 owns repo1, an article on subject 1
	owner, repo, subjectName := loadArticleRepo(t, 1)
	delegate := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
	maintainer := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 5})
	outsider := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 8})

	ownerSession := loginUser(t, owner.Name)
	delegateSession := loginUser(t, delegate.Name)
	maintainerSession := loginUser(t, maintainer.Name)
	outsiderSession := loginUser(t, outsider.Name)

	collaboratorsURL := fmt.Sprintf("/%s/%s/article_collaborators", owner.Name, repo.Name)
	articleURL := fmt.Sprintf("/subject/%s/%s?view=article", subjectName, owner.Name)

	articlePage := func(t *testing.T, session *TestSession, mode string) *HTMLDoc {
		t.Helper()
		resp := session.MakeRequest(t, NewRequest(t, "GET", articleURL+"&mode="+mode), http.StatusOK)
		return NewHTMLParser(t, resp.Body)
	}

	post := func(t *testing.T, session *TestSession, action string, values map[string]string) *httptest.ResponseRecorder {
		t.Helper()
		values["_csrf"] = GetUserCSRFToken(t, session)
		return session.MakeRequest(t, NewRequestWithValues(t, "POST", collaboratorsURL+"/"+action, values), http.StatusSeeOther)
	}

	invite := func(t *testing.T, session *TestSession, invitee *user_model.User, role repo_model.ArticleCollaboratorRole) {
		t.Helper()
		resp := post(t, session, "invite", map[string]string{"invitee_name": invitee.Name, "role": strconv.Itoa(int(role))})
		assert.Contains(t, test.RedirectURL(resp), "section=collaborators")
	}

	t.Run("EmptyState", func(t *testing.T) {
		htmlDoc := articlePage(t, ownerSession, "settings")
		AssertHTMLElement(t, htmlDoc, "#article-collaborators-empty", true)
		AssertHTMLElement(t, htmlDoc, "#article-collaborator-invite-modal", true)
	})

	t.Run("PendingInviteKeepsChangeRequestFlow", func(t *testing.T) {
		invite(t, ownerSession, delegate, repo_model.ArticleCollaboratorRoleMaintainerDelegate)
		c := unittest.AssertExistsAndLoadBean(t, &repo_model.ArticleCollaborator{RepoID: repo.ID, UserID: delegate.ID})
		assert.True(t, c.IsPending())

		htmlDoc := articlePage(t, delegateSession, "edit")
		AssertHTMLElement(t, htmlDoc, "#article-collaborator-invite-notice", true)
		AssertHTMLElement(t, htmlDoc, "#submit-changes-button", false)
		AssertHTMLElement(t, htmlDoc, "#pre-submit-changes-button", true)

		// the settings stay out of reach until the invitation is accepted
		AssertHTMLElement(t, htmlDoc, `[data-article-tab="settings"]`, false)
	})

	t.Run("AcceptOffersDirectEdit", func(t *testing.T) {
		post(t, delegateSession, "accept", map[string]string{})

		htmlDoc := articlePage(t, delegateSession, "edit")
		AssertHTMLElement(t, htmlDoc, "#article-collaborator-invite-notice", false)
		AssertHTMLElement(t, htmlDoc, "#submit-changes-button", true)
	})

	t.Run("DelegateSeesOnlyCollaborators", func(t *testing.T) {
		htmlDoc := articlePage(t, delegateSession, "settings")
		AssertHTMLElement(t, htmlDoc, "#article-settings-collaborators", true)
		AssertHTMLElement(t, htmlDoc, "#article-settings-general", false)
		AssertHTMLElement(t, htmlDoc, "#article-transfer-modal", false)
		// no General | Collaborators switch, and the page is named after its one section
		AssertHTMLElement(t, htmlDoc, ".forkana-segmented", false)
		AssertHTMLElement(t, htmlDoc, `[data-article-settings-panel="general"]`, false)
		assert.Equal(t, "Collaborators", strings.TrimSpace(htmlDoc.Find(".article-settings-heading").Text()))
		assert.Equal(t, "collaborators", htmlDoc.Find("#article-settings-root").AttrOr("data-section", ""))
	})

	t.Run("DelegateCannotGrantDelegation", func(t *testing.T) {
		invite(t, delegateSession, maintainer, repo_model.ArticleCollaboratorRoleMaintainerDelegate)
		unittest.AssertNotExistsBean(t, &repo_model.ArticleCollaborator{RepoID: repo.ID, UserID: maintainer.ID})
	})

	t.Run("DelegateInvitesMaintainer", func(t *testing.T) {
		invite(t, delegateSession, maintainer, repo_model.ArticleCollaboratorRoleMaintainer)
		post(t, maintainerSession, "accept", map[string]string{})

		c := unittest.AssertExistsAndLoadBean(t, &repo_model.ArticleCollaborator{RepoID: repo.ID, UserID: maintainer.ID})
		assert.True(t, c.IsAccepted())
		assert.Equal(t, repo_model.ArticleCollaboratorRoleMaintainer, c.Role)

		// a plain maintainer has no Settings, in the "…" menu or anywhere else
		AssertHTMLElement(t, articlePage(t, maintainerSession, "read"), `[data-article-tab="settings"]`, false)
	})

	// #477: Settings is no tab any more, the "…" menu holds it, above a divider and Report
	t.Run("SettingsInMoreMenu", func(t *testing.T) {
		htmlDoc := articlePage(t, ownerSession, "read")
		AssertHTMLElement(t, htmlDoc, `#article-tabs .tabular.menu [data-article-tab="settings"]`, false)
		AssertHTMLElement(t, htmlDoc, `[data-role="article-more"] .menu > a.article-more-settings[data-article-tab="settings"]`, true)
		AssertHTMLElement(t, htmlDoc, `[data-role="article-more"] .menu > .article-more-divider`, true)
		AssertHTMLElement(t, htmlDoc, `[data-role="article-more"] .menu > .article-more-report`, true)

		assert.Equal(t, "Settings", strings.TrimSpace(htmlDoc.Find(".article-more-settings").Text()))

		// the delegate manages the collaborators: their item is "Collaborators", leading to
		// the same page, on that section
		delegateItem := articlePage(t, delegateSession, "read").Find(`[data-role="article-more"] .menu > a.article-more-settings[data-article-tab="settings"]`)
		require.Equal(t, 1, delegateItem.Length())
		assert.Equal(t, "Collaborators", strings.TrimSpace(delegateItem.Text()))

		// everyone else sees Report only, and asking for the settings shows the article
		htmlDoc = articlePage(t, outsiderSession, "settings")
		AssertHTMLElement(t, htmlDoc, `.article-more-settings`, false)
		AssertHTMLElement(t, htmlDoc, `.article-more-divider`, false)
		AssertHTMLElement(t, htmlDoc, `.article-more-report`, true)
		AssertHTMLElement(t, htmlDoc, "#article-settings-root", false)
		assert.Equal(t, "read", htmlDoc.Find("#article-view-root").AttrOr("data-article-mode", ""))
	})

	// #461: the owner first, then a line counting who can edit the article
	t.Run("ListShowsOwnerAndCounts", func(t *testing.T) {
		htmlDoc := articlePage(t, ownerSession, "settings")
		// the settings page has no row of tabs: "Back" leads to the Read tab
		AssertHTMLElement(t, htmlDoc, "#article-tabs", false)
		AssertHTMLElement(t, htmlDoc, `.article-settings-back[data-article-tab="read"]`, true)
		AssertHTMLElement(t, htmlDoc, `[data-article-settings-section="general"].active`, true)

		ownerRow := htmlDoc.Find("[data-article-collaborator-owner]")
		require.Equal(t, 1, ownerRow.Length())
		assert.Contains(t, ownerRow.Text(), owner.DisplayName()+" (you)")
		assert.Contains(t, ownerRow.Text(), "Owner")
		assert.Equal(t, 0, ownerRow.Find("input[type=checkbox]").Length(), "the owner cannot be selected")
		assert.Equal(t, 0, ownerRow.Find("[data-article-collaborator-remove]").Length(), "nor removed")

		assert.Equal(t, "3 people can edit this article and approve change requests: you and 2 collaborators.",
			strings.TrimSpace(htmlDoc.Find(`[data-role="collaborators-summary"]`).Text()))
	})

	t.Run("OwnerChangesRole", func(t *testing.T) {
		changeRole := func(t *testing.T, role repo_model.ArticleCollaboratorRole) map[string]any {
			t.Helper()
			values := map[string]string{
				"_csrf": GetUserCSRFToken(t, ownerSession),
				"uid":   strconv.FormatInt(maintainer.ID, 10),
				"role":  strconv.Itoa(int(role)),
			}
			resp := ownerSession.MakeRequest(t, NewRequestWithValues(t, "POST", collaboratorsURL+"/role", values), http.StatusOK)
			data := map[string]any{}
			DecodeJSON(t, resp, &data)
			return data
		}

		data := changeRole(t, repo_model.ArticleCollaboratorRoleMaintainerDelegate)
		assert.Equal(t, fmt.Sprintf("The role of %s has been updated.", maintainer.DisplayName()), data["message"])
		c := unittest.AssertExistsAndLoadBean(t, &repo_model.ArticleCollaborator{RepoID: repo.ID, UserID: maintainer.ID})
		assert.Equal(t, repo_model.ArticleCollaboratorRoleMaintainerDelegate, c.Role)

		changeRole(t, repo_model.ArticleCollaboratorRoleMaintainer)

		values := map[string]string{"_csrf": GetUserCSRFToken(t, ownerSession), "uid": strconv.FormatInt(maintainer.ID, 10), "role": "99"}
		resp := ownerSession.MakeRequest(t, NewRequestWithValues(t, "POST", collaboratorsURL+"/role", values), http.StatusBadRequest)
		assert.Contains(t, resp.Body.String(), "That role is not valid.")
	})

	t.Run("OutsiderCannotManage", func(t *testing.T) {
		values := map[string]string{"_csrf": GetUserCSRFToken(t, outsiderSession), "invitee_name": outsider.Name, "role": "1"}
		outsiderSession.MakeRequest(t, NewRequestWithValues(t, "POST", collaboratorsURL+"/invite", values), http.StatusNotFound)
	})

	t.Run("CandidatesExcludeBlockedUsers", func(t *testing.T) {
		candidates := func(t *testing.T, q string) string {
			t.Helper()
			candidatesURL := fmt.Sprintf("/subject/%s/%s/settings/collaborator_candidates?q=%s", subjectName, owner.Name, q)
			return ownerSession.MakeRequest(t, NewRequest(t, "GET", candidatesURL), http.StatusOK).Body.String()
		}
		// the owner blocked user29 (user_blocking.yml)
		assert.NotContains(t, candidates(t, "user29"), `"login":"user29"`)
		assert.Contains(t, candidates(t, "user28"), `"login":"user28"`)
	})

	t.Run("InviteWithoutRoleRefused", func(t *testing.T) {
		post(t, ownerSession, "invite", map[string]string{"invitee_name": outsider.Name})
		assert.Equal(t, "That role is not valid.", ownerSession.GetCookieFlashMessage().ErrorMsg)
		unittest.AssertNotExistsBean(t, &repo_model.ArticleCollaborator{RepoID: repo.ID, UserID: outsider.ID})
	})

	t.Run("MergeGate", func(t *testing.T) {
		mergeForm := func(session *TestSession) map[string]string {
			return map[string]string{"_csrf": GetUserCSRFToken(t, session), "do": string(repo_model.MergeStyleMerge)}
		}
		// pull 3 of repo1 is open
		mergeURL := path.Join(owner.Name, repo.Name, "pulls", "3", "merge")

		resp := outsiderSession.MakeRequest(t, NewRequestWithValues(t, "POST", mergeURL, mergeForm(outsiderSession)), NoExpectedStatus)
		assert.Contains(t, resp.Body.String(), "Only the article owner and its maintainers can accept change requests.")

		maintainerSession.MakeRequest(t, NewRequestWithValues(t, "POST", mergeURL, mergeForm(maintainerSession)), http.StatusOK)
		pull := unittest.AssertExistsAndLoadBean(t, &issues_model.PullRequest{BaseRepoID: repo.ID, Index: 3})
		assert.True(t, pull.HasMerged)
	})

	// runs after MergeGate: editing README.md first would make the fixture pull conflict
	t.Run("AcceptedMaintainerCommitsDirectly", func(t *testing.T) {
		pullsBefore := unittest.GetCount(t, &issues_model.PullRequest{BaseRepoID: repo.ID})

		editURL := path.Join(owner.Name, repo.Name, "_edit", repo.DefaultBranch, "README.md")
		resp := delegateSession.MakeRequest(t, NewRequest(t, "GET", editURL), http.StatusOK)
		editDoc := NewHTMLParser(t, resp.Body)
		req := NewRequestWithValues(t, "POST", editURL, map[string]string{
			"_csrf":         editDoc.GetCSRF(),
			"last_commit":   editDoc.GetInputValueByName("last_commit"),
			"tree_path":     "README.md",
			"content":       "# Edited by a maintainer\n",
			"commit_choice": "direct",
		})
		delegateSession.MakeRequest(t, req, http.StatusOK)

		assert.Equal(t, pullsBefore, unittest.GetCount(t, &issues_model.PullRequest{BaseRepoID: repo.ID}),
			"a maintainer's edit must not open a change request")
	})

	t.Run("RemoveRevokesAccess", func(t *testing.T) {
		// the confirmation dialog asks for JSON, so that it can show a refusal in place
		values := map[string]string{"_csrf": GetUserCSRFToken(t, ownerSession), "uid": strconv.FormatInt(maintainer.ID, 10)}
		req := NewRequestWithValues(t, "POST", collaboratorsURL+"/remove", values).SetHeader("Accept", "application/json")
		data := map[string]any{}
		DecodeJSON(t, ownerSession.MakeRequest(t, req, http.StatusOK), &data)
		assert.Contains(t, data["redirect"], "section=collaborators")
		assert.Equal(t, maintainer.DisplayName()+" has been removed from this article.", ownerSession.GetCookieFlashMessage().SuccessMsg)
		unittest.AssertNotExistsBean(t, &repo_model.ArticleCollaborator{RepoID: repo.ID, UserID: maintainer.ID})

		isCollaborator, err := repo_model.IsCollaborator(t.Context(), repo.ID, maintainer.ID)
		require.NoError(t, err)
		assert.False(t, isCollaborator)
		AssertHTMLElement(t, articlePage(t, maintainerSession, "edit"), "#submit-changes-button", false)
	})

	t.Run("GiteaCollaborationPageRefused", func(t *testing.T) {
		values := map[string]string{"_csrf": GetUserCSRFToken(t, ownerSession), "collaborator": outsider.Name}
		collaborationURL := fmt.Sprintf("/%s/%s/settings/collaboration", owner.Name, repo.Name)
		ownerSession.MakeRequest(t, NewRequestWithValues(t, "POST", collaborationURL, values), http.StatusNotFound)

		// the page itself sends the owner to the article settings
		resp := ownerSession.MakeRequest(t, NewRequest(t, "GET", collaborationURL), http.StatusSeeOther)
		assert.Contains(t, test.RedirectURL(resp), "mode=settings&section=collaborators")
	})

	t.Run("ArchivedTakesNoNewCollaborators", func(t *testing.T) {
		// listed before the article is archived
		invite(t, ownerSession, maintainer, repo_model.ArticleCollaboratorRoleMaintainerDelegate)
		require.NoError(t, repo_model.SetArchiveRepoState(t.Context(), repo, true))
		defer func() {
			require.NoError(t, repo_model.SetArchiveRepoState(t.Context(), repo, false))
		}()

		htmlDoc := articlePage(t, ownerSession, "settings")
		AssertHTMLElement(t, htmlDoc, "#article-collaborator-invite-modal", false)
		AssertHTMLElement(t, htmlDoc, `[data-article-settings-modal="#article-collaborator-invite-modal"]`, false)
		AssertHTMLElement(t, htmlDoc, "#article-collaborators-remove-modal", true)
		AssertHTMLElement(t, htmlDoc, ".article-collaborator-role", true)
		AssertHTMLElement(t, htmlDoc, "[data-article-collaborator-remove]", true)

		post(t, ownerSession, "invite", map[string]string{"invitee_name": outsider.Name, "role": "1"})
		unittest.AssertNotExistsBean(t, &repo_model.ArticleCollaborator{RepoID: repo.ID, UserID: outsider.ID})

		values := map[string]string{
			"_csrf": GetUserCSRFToken(t, ownerSession),
			"uid":   strconv.FormatInt(maintainer.ID, 10),
			"role":  strconv.Itoa(int(repo_model.ArticleCollaboratorRoleMaintainer)),
		}
		ownerSession.MakeRequest(t, NewRequestWithValues(t, "POST", collaboratorsURL+"/role", values), http.StatusOK)
		c := unittest.AssertExistsAndLoadBean(t, &repo_model.ArticleCollaborator{RepoID: repo.ID, UserID: maintainer.ID})
		assert.Equal(t, repo_model.ArticleCollaboratorRoleMaintainer, c.Role)

		post(t, ownerSession, "remove", map[string]string{"uid": strconv.FormatInt(maintainer.ID, 10)})
		unittest.AssertNotExistsBean(t, &repo_model.ArticleCollaborator{RepoID: repo.ID, UserID: maintainer.ID})
	})
}

// TestArticleCollaboratorPrivateInvite checks that a user invited to a private article can
// open it and answer the invitation: the write access only comes with accepting, so without
// the read access granted to the invitee the article and its answer routes answered 404.
func TestArticleCollaboratorPrivateInvite(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	owner, repo, subjectName := loadArticleRepo(t, 1)
	invitee := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 5})
	outsider := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 8})

	repo.IsPrivate = true
	require.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(t.Context(), repo, "is_private"))
	require.NoError(t, repo_service.InviteArticleCollaborator(t.Context(), owner, repo, invitee, repo_model.ArticleCollaboratorRoleMaintainer))

	articleURL := fmt.Sprintf("/subject/%s/%s?view=article", subjectName, owner.Name)
	loginUser(t, outsider.Name).MakeRequest(t, NewRequest(t, "GET", articleURL), http.StatusNotFound)

	session := loginUser(t, invitee.Name)
	resp := session.MakeRequest(t, NewRequest(t, "GET", articleURL), http.StatusOK)
	AssertHTMLElement(t, NewHTMLParser(t, resp.Body), "#article-collaborator-invite-notice", true)

	acceptURL := fmt.Sprintf("/%s/%s/article_collaborators/accept", owner.Name, repo.Name)
	session.MakeRequest(t, NewRequestWithValues(t, "POST", acceptURL, map[string]string{"_csrf": GetUserCSRFToken(t, session)}), http.StatusSeeOther)
	c := unittest.AssertExistsAndLoadBean(t, &repo_model.ArticleCollaborator{RepoID: repo.ID, UserID: invitee.ID})
	assert.True(t, c.IsAccepted())
}
