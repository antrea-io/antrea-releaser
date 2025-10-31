# Antrea Release Notes Generator

An AI-powered tool to generate CHANGELOG drafts for Antrea releases using Google's Gemini API.

## Features

- **AI-Powered Analysis**: Uses Google's Gemini 2.5 Flash to intelligently classify, describe, and rank PRs by importance
- **Historical Consistency**: Automatically reuses descriptions from previous releases for backported fixes
- **Context-Aware**: Includes recent CHANGELOGs as examples to maintain consistent style
- **Smart Ordering**: PRs are automatically sorted by importance within each category (ADDED/CHANGED/FIXED)
- **Cherry-Pick Support**: Handles patch releases with cherry-picked PRs
- **Flexible Output**: Write to stdout or file
- **Model Transparency**: Saves model output and usage details (latency, tokens, estimated cost) as JSON files

## Prerequisites

- Go 1.25 or later
- Google API Key with Gemini API access
- (Optional) GitHub Personal Access Token for higher rate limits

## Setup

1. Clone this repository or copy the files to your local machine

2. Install dependencies:
```bash
go mod download
```

3. Create a `.env` file in the project root:
```bash
cp .env.example .env
```

4. Edit `.env` and add your API keys:
```
GOOGLE_API_KEY=your_google_api_key_here
GITHUB_TOKEN=your_github_token_here  # Optional but recommended
```

## Usage

### Basic Usage

Generate changelog for a release:

```bash
go run ./cmd/prepare-changelog --release 2.5.0
```

### Advanced Options

```bash
# Specify the starting release (auto-calculated if omitted)
go run ./cmd/prepare-changelog --release 2.5.0 --from-release 2.4.0

# Send ALL PRs to the model (not just those with action/release-note label)
go run ./cmd/prepare-changelog --release 2.5.0 --all

# Write output to a file
go run ./cmd/prepare-changelog --release 2.5.0 --output CHANGELOG-draft.md

# Use a different Gemini model
go run ./cmd/prepare-changelog --release 2.5.0 --model gemini-1.5-pro

# Combine with other options
go run ./cmd/prepare-changelog --release 2.5.0 --all --model gemini-1.5-pro

# Patch release example
go run ./cmd/prepare-changelog --release 2.4.1
```

### Build and Install

```bash
# Build the binary using Make
make bin

# Use the binary
./bin/prepare-changelog --release 2.5.0

# Or build manually
go build -o bin/prepare-changelog ./cmd/prepare-changelog
```

## How It Works

1. **Environment Setup**: Loads API keys from `.env` and environment variables
2. **Version Analysis**: Parses release version and determines target branch
3. **Historical Context**: Fetches and parses the 3 most recent CHANGELOGs from GitHub
4. **PR Collection**: Fetches PRs from GitHub based on `--all` flag:
   - Without `--all`: Only PRs with `action/release-note` label
   - With `--all`: All merged PRs (for comprehensive analysis)
   - Cherry-picks are always included for patch releases
   - **Bot PRs are always filtered out** (renovate[bot], dependabot, antrea-bot)
5. **AI Analysis**: Sends filtered PR data and historical context to Gemini API for:
   - Classification (ADDED/CHANGED/FIXED)
   - One-sentence descriptions
   - Inclusion scoring (whether to include in CHANGELOG)
   - Importance scoring (for sorting within categories)
6. **Save Model Data**: Saves three files:
   - `changelog-model-prompt-<VERSION>-<TIMESTAMP>.txt`: Full prompt sent to model
   - `changelog-model-output-<VERSION>-<TIMESTAMP>.json`: Complete model response
   - `changelog-model-details-<VERSION>-<TIMESTAMP>.json`: Usage metadata (latency, tokens, cost)
7. **CHANGELOG Generation**: Formats the AI response into standard CHANGELOG format
   - PRs sorted by `importance_score` within each category (highest first)
   - PRs with `include_score >= 50`: Included normally
   - PRs with `include_score 25-49`: Included with `*OPTIONAL*` prefix
   - PRs with `include_score < 25`: Excluded from output (but still in model JSON for troubleshooting)
8. **Output**: Writes to stdout or specified file

## Generated Files

When you run the tool, it creates several files:

### Model Output Files (Always Created)

- **`changelog-model-prompt-<VERSION>-<TIMESTAMP>.txt`**: The complete prompt sent to the Gemini model, including the template, historical CHANGELOGs, and all PR data.

- **`changelog-model-output-<VERSION>-<TIMESTAMP>.json`**: The raw structured JSON response from the Gemini model, containing all PR classifications, descriptions, and confidence scores.

- **`changelog-model-details-<VERSION>-<TIMESTAMP>.json`**: Metadata about the model invocation:
  ```json
  {
    "version": "2.5.0",
    "timestamp": "20250130-143025",
    "model": "gemini-2.5-flash",
    "latency_seconds": 12.45,
    "prompt_tokens": 45000,
    "candidates_tokens": 3500,
    "total_tokens": 48500,
    "estimated_cost_usd": 0.00438
  }
  ```

All three files share the same timestamp for easy correlation.

### CHANGELOG Output (Optional)

- **Stdout** (default): The formatted CHANGELOG is printed to stdout
- **Custom file** (with `--output` flag): Writes the CHANGELOG to the specified file

All JSON files are automatically ignored by Git.

## Configuration Options

### Command-Line Flags

- `--release` (required): Target release version (e.g., "2.5.0")
- `--from-release` (optional): Starting release version (auto-calculated if omitted)
- `--all` (optional): Send ALL PRs to the model for analysis, not just those with `action/release-note` label (default: false)
- `--output` (optional): Output file path (default: stdout)
- `--model` (optional): Gemini model to use (default: "gemini-2.5-flash", must start with "gemini-")

### Supported Gemini Models

You can use any Gemini model that supports structured JSON output. Common options:
- `gemini-2.5-flash` (default) - Fast, cost-effective
- `gemini-1.5-pro` - More capable, higher quality
- `gemini-1.5-flash` - Older version

The model name must start with `gemini-` or the program will fail with an error.

## CHANGELOG Format

The generated CHANGELOG follows the format:

```markdown
# Changelog X.Y (for minor releases only)

## X.Y.Z - YYYY-MM-DD

### Added

- Description. ([#123](url), [@author])

### Changed

- Description. ([#456](url), [@author])

### Fixed

- Description. ([#789](url), [@author])

[@author]: https://github.com/author
```

## Customizing the Prompt

The AI prompt template is stored in `PROMPT.md`. You can edit this file to:
- Adjust classification guidelines
- Modify description style preferences
- Add project-specific context
- Change output format instructions

## Rate Limits

- **GitHub API**: Unauthenticated requests have a low rate limit (60/hour). Using a `GITHUB_TOKEN` increases this to 5000/hour.
- **Gemini API**: Check your Google Cloud project quotas for API limits.

## Troubleshooting

### "GOOGLE_API_KEY environment variable is required"
Make sure you have created a `.env` file with your Google API key, or export it in your shell:
```bash
export GOOGLE_API_KEY=your_key_here
```

### "failed to get release start time"
This usually means the `from-release` tag doesn't exist. Verify the tag exists:
```bash
git ls-remote --tags https://github.com/antrea-io/antrea | grep v2.4.0
```

### GitHub Rate Limit Errors
Add a `GITHUB_TOKEN` to your `.env` file to increase rate limits.

## License

Licensed under the Apache License, Version 2.0. See the Antrea project for full license details.

## Contributing

This tool is part of the Antrea release process. For bugs or feature requests, please open an issue in the Antrea repository.

