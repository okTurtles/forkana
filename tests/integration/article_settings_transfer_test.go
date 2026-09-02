// Copyright 2025 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	activities_model "code.gitea.io/gitea/models/activities"
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

// transferForm builds the payload the article transfer modal submits. The modal shows
// the recipient by full name but submits their username.
func transferForm(csrf, owner, subject, newOwnerName string) map[string]string {
	return map[string]string{
		"_csrf":               csrf,
		"action":              "transfer",
		"redirect_to_article": "true",
		"article_name":        owner + "/" + subject,
		"new_owner_name":      newOwnerName,
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
		form := transferForm(GetUserCSRFToken(t, session), owner.Name, subjectName, recipient.Name)
		// the confirmation is case-sensitive on purpose
		form["article_name"] = owner.Name + "/" + subjectName + "x"
		post(t, form)

		assert.Contains(t, flashText(t), "The article owner and subject you entered are incorrect.")
		unchanged := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: repo.ID})
		assert.Equal(t, repo_model.RepositoryReady, unchanged.Status)
	})

	t.Run("UnknownUsername", func(t *testing.T) {
		post(t, transferForm(GetUserCSRFToken(t, session), owner.Name, subjectName, "nobodyatall"))

		assert.Contains(t, flashText(t), "No user was found with that name.")
		unchanged := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: repo.ID})
		assert.Equal(t, repo_model.RepositoryReady, unchanged.Status)
	})

	t.Run("FullNameIsNotAccepted", func(t *testing.T) {
		post(t, transferForm(GetUserCSRFToken(t, session), owner.Name, subjectName, recipient.FullName))

		assert.Contains(t, flashText(t), "No user was found with that name.")
		unchanged := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: repo.ID})
		assert.Equal(t, repo_model.RepositoryReady, unchanged.Status)
	})

	t.Run("Start", func(t *testing.T) {
		// the recipient is resolved by username, which the modal submits
		post(t, transferForm(GetUserCSRFToken(t, session), owner.Name, subjectName, recipient.Name))

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

	t.Run("RecipientSeesBanner", func(t *testing.T) {
		articleURL := fmt.Sprintf("/article/%s/%s?view=article", owner.Name, subjectName)

		// the owner is not the recipient, so no banner is offered to them
		AssertHTMLElement(t, NewHTMLParser(t, session.MakeRequest(t, NewRequest(t, "GET", articleURL), http.StatusOK).Body),
			"#article-transfer-notice", false)

		recipientSession := loginUser(t, recipient.Name)
		htmlDoc := NewHTMLParser(t, recipientSession.MakeRequest(t, NewRequest(t, "GET", articleURL), http.StatusOK).Body)
		AssertHTMLElement(t, htmlDoc, "#article-transfer-notice", true)
		assert.Contains(t, htmlDoc.Find("#article-transfer-notice").Text(),
			fmt.Sprintf("%s wants to transfer the article %s to you.", owner.DisplayName(), subjectName))
	})

	t.Run("RejectNotifiesInitiator", func(t *testing.T) {
		// no notification for the initiator exists while the transfer is still pending
		unittest.AssertNotExistsBean(t, &activities_model.Notification{
			UserID: owner.ID,
			RepoID: repo.ID,
			Source: activities_model.NotificationSourceRepoTransferRejected,
		})

		recipientSession := loginUser(t, recipient.Name)
		req := NewRequestWithValues(t, "POST", fmt.Sprintf("/%s/%s/action/reject_transfer", owner.Name, repo.Name), map[string]string{
			"_csrf": GetUserCSRFToken(t, recipientSession),
		})
		recipientSession.MakeRequest(t, req, http.StatusSeeOther)

		// the article stays with its original owner and is usable again
		reverted := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: repo.ID})
		assert.Equal(t, repo_model.RepositoryReady, reverted.Status)
		assert.Equal(t, owner.ID, reverted.OwnerID)
		unittest.AssertNotExistsBean(t, &repo_model.RepoTransfer{RepoID: repo.ID})

		// and the owner is told the transfer was rejected rather than left guessing
		notification := unittest.AssertExistsAndLoadBean(t, &activities_model.Notification{
			UserID: owner.ID,
			RepoID: repo.ID,
			Source: activities_model.NotificationSourceRepoTransferRejected,
		})
		assert.Equal(t, activities_model.NotificationStatusUnread, notification.Status)
		assert.Equal(t, recipient.ID, notification.UpdatedBy)

		// the notification list names the rejection instead of repeating the article path
		htmlDoc := NewHTMLParser(t, session.MakeRequest(t, NewRequest(t, "GET", "/notifications"), http.StatusOK).Body)
		assert.Contains(t, htmlDoc.Find("#notification_"+strconv.FormatInt(notification.ID, 10)).Text(),
			"Your transfer of this article was rejected.")

		// restore the pending transfer the following subtests expect
		post(t, transferForm(GetUserCSRFToken(t, session), owner.Name, subjectName, recipient.Name))
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

	t.Run("StartWithoutFullName", func(t *testing.T) {
		// users without a full name are listed by their username, which must resolve too
		byUsername := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
		require.Empty(t, strings.TrimSpace(byUsername.FullName))
		post(t, transferForm(GetUserCSRFToken(t, session), owner.Name, subjectName, byUsername.Name))

		assert.Contains(t, flashText(t), "awaits confirmation from "+byUsername.DisplayName())
		unittest.AssertExistsAndLoadBean(t, &repo_model.RepoTransfer{RepoID: repo.ID, RecipientID: byUsername.ID})
	})

	t.Run("SelfTransferByUsername", func(t *testing.T) {
		post(t, transferForm(GetUserCSRFToken(t, session), owner.Name, subjectName, owner.Name))

		assert.Contains(t, flashText(t), "This article already belongs to that user.")
	})
}

func TestArticleSettingsTransferRecipientHasArticleOnSubject(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	owner, repo, subjectName := loadArticleRepo(t, 1)
	recipient := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 5})

	// the candidate search hides such users, the handler has to reject them as well
	sameSubject := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{OwnerID: recipient.ID})
	sameSubject.SubjectID = repo.SubjectID
	_, err := db.GetEngine(t.Context()).ID(sameSubject.ID).Cols("subject_id").Update(sameSubject)
	require.NoError(t, err)

	session := loginUser(t, owner.Name)
	articleSettingsURL := fmt.Sprintf("/article/%s/%s?view=article&mode=settings", owner.Name, subjectName)
	req := NewRequestWithValues(t, "POST", fmt.Sprintf("/%s/%s/settings", owner.Name, repo.Name),
		transferForm(GetUserCSRFToken(t, session), owner.Name, subjectName, recipient.Name))
	resp := session.MakeRequest(t, req, http.StatusSeeOther)
	assert.Equal(t, articleSettingsURL, test.RedirectURL(resp))

	flash := NewHTMLParser(t, session.MakeRequest(t, NewRequest(t, "GET", articleSettingsURL), http.StatusOK).Body).Find(".flash-message")
	require.Equal(t, 1, flash.Length())
	assert.Contains(t, flash.Text(), "The new owner already has an article on this subject.")

	unchanged := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: repo.ID})
	assert.Equal(t, repo_model.RepositoryReady, unchanged.Status)
	unittest.AssertNotExistsBean(t, &repo_model.RepoTransfer{RepoID: repo.ID})
}

func TestArticleSettingsTransferCandidates(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	owner, repo, subjectName := loadArticleRepo(t, 1)
	session := loginUser(t, owner.Name)
	candidatesURL := fmt.Sprintf("/article/%s/%s/settings/transfer_candidates", owner.Name, subjectName)

	searchUsers := func(t *testing.T, keyword string) []*api.User {
		t.Helper()
		req := NewRequest(t, "GET", candidatesURL+"?q="+url.QueryEscape(keyword))
		resp := session.MakeRequest(t, req, http.StatusOK)
		var body struct {
			Data []*api.User `json:"data"`
		}
		DecodeJSON(t, resp, &body)
		return body.Data
	}

	search := func(t *testing.T, keyword string) []string {
		t.Helper()
		users := searchUsers(t, keyword)
		logins := make([]string, 0, len(users))
		for _, u := range users {
			logins = append(logins, u.UserName)
		}
		return logins
	}

	t.Run("MatchesFullName", func(t *testing.T) {
		recipient := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 5})
		users := searchUsers(t, recipient.FullName)

		var found *api.User
		for _, u := range users {
			if u.UserName == recipient.Name {
				found = u
			}
		}
		require.NotNil(t, found)
		// the dropdown renders the avatar next to the username
		assert.NotEmpty(t, found.AvatarURL)
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
		transferForm(GetUserCSRFToken(t, session), owner.Name, subjectName, recipient.Name))
	session.MakeRequest(t, req, http.StatusNotFound)

	unchanged := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: repo.ID})
	assert.Equal(t, repo_model.RepositoryReady, unchanged.Status)
}
