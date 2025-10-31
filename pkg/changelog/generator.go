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

package changelog

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/antrea-io/antrea-releaser/pkg/changelog/prompt"
	"github.com/antrea-io/antrea-releaser/pkg/changelog/types"
	"github.com/antrea-io/antrea-releaser/pkg/changelog/version"
	gogithub "github.com/google/go-github/v67/github"
)

// ChangelogGenerator generates changelog entries using AI
type ChangelogGenerator struct {
	release      string
	fromRelease  string
	all          bool
	model        string
	modelCaller  types.ModelCaller
	githubClient types.GitHubClient
}

// NewChangelogGenerator creates a new ChangelogGenerator
func NewChangelogGenerator(
	release string,
	fromRelease string,
	all bool,
	model string,
	modelCaller types.ModelCaller,
	githubClient types.GitHubClient,
) *ChangelogGenerator {
	return &ChangelogGenerator{
		release:      release,
		fromRelease:  fromRelease,
		all:          all,
		model:        model,
		modelCaller:  modelCaller,
		githubClient: githubClient,
	}
}

// Generate generates the changelog by fetching PRs, calling the AI model, and returning the formatted changelog
func (g *ChangelogGenerator) Generate(ctx context.Context) (string, *types.Prompt, *types.ModelResponse, *types.ModelDetails, error) {
	// Parse version information
	ver, err := version.Parse(g.release)
	if err != nil {
		return "", nil, nil, nil, fmt.Errorf("invalid release version: %w", err)
	}

	// Calculate from-release if not provided
	fromRelease := g.fromRelease
	if fromRelease == "" {
		fromRelease = ver.CalculatePreviousRelease()
	}

	// Determine target branch
	branch := determineBranch(ver)

	log.Printf("Generating changelog for %s (from %s, branch: %s)", g.release, fromRelease, branch)

	// Fetch historical CHANGELOGs
	log.Println("Fetching historical CHANGELOGs...")
	historicalCHANGELOGs, prCache, err := g.fetchHistoricalCHANGELOGs(ctx)
	if err != nil {
		return "", nil, nil, nil, fmt.Errorf("failed to fetch historical CHANGELOGs: %w", err)
	}
	log.Printf("Found %d historical PR entries", len(prCache))

	// Fetch PR data
	log.Println("Fetching PR data from GitHub...")
	prs, err := g.fetchPRs(ctx, branch, fromRelease, ver)
	if err != nil {
		return "", nil, nil, nil, fmt.Errorf("failed to fetch PRs: %w", err)
	}
	log.Printf("Found %d PRs", len(prs))

	// Filter out bot-authored PRs
	prs = filterBotPRs(prs)
	log.Printf("After filtering bot PRs: %d PRs remaining", len(prs))

	// Build the prompt
	promptText := g.buildPrompt(historicalCHANGELOGs, prs, prCache)
	timestamp := time.Now().Format("20060102-150405")

	promptData := &types.Prompt{
		Text:      promptText,
		Version:   g.release,
		Timestamp: timestamp,
	}

	// Call AI model
	log.Printf("Calling AI model (model: %s)...", g.model)
	modelResponse, modelDetails, err := g.modelCaller.Call(ctx, promptText, g.release, g.model)
	if err != nil {
		return "", promptData, nil, nil, fmt.Errorf("failed to call AI model: %w", err)
	}
	log.Printf("Received %d change entries from model", len(modelResponse.Changes))
	log.Printf("Model latency: %.2f seconds, Total tokens: %d", modelDetails.LatencySeconds, modelDetails.TotalTokens)

	// Enrich with author information
	g.enrichWithAuthors(modelResponse, prs)

	// Format the changelog
	changelogText := formatChangelog(ver, modelResponse)

	return changelogText, promptData, modelResponse, modelDetails, nil
}

func (g *ChangelogGenerator) enrichWithAuthors(response *types.ModelResponse, prs []types.PRInfo) {
	for i := range response.Changes {
		for _, pr := range prs {
			if pr.Number == response.Changes[i].PRNumber {
				response.Changes[i].Author = pr.Author
				break
			}
		}
	}
}

func (g *ChangelogGenerator) fetchHistoricalCHANGELOGs(ctx context.Context) (string, map[int]types.HistoricalPR, error) {
	// List contents of CHANGELOG directory
	dirContent, err := g.githubClient.GetDirectoryContents(ctx, repoOwner, repoName, "CHANGELOG")
	if err != nil {
		return "", nil, fmt.Errorf("failed to list CHANGELOG directory: %w", err)
	}

	// Find CHANGELOG files and extract version numbers
	type changelogFile struct {
		name    string
		version *version.Version
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
		v, err := version.Parse(versionStr + ".0")
		if err != nil {
			continue
		}

		changelogFiles = append(changelogFiles, changelogFile{name: name, version: v})
	}

	// Sort by version (descending)
	sort.Slice(changelogFiles, func(i, j int) bool {
		return changelogFiles[i].version.GreaterThan(changelogFiles[j].version)
	})

	// Parse ALL CHANGELOGs for PR cache (historical consistency)
	// But only include the 3 most recent in the prompt (for styling guidance)
	prCache := make(map[int]types.HistoricalPR)

	log.Printf("Parsing %d CHANGELOG files for historical PR entries...", len(changelogFiles))
	for _, file := range changelogFiles {
		// Fetch raw content
		content, err := g.githubClient.GetFileContent(ctx, repoOwner, repoName, "CHANGELOG/"+file.name)
		if err != nil {
			log.Printf("Warning: failed to fetch %s: %v", file.name, err)
			continue
		}

		// Parse ALL files for PR cache
		g.parseCHANGELOG(content, prCache)
	}
	log.Printf("Found %d unique historical PR entries across all CHANGELOGs", len(prCache))

	// Include only the 3 most recent CHANGELOGs in the prompt (for styling)
	numToInclude := min(3, len(changelogFiles))

	var historicalContent strings.Builder
	for _, file := range changelogFiles[:numToInclude] {
		log.Printf("Including %s in prompt for styling reference...", file.name)

		// Fetch raw content again (we need the full text for the prompt)
		content, err := g.githubClient.GetFileContent(ctx, repoOwner, repoName, "CHANGELOG/"+file.name)
		if err != nil {
			return "", nil, fmt.Errorf("failed to fetch %s: %w", file.name, err)
		}

		historicalContent.WriteString(fmt.Sprintf("\n\n=== %s ===\n\n", file.name))
		historicalContent.WriteString(content)
	}

	return historicalContent.String(), prCache, nil
}

func (g *ChangelogGenerator) parseCHANGELOG(content string, prCache map[int]types.HistoricalPR) {
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
					// Skip "*OPTIONAL*" prefix if present
					description = strings.TrimPrefix(description, "*OPTIONAL* ")
					description = strings.TrimSuffix(description, ".")

					// Only store if not already present (first occurrence wins)
					if _, exists := prCache[prNum]; !exists {
						prCache[prNum] = types.HistoricalPR{
							Description: description,
							Category:    currentCategory,
						}
					}
				}
			}
		}
	}
}

func (g *ChangelogGenerator) fetchPRs(ctx context.Context, branch, fromRelease string, ver *version.Version) ([]types.PRInfo, error) {
	var allPRs []types.PRInfo

	// Get the merge time of the from-release to use as start time
	releaseStartTime, err := g.getReleaseStartTime(ctx, fromRelease)
	if err != nil {
		return nil, fmt.Errorf("failed to get release start time: %w", err)
	}

	log.Printf("Fetching PRs merged after %s", releaseStartTime.Format(time.RFC3339))

	if g.all {
		// Fetch all PRs (except those with kind/cherry-pick label which are handled separately)
		log.Println("Fetching all PRs for model analysis...")
		allMergedPRs, err := g.fetchAllPRs(ctx, branch, releaseStartTime)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch all PRs: %w", err)
		}
		allPRs = append(allPRs, allMergedPRs...)
	} else {
		// Fetch only PRs with action/release-note label
		log.Println("Fetching PRs with action/release-note label...")
		prsWithLabel, err := g.fetchPRsWithLabel(ctx, branch, releaseStartTime, "action/release-note")
		if err != nil {
			return nil, fmt.Errorf("failed to fetch PRs with action/release-note label: %w", err)
		}
		allPRs = append(allPRs, prsWithLabel...)
	}

	// For patch releases, handle cherry-picks
	if ver.Patch() != 0 {
		cherryPickPRs, err := g.handleCherryPicks(ctx, branch, releaseStartTime)
		if err != nil {
			return nil, fmt.Errorf("failed to handle cherry-picks: %w", err)
		}
		allPRs = append(allPRs, cherryPickPRs...)
	}

	// Deduplicate PRs by number
	prMap := make(map[int]types.PRInfo)
	for _, pr := range allPRs {
		if _, exists := prMap[pr.Number]; !exists {
			prMap[pr.Number] = pr
		}
	}

	// Convert back to slice
	var uniquePRs []types.PRInfo
	for _, pr := range prMap {
		uniquePRs = append(uniquePRs, pr)
	}

	// Sort by merge time
	sort.Slice(uniquePRs, func(i, j int) bool {
		return uniquePRs[i].MergedAt.Before(uniquePRs[j].MergedAt)
	})

	return uniquePRs, nil
}

func (g *ChangelogGenerator) getReleaseStartTime(ctx context.Context, fromRelease string) (time.Time, error) {
	// Search for the commit that was tagged with the from-release
	tag := "v" + fromRelease
	ref, err := g.githubClient.GetTagRef(ctx, repoOwner, repoName, tag)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get tag %s: %w", tag, err)
	}

	// Get the commit
	commit, err := g.githubClient.GetCommit(ctx, repoOwner, repoName, ref.Object.GetSHA())
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get commit for tag %s: %w", tag, err)
	}

	return commit.Committer.GetDate().Time, nil
}

func (g *ChangelogGenerator) fetchPRsWithLabel(ctx context.Context, branch string, since time.Time, label string) ([]types.PRInfo, error) {
	var prs []types.PRInfo

	opts := &gogithub.PullRequestListOptions{
		State:     "closed",
		Base:      branch,
		Sort:      "updated",
		Direction: "desc",
		ListOptions: gogithub.ListOptions{
			PerPage: 100,
		},
	}

	for {
		pulls, resp, err := g.githubClient.ListPullRequests(ctx, repoOwner, repoName, opts)
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

			prs = append(prs, types.PRInfo{
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

func (g *ChangelogGenerator) handleCherryPicks(ctx context.Context, branch string, since time.Time) ([]types.PRInfo, error) {
	var prs []types.PRInfo

	// Fetch PRs with kind/cherry-pick label
	opts := &gogithub.PullRequestListOptions{
		State:     "closed",
		Base:      branch,
		Sort:      "updated",
		Direction: "desc",
		ListOptions: gogithub.ListOptions{
			PerPage: 100,
		},
	}

	cherryPickRegex := regexp.MustCompile(`#(\d+)`)

	for {
		pulls, resp, err := g.githubClient.ListPullRequests(ctx, repoOwner, repoName, opts)
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
				originalPR, err := g.githubClient.GetPullRequest(ctx, repoOwner, repoName, prNum)
				if err != nil {
					log.Printf("Warning: failed to fetch original PR #%d: %v", prNum, err)
					continue
				}

				var labels []string
				for _, l := range originalPR.Labels {
					labels = append(labels, l.GetName())
				}

				prs = append(prs, types.PRInfo{
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

func (g *ChangelogGenerator) fetchAllPRs(ctx context.Context, branch string, since time.Time) ([]types.PRInfo, error) {
	var prs []types.PRInfo

	opts := &gogithub.PullRequestListOptions{
		State:     "closed",
		Base:      branch,
		Sort:      "updated",
		Direction: "desc",
		ListOptions: gogithub.ListOptions{
			PerPage: 100,
		},
	}

	for {
		pulls, resp, err := g.githubClient.ListPullRequests(ctx, repoOwner, repoName, opts)
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

			// Collect labels
			var labels []string
			for _, l := range pull.Labels {
				labels = append(labels, l.GetName())
			}

			// Skip cherry-pick PRs as they are handled separately
			hasCherryPickLabel := false
			for _, l := range labels {
				if l == "kind/cherry-pick" {
					hasCherryPickLabel = true
					break
				}
			}
			if hasCherryPickLabel {
				continue
			}

			prs = append(prs, types.PRInfo{
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

func (g *ChangelogGenerator) buildPrompt(historicalCHANGELOGs string, prs []types.PRInfo, prCache map[int]types.HistoricalPR) string {
	var sb strings.Builder

	sb.WriteString(prompt.Template)
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

// determineBranch determines the Git branch for a release
func determineBranch(v *version.Version) string {
	if v.Patch() == 0 {
		return "main"
	}
	return fmt.Sprintf("release-%d.%d", v.Major(), v.Minor())
}

// filterBotPRs filters out PRs authored by bots
func filterBotPRs(prs []types.PRInfo) []types.PRInfo {
	filtered := make([]types.PRInfo, 0, len(prs))
	for _, pr := range prs {
		if !ignoredAuthors[pr.Author] {
			filtered = append(filtered, pr)
		}
	}
	return filtered
}
