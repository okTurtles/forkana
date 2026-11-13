// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

// Package moderation holds Forkana's moderation rules: setup, access, reports, decisions and
// sanctions. Handlers call it; they never write moderation data themselves.
package moderation

import (
	"context"
	"errors"
	"fmt"

	"code.gitea.io/gitea/models/db"
	issues_model "code.gitea.io/gitea/models/issues"
	moderation_model "code.gitea.io/gitea/models/moderation"
	"code.gitea.io/gitea/models/organization"
	"code.gitea.io/gitea/models/perm"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unit"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/structs"
	org_service "code.gitea.io/gitea/services/org"
	repo_service "code.gitea.io/gitea/services/repository"
)

// SetupOptions configures EnsureSetup.
type SetupOptions struct {
	// Owner becomes the owner of the moderation organization and creates the cases
	// repository. It must be a site administrator. Only needed when something is created.
	Owner *user_model.User
	// DryRun reports what would be created without writing anything.
	DryRun bool
}

// SetupItem is one thing EnsureSetup looked for.
type SetupItem struct {
	Kind    string // "organization", "team", "repository", "team repository" or "label"
	Name    string
	Created bool // created by this run (or, in a dry run, would be)
}

// SetupResult is what EnsureSetup found and did.
type SetupResult struct {
	Items []SetupItem
	// Unwatched counts the users whose watch on the cases repository this run removed.
	Unwatched int
}

func (r *SetupResult) add(kind, name string, created bool) {
	r.Items = append(r.Items, SetupItem{Kind: kind, Name: name, Created: created})
}

// EnsureSetup creates what moderation needs and is missing: the organization, the moderators
// team, the private cases repository, the team's access to it and the core labels. Whatever
// already exists is left as it is, so it is safe to run again, also after a failed run.
// It runs whether moderation is enabled or not, so that it can be prepared before it is.
//
// It finishes by removing every watch on the cases repository: moderators get targeted
// alerts instead of a notification for every report.
func EnsureSetup(ctx context.Context, opts SetupOptions) (*SetupResult, error) {
	cfg := setting.Moderation
	result := &SetupResult{}

	needOwner := func() error {
		if opts.Owner == nil {
			return errors.New("an owner is needed to create the moderation organization and repository")
		}
		if !opts.Owner.IsAdmin {
			return fmt.Errorf("owner %q is not a site administrator", opts.Owner.Name)
		}
		return nil
	}

	org, err := organization.GetOrgByName(ctx, cfg.Org)
	if err != nil && !organization.IsErrOrgNotExist(err) {
		return nil, fmt.Errorf("get organization %q: %w", cfg.Org, err)
	}
	if org == nil {
		if err := needOwner(); err != nil {
			return nil, err
		}
		result.add("organization", cfg.Org, true)
		if opts.DryRun {
			// Nothing below can exist without the organization.
			result.add("team", cfg.Team, true)
			result.add("repository", cfg.Org+"/"+cfg.Repo, true)
			result.add("team repository", cfg.Team+" → "+cfg.Repo, true)
			for _, l := range moderation_model.CoreLabels() {
				result.add("label", l.Name, true)
			}
			return result, nil
		}
		org = &organization.Organization{
			Name:        cfg.Org,
			FullName:    "Forkana moderation",
			Description: "Reports and moderation cases",
			IsActive:    true,
			Type:        user_model.UserTypeOrganization,
			Visibility:  structs.VisibleTypePrivate,
		}
		if err := organization.CreateOrganization(ctx, org, opts.Owner); err != nil {
			return nil, fmt.Errorf("create organization %q: %w", cfg.Org, err)
		}
	} else {
		result.add("organization", cfg.Org, false)
	}

	team, err := organization.GetTeam(ctx, org.ID, cfg.Team)
	if err != nil && !organization.IsErrTeamNotExist(err) {
		return nil, fmt.Errorf("get team %q: %w", cfg.Team, err)
	}
	if team == nil {
		result.add("team", cfg.Team, true)
		if !opts.DryRun {
			team = &organization.Team{
				OrgID:       org.ID,
				Name:        cfg.Team,
				Description: "Moderators: they see and decide the reports and cases",
				AccessMode:  perm.AccessModeWrite,
				Units: []*organization.TeamUnit{{
					OrgID:      org.ID,
					Type:       unit.TypeIssues,
					AccessMode: perm.AccessModeWrite,
				}},
			}
			if err := org_service.NewTeam(ctx, team); err != nil {
				return nil, fmt.Errorf("create team %q: %w", cfg.Team, err)
			}
		}
	} else {
		result.add("team", cfg.Team, false)
	}

	repo, err := repo_model.GetRepositoryByName(ctx, org.ID, cfg.Repo)
	if err != nil && !repo_model.IsErrRepoNotExist(err) {
		return nil, fmt.Errorf("get repository %q: %w", cfg.Repo, err)
	}
	if repo == nil {
		result.add("repository", cfg.Org+"/"+cfg.Repo, true)
		if !opts.DryRun {
			if err := needOwner(); err != nil {
				return nil, err
			}
			if repo, err = createCasesRepo(ctx, opts.Owner, org); err != nil {
				return nil, err
			}
		}
	} else {
		if !repo.IsPrivate {
			// Reports name their reporters: never let a public repository hold them.
			return nil, fmt.Errorf("repository %s/%s exists but is public; make it private and run setup again", cfg.Org, cfg.Repo)
		}
		result.add("repository", cfg.Org+"/"+cfg.Repo, false)
	}

	if team != nil && repo != nil {
		has := repo_service.HasRepository(ctx, team, repo.ID)
		result.add("team repository", cfg.Team+" → "+cfg.Repo, !has)
		if !has && !opts.DryRun {
			if err := repo_service.TeamAddRepository(ctx, team, repo); err != nil {
				return nil, fmt.Errorf("add repository %q to team %q: %w", cfg.Repo, cfg.Team, err)
			}
		}
	} else {
		result.add("team repository", cfg.Team+" → "+cfg.Repo, true)
	}

	if err := ensureCoreLabels(ctx, org.ID, opts.DryRun, result); err != nil {
		return nil, err
	}

	if repo != nil && !opts.DryRun {
		if result.Unwatched, err = unwatchAll(ctx, repo); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// createCasesRepo creates the private cases repository with the Issues unit only (with
// dependencies on: a case depends on its reports).
func createCasesRepo(ctx context.Context, doer *user_model.User, org *organization.Organization) (*repo_model.Repository, error) {
	repo, err := repo_service.CreateRepositoryDirectly(ctx, doer, org.AsUser(), repo_service.CreateRepoOptions{
		Name:        setting.Moderation.Repo,
		Description: "Reports and moderation cases",
		IsPrivate:   true,
	}, true)
	if err != nil {
		return nil, fmt.Errorf("create repository %q: %w", setting.Moderation.Repo, err)
	}

	if err := repo.LoadUnits(ctx); err != nil {
		return nil, fmt.Errorf("load units: %w", err)
	}
	var deleteTypes []unit.Type
	for _, u := range repo.Units {
		if u.Type != unit.TypeIssues {
			deleteTypes = append(deleteTypes, u.Type)
		}
	}
	issues := repo_model.RepoUnit{
		RepoID: repo.ID,
		Type:   unit.TypeIssues,
		Config: &repo_model.IssuesConfig{EnableDependencies: true},
	}
	if err := repo_service.UpdateRepositoryUnits(ctx, repo, []repo_model.RepoUnit{issues}, deleteTypes); err != nil {
		return nil, fmt.Errorf("keep only the Issues unit: %w", err)
	}
	return repo, nil
}

// ensureCoreLabels creates, as org labels, the core labels not there yet. Labels are matched by
// name only: one an admin changed keeps its changes.
func ensureCoreLabels(ctx context.Context, orgID int64, dryRun bool, result *SetupResult) error {
	existing := map[string]bool{}
	if orgID > 0 {
		labels, err := issues_model.GetLabelsByOrgID(ctx, orgID, "", db.ListOptionsAll)
		if err != nil {
			return fmt.Errorf("get organization labels: %w", err)
		}
		for _, l := range labels {
			existing[l.Name] = true
		}
	}
	for _, l := range moderation_model.CoreLabels() {
		if existing[l.Name] {
			result.add("label", l.Name, false)
			continue
		}
		result.add("label", l.Name, true)
		if dryRun {
			continue
		}
		if err := issues_model.NewLabel(ctx, &issues_model.Label{
			OrgID:       orgID,
			Name:        l.Name,
			Exclusive:   l.Exclusive,
			Color:       l.Color,
			Description: l.Description,
		}); err != nil {
			return fmt.Errorf("create label %q: %w", l.Name, err)
		}
	}
	return nil
}

// unwatchAll removes every watch on repo and returns how many it removed.
func unwatchAll(ctx context.Context, repo *repo_model.Repository) (int, error) {
	ids, err := repo_model.GetRepoWatchersIDs(ctx, repo.ID)
	if err != nil {
		return 0, fmt.Errorf("get watchers: %w", err)
	}
	if len(ids) == 0 {
		return 0, nil
	}
	users, err := user_model.GetUsersByIDs(ctx, ids)
	if err != nil {
		return 0, fmt.Errorf("get watchers: %w", err)
	}
	for _, u := range users {
		if err := repo_model.WatchRepo(ctx, u, repo, false); err != nil {
			return 0, fmt.Errorf("unwatch %q: %w", u.Name, err)
		}
	}
	return len(users), nil
}
