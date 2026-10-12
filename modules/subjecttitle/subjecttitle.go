// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

// Package subjecttitle holds the subject title rule (issue #401). It follows Wikipedia's
// technical restrictions on page titles
// (https://en.wikipedia.org/wiki/Wikipedia:Naming_conventions_(technical_restrictions)), that
// is MediaWiki's title rules, where they apply to a site with a single namespace:
//
//   - forbidden characters: # < > [ ] | { }, the ASCII control characters (U+0000–U+001F and
//     U+007F), U+FFFD (the replacement character) and the noncharacters U+FFFE and U+FFFF, as
//     well as invalid UTF-8;
//   - forbidden sequences: percent-encoding ("%" followed by two hex digits), HTML character
//     references ("&amp;", "&#47;", "&#x2F;") and three or more consecutive tildes ("~~~");
//   - relative paths: the title may not be "." or "..", start with "./" or "../", contain
//     "/./" or "/../", or end with "/." or "/..";
//   - the title may not start with a colon (Forkana has no namespaces or interwiki prefixes,
//     so the other colon restrictions do not apply);
//   - the title must be fewer than 256 bytes long in UTF-8 (MaxBytes).
//
// Everything else is allowed: punctuation, symbols and any script. Before validating, titles
// are normalized like MediaWiki does (see Normalize), including capitalizing the first letter.
//
// The rule is mirrored in JavaScript by web_src/js/features/subject-title.ts; both are tested
// against the shared cases in testdata/cases.json.
//
// The package only depends on the standard library and golang.org/x/text so that the
// standalone tools in custom/services (article-creator) can share it with the server.
package subjecttitle

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"golang.org/x/text/unicode/norm"
)

// MaxBytes is the maximum length of a subject title in bytes, encoded in UTF-8: Wikipedia
// titles "must be fewer than 256 bytes long when encoded in UTF-8". A title of at most 255
// bytes always fits the VARCHAR(255) subject.name column, which counts characters.
const MaxBytes = 255

// Problem is the reason a title breaks the rule; it is empty for a valid title. The values
// are shared with the JavaScript validator and name the locale strings
// "repo.form.subject_title_<problem>".
type Problem string

const (
	ProblemNone            Problem = ""
	ProblemEmpty           Problem = "empty"
	ProblemForbiddenChar   Problem = "forbidden_char"
	ProblemPercentEncoding Problem = "percent_encoding"
	ProblemHTMLEntity      Problem = "html_entity"
	ProblemTildes          Problem = "tildes"
	ProblemRelativePath    Problem = "relative_path"
	ProblemLeadingColon    Problem = "leading_colon"
	ProblemTooLong         Problem = "too_long"
)

var (
	// MediaWiki's $wgLegalTitleChars without the bytes it forbids, plus the three code points
	// it uses as placeholders for invalid UTF-8.
	forbiddenCharRe = regexp.MustCompile(`[#<>\[\]|{}\x00-\x1F\x7F\x{FFFD}\x{FFFE}\x{FFFF}]`)
	// MediaWikiTitleCodec::getTitleInvalidRegex
	percentEncodingRe = regexp.MustCompile(`%[0-9A-Fa-f]{2}`)
	htmlEntityRe      = regexp.MustCompile(`&[A-Za-z0-9\x{80}-\x{10FFFF}]+;|&#[0-9]+;|&#x[0-9A-Fa-f]+;`)

	// Title::secureAndSplit: the characters MediaWiki treats as spaces, and the directional
	// marks it strips.
	spacesRe      = regexp.MustCompile(`[ _\x{00A0}\x{1680}\x{180E}\x{2000}-\x{200A}\x{2028}\x{2029}\x{202F}\x{205F}\x{3000}]+`)
	directionalRe = regexp.MustCompile(`[\x{200E}\x{200F}\x{202A}-\x{202E}]`)

	upperCaser = cases.Upper(language.Und)
)

// Normalize applies MediaWiki's title normalization: NFC, directional marks (U+200E, U+200F,
// U+202A–U+202E) are removed, underscores and Unicode spaces become spaces, runs of spaces
// collapse into one, leading and trailing spaces are stripped, and the first letter is
// capitalized ($wgCapitalLinks: "iPhone" becomes "IPhone"). Only the first character is
// changed, and only when it has a single-character uppercase form ("ß" stays "ß").
//
// It only applies to new subjects: existing subjects are found by slug and keep their name.
func Normalize(title string) string {
	title = norm.NFC.String(title)
	title = directionalRe.ReplaceAllString(title, "")
	title = strings.Trim(spacesRe.ReplaceAllString(title, " "), " ")
	if r, size := utf8.DecodeRuneInString(title); r != utf8.RuneError {
		// the full Unicode mapping, as JavaScript's toUpperCase, applied only when it is one
		// character: "ß" ("SS") and "ᾳ" ("ΑΙ") stay as they are on both sides
		if upper := upperCaser.String(string(r)); utf8.RuneCountInString(upper) == 1 {
			title = upper + title[size:]
		}
	}
	return norm.NFC.String(title)
}

func isRelativePath(title string) bool {
	return title == "." || title == ".." ||
		strings.HasPrefix(title, "./") || strings.HasPrefix(title, "../") ||
		strings.Contains(title, "/./") || strings.Contains(title, "/../") ||
		strings.HasSuffix(title, "/.") || strings.HasSuffix(title, "/..")
}

// Check returns why an already normalized title breaks the rule, or ProblemNone.
func Check(title string) Problem {
	switch {
	case title == "":
		return ProblemEmpty
	case !utf8.ValidString(title):
		return ProblemForbiddenChar
	case htmlEntityRe.MatchString(title): // before the characters: "&#47;" contains "#"
		return ProblemHTMLEntity
	case percentEncodingRe.MatchString(title):
		return ProblemPercentEncoding
	case forbiddenCharRe.MatchString(title):
		return ProblemForbiddenChar
	case strings.Contains(title, "~~~"):
		return ProblemTildes
	case isRelativePath(title):
		return ProblemRelativePath
	case strings.HasPrefix(title, ":"):
		return ProblemLeadingColon
	case IsTooLong(title):
		return ProblemTooLong
	}
	return ProblemNone
}

// IsValid reports whether an already normalized title follows the rule, length included.
func IsValid(title string) bool {
	return Check(title) == ProblemNone
}

// IsTooLong reports whether the title is longer than MaxBytes bytes in UTF-8.
func IsTooLong(title string) bool {
	return len(title) > MaxBytes
}

func removeEntitySemicolons(s string) string {
	// "&amp;" → "&amp", as Wikipedia suggests ("omitting a semicolon"); repeated because
	// removing a semicolon can join two references ("&a;b;" → "&ab;").
	for htmlEntityRe.MatchString(s) {
		s = htmlEntityRe.ReplaceAllStringFunc(s, func(m string) string { return strings.TrimSuffix(m, ";") })
	}
	return s
}

func removeRelativePath(s string) string {
	for {
		before := s
		s = strings.ReplaceAll(s, "/./", "/")
		s = strings.ReplaceAll(s, "/../", "/")
		s = strings.TrimPrefix(s, "./")
		s = strings.TrimPrefix(s, "../")
		s = strings.TrimSuffix(s, "/.")
		s = strings.TrimSuffix(s, "/..")
		if s == "." || s == ".." {
			s = ""
		}
		if s == before {
			return s
		}
	}
}

func truncateBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

// Clean turns arbitrary text (a search keyword, a Wikipedia title) into a valid subject title,
// changing as little as possible: control characters become spaces, the other forbidden
// characters are removed (as Wikipedia does for "M|A|R|R|S" → "MARRS"), a space is inserted
// after "%" in percent-encodings, the semicolon of HTML character references is dropped, runs
// of tildes are shortened to "~~", relative path segments and leading colons are removed, the
// title is normalized and cut to MaxBytes bytes. It returns "" when nothing valid is left.
//
//	";alskdjf"                      → ";alskdjf"
//	"Python (programming language)" → "Python (programming language)"
//	"C#"                            → "C"
//	"iPhone"                        → "IPhone"
func Clean(text string) string {
	s := strings.ToValidUTF8(text, "")
	// Each pass removes characters, or breaks a percent-encoding by inserting a space, which
	// can never form a new forbidden sequence; every pass that changes the title therefore
	// leaves fewer violations, and the loop stops at its fixed point after a few passes.
	for {
		before := s
		s = strings.Map(func(r rune) rune {
			switch {
			case r < 0x20 || r == 0x7F:
				return ' '
			case strings.ContainsRune("#<>[]|{}\ufffd\ufffe\uffff", r):
				return -1
			}
			return r
		}, s)
		s = percentEncodingRe.ReplaceAllStringFunc(s, func(m string) string { return "% " + m[1:] })
		s = removeEntitySemicolons(s)
		for strings.Contains(s, "~~~") {
			s = strings.ReplaceAll(s, "~~~", "~~")
		}
		s = Normalize(s)
		s = removeRelativePath(s)
		s = Normalize(strings.TrimLeft(s, ": "))
		s = Normalize(truncateBytes(s, MaxBytes))
		if s == before {
			break
		}
	}
	if !IsValid(s) {
		return ""
	}
	return s
}
