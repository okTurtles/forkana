// Copyright 2025 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"errors"
	"net/http"
	"strings"

	access_model "code.gitea.io/gitea/models/perm/access"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unit"
	"code.gitea.io/gitea/modules/gitrepo"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/services/context"
	"code.gitea.io/gitea/services/gitdiff"

	"github.com/sergi/go-diff/diffmatchpatch"
)

// readmeFileNames is the list of README file names to search for, in priority order
var readmeFileNames = []string{
	"README.md",
	"readme.md",
	"Readme.md",
	"README.MD",
	"README",
	"readme",
	"README.txt",
	"readme.txt",
}

// CompareReadme shows a diff between README files from two repos in the same subject
func CompareReadme(ctx *context.Context) {
	subjectName := ctx.PathParam("subjectname")
	ownerParams := ctx.PathParam("owners")

	// Parse owner1...owner2 format
	owner1, owner2, err := parseOwnerParams(ownerParams)
	if err != nil {
		ctx.NotFound(err)
		return
	}

	// Prevent self-comparison
	if strings.EqualFold(owner1, owner2) {
		ctx.NotFound(errors.New("cannot compare repository with itself"))
		return
	}

	// Get subject
	subject, err := repo_model.GetSubjectByName(ctx, subjectName)
	if err != nil {
		if repo_model.IsErrSubjectNotExist(err) {
			ctx.NotFound(err)
		} else {
			ctx.ServerError("GetSubjectByName", err)
		}
		return
	}

	// Get both repositories
	repo1, err := repo_model.GetRepositoryByOwnerAndSubject(ctx, owner1, subjectName)
	if err != nil {
		if repo_model.IsErrRepoNotExist(err) || repo_model.IsErrSubjectNotExist(err) {
			ctx.NotFound(err)
		} else {
			ctx.ServerError("GetRepositoryByOwnerAndSubject (repo1)", err)
		}
		return
	}

	repo2, err := repo_model.GetRepositoryByOwnerAndSubject(ctx, owner2, subjectName)
	if err != nil {
		if repo_model.IsErrRepoNotExist(err) || repo_model.IsErrSubjectNotExist(err) {
			ctx.NotFound(err)
		} else {
			ctx.ServerError("GetRepositoryByOwnerAndSubject (repo2)", err)
		}
		return
	}

	// Load owners for both repos
	if err := repo1.LoadOwner(ctx); err != nil {
		ctx.ServerError("LoadOwner (repo1)", err)
		return
	}
	if err := repo2.LoadOwner(ctx); err != nil {
		ctx.ServerError("LoadOwner (repo2)", err)
		return
	}

	// Check permissions for both repos
	perm1, err := access_model.GetUserRepoPermission(ctx, repo1, ctx.Doer)
	if err != nil {
		ctx.ServerError("GetUserRepoPermission (repo1)", err)
		return
	}
	if !perm1.CanRead(unit.TypeCode) {
		ctx.NotFound(nil)
		return
	}

	perm2, err := access_model.GetUserRepoPermission(ctx, repo2, ctx.Doer)
	if err != nil {
		ctx.ServerError("GetUserRepoPermission (repo2)", err)
		return
	}
	if !perm2.CanRead(unit.TypeCode) {
		ctx.NotFound(nil)
		return
	}

	// Get README content from each repo's default branch HEAD
	readme1Content, readme1Name, err := getReadmeContent(ctx, repo1)
	if err != nil && !isReadmeNotFoundError(err) {
		ctx.ServerError("getReadmeContent (repo1)", err)
		return
	}

	readme2Content, readme2Name, err := getReadmeContent(ctx, repo2)
	if err != nil && !isReadmeNotFoundError(err) {
		ctx.ServerError("getReadmeContent (repo2)", err)
		return
	}

	// Generate diff using diffmatchpatch
	diff := generateReadmeDiff(readme1Content, readme2Content, readme1Name, readme2Name)

	// Set up template data
	ctx.Data["Title"] = "Point of Contention: " + owner1 + " vs " + owner2
	ctx.Data["Subject"] = subject
	ctx.Data["Repo1"] = repo1
	ctx.Data["Repo2"] = repo2
	//ctx.Data["Repo1ContributorCount"] = repo1.GetContributorCount()
	//ctx.Data["Repo2ContributorCount"] = repo2.GetContributorCount()
	ctx.Data["Owner1"] = owner1
	ctx.Data["Owner2"] = owner2
	ctx.Data["Readme1Content"] = readme1Content
	ctx.Data["Readme2Content"] = readme2Content
	ctx.Data["Readme1Name"] = readme1Name
	ctx.Data["Readme2Name"] = readme2Name
	ctx.Data["Diff"] = diff
	ctx.Data["IsSplitStyle"] = true
	ctx.Data["PageIsSubjectCompare"] = true

	ctx.HTML(http.StatusOK, "repo/diff/subject_compare")
}

// parseOwnerParams parses the "owner1...owner2" format from the URL
func parseOwnerParams(params string) (owner1, owner2 string, err error) {
	parts := strings.Split(params, "...")
	if len(parts) != 2 {
		return "", "", errors.New("invalid owner format, expected owner1...owner2")
	}
	owner1 = strings.TrimSpace(parts[0])
	owner2 = strings.TrimSpace(parts[1])
	if owner1 == "" || owner2 == "" {
		return "", "", errors.New("owner names cannot be empty")
	}
	return owner1, owner2, nil
}

// ErrReadmeNotFound is returned when no README file is found in the repository
var ErrReadmeNotFound = errors.New("README file not found")

// isReadmeNotFoundError checks if the error is a README not found error
func isReadmeNotFoundError(err error) bool {
	return errors.Is(err, ErrReadmeNotFound)
}

// getReadmeContent retrieves the README content from a repository's default branch HEAD
func getReadmeContent(ctx *context.Context, repo *repo_model.Repository) (content, filename string, err error) {
	// Handle empty repositories
	if repo.IsEmpty {
		return "", "", ErrReadmeNotFound
	}

	// Open the git repository
	gitRepo, err := gitrepo.OpenRepository(ctx, repo)
	if err != nil {
		return "", "", err
	}
	defer gitRepo.Close()

	// Get the default branch commit
	commit, err := gitRepo.GetBranchCommit(repo.DefaultBranch)
	if err != nil {
		return "", "", err
	}

	// Try to find README file
	for _, name := range readmeFileNames {
		fileContent, err := commit.GetFileContent(name, int(setting.UI.MaxDisplayFileSize))
		if err == nil {
			return fileContent, name, nil
		}
	}

	return "", "", ErrReadmeNotFound
}

// generateReadmeDiff generates a diff between two README contents
func generateReadmeDiff(content1, content2, name1, name2 string) *gitdiff.Diff {
	// Split content into lines for line-based diff
	lines1 := strings.Split(content1, "\n")
	lines2 := strings.Split(content2, "\n")

	// Convert to Gitea's diff format
	return convertToGiteaDiff(lines1, lines2, name1, name2)
}

// convertToGiteaDiff converts line arrays to Gitea's Diff structure
func convertToGiteaDiff(lines1, lines2 []string, name1, name2 string) *gitdiff.Diff {
	// Determine the filename to use
	filename := "README.md"
	if name1 != "" {
		filename = name1
	} else if name2 != "" {
		filename = name2
	}

	// Build DiffLines from the line-based comparison
	diffLines := buildDiffLines(lines1, lines2)

	// Create a single section with all lines
	section := &gitdiff.DiffSection{
		FileName: filename,
		Lines:    diffLines,
	}

	// Calculate additions and deletions
	additions := 0
	deletions := 0
	for _, line := range diffLines {
		switch line.Type {
		case gitdiff.DiffLineAdd:
			additions++
		case gitdiff.DiffLineDel:
			deletions++
		}
	}

	// Create the DiffFile
	diffFile := &gitdiff.DiffFile{
		Name:      filename,
		OldName:   filename,
		Addition:  additions,
		Deletion:  deletions,
		Type:      gitdiff.DiffFileChange,
		Sections:  []*gitdiff.DiffSection{section},
		IsCreated: name1 == "" && name2 != "",
		IsDeleted: name1 != "" && name2 == "",
	}

	return &gitdiff.Diff{
		Files: []*gitdiff.DiffFile{diffFile},
	}
}

// buildDiffLines creates DiffLines by comparing two sets of lines
func buildDiffLines(lines1, lines2 []string) []*gitdiff.DiffLine {
	// Use a simple line-by-line diff algorithm (LCS-based)
	diffLines := make([]*gitdiff.DiffLine, 0)

	// Add section header
	diffLines = append(diffLines, &gitdiff.DiffLine{
		Type:    gitdiff.DiffLineSection,
		Content: "@@ -1," + string(rune(len(lines1))) + " +1," + string(rune(len(lines2))) + " @@",
		SectionInfo: &gitdiff.DiffLineSectionInfo{
			Path:          "README.md",
			LastLeftIdx:   0,
			LastRightIdx:  0,
			LeftIdx:       1,
			RightIdx:      1,
			LeftHunkSize:  len(lines1),
			RightHunkSize: len(lines2),
		},
	})

	// Use Myers diff algorithm via diffmatchpatch for line-level comparison
	dmp := diffmatchpatch.New()

	// Join lines with a unique separator for line-based diff
	text1 := strings.Join(lines1, "\n")
	text2 := strings.Join(lines2, "\n")

	// Get line-based diff
	a, b, lineArray := dmp.DiffLinesToChars(text1, text2)
	diffs := dmp.DiffMain(a, b, false)
	diffs = dmp.DiffCharsToLines(diffs, lineArray)
	diffs = dmp.DiffCleanupSemantic(diffs)

	leftIdx := 1
	rightIdx := 1

	for _, d := range diffs {
		lines := strings.Split(strings.TrimSuffix(d.Text, "\n"), "\n")
		for _, line := range lines {
			if line == "" && d.Text == "" {
				continue
			}
			switch d.Type {
			case diffmatchpatch.DiffEqual:
				diffLines = append(diffLines, &gitdiff.DiffLine{
					LeftIdx:  leftIdx,
					RightIdx: rightIdx,
					Type:     gitdiff.DiffLinePlain,
					Content:  " " + line,
					Match:    0,
				})
				leftIdx++
				rightIdx++
			case diffmatchpatch.DiffDelete:
				diffLines = append(diffLines, &gitdiff.DiffLine{
					LeftIdx:  leftIdx,
					RightIdx: 0,
					Type:     gitdiff.DiffLineDel,
					Content:  "-" + line,
					Match:    -1,
				})
				leftIdx++
			case diffmatchpatch.DiffInsert:
				diffLines = append(diffLines, &gitdiff.DiffLine{
					LeftIdx:  0,
					RightIdx: rightIdx,
					Type:     gitdiff.DiffLineAdd,
					Content:  "+" + line,
					Match:    -1,
				})
				rightIdx++
			}
		}
	}

	return diffLines
}
