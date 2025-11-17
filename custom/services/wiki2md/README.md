# wiki2md

A command-line tool that fetches Wikipedia articles and converts them to Markdown format with YAML front matter.

## Purpose

`wiki2md` automates the process of:
- Fetching articles from Wikipedia using the MediaWiki API
- Converting HTML content to clean Markdown using the Parsoid REST API
- Adding structured YAML front matter with metadata
- Generating an index file for tracking fetched articles
- Handling errors gracefully with detailed logging

This tool is designed to work standalone or as part of the Forkana article population workflow (see `make populate` in the main README).

## Installation

### Building from Source

From the repository root:

```bash
make wiki2md
```

This will create a `wiki2md` binary in the repository root directory.

### Manual Build

```bash
cd custom/services/wiki2md
go build -o wiki2md .
```

## Usage

### Basic Usage

Fetch 10 random Wikipedia articles:

```bash
./wiki2md --out articles --count 10
```

### Fetch from a Specific Category

Fetch 50 articles from the "Physics" category:

```bash
./wiki2md --out physics_articles --count 50 --category "Category:Physics"
```

### Adjust Rate Limiting

Fetch articles with a 500ms delay between requests:

```bash
./wiki2md --out articles --count 100 --sleep 500ms
```

## Command-Line Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--out` | string | `"out_md"` | Output directory for Markdown files |
| `--count` | int | `1000` | Number of articles to fetch |
| `--category` | string | `""` | Wikipedia category to fetch from (e.g., 'Category:Physics'). If empty, fetches random articles |
| `--sleep` | duration | `100ms` | Sleep duration between API requests to avoid rate limiting |

## Output Format

### Directory Structure

```
output_dir/
├── Article_Title_1.md
├── Article_Title_2.md
├── ...
├── index.jsonl
└── errors.log
```

### Markdown File Format

Each article is saved as a Markdown file with YAML front matter:

```markdown
---
title: "Article Title"
source: https://en.wikipedia.org/wiki/Article_Title
license: CC BY-SA 4.0
attribution: Wikipedia contributors
fetched_at: 2025-11-17T16:10:16Z
---

# Article Title

Article content in Markdown format...
```

### YAML Front Matter Fields

- **title**: The article title from Wikipedia
- **source**: Direct URL to the Wikipedia article
- **license**: Content license (always "CC BY-SA 4.0" for Wikipedia)
- **attribution**: Content attribution (always "Wikipedia contributors")
- **fetched_at**: ISO 8601 timestamp of when the article was fetched

### Index File (index.jsonl)

A JSON Lines file tracking all successfully fetched articles:

```json
{"title":"Article Title","source":"https://en.wikipedia.org/wiki/Article_Title","saved_as":"Article_Title.md","fetched_at":"2025-11-17T16:10:16Z"}
```

Each line is a separate JSON object with:
- **title**: Article title
- **source**: Wikipedia URL
- **saved_as**: Filename where the article was saved
- **fetched_at**: Fetch timestamp

### Error Log (errors.log)

Contains error messages for articles that failed to fetch or convert. Empty if all articles were processed successfully.

## Examples

### Example 1: Quick Test

Fetch 5 random articles to test the tool:

```bash
./wiki2md --out test_output --count 5 --sleep 200ms
```

### Example 2: Category-Based Collection

Fetch 100 articles about mathematics:

```bash
./wiki2md --out math_articles --count 100 --category "Category:Mathematics"
```

### Example 3: Large Collection with Conservative Rate Limiting

Fetch 1000 articles with 1 second between requests:

```bash
./wiki2md --out large_collection --count 1000 --sleep 1s
```

## Features

- **Random or Category-Based Fetching**: Choose between random articles or articles from specific Wikipedia categories
- **Recursive Category Traversal**: When fetching from categories, automatically traverses subcategories
- **Redirect Handling**: Automatically skips redirect pages to avoid duplicates
- **Filename Collision Handling**: Generates unique filenames when titles conflict
- **Rate Limiting**: Configurable delays between API requests to respect Wikipedia's rate limits
- **Progress Tracking**: Real-time progress updates during fetching
- **Error Recovery**: Continues processing even if individual articles fail
- **Structured Output**: YAML front matter makes articles easy to parse and process

## Notes

- The tool respects Wikipedia's API rate limits. The default 100ms delay is conservative; adjust as needed.
- Category fetching is recursive and may fetch more articles than requested if the category tree is large.
- Redirect pages are automatically skipped to avoid duplicate content.
- Image URLs in the Markdown are converted to proper links (not embedded images).
- The tool uses Wikipedia's Parsoid REST API for high-quality HTML-to-Markdown conversion.

## License

Copyright 2025 The Gitea Authors. All rights reserved.
SPDX-License-Identifier: MIT

