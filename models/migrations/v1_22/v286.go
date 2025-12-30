// Copyright 2023 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT
package v1_22

import (
	"errors"
	"fmt"

	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/setting"

	"xorm.io/xorm"
)

func expandHashReferencesToSha256(x *xorm.Engine) error {
	alteredTables := [][2]string{
		{"commit_status", "context_hash"},
		{"comment", "commit_sha"},
		{"pull_request", "merge_base"},
		{"pull_request", "merged_commit_id"},
		{"review", "commit_id"},
		{"review_state", "commit_sha"},
		{"repo_archiver", "commit_id"},
		{"release", "sha1"},
		{"repo_indexer_status", "commit_sha"},
	}

	db := x.NewSession()
	defer db.Close()

	if err := db.Begin(); err != nil {
		return err
	}

	if !setting.Database.Type.IsSQLite3() {
		for _, alts := range alteredTables {
			var err error
			if setting.Database.Type.IsMySQL() {
				_, err = db.Exec(fmt.Sprintf("ALTER TABLE `%s` MODIFY COLUMN `%s` VARCHAR(64)", alts[0], alts[1]))
			} else {
				_, err = db.Exec(fmt.Sprintf("ALTER TABLE `%s` ALTER COLUMN `%s` TYPE VARCHAR(64)", alts[0], alts[1]))
			}
			if err != nil {
				return fmt.Errorf("alter column '%s' of table '%s' failed: %w", alts[1], alts[0], err)
			}
		}
	}
	log.Debug("Updated database tables to hold SHA256 git hash references")

	return db.Commit()
}

func addObjectFormatNameToRepository(x *xorm.Engine) error {
	type Repository struct {
		ObjectFormatName string `xorm:"VARCHAR(6) NOT NULL DEFAULT 'sha1'"`
	}

	if _, err := x.SyncWithOptions(xorm.SyncOptions{
		IgnoreIndices:    true,
		IgnoreConstrains: true,
	}, new(Repository)); err != nil {
		return err
	}

	// Here to catch weird edge-cases where column constraints above are
	// not applied by the DB backend
	_, err := x.Exec("UPDATE `repository` set `object_format_name` = 'sha1' WHERE `object_format_name` = '' or `object_format_name` IS NULL")
	return err
}

func AdjustDBForSha256(x *xorm.Engine) error {
	if err := expandHashReferencesToSha256(x); err != nil {
		return err
	}
	return addObjectFormatNameToRepository(x)
}
