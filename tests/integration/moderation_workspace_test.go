// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"fmt"
	"net/http"
	"testing"

	issues_model "code.gitea.io/gitea/models/issues"
	moderation_model "code.gitea.io/gitea/models/moderation"
	"code.gitea.io/gitea/models/organization"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/test"
	moderation_service "code.gitea.io/gitea/services/moderation"
	org_service "code.gitea.io/gitea/services/org"
	"code.gitea.io/gitea/tests"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModerationWorkspace(t *testing.T) {
	defer tests.PrepareTestEnv(t)()
	defer test.MockVariableValue(&setting.Moderation.Enabled, false)()

	admin := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 1})
	moderator := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
	outsider := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 5})

	_, err := moderation_service.EnsureSetup(t.Context(), moderation_service.SetupOptions{Owner: admin})
	require.NoError(t, err)
	org, err := organization.GetOrgByName(t.Context(), setting.Moderation.Org)
	require.NoError(t, err)
	team, err := organization.GetTeam(t.Context(), org.ID, setting.Moderation.Team)
	require.NoError(t, err)
	require.NoError(t, org_service.AddTeamMember(t.Context(), team, moderator))

	// One open case assigned to the moderator: the account menu badge shows 1.
	repo, err := moderation_service.GetCasesRepo(t.Context())
	require.NoError(t, err)
	caseLabel, err := issues_model.GetLabelInOrgByName(t.Context(), org.ID, moderation_model.LabelTypeCase)
	require.NoError(t, err)
	caseIssue := &issues_model.Issue{RepoID: repo.ID, Repo: repo, Title: "owner / article", PosterID: admin.ID, Poster: admin}
	require.NoError(t, issues_model.NewIssue(t.Context(), repo, caseIssue, []int64{caseLabel.ID}, nil))
	_, _, err = issues_model.ToggleIssueAssignee(t.Context(), caseIssue, admin, moderator.ID)
	require.NoError(t, err)
	caseURL := fmt.Sprintf("/-/moderation/cases/%d", caseIssue.Index)

	adminSession := loginUser(t, admin.Name)
	moderatorSession := loginUser(t, moderator.Name)
	outsiderSession := loginUser(t, outsider.Name)

	t.Run("Off", func(t *testing.T) {
		MakeRequest(t, NewRequest(t, "GET", "/-/moderation"), http.StatusNotFound)
		for _, s := range []*TestSession{adminSession, moderatorSession, outsiderSession} {
			s.MakeRequest(t, NewRequest(t, "GET", "/-/moderation"), http.StatusNotFound)
			s.MakeRequest(t, NewRequest(t, "GET", caseURL), http.StatusNotFound)
		}
		resp := moderatorSession.MakeRequest(t, NewRequest(t, "GET", "/"), http.StatusOK)
		assert.NotContains(t, resp.Body.String(), `id="navbar-moderation"`)
	})

	defer test.MockVariableValue(&setting.Moderation.Enabled, true)()

	t.Run("On", func(t *testing.T) {
		resp := MakeRequest(t, NewRequest(t, "GET", "/-/moderation"), http.StatusSeeOther)
		assert.Contains(t, resp.Header().Get("Location"), "/user/login")

		outsiderSession.MakeRequest(t, NewRequest(t, "GET", "/-/moderation"), http.StatusNotFound)
		outsiderSession.MakeRequest(t, NewRequest(t, "GET", caseURL), http.StatusNotFound)
		resp = outsiderSession.MakeRequest(t, NewRequest(t, "GET", "/"), http.StatusOK)
		assert.NotContains(t, resp.Body.String(), `id="navbar-moderation"`)

		for _, s := range []*TestSession{adminSession, moderatorSession} {
			s.MakeRequest(t, NewRequest(t, "GET", "/-/moderation"), http.StatusOK)
			s.MakeRequest(t, NewRequest(t, "GET", caseURL), http.StatusOK)
			s.MakeRequest(t, NewRequest(t, "GET", "/-/moderation/cases/999"), http.StatusNotFound)
		}

		doc := NewHTMLParser(t, moderatorSession.MakeRequest(t, NewRequest(t, "GET", "/"), http.StatusOK).Body)
		entry := doc.Find("#navbar-moderation")
		require.Equal(t, 1, entry.Length())
		assert.Equal(t, "1", entry.Find(".label").Text(), "the moderator's open cases")

		doc = NewHTMLParser(t, adminSession.MakeRequest(t, NewRequest(t, "GET", "/"), http.StatusOK).Body)
		entry = doc.Find("#navbar-moderation")
		require.Equal(t, 1, entry.Length())
		assert.Zero(t, entry.Find(".label").Length(), "no badge without assigned cases")
	})
}
