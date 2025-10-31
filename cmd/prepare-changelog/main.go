// Copyright 2025 Antrea Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/generative-ai-go/genai"
	"github.com/google/go-github/v67/github"
	"github.com/joho/godotenv"
	"golang.org/x/oauth2"
	"google.golang.org/api/option"
)

const (
	repoOwner = "antrea-io"
	repoName  = "antrea"
)

var ignoredAuthors = map[string]bool{
	"renovate[bot]":   true,
	"dependabot":      true,
	"dependabot[bot]": true,
	"antrea-bot":      true,
}

type Config struct {
	GoogleAPIKey string
	GitHubToken  string
	Release      string
	FromRelease  string
	All          bool
	OutputFile   string
	Model        string
}

type PRInfo struct {
	Number   int
	Title    string
	Body     string
	Author   string
	Labels   []string
	MergedAt time.Time
}

type HistoricalPR struct {
	Description string
	Category    string
}

type ChangeEntry struct {
	PRNumber                 int    `json:"pr_number"`
	Category                 string `json:"category"`
	Description              string `json:"description"`
	ConfidenceDescription    int    `json:"confidence_description"`
	ConfidenceClassification int    `json:"confidence_classification"`
	ConfidenceInclude        int    `json:"confidence_include"`
	GroupedWith              []int  `json:"grouped_with"`
	ReusedFromHistory        bool   `json:"reused_from_history"`
	Author                   string `json:"-"`
}

type ModelResponse struct {
	Changes []ChangeEntry `json:"changes"`
}

type ModelDetails struct {
	Version          string  `json:"version"`
	Timestamp        string  `json:"timestamp"`
	Model            string  `json:"model"`
	LatencySeconds   float64 `json:"latency_seconds"`
	PromptTokens     int32   `json:"prompt_tokens,omitempty"`
	CandidatesTokens int32   `json:"candidates_tokens,omitempty"`
	TotalTokens      int32   `json:"total_tokens,omitempty"`
	EstimatedCostUSD float64 `json:"estimated_cost_usd,omitempty"`
}

func main() {
	if err := run(); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

func run() error {
	config, err := loadConfig()
	if err != nil {
		return err
	}

	ctx := context.Background()

	// Parse version information
	version, err := parseVersion(config.Release)
	if err != nil {
		return fmt.Errorf("invalid release version: %w", err)
	}

	// Calculate from-release if not provided
	if config.FromRelease == "" {
		config.FromRelease = calculateFromRelease(version)
	}

	// Determine target branch
	branch := determineBranch(version)

	log.Printf("Generating changelog for %s (from %s, branch: %s)", config.Release, config.FromRelease, branch)

	// Initialize GitHub client
	githubClient := createGitHubClient(ctx, config.GitHubToken)

	// Fetch historical CHANGELOGs
	log.Println("Fetching historical CHANGELOGs...")
	historicalCHANGELOGs, prCache, err := fetchHistoricalCHANGELOGs(ctx, githubClient)
	if err != nil {
		return fmt.Errorf("failed to fetch historical CHANGELOGs: %w", err)
	}
	log.Printf("Found %d historical PR entries", len(prCache))

	// Fetch PR data
	log.Println("Fetching PR data from GitHub...")
	prs, err := fetchPRs(ctx, githubClient, branch, config.FromRelease, version)
	if err != nil {
		return fmt.Errorf("failed to fetch PRs: %w", err)
	}
	log.Printf("Found %d PRs", len(prs))

	// Filter out bot-authored PRs
	prs = filterBotPRs(prs)
	log.Printf("After filtering bot PRs: %d PRs remaining", len(prs))

	// Load prompt template
	promptTemplate, err := os.ReadFile("PROMPT.md")
	if err != nil {
		return fmt.Errorf("failed to read PROMPT.md: %w", err)
	}

	// Build the prompt
	prompt := buildPrompt(string(promptTemplate), historicalCHANGELOGs, prs, prCache)

	// Call Gemini API
	log.Printf("Calling Gemini API (model: %s)...", config.Model)
	modelResponse, modelDetails, err := callGemini(ctx, config.GoogleAPIKey, prompt, config.Release, config.Model)
	if err != nil {
		return fmt.Errorf("failed to call Gemini API: %w", err)
	}
	log.Printf("Received %d change entries from model", len(modelResponse.Changes))
	log.Printf("Model latency: %.2f seconds, Total tokens: %d", modelDetails.LatencySeconds, modelDetails.TotalTokens)

	// Enrich with author information
	for i := range modelResponse.Changes {
		for _, pr := range prs {
			if pr.Number == modelResponse.Changes[i].PRNumber {
				modelResponse.Changes[i].Author = pr.Author
				break
			}
		}
	}

	// Save prompt
	timestamp := modelDetails.Timestamp
	promptFile := fmt.Sprintf("changelog-model-prompt-%s-%s.txt", config.Release, timestamp)
	if err := os.WriteFile(promptFile, []byte(prompt), 0644); err != nil {
		return fmt.Errorf("failed to save prompt: %w", err)
	}
	log.Printf("Prompt saved to %s", promptFile)

	// Save model output JSON
	modelOutputFile := fmt.Sprintf("changelog-model-output-%s-%s.json", config.Release, timestamp)
	if err := saveModelOutput(modelResponse, modelOutputFile); err != nil {
		return fmt.Errorf("failed to save model output: %w", err)
	}
	log.Printf("Model output saved to %s", modelOutputFile)

	// Save model details JSON
	modelDetailsFile := fmt.Sprintf("changelog-model-details-%s-%s.json", config.Release, timestamp)
	if err := saveModelDetails(modelDetails, modelDetailsFile); err != nil {
		return fmt.Errorf("failed to save model details: %w", err)
	}
	log.Printf("Model details saved to %s", modelDetailsFile)

	// Generate CHANGELOG
	changelog := generateChangelog(version, modelResponse, config.All)

	// Output
	if config.OutputFile != "" {
		if err := os.WriteFile(config.OutputFile, []byte(changelog), 0644); err != nil {
			return fmt.Errorf("failed to write output file: %w", err)
		}
		log.Printf("Changelog written to %s", config.OutputFile)
	} else {
		fmt.Print(changelog)
	}

	return nil
}

func loadConfig() (*Config, error) {
	// Try to load .env file
	if _, err := os.Stat(".env"); err == nil {
		if err := godotenv.Load(); err != nil {
			return nil, fmt.Errorf("error loading .env file: %w", err)
		}
	}

	// Parse command-line flags
	release := flag.String("release", "", "The release for which the changelog is generated (required)")
	fromRelease := flag.String("from-release", "", "The last release from which the changelog is generated (optional)")
	all := flag.Bool("all", false, "Include PRs that are not labelled with 'action/release-note' in a separate section")
	output := flag.String("output", "", "Output file path (default: stdout)")
	model := flag.String("model", "gemini-2.5-flash", "Gemini model to use (must start with 'gemini-')")
	flag.Parse()

	if *release == "" {
		flag.Usage()
		return nil, fmt.Errorf("--release flag is required")
	}

	// Validate model name
	if !strings.HasPrefix(*model, "gemini-") {
		return nil, fmt.Errorf("model must start with 'gemini-', got: %s", *model)
	}

	// Get API keys
	googleAPIKey := os.Getenv("GOOGLE_API_KEY")
	if googleAPIKey == "" {
		return nil, fmt.Errorf("GOOGLE_API_KEY environment variable is required")
	}

	githubToken := os.Getenv("GITHUB_TOKEN")

	return &Config{
		GoogleAPIKey: googleAPIKey,
		GitHubToken:  githubToken,
		Release:      *release,
		FromRelease:  *fromRelease,
		All:          *all,
		OutputFile:   *output,
		Model:        *model,
	}, nil
}

type Version struct {
	Major int
	Minor int
	Patch int
}

func parseVersion(version string) (*Version, error) {
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("version must be in format X.Y.Z")
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid major version: %w", err)
	}

	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid minor version: %w", err)
	}

	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return nil, fmt.Errorf("invalid patch version: %w", err)
	}

	return &Version{Major: major, Minor: minor, Patch: patch}, nil
}

func calculateFromRelease(v *Version) string {
	if v.Patch == 0 {
		// Minor release: previous minor version
		if v.Minor > 0 {
			return fmt.Sprintf("%d.%d.0", v.Major, v.Minor-1)
		}
		// First minor version of major release
		return fmt.Sprintf("%d.0.0", v.Major)
	}
	// Patch release: previous patch version
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch-1)
}

func determineBranch(v *Version) string {
	if v.Patch == 0 {
		return "main"
	}
	return fmt.Sprintf("release-%d.%d", v.Major, v.Minor)
}

func createGitHubClient(ctx context.Context, token string) *github.Client {
	if token != "" {
		ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
		tc := oauth2.NewClient(ctx, ts)
		return github.NewClient(tc)
	}
	return github.NewClient(nil)
}

func fetchHistoricalCHANGELOGs(ctx context.Context, client *github.Client) (string, map[int]HistoricalPR, error) {
	// List contents of CHANGELOG directory
	_, dirContent, _, err := client.Repositories.GetContents(ctx, repoOwner, repoName, "CHANGELOG", nil)
	if err != nil {
		return "", nil, fmt.Errorf("failed to list CHANGELOG directory: %w", err)
	}

	// Find CHANGELOG files and extract version numbers
	type changelogFile struct {
		name    string
		version *Version
	}
	var changelogFiles []changelogFile

	for _, file := range dirContent {
		if file.Name == nil {
			continue
		}
		name := *file.Name
		if !strings.HasPrefix(name, "CHANGELOG-") || !strings.HasSuffix(name, ".md") {
			continue
		}

		// Extract version from filename: CHANGELOG-X.Y.md
		versionStr := strings.TrimPrefix(name, "CHANGELOG-")
		versionStr = strings.TrimSuffix(versionStr, ".md")
		// Parse as X.Y.0
		v, err := parseVersion(versionStr + ".0")
		if err != nil {
			continue
		}

		changelogFiles = append(changelogFiles, changelogFile{name: name, version: v})
	}

	// Sort by version (descending)
	sort.Slice(changelogFiles, func(i, j int) bool {
		vi, vj := changelogFiles[i].version, changelogFiles[j].version
		if vi.Major != vj.Major {
			return vi.Major > vj.Major
		}
		if vi.Minor != vj.Minor {
			return vi.Minor > vj.Minor
		}
		return vi.Patch > vj.Patch
	})

	// Take the 3 most recent
	numToFetch := 3
	if len(changelogFiles) < numToFetch {
		numToFetch = len(changelogFiles)
	}

	var historicalContent strings.Builder
	prCache := make(map[int]HistoricalPR)

	for i := 0; i < numToFetch; i++ {
		file := changelogFiles[i]
		log.Printf("Fetching %s...", file.name)

		// Fetch raw content
		fileContent, _, _, err := client.Repositories.GetContents(ctx, repoOwner, repoName, "CHANGELOG/"+file.name, nil)
		if err != nil {
			return "", nil, fmt.Errorf("failed to fetch %s: %w", file.name, err)
		}

		content, err := fileContent.GetContent()
		if err != nil {
			return "", nil, fmt.Errorf("failed to decode %s: %w", file.name, err)
		}

		historicalContent.WriteString(fmt.Sprintf("\n\n=== %s ===\n\n", file.name))
		historicalContent.WriteString(content)

		// Parse for PR numbers and descriptions
		parseCHANGELOG(content, prCache)
	}

	return historicalContent.String(), prCache, nil
}

func parseCHANGELOG(content string, prCache map[int]HistoricalPR) {
	lines := strings.Split(content, "\n")
	currentCategory := ""

	// Regex to match PR entries: - Description. ([#123](url), [@author])
	prRegex := regexp.MustCompile(`\[#(\d+)\]\(https://github\.com/antrea-io/antrea/pull/\d+\)`)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect category headers
		if strings.HasPrefix(trimmed, "### ") {
			category := strings.TrimPrefix(trimmed, "### ")
			category = strings.ToUpper(strings.TrimSpace(category))
			if category == "ADDED" || category == "CHANGED" || category == "FIXED" {
				currentCategory = category
			}
			continue
		}

		// Parse PR entries
		if strings.HasPrefix(trimmed, "- ") && currentCategory != "" {
			matches := prRegex.FindAllStringSubmatch(line, -1)
			if len(matches) > 0 {
				// Extract PR number
				prNum, err := strconv.Atoi(matches[0][1])
				if err != nil {
					continue
				}

				// Extract description (everything before the first [#
				descEnd := strings.Index(line, "([#")
				if descEnd > 0 {
					description := strings.TrimSpace(line[2:descEnd]) // Skip "- " prefix
					if strings.HasSuffix(description, ".") {
						description = strings.TrimSuffix(description, ".")
					}

					// Only store if not already present (first occurrence wins)
					if _, exists := prCache[prNum]; !exists {
						prCache[prNum] = HistoricalPR{
							Description: description,
							Category:    currentCategory,
						}
					}
				}
			}
		}
	}
}

func fetchPRs(ctx context.Context, client *github.Client, branch, fromRelease string, version *Version) ([]PRInfo, error) {
	var allPRs []PRInfo

	// Get the merge time of the from-release to use as start time
	releaseStartTime, err := getReleaseStartTime(ctx, client, fromRelease, branch)
	if err != nil {
		return nil, fmt.Errorf("failed to get release start time: %w", err)
	}

	log.Printf("Fetching PRs merged after %s", releaseStartTime.Format(time.RFC3339))

	// Fetch PRs with action/release-note label
	prsWithLabel, err := fetchPRsWithLabel(ctx, client, branch, releaseStartTime, "action/release-note")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PRs with action/release-note label: %w", err)
	}
	allPRs = append(allPRs, prsWithLabel...)

	// For patch releases, handle cherry-picks
	if version.Patch != 0 {
		cherryPickPRs, err := handleCherryPicks(ctx, client, branch, releaseStartTime)
		if err != nil {
			return nil, fmt.Errorf("failed to handle cherry-picks: %w", err)
		}
		allPRs = append(allPRs, cherryPickPRs...)
	}

	// Fetch unlabeled PRs (for --all flag)
	unlabeledPRs, err := fetchUnlabeledPRs(ctx, client, branch, releaseStartTime)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch unlabeled PRs: %w", err)
	}
	allPRs = append(allPRs, unlabeledPRs...)

	// Deduplicate PRs by number
	prMap := make(map[int]PRInfo)
	for _, pr := range allPRs {
		if _, exists := prMap[pr.Number]; !exists {
			prMap[pr.Number] = pr
		}
	}

	// Convert back to slice
	var uniquePRs []PRInfo
	for _, pr := range prMap {
		uniquePRs = append(uniquePRs, pr)
	}

	// Sort by merge time
	sort.Slice(uniquePRs, func(i, j int) bool {
		return uniquePRs[i].MergedAt.Before(uniquePRs[j].MergedAt)
	})

	return uniquePRs, nil
}

func getReleaseStartTime(ctx context.Context, client *github.Client, fromRelease, branch string) (time.Time, error) {
	// Search for the commit that was tagged with the from-release
	tag := "v" + fromRelease
	ref, _, err := client.Git.GetRef(ctx, repoOwner, repoName, "tags/"+tag)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get tag %s: %w", tag, err)
	}

	// Get the commit
	commit, _, err := client.Git.GetCommit(ctx, repoOwner, repoName, ref.Object.GetSHA())
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get commit for tag %s: %w", tag, err)
	}

	return commit.Committer.GetDate().Time, nil
}

func fetchPRsWithLabel(ctx context.Context, client *github.Client, branch string, since time.Time, label string) ([]PRInfo, error) {
	var prs []PRInfo

	opts := &github.PullRequestListOptions{
		State:     "closed",
		Base:      branch,
		Sort:      "updated",
		Direction: "desc",
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	for {
		pulls, resp, err := client.PullRequests.List(ctx, repoOwner, repoName, opts)
		if err != nil {
			return nil, err
		}

		for _, pull := range pulls {
			if pull.MergedAt == nil {
				continue
			}
			if pull.MergedAt.Before(since) {
				// We've gone past our start time
				return prs, nil
			}

			// Check if PR has the required label
			hasLabel := false
			var labels []string
			for _, l := range pull.Labels {
				labels = append(labels, l.GetName())
				if l.GetName() == label {
					hasLabel = true
				}
			}

			if !hasLabel {
				continue
			}

			prs = append(prs, PRInfo{
				Number:   pull.GetNumber(),
				Title:    pull.GetTitle(),
				Body:     pull.GetBody(),
				Author:   pull.User.GetLogin(),
				Labels:   labels,
				MergedAt: pull.MergedAt.Time,
			})
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return prs, nil
}

func handleCherryPicks(ctx context.Context, client *github.Client, branch string, since time.Time) ([]PRInfo, error) {
	var prs []PRInfo

	// Fetch PRs with kind/cherry-pick label
	opts := &github.PullRequestListOptions{
		State:     "closed",
		Base:      branch,
		Sort:      "updated",
		Direction: "desc",
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	cherryPickRegex := regexp.MustCompile(`#(\d+)`)

	for {
		pulls, resp, err := client.PullRequests.List(ctx, repoOwner, repoName, opts)
		if err != nil {
			return nil, err
		}

		for _, pull := range pulls {
			if pull.MergedAt == nil {
				continue
			}
			if pull.MergedAt.Before(since) {
				return prs, nil
			}

			// Check if PR has kind/cherry-pick label
			hasCherryPickLabel := false
			for _, l := range pull.Labels {
				if l.GetName() == "kind/cherry-pick" {
					hasCherryPickLabel = true
					break
				}
			}

			if !hasCherryPickLabel {
				continue
			}

			// Parse body for original PR numbers
			body := pull.GetBody()
			matches := cherryPickRegex.FindAllStringSubmatch(body, -1)
			for _, match := range matches {
				prNum, err := strconv.Atoi(match[1])
				if err != nil {
					continue
				}

				// Fetch the original PR
				originalPR, _, err := client.PullRequests.Get(ctx, repoOwner, repoName, prNum)
				if err != nil {
					log.Printf("Warning: failed to fetch original PR #%d: %v", prNum, err)
					continue
				}

				var labels []string
				for _, l := range originalPR.Labels {
					labels = append(labels, l.GetName())
				}

				prs = append(prs, PRInfo{
					Number:   originalPR.GetNumber(),
					Title:    originalPR.GetTitle(),
					Body:     originalPR.GetBody(),
					Author:   originalPR.User.GetLogin(),
					Labels:   labels,
					MergedAt: pull.MergedAt.Time, // Use cherry-pick merge time
				})
			}
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return prs, nil
}

func filterBotPRs(prs []PRInfo) []PRInfo {
	filtered := make([]PRInfo, 0, len(prs))
	for _, pr := range prs {
		if !ignoredAuthors[pr.Author] {
			filtered = append(filtered, pr)
		}
	}
	return filtered
}

func fetchUnlabeledPRs(ctx context.Context, client *github.Client, branch string, since time.Time) ([]PRInfo, error) {
	var prs []PRInfo

	opts := &github.PullRequestListOptions{
		State:     "closed",
		Base:      branch,
		Sort:      "updated",
		Direction: "desc",
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	for {
		pulls, resp, err := client.PullRequests.List(ctx, repoOwner, repoName, opts)
		if err != nil {
			return nil, err
		}

		for _, pull := range pulls {
			if pull.MergedAt == nil {
				continue
			}
			if pull.MergedAt.Before(since) {
				return prs, nil
			}

			// Check if PR has action/release-note or kind/cherry-pick label
			hasLabel := false
			var labels []string
			for _, l := range pull.Labels {
				labels = append(labels, l.GetName())
				if l.GetName() == "action/release-note" || l.GetName() == "kind/cherry-pick" {
					hasLabel = true
					break
				}
			}

			if hasLabel {
				continue
			}

			prs = append(prs, PRInfo{
				Number:   pull.GetNumber(),
				Title:    pull.GetTitle(),
				Body:     pull.GetBody(),
				Author:   pull.User.GetLogin(),
				Labels:   labels,
				MergedAt: pull.MergedAt.Time,
			})
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return prs, nil
}

func buildPrompt(template string, historicalCHANGELOGs string, prs []PRInfo, prCache map[int]HistoricalPR) string {
	var sb strings.Builder

	sb.WriteString(template)
	sb.WriteString("\n\n")

	// Add historical CHANGELOGs
	sb.WriteString("# HISTORICAL CHANGELOGS (for reference and consistency)\n\n")
	sb.WriteString(historicalCHANGELOGs)
	sb.WriteString("\n\n")

	// Add PR list
	sb.WriteString("# PULL REQUESTS FOR THIS RELEASE\n\n")
	for _, pr := range prs {
		sb.WriteString(fmt.Sprintf("## PR #%d\n", pr.Number))
		sb.WriteString(fmt.Sprintf("**Title:** %s\n", pr.Title))
		sb.WriteString(fmt.Sprintf("**Author:** %s\n", pr.Author))
		sb.WriteString(fmt.Sprintf("**Labels:** %s\n", strings.Join(pr.Labels, ", ")))

		// Check if this PR is in historical cache
		if historical, exists := prCache[pr.Number]; exists {
			sb.WriteString(fmt.Sprintf("**HISTORICAL ENTRY (MUST REUSE):**\n"))
			sb.WriteString(fmt.Sprintf("- Category: %s\n", historical.Category))
			sb.WriteString(fmt.Sprintf("- Description: %s\n", historical.Description))
		}

		sb.WriteString(fmt.Sprintf("**Body:**\n%s\n", pr.Body))
		sb.WriteString("\n---\n\n")
	}

	return sb.String()
}

func callGemini(ctx context.Context, apiKey, prompt, version, modelName string) (*ModelResponse, *ModelDetails, error) {
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}
	defer client.Close()

	model := client.GenerativeModel(modelName)
	model.SetTemperature(0.2)
	model.ResponseMIMEType = "application/json"

	// Measure latency
	startTime := time.Now()
	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	latency := time.Since(startTime).Seconds()

	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate content: %w", err)
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return nil, nil, fmt.Errorf("no response from model")
	}

	// Extract JSON from response
	var jsonStr string
	for _, part := range resp.Candidates[0].Content.Parts {
		if text, ok := part.(genai.Text); ok {
			jsonStr += string(text)
		}
	}

	// Parse JSON response
	var modelResponse ModelResponse
	if err := json.Unmarshal([]byte(jsonStr), &modelResponse); err != nil {
		return nil, nil, fmt.Errorf("failed to parse model response: %w\nResponse: %s", err, jsonStr)
	}

	// Extract usage metadata
	var promptTokens, candidatesTokens, totalTokens int32
	var estimatedCost float64

	if resp.UsageMetadata != nil {
		promptTokens = resp.UsageMetadata.PromptTokenCount
		candidatesTokens = resp.UsageMetadata.CandidatesTokenCount
		totalTokens = resp.UsageMetadata.TotalTokenCount

		// Gemini 2.5 Flash pricing (as of 2025):
		// Free tier: Up to 2M tokens/min, 10M tokens/day
		// Paid tier: $0.075 per 1M prompt tokens, $0.30 per 1M output tokens (128K context)
		// Using paid tier pricing for estimation
		promptCost := float64(promptTokens) / 1_000_000.0 * 0.075
		outputCost := float64(candidatesTokens) / 1_000_000.0 * 0.30
		estimatedCost = promptCost + outputCost
	}

	// Generate timestamp
	timestamp := time.Now().Format("20060102-150405")

	details := &ModelDetails{
		Version:          version,
		Timestamp:        timestamp,
		Model:            modelName,
		LatencySeconds:   latency,
		PromptTokens:     promptTokens,
		CandidatesTokens: candidatesTokens,
		TotalTokens:      totalTokens,
		EstimatedCostUSD: estimatedCost,
	}

	return &modelResponse, details, nil
}

func saveModelOutput(response *ModelResponse, filename string) error {
	data, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal model response: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

func saveModelDetails(details *ModelDetails, filename string) error {
	data, err := json.MarshalIndent(details, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal model details: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

func generateChangelog(version *Version, response *ModelResponse, includeAll bool) string {
	var sb strings.Builder

	// Title for minor releases only
	if version.Patch == 0 {
		sb.WriteString(fmt.Sprintf("# Changelog %d.%d\n\n", version.Major, version.Minor))
	}

	// Release header
	sb.WriteString(fmt.Sprintf("## %d.%d.%d - %s\n\n", version.Major, version.Minor, version.Patch, time.Now().Format("2006-01-02")))

	// Group changes by category
	categories := []string{"ADDED", "CHANGED", "FIXED"}
	changesByCategory := make(map[string][]ChangeEntry)
	var unlabeled []ChangeEntry

	for _, change := range response.Changes {
		if change.ConfidenceInclude < 50 && !includeAll {
			continue
		}

		category := strings.ToUpper(change.Category)
		if category == "ADDED" || category == "CHANGED" || category == "FIXED" {
			changesByCategory[category] = append(changesByCategory[category], change)
		} else if includeAll && change.ConfidenceInclude < 100 {
			unlabeled = append(unlabeled, change)
		}
	}

	// Collect authors
	authorSet := make(map[string]bool)

	// Output each category
	for _, category := range categories {
		sb.WriteString(fmt.Sprintf("### %s\n\n", strings.Title(strings.ToLower(category))))

		changes := changesByCategory[category]
		if len(changes) > 0 {
			for _, change := range changes {
				sb.WriteString(fmt.Sprintf("- %s. ([#%d](https://github.com/antrea-io/antrea/pull/%d), [@%s])\n",
					change.Description, change.PRNumber, change.PRNumber, change.Author))
				authorSet[change.Author] = true
			}
		}

		sb.WriteString("\n")
	}

	// Add unlabeled section if requested
	if includeAll && len(unlabeled) > 0 {
		sb.WriteString("### Unlabeled (Remove this section eventually)\n\n")
		for _, change := range unlabeled {
			sb.WriteString(fmt.Sprintf("- %s. ([#%d](https://github.com/antrea-io/antrea/pull/%d), [@%s])\n",
				change.Description, change.PRNumber, change.PRNumber, change.Author))
			authorSet[change.Author] = true
		}
		sb.WriteString("\n")
	}

	sb.WriteString("\n")

	// Add author links
	var authors []string
	for author := range authorSet {
		authors = append(authors, author)
	}
	sort.Strings(authors)

	for _, author := range authors {
		sb.WriteString(fmt.Sprintf("[@%s]: https://github.com/%s\n", author, author))
	}

	return sb.String()
}
