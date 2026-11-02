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
//
// The existing subject the name resolved to is returned too (nil for a new subject), so that a
// form rendered again after an error still lets a subject that predates the rule through.
func PrepareSubjectAndRepoName(ctx context.Context, subject, repoName string) (string, string, *repo_model.Subject, error) {
	if subject == "" {
		return subject, repoName, nil, nil
	}

	resolved, existing, err := repo_model.ResolveSubject(ctx, subject)
	if err != nil {
		return subject, repoName, nil, err
	}

	if repoName == "" ||
		repo_model.IsGeneratedRepoName(repoName, subject) ||
		repo_model.IsGeneratedRepoName(repoName, resolved) {
		repoName = repo_model.GenerateRepoNameFromSubject(resolved)
	}
	return resolved, repoName, existing, nil
}
