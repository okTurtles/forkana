# article-creator

A command-line tool that creates Gitea/Forkana repositories from Markdown files with YAML front matter.

## Purpose

`article-creator` automates the process of:
- Creating repositories on a Gitea/Forkana instance via the REST API
- Extracting metadata from YAML front matter in Markdown files
- Generating URL-safe repository names (slugs) from filenames
- Initializing repositories with README.md content
- Handling rate limiting and duplicate detection
- Providing detailed statistics on the creation process

This tool is designed to work standalone or as part of the Forkana article population workflow (typically after `wiki2md`). See `make populate` in the main README for the combined workflow.

## Installation

### Building from Source

From the repository root:

```bash
make article-creator
```

This will create an `article-creator` binary in the repository root directory.

### Manual Build

```bash
cd custom/services/article-creator
go build -o article-creator .
```

## Usage

### Basic Usage

Create repositories from a single Markdown file:

```bash
./article-creator --url https://gitea.example.com --token YOUR_API_TOKEN --input article.md
```

Create repositories from all Markdown files in a directory:

```bash
./article-creator --url https://gitea.example.com --token YOUR_API_TOKEN --input ./articles/
```

### Using Environment Variables

Set configuration via environment variables:

```bash
export GITEA_URL=https://gitea.example.com
export GITEA_API_TOKEN=YOUR_API_TOKEN
./article-creator --input ./articles/
```

### Create Private Repositories

```bash
./article-creator --url https://gitea.example.com --token YOUR_API_TOKEN --input ./articles/ --private
```

### Adjust Rate Limiting

Create repositories with a 1-second delay between API calls:

```bash
./article-creator --url https://gitea.example.com --token YOUR_API_TOKEN --input ./articles/ --delay 1s
```

## Command-Line Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--url` | string | `""` | Gitea instance URL (e.g., https://gitea.example.com) |
| `--token` | string | `""` | API token with repository creation permissions |
| `--input` | string | `""` | Path to Markdown file or directory containing Markdown files |
| `--private` | bool | `false` | Create private repositories (default: public) |
| `--delay` | duration | `500ms` | Delay between API calls to avoid rate limiting |

## Environment Variables

The tool supports the following environment variables (command-line flags take precedence):

| Variable | Description |
|----------|-------------|
| `GITEA_URL` | Gitea instance URL |
| `GITEA_API_TOKEN` | API token for authentication |
| `GITEA_INPUT_PATH` | Path to input file or directory |
| `GITEA_PRIVATE` | Set to "true" to create private repositories |
| `GITEA_DELAY` | Delay between API calls (e.g., "500ms", "1s") |

## Required Configuration

### API Token

You need a Gitea API token with repository creation permissions:

1. Log in to your Gitea/Forkana instance
2. Go to Settings → Applications → Generate New Token
3. Select the following permissions:
   - `write:repository` (to create repositories)
   - `write:user` (to create repositories under your account)
4. Copy the generated token

### Gitea Instance URL

The URL should be the base URL of your Gitea instance (without trailing slash):
- ✅ `https://gitea.example.com`
- ✅ `http://localhost:3000`
- ❌ `https://gitea.example.com/`
- ❌ `https://gitea.example.com/api/v1`

## Expected Input Format

### Markdown File with YAML Front Matter

```markdown
---
title: "Article Title"
source: https://example.com/article
license: CC BY-SA 4.0
attribution: Author Name
fetched_at: 2025-11-17T16:00:00Z
---

# Article Title

Article content goes here...
```

### Required Front Matter Fields

- **title**: Used as the repository description and README title

### Filename to Repository Name Conversion

The tool converts filenames to URL-safe repository names (slugs):

| Filename | Repository Name |
|----------|-----------------|
| `My_Article.md` | `my-article` |
| `Physics & Chemistry.md` | `physics-chemistry` |
| `Article (2024).md` | `article-2024` |

Rules:
- Converts to lowercase
- Replaces spaces and underscores with hyphens
- Removes special characters
- Collapses multiple hyphens into one
- Trims leading/trailing hyphens

## Output

### Console Output

The tool provides real-time progress updates:

```
Validating connection to https://gitea.example.com...
Connected as: username

Processing: article1.md
  → Repository: my-article
  ✓ Created repository
  ✓ Initialized with README.md

Processing: article2.md
  → Repository: another-article
  ⚠ Repository already exists, skipping

Summary:
  Processed: 2
  Created:   1
  Skipped:   1
  Failed:    0
```

### Statistics

At the end of processing, the tool displays:
- **Processed**: Total number of files processed
- **Created**: Number of repositories successfully created
- **Skipped**: Number of repositories skipped (already exist)
- **Failed**: Number of repositories that failed to create

## Examples

### Example 1: Single File

```bash
./article-creator \
  --url https://gitea.example.com \
  --token abc123def456 \
  --input my_article.md
```

### Example 2: Directory of Files

```bash
./article-creator \
  --url https://gitea.example.com \
  --token abc123def456 \
  --input ./wikipedia_articles/
```

### Example 3: Using Environment Variables

```bash
export GITEA_URL=https://gitea.example.com
export GITEA_API_TOKEN=abc123def456
./article-creator --input ./articles/ --private
```

### Example 4: Conservative Rate Limiting

```bash
./article-creator \
  --url https://gitea.example.com \
  --token abc123def456 \
  --input ./articles/ \
  --delay 2s
```

## Features

- **Batch Processing**: Process single files or entire directories
- **YAML Front Matter Extraction**: Automatically extracts metadata from Markdown files
- **Duplicate Detection**: Skips repositories that already exist
- **Rate Limiting**: Configurable delays between API calls
- **Error Handling**: Continues processing even if individual files fail
- **Progress Tracking**: Real-time updates on processing status
- **Statistics**: Detailed summary of the creation process
- **Environment Variable Support**: Flexible configuration options

## Error Handling

The tool handles various error conditions gracefully:

- **Connection Errors**: Validates connection before processing
- **Authentication Errors**: Checks API token validity
- **Duplicate Repositories**: Skips existing repositories instead of failing
- **Invalid Files**: Logs errors and continues with remaining files
- **Rate Limiting**: Respects configured delays to avoid API throttling

## Notes

- The tool creates repositories under the authenticated user's account
- Repository names are automatically generated from filenames (URL-safe slugs)
- The default delay (500ms) is conservative; adjust based on your instance's rate limits
- Private repositories require appropriate permissions on the API token
- The tool does not delete or modify existing repositories

## License

Copyright 2025 The Gitea Authors. All rights reserved.
SPDX-License-Identifier: MIT

