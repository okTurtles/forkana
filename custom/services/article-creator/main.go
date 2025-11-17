// Copyright 2025 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

// article-creator automates repository creation and initialization on a Gitea/Forkana instance.
//
// This tool creates Gitea/Forkana repositories from Markdown files using the Gitea REST API.
// It supports both single file and batch processing modes, extracting metadata from
// YAML front matter and initializing repositories with README.md content.
//
// Usage:
//
//	article-creator --url https://gitea.example.com --token TOKEN --input file.md
//	article-creator --url https://gitea.example.com --token TOKEN --input ./docs/
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type config struct {
	giteaURL   string
	apiToken   string
	inputPath  string
	private    bool
	rateDelay  time.Duration
}

type stats struct {
	processed int
	created   int
	failed    int
	skipped   int
}

type giteaClient struct {
	baseURL    string
	apiToken   string
	httpClient *http.Client
	stats      stats
	rateDelay  time.Duration
}

type createRepoRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Subject     string `json:"subject"`
	Private     bool   `json:"private"`
	AutoInit    bool   `json:"auto_init"`
	Gitignores  string `json:"gitignores"`
	License     string `json:"license"`
	Readme      string `json:"readme"`
}

type createFileRequest struct {
	Message string `json:"message"`
	Content string `json:"content"`
	Branch  string `json:"branch"`
}

type userInfo struct {
	Login string `json:"login"`
}

type repoInfo struct {
	HTMLURL string `json:"html_url"`
}

func main() {
	cfg := config{}
	flag.StringVar(&cfg.giteaURL, "url", os.Getenv("GITEA_URL"), "Gitea instance URL")
	flag.StringVar(&cfg.apiToken, "token", os.Getenv("GITEA_API_TOKEN"), "API token with repository creation permissions")
	flag.StringVar(&cfg.inputPath, "input", os.Getenv("GITEA_INPUT_PATH"), "Path to Markdown file or directory")
	flag.BoolVar(&cfg.private, "private", os.Getenv("GITEA_PRIVATE") == "true", "Create private repositories")
	flag.DurationVar(&cfg.rateDelay, "delay", 500*time.Millisecond, "Delay between API calls")
	flag.Parse()

	// Validate required arguments
	if cfg.giteaURL == "" {
		log.Fatal("Error: --url is required (or set GITEA_URL environment variable)")
	}
	if cfg.apiToken == "" {
		log.Fatal("Error: --token is required (or set GITEA_API_TOKEN environment variable)")
	}
	if cfg.inputPath == "" {
		log.Fatal("Error: --input is required (or set GITEA_INPUT_PATH environment variable)")
	}

	// Parse rate delay from environment if not set via flag
	if !isFlagSet("delay") {
		if delayEnv := os.Getenv("GITEA_DELAY"); delayEnv != "" {
			if d, err := time.ParseDuration(delayEnv); err == nil {
				cfg.rateDelay = d
			}
		}
	}

	if err := run(cfg); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

func isFlagSet(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

func run(cfg config) error {
	client := &giteaClient{
		baseURL:    strings.TrimSuffix(cfg.giteaURL, "/"),
		apiToken:   cfg.apiToken,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		rateDelay:  cfg.rateDelay,
	}

	// Validate connection
	username, err := client.validateConnection()
	if err != nil {
		return fmt.Errorf("connection validation failed: %w", err)
	}
	fmt.Printf("✓ Connected to Gitea as user: %s\n", username)

	// Determine if input is file or directory
	info, err := os.Stat(cfg.inputPath)
	if err != nil {
		return fmt.Errorf("input path error: %w", err)
	}

	var success bool
	if info.IsDir() {
		fmt.Printf("\nProcessing directory: %s\n", cfg.inputPath)
		success, err = client.processDirectory(cfg.inputPath, username, !cfg.private)
	} else {
		fmt.Printf("\nProcessing single file: %s\n", cfg.inputPath)
		success, err = client.processSingleFile(cfg.inputPath, username, !cfg.private)
	}

	if err != nil {
		return err
	}

	// Print summary
	client.printSummary()

	if !success {
		os.Exit(1)
	}

	return nil
}

func (c *giteaClient) validateConnection() (string, error) {
	req, err := http.NewRequest("GET", c.baseURL+"/api/v1/user", nil)
	if err != nil {
		return "", err
	}
	c.setAuthHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("connection error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("authentication failed: %d - %s", resp.StatusCode, string(body))
	}

	var user userInfo
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return "", err
	}

	return user.Login, nil
}

func (c *giteaClient) processSingleFile(filePath, username string, public bool) (bool, error) {
	if !strings.HasSuffix(strings.ToLower(filePath), ".md") {
		return false, fmt.Errorf("file is not a Markdown file: %s", filePath)
	}

	return c.processFile(filePath, username, public), nil
}

func (c *giteaClient) processDirectory(dirPath, username string, public bool) (bool, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return false, err
	}

	var mdFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
			mdFiles = append(mdFiles, filepath.Join(dirPath, entry.Name()))
		}
	}

	if len(mdFiles) == 0 {
		return false, fmt.Errorf("no Markdown files found in directory: %s", dirPath)
	}

	fmt.Printf("Found %d Markdown files to process\n", len(mdFiles))

	success := false
	for i, mdFile := range mdFiles {
		if c.processFile(mdFile, username, public) {
			success = true
		}

		if i < len(mdFiles)-1 {
			time.Sleep(c.rateDelay)
		}
	}

	return success, nil
}

func (c *giteaClient) processFile(filePath, username string, public bool) bool {
	c.stats.processed++

	fmt.Printf("\nProcessing: %s\n", filepath.Base(filePath))

	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Printf("  ✗ Failed to read file: %v\n", err)
		c.stats.failed++
		return false
	}

	// Extract title from YAML front matter
	title := extractYAMLTitle(string(content))
	var description string
	if title != "" {
		description = title
		fmt.Printf("  Article title: %s\n", title)
	} else {
		// Fallback: use filename without extension
		base := filepath.Base(filePath)
		description = strings.TrimSuffix(base, filepath.Ext(base))
		description = strings.ReplaceAll(description, "_", " ")
		description = strings.ReplaceAll(description, "-", " ")
		fmt.Printf("  No YAML title found, using filename as description\n")
	}

	// Create repository slug
	repoName := createSlug(filepath.Base(filePath))
	fmt.Printf("  Repository name: %s\n", repoName)

	// Check if repository already exists
	if c.checkRepoExists(username, repoName) {
		fmt.Printf("  ⚠ Repository '%s' already exists, skipping\n", repoName)
		c.stats.skipped++
		return false
	}

	// Create repository
	repoURL, err := c.createRepository(repoName, description, description, public)
	if err != nil {
		fmt.Printf("  ✗ Failed to create repository: %v\n", err)
		c.stats.failed++
		return false
	}

	// Create README.md file
	if err := c.createReadmeFile(username, repoName, string(content)); err != nil {
		fmt.Printf("  ✗ Failed to create README.md: %v\n", err)
		c.stats.failed++
		return false
	}

	fmt.Printf("  ✓ Repository created successfully: %s\n", repoURL)
	c.stats.created++
	return true
}

func (c *giteaClient) checkRepoExists(username, repoName string) bool {
	url := fmt.Sprintf("%s/api/v1/repos/%s/%s", c.baseURL, username, repoName)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false
	}
	c.setAuthHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

func (c *giteaClient) createRepository(repoName, description, subject string, public bool) (string, error) {
	reqData := createRepoRequest{
		Name:        repoName,
		Description: description,
		Subject:     subject,
		Private:     !public,
		AutoInit:    false,
		Gitignores:  "",
		License:     "",
		Readme:      "",
	}

	jsonData, err := json.Marshal(reqData)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/api/v1/user/repos", c.baseURL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	c.setAuthHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		return "", fmt.Errorf("repository already exists")
	}

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var repo repoInfo
	if err := json.NewDecoder(resp.Body).Decode(&repo); err != nil {
		return "", err
	}

	return repo.HTMLURL, nil
}

func (c *giteaClient) createReadmeFile(username, repoName, content string) error {
	contentB64 := base64.StdEncoding.EncodeToString([]byte(content))

	reqData := createFileRequest{
		Message: "Initial commit: Add README.md",
		Content: contentB64,
		Branch:  "main",
	}

	jsonData, err := json.Marshal(reqData)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/api/v1/repos/%s/%s/contents/README.md", c.baseURL, username, repoName)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	c.setAuthHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (c *giteaClient) setAuthHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
}

func (c *giteaClient) printSummary() {
	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("SUMMARY")
	fmt.Println(strings.Repeat("=", 50))
	fmt.Printf("Files processed: %d\n", c.stats.processed)
	fmt.Printf("Repositories created: %d\n", c.stats.created)
	fmt.Printf("Repositories skipped: %d\n", c.stats.skipped)
	fmt.Printf("Failures: %d\n", c.stats.failed)

	if c.stats.processed > 0 {
		successRate := float64(c.stats.created) / float64(c.stats.processed) * 100
		fmt.Printf("Success rate: %.1f%%\n", successRate)
	}
}

func extractYAMLTitle(content string) string {
	if !strings.HasPrefix(content, "---") {
		return ""
	}

	endIdx := strings.Index(content[3:], "\n---\n")
	if endIdx == -1 {
		endIdx = strings.Index(content[3:], "\n---")
		if endIdx == -1 {
			return ""
		}
	}

	yamlContent := content[3 : 3+endIdx]

	// Match title field
	re := regexp.MustCompile(`(?m)^title:\s*(.+)$`)
	matches := re.FindStringSubmatch(yamlContent)
	if len(matches) < 2 {
		return ""
	}

	title := strings.TrimSpace(matches[1])

	// Handle quoted strings
	if (strings.HasPrefix(title, `"`) && strings.HasSuffix(title, `"`)) ||
		(strings.HasPrefix(title, `'`) && strings.HasSuffix(title, `'`)) {
		title = title[1 : len(title)-1]
		// Unescape quotes
		title = strings.ReplaceAll(title, `\"`, `"`)
		title = strings.ReplaceAll(title, `\'`, `'`)
	}

	return title
}

func createSlug(filename string) string {
	// Remove .md extension if present
	name := filename
	if strings.HasSuffix(strings.ToLower(name), ".md") {
		name = name[:len(name)-3]
	}

	// Convert to lowercase
	slug := strings.ToLower(name)

	// Handle common Unicode characters
	replacements := map[string]string{
		"²": "2", "³": "3", "¹": "1",
		"é": "e", "è": "e", "ê": "e",
		"à": "a", "á": "a", "â": "a",
		"ü": "u", "ö": "o", "ä": "a",
		"ñ": "n", "ç": "c",
	}
	for old, new := range replacements {
		slug = strings.ReplaceAll(slug, old, new)
	}

	// Replace special characters with hyphens
	re := regexp.MustCompile(`[^a-z0-9\-]`)
	slug = re.ReplaceAllString(slug, "-")

	// Collapse multiple consecutive hyphens
	re2 := regexp.MustCompile(`-+`)
	slug = re2.ReplaceAllString(slug, "-")

	// Remove leading/trailing hyphens
	slug = strings.Trim(slug, "-")

	// Ensure slug is not empty
	if slug == "" {
		slug = "untitled-repo"
	}

	return slug
}

