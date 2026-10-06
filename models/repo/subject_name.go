// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"code.gitea.io/gitea/modules/subjecttitle"
	"code.gitea.io/gitea/modules/util"
)

// maxSubjectLookupBytes bounds the input looked up as an existing subject: names are stored in
// a VARCHAR(255) column, so an existing name has at most 255 characters of up to 4 bytes.
const maxSubjectLookupBytes = 4 * 255

// ValidateSubjectName normalizes the given name and checks it against the subject title rule
// (see modules/subjecttitle), including the maximum length. It returns the normalized name, or
// ErrSubjectNameInvalid.
func ValidateSubjectName(name string) (string, error) {
	name = subjecttitle.Normalize(name)
	if problem := subjecttitle.Check(name); problem != subjecttitle.ProblemNone {
		return name, ErrSubjectNameInvalid{Name: name, Problem: problem}
	}
	return name, nil
}

// lookupSubjectForCreate normalizes the name and returns the existing subject it resolves to
// by slug, if any. When there is none, the normalized name must follow the subject title rule
// because a new subject would be created; ErrSubjectNameInvalid is returned otherwise.
// Existing subjects are used as-is, so subjects created before the rule stay usable and
// "Moon~~~" still resolves to an existing "Moon".
//
// The slug of the name as typed is tried first, so that the normalization of new titles
// (underscores, Unicode spaces, first letter) never changes which existing subject is found.
func lookupSubjectForCreate(ctx context.Context, name string) (string, *Subject, error) {
	typed := strings.TrimSpace(name)
	name = subjecttitle.Normalize(name)
	if name == "" {
		return "", nil, ErrSubjectNameInvalid{Name: name, Problem: subjecttitle.ProblemEmpty}
	}
	if len(typed) > maxSubjectLookupBytes {
		return name, nil, ErrSubjectNameInvalid{Name: name, Problem: subjecttitle.ProblemTooLong}
	}

	slugs := []string{GenerateSlugFromName(typed)}
	if slug := GenerateSlugFromName(name); slug != slugs[0] {
		slugs = append(slugs, slug)
	}
	for _, slug := range slugs {
		existing, err := GetSubjectBySlug(ctx, slug)
		if err == nil {
			return name, existing, nil
		}
		if !IsErrSubjectNotExist(err) {
			return name, nil, err
		}
	}

	name, err := ValidateSubjectName(name)
	return name, nil, err
}

// ResolveSubjectName returns the subject name to use for a new article: the name of the
// existing subject the given name resolves to (by slug), or else the normalized name, which
// must then follow the subject title rule (ErrSubjectNameInvalid otherwise).
func ResolveSubjectName(ctx context.Context, name string) (string, error) {
	name, existing, err := lookupSubjectForCreate(ctx, name)
	if err != nil {
		return name, err
	}
	if existing != nil {
		return existing.Name, nil
	}
	return name, nil
}

// ErrSubjectNameInvalid is returned when a new subject's title does not follow the subject
// title rule; Problem says why.
type ErrSubjectNameInvalid struct {
	Name    string
	Problem subjecttitle.Problem
}

// IsErrSubjectNameInvalid checks if an error is (or wraps) ErrSubjectNameInvalid
func IsErrSubjectNameInvalid(err error) bool {
	return errors.As(err, &ErrSubjectNameInvalid{})
}

func (err ErrSubjectNameInvalid) Error() string {
	switch err.Problem {
	case subjecttitle.ProblemEmpty:
		return "subject name cannot be empty"
	case subjecttitle.ProblemTooLong:
		return fmt.Sprintf("subject name is too long (maximum %d bytes in UTF-8)", subjecttitle.MaxBytes)
	case subjecttitle.ProblemForbiddenChar:
		return fmt.Sprintf("subject name %q is invalid: it cannot contain # < > [ ] | { }, control characters, U+FFFD, U+FFFE or U+FFFF", err.Name)
	case subjecttitle.ProblemPercentEncoding:
		return fmt.Sprintf("subject name %q is invalid: it cannot contain %% followed by two hexadecimal digits", err.Name)
	case subjecttitle.ProblemHTMLEntity:
		return fmt.Sprintf("subject name %q is invalid: it cannot contain HTML character references such as &amp;", err.Name)
	case subjecttitle.ProblemTildes:
		return fmt.Sprintf("subject name %q is invalid: it cannot contain three or more consecutive tildes", err.Name)
	case subjecttitle.ProblemRelativePath:
		return fmt.Sprintf("subject name %q is invalid: it cannot be . or .., start with ./ or ../, contain /./ or /../, or end with /. or /..", err.Name)
	case subjecttitle.ProblemLeadingColon:
		return fmt.Sprintf("subject name %q is invalid: it cannot start with a colon", err.Name)
	}
	return fmt.Sprintf("subject name %q is invalid", err.Name)
}

// Unwrap lets callers treat the error as an invalid argument
func (err ErrSubjectNameInvalid) Unwrap() error {
	return util.ErrInvalidArgument
}
