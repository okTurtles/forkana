// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package v1_25_custom

import (
	"fmt"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/modules/log"

	"xorm.io/xorm"
)

// ResluggifySymbolSubjects gives the subjects whose name has no ASCII letter or digit (three
// apostrophes, "東京") the slug repo_model.GenerateSlugFromName now gives them (#401). They all used to get
// the slug "subject" (or "subject-2", "subject-3"… when migration 326 found several), so
// typing any such title opened whichever of them held "subject".
//
// Names and repositories are left alone: only the slug, which no URL shows, changes. A subject
// keeps its slug when the new one is already taken. Moving a subject can free a slug that
// another one now gets ("Subject" kept as "subject-2" by migration 326 while three
// apostrophes held "subject"), so the subjects are walked until nothing moves.
func ResluggifySymbolSubjects(x *xorm.Engine) error {
	type Subject struct {
		ID   int64  `xorm:"pk autoincr"`
		Name string `xorm:"VARCHAR(255) NOT NULL"`
		Slug string `xorm:"VARCHAR(255) UNIQUE NOT NULL"`
	}

	var subjects []*Subject
	if err := x.Table("subject").Asc("id").Find(&subjects); err != nil {
		return fmt.Errorf("failed to fetch subjects: %w", err)
	}

	taken := make(map[string]bool, len(subjects))
	for _, subject := range subjects {
		taken[subject.Slug] = true
	}

	for moved := true; moved; {
		moved = false
		for _, subject := range subjects {
			slug := repo_model.GenerateSlugFromName(subject.Name)
			if slug == subject.Slug || taken[slug] {
				continue
			}
			if _, err := x.Table("subject").ID(subject.ID).Cols("slug").Update(&Subject{Slug: slug}); err != nil {
				return fmt.Errorf("failed to update the slug of subject %d (%q) to %q: %w", subject.ID, subject.Name, slug, err)
			}
			log.Info("Migration v332: subject %d (%q): slug %q → %q", subject.ID, subject.Name, subject.Slug, slug)
			delete(taken, subject.Slug)
			taken[slug] = true
			subject.Slug = slug
			moved = true
		}
	}
	return nil
}
