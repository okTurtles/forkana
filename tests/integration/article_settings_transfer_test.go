// Copyright 2025 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"code.gitea.io/gitea/models/db"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	api "code.gitea.io/gitea/modules/structs"
	"code.gitea.io/gitea/modules/test"
	"code.gitea.io/gitea/tests"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// transferForm builds the payload the article transfer modal submits.
func transferForm(csrf, owner, subject, newOwnerFullName string) map[string]string {
	return map[string]string{
		"_csrf":               csrf,
		"action":              "transfer",
		"redirect_to_article": "true",
		"article_name":        owner + "/" + subject,
		"new_owner_name":      newOwnerFullName,
	}
}

func TestArticleSettingsTransfer(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	owner, repo, subjectName := loadArticleRepo(t, 1)
	recipient := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 5})

	session := loginUser(t, owner.Name)
	settingsURL := fmt.Sprintf("/%s/%s/settings", owner.Name, repo.Name)
	articleSettingsURL := fmt.Sprintf("/article/%s/%s?view=article&mode=settings", owner.Name, subjectName)

	post := func(t *testing.T, form map[string]string) {
		t.Helper()
		req := NewRequestWithValues(t, "POST", settingsURL, form)
		resp := session.MakeRequest(t, req, http.StatusSeeOther)
		assert.Equal(t, articleSettingsURL, test.RedirectURL(resp))
	}

	flashText := func(t *testing.T) string {
		t.Helper()
		req := NewRequest(t, "GET", articleSettingsURL)
		resp := session.MakeRequest(t, req, http.StatusOK)
		flash := NewHTMLParser(t, resp.Body).Find(".flash-message")
		require.Equal(t, 1, flash.Length())
		return flash.Text()
	}

	t.Run("WrongArticleName", func(t *testing.T) {
		form := transferForm(GetUserCSRFToken(t, session), owner.Name, subjectName, recipient.FullName)
		// the confirmation is case-sensitive on purpose
		form["article_name"] = owner.Name + "/" + subjectName + "x"
		post(t, form)

		assert.Contains(t, flashText(t), "The article owner and subject you entered are incorrect.")
		unchanged := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: repo.ID})
		assert.Equal(t, repo_model.RepositoryReady, unchanged.Status)
	})

	t.Run("UnknownFullName", func(t *testing.T) {
		post(t, transferForm(GetUserCSRFToken(t, session), owner.Name, subjectName, "Nobody At All"))

		assert.Contains(t, flashText(t), "No user was found with that first and last name.")
		unchanged := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: repo.ID})
		assert.Equal(t, repo_model.RepositoryReady, unchanged.Status)
	})

	t.Run("Start", func(t *testing.T) {
		// the recipient is resolved by first and last name, not by username
		post(t, transferForm(GetUserCSRFToken(t, session), owner.Name, subjectName, recipient.FullName))

		assert.Contains(t, flashText(t), "awaits confirmation from "+recipient.DisplayName())

		pending := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: repo.ID})
		assert.Equal(t, repo_model.RepositoryPendingTransfer, pending.Status)
		unittest.AssertExistsAndLoadBean(t, &repo_model.RepoTransfer{RepoID: repo.ID, RecipientID: recipient.ID})
	})

	t.Run("PendingHidesTransferButton", func(t *testing.T) {
		req := NewRequest(t, "GET", articleSettingsURL)
		resp := session.MakeRequest(t, req, http.StatusOK)
		htmlDoc := NewHTMLParser(t, resp.Body)

		AssertHTMLElement(t, htmlDoc, `[data-article-settings-modal="#article-transfer-modal"]`, false)
		AssertHTMLElement(t, htmlDoc, "#article-transfer-cancel", true)
		assert.Contains(t, htmlDoc.Find("#article-settings-transfer").Text(),
			"awaiting confirmation from "+recipient.DisplayName())
	})

	t.Run("Cancel", func(t *testing.T) {
		post(t, map[string]string{
			"_csrf":               GetUserCSRFToken(t, session),
			"action":              "cancel_transfer",
			"redirect_to_article": "true",
		})

		assert.Contains(t, flashText(t),
			fmt.Sprintf("The article transfer to %s was successfully canceled.", recipient.DisplayName()))

		reverted := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: repo.ID})
		assert.Equal(t, repo_model.RepositoryReady, reverted.Status)
		unittest.AssertNotExistsBean(t, &repo_model.RepoTransfer{RepoID: repo.ID})
	})
}

func TestArticleSettingsTransferCandidates(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	owner, repo, subjectName := loadArticleRepo(t, 1)
	session := loginUser(t, owner.Name)
	candidatesURL := fmt.Sprintf("/article/%s/%s/settings/transfer_candidates", owner.Name, subjectName)

	search := func(t *testing.T, keyword string) []string {
		t.Helper()
		req := NewRequest(t, "GET", candidatesURL+"?q="+url.QueryEscape(keyword))
		resp := session.MakeRequest(t, req, http.StatusOK)
		var body struct {
			Data []*api.User `json:"data"`
		}
		DecodeJSON(t, resp, &body)
		logins := make([]string, 0, len(body.Data))
		for _, u := range body.Data {
			logins = append(logins, u.UserName)
		}
		return logins
	}

	t.Run("MatchesFullName", func(t *testing.T) {
		recipient := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 5})
		assert.Contains(t, search(t, recipient.FullName), recipient.Name)
	})

	t.Run("ExcludesCurrentOwner", func(t *testing.T) {
		assert.NotContains(t, search(t, owner.Name), owner.Name)
	})

	t.Run("ExcludesOwnersOfSameSubject", func(t *testing.T) {
		other := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 5})
		sameSubject := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{OwnerID: other.ID})
		sameSubject.SubjectID = repo.SubjectID
		_, err := db.GetEngine(t.Context()).ID(sameSubject.ID).Cols("subject_id").Update(sameSubject)
		require.NoError(t, err)

		assert.NotContains(t, search(t, other.Name), other.Name)
	})

	t.Run("Unauthorized", func(t *testing.T) {
		user4 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
		otherSession := loginUser(t, user4.Name)
		req := NewRequest(t, "GET", candidatesURL+"?q=user")
		otherSession.MakeRequest(t, req, http.StatusNotFound)
	})
}

func TestArticleSettingsTransferUnauthorized(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	owner, repo, subjectName := loadArticleRepo(t, 1)
	user4 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
	recipient := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 5})

	// blocked before the handler runs, by the "settings" group admin requirement
	session := loginUser(t, user4.Name)
	req := NewRequestWithValues(t, "POST", fmt.Sprintf("/%s/%s/settings", owner.Name, repo.Name),
		transferForm(GetUserCSRFToken(t, session), owner.Name, subjectName, recipient.FullName))
	session.MakeRequest(t, req, http.StatusNotFound)

	unchanged := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: repo.ID})
	assert.Equal(t, repo_model.RepositoryReady, unchanged.Status)
}
