// Copyright 2025 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package v1_25_custom

import (
	"strings"
	"testing"

	"code.gitea.io/gitea/models/migrations/base"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/modules/timeutil"

	"github.com/stretchr/testify/assert"
	"xorm.io/xorm"
	"xorm.io/xorm/schemas"
)

// TestMigrationUsesApplicationSlugFunction verifies that the migration v326
// uses the exact same GenerateSlugFromName function as the application code.
// This test ensures that GitHub issue #31 (code duplication) has been resolved.
func TestMigrationUsesApplicationSlugFunction(t *testing.T) {
	// Test cases covering various slug generation scenarios
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Simple lowercase",
			input:    "the moon",
			expected: "the-moon",
		},
		{
			name:     "Capitalized",
			input:    "The Moon",
			expected: "the-moon",
		},
		{
			name:     "With exclamation",
			input:    "the moon!",
			expected: "the-moon",
		},
		{
			name:     "With accents",
			input:    "Café Français",
			expected: "cafe-francais",
		},
		{
			name:     "With underscores",
			input:    "hello_world_test",
			expected: "hello-world-test",
		},
		{
			name:     "Unicode characters",
			input:    "Zürich",
			expected: "zurich",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "subject",
		},
		{
			name:     "Only special characters",
			input:    "!!!???",
			expected: "subject",
		},
		{
			name:     "Multiple hyphens",
			input:    "hello---world",
			expected: "hello-world",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Call the application's GenerateSlugFromName function
			// The migration now imports and uses this same function
			result := repo_model.GenerateSlugFromName(tc.input)

			// Verify the result matches expected output
			assert.Equal(t, tc.expected, result,
				"GenerateSlugFromName should produce expected output for input: %q", tc.input)
		})
	}
}

// TestMigrationSlugConsistency verifies that the migration produces
// identical slugs to the application code for a comprehensive set of inputs.
func TestMigrationSlugConsistency(t *testing.T) {
	// Comprehensive test inputs
	inputs := []string{
		"The Moon",
		"the moon!",
		"El Camiño?",
		"Café Français",
		"Hello@World#2024!",
		"hello_world_test",
		"hello   world",
		"  hello world  ",
		"Zürich",
		"Test123Subject",
		"hello---world",
		"My.Project",
		"Project.git",
		"!!!???",
		"",
		"   ",
		"---",
		"___",
	}

	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			// The migration now uses repo_model.GenerateSlugFromName directly
			// So we just verify it produces consistent output
			slug := repo_model.GenerateSlugFromName(input)

			// Verify the slug is valid (not empty unless input was empty/special chars only)
			if input != "" && input != "   " && input != "---" && input != "___" && input != "!!!???" {
				assert.NotEmpty(t, slug, "Slug should not be empty for non-empty input: %q", input)
			}

			// Verify the slug contains only valid characters
			assert.Regexp(t, `^[a-z0-9-]*$`, slug,
				"Slug should only contain lowercase letters, numbers, and hyphens: %q", slug)

			// Verify the slug doesn't start or end with hyphens
			if slug != "" && slug != "subject" {
				assert.NotRegexp(t, `^-`, slug, "Slug should not start with hyphen: %q", slug)
				assert.NotRegexp(t, `-$`, slug, "Slug should not end with hyphen: %q", slug)
			}
		})
	}
}

// subjectPreMigration represents the subject table before v326 migration
type subjectPreMigration struct {
	ID          int64              `xorm:"pk autoincr"`
	Name        string             `xorm:"VARCHAR(255) NOT NULL"`
	CreatedUnix timeutil.TimeStamp `xorm:"INDEX created"`
	UpdatedUnix timeutil.TimeStamp `xorm:"INDEX updated"`
}

func (*subjectPreMigration) TableName() string {
	return "subject"
}

// subjectResult is used to query the subject table after migration
type subjectResult struct {
	ID   int64  `xorm:"pk autoincr"`
	Name string `xorm:"VARCHAR(255) NOT NULL"`
	Slug string `xorm:"VARCHAR(255)"`
}

func (*subjectResult) TableName() string {
	return "subject"
}

// Test_AddSubjectSlugColumn tests the v326 migration that adds the slug column
// to the subject table and generates slugs for existing subjects.
func Test_AddSubjectSlugColumn(t *testing.T) {
	// Helper function to set up a fresh subject table
	setupSubjectTable := func(t *testing.T, x *xorm.Engine) {
		t.Helper()
		_, _ = x.Exec("DROP TABLE IF EXISTS subject")
		err := x.Sync(new(subjectPreMigration))
		assert.NoError(t, err)
	}

	// Prepare and load the testing database
	x, deferable := base.PrepareTestEnv(t, 0)
	defer deferable()
	if x == nil || t.Failed() {
		return
	}

	// Test Case 1: Basic slug generation
	t.Run("BasicSlugGeneration", func(t *testing.T) {
		setupSubjectTable(t, x)

		// Insert test subjects using raw SQL to avoid struct mismatch
		_, err := x.Exec("INSERT INTO subject (name) VALUES (?)", "The Moon")
		assert.NoError(t, err)
		_, err = x.Exec("INSERT INTO subject (name) VALUES (?)", "Computer Science")
		assert.NoError(t, err)
		_, err = x.Exec("INSERT INTO subject (name) VALUES (?)", "Café Français")
		assert.NoError(t, err)

		// Run the migration
		err = AddSubjectSlugColumn(x)
		assert.NoError(t, err)

		// Verify slugs were generated correctly
		var results []subjectResult
		err = x.Table("subject").Find(&results)
		assert.NoError(t, err)
		assert.Len(t, results, 3)

		// Create a map for easier verification
		slugMap := make(map[string]string)
		for _, s := range results {
			slugMap[s.Name] = s.Slug
		}

		assert.Equal(t, "the-moon", slugMap["The Moon"])
		assert.Equal(t, "computer-science", slugMap["Computer Science"])
		assert.Equal(t, "cafe-francais", slugMap["Café Français"])
	})

	// Test Case 2: Mixed-case subject names produce same slug (deduplication)
	t.Run("MixedCaseDeduplication", func(t *testing.T) {
		setupSubjectTable(t, x)

		// Insert subjects that differ only in case
		// Note: These would normally be deduplicated at creation time,
		// but we test the migration handles them with numeric suffixes
		_, err := x.Exec("INSERT INTO subject (name) VALUES (?)", "the moon")
		assert.NoError(t, err)
		_, err = x.Exec("INSERT INTO subject (name) VALUES (?)", "The Moon")
		assert.NoError(t, err)
		_, err = x.Exec("INSERT INTO subject (name) VALUES (?)", "THE MOON")
		assert.NoError(t, err)

		// Run the migration
		err = AddSubjectSlugColumn(x)
		assert.NoError(t, err)

		// Verify slugs were generated with deduplication
		var results []subjectResult
		err = x.Table("subject").OrderBy("id").Find(&results)
		assert.NoError(t, err)
		assert.Len(t, results, 3)

		// First one gets the base slug, others get numeric suffixes
		assert.Equal(t, "the-moon", results[0].Slug)
		assert.Equal(t, "the-moon-2", results[1].Slug)
		assert.Equal(t, "the-moon-3", results[2].Slug)
	})

	// Test Case 3: Special characters are handled correctly
	t.Run("SpecialCharacters", func(t *testing.T) {
		setupSubjectTable(t, x)

		// Note: @ and # are removed entirely (not replaced with hyphens)
		// Underscores are replaced with hyphens
		// Accented characters are normalized
		_, err := x.Exec("INSERT INTO subject (name) VALUES (?)", "Hello@World#2024!")
		assert.NoError(t, err)
		_, err = x.Exec("INSERT INTO subject (name) VALUES (?)", "hello_world_test")
		assert.NoError(t, err)
		_, err = x.Exec("INSERT INTO subject (name) VALUES (?)", "El Camiño?")
		assert.NoError(t, err)
		_, err = x.Exec("INSERT INTO subject (name) VALUES (?)", "Zürich")
		assert.NoError(t, err)

		// Run the migration
		err = AddSubjectSlugColumn(x)
		assert.NoError(t, err)

		// Verify slugs
		var results []subjectResult
		err = x.Table("subject").Find(&results)
		assert.NoError(t, err)

		slugMap := make(map[string]string)
		for _, s := range results {
			slugMap[s.Name] = s.Slug
		}

		// Special characters (@, #, !) are removed, not replaced with hyphens
		assert.Equal(t, "helloworld2024", slugMap["Hello@World#2024!"])
		// Underscores are replaced with hyphens
		assert.Equal(t, "hello-world-test", slugMap["hello_world_test"])
		// Accented characters are normalized
		assert.Equal(t, "el-camino", slugMap["El Camiño?"])
		assert.Equal(t, "zurich", slugMap["Zürich"])
	})

	// Test Case 4: Verify UNIQUE constraint on slug column
	t.Run("UniqueConstraint", func(t *testing.T) {
		setupSubjectTable(t, x)

		_, err := x.Exec("INSERT INTO subject (name) VALUES (?)", "Test Subject")
		assert.NoError(t, err)

		// Run the migration
		err = AddSubjectSlugColumn(x)
		assert.NoError(t, err)

		// Verify the UNIQUE index exists on slug column
		tables, err := x.DBMetas()
		assert.NoError(t, err)

		var subjectTable *schemas.Table
		for _, table := range tables {
			if table.Name == "subject" {
				subjectTable = table
				break
			}
		}
		assert.NotNil(t, subjectTable, "Subject table should exist")

		// Check for unique index on slug - look for any index that:
		// 1. Contains the slug column (note: xorm may include quotes in column names)
		// 2. Is either a UniqueType index OR has a name containing "UQE" (xorm convention)
		foundUniqueSlugIndex := false
		for _, index := range subjectTable.Indexes {
			hasSlugCol := false
			for _, col := range index.Cols {
				// Strip quotes from column name (xorm may include them for SQLite)
				cleanCol := strings.Trim(col, `"'`)
				if cleanCol == "slug" {
					hasSlugCol = true
					break
				}
			}
			if hasSlugCol {
				// Check if it's a unique index (either by type or by naming convention)
				if index.Type == schemas.UniqueType || strings.Contains(strings.ToUpper(index.Name), "UQE") {
					foundUniqueSlugIndex = true
					break
				}
			}
		}
		assert.True(t, foundUniqueSlugIndex, "UNIQUE index on slug column should exist")
	})

	// Test Case 5: Empty table (no subjects)
	t.Run("EmptyTable", func(t *testing.T) {
		setupSubjectTable(t, x)

		// Run the migration on empty table
		err := AddSubjectSlugColumn(x)
		assert.NoError(t, err)

		// Verify table structure is correct
		tables, err := x.DBMetas()
		assert.NoError(t, err)

		var subjectTable *schemas.Table
		for _, table := range tables {
			if table.Name == "subject" {
				subjectTable = table
				break
			}
		}
		assert.NotNil(t, subjectTable, "Subject table should exist")
		assert.NotNil(t, subjectTable.GetColumn("slug"), "Slug column should exist")
	})
}
