// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repository

import (
	"context"

	repo_model "code.gitea.io/gitea/models/repo"
)

// PrepareSubjectAndRepoName resolves the subject submitted with a new article and derives the
// repository name from it. It is shared by the web and API create, generate and migrate
// handlers.
//
// An empty subject is left alone. Otherwise the subject is resolved with
// repo_model.ResolveSubjectName: an existing subject (matched by slug) is used as-is, with no
// title rule check, and only a subject that would be newly created has to follow the rule
// (repo_model.ErrSubjectNameInvalid otherwise).
//
// The repository name is generated from the resolved subject when it is empty or equals the
// name generated from the submitted or the resolved subject (i.e. the user did not choose one).
// The two can differ: "foo bar" gives "foobar", its resolved "Foo bar" gives "foo-bar".
func PrepareSubjectAndRepoName(ctx context.Context, subject, repoName string) (string, string, error) {
	if subject == "" {
		return subject, repoName, nil
	}

	resolved, err := repo_model.ResolveSubjectName(ctx, subject)
	if err != nil {
		return subject, repoName, err
	}

	if repoName == "" ||
		repoName == repo_model.GenerateRepoNameFromSubject(subject) ||
		repoName == repo_model.GenerateRepoNameFromSubject(resolved) {
		repoName = repo_model.GenerateRepoNameFromSubject(resolved)
	}
	return resolved, repoName, nil
}
