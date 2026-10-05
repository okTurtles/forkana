// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"code.gitea.io/gitea/modules/util"

	"golang.org/x/text/unicode/norm"
)

// Subject title rule (issue #401):
//
//   - allowed characters: Unicode letters (accented names such as "Gaudí" are fine, combining
//     marks are accepted after the first character so scripts that need them keep working),
//     decimal digits, spaces, hyphens (-) and apostrophes (' and the typographic ’);
//   - the title must start with a letter or a digit.
//
// The HTML pattern used by the subject inputs (SubjectNameHTMLPattern) mirrors this rule.

// SubjectNameHTMLPattern is the HTML `pattern` attribute (evaluated with the `v` flag by
// browsers) matching what IsValidSubjectName accepts. Leading/trailing whitespace is tolerated
// because the server trims it before validating.
const SubjectNameHTMLPattern = `\s*[\p{L}\p{Nd}][\p{L}\p{M}\p{Nd}\s'’\-]*`

// NormalizeSubjectName trims surrounding whitespace, collapses runs of whitespace into a
// single space and converts the name to Unicode NFC so that "Gaudí" typed with a combining
// accent is stored the same way as the precomposed form.
func NormalizeSubjectName(name string) string {
	return norm.NFC.String(strings.Join(strings.Fields(name), " "))
}

// IsValidSubjectName reports whether an already normalized subject name follows the subject
// title rule. It does not check the length; see MaxSubjectNameLength.
func IsValidSubjectName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			// always allowed, including as the first character
		case i == 0:
			return false
		case r == ' ', r == '-', r == '\'', r == '’', unicode.Is(unicode.M, r):
			// allowed after the first character
		default:
			return false
		}
	}
	return true
}

// ValidateSubjectName normalizes the given name and checks it against the subject title
// rule and the maximum length. It returns the normalized name.
func ValidateSubjectName(name string) (string, error) {
	name = NormalizeSubjectName(name)
	if name == "" {
		return "", ErrSubjectNameInvalid{Name: name}
	}
	if len(name) > MaxSubjectNameLength {
		return name, fmt.Errorf("subject name is too long (maximum %d characters)", MaxSubjectNameLength)
	}
	if !IsValidSubjectName(name) {
		return name, ErrSubjectNameInvalid{Name: name}
	}
	return name, nil
}

// CheckSubjectNameForCreate normalizes a subject name submitted with a new article and
// returns the normalized name. A name that follows the subject title rule is accepted (it may
// match an existing subject by slug or create a new one). A name that breaks the rule is only
// accepted when it is exactly the name of an existing subject, so that subjects created before
// the rule existed stay usable; otherwise ErrSubjectNameInvalid is returned.
func CheckSubjectNameForCreate(ctx context.Context, name string) (string, error) {
	normalized, err := ValidateSubjectName(name)
	if err == nil || !IsErrSubjectNameInvalid(err) || normalized == "" {
		return normalized, err
	}
	existing, getErr := GetSubjectBySlug(ctx, GenerateSlugFromName(normalized))
	if getErr != nil {
		if IsErrSubjectNotExist(getErr) {
			return normalized, err
		}
		return normalized, getErr
	}
	if existing.Name != normalized {
		return normalized, err
	}
	return normalized, nil
}

// ErrSubjectNameInvalid is returned when a new subject's title does not follow the subject
// title rule.
type ErrSubjectNameInvalid struct {
	Name string
}

// IsErrSubjectNameInvalid checks if an error is (or wraps) ErrSubjectNameInvalid
func IsErrSubjectNameInvalid(err error) bool {
	return errors.As(err, &ErrSubjectNameInvalid{})
}

func (err ErrSubjectNameInvalid) Error() string {
	return fmt.Sprintf("subject name %q is invalid: it may only contain letters, digits, spaces, hyphens and apostrophes, and must start with a letter or digit", err.Name)
}

// Unwrap lets callers treat the error as an invalid argument
func (err ErrSubjectNameInvalid) Unwrap() error {
	return util.ErrInvalidArgument
}
