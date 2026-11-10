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
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"code.gitea.io/gitea/build/forkanalocale"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	r, err := forkanalocale.OpenRepository(".")
	if err == nil {
		err = run(r, os.Args[1], os.Args[2:])
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

func run(r *forkanalocale.Repository, command string, args []string) error {
	fs := flag.NewFlagSet(command, flag.ExitOnError)
	fs.Usage = usage
	strict := fs.Bool("strict", false, "check: fail on missing or stale translations too")
	_ = fs.Parse(args)
	dir := func() string {
		if fs.NArg() != 1 {
			usage()
		}
		return fs.Arg(0)
	}
	if (command != "check" && *strict) || (command != "todo" && command != "merge" && fs.NArg() > 0) {
		usage()
	}

	switch command {
	case "generate":
		if err := r.Generate(); err != nil {
			return err
		}
		fmt.Printf("generated %d locale files from %d Forkana keys\n", len(r.Languages), len(r.Forkana.Keys))
	case "check":
		return check(r, *strict)
	case "todo":
		counts, err := r.Todo(dir())
		if err != nil {
			return err
		}
		for _, lang := range r.Languages {
			if n := counts[lang]; n > 0 {
				fmt.Printf("%s: %d string(s) to translate\n", forkanalocale.FileName(lang), n)
			}
		}
		if len(counts) == 0 {
			fmt.Println("nothing to translate")
		}
	case "merge":
		untranslated, err := r.Merge(dir())
		if err != nil {
			return err
		}
		for _, lang := range r.Languages {
			if keys := untranslated[lang]; len(keys) > 0 {
				fmt.Printf("%s: left untranslated (value still the English): %s\n", lang, strings.Join(keys, ", "))
			}
		}
		fmt.Printf("merged and generated %d locale files from %d Forkana keys\n", len(r.Languages), len(r.Forkana.Keys))
	default:
		usage()
	}
	return nil
}

// check prints the errors and warnings of every language. Errors fail it;
// warnings (missing or stale translations) only fail it when strict.
func check(r *forkanalocale.Repository, strict bool) error {
	errs, warnings, err := r.Check()
	if err != nil {
		return err
	}
	if len(warnings) > 0 {
		for _, w := range forkanalocale.GroupByLanguage(warnings) {
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
		return errors.New("-strict: translations missing or stale")
	case len(warnings) == 0:
		fmt.Printf("%d languages translate all %d Forkana keys\n", len(r.Languages), len(r.Forkana.Keys))
	}
	return nil
}
