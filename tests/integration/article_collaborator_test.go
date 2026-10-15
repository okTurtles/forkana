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
	"testing"

	issues_model "code.gitea.io/gitea/models/issues"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/test"

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

		// a plain maintainer has no settings tab
		AssertHTMLElement(t, articlePage(t, maintainerSession, "read"), `[data-article-tab="settings"]`, false)
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
		post(t, ownerSession, "remove", map[string]string{"uid": strconv.FormatInt(maintainer.ID, 10)})
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
