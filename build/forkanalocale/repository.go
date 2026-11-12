// Copyright 2026 The Forkana Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package forkanalocale

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Paths of a checkout, relative to its root.
const (
	UpstreamDir    = "options/locale"
	CustomDir      = "custom/options/locale"
	TranslationDir = "custom/translations"
	englishFile    = "locale_en-US.ini"
)

var localeFileRe = regexp.MustCompile(`^locale_([a-zA-Z]{2,3}(?:-[a-zA-Z0-9]+)*)\.ini$`)

// FileName is the locale file name of a language.
func FileName(lang string) string { return "locale_" + lang + ".ini" }

// Repository is the locale files of a checkout.
type Repository struct {
	Root      string
	Forkana   *Forkana
	Languages []string // upstream languages other than en-US
}

// OpenRepository reads the en-US files and lists the languages of a checkout.
func OpenRepository(root string) (*Repository, error) {
	r := &Repository{Root: root}
	upstreamEnglish, err := r.parse(filepath.Join(UpstreamDir, englishFile))
	if err != nil {
		return nil, err
	}
	customEnglish, err := r.parse(filepath.Join(CustomDir, englishFile))
	if err != nil {
		return nil, err
	}
	r.Forkana = NewForkana(upstreamEnglish, customEnglish)
	entries, err := os.ReadDir(filepath.Join(root, UpstreamDir))
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if m := localeFileRe.FindStringSubmatch(e.Name()); m != nil && e.Name() != englishFile {
			r.Languages = append(r.Languages, m[1])
		}
	}
	return r, nil
}

func (r *Repository) path(rel string) string { return filepath.Join(r.Root, rel) }

func (r *Repository) parse(rel string) (*File, error) {
	data, err := os.ReadFile(r.path(rel))
	if err != nil {
		return nil, err
	}
	f, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", rel, err)
	}
	return f, nil
}

// Translations reads custom/translations/locale_<lang>.ini, empty if absent.
func (r *Repository) Translations(lang string) (*File, error) {
	rel := filepath.Join(TranslationDir, FileName(lang))
	if _, err := os.Stat(r.path(rel)); errors.Is(err, os.ErrNotExist) {
		return Parse(nil)
	}
	return r.parse(rel)
}

// language is what one language's files should be.
type language struct {
	translations *File
	generated    []byte // custom/options/locale/locale_<lang>.ini
	formatted    []byte // custom/translations/locale_<lang>.ini in canonical form
}

func (r *Repository) generateLanguage(lang string) (*language, error) {
	tr, err := r.Translations(lang)
	if err != nil {
		return nil, err
	}
	upstream, err := os.ReadFile(r.path(filepath.Join(UpstreamDir, FileName(lang))))
	if err != nil {
		return nil, err
	}
	generated, err := Generate(FileName(lang), upstream, r.Forkana, tr)
	if err != nil {
		return nil, err
	}
	return &language{translations: tr, generated: generated, formatted: FormatTranslations(lang, r.Forkana, tr)}, nil
}

// Generate writes the generated locale file and the canonical translation file of every language.
func (r *Repository) Generate() error {
	if err := os.MkdirAll(r.path(TranslationDir), 0o755); err != nil {
		return err
	}
	for _, lang := range r.Languages {
		l, err := r.generateLanguage(lang)
		if err != nil {
			return err
		}
		if err := os.WriteFile(r.path(filepath.Join(CustomDir, FileName(lang))), l.generated, 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(r.path(filepath.Join(TranslationDir, FileName(lang))), l.formatted, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// Check returns the errors and warnings of every language (see Problems), plus
// out-of-date files, generated files Gitea would read differently, and custom
// locale files without upstream file. Errors are sorted.
func (r *Repository) Check() (errs, warnings []string, err error) {
	for _, lang := range r.Languages {
		l, err := r.generateLanguage(lang)
		if err != nil {
			return nil, nil, err
		}
		p := Check(lang, r.Forkana, l.translations)
		errs = append(errs, p.Errors...)
		warnings = append(warnings, p.Warnings...)
		errs = append(errs, VerifyGenerated(lang, l.generated, r.Forkana, l.translations)...)
		for rel, want := range map[string][]byte{
			filepath.Join(CustomDir, FileName(lang)):      l.generated,
			filepath.Join(TranslationDir, FileName(lang)): l.formatted,
		} {
			if onDisk, err := os.ReadFile(r.path(rel)); err != nil || !bytes.Equal(onDisk, want) {
				errs = append(errs, fmt.Sprintf("%s: %s is out of date: run `make generate-locale-forkana`", lang, rel))
			}
		}
	}
	entries, err := os.ReadDir(r.path(CustomDir))
	if err != nil {
		return nil, nil, err
	}
	for _, e := range entries {
		if m := localeFileRe.FindStringSubmatch(e.Name()); m != nil && e.Name() != englishFile && !slices.Contains(r.Languages, m[1]) {
			errs = append(errs, fmt.Sprintf("%s: no upstream %s/%s", e.Name(), UpstreamDir, e.Name()))
		}
	}
	slices.Sort(errs)
	return errs, warnings, nil
}

// Todo writes <dir>/locale_<lang>.ini for every language with strings to
// translate, and returns how many each has.
func (r *Repository) Todo(dir string) (map[string]int, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, lang := range r.Languages {
		tr, err := r.Translations(lang)
		if err != nil {
			return nil, err
		}
		fragment, n := Todo(r.Forkana, tr)
		path := filepath.Join(dir, FileName(lang))
		if n == 0 {
			_ = os.Remove(path)
			continue
		}
		counts[lang] = n
		header := fmt.Sprintf("# Translate each value into %s, keeping the \"# en:\" lines. Then run: go run build/forkana-locales.go merge %s\n", lang, dir)
		if err := os.WriteFile(path, append([]byte(header), fragment...), 0o644); err != nil {
			return nil, err
		}
	}
	return counts, nil
}

// Merge merges the translated <dir>/locale_<lang>.ini fragments into
// custom/translations/, then generates. It returns, per language, the
// fragment's keys left untranslated (see Merge).
func (r *Repository) Merge(dir string) (map[string][]string, error) {
	untranslated := map[string][]string{}
	for _, lang := range r.Languages {
		path := filepath.Join(dir, FileName(lang))
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, err
		}
		fragment, err := Parse(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		tr, err := r.Translations(lang)
		if err != nil {
			return nil, err
		}
		merged, left := Merge(r.Forkana, tr, fragment)
		if len(left) > 0 {
			untranslated[lang] = left
		}
		if err := os.WriteFile(r.path(filepath.Join(TranslationDir, FileName(lang))), FormatTranslations(lang, r.Forkana, merged), 0o644); err != nil {
			return nil, err
		}
	}
	return untranslated, r.Generate()
}

// GroupByLanguage turns "lang: message" lines into "message (n: langs)" lines,
// so a key missing in every language takes one line.
func GroupByLanguage(lines []string) []string {
	langsOf := map[string][]string{}
	var order []string
	for _, l := range lines {
		lang, msg, _ := strings.Cut(l, ": ")
		if _, ok := langsOf[msg]; !ok {
			order = append(order, msg)
		}
		langsOf[msg] = append(langsOf[msg], lang)
	}
	grouped := make([]string, 0, len(order))
	for _, msg := range order {
		grouped = append(grouped, fmt.Sprintf("%s (%d: %s)", msg, len(langsOf[msg]), strings.Join(langsOf[msg], " ")))
	}
	return grouped
}
