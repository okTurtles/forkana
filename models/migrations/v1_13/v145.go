// Copyright 2020 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package v1_13

import (
	"fmt"

	"code.gitea.io/gitea/modules/setting"

	"xorm.io/xorm"
)

func IncreaseLanguageField(x *xorm.Engine) error {
	type LanguageStat struct {
		RepoID   int64  `xorm:"UNIQUE(s) INDEX NOT NULL"`
		Language string `xorm:"VARCHAR(50) UNIQUE(s) INDEX NOT NULL"`
	}

	if err := x.Sync(new(LanguageStat)); err != nil {
		return err
	}

	if setting.Database.Type.IsSQLite3() {
		// SQLite maps VARCHAR to TEXT without size so we're done
		return nil
	}

	// need to get the correct type for the new column
	inferredTable, err := x.TableInfo(new(LanguageStat))
	if err != nil {
		return err
	}
	column := inferredTable.GetColumn("language")
	sqlType := x.Dialect().SQLType(column)

	sess := x.NewSession()
	defer sess.Close()
	if err := sess.Begin(); err != nil {
		return err
	}

	switch {
	case setting.Database.Type.IsMySQL():
		if _, err := sess.Exec("ALTER TABLE language_stat MODIFY COLUMN language " + sqlType); err != nil {
			return err
		}
	case setting.Database.Type.IsPostgreSQL():
		if _, err := sess.Exec("ALTER TABLE language_stat ALTER COLUMN language TYPE " + sqlType); err != nil {
			return err
		}
	}

	return sess.Commit()
}
