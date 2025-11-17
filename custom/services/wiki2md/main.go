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

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
)

const (
	wikiAPI  = "https://en.wikipedia.org/w/api.php"
	wikiREST = "https://en.wikipedia.org/api/rest_v1"
)

var (
	userAgent = "wiki2md/1.0 (Gitea; +https://github.com/go-gitea/gitea)"
	client    = &http.Client{Timeout: 30 * time.Second}
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

	// Fetch and convert articles
	converted := 0
	for i, title := range titles {
		if err := processArticle(title, cfg.outputDir, indexFile, errorLog); err != nil {
			fmt.Fprintf(errorLog, "%s\t%v\n", title, err)
			continue
		}
		converted++

		if i < len(titles)-1 {
			time.Sleep(cfg.sleepInterval)
		}
	}

	fmt.Printf("Done. Converted %d articles to Markdown in: %s\n", converted, cfg.outputDir)
	return nil
}

func processArticle(title, outputDir string, indexFile, errorLog io.Writer) error {
	// Check if redirect
	isRedir, err := isRedirect(title)
	if err != nil {
		return fmt.Errorf("redirect check failed: %w", err)
	}
	if isRedir {
		return nil // Skip redirects silently
	}

	// Fetch HTML
	htmlContent, err := getParsoidHTML(title)
	if err != nil {
		return fmt.Errorf("failed to fetch HTML: %w", err)
	}
	if htmlContent == "" {
		return nil // Skip empty content silently
	}

	// Convert to Markdown
	md, err := htmlToMarkdown(htmlContent)
	if err != nil {
		return fmt.Errorf("failed to convert to markdown: %w", err)
	}

	// Normalize image URLs
	md = normalizeImageURLs(md)

	// Add front matter
	md = addFrontMatter(title, md)

	// Generate unique filename
	filename, err := writeMarkdown(outputDir, title, md)
	if err != nil {
		return fmt.Errorf("failed to write markdown: %w", err)
	}

	// Write to index
	record := articleRecord{
		Title:     title,
		Source:    fmt.Sprintf("https://en.wikipedia.org/wiki/%s", strings.ReplaceAll(title, " ", "_")),
		SavedAs:   filename,
		FetchedAt: time.Now().UTC().Format("2006-01-02T15:04:05Z"),
	}
	recordJSON, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal record: %w", err)
	}
	fmt.Fprintf(indexFile, "%s\n", recordJSON)

	return nil
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
			return nil, err
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

func addFrontMatter(title, mdBody string) string {
	safeTitle := strings.ReplaceAll(title, `"`, `\"`)
	sourceURL := fmt.Sprintf("https://en.wikipedia.org/wiki/%s", strings.ReplaceAll(title, " ", "_"))
	fetchedAt := time.Now().UTC().Format("2006-01-02T15:04:05Z")

	frontMatter := fmt.Sprintf(`---
title: "%s"
source: %s
license: CC BY-SA 4.0
attribution: Wikipedia contributors
fetched_at: %s
---

`, safeTitle, sourceURL, fetchedAt)

	return frontMatter + mdBody
}

func safeFilename(title string, maxLength int) string {
	// Replace problematic characters with underscores
	re := regexp.MustCompile(`[^\w.\- ]+`)
	name := re.ReplaceAllString(title, "_")
	name = strings.TrimSpace(name)

	if name == "" {
		return "untitled"
	}

	// Replace multiple consecutive underscores/spaces with single underscore
	re2 := regexp.MustCompile(`[_\s]+`)
	name = re2.ReplaceAllString(name, "_")

	// Remove leading/trailing underscores
	name = strings.Trim(name, "_")

	// Truncate if too long
	if len(name) > maxLength {
		if idx := strings.LastIndex(name, "."); idx > 0 {
			ext := name[idx:]
			base := name[:idx]
			if len(base) > maxLength-len(ext)-1 {
				base = base[:maxLength-len(ext)-1]
			}
			name = base + ext
		} else {
			name = name[:maxLength]
		}
	}

	if name == "" {
		return "untitled"
	}

	return name
}

func getUniqueFilename(outputDir, baseName string) string {
	fname := baseName + ".md"
	path := filepath.Join(outputDir, fname)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fname
	}

	// File exists, add counter
	counter := 1
	for {
		fname = fmt.Sprintf("%s_%d.md", baseName, counter)
		path = filepath.Join(outputDir, fname)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return fname
		}
		counter++
	}
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

