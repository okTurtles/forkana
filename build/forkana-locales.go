// Copyright 2026 The Forkana Authors. All rights reserved.
// SPDX-License-Identifier: MIT

//go:build ignore

// forkana-locales builds and checks Forkana's translated locale files. See
// build/forkanalocale and "Translations" in AGENTS.md.
//
//	go run build/forkana-locales.go generate        # write custom/options/locale/locale_<lang>.ini
//	go run build/forkana-locales.go check           # fail on broken translations or outdated files, warn on missing/stale ones
//	go run build/forkana-locales.go check -strict   # fail on missing/stale translations too
//	go run build/forkana-locales.go todo <dir>      # write <dir>/locale_<lang>.ini: the strings to translate
//	go run build/forkana-locales.go merge <dir>     # merge translated <dir>/locale_<lang>.ini files, then generate
package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"code.gitea.io/gitea/build/forkanalocale"
)

const (
	upstreamDir    = "options/locale"
	customDir      = "custom/options/locale"
	translationDir = "custom/translations"
	english        = "locale_en-US.ini"
)

var localeFileRe = regexp.MustCompile(`^locale_([a-zA-Z]{2,3}(?:-[a-zA-Z0-9]+)*)\.ini$`)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	langs := languages()
	fk := loadForkana()
	var err error
	switch os.Args[1] {
	case "generate":
		err = generate(fk, langs)
	case "check":
		strict := len(os.Args) == 3 && os.Args[2] == "-strict"
		if len(os.Args) > 3 || (len(os.Args) == 3 && !strict) {
			usage()
		}
		err = check(fk, langs, strict)
	case "todo":
		if len(os.Args) != 3 {
			usage()
		}
		err = todo(fk, langs, os.Args[2])
	case "merge":
		if len(os.Args) != 3 {
			usage()
		}
		if err = merge(fk, langs, os.Args[2]); err == nil {
			err = generate(fk, langs)
		}
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: go run build/forkana-locales.go generate | check [-strict] | todo <dir> | merge <dir>")
	os.Exit(2)
}

func mustParse(path string) *forkanalocale.File {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	f, err := forkanalocale.Parse(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
		os.Exit(1)
	}
	return f
}

func loadForkana() *forkanalocale.Forkana {
	return forkanalocale.NewForkana(mustParse(filepath.Join(upstreamDir, english)), mustParse(filepath.Join(customDir, english)))
}

// languages lists the upstream languages other than en-US.
func languages() []string {
	entries, err := os.ReadDir(upstreamDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var langs []string
	for _, e := range entries {
		if m := localeFileRe.FindStringSubmatch(e.Name()); m != nil && e.Name() != english {
			langs = append(langs, m[1])
		}
	}
	return langs
}

func fileName(lang string) string { return "locale_" + lang + ".ini" }

// loadTranslations reads custom/translations/locale_<lang>.ini, empty if absent.
func loadTranslations(lang string) *forkanalocale.File {
	path := filepath.Join(translationDir, fileName(lang))
	if _, err := os.Stat(path); os.IsNotExist(err) {
		f, _ := forkanalocale.Parse(nil)
		return f
	}
	return mustParse(path)
}

func build(fk *forkanalocale.Forkana, lang string) (generated, translations []byte, err error) {
	name := fileName(lang)
	upstream, err := os.ReadFile(filepath.Join(upstreamDir, name))
	if err != nil {
		return nil, nil, err
	}
	tr := loadTranslations(lang)
	generated, err = forkanalocale.Generate(name, upstream, fk, tr)
	return generated, forkanalocale.FormatTranslations(lang, fk, tr), err
}

func generate(fk *forkanalocale.Forkana, langs []string) error {
	if err := os.MkdirAll(translationDir, 0o755); err != nil {
		return err
	}
	for _, lang := range langs {
		generated, translations, err := build(fk, lang)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(customDir, fileName(lang)), generated, 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(translationDir, fileName(lang)), translations, 0o644); err != nil {
			return err
		}
	}
	fmt.Printf("generated %d locale files from %d Forkana keys\n", len(langs), len(fk.Keys))
	return nil
}

// check prints the errors and warnings of every language. Errors fail it;
// warnings (missing or stale translations) only fail it when strict.
func check(fk *forkanalocale.Forkana, langs []string, strict bool) error {
	var errs, warnings []string
	for _, lang := range langs {
		p := forkanalocale.Check(lang, fk, loadTranslations(lang))
		errs = append(errs, p.Errors...)
		warnings = append(warnings, p.Warnings...)
		generated, translations, err := build(fk, lang)
		if err != nil {
			return err
		}
		for path, want := range map[string][]byte{
			filepath.Join(customDir, fileName(lang)):      generated,
			filepath.Join(translationDir, fileName(lang)): translations,
		} {
			if onDisk, err := os.ReadFile(path); err != nil || !bytes.Equal(onDisk, want) {
				errs = append(errs, fmt.Sprintf("%s: %s is out of date: run `make generate-locale-forkana`", lang, path))
			}
		}
	}
	// A custom locale file without upstream file would be missed by the loop above.
	entries, _ := os.ReadDir(customDir)
	for _, e := range entries {
		if m := localeFileRe.FindStringSubmatch(e.Name()); m != nil && e.Name() != english && !slices.Contains(langs, m[1]) {
			errs = append(errs, fmt.Sprintf("%s: no upstream %s/%s", e.Name(), upstreamDir, e.Name()))
		}
	}
	slices.Sort(errs)

	if len(warnings) > 0 {
		for _, w := range groupByLanguage(warnings) {
			fmt.Println("warning: " + w)
		}
		fmt.Printf("warning: %d translation(s) missing or stale; to translate them: go run build/forkana-locales.go todo <dir> (see \"Translations\" in AGENTS.md)\n", len(warnings))
	}
	for _, e := range errs {
		fmt.Fprintln(os.Stderr, "error: "+e)
	}
	switch {
	case len(errs) > 0:
		return fmt.Errorf("%d error(s) in the Forkana translations; see \"Translations\" in AGENTS.md", len(errs))
	case strict && len(warnings) > 0:
		return fmt.Errorf("-strict: %d translation(s) missing or stale", len(warnings))
	case len(warnings) == 0:
		fmt.Printf("%d languages translate all %d Forkana keys\n", len(langs), len(fk.Keys))
	}
	return nil
}

// groupByLanguage turns "lang: message" lines into "message (langs)" lines, so a
// key missing in every language takes one line.
func groupByLanguage(lines []string) []string {
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

func todo(fk *forkanalocale.Forkana, langs []string, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	total := 0
	for _, lang := range langs {
		fragment, n := forkanalocale.Todo(fk, loadTranslations(lang))
		path := filepath.Join(dir, fileName(lang))
		if n == 0 {
			_ = os.Remove(path)
			continue
		}
		total += n
		header := fmt.Sprintf("# Translate each value into %s, keeping the \"# en:\" lines. Then run: go run build/forkana-locales.go merge %s\n", lang, dir)
		if err := os.WriteFile(path, append([]byte(header), fragment...), 0o644); err != nil {
			return err
		}
		fmt.Printf("%s: %d string(s) to translate\n", path, n)
	}
	if total == 0 {
		fmt.Println("nothing to translate")
	}
	return nil
}

func merge(fk *forkanalocale.Forkana, langs []string, dir string) error {
	for _, lang := range langs {
		path := filepath.Join(dir, fileName(lang))
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		}
		merged := forkanalocale.Merge(loadTranslations(lang), mustParse(path))
		if err := os.WriteFile(filepath.Join(translationDir, fileName(lang)), forkanalocale.FormatTranslations(lang, fk, merged), 0o644); err != nil {
			return err
		}
		fmt.Printf("merged %s\n", path)
	}
	return nil
}
