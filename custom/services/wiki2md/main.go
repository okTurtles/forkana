// Copyright 2025 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

// wiki2md fetches Wikipedia articles and converts them to Markdown format.
//
// This tool fetches articles from Wikipedia using the MediaWiki API,
// converts them to Markdown, and saves them with YAML front matter.
// It supports both random article selection and category-based fetching.
//
// Usage:
//
//	wiki2md --out output_dir --count 100 --category "Category:Physics"
//	wiki2md --out output_dir --count 50
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
)

const (
	wikiAPI  = "https://en.wikipedia.org/w/api.php"
	wikiREST = "https://en.wikipedia.org/api/rest_v1"
)

const userAgent = "wiki2md/1.0 (Gitea; +https://github.com/go-gitea/gitea)"

var (
	client = &http.Client{Timeout: 30 * time.Second}

	// Pre-compiled regexes for safeFilename (Issue 4: avoid recompiling in hot path)
	safeFilenameRE = regexp.MustCompile(`[^\w.\- ]+`)
	multiSpaceRE   = regexp.MustCompile(`[_\s]+`)
)

// processResult represents the outcome of processing an article
type processResult int

const (
	resultSuccess  processResult = iota // Article was successfully converted
	resultSkipped                       // Article was skipped (redirect or empty)
	resultError                         // Article processing failed with an error
)

// skipReason describes why an article was skipped
type skipReason string

const (
	skipRedirect     skipReason = "redirect"
	skipEmptyContent skipReason = "empty_content"
)

type config struct {
	outputDir     string
	count         int
	category      string
	sleepInterval time.Duration
}

type articleRecord struct {
	Title     string `json:"title"`
	Source    string `json:"source"`
	SavedAs   string `json:"saved_as"`
	FetchedAt string `json:"fetched_at"`
}

func main() {
	cfg := config{}
	flag.StringVar(&cfg.outputDir, "out", "out_md", "Output directory for Markdown files")
	flag.IntVar(&cfg.count, "count", 1000, "Number of articles to fetch")
	flag.StringVar(&cfg.category, "category", "", "Wikipedia category to fetch from (e.g., 'Category:Physics')")
	flag.DurationVar(&cfg.sleepInterval, "sleep", 100*time.Millisecond, "Sleep duration between API requests")
	flag.Parse()

	if err := run(cfg); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

func run(cfg config) error {
	// Create output directory
	if err := os.MkdirAll(cfg.outputDir, 0o755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Discover article titles
	var titles []string
	var err error
	if cfg.category != "" {
		titles, err = getCategoryMembers(cfg.category, cfg.count, cfg.sleepInterval)
		if err != nil {
			return fmt.Errorf("failed to get category members: %w", err)
		}
		// Top up with random articles if category is small
		if len(titles) < cfg.count {
			needed := cfg.count - len(titles)
			randomTitles, err := getRandomTitles(needed, cfg.sleepInterval)
			if err != nil {
				return fmt.Errorf("failed to get random titles: %w", err)
			}
			titles = append(titles, randomTitles...)
		}
	} else {
		titles, err = getRandomTitles(cfg.count, cfg.sleepInterval)
		if err != nil {
			return fmt.Errorf("failed to get random titles: %w", err)
		}
	}

	// Deduplicate and filter redirects
	titles = deduplicateTitles(titles)

	// Open index file
	indexPath := filepath.Join(cfg.outputDir, "index.jsonl")
	indexFile, err := os.OpenFile(indexPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open index file: %w", err)
	}
	defer indexFile.Close()

	// Open error log
	errorLogPath := filepath.Join(cfg.outputDir, "errors.log")
	errorLog, err := os.OpenFile(errorLogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open error log: %w", err)
	}
	defer errorLog.Close()

	// Open skip log for tracking skipped articles
	skipLogPath := filepath.Join(cfg.outputDir, "skipped.log")
	skipLog, err := os.OpenFile(skipLogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open skip log: %w", err)
	}
	defer skipLog.Close()

	// Fetch and convert articles with detailed tracking
	var stats struct {
		converted int
		skipped   int
		errors    int
		redirects int
		empty     int
	}

	for i, title := range titles {
		result, reason, err := processArticle(title, cfg.outputDir, indexFile)

		switch result {
		case resultSuccess:
			stats.converted++
		case resultSkipped:
			stats.skipped++
			fmt.Fprintf(skipLog, "%s\t%s\n", title, reason)
			switch reason {
			case skipRedirect:
				stats.redirects++
			case skipEmptyContent:
				stats.empty++
			}
		case resultError:
			stats.errors++
			fmt.Fprintf(errorLog, "%s\t%v\n", title, err)
		}

		if i < len(titles)-1 {
			time.Sleep(cfg.sleepInterval)
		}
	}

	// Print summary
	fmt.Printf("Done. Processed %d articles in: %s\n", len(titles), cfg.outputDir)
	fmt.Printf("  Converted: %d\n", stats.converted)
	fmt.Printf("  Skipped:   %d (redirects: %d, empty: %d)\n", stats.skipped, stats.redirects, stats.empty)
	if stats.errors > 0 {
		fmt.Printf("  Errors:    %d (see %s)\n", stats.errors, errorLogPath)
	}
	return nil
}

// processArticle fetches and converts a Wikipedia article to Markdown.
// It returns the processing result and any skip reason or error.
func processArticle(title, outputDir string, indexFile io.Writer) (processResult, skipReason, error) {
	// Check if redirect
	isRedir, err := isRedirect(title)
	if err != nil {
		return resultError, "", fmt.Errorf("redirect check failed: %w", err)
	}
	if isRedir {
		return resultSkipped, skipRedirect, nil
	}

	// Fetch HTML
	htmlContent, err := getParsoidHTML(title)
	if err != nil {
		return resultError, "", fmt.Errorf("failed to fetch HTML: %w", err)
	}
	if htmlContent == "" {
		return resultSkipped, skipEmptyContent, nil
	}

	// Convert to Markdown
	md, err := htmlToMarkdown(htmlContent)
	if err != nil {
		return resultError, "", fmt.Errorf("failed to convert to markdown: %w", err)
	}

	// Normalize image URLs
	md = normalizeImageURLs(md)

	// Add front matter
	md = addFrontMatter(title, md)

	// Generate unique filename
	filename, err := writeMarkdown(outputDir, title, md)
	if err != nil {
		return resultError, "", fmt.Errorf("failed to write markdown: %w", err)
	}

	// Write to index
	record := articleRecord{
		Title:     title,
		Source:    fmt.Sprintf("https://en.wikipedia.org/wiki/%s", url.PathEscape(strings.ReplaceAll(title, " ", "_"))),
		SavedAs:   filename,
		FetchedAt: time.Now().UTC().Format("2006-01-02T15:04:05Z"),
	}
	recordJSON, err := json.Marshal(record)
	if err != nil {
		return resultError, "", fmt.Errorf("failed to marshal record: %w", err)
	}
	fmt.Fprintf(indexFile, "%s\n", recordJSON)

	return resultSuccess, "", nil
}

func getRandomTitles(count int, sleepInterval time.Duration) ([]string, error) {
	var titles []string
	batchSize := 100

	for len(titles) < count {
		limit := batchSize
		if count-len(titles) < batchSize {
			limit = count - len(titles)
		}

		params := url.Values{
			"action":      {"query"},
			"list":        {"random"},
			"rnnamespace": {"0"},
			"rnlimit":     {fmt.Sprintf("%d", limit)},
			"format":      {"json"},
		}

		var result struct {
			Query struct {
				Random []struct {
					Title string `json:"title"`
				} `json:"random"`
			} `json:"query"`
		}

		if err := apiRequest(wikiAPI, params, &result); err != nil {
			return nil, fmt.Errorf("random titles API request failed: %w", err)
		}

		for _, r := range result.Query.Random {
			titles = append(titles, r.Title)
		}

		if len(titles) < count {
			time.Sleep(sleepInterval)
		}
	}

	return titles, nil
}

func getCategoryMembers(category string, limit int, sleepInterval time.Duration) ([]string, error) {
	var titles []string
	visited := make(map[string]bool)
	stack := []string{category}

	for len(stack) > 0 && len(titles) < limit {
		cat := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if visited[cat] {
			continue
		}
		visited[cat] = true

		cmcontinue := ""
		for {
			params := url.Values{
				"action":  {"query"},
				"list":    {"categorymembers"},
				"cmtitle": {cat},
				"cmlimit": {"500"},
				"format":  {"json"},
			}
			if cmcontinue != "" {
				params.Set("cmcontinue", cmcontinue)
			}

			var result struct {
				Query struct {
					CategoryMembers []struct {
						NS    int    `json:"ns"`
						Title string `json:"title"`
					} `json:"categorymembers"`
				} `json:"query"`
				Continue struct {
					CMContinue string `json:"cmcontinue"`
				} `json:"continue"`
			}

			if err := apiRequest(wikiAPI, params, &result); err != nil {
				return nil, err
			}

			for _, m := range result.Query.CategoryMembers {
				if m.NS == 14 { // Category
					stack = append(stack, m.Title)
				} else if m.NS == 0 { // Article
					if len(titles) < limit {
						titles = append(titles, m.Title)
					}
				}
			}

			cmcontinue = result.Continue.CMContinue
			if cmcontinue == "" || len(titles) >= limit {
				break
			}
		}

		if len(stack) > 0 || len(titles) < limit {
			time.Sleep(sleepInterval)
		}
	}

	return titles[:min(len(titles), limit)], nil
}

func isRedirect(title string) (bool, error) {
	params := url.Values{
		"action":    {"query"},
		"titles":    {title},
		"redirects": {""},
		"format":    {"json"},
	}

	var result struct {
		Query struct {
			Redirects []struct{} `json:"redirects"`
		} `json:"query"`
	}

	if err := apiRequest(wikiAPI, params, &result); err != nil {
		return false, err
	}

	return len(result.Query.Redirects) > 0, nil
}

func getParsoidHTML(title string) (string, error) {
	urlPath := fmt.Sprintf("%s/page/html/%s", wikiREST, url.PathEscape(strings.ReplaceAll(title, " ", "_")))
	req, err := http.NewRequest("GET", urlPath, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

func htmlToMarkdown(htmlContent string) (string, error) {
	md, err := htmltomarkdown.ConvertString(htmlContent)
	if err != nil {
		return "", err
	}
	return md, nil
}

var imgEmbedRE = regexp.MustCompile(`!\[([^\]]*)\]\(([^)]+)\)`)

func normalizeImageURLs(md string) string {
	return imgEmbedRE.ReplaceAllStringFunc(md, func(match string) string {
		parts := imgEmbedRE.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}

		alt := strings.TrimSpace(parts[1])
		imgURL := strings.TrimSpace(parts[2])

		// Clean up alt text
		alt = strings.TrimPrefix(alt, "./")
		if alt == "" {
			alt = "image"
		}

		// Ensure URL has proper protocol
		if strings.HasPrefix(imgURL, "//") {
			imgURL = "https:" + imgURL
		} else if !strings.HasPrefix(imgURL, "http://") && !strings.HasPrefix(imgURL, "https://") {
			if strings.HasPrefix(imgURL, "/") {
				imgURL = "https://en.wikipedia.org" + imgURL
			} else {
				imgURL = "https://en.wikipedia.org/" + imgURL
			}
		}

		return fmt.Sprintf("![%s](%s)", alt, imgURL)
	})
}

// escapeYAMLString escapes a string for use in a double-quoted YAML value.
// It handles backslashes, quotes, and control characters that could break YAML parsing.
func escapeYAMLString(s string) string {
	// Escape backslashes first, then other special characters
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	return s
}

func addFrontMatter(title, mdBody string) string {
	safeTitle := escapeYAMLString(title)
	sourceURL := fmt.Sprintf("https://en.wikipedia.org/wiki/%s", url.PathEscape(strings.ReplaceAll(title, " ", "_")))
	fetchedAt := time.Now().UTC().Format("2006-01-02T15:04:05Z")

	frontMatter := fmt.Sprintf(`---
title: "%s"
source: "%s"
license: CC BY-SA 4.0
attribution: Wikipedia contributors
fetched_at: %s
---

`, safeTitle, sourceURL, fetchedAt)

	return frontMatter + mdBody
}

// truncateToByteLimit truncates a string to fit within maxBytes while preserving
// valid UTF-8 encoding. It removes runes from the end until the byte length is
// within the limit.
func truncateToByteLimit(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	// Convert to runes and remove from end until we fit
	runes := []rune(s)
	for len(runes) > 0 && len(string(runes)) > maxBytes {
		runes = runes[:len(runes)-1]
	}
	return string(runes)
}

func safeFilename(title string, maxLength int) string {
	// Replace problematic characters with underscores (using pre-compiled regex)
	name := safeFilenameRE.ReplaceAllString(title, "_")
	name = strings.TrimSpace(name)

	if name == "" {
		return "untitled"
	}

	// Replace multiple consecutive underscores/spaces with single underscore (using pre-compiled regex)
	name = multiSpaceRE.ReplaceAllString(name, "_")

	// Remove leading/trailing underscores
	name = strings.Trim(name, "_")

	// Truncate if too long (using Unicode-safe truncation to avoid splitting
	// multi-byte characters, which would result in invalid UTF-8)
	if len(name) > maxLength {
		if idx := strings.LastIndex(name, "."); idx > 0 {
			ext := name[idx:]
			base := name[:idx]
			maxBaseBytes := maxLength - len(ext)
			if maxBaseBytes > 0 {
				base = truncateToByteLimit(base, maxBaseBytes)
			} else {
				// Extension alone exceeds limit, truncate the whole thing
				base = ""
				ext = truncateToByteLimit(ext, maxLength)
			}
			name = base + ext
		} else {
			name = truncateToByteLimit(name, maxLength)
		}
	}

	// Verify the result is valid UTF-8 (should always be true after our processing)
	if !utf8.ValidString(name) {
		return "untitled"
	}

	if name == "" {
		return "untitled"
	}

	return name
}

func getUniqueFilename(outputDir, baseName string) string {
	fname := baseName + ".md"
	path := filepath.Join(outputDir, fname)

	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fname
		}
		// Log unexpected errors but continue (file might exist)
		log.Printf("Warning: unexpected error checking %s: %v", path, err)
	}

	// File exists or error occurred, add counter with bounds checking
	const maxAttempts = 10000
	for counter := 1; counter <= maxAttempts; counter++ {
		fname = fmt.Sprintf("%s_%d.md", baseName, counter)
		path = filepath.Join(outputDir, fname)
		_, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return fname
			}
			// Log unexpected errors but continue
			log.Printf("Warning: unexpected error checking %s: %v", path, err)
		}
	}

	// Fallback with timestamp to ensure uniqueness
	return fmt.Sprintf("%s_%d.md", baseName, time.Now().UnixNano())
}

func writeMarkdown(outputDir, title, md string) (string, error) {
	baseName := safeFilename(title, 200)
	filename := getUniqueFilename(outputDir, baseName)
	path := filepath.Join(outputDir, filename)

	if err := os.WriteFile(path, []byte(md), 0o644); err != nil {
		return "", err
	}

	return filename, nil
}

func deduplicateTitles(titles []string) []string {
	seen := make(map[string]bool)
	var result []string

	for _, title := range titles {
		if !seen[title] {
			seen[title] = true
			result = append(result, title)
		}
	}

	return result
}

func apiRequest(apiURL string, params url.Values, result interface{}) error {
	req, err := http.NewRequest("GET", apiURL+"?"+params.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return json.NewDecoder(resp.Body).Decode(result)
}
