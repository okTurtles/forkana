// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"context"
	"errors"
	"fmt"

	"code.gitea.io/gitea/modules/subjecttitle"
	"code.gitea.io/gitea/modules/util"
)

// ValidateSubjectName normalizes the given name and checks it against the subject title rule
// (see modules/subjecttitle), including the maximum length. It returns the normalized name, or
// ErrSubjectNameInvalid.
func ValidateSubjectName(name string) (string, error) {
	name = subjecttitle.Normalize(name)
	if subjecttitle.IsTooLong(name) {
		return name, ErrSubjectNameInvalid{Name: name, TooLong: true}
	}
	if !subjecttitle.IsValid(name) {
		return name, ErrSubjectNameInvalid{Name: name}
	}
	return name, nil
}

// lookupSubjectForCreate normalizes the name and returns the existing subject it resolves to
// by slug, if any. When there is none, the normalized name must follow the subject title rule
// because a new subject would be created; ErrSubjectNameInvalid is returned otherwise.
// Existing subjects are used as-is, so subjects created before the rule stay usable and
// "Moon!" still resolves to an existing "Moon".
func lookupSubjectForCreate(ctx context.Context, name string) (string, *Subject, error) {
	name = subjecttitle.Normalize(name)
	if name == "" {
		return "", nil, ErrSubjectNameInvalid{Name: name}
	}
	if subjecttitle.IsTooLong(name) {
		return name, nil, ErrSubjectNameInvalid{Name: name, TooLong: true}
	}

	existing, err := GetSubjectBySlug(ctx, GenerateSlugFromName(name))
	if err == nil {
		return name, existing, nil
	}
	if !IsErrSubjectNotExist(err) {
		return name, nil, err
	}

	name, err = ValidateSubjectName(name)
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
// title rule, or is too long (TooLong).
type ErrSubjectNameInvalid struct {
	Name    string
	TooLong bool
}

// IsErrSubjectNameInvalid checks if an error is (or wraps) ErrSubjectNameInvalid
func IsErrSubjectNameInvalid(err error) bool {
	return errors.As(err, &ErrSubjectNameInvalid{})
}

func (err ErrSubjectNameInvalid) Error() string {
	if err.TooLong {
		return fmt.Sprintf("subject name is too long (maximum %d characters)", subjecttitle.MaxLength)
	}
	if err.Name == "" {
		return "subject name cannot be empty"
	}
	return fmt.Sprintf("subject name %q is invalid: it may only contain letters, digits, spaces, hyphens and apostrophes, and must start with a letter or digit", err.Name)
}

// Unwrap lets callers treat the error as an invalid argument
func (err ErrSubjectNameInvalid) Unwrap() error {
	return util.ErrInvalidArgument
}
