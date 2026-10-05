// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

// Package subjecttitle holds the subject title rule (issue #401):
//
//   - allowed characters: Unicode letters (accented names such as "Gaudí" are fine, combining
//     marks are accepted after the first character so scripts that need them keep working),
//     decimal digits, spaces, hyphens (-) and apostrophes (' and the typographic ’);
//   - the title must start with a letter or a digit;
//   - the title is at most MaxLength characters long.
//
// The package only depends on the standard library and golang.org/x/text so that the
// standalone tools in custom/services (article-creator) can share it with the server.
package subjecttitle

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// MaxLength is the maximum number of characters (runes) of a subject title. It matches the
// VARCHAR(255) subject.name column, which counts characters on the supported databases.
const MaxLength = 255

// Pattern is an unanchored regular expression matching what IsValid accepts, valid both in
// Go (RE2) and in JavaScript with the `u` flag. Leading/trailing whitespace is tolerated
// because the server trims it before validating.
//
// The forms use it only for a non-blocking hint (see templates/repo/subject_title_hint.tmpl),
// never as a blocking HTML `pattern`: a title that breaks the rule is still accepted when it
// resolves to an existing subject (models/repo.ResolveSubjectName), and only the server knows
// that.
const Pattern = `\s*[\p{L}\p{Nd}][\p{L}\p{M}\p{Nd}\s'’\-]*`

// Normalize trims surrounding whitespace, collapses runs of whitespace into a single space and
// converts the title to Unicode NFC, so that "Gaudí" typed with a combining accent is stored
// the same way as the precomposed form.
func Normalize(title string) string {
	return norm.NFC.String(strings.Join(strings.Fields(title), " "))
}

func isAllowedAfterFirst(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.Is(unicode.M, r) ||
		r == ' ' || r == '-' || r == '\'' || r == '’'
}

// IsValid reports whether an already normalized title follows the character rule. It does not
// check the length; see IsTooLong.
func IsValid(title string) bool {
	if title == "" {
		return false
	}
	for i, r := range title {
		if i == 0 {
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
				return false
			}
		} else if !isAllowedAfterFirst(r) {
			return false
		}
	}
	return true
}

// IsTooLong reports whether the title has more than MaxLength characters.
func IsTooLong(title string) bool {
	return utf8.RuneCountInString(title) > MaxLength
}

// Clean turns arbitrary text (a search keyword, a Wikipedia title) into a valid subject title:
// "+" is spelled out as "plus" (so "C++" does not collapse into "C"), other disallowed
// characters become spaces, the result is normalized, anything before the first letter or
// digit is dropped and the title is cut to MaxLength characters. It returns "" when nothing
// valid is left.
//
//	";alskdjf"                      → "alskdjf"
//	"Python (programming language)" → "Python programming language"
//	"C++"                           → "C plus plus"
func Clean(text string) string {
	cleaned := strings.ReplaceAll(text, "+", " plus ")
	cleaned = strings.Map(func(r rune) rune {
		if isAllowedAfterFirst(r) {
			return r
		}
		return ' '
	}, cleaned)
	cleaned = Normalize(cleaned)
	cleaned = strings.TrimLeftFunc(cleaned, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if IsTooLong(cleaned) {
		cleaned = string([]rune(cleaned)[:MaxLength])
	}
	cleaned = strings.TrimSpace(cleaned)
	if !IsValid(cleaned) {
		return ""
	}
	return cleaned
}
