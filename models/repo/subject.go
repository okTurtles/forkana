// Copyright 2025 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"code.gitea.io/gitea/models/db"
	"code.gitea.io/gitea/modules/timeutil"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
	"xorm.io/builder"
)

// Subject represents a repository subject that can be shared across repositories
type Subject struct {
	ID          int64              `xorm:"pk autoincr"`
	Name        string             `xorm:"VARCHAR(255) NOT NULL"`        // Display name (can contain special chars)
	Slug        string             `xorm:"VARCHAR(255) UNIQUE NOT NULL"` // URL-safe slug (globally unique)
	CreatedUnix timeutil.TimeStamp `xorm:"INDEX created"`
	UpdatedUnix timeutil.TimeStamp `xorm:"INDEX updated"`
}

func init() {
	db.RegisterModel(new(Subject))
}

// TableName returns the table name for Subject
func (s *Subject) TableName() string {
	return "subject"
}

// GenerateSlugFromName creates a URL-safe slug from a subject display name
// Examples:
//
//	"The Moon" → "the-moon"
//	"the moon!" → "the-moon"
//	"El Camiño?" → "el-camino"
func GenerateSlugFromName(name string) string {
	// Normalize Unicode (NFD = decompose accents)
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	normalized, _, _ := transform.String(t, name)

	// Convert to lowercase
	slug := strings.ToLower(normalized)

	// Replace spaces and underscores with hyphens
	slug = strings.ReplaceAll(slug, " ", "-")
	slug = strings.ReplaceAll(slug, "_", "-")

	// Remove all characters except alphanumeric and hyphens
	reg := regexp.MustCompile(`[^a-z0-9-]+`)
	slug = reg.ReplaceAllString(slug, "")

	// Collapse multiple consecutive hyphens
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}

	// Trim leading/trailing hyphens
	slug = strings.Trim(slug, "-")

	// Ensure slug is not empty
	if slug == "" {
		slug = "subject"
	}

	// Limit length
	const maxSlugLength = 100
	if len(slug) > maxSlugLength {
		slug = slug[:maxSlugLength]
		slug = strings.TrimRight(slug, "-")
	}

	return slug
}

// CreateSubject creates a new subject with the given name
// Returns ErrSubjectSlugAlreadyExists if a subject with the same slug already exists
func CreateSubject(ctx context.Context, name string) (*Subject, error) {
	if name == "" {
		return nil, errors.New("subject name cannot be empty")
	}

	slug := GenerateSlugFromName(name)

	subject := &Subject{
		Name: name,
		Slug: slug,
	}

	// Use transaction to prevent race conditions
	err := db.WithTx(ctx, func(ctx context.Context) error {
		// Check if slug already exists
		existing := &Subject{Slug: slug}
		has, err := db.GetEngine(ctx).Get(existing)
		if err != nil {
			return err
		}
		if has {
			return ErrSubjectSlugAlreadyExists{Slug: slug, Name: name}
		}

		// Insert the new subject
		if err := db.Insert(ctx, subject); err != nil {
			// Check if it's a unique constraint violation using database-specific error codes
			if db.IsErrDuplicateKey(err) {
				return ErrSubjectSlugAlreadyExists{Slug: slug, Name: name}
			}
			return err
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return subject, nil
}

// GetOrCreateSubject gets an existing subject by slug or creates a new one if it doesn't exist
// This function is idempotent and safe for concurrent use
func GetOrCreateSubject(ctx context.Context, name string) (*Subject, error) {
	if name == "" {
		return nil, nil
	}

	slug := GenerateSlugFromName(name)

	// Try to get existing subject by slug
	subject := &Subject{Slug: slug}
	has, err := db.GetEngine(ctx).Get(subject)
	if err != nil {
		return nil, err
	}
	if has {
		return subject, nil
	}

	// Create new subject
	subject = &Subject{
		Name: name,
		Slug: slug,
	}

	if err := db.Insert(ctx, subject); err != nil {
		// Handle race condition: another process might have created it
		// Try to get it again by slug
		subject = &Subject{Slug: slug}
		has, err := db.GetEngine(ctx).Get(subject)
		if err != nil {
			return nil, err
		}
		if has {
			return subject, nil
		}
		return nil, fmt.Errorf("failed to create subject: %w", err)
	}

	return subject, nil
}

// GetSubjectByID gets a subject by its ID
func GetSubjectByID(ctx context.Context, id int64) (*Subject, error) {
	subject := &Subject{ID: id}
	has, err := db.GetEngine(ctx).Get(subject)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, ErrSubjectNotExist{ID: id}
	}
	return subject, nil
}

// GetSubjectByName gets a subject by its name (exact match)
func GetSubjectByName(ctx context.Context, name string) (*Subject, error) {
	subject := &Subject{Name: name}
	has, err := db.GetEngine(ctx).Get(subject)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, ErrSubjectNotExist{Name: name}
	}
	return subject, nil
}

// GetSubjectBySlug gets a subject by its slug
func GetSubjectBySlug(ctx context.Context, slug string) (*Subject, error) {
	subject := &Subject{Slug: slug}
	has, err := db.GetEngine(ctx).Get(subject)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, ErrSubjectNotExist{Name: slug}
	}
	return subject, nil
}

// UpdateSubject updates a subject's properties
func UpdateSubject(ctx context.Context, subject *Subject) error {
	_, err := db.GetEngine(ctx).ID(subject.ID).AllCols().Update(subject)
	return err
}

// DeleteSubject deletes a subject (only if no repositories reference it)
func DeleteSubject(ctx context.Context, id int64) error {
	// Check if any repositories reference this subject
	count, err := db.GetEngine(ctx).Where("subject_id = ?", id).Count(new(Repository))
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrSubjectInUse{ID: id, RepoCount: count}
	}

	_, err = db.GetEngine(ctx).ID(id).Delete(new(Subject))
	return err
}

// FindSubjects finds subjects based on options
func FindSubjects(ctx context.Context, opts FindSubjectsOptions) ([]*Subject, int64, error) {
	sess := db.GetEngine(ctx).Where(opts.ToConds())

	// Apply sorting
	if opts.OrderBy != "" {
		sess = sess.OrderBy(opts.OrderBy)
	} else {
		// Default sort by updated time descending
		sess = sess.OrderBy("updated_unix DESC")
	}

	if opts.PageSize > 0 {
		sess = db.SetSessionPagination(sess, &opts.ListOptions)
	}

	subjects := make([]*Subject, 0, opts.PageSize)
	count, err := sess.FindAndCount(&subjects)
	return subjects, count, err
}

// FindSubjectsOptions represents options for finding subjects
type FindSubjectsOptions struct {
	db.ListOptions
	Keyword        string
	OrderBy        string
	ExcludeIDs     []int64 // IDs to exclude from results
	ExactMatchOnly bool    // Only find exact matches
}

// ToConds converts options to database conditions
func (opts FindSubjectsOptions) ToConds() builder.Cond {
	cond := builder.NewCond()
	if opts.Keyword != "" {
		if opts.ExactMatchOnly {
			// Exact match on name
			cond = cond.And(builder.Eq{"LOWER(name)": strings.ToLower(opts.Keyword)})
		} else {
			// Fuzzy match using LIKE
			cond = cond.And(builder.Like{"LOWER(name)", strings.ToLower(opts.Keyword)})
		}
	}
	if len(opts.ExcludeIDs) > 0 {
		cond = cond.And(builder.NotIn("id", opts.ExcludeIDs))
	}
	return cond
}

// FindSimilarSubjects finds subjects similar to the given keyword
// It returns subjects that partially match the keyword, excluding exact matches
func FindSimilarSubjects(ctx context.Context, keyword string, limit int, excludeIDs []int64) ([]*Subject, error) {
	if keyword == "" {
		return nil, nil
	}

	keyword = strings.ToLower(strings.TrimSpace(keyword))

	// Find subjects that contain the keyword but are not exact matches
	// Fetch more results than needed for better scoring, then trim to limit after sorting
	fetchLimit := limit * 2
	subjects := make([]*Subject, 0, fetchLimit)
	sess := db.GetEngine(ctx).
		Where("LOWER(name) LIKE ? AND LOWER(name) != ?", "%"+keyword+"%", keyword)
	if len(excludeIDs) > 0 {
		sess = sess.NotIn("id", excludeIDs)
	}
	err := sess.OrderBy("updated_unix DESC").
		Limit(fetchLimit).
		Find(&subjects)
	if err != nil {
		return nil, err
	}

	// Calculate similarity scores and sort by relevance
	type subjectWithScore struct {
		subject *Subject
		score   int
	}

	scoredSubjects := make([]subjectWithScore, 0, len(subjects))
	for _, subject := range subjects {
		score := calculateSimilarityScore(keyword, strings.ToLower(subject.Name))
		scoredSubjects = append(scoredSubjects, subjectWithScore{subject, score})
	}

	// Sort by score (lower is better) using O(n log n) algorithm
	slices.SortFunc(scoredSubjects, func(a, b subjectWithScore) int {
		return a.score - b.score
	})

	// Extract sorted subjects, trimmed to original limit
	resultLimit := min(len(scoredSubjects), limit)
	result := make([]*Subject, 0, resultLimit)
	for i := range resultLimit {
		result = append(result, scoredSubjects[i].subject)
	}

	return result, nil
}

// calculateSimilarityScore calculates a similarity score between keyword and subject name
// Lower score means more similar
// 1 = starts with keyword, 2 = contains keyword at word boundary, 3 = contains keyword anywhere
func calculateSimilarityScore(keyword, subjectName string) int {
	keyword = strings.ToLower(keyword)
	subjectName = strings.ToLower(subjectName)

	// Check if subject name starts with keyword
	if strings.HasPrefix(subjectName, keyword) {
		return 1
	}

	// Check if keyword appears at word boundary
	words := strings.FieldsSeq(subjectName)
	for word := range words {
		if strings.HasPrefix(word, keyword) {
			return 2
		}
	}

	// Keyword is contained somewhere in the name
	return 3
}

// SubjectSortType represents the sort type for subjects
type SubjectSortType string

const (
	SubjectSortAlphabetically SubjectSortType = "alphabetically"
	SubjectSortAlphaReverse   SubjectSortType = "reversealphabetically"
	SubjectSortNewest         SubjectSortType = "newest"
	SubjectSortOldest         SubjectSortType = "oldest"
	SubjectSortRecentUpdate   SubjectSortType = "recentupdate"
	SubjectSortLeastUpdate    SubjectSortType = "leastupdate"
)

// SubjectOrderByMap maps sort types to database ORDER BY clauses
var SubjectOrderByMap = map[SubjectSortType]string{
	SubjectSortAlphabetically: "name ASC",
	SubjectSortAlphaReverse:   "name DESC",
	SubjectSortNewest:         "created_unix DESC",
	SubjectSortOldest:         "created_unix ASC",
	SubjectSortRecentUpdate:   "updated_unix DESC",
	SubjectSortLeastUpdate:    "updated_unix ASC",
}

// CountRepositoriesBySubject counts the number of repositories for a given subject
func CountRepositoriesBySubject(ctx context.Context, subjectID int64) (int64, error) {
	return db.GetEngine(ctx).Where("subject_id = ?", subjectID).Count(new(Repository))
}

// CountRootRepositoriesBySubject counts the number of root (non-fork, non-empty) repositories for a given subject.
// Only non-empty repositories are considered as potential roots because the first-article-becomes-root
// logic should only trigger when a user commits content, not when they create an empty repository.
func CountRootRepositoriesBySubject(ctx context.Context, subjectID int64) (int64, error) {
	return db.GetEngine(ctx).Where("subject_id = ? AND is_fork = ? AND is_empty = ?", subjectID, false, false).Count(new(Repository))
}

// SubjectRepoCounts holds repository counts for a subject
type SubjectRepoCounts struct {
	SubjectID     int64
	RepoCount     int64
	RootRepoCount int64
}

// BatchCountRepositoriesBySubjects counts repositories for multiple subjects in a single query.
// It returns a map of subject ID to SubjectRepoCounts containing both total repository count
// and root (non-fork, non-empty) repository count for each subject.
//
// Note: If a subject ID doesn't exist in the database or has no repositories, the returned
// SubjectRepoCounts will have zero values for RepoCount and RootRepoCount. This is intentional
// behavior to allow callers to handle missing subjects gracefully. Callers should validate
// subject existence separately if they need to distinguish between "subject exists with zero
// repos" and "subject doesn't exist".
func BatchCountRepositoriesBySubjects(ctx context.Context, subjectIDs []int64) (map[int64]*SubjectRepoCounts, error) {
	if len(subjectIDs) == 0 {
		return make(map[int64]*SubjectRepoCounts), nil
	}

	// Initialize result map with zero counts for all requested subjects
	result := make(map[int64]*SubjectRepoCounts, len(subjectIDs))
	for _, id := range subjectIDs {
		result[id] = &SubjectRepoCounts{SubjectID: id}
	}

	// Count all repositories per subject
	type countResult struct {
		SubjectID int64 `xorm:"subject_id"`
		Count     int64 `xorm:"count"`
	}

	var allCounts []countResult
	err := db.GetEngine(ctx).
		Table("repository").
		Select("subject_id, COUNT(*) as count").
		In("subject_id", subjectIDs).
		GroupBy("subject_id").
		Find(&allCounts)
	if err != nil {
		return nil, fmt.Errorf("count all repositories: %w", err)
	}

	for _, c := range allCounts {
		if counts, ok := result[c.SubjectID]; ok {
			counts.RepoCount = c.Count
		}
	}

	// Count root (non-fork) repositories per subject
	var rootCounts []countResult
	err = db.GetEngine(ctx).
		Table("repository").
		Select("subject_id, COUNT(*) as count").
		In("subject_id", subjectIDs).
		And("is_fork = ?", false).
		GroupBy("subject_id").
		Find(&rootCounts)
	if err != nil {
		return nil, fmt.Errorf("count root repositories: %w", err)
	}

	for _, c := range rootCounts {
		if counts, ok := result[c.SubjectID]; ok {
			counts.RootRepoCount = c.Count
		}
	}

	return result, nil
}

// ErrSubjectNotExist represents a "SubjectNotExist" error
type ErrSubjectNotExist struct {
	ID   int64
	Name string
}

// IsErrSubjectNotExist checks if an error is ErrSubjectNotExist
func IsErrSubjectNotExist(err error) bool {
	_, ok := err.(ErrSubjectNotExist)
	return ok
}

func (err ErrSubjectNotExist) Error() string {
	if err.Name != "" {
		return fmt.Sprintf("subject does not exist [name: %s]", err.Name)
	}
	return fmt.Sprintf("subject does not exist [id: %d]", err.ID)
}

// ErrSubjectInUse represents a "SubjectInUse" error
type ErrSubjectInUse struct {
	ID        int64
	RepoCount int64
}

// IsErrSubjectInUse checks if an error is ErrSubjectInUse
func IsErrSubjectInUse(err error) bool {
	_, ok := err.(ErrSubjectInUse)
	return ok
}

func (err ErrSubjectInUse) Error() string {
	return fmt.Sprintf("subject is in use by %d repositories [id: %d]", err.RepoCount, err.ID)
}

// ErrSubjectSlugAlreadyExists represents a "SubjectSlugAlreadyExists" error
type ErrSubjectSlugAlreadyExists struct {
	Slug string
	Name string
}

// IsErrSubjectSlugAlreadyExists checks if an error is ErrSubjectSlugAlreadyExists
func IsErrSubjectSlugAlreadyExists(err error) bool {
	_, ok := err.(ErrSubjectSlugAlreadyExists)
	return ok
}

func (err ErrSubjectSlugAlreadyExists) Error() string {
	return fmt.Sprintf("subject slug already exists [slug: %s, name: %s]", err.Slug, err.Name)
}
