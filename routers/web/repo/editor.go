// Copyright 2016 The Gogs Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"bytes"
	stdctx "context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"

	"code.gitea.io/gitea/models/db"
	git_model "code.gitea.io/gitea/models/git"
	issues_model "code.gitea.io/gitea/models/issues"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unit"
	"code.gitea.io/gitea/modules/charset"
	"code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/gitrepo"
	"code.gitea.io/gitea/modules/httplib"
	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/markup"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/templates"
	"code.gitea.io/gitea/modules/util"
	"code.gitea.io/gitea/modules/web"
	"code.gitea.io/gitea/services/context"
	"code.gitea.io/gitea/services/context/upload"
	"code.gitea.io/gitea/services/forms"
	pull_service "code.gitea.io/gitea/services/pull"
	repo_service "code.gitea.io/gitea/services/repository"
	files_service "code.gitea.io/gitea/services/repository/files"
)

const (
	tplEditFile        templates.TplName = "repo/editor/edit"
	tplEditDiffPreview templates.TplName = "repo/editor/diff_preview"
	tplDeleteFile      templates.TplName = "repo/editor/delete"
	tplUploadFile      templates.TplName = "repo/editor/upload"
	tplPatchFile       templates.TplName = "repo/editor/patch"
	tplCherryPick      templates.TplName = "repo/editor/cherry_pick"

	editorCommitChoiceDirect    string = "direct"
	editorCommitChoiceNewBranch string = "commit-to-new-branch"
)

func prepareEditorCommitFormOptions(ctx *context.Context, editorAction string) *context.CommitFormOptions {
	cleanedTreePath := files_service.CleanGitTreePath(ctx.Repo.TreePath)
	if cleanedTreePath != ctx.Repo.TreePath {
		redirectTo := fmt.Sprintf("%s/%s/%s/%s", ctx.Repo.RepoLink, editorAction, util.PathEscapeSegments(ctx.Repo.BranchName), util.PathEscapeSegments(cleanedTreePath))
		if ctx.Req.URL.RawQuery != "" {
			redirectTo += "?" + ctx.Req.URL.RawQuery
		}
		ctx.Redirect(redirectTo)
		return nil
	}

	commitFormOptions, err := context.PrepareCommitFormOptions(ctx, ctx.Doer, ctx.Repo.Repository, ctx.Repo.Permission, ctx.Repo.RefFullName)
	if err != nil {
		ctx.ServerError("PrepareCommitFormOptions", err)
		return nil
	}

	// Allow README.md creation for the auto-fork feature
	if commitFormOptions.NeedFork && !strings.EqualFold(ctx.Repo.TreePath, "README.md") {
		redirectURL := fmt.Sprintf("%s/_new/%s/README.md", ctx.Repo.RepoLink, util.PathEscapeSegments(ctx.Repo.BranchName))
		ctx.Redirect(redirectURL)
		return nil
	}

	if commitFormOptions.WillSubmitToFork && !commitFormOptions.TargetRepo.CanEnableEditor() {
		ctx.Data["NotFoundPrompt"] = ctx.Locale.Tr("repo.editor.fork_not_editable")
		ctx.NotFound(nil)
	}

	ctx.Data["BranchLink"] = ctx.Repo.RepoLink + "/src/" + ctx.Repo.RefTypeNameSubURL()
	ctx.Data["TreePath"] = ctx.Repo.TreePath
	ctx.Data["CommitFormOptions"] = commitFormOptions

	// for online editor
	ctx.Data["PreviewableExtensions"] = strings.Join(markup.PreviewableExtensions(), ",")
	ctx.Data["LineWrapExtensions"] = strings.Join(setting.Repository.Editor.LineWrapExtensions, ",")
	ctx.Data["IsEditingFileOnly"] = ctx.FormString("return_uri") != ""
	ctx.Data["ReturnURI"] = ctx.FormString("return_uri")

	// form fields
	ctx.Data["commit_summary"] = ""
	ctx.Data["commit_message"] = ""
	ctx.Data["commit_choice"] = util.Iif(commitFormOptions.CanCommitToBranch, editorCommitChoiceDirect, editorCommitChoiceNewBranch)
	targetRepo := commitFormOptions.TargetRepo
	if targetRepo == nil && commitFormOptions.NeedFork {
		targetRepo = ctx.Repo.Repository
		commitFormOptions.TargetRepo = targetRepo
	}
	ctx.Data["new_branch_name"] = getUniquePatchBranchName(ctx, ctx.Doer.LowerName, targetRepo)
	ctx.Data["last_commit"] = ctx.Repo.CommitID
	return commitFormOptions
}

func prepareTreePathFieldsAndPaths(ctx *context.Context, treePath string) {
	// show the tree path fields in the "breadcrumb" and help users to edit the target tree path
	ctx.Data["TreeNames"], ctx.Data["TreePaths"] = getParentTreeFields(strings.TrimPrefix(treePath, "/"))
}

type preparedEditorCommitForm[T any] struct {
	form              T
	commonForm        *forms.CommitCommonForm
	CommitFormOptions *context.CommitFormOptions
	OldBranchName     string
	NewBranchName     string
	GitCommitter      *files_service.IdentityOptions
}

func (f *preparedEditorCommitForm[T]) GetCommitMessage(defaultCommitMessage string) string {
	commitMessage := util.IfZero(strings.TrimSpace(f.commonForm.CommitSummary), defaultCommitMessage)
	if body := strings.TrimSpace(f.commonForm.CommitMessage); body != "" {
		commitMessage += "\n\n" + body
	}
	return commitMessage
}

func prepareEditorCommitSubmittedForm[T forms.CommitCommonFormInterface](ctx *context.Context) *preparedEditorCommitForm[T] {
	form := web.GetForm(ctx).(T)
	if ctx.HasError() {
		ctx.JSONError(ctx.GetErrMsg())
		return nil
	}

	commonForm := form.GetCommitCommonForm()
	commonForm.TreePath = files_service.CleanGitTreePath(commonForm.TreePath)

	commitFormOptions, err := context.PrepareCommitFormOptions(ctx, ctx.Doer, ctx.Repo.Repository, ctx.Repo.Permission, ctx.Repo.RefFullName)
	if err != nil {
		ctx.ServerError("PrepareCommitFormOptions", err)
		return nil
	}
	if commitFormOptions.NeedFork && !strings.EqualFold(commonForm.TreePath, "README.md") {
		// It shouldn't happen, because we should have done the checks in the "GET" request. But just in case.
		ctx.JSONError(ctx.Locale.TrString("error.not_found"))
		return nil
	}

	// check commit behavior
	fromBaseBranch := ctx.FormString("from_base_branch")
	commitToNewBranch := commonForm.CommitChoice == editorCommitChoiceNewBranch || fromBaseBranch != ""
	targetBranchName := util.Iif(commitToNewBranch, commonForm.NewBranchName, ctx.Repo.BranchName)
	if targetBranchName == ctx.Repo.BranchName && !commitFormOptions.CanCommitToBranch && !commitFormOptions.NeedFork {
		ctx.JSONError(ctx.Tr("repo.editor.cannot_commit_to_protected_branch", targetBranchName))
		return nil
	}

	if !commitFormOptions.NeedFork && !issues_model.CanMaintainerWriteToBranch(ctx, ctx.Repo.Permission, targetBranchName, ctx.Doer) {
		ctx.NotFound(nil)
		return nil
	}

	// Committer user info
	gitCommitter, valid := WebGitOperationGetCommitChosenEmailIdentity(ctx, commonForm.CommitEmail)
	if !valid {
		ctx.JSONError(ctx.Tr("repo.editor.invalid_commit_email"))
		return nil
	}

	if commitToNewBranch {
		// if target branch exists, we should stop
		targetRepo := commitFormOptions.TargetRepo
		if targetRepo == nil && commitFormOptions.NeedFork {
			targetRepo = ctx.Repo.Repository
			commitFormOptions.TargetRepo = targetRepo
		}
		targetBranchExists, err := git_model.IsBranchExist(ctx, targetRepo.ID, targetBranchName)
		if err != nil {
			ctx.ServerError("IsBranchExist", err)
			return nil
		} else if targetBranchExists {
			if fromBaseBranch != "" {
				ctx.JSONError(ctx.Tr("repo.editor.fork_branch_exists", targetBranchName))
			} else {
				ctx.JSONError(ctx.Tr("repo.editor.branch_already_exists", targetBranchName))
			}
			return nil
		}
	}

	oldBranchName := ctx.Repo.BranchName
	if fromBaseBranch != "" && !commitFormOptions.NeedFork {
		err = editorPushBranchToForkedRepository(ctx, ctx.Doer, ctx.Repo.Repository.BaseRepo, fromBaseBranch, commitFormOptions.TargetRepo, targetBranchName)
		if err != nil {
			log.Error("Unable to editorPushBranchToForkedRepository: %v", err)
			ctx.JSONError(ctx.Tr("repo.editor.fork_failed_to_push_branch", targetBranchName))
			return nil
		}
		// we have pushed the base branch as the new branch, now we need to commit the changes directly to the new branch
		oldBranchName = targetBranchName
	}

	return &preparedEditorCommitForm[T]{
		form:              form,
		commonForm:        commonForm,
		CommitFormOptions: commitFormOptions,
		OldBranchName:     oldBranchName,
		NewBranchName:     targetBranchName,
		GitCommitter:      gitCommitter,
	}
}

// redirectForCommitChoice redirects after committing the edit to a branch
func redirectForCommitChoice[T any](ctx *context.Context, parsed *preparedEditorCommitForm[T], treePath string) {
	// when editing a file in a PR, it should return to the origin location
	if returnURI := ctx.FormString("return_uri"); returnURI != "" && httplib.IsCurrentGiteaSiteURL(ctx, returnURI) {
		ctx.JSONRedirect(returnURI)
		return
	}

	if parsed.commonForm.CommitChoice == editorCommitChoiceNewBranch {
		// Redirect to a pull request when possible
		redirectToPullRequest := false
		repo, baseBranch, headBranch := ctx.Repo.Repository, parsed.OldBranchName, parsed.NewBranchName
		if ctx.Repo.Repository.IsFork && parsed.CommitFormOptions.CanCreateBasePullRequest {
			redirectToPullRequest = true
			baseBranch = repo.BaseRepo.DefaultBranch
			headBranch = repo.Owner.Name + "/" + repo.Name + ":" + headBranch
			repo = repo.BaseRepo
		} else if repo.UnitEnabled(ctx, unit.TypePullRequests) {
			redirectToPullRequest = true
		}
		if redirectToPullRequest {
			ctx.JSONRedirect(repo.Link() + "/compare/" + util.PathEscapeSegments(baseBranch) + "..." + util.PathEscapeSegments(headBranch))
			return
		}
	}

	// Check if the request came from an edit to the article route
	// If so, redirect to the article view instead of the file view
	articleEditPath := path.Join("_edit", parsed.NewBranchName, treePath)
	if strings.Contains(ctx.Req.URL.Path, articleEditPath) {
		ctx.JSONRedirect(ctx.Repo.RepoLink)
		return
	}

	// redirect to the newly updated file
	redirectTo := util.URLJoin(ctx.Repo.RepoLink, "src/branch", util.PathEscapeSegments(parsed.NewBranchName), util.PathEscapeSegments(treePath))
	ctx.JSONRedirect(redirectTo)
}

func editFileOpenExisting(ctx *context.Context) (prefetch []byte, dataRc io.ReadCloser, fInfo *fileInfo) {
	entry, err := ctx.Repo.Commit.GetTreeEntryByPath(ctx.Repo.TreePath)
	if err != nil {
		HandleGitError(ctx, "GetTreeEntryByPath", err)
		return nil, nil, nil
	}

	// No way to edit a directory online.
	if entry.IsDir() {
		ctx.NotFound(nil)
		return nil, nil, nil
	}

	blob := entry.Blob()
	buf, dataRc, fInfo, err := getFileReader(ctx, ctx.Repo.Repository.ID, blob)
	if err != nil {
		if git.IsErrNotExist(err) {
			ctx.NotFound(err)
		} else {
			ctx.ServerError("getFileReader", err)
		}
		return nil, nil, nil
	}

	if fInfo.isLFSFile() {
		lfsLock, err := git_model.GetTreePathLock(ctx, ctx.Repo.Repository.ID, ctx.Repo.TreePath)
		if err != nil {
			_ = dataRc.Close()
			ctx.ServerError("GetTreePathLock", err)
			return nil, nil, nil
		} else if lfsLock != nil && lfsLock.OwnerID != ctx.Doer.ID {
			_ = dataRc.Close()
			ctx.NotFound(nil)
			return nil, nil, nil
		}
	}

	return buf, dataRc, fInfo
}

func EditFile(ctx *context.Context) {
	editorAction := ctx.PathParam("editor_action")
	isNewFile := editorAction == "_new"
	ctx.Data["IsNewFile"] = isNewFile

	// Check if the filename (and additional path) is specified in the querystring
	// (filename is a misnomer, but kept for compatibility with GitHub)
	urlQuery := ctx.Req.URL.Query()
	queryFilename := urlQuery.Get("filename")
	if queryFilename != "" {
		newTreePath := path.Join(ctx.Repo.TreePath, queryFilename)
		redirectTo := fmt.Sprintf("%s/%s/%s/%s", ctx.Repo.RepoLink, editorAction, util.PathEscapeSegments(ctx.Repo.BranchName), util.PathEscapeSegments(newTreePath))
		urlQuery.Del("filename")
		if newQueryParams := urlQuery.Encode(); newQueryParams != "" {
			redirectTo += "?" + newQueryParams
		}
		ctx.Redirect(redirectTo)
		return
	}

	// on the "New File" page, we should add an empty path field to make end users could input a new name
	prepareTreePathFieldsAndPaths(ctx, util.Iif(isNewFile, ctx.Repo.TreePath+"/", ctx.Repo.TreePath))

	prepareEditorCommitFormOptions(ctx, editorAction)
	if ctx.Written() {
		return
	}

	// Check if this is creating the first article (README.md in empty repo)
	treePath := strings.Trim(ctx.Repo.TreePath, "/")
	fileName := strings.ToLower(path.Base(treePath))
	isCreatingFirstArticle := isNewFile && ctx.Repo.Repository.IsEmpty && (fileName == "readme.md" || strings.EqualFold(treePath, "readme.md"))
	ctx.Data["IsCreatingFirstArticle"] = isCreatingFirstArticle

	if !isNewFile {
		prefetch, dataRc, fInfo := editFileOpenExisting(ctx)
		if ctx.Written() {
			return
		}
		defer dataRc.Close()

		ctx.Data["FileSize"] = fInfo.fileSize

		// Only some file types are editable online as text.
		if fInfo.isLFSFile() {
			ctx.Data["NotEditableReason"] = ctx.Tr("repo.editor.cannot_edit_lfs_files")
		} else if !fInfo.st.IsRepresentableAsText() {
			ctx.Data["NotEditableReason"] = ctx.Tr("repo.editor.cannot_edit_non_text_files")
		} else if fInfo.fileSize >= setting.UI.MaxDisplayFileSize {
			ctx.Data["NotEditableReason"] = ctx.Tr("repo.editor.cannot_edit_too_large_file")
		}

		if ctx.Data["NotEditableReason"] == nil {
			buf, err := io.ReadAll(io.MultiReader(bytes.NewReader(prefetch), dataRc))
			if err != nil {
				ctx.ServerError("ReadAll", err)
				return
			}
			if content, err := charset.ToUTF8(buf, charset.ConvertOpts{KeepBOM: true}); err != nil {
				ctx.Data["FileContent"] = string(buf)
			} else {
				ctx.Data["FileContent"] = content
			}
		}
	}

	ctx.Data["EditorconfigJson"] = getContextRepoEditorConfig(ctx, ctx.Repo.TreePath)
	ctx.HTML(http.StatusOK, tplEditFile)
}

func EditFilePost(ctx *context.Context) {
	editorAction := ctx.PathParam("editor_action")
	isNewFile := editorAction == "_new"
	parsed := prepareEditorCommitSubmittedForm[*forms.EditRepoFileForm](ctx)
	if ctx.Written() {
		return
	}

	// Skip the NeedFork workflow if ForkAndEdit or SubmitChangeRequest is true
	// The ForkAndEdit workflow (handled later) will create the fork
	// The SubmitChangeRequest workflow creates a branch in the target repo directly (no fork)
	if parsed.CommitFormOptions.NeedFork && !parsed.form.ForkAndEdit && !parsed.form.SubmitChangeRequest {
		baseRepo := ctx.Repo.Repository
		repoName := getUniqueRepositoryName(ctx, ctx.Doer.ID, baseRepo.Name)
		if repoName == "" {
			ctx.ServerError("getUniqueRepositoryName", errors.New("failed to generate unique repository name"))
			return
		}
		forkedRepo := ForkRepoTo(ctx, ctx.Doer, repo_service.ForkRepoOptions{
			BaseRepo:     baseRepo,
			Name:         repoName,
			Description:  baseRepo.Description,
			SingleBranch: baseRepo.DefaultBranch,
		})
		if ctx.Written() {
			return
		}

		// Check if the base repository has a README
		// If not, we should promote the new fork to be the root repository
		// This is to handle the case where a user creates a subject (empty repo) and another user
		// contributes the first content (README). The contributor should become the owner of the "main" repo.
		hasReadme := false
		if !baseRepo.IsEmpty {
			// Check for README in the default branch
			gitRepo, err := gitrepo.OpenRepository(ctx, baseRepo)
			if err != nil {
				log.Error("OpenRepository failed: %v", err)
			} else {
				defer gitRepo.Close()
				commit, err := gitRepo.GetBranchCommit(baseRepo.DefaultBranch)
				if err != nil {
					log.Error("GetBranchCommit failed: %v", err)
				} else {
					entry, err := commit.GetTreeEntryByPath("README.md")
					if err == nil && entry != nil {
						hasReadme = true
					} else {
						// Try other common names
						entry, err = commit.GetTreeEntryByPath("readme.md")
						if err == nil && entry != nil {
							hasReadme = true
						}
					}
				}
			}
		}

		if !hasReadme {
			// Swap fork status atomically in a transaction
			err := db.WithTx(ctx, func(txCtx stdctx.Context) error {
				// 1. Promote forkedRepo to root
				forkedRepo.IsFork = false
				forkedRepo.ForkID = 0
				if err := repo_model.UpdateRepositoryColsNoAutoTime(txCtx, forkedRepo, "is_fork", "fork_id"); err != nil {
					return fmt.Errorf("failed to update forked repo to root: %w", err)
				}

				// 2. Demote baseRepo to fork
				baseRepo.IsFork = true
				baseRepo.ForkID = forkedRepo.ID
				if err := repo_model.UpdateRepositoryColsNoAutoTime(txCtx, baseRepo, "is_fork", "fork_id"); err != nil {
					return fmt.Errorf("failed to update base repo to fork: %w", err)
				}

				// 3. Update NumForks counters
				// forkedRepo is no longer a fork of baseRepo, so decrement baseRepo's count
				if err := repo_model.DecrementRepoForkNum(txCtx, baseRepo.ID); err != nil {
					return fmt.Errorf("failed to decrement fork count on old root: %w", err)
				}
				// baseRepo is now a fork of forkedRepo, so increment forkedRepo's count
				if err := repo_model.IncrementRepoForkNum(txCtx, forkedRepo.ID); err != nil {
					return fmt.Errorf("failed to increment fork count on new root: %w", err)
				}

				return nil
			})
			if err != nil {
				log.Error("Failed to swap fork status: %v", err)
				ctx.ServerError("SwapForkStatus", err)
				return
			}

			// If base repo was empty, the fork is also empty.
			// We should commit to the default branch instead of a patch branch.
			if baseRepo.IsEmpty {
				parsed.NewBranchName = forkedRepo.DefaultBranch
				if parsed.NewBranchName == "" {
					parsed.NewBranchName = setting.Repository.DefaultBranch
				}
				parsed.OldBranchName = ""
			}
		}

		ctx.Repo.Repository = forkedRepo
		ctx.Repo.Owner = ctx.Doer
		ctx.Repo.RepoLink = forkedRepo.Link()
	}

	defaultCommitMessage := util.Iif(isNewFile, ctx.Locale.TrString("repo.editor.add", parsed.form.TreePath), ctx.Locale.TrString("repo.editor.update", parsed.form.TreePath))

	var operation string
	if isNewFile {
		operation = "create"
	} else if parsed.form.Content.Has() {
		// The form content only has data if the file is representable as text, is not too large and not in lfs.
		operation = "update"
	} else if ctx.Repo.TreePath != parsed.form.TreePath {
		// If it doesn't have data, the only possible operation is a "rename"
		operation = "rename"
	} else {
		// It should never happen, just in case
		ctx.JSONError(ctx.Tr("error.occurred"))
		return
	}

	// Validate mutually exclusive workflow flags
	// Both cannot be true simultaneously - this is a security check in case JavaScript fails
	if parsed.form.ForkAndEdit && parsed.form.SubmitChangeRequest {
		ctx.JSONError(ctx.Tr("error.occurred"))
		return
	}

	// Handle submit-change-request workflow (fork + branch + commit + PR)
	if parsed.form.SubmitChangeRequest {
		pr := handleSubmitChangeRequest(ctx, parsed.form, parsed)
		if ctx.Written() || pr == nil {
			return
		}
		// Redirect to the created pull request
		ctx.JSONRedirect(pr.Issue.Link())
		return
	}

	// Determine target repository - either the original or a fork
	targetRepo := ctx.Repo.Repository

	// Handle fork-and-edit workflow
	if parsed.form.ForkAndEdit {
		targetRepo = handleForkAndEdit(ctx)
		if ctx.Written() {
			return
		}
	}

	// Check if this is the first content being added to an empty repository with a subject
	// We need to capture this before the commit because the commit will mark the repo as non-empty
	wasEmpty := ctx.Repo.Repository.IsEmpty
	subjectID := ctx.Repo.Repository.SubjectID
	isNotFork := !ctx.Repo.Repository.IsFork

	_, err := files_service.ChangeRepoFiles(ctx, targetRepo, ctx.Doer, &files_service.ChangeRepoFilesOptions{
		LastCommitID: parsed.form.LastCommit,
		OldBranch:    parsed.OldBranchName,
		NewBranch:    parsed.NewBranchName,
		Message:      parsed.GetCommitMessage(defaultCommitMessage),
		Files: []*files_service.ChangeRepoFile{
			{
				Operation:     operation,
				FromTreePath:  ctx.Repo.TreePath,
				TreePath:      parsed.form.TreePath,
				ContentReader: strings.NewReader(strings.ReplaceAll(parsed.form.Content.Value(), "\r", "")),
			},
		},
		Signoff:   parsed.form.Signoff,
		Author:    parsed.GitCommitter,
		Committer: parsed.GitCommitter,
	})
	if err != nil {
		editorHandleFileOperationError(ctx, parsed.NewBranchName, err)
		return
	}

	// First-article-becomes-root logic:
	// If this was an empty repository with a subject, and it's not already a fork,
	// check if there's already a root repository for this subject.
	// If so, convert this repository to a fork of the root.
	if wasEmpty && subjectID > 0 && isNotFork && isNewFile {
		handleFirstArticleBecomesRoot(ctx, subjectID)
	}

	// If we committed to a fork, redirect to the fork's article page
	if parsed.form.ForkAndEdit && targetRepo != nil {
		ctx.JSONRedirect(targetRepo.Link() + "?mode=read")
		return
	}

	redirectForCommitChoice(ctx, parsed, parsed.form.TreePath)
}

// handleForkAndEdit handles the fork-and-edit workflow
// It returns the fork repository to commit to, or nil if an error occurred
func handleForkAndEdit(ctx *context.Context) *repo_model.Repository {
	originalRepo := ctx.Repo.Repository

	// Prevent bypassing UI restrictions
	perms, err := repo_service.CheckForkOnEditPermissions(ctx, ctx.Doer, originalRepo)
	if err != nil {
		ctx.ServerError("CheckForkOnEditPermissions", err)
		return nil
	}

	// Block if user already owns a different repository for the same subject
	if perms.BlockedBySubject {
		ctx.JSONError(ctx.Tr("repo.fork.already_own_subject_repo"))
		return nil
	}

	// Return existing fork if user already has one
	if perms.HasExistingFork && perms.ExistingFork != nil {
		return perms.ExistingFork
	}

	// Create a new fork
	forkName := getUniqueRepositoryName(ctx, ctx.Doer.ID, originalRepo.Name)
	if forkName == "" {
		ctx.JSONError(ctx.Tr("repo.fork.failed"))
		return nil
	}

	fork := ForkRepoTo(ctx, ctx.Doer, repo_service.ForkRepoOptions{
		BaseRepo:     originalRepo,
		Name:         forkName,
		Description:  originalRepo.Description,
		SingleBranch: originalRepo.DefaultBranch,
	})
	if ctx.Written() {
		return nil
	}

	return fork
}

// handleSubmitChangeRequest handles the submit-change-request workflow for article contributions.
// It creates a unique branch in the target repository, commits the changes, and creates a change request
// from that branch to the default branch (same-repo CR, no fork involved).
// Returns the created change request, or nil if an error occurred.
func handleSubmitChangeRequest(ctx *context.Context, form *forms.EditRepoFileForm, parsed *preparedEditorCommitForm[*forms.EditRepoFileForm]) *issues_model.PullRequest {
	// Verify user is authenticated (defense-in-depth, middleware should already handle this)
	if ctx.Doer == nil {
		ctx.JSONError(ctx.Tr("error.not_found"))
		return nil
	}

	targetRepo := ctx.Repo.Repository

	// Verify user has permission to submit change requests
	// This checks: not repo owner, not blocked by subject ownership, etc.
	perms, err := repo_service.CheckForkOnEditPermissions(ctx, ctx.Doer, targetRepo)
	if err != nil {
		ctx.ServerError("CheckForkOnEditPermissions", err)
		return nil
	}

	// Prevent users from submitting change requests to their own repository
	if perms.IsRepoOwner {
		ctx.JSONError(ctx.Tr("repo.editor.cannot_submit_change_request_to_own_repo"))
		return nil
	}

	// Block users who own an independent article for this subject
	if perms.BlockedBySubject {
		ctx.JSONError(ctx.Tr("repo.fork.already_own_subject_repo"))
		return nil
	}

	// Verify user can actually submit change requests
	if !perms.CanSubmitChangeRequest {
		ctx.JSONError(ctx.Tr("repo.pulls.disabled"))
		return nil
	}

	// Check if the repository allows pull requests
	if !targetRepo.AllowsPulls(ctx) {
		ctx.JSONError(ctx.Tr("repo.pulls.disabled"))
		return nil
	}

	// Generate a unique branch name for the change request
	branchName := getUniquePatchBranchName(ctx, ctx.Doer.LowerName, targetRepo)
	if branchName == "" {
		ctx.JSONError(ctx.Tr("repo.editor.cannot_create_branch"))
		return nil
	}

	// Validate that content is provided and is not empty/whitespace-only
	if !form.Content.Has() || strings.TrimSpace(form.Content.Value()) == "" {
		ctx.JSONError(ctx.Tr("repo.editor.content_required"))
		return nil
	}

	// Commit the changes to a new branch in the target repository
	// The ChangeRepoFiles function will create the new branch from the default branch
	// We use InternalPush to skip pre-receive hooks since this is a programmatic operation
	// where we've already verified the user can submit change requests (via middleware)
	defaultCommitMessage := ctx.Locale.TrString("repo.editor.update", form.TreePath)
	_, err = files_service.ChangeRepoFiles(ctx, targetRepo, ctx.Doer, &files_service.ChangeRepoFilesOptions{
		LastCommitID: form.LastCommit,
		OldBranch:    targetRepo.DefaultBranch,
		NewBranch:    branchName,
		Message:      parsed.GetCommitMessage(defaultCommitMessage),
		Files: []*files_service.ChangeRepoFile{
			{
				Operation:     "update",
				FromTreePath:  ctx.Repo.TreePath,
				TreePath:      form.TreePath,
				ContentReader: strings.NewReader(strings.ReplaceAll(form.Content.Value(), "\r", "")),
			},
		},
		Signoff:      form.Signoff,
		Author:       parsed.GitCommitter,
		Committer:    parsed.GitCommitter,
		InternalPush: true,
	})
	if err != nil {
		log.Error("handleSubmitChangeRequest: failed to commit changes: %v", err)
		editorHandleFileOperationError(ctx, branchName, err)
		return nil
	}

	// Get compare info for the pull request
	gitRepo, err := gitrepo.OpenRepository(ctx, targetRepo)
	if err != nil {
		log.Error("handleSubmitChangeRequest: failed to open git repo: %v", err)
		// Attempt to clean up the orphaned branch - need to open repo specifically for cleanup
		if cleanupRepo, cleanupErr := gitrepo.OpenRepository(ctx, targetRepo); cleanupErr == nil {
			if delErr := repo_service.DeleteBranch(ctx, ctx.Doer, targetRepo, cleanupRepo, branchName, nil); delErr != nil {
				log.Error("handleSubmitChangeRequest: failed to cleanup branch %s: %v", branchName, delErr)
			}
			cleanupRepo.Close()
		} else {
			log.Error("handleSubmitChangeRequest: failed to open repo for branch cleanup: %v", cleanupErr)
		}
		ctx.ServerError("OpenRepository", err)
		return nil
	}
	defer gitRepo.Close()

	// Same-repo CR: both head and base are in the target repository
	compareInfo, err := pull_service.GetCompareInfo(ctx, targetRepo, targetRepo, gitRepo,
		git.BranchPrefix+targetRepo.DefaultBranch, git.BranchPrefix+branchName, false, false)
	if err != nil {
		log.Error("handleSubmitChangeRequest: failed to get compare info: %v", err)
		// Attempt to clean up the orphaned branch
		if delErr := repo_service.DeleteBranch(ctx, ctx.Doer, targetRepo, gitRepo, branchName, nil); delErr != nil {
			log.Error("handleSubmitChangeRequest: failed to cleanup branch %s: %v", branchName, delErr)
		}
		ctx.ServerError("GetCompareInfo", err)
		return nil
	}

	// Create the change request
	// Use custom title if provided, otherwise generate a title based on the file being edited
	prTitle := util.IfZero(strings.TrimSpace(form.ChangeRequestTitle), ctx.Locale.TrString("repo.editor.submit_changes_pr_title", path.Base(form.TreePath)))
	// Enforce maximum PR title length (255 characters) to prevent excessively long titles.
	// Use rune-based truncation to avoid corrupting multi-byte UTF-8 characters.
	prTitle = util.TruncateRunes(prTitle, 255)
	prContent := strings.TrimSpace(form.ChangeRequestDescription)

	pullIssue := &issues_model.Issue{
		RepoID:   targetRepo.ID,
		Repo:     targetRepo,
		Title:    prTitle,
		PosterID: ctx.Doer.ID,
		Poster:   ctx.Doer,
		IsPull:   true,
		Content:  prContent,
	}

	// Same-repo CR: HeadRepo and BaseRepo are both the target repository
	changeRequest := &issues_model.PullRequest{
		HeadRepoID: targetRepo.ID,
		BaseRepoID: targetRepo.ID,
		HeadBranch: branchName,
		BaseBranch: targetRepo.DefaultBranch,
		HeadRepo:   targetRepo,
		BaseRepo:   targetRepo,
		MergeBase:  compareInfo.MergeBase,
		Type:       issues_model.PullRequestGitea,
	}

	prOpts := &pull_service.NewPullRequestOptions{
		Repo:        targetRepo,
		Issue:       pullIssue,
		PullRequest: changeRequest,
		// AllowNonCollaborator: The user was already authorized to submit change requests
		// by the CanSubmitChangeRequest middleware check. This bypasses the collaborator
		// check since the user created the patch branch programmatically (not via git push).
		AllowNonCollaborator: true,
	}

	if err := pull_service.NewPullRequest(ctx, prOpts); err != nil {
		log.Error("handleSubmitChangeRequest: failed to create change request: %v", err)
		// Attempt to clean up the orphaned branch
		if delErr := repo_service.DeleteBranch(ctx, ctx.Doer, targetRepo, gitRepo, branchName, nil); delErr != nil {
			log.Error("handleSubmitChangeRequest: failed to cleanup branch %s: %v", branchName, delErr)
		}
		ctx.ServerError("NewPullRequest", err)
		return nil
	}

	log.Info("handleSubmitChangeRequest: created CR #%d from %s to %s in %s/%s",
		changeRequest.Index,
		branchName,
		targetRepo.DefaultBranch,
		targetRepo.OwnerName, targetRepo.Name)

	return changeRequest
}

// DeleteFile render delete file page
func DeleteFile(ctx *context.Context) {
	prepareEditorCommitFormOptions(ctx, "_delete")
	if ctx.Written() {
		return
	}
	ctx.Data["PageIsDelete"] = true
	ctx.HTML(http.StatusOK, tplDeleteFile)
}

// DeleteFilePost response for deleting file
func DeleteFilePost(ctx *context.Context) {
	parsed := prepareEditorCommitSubmittedForm[*forms.DeleteRepoFileForm](ctx)
	if ctx.Written() {
		return
	}

	treePath := ctx.Repo.TreePath
	_, err := files_service.ChangeRepoFiles(ctx, ctx.Repo.Repository, ctx.Doer, &files_service.ChangeRepoFilesOptions{
		LastCommitID: parsed.form.LastCommit,
		OldBranch:    parsed.OldBranchName,
		NewBranch:    parsed.NewBranchName,
		Files: []*files_service.ChangeRepoFile{
			{
				Operation: "delete",
				TreePath:  treePath,
			},
		},
		Message:   parsed.GetCommitMessage(ctx.Locale.TrString("repo.editor.delete", treePath)),
		Signoff:   parsed.form.Signoff,
		Author:    parsed.GitCommitter,
		Committer: parsed.GitCommitter,
	})
	if err != nil {
		editorHandleFileOperationError(ctx, parsed.NewBranchName, err)
		return
	}

	ctx.Flash.Success(ctx.Tr("repo.editor.file_delete_success", treePath))
	redirectTreePath := getClosestParentWithFiles(ctx.Repo.GitRepo, parsed.NewBranchName, treePath)
	redirectForCommitChoice(ctx, parsed, redirectTreePath)
}

func UploadFile(ctx *context.Context) {
	ctx.Data["PageIsUpload"] = true
	prepareTreePathFieldsAndPaths(ctx, ctx.Repo.TreePath)
	opts := prepareEditorCommitFormOptions(ctx, "_upload")
	if ctx.Written() {
		return
	}
	upload.AddUploadContextForRepo(ctx, opts.TargetRepo)

	ctx.HTML(http.StatusOK, tplUploadFile)
}

func UploadFilePost(ctx *context.Context) {
	parsed := prepareEditorCommitSubmittedForm[*forms.UploadRepoFileForm](ctx)
	if ctx.Written() {
		return
	}

	defaultCommitMessage := ctx.Locale.TrString("repo.editor.upload_files_to_dir", util.IfZero(parsed.form.TreePath, "/"))
	err := files_service.UploadRepoFiles(ctx, ctx.Repo.Repository, ctx.Doer, &files_service.UploadRepoFileOptions{
		LastCommitID: parsed.form.LastCommit,
		OldBranch:    parsed.OldBranchName,
		NewBranch:    parsed.NewBranchName,
		TreePath:     parsed.form.TreePath,
		Message:      parsed.GetCommitMessage(defaultCommitMessage),
		Files:        parsed.form.Files,
		Signoff:      parsed.form.Signoff,
		Author:       parsed.GitCommitter,
		Committer:    parsed.GitCommitter,
	})
	if err != nil {
		editorHandleFileOperationError(ctx, parsed.NewBranchName, err)
		return
	}
	redirectForCommitChoice(ctx, parsed, parsed.form.TreePath)
}

// handleFirstArticleBecomesRoot handles the first-article-becomes-root logic.
// When a user commits content to an empty repository with a subject, we need to check
// if there's already a root repository for that subject. If so, this repository should
// become a fork of the root. If not, this repository becomes the root.
//
// IMPORTANT: We exclude the current repository from the search because at this point,
// the current repository has already been marked as non-empty (the file was just committed).
// If we don't exclude it, and the current repo was created before other repos, it would
// incorrectly be returned as the "root" even though another repo may have had content
// committed first.
func handleFirstArticleBecomesRoot(ctx *context.Context, subjectID int64) {
	// Check if there's already a root repository for this subject, EXCLUDING the current repository.
	// This ensures we find repos that had content committed BEFORE the current one.
	rootRepo, err := repo_model.GetSubjectRootRepositoryExcluding(ctx, subjectID, ctx.Repo.Repository.ID)
	if err != nil {
		if repo_model.IsErrRepoNotExist(err) {
			// No other root exists - this repository becomes the root (it's already not a fork)
			log.Info("Repository %s/%s becomes the root for subject ID %d (first article submitted)",
				ctx.Repo.Repository.OwnerName, ctx.Repo.Repository.Name, subjectID)
			return
		}
		log.Error("handleFirstArticleBecomesRoot: failed to get root repository: %v", err)
		return
	}

	// A root already exists - convert this repository to a fork of the root
	// Use ConvertNormalToForkRepository which includes fork tree limit checks
	log.Info("Converting repository %s/%s to fork of root %s/%s for subject ID %d",
		ctx.Repo.Repository.OwnerName, ctx.Repo.Repository.Name,
		rootRepo.OwnerName, rootRepo.Name, subjectID)

	if err := repo_service.ConvertNormalToForkRepository(ctx, ctx.Repo.Repository, rootRepo.ID); err != nil {
		if repo_model.IsErrForkTreeTooLarge(err) {
			log.Warn("handleFirstArticleBecomesRoot: fork tree limit reached for subject ID %d, repository %s/%s will remain as root",
				subjectID, ctx.Repo.Repository.OwnerName, ctx.Repo.Repository.Name)
			return
		}
		log.Error("handleFirstArticleBecomesRoot: failed to convert to fork: %v", err)
		return
	}

	// Update local copy to reflect the change
	ctx.Repo.Repository.IsFork = true
	ctx.Repo.Repository.ForkID = rootRepo.ID
}
