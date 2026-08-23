// Copyright 2014 The Gogs Authors. All rights reserved.
// Copyright 2018 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package setting

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"code.gitea.io/gitea/models/db"
	"code.gitea.io/gitea/models/organization"
	repo_model "code.gitea.io/gitea/models/repo"
	unit_model "code.gitea.io/gitea/models/unit"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/gitrepo"
	"code.gitea.io/gitea/modules/htmlutil"
	"code.gitea.io/gitea/modules/indexer/code"
	issue_indexer "code.gitea.io/gitea/modules/indexer/issues"
	"code.gitea.io/gitea/modules/indexer/stats"
	"code.gitea.io/gitea/modules/lfs"
	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/optional"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/structs"
	"code.gitea.io/gitea/modules/templates"
	"code.gitea.io/gitea/modules/util"
	"code.gitea.io/gitea/modules/validation"
	"code.gitea.io/gitea/modules/web"
	actions_service "code.gitea.io/gitea/services/actions"
	asymkey_service "code.gitea.io/gitea/services/asymkey"
	"code.gitea.io/gitea/services/context"
	convert_service "code.gitea.io/gitea/services/convert"
	"code.gitea.io/gitea/services/forms"
	"code.gitea.io/gitea/services/migrations"
	mirror_service "code.gitea.io/gitea/services/mirror"
	repo_service "code.gitea.io/gitea/services/repository"
	wiki_service "code.gitea.io/gitea/services/wiki"

	"xorm.io/xorm/convert"
)

const (
	tplSettingsOptions templates.TplName = "repo/settings/options"
	tplCollaboration   templates.TplName = "repo/settings/collaboration"
	tplBranches        templates.TplName = "repo/settings/branches"
	tplGithooks        templates.TplName = "repo/settings/githooks"
	tplGithookEdit     templates.TplName = "repo/settings/githook_edit"
	tplDeployKeys      templates.TplName = "repo/settings/deploy_keys"
)

// SettingsCtxData is a middleware that sets all the general context data for the
// settings template.
func SettingsCtxData(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.settings.options")
	ctx.Data["PageIsSettingsOptions"] = true
	ctx.Data["ForcePrivate"] = setting.Repository.ForcePrivate
	ctx.Data["MirrorsEnabled"] = setting.Mirror.Enabled
	ctx.Data["DisableNewPullMirrors"] = setting.Mirror.DisableNewPull
	ctx.Data["DisableNewPushMirrors"] = setting.Mirror.DisableNewPush
	ctx.Data["DefaultMirrorInterval"] = setting.Mirror.DefaultInterval
	ctx.Data["MinimumMirrorInterval"] = setting.Mirror.MinInterval
	ctx.Data["CanConvertFork"] = ctx.Repo.Repository.IsFork && ctx.Doer.CanCreateRepoIn(ctx.Repo.Repository.Owner)

	signing, _ := asymkey_service.SigningKey(ctx, ctx.Repo.Repository.RepoPath())
	ctx.Data["SigningKeyAvailable"] = signing != nil
	ctx.Data["SigningSettings"] = setting.Repository.Signing
	ctx.Data["IsRepoIndexerEnabled"] = setting.Indexer.RepoIndexerEnabled

	if ctx.Doer.IsAdmin {
		if setting.Indexer.RepoIndexerEnabled {
			status, err := repo_model.GetIndexerStatus(ctx, ctx.Repo.Repository, repo_model.RepoIndexerTypeCode)
			if err != nil {
				ctx.ServerError("repo.indexer_status", err)
				return
			}
			ctx.Data["CodeIndexerStatus"] = status
		}
		status, err := repo_model.GetIndexerStatus(ctx, ctx.Repo.Repository, repo_model.RepoIndexerTypeStats)
		if err != nil {
			ctx.ServerError("repo.indexer_status", err)
			return
		}
		ctx.Data["StatsIndexerStatus"] = status
	}
	pushMirrors, _, err := repo_model.GetPushMirrorsByRepoID(ctx, ctx.Repo.Repository.ID, db.ListOptions{})
	if err != nil {
		ctx.ServerError("GetPushMirrorsByRepoID", err)
		return
	}
	ctx.Data["PushMirrors"] = pushMirrors
}

// Settings show a repository's settings page
func Settings(ctx *context.Context) {
	ctx.HTML(http.StatusOK, tplSettingsOptions)
}

// SettingsPost response for changes of a repository
func SettingsPost(ctx *context.Context) {
	ctx.Data["ForcePrivate"] = setting.Repository.ForcePrivate
	ctx.Data["MirrorsEnabled"] = setting.Mirror.Enabled
	ctx.Data["DisableNewPullMirrors"] = setting.Mirror.DisableNewPull
	ctx.Data["DisableNewPushMirrors"] = setting.Mirror.DisableNewPush
	ctx.Data["DefaultMirrorInterval"] = setting.Mirror.DefaultInterval
	ctx.Data["MinimumMirrorInterval"] = setting.Mirror.MinInterval

	signing, _ := asymkey_service.SigningKey(ctx, ctx.Repo.Repository.RepoPath())
	ctx.Data["SigningKeyAvailable"] = signing != nil
	ctx.Data["SigningSettings"] = setting.Repository.Signing
	ctx.Data["IsRepoIndexerEnabled"] = setting.Indexer.RepoIndexerEnabled

	switch ctx.FormString("action") {
	case "update":
		handleSettingsPostUpdate(ctx)
	case "mirror":
		handleSettingsPostMirror(ctx)
	case "mirror-sync":
		handleSettingsPostMirrorSync(ctx)
	case "push-mirror-sync":
		handleSettingsPostPushMirrorSync(ctx)
	case "push-mirror-update":
		handleSettingsPostPushMirrorUpdate(ctx)
	case "push-mirror-remove":
		handleSettingsPostPushMirrorRemove(ctx)
	case "push-mirror-add":
		handleSettingsPostPushMirrorAdd(ctx)
	case "advanced":
		handleSettingsPostAdvanced(ctx)
	case "signing":
		handleSettingsPostSigning(ctx)
	case "admin":
		handleSettingsPostAdmin(ctx)
	case "admin_index":
		handleSettingsPostAdminIndex(ctx)
	case "convert":
		handleSettingsPostConvert(ctx)
	case "convert_fork":
		handleSettingsPostConvertFork(ctx)
	case "transfer":
		handleSettingsPostTransfer(ctx)
	case "cancel_transfer":
		handleSettingsPostCancelTransfer(ctx)
	case "delete":
		handleSettingsPostDelete(ctx)
	case "delete-wiki":
		handleSettingsPostDeleteWiki(ctx)
	case "archive":
		handleSettingsPostArchive(ctx)
	case "unarchive":
		handleSettingsPostUnarchive(ctx)
	case "visibility":
		handleSettingsPostVisibility(ctx)
	default:
		ctx.NotFound(nil)
	}
}

func handleSettingsPostUpdate(ctx *context.Context) {
	form := web.GetForm(ctx).(*forms.RepoSettingForm)
	repo := ctx.Repo.Repository
	if ctx.HasError() {
		ctx.HTML(http.StatusOK, tplSettingsOptions)
		return
	}

	newRepoName := form.RepoName
	// Check if repository name has been changed.
	if !strings.EqualFold(repo.LowerName, newRepoName) {
		// Close the GitRepo if open
		if ctx.Repo.GitRepo != nil {
			ctx.Repo.GitRepo.Close()
			ctx.Repo.GitRepo = nil
		}
		if err := repo_service.ChangeRepositoryName(ctx, ctx.Doer, repo, newRepoName); err != nil {
			ctx.Data["Err_RepoName"] = true
			switch {
			case repo_model.IsErrRepoAlreadyExist(err):
				ctx.RenderWithErr(ctx.Tr("form.repo_name_been_taken"), tplSettingsOptions, &form)
			case db.IsErrNameReserved(err):
				ctx.RenderWithErr(ctx.Tr("repo.form.name_reserved", err.(db.ErrNameReserved).Name), tplSettingsOptions, &form)
			case repo_model.IsErrRepoFilesAlreadyExist(err):
				ctx.Data["Err_RepoName"] = true
				switch {
				case ctx.IsUserSiteAdmin() || (setting.Repository.AllowAdoptionOfUnadoptedRepositories && setting.Repository.AllowDeleteOfUnadoptedRepositories):
					ctx.RenderWithErr(ctx.Tr("form.repository_files_already_exist.adopt_or_delete"), tplSettingsOptions, form)
				case setting.Repository.AllowAdoptionOfUnadoptedRepositories:
					ctx.RenderWithErr(ctx.Tr("form.repository_files_already_exist.adopt"), tplSettingsOptions, form)
				case setting.Repository.AllowDeleteOfUnadoptedRepositories:
					ctx.RenderWithErr(ctx.Tr("form.repository_files_already_exist.delete"), tplSettingsOptions, form)
				default:
					ctx.RenderWithErr(ctx.Tr("form.repository_files_already_exist"), tplSettingsOptions, form)
				}
			case db.IsErrNamePatternNotAllowed(err):
				ctx.RenderWithErr(ctx.Tr("repo.form.name_pattern_not_allowed", err.(db.ErrNamePatternNotAllowed).Pattern), tplSettingsOptions, &form)
			default:
				ctx.ServerError("ChangeRepositoryName", err)
			}
			return
		}

		log.Trace("Repository name changed: %s/%s -> %s", ctx.Repo.Owner.Name, repo.Name, newRepoName)
	}
	// In case it's just a case change.
	repo.Name = newRepoName
	repo.LowerName = strings.ToLower(newRepoName)

	// Subject cannot be edited after repository creation
	if form.Subject != repo.GetSubject(ctx) {
		ctx.Data["Err_Subject"] = true
		ctx.RenderWithErr(ctx.Tr("repo.subject_cannot_be_modified"), tplSettingsOptions, &form)
		return
	}

	repo.Description = form.Description
	repo.Website = form.Website
	repo.IsTemplate = form.Template

	// Visibility of forked repository is forced sync with base repository.
	if repo.IsFork {
		form.Private = repo.BaseRepo.IsPrivate || repo.BaseRepo.Owner.Visibility == structs.VisibleTypePrivate
	}

	if err := repo_service.UpdateRepository(ctx, repo, false); err != nil {
		ctx.ServerError("UpdateRepository", err)
		return
	}
	log.Trace("Repository basic settings updated: %s/%s", ctx.Repo.Owner.Name, repo.Name)

	ctx.Flash.Success(ctx.Tr("repo.settings.update_settings_success"))
	ctx.Redirect(repo.Link() + "/settings")
}

func handleSettingsPostMirror(ctx *context.Context) {
	form := web.GetForm(ctx).(*forms.RepoSettingForm)
	repo := ctx.Repo.Repository
	if !setting.Mirror.Enabled || !repo.IsMirror || repo.IsArchived {
		ctx.NotFound(nil)
		return
	}

	pullMirror, err := repo_model.GetMirrorByRepoID(ctx, ctx.Repo.Repository.ID)
	if err == repo_model.ErrMirrorNotExist {
		ctx.NotFound(nil)
		return
	}
	if err != nil {
		ctx.ServerError("GetMirrorByRepoID", err)
		return
	}
	// This section doesn't require repo_name/RepoName to be set in the form, don't show it
	// as an error on the UI for this action
	ctx.Data["Err_RepoName"] = nil

	interval, err := time.ParseDuration(form.Interval)
	if err != nil || (interval != 0 && interval < setting.Mirror.MinInterval) {
		ctx.Data["Err_Interval"] = true
		ctx.RenderWithErr(ctx.Tr("repo.mirror_interval_invalid"), tplSettingsOptions, &form)
		return
	}

	pullMirror.EnablePrune = form.EnablePrune
	pullMirror.Interval = interval
	pullMirror.ScheduleNextUpdate()
	if err := repo_model.UpdateMirror(ctx, pullMirror); err != nil {
		ctx.ServerError("UpdateMirror", err)
		return
	}

	u, err := gitrepo.GitRemoteGetURL(ctx, ctx.Repo.Repository, pullMirror.GetRemoteName())
	if err != nil {
		ctx.Data["Err_MirrorAddress"] = true
		handleSettingRemoteAddrError(ctx, err, form)
		return
	}
	if u.User != nil && form.MirrorPassword == "" && form.MirrorUsername == u.User.Username() {
		form.MirrorPassword, _ = u.User.Password()
	}

	address, err := git.ParseRemoteAddr(form.MirrorAddress, form.MirrorUsername, form.MirrorPassword)
	if err == nil {
		err = migrations.IsMigrateURLAllowed(address, ctx.Doer)
	}
	if err != nil {
		ctx.Data["Err_MirrorAddress"] = true
		handleSettingRemoteAddrError(ctx, err, form)
		return
	}

	if err := mirror_service.UpdateAddress(ctx, pullMirror, address); err != nil {
		ctx.ServerError("UpdateAddress", err)
		return
	}

	remoteAddress, err := util.SanitizeURL(form.MirrorAddress)
	if err != nil {
		ctx.Data["Err_MirrorAddress"] = true
		handleSettingRemoteAddrError(ctx, err, form)
		return
	}
	pullMirror.RemoteAddress = remoteAddress

	form.LFS = form.LFS && setting.LFS.StartServer

	if len(form.LFSEndpoint) > 0 {
		ep := lfs.DetermineEndpoint("", form.LFSEndpoint)
		if ep == nil {
			ctx.Data["Err_LFSEndpoint"] = true
			ctx.RenderWithErr(ctx.Tr("repo.migrate.invalid_lfs_endpoint"), tplSettingsOptions, &form)
			return
		}
		err = migrations.IsMigrateURLAllowed(ep.String(), ctx.Doer)
		if err != nil {
			ctx.Data["Err_LFSEndpoint"] = true
			handleSettingRemoteAddrError(ctx, err, form)
			return
		}
	}

	pullMirror.LFS = form.LFS
	pullMirror.LFSEndpoint = form.LFSEndpoint
	if err := repo_model.UpdateMirror(ctx, pullMirror); err != nil {
		ctx.ServerError("UpdateMirror", err)
		return
	}

	ctx.Flash.Success(ctx.Tr("repo.settings.update_settings_success"))
	ctx.Redirect(repo.Link() + "/settings")
}

func handleSettingsPostMirrorSync(ctx *context.Context) {
	repo := ctx.Repo.Repository
	if !setting.Mirror.Enabled || !repo.IsMirror || repo.IsArchived {
		ctx.NotFound(nil)
		return
	}

	mirror_service.AddPullMirrorToQueue(repo.ID)

	ctx.Flash.Info(ctx.Tr("repo.settings.pull_mirror_sync_in_progress", repo.OriginalURL))
	ctx.Redirect(repo.Link() + "/settings")
}

func handleSettingsPostPushMirrorSync(ctx *context.Context) {
	form := web.GetForm(ctx).(*forms.RepoSettingForm)
	repo := ctx.Repo.Repository

	if !setting.Mirror.Enabled {
		ctx.NotFound(nil)
		return
	}

	m, _, _ := repo_model.GetPushMirrorByIDAndRepoID(ctx, form.PushMirrorID, repo.ID)
	if m == nil {
		ctx.NotFound(nil)
		return
	}

	mirror_service.AddPushMirrorToQueue(m.ID)

	ctx.Flash.Info(ctx.Tr("repo.settings.push_mirror_sync_in_progress", m.RemoteAddress))
	ctx.Redirect(repo.Link() + "/settings")
}

func handleSettingsPostPushMirrorUpdate(ctx *context.Context) {
	form := web.GetForm(ctx).(*forms.RepoSettingForm)
	repo := ctx.Repo.Repository

	if !setting.Mirror.Enabled || repo.IsArchived {
		ctx.NotFound(nil)
		return
	}

	// This section doesn't require repo_name/RepoName to be set in the form, don't show it
	// as an error on the UI for this action
	ctx.Data["Err_RepoName"] = nil

	interval, err := time.ParseDuration(form.PushMirrorInterval)
	if err != nil || (interval != 0 && interval < setting.Mirror.MinInterval) {
		ctx.RenderWithErr(ctx.Tr("repo.mirror_interval_invalid"), tplSettingsOptions, &forms.RepoSettingForm{})
		return
	}

	m, _, _ := repo_model.GetPushMirrorByIDAndRepoID(ctx, form.PushMirrorID, repo.ID)
	if m == nil {
		ctx.NotFound(nil)
		return
	}

	m.Interval = interval
	if err := repo_model.UpdatePushMirrorInterval(ctx, m); err != nil {
		ctx.ServerError("UpdatePushMirrorInterval", err)
		return
	}
	// Background why we are adding it to Queue
	// If we observed its implementation in the context of `push-mirror-sync` where it
	// is evident that pushing to the queue is necessary for updates.
	// So, there are updates within the given interval, it is necessary to update the queue accordingly.
	if !ctx.FormBool("push_mirror_defer_sync") {
		// push_mirror_defer_sync is mainly for testing purpose, we do not really want to sync the push mirror immediately
		mirror_service.AddPushMirrorToQueue(m.ID)
	}
	ctx.Flash.Success(ctx.Tr("repo.settings.update_settings_success"))
	ctx.Redirect(repo.Link() + "/settings")
}

func handleSettingsPostPushMirrorRemove(ctx *context.Context) {
	form := web.GetForm(ctx).(*forms.RepoSettingForm)
	repo := ctx.Repo.Repository

	if !setting.Mirror.Enabled || repo.IsArchived {
		ctx.NotFound(nil)
		return
	}

	// This section doesn't require repo_name/RepoName to be set in the form, don't show it
	// as an error on the UI for this action
	ctx.Data["Err_RepoName"] = nil

	m, _, _ := repo_model.GetPushMirrorByIDAndRepoID(ctx, form.PushMirrorID, repo.ID)
	if m == nil {
		ctx.NotFound(nil)
		return
	}

	if err := mirror_service.RemovePushMirrorRemote(ctx, m); err != nil {
		ctx.ServerError("RemovePushMirrorRemote", err)
		return
	}

	if err := repo_model.DeletePushMirrors(ctx, repo_model.PushMirrorOptions{ID: m.ID, RepoID: m.RepoID}); err != nil {
		ctx.ServerError("DeletePushMirrorByID", err)
		return
	}

	ctx.Flash.Success(ctx.Tr("repo.settings.update_settings_success"))
	ctx.Redirect(repo.Link() + "/settings")
}

func handleSettingsPostPushMirrorAdd(ctx *context.Context) {
	form := web.GetForm(ctx).(*forms.RepoSettingForm)
	repo := ctx.Repo.Repository

	if setting.Mirror.DisableNewPush || repo.IsArchived {
		ctx.NotFound(nil)
		return
	}

	// This section doesn't require repo_name/RepoName to be set in the form, don't show it
	// as an error on the UI for this action
	ctx.Data["Err_RepoName"] = nil

	interval, err := time.ParseDuration(form.PushMirrorInterval)
	if err != nil || (interval != 0 && interval < setting.Mirror.MinInterval) {
		ctx.Data["Err_PushMirrorInterval"] = true
		ctx.RenderWithErr(ctx.Tr("repo.mirror_interval_invalid"), tplSettingsOptions, &form)
		return
	}

	address, err := git.ParseRemoteAddr(form.PushMirrorAddress, form.PushMirrorUsername, form.PushMirrorPassword)
	if err == nil {
		err = migrations.IsMigrateURLAllowed(address, ctx.Doer)
	}
	if err != nil {
		ctx.Data["Err_PushMirrorAddress"] = true
		handleSettingRemoteAddrError(ctx, err, form)
		return
	}

	remoteSuffix, err := util.CryptoRandomString(10)
	if err != nil {
		ctx.ServerError("RandomString", err)
		return
	}

	remoteAddress, err := util.SanitizeURL(form.PushMirrorAddress)
	if err != nil {
		ctx.Data["Err_PushMirrorAddress"] = true
		handleSettingRemoteAddrError(ctx, err, form)
		return
	}

	m := &repo_model.PushMirror{
		RepoID:        repo.ID,
		Repo:          repo,
		RemoteName:    "remote_mirror_" + remoteSuffix,
		SyncOnCommit:  form.PushMirrorSyncOnCommit,
		Interval:      interval,
		RemoteAddress: remoteAddress,
	}
	if err := db.Insert(ctx, m); err != nil {
		ctx.ServerError("InsertPushMirror", err)
		return
	}

	if err := mirror_service.AddPushMirrorRemote(ctx, m, address); err != nil {
		if err := repo_model.DeletePushMirrors(ctx, repo_model.PushMirrorOptions{ID: m.ID, RepoID: m.RepoID}); err != nil {
			log.Error("DeletePushMirrors %v", err)
		}
		ctx.ServerError("AddPushMirrorRemote", err)
		return
	}

	ctx.Flash.Success(ctx.Tr("repo.settings.update_settings_success"))
	ctx.Redirect(repo.Link() + "/settings")
}

func newRepoUnit(repo *repo_model.Repository, unitType unit_model.Type, config convert.Conversion) repo_model.RepoUnit {
	repoUnit := repo_model.RepoUnit{RepoID: repo.ID, Type: unitType, Config: config}
	for _, u := range repo.Units {
		if u.Type == unitType {
			repoUnit.EveryoneAccessMode = u.EveryoneAccessMode
			repoUnit.AnonymousAccessMode = u.AnonymousAccessMode
		}
	}
	return repoUnit
}

func handleSettingsPostAdvanced(ctx *context.Context) {
	form := web.GetForm(ctx).(*forms.RepoSettingForm)
	repo := ctx.Repo.Repository
	var repoChanged bool
	var units []repo_model.RepoUnit
	var deleteUnitTypes []unit_model.Type

	// This section doesn't require repo_name/RepoName to be set in the form, don't show it
	// as an error on the UI for this action
	ctx.Data["Err_RepoName"] = nil

	if repo.CloseIssuesViaCommitInAnyBranch != form.EnableCloseIssuesViaCommitInAnyBranch {
		repo.CloseIssuesViaCommitInAnyBranch = form.EnableCloseIssuesViaCommitInAnyBranch
		repoChanged = true
	}

	if form.EnableCode && !unit_model.TypeCode.UnitGlobalDisabled() {
		units = append(units, newRepoUnit(repo, unit_model.TypeCode, nil))
	} else if !unit_model.TypeCode.UnitGlobalDisabled() {
		deleteUnitTypes = append(deleteUnitTypes, unit_model.TypeCode)
	}

	if form.EnableWiki && form.EnableExternalWiki && !unit_model.TypeExternalWiki.UnitGlobalDisabled() {
		if !validation.IsValidExternalURL(form.ExternalWikiURL) {
			ctx.Flash.Error(ctx.Tr("repo.settings.external_wiki_url_error"))
			ctx.Redirect(repo.Link() + "/settings")
			return
		}

		units = append(units, newRepoUnit(repo, unit_model.TypeExternalWiki, &repo_model.ExternalWikiConfig{
			ExternalWikiURL: form.ExternalWikiURL,
		}))
		deleteUnitTypes = append(deleteUnitTypes, unit_model.TypeWiki)
	} else if form.EnableWiki && !form.EnableExternalWiki && !unit_model.TypeWiki.UnitGlobalDisabled() {
		units = append(units, newRepoUnit(repo, unit_model.TypeWiki, new(repo_model.UnitConfig)))
		deleteUnitTypes = append(deleteUnitTypes, unit_model.TypeExternalWiki)
	} else {
		if !unit_model.TypeExternalWiki.UnitGlobalDisabled() {
			deleteUnitTypes = append(deleteUnitTypes, unit_model.TypeExternalWiki)
		}
		if !unit_model.TypeWiki.UnitGlobalDisabled() {
			deleteUnitTypes = append(deleteUnitTypes, unit_model.TypeWiki)
		}
	}

	if form.DefaultWikiBranch != "" {
		if err := wiki_service.ChangeDefaultWikiBranch(ctx, repo, form.DefaultWikiBranch); err != nil {
			log.Error("ChangeDefaultWikiBranch failed, err: %v", err)
			ctx.Flash.Warning(ctx.Tr("repo.settings.failed_to_change_default_wiki_branch"))
		}
	}

	if form.EnableIssues && form.EnableExternalTracker && !unit_model.TypeExternalTracker.UnitGlobalDisabled() {
		if !validation.IsValidExternalURL(form.ExternalTrackerURL) {
			ctx.Flash.Error(ctx.Tr("repo.settings.external_tracker_url_error"))
			ctx.Redirect(repo.Link() + "/settings")
			return
		}
		if len(form.TrackerURLFormat) != 0 && !validation.IsValidExternalTrackerURLFormat(form.TrackerURLFormat) {
			ctx.Flash.Error(ctx.Tr("repo.settings.tracker_url_format_error"))
			ctx.Redirect(repo.Link() + "/settings")
			return
		}
		units = append(units, newRepoUnit(repo, unit_model.TypeExternalTracker, &repo_model.ExternalTrackerConfig{
			ExternalTrackerURL:           form.ExternalTrackerURL,
			ExternalTrackerFormat:        form.TrackerURLFormat,
			ExternalTrackerStyle:         form.TrackerIssueStyle,
			ExternalTrackerRegexpPattern: form.ExternalTrackerRegexpPattern,
		}))
		deleteUnitTypes = append(deleteUnitTypes, unit_model.TypeIssues)
	} else if form.EnableIssues && !form.EnableExternalTracker && !unit_model.TypeIssues.UnitGlobalDisabled() {
		units = append(units, newRepoUnit(repo, unit_model.TypeIssues, &repo_model.IssuesConfig{
			EnableTimetracker:                form.EnableTimetracker,
			AllowOnlyContributorsToTrackTime: form.AllowOnlyContributorsToTrackTime,
			EnableDependencies:               form.EnableIssueDependencies,
		}))
		deleteUnitTypes = append(deleteUnitTypes, unit_model.TypeExternalTracker)
	} else {
		if !unit_model.TypeExternalTracker.UnitGlobalDisabled() {
			deleteUnitTypes = append(deleteUnitTypes, unit_model.TypeExternalTracker)
		}
		if !unit_model.TypeIssues.UnitGlobalDisabled() {
			deleteUnitTypes = append(deleteUnitTypes, unit_model.TypeIssues)
		}
	}

	if form.EnableProjects && !unit_model.TypeProjects.UnitGlobalDisabled() {
		units = append(units, newRepoUnit(repo, unit_model.TypeProjects, &repo_model.ProjectsConfig{
			ProjectsMode: repo_model.ProjectsMode(form.ProjectsMode),
		}))
	} else if !unit_model.TypeProjects.UnitGlobalDisabled() {
		deleteUnitTypes = append(deleteUnitTypes, unit_model.TypeProjects)
	}

	if form.EnableReleases && !unit_model.TypeReleases.UnitGlobalDisabled() {
		units = append(units, newRepoUnit(repo, unit_model.TypeReleases, nil))
	} else if !unit_model.TypeReleases.UnitGlobalDisabled() {
		deleteUnitTypes = append(deleteUnitTypes, unit_model.TypeReleases)
	}

	if form.EnablePackages && !unit_model.TypePackages.UnitGlobalDisabled() {
		units = append(units, newRepoUnit(repo, unit_model.TypePackages, nil))
	} else if !unit_model.TypePackages.UnitGlobalDisabled() {
		deleteUnitTypes = append(deleteUnitTypes, unit_model.TypePackages)
	}

	if form.EnableActions && !unit_model.TypeActions.UnitGlobalDisabled() {
		units = append(units, newRepoUnit(repo, unit_model.TypeActions, nil))
	} else if !unit_model.TypeActions.UnitGlobalDisabled() {
		deleteUnitTypes = append(deleteUnitTypes, unit_model.TypeActions)
	}

	if form.EnablePulls && !unit_model.TypePullRequests.UnitGlobalDisabled() {
		units = append(units, newRepoUnit(repo, unit_model.TypePullRequests, &repo_model.PullRequestsConfig{
			IgnoreWhitespaceConflicts:     form.PullsIgnoreWhitespace,
			AllowMerge:                    form.PullsAllowMerge,
			AllowRebase:                   form.PullsAllowRebase,
			AllowRebaseMerge:              form.PullsAllowRebaseMerge,
			AllowSquash:                   form.PullsAllowSquash,
			AllowFastForwardOnly:          form.PullsAllowFastForwardOnly,
			AllowManualMerge:              form.PullsAllowManualMerge,
			AutodetectManualMerge:         form.EnableAutodetectManualMerge,
			AllowRebaseUpdate:             form.PullsAllowRebaseUpdate,
			DefaultDeleteBranchAfterMerge: form.DefaultDeleteBranchAfterMerge,
			DefaultMergeStyle:             repo_model.MergeStyle(form.PullsDefaultMergeStyle),
			DefaultAllowMaintainerEdit:    form.DefaultAllowMaintainerEdit,
		}))
	} else if !unit_model.TypePullRequests.UnitGlobalDisabled() {
		deleteUnitTypes = append(deleteUnitTypes, unit_model.TypePullRequests)
	}

	if len(units) == 0 {
		ctx.Flash.Error(ctx.Tr("repo.settings.update_settings_no_unit"))
		ctx.Redirect(ctx.Repo.RepoLink + "/settings")
		return
	}

	if err := repo_service.UpdateRepositoryUnits(ctx, repo, units, deleteUnitTypes); err != nil {
		ctx.ServerError("UpdateRepositoryUnits", err)
		return
	}
	if repoChanged {
		if err := repo_service.UpdateRepository(ctx, repo, false); err != nil {
			ctx.ServerError("UpdateRepository", err)
			return
		}
	}
	log.Trace("Repository advanced settings updated: %s/%s", ctx.Repo.Owner.Name, repo.Name)

	ctx.Flash.Success(ctx.Tr("repo.settings.update_settings_success"))
	ctx.Redirect(ctx.Repo.RepoLink + "/settings")
}

func handleSettingsPostSigning(ctx *context.Context) {
	form := web.GetForm(ctx).(*forms.RepoSettingForm)
	repo := ctx.Repo.Repository
	trustModel := repo_model.ToTrustModel(form.TrustModel)
	if trustModel != repo.TrustModel {
		repo.TrustModel = trustModel
		if err := repo_model.UpdateRepositoryColsNoAutoTime(ctx, repo, "trust_model"); err != nil {
			ctx.ServerError("UpdateRepositoryColsNoAutoTime", err)
			return
		}
		log.Trace("Repository signing settings updated: %s/%s", ctx.Repo.Owner.Name, repo.Name)
	}

	ctx.Flash.Success(ctx.Tr("repo.settings.update_settings_success"))
	ctx.Redirect(ctx.Repo.RepoLink + "/settings")
}

func handleSettingsPostAdmin(ctx *context.Context) {
	if !ctx.Doer.IsAdmin {
		ctx.HTTPError(http.StatusForbidden)
		return
	}

	repo := ctx.Repo.Repository
	form := web.GetForm(ctx).(*forms.RepoSettingForm)
	if repo.IsFsckEnabled != form.EnableHealthCheck {
		repo.IsFsckEnabled = form.EnableHealthCheck
		if err := repo_model.UpdateRepositoryColsNoAutoTime(ctx, repo, "is_fsck_enabled"); err != nil {
			ctx.ServerError("UpdateRepositoryColsNoAutoTime", err)
			return
		}
		log.Trace("Repository admin settings updated: %s/%s", ctx.Repo.Owner.Name, repo.Name)
	}

	ctx.Flash.Success(ctx.Tr("repo.settings.update_settings_success"))
	ctx.Redirect(ctx.Repo.RepoLink + "/settings")
}

func handleSettingsPostAdminIndex(ctx *context.Context) {
	form := web.GetForm(ctx).(*forms.RepoSettingForm)
	repo := ctx.Repo.Repository
	if !ctx.Doer.IsAdmin {
		ctx.HTTPError(http.StatusForbidden)
		return
	}

	switch form.RequestReindexType {
	case "stats":
		if err := stats.UpdateRepoIndexer(ctx.Repo.Repository); err != nil {
			ctx.ServerError("UpdateStatsRepondexer", err)
			return
		}
	case "code":
		if !setting.Indexer.RepoIndexerEnabled {
			ctx.HTTPError(http.StatusForbidden)
			return
		}
		code.UpdateRepoIndexer(ctx.Repo.Repository)
	default:
		ctx.NotFound(nil)
		return
	}

	log.Trace("Repository reindex for %s requested: %s/%s", form.RequestReindexType, ctx.Repo.Owner.Name, repo.Name)

	ctx.Flash.Success(ctx.Tr("repo.settings.reindex_requested"))
	ctx.Redirect(ctx.Repo.RepoLink + "/settings")
}

func handleSettingsPostConvert(ctx *context.Context) {
	form := web.GetForm(ctx).(*forms.RepoSettingForm)
	repo := ctx.Repo.Repository
	if !ctx.Repo.IsOwner() {
		ctx.HTTPError(http.StatusNotFound)
		return
	}
	if repo.Name != form.RepoName {
		ctx.RenderWithErr(ctx.Tr("form.enterred_invalid_repo_name"), tplSettingsOptions, nil)
		return
	}

	if !repo.IsMirror {
		ctx.HTTPError(http.StatusNotFound)
		return
	}
	repo.IsMirror = false

	if _, err := repo_service.CleanUpMigrateInfo(ctx, repo); err != nil {
		ctx.ServerError("CleanUpMigrateInfo", err)
		return
	} else if err = repo_model.DeleteMirrorByRepoID(ctx, ctx.Repo.Repository.ID); err != nil {
		ctx.ServerError("DeleteMirrorByRepoID", err)
		return
	}
	log.Trace("Repository converted from mirror to regular: %s", repo.FullName())
	ctx.Flash.Success(ctx.Tr("repo.settings.convert_succeed"))
	ctx.Redirect(repo.Link())
}

func handleSettingsPostConvertFork(ctx *context.Context) {
	form := web.GetForm(ctx).(*forms.RepoSettingForm)
	repo := ctx.Repo.Repository
	if !ctx.Repo.IsOwner() {
		ctx.HTTPError(http.StatusNotFound)
		return
	}
	if err := repo.LoadOwner(ctx); err != nil {
		ctx.ServerError("Convert Fork", err)
		return
	}
	if repo.Name != form.RepoName {
		ctx.RenderWithErr(ctx.Tr("form.enterred_invalid_repo_name"), tplSettingsOptions, nil)
		return
	}

	if !repo.IsFork {
		ctx.HTTPError(http.StatusNotFound)
		return
	}

	if !ctx.Doer.CanForkRepoIn(ctx.Repo.Owner) {
		maxCreationLimit := ctx.Repo.Owner.MaxCreationLimit()
		msg := ctx.TrN(maxCreationLimit, "repo.form.reach_limit_of_creation_1", "repo.form.reach_limit_of_creation_n", maxCreationLimit)
		ctx.Flash.Error(msg)
		ctx.Redirect(repo.Link() + "/settings")
		return
	}

	if err := repo_service.ConvertForkToNormalRepository(ctx, repo); err != nil {
		log.Error("Unable to convert repository %-v from fork. Error: %v", repo, err)
		ctx.ServerError("Convert Fork", err)
		return
	}

	log.Trace("Repository converted from fork to regular: %s", repo.FullName())
	ctx.Flash.Success(ctx.Tr("repo.settings.convert_fork_succeed"))
	ctx.Redirect(repo.Link())
}

// articleSettingsURL is where the article settings modals return to, the repository
// settings page is not reachable from the article UI.
func articleSettingsURL(ctx *context.Context) string {
	return ctx.Repo.RepoLink + "?view=article&mode=settings"
}

// ArticleTransferCandidates searches the users the current article can be transferred
// to, excluding its owner and everyone who already owns an article on the same subject.
func ArticleTransferCandidates(ctx *context.Context) {
	repo := ctx.Repo.Repository
	users, _, err := user_model.SearchUsers(ctx, user_model.SearchUserOptions{
		Actor:                    ctx.Doer,
		Keyword:                  ctx.FormTrim("q"),
		Type:                     user_model.UserTypeIndividual,
		IsActive:                 optional.Some(true),
		ExcludeUserIDs:           []int64{repo.OwnerID},
		ExcludeOwnersOfSubjectID: repo.SubjectID,
		ListOptions:              db.ListOptions{PageSize: setting.UI.MembersPagingNum},
	})
	if err != nil {
		ctx.ServerError("SearchUsers", err)
		return
	}
	ctx.JSON(http.StatusOK, map[string]any{"data": convert_service.ToUsers(ctx, ctx.Doer, users)})
}

// resolveArticleTransferRecipient finds the transfer recipient by their username, which
// the article transfer modal submits while showing the full name as the display value.
// The username is unique and indexed, so the lookup neither scans the table nor has to
// deal with two users sharing a name. Only the active users the candidate search lists
// for the doer are accepted, so that a crafted request cannot reach a hidden account or
// confirm that it exists. It reports the failure through a flash message and returns nil
// when the name is unusable.
func resolveArticleTransferRecipient(ctx *context.Context, name string) *user_model.User {
	name = strings.TrimSpace(name)
	if name == "" {
		ctx.Flash.Error(ctx.Tr("repo.settings.article_transfer_recipient_not_found"))
		return nil
	}

	recipient, err := user_model.GetUserByName(ctx, name)
	if err != nil {
		if !user_model.IsErrUserNotExist(err) {
			ctx.ServerError("GetUserByName", err)
			return nil
		}
		ctx.Flash.Error(ctx.Tr("repo.settings.article_transfer_recipient_not_found"))
		return nil
	}

	if !recipient.IsIndividual() || !recipient.IsActive || recipient.ProhibitLogin ||
		!user_model.IsUserVisibleToViewer(ctx, recipient, ctx.Doer) {
		ctx.Flash.Error(ctx.Tr("repo.settings.article_transfer_recipient_not_found"))
		return nil
	}

	if recipient.ID == ctx.Repo.Owner.ID {
		ctx.Flash.Error(ctx.Tr("repo.settings.article_transfer_recipient_is_current"))
		return nil
	}
	return recipient
}

// handleArticleSettingsPostTransfer serves the article transfer modal, which confirms
// with "<owner>/<subject>", identifies the recipient by username and always reports
// back on the article settings page.
func handleArticleSettingsPostTransfer(ctx *context.Context) {
	form := web.GetForm(ctx).(*forms.RepoSettingForm)
	repo := ctx.Repo.Repository
	redirectURL := articleSettingsURL(ctx)

	// An archived article is read-only, deletion is the only action left on it.
	if repo.IsArchived {
		archivedAt := util.Iif(repo.ArchivedUnix.IsZero(), repo.UpdatedUnix, repo.ArchivedUnix)
		ctx.Flash.Error(ctx.Tr("repo.settings.article_archived_notice", archivedAt.FormatDate()))
		ctx.Redirect(redirectURL)
		return
	}

	if form.ArticleName != ctx.Repo.Owner.Name+"/"+repo.GetSubject(ctx) {
		ctx.Flash.Error(ctx.Tr("form.enterred_invalid_article_name"))
		ctx.Redirect(redirectURL)
		return
	}

	newOwner := resolveArticleTransferRecipient(ctx, ctx.FormString("new_owner_name"))
	if newOwner == nil {
		if !ctx.Written() {
			ctx.Redirect(redirectURL)
		}
		return
	}

	// The candidate search already hides these users, but the rule has to hold for a
	// crafted request and for a recipient who acquired an article on this subject in
	// the meantime. The transfer itself only guards against a repository name clash.
	if repo.SubjectID > 0 {
		existing, err := repo_model.GetRepositoryByOwnerIDAndSubjectID(ctx, newOwner.ID, repo.SubjectID)
		if err != nil {
			ctx.ServerError("GetRepositoryByOwnerIDAndSubjectID", err)
			return
		}
		if existing != nil {
			ctx.Flash.Error(ctx.Tr("repo.settings.article_transfer_recipient_has_article"))
			ctx.Redirect(redirectURL)
			return
		}
	}

	// Close the GitRepo if open
	if ctx.Repo.GitRepo != nil {
		ctx.Repo.GitRepo.Close()
		ctx.Repo.GitRepo = nil
	}

	if err := repo_service.StartRepositoryTransfer(ctx, ctx.Doer, newOwner, repo, nil); err != nil {
		switch {
		case repo_model.IsErrRepoAlreadyExist(err):
			ctx.Flash.Error(ctx.Tr("repo.settings.article_transfer_recipient_has_article"))
		case repo_model.IsErrRepoTransferInProgress(err):
			ctx.Flash.Error(ctx.Tr("repo.settings.article_transfer_in_progress"))
		case repo_service.IsRepositoryLimitReached(err):
			limit := err.(repo_service.LimitReachedError).Limit
			ctx.Flash.Error(ctx.TrN(limit, "repo.form.reach_limit_of_creation_1", "repo.form.reach_limit_of_creation_n", limit))
		case errors.Is(err, user_model.ErrBlockedUser):
			ctx.Flash.Error(ctx.Tr("repo.settings.transfer.blocked_user"))
		default:
			ctx.ServerError("TransferOwnership", err)
			return
		}
		ctx.Redirect(redirectURL)
		return
	}

	recipientLink := htmlutil.HTMLFormat(`<a href="%s">%s</a>`, newOwner.HomeLink(), newOwner.DisplayName())
	if repo.Status == repo_model.RepositoryPendingTransfer {
		log.Trace("Article transfer process was started: %s/%s -> %s", ctx.Repo.Owner.Name, repo.Name, newOwner.Name)
		ctx.Flash.Success(ctx.Tr("repo.settings.article_transfer_started", recipientLink))
	} else {
		log.Trace("Article transferred: %s/%s -> %s", ctx.Repo.Owner.Name, repo.Name, newOwner.Name)
		ctx.Flash.Success(ctx.Tr("repo.settings.article_transfer_succeed", recipientLink))
	}
	ctx.Redirect(redirectURL)
}

func handleSettingsPostTransfer(ctx *context.Context) {
	form := web.GetForm(ctx).(*forms.RepoSettingForm)
	repo := ctx.Repo.Repository
	if !ctx.Repo.IsOwner() {
		ctx.HTTPError(http.StatusNotFound)
		return
	}

	if ctx.FormBool("redirect_to_article") {
		handleArticleSettingsPostTransfer(ctx)
		return
	}

	if repo.Name != form.RepoName {
		ctx.RenderWithErr(ctx.Tr("form.enterred_invalid_repo_name"), tplSettingsOptions, nil)
		return
	}

	newOwner, err := user_model.GetUserByName(ctx, ctx.FormString("new_owner_name"))
	if err != nil {
		if user_model.IsErrUserNotExist(err) {
			ctx.RenderWithErr(ctx.Tr("form.enterred_invalid_owner_name"), tplSettingsOptions, nil)
			return
		}
		ctx.ServerError("IsUserExist", err)
		return
	}

	if newOwner.Type == user_model.UserTypeOrganization {
		if !ctx.Doer.IsAdmin && newOwner.Visibility == structs.VisibleTypePrivate && !organization.OrgFromUser(newOwner).HasMemberWithUserID(ctx, ctx.Doer.ID) {
			// The user shouldn't know about this organization
			ctx.RenderWithErr(ctx.Tr("form.enterred_invalid_owner_name"), tplSettingsOptions, nil)
			return
		}
	}

	// Close the GitRepo if open
	if ctx.Repo.GitRepo != nil {
		ctx.Repo.GitRepo.Close()
		ctx.Repo.GitRepo = nil
	}

	oldFullname := repo.FullName()
	if err := repo_service.StartRepositoryTransfer(ctx, ctx.Doer, newOwner, repo, nil); err != nil {
		if repo_model.IsErrRepoAlreadyExist(err) {
			ctx.RenderWithErr(ctx.Tr("repo.settings.new_owner_has_same_repo"), tplSettingsOptions, nil)
		} else if repo_model.IsErrRepoTransferInProgress(err) {
			ctx.RenderWithErr(ctx.Tr("repo.settings.transfer_in_progress"), tplSettingsOptions, nil)
		} else if repo_service.IsRepositoryLimitReached(err) {
			limit := err.(repo_service.LimitReachedError).Limit
			ctx.RenderWithErr(ctx.TrN(limit, "repo.form.reach_limit_of_creation_1", "repo.form.reach_limit_of_creation_n", limit), tplSettingsOptions, nil)
		} else if errors.Is(err, user_model.ErrBlockedUser) {
			ctx.RenderWithErr(ctx.Tr("repo.settings.transfer.blocked_user"), tplSettingsOptions, nil)
		} else {
			ctx.ServerError("TransferOwnership", err)
		}

		return
	}

	if ctx.Repo.Repository.Status == repo_model.RepositoryPendingTransfer {
		log.Trace("Repository transfer process was started: %s/%s -> %s", ctx.Repo.Owner.Name, repo.Name, newOwner)
		ctx.Flash.Success(ctx.Tr("repo.settings.transfer_started", newOwner.DisplayName()))
	} else {
		log.Trace("Repository transferred: %s -> %s", oldFullname, ctx.Repo.Repository.FullName())
		ctx.Flash.Success(ctx.Tr("repo.settings.transfer_succeed"))
	}
	ctx.Redirect(repo.Link() + "/settings")
}

func handleSettingsPostCancelTransfer(ctx *context.Context) {
	repo := ctx.Repo.Repository
	if !ctx.Repo.IsOwner() {
		ctx.HTTPError(http.StatusNotFound)
		return
	}

	// The article settings UI posts to this same dispatcher, but it must return to
	// the article view instead of the repository settings page.
	fromArticle := ctx.FormBool("redirect_to_article")
	redirectURL := repo.Link() + "/settings"
	if fromArticle {
		redirectURL = articleSettingsURL(ctx)
	}

	repoTransfer, err := repo_model.GetPendingRepositoryTransfer(ctx, ctx.Repo.Repository)
	if err != nil {
		if repo_model.IsErrNoPendingTransfer(err) {
			ctx.Flash.Error(ctx.Tr("repo.settings.transfer_abort_invalid"))
			ctx.Redirect(redirectURL)
		} else {
			ctx.ServerError("GetPendingRepositoryTransfer", err)
		}
		return
	}

	if err := repo_service.CancelRepositoryTransfer(ctx, repoTransfer, ctx.Doer); err != nil {
		ctx.ServerError("CancelRepositoryTransfer", err)
		return
	}

	log.Trace("Repository transfer process was cancelled: %s/%s ", ctx.Repo.Owner.Name, repo.Name)
	if fromArticle {
		if err := repoTransfer.LoadRecipient(ctx); err != nil {
			ctx.ServerError("LoadRecipient", err)
			return
		}
		ctx.Flash.Success(ctx.Tr("repo.settings.article_transfer_abort_success", repoTransfer.Recipient.DisplayName()))
	} else {
		ctx.Flash.Success(ctx.Tr("repo.settings.transfer_abort_success", repoTransfer.Recipient.Name))
	}
	ctx.Redirect(redirectURL)
}

func handleSettingsPostDelete(ctx *context.Context) {
	form := web.GetForm(ctx).(*forms.RepoSettingForm)
	repo := ctx.Repo.Repository
	if !ctx.Repo.IsOwner() {
		ctx.HTTPError(http.StatusNotFound)
		return
	}

	// The article settings UI confirms the deletion with "<owner>/<subject>" and
	// returns to the article settings view instead of the repository settings page.
	fromArticle := ctx.FormBool("redirect_to_article")
	if fromArticle {
		if form.ArticleName != ctx.Repo.Owner.Name+"/"+repo.GetSubject(ctx) {
			ctx.Flash.Error(ctx.Tr("form.enterred_invalid_article_name"))
			ctx.Redirect(articleSettingsURL(ctx))
			return
		}
	} else if repo.Name != form.RepoName {
		ctx.RenderWithErr(ctx.Tr("form.enterred_invalid_repo_name"), tplSettingsOptions, nil)
		return
	}

	// Close the gitrepository before doing this.
	if ctx.Repo.GitRepo != nil {
		ctx.Repo.GitRepo.Close()
	}

	if err := repo_service.DeleteRepository(ctx, ctx.Doer, ctx.Repo.Repository, true); err != nil {
		ctx.ServerError("DeleteRepository", err)
		return
	}
	log.Trace("Repository deleted: %s/%s", ctx.Repo.Owner.Name, repo.Name)

	if fromArticle {
		ctx.Flash.Success(ctx.Tr("repo.settings.article_delete_success"))
	} else {
		ctx.Flash.Success(ctx.Tr("repo.settings.deletion_success"))
	}
	// The dashboard renders base/alert, so the flash above stays visible; the owner
	// profile template does not.
	ctx.Redirect(ctx.Repo.Owner.DashboardLink())
}

func handleSettingsPostDeleteWiki(ctx *context.Context) {
	form := web.GetForm(ctx).(*forms.RepoSettingForm)
	repo := ctx.Repo.Repository
	if !ctx.Repo.IsOwner() {
		ctx.HTTPError(http.StatusNotFound)
		return
	}
	if repo.Name != form.RepoName {
		ctx.RenderWithErr(ctx.Tr("form.enterred_invalid_repo_name"), tplSettingsOptions, nil)
		return
	}

	err := wiki_service.DeleteWiki(ctx, repo)
	if err != nil {
		log.Error("Delete Wiki: %v", err.Error())
	}
	log.Trace("Repository wiki deleted: %s/%s", ctx.Repo.Owner.Name, repo.Name)

	ctx.Flash.Success(ctx.Tr("repo.settings.wiki_deletion_success"))
	ctx.Redirect(ctx.Repo.RepoLink + "/settings")
}

func handleSettingsPostArchive(ctx *context.Context) {
	repo := ctx.Repo.Repository
	if !ctx.Repo.IsOwner() {
		ctx.HTTPError(http.StatusForbidden)
		return
	}

	// The article settings UI posts to this same dispatcher, but it must return to
	// the article view instead of the repository settings page.
	fromArticle := ctx.FormBool("redirect_to_article")
	redirectURL := ctx.Repo.RepoLink + "/settings"
	if fromArticle {
		redirectURL = articleSettingsURL(ctx)
	}

	if repo.IsMirror {
		ctx.Flash.Error(ctx.Tr("repo.settings.archive.error_ismirror"))
		ctx.Redirect(redirectURL)
		return
	}

	// An archived article only keeps the delete action, so a transfer that is still
	// pending must not survive archiving and be accepted by the recipient afterwards.
	if repo.SubjectID > 0 {
		repoTransfer, err := repo_model.GetPendingRepositoryTransfer(ctx, repo)
		if err != nil && !repo_model.IsErrNoPendingTransfer(err) {
			ctx.ServerError("GetPendingRepositoryTransfer", err)
			return
		}
		if repoTransfer != nil {
			if err := repo_service.CancelRepositoryTransfer(ctx, repoTransfer, ctx.Doer); err != nil {
				ctx.ServerError("CancelRepositoryTransfer", err)
				return
			}
			log.Trace("Pending article transfer cancelled on archive: %s/%s", ctx.Repo.Owner.Name, repo.Name)
		}
	}

	if err := repo_model.SetArchiveRepoState(ctx, repo, true); err != nil {
		log.Error("Tried to archive a repo: %s", err)
		ctx.Flash.Error(ctx.Tr("repo.settings.archive.error"))
		ctx.Redirect(redirectURL)
		return
	}

	if err := actions_service.CleanRepoScheduleTasks(ctx, repo); err != nil {
		log.Error("CleanRepoScheduleTasks for archived repo %s/%s: %v", ctx.Repo.Owner.Name, repo.Name, err)
	}

	// update issue indexer
	issue_indexer.UpdateRepoIndexer(ctx, repo.ID)

	if fromArticle {
		ctx.Flash.Success(ctx.Tr("repo.settings.article_archive_success"))
	} else {
		ctx.Flash.Success(ctx.Tr("repo.settings.archive.success"))
	}

	log.Trace("Repository was archived: %s/%s", ctx.Repo.Owner.Name, repo.Name)
	ctx.Redirect(redirectURL)
}

func handleSettingsPostUnarchive(ctx *context.Context) {
	repo := ctx.Repo.Repository
	if !ctx.Repo.IsOwner() {
		ctx.HTTPError(http.StatusForbidden)
		return
	}

	if err := repo_model.SetArchiveRepoState(ctx, repo, false); err != nil {
		log.Error("Tried to unarchive a repo: %s", err)
		ctx.Flash.Error(ctx.Tr("repo.settings.unarchive.error"))
		ctx.Redirect(ctx.Repo.RepoLink + "/settings")
		return
	}

	if ctx.Repo.Repository.UnitEnabled(ctx, unit_model.TypeActions) {
		if err := actions_service.DetectAndHandleSchedules(ctx, repo); err != nil {
			log.Error("DetectAndHandleSchedules for un-archived repo %s/%s: %v", ctx.Repo.Owner.Name, repo.Name, err)
		}
	}

	// update issue indexer
	issue_indexer.UpdateRepoIndexer(ctx, repo.ID)

	ctx.Flash.Success(ctx.Tr("repo.settings.unarchive.success"))

	log.Trace("Repository was un-archived: %s/%s", ctx.Repo.Owner.Name, repo.Name)
	ctx.Redirect(ctx.Repo.RepoLink + "/settings")
}

func handleSettingsPostVisibility(ctx *context.Context) {
	form := web.GetForm(ctx).(*forms.RepoSettingForm)
	repo := ctx.Repo.Repository
	if repo.IsFork {
		ctx.Flash.Error(ctx.Tr("repo.settings.visibility.fork_error"))
		ctx.Redirect(ctx.Repo.RepoLink + "/settings")
		return
	}

	var err error

	// when ForcePrivate enabled, you could change public repo to private, but only admin users can change private to public
	if setting.Repository.ForcePrivate && repo.IsPrivate && !ctx.Doer.IsAdmin {
		ctx.RenderWithErr(ctx.Tr("form.repository_force_private"), tplSettingsOptions, form)
		return
	}

	if repo.IsPrivate {
		err = repo_service.MakeRepoPublic(ctx, repo)
	} else {
		err = repo_service.MakeRepoPrivate(ctx, repo)
	}

	if err != nil {
		log.Error("Tried to change the visibility of the repo: %s", err)
		ctx.Flash.Error(ctx.Tr("repo.settings.visibility.error"))
		ctx.Redirect(ctx.Repo.RepoLink + "/settings")
		return
	}

	ctx.Flash.Success(ctx.Tr("repo.settings.visibility.success"))

	log.Trace("Repository visibility changed: %s/%s", ctx.Repo.Owner.Name, repo.Name)
	ctx.Redirect(ctx.Repo.RepoLink + "/settings")
}

func handleSettingRemoteAddrError(ctx *context.Context, err error, form *forms.RepoSettingForm) {
	if git.IsErrInvalidCloneAddr(err) {
		addrErr := err.(*git.ErrInvalidCloneAddr)
		switch {
		case addrErr.IsProtocolInvalid:
			ctx.RenderWithErr(ctx.Tr("repo.mirror_address_protocol_invalid"), tplSettingsOptions, form)
		case addrErr.IsURLError:
			ctx.RenderWithErr(ctx.Tr("form.url_error", addrErr.Host), tplSettingsOptions, form)
		case addrErr.IsPermissionDenied:
			if addrErr.LocalPath {
				ctx.RenderWithErr(ctx.Tr("repo.migrate.permission_denied"), tplSettingsOptions, form)
			} else {
				ctx.RenderWithErr(ctx.Tr("repo.migrate.permission_denied_blocked"), tplSettingsOptions, form)
			}
		case addrErr.IsInvalidPath:
			ctx.RenderWithErr(ctx.Tr("repo.migrate.invalid_local_path"), tplSettingsOptions, form)
		default:
			ctx.ServerError("Unknown error", err)
		}
		return
	}
	ctx.RenderWithErr(ctx.Tr("repo.mirror_address_url_invalid"), tplSettingsOptions, form)
}
