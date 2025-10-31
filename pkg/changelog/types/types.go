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

package types

import (
	"context"
	"time"

	"github.com/google/go-github/v76/github"
)

// PRInfo contains information about a pull request
type PRInfo struct {
	Number   int
	Title    string
	Body     string
	Author   string
	Labels   []string
	MergedAt time.Time
}

// ChangeEntry represents a single changelog entry from the model
type ChangeEntry struct {
	PRNumber          int    `json:"pr_number"`
	Category          string `json:"category"`
	Description       string `json:"description"`
	IncludeScore      int    `json:"include_score"`
	ImportanceScore   int    `json:"importance_score"`
	ReusedFromHistory bool   `json:"reused_from_history"`
	Author            string `json:"-"`
}

// ModelResponse is the structured response from the AI model
type ModelResponse struct {
	Changes []ChangeEntry `json:"changes"`
}

// ModelDetails contains metadata about the model invocation
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

// Prompt contains the full prompt sent to the model
type Prompt struct {
	Text      string
	Version   string
	Timestamp string
}

// HistoricalPR represents a PR entry from historical CHANGELOGs
type HistoricalPR struct {
	Description string
	Category    string
}

// ModelCaller is an interface for calling AI models to generate changelog entries
type ModelCaller interface {
	// Call sends a prompt to the model and returns the structured response and metadata
	Call(ctx context.Context, prompt, version, modelName string) (*ModelResponse, *ModelDetails, error)
}

// GitHubClient is an interface for GitHub API operations needed for changelog generation
type GitHubClient interface {
	// GetDirectoryContents lists contents of a directory in a repository
	GetDirectoryContents(ctx context.Context, owner, repo, path string) ([]*github.RepositoryContent, error)

	// GetFileContent gets the content of a file from a repository
	GetFileContent(ctx context.Context, owner, repo, path string) (string, error)

	// GetTagRef gets a Git reference for a tag
	GetTagRef(ctx context.Context, owner, repo, tag string) (*github.Reference, error)

	// GetCommit gets a Git commit
	GetCommit(ctx context.Context, owner, repo, sha string) (*github.Commit, error)

	// ListPullRequests lists pull requests with pagination
	ListPullRequests(ctx context.Context, owner, repo string, opts *github.PullRequestListOptions) ([]*github.PullRequest, *github.Response, error)

	// GetPullRequest gets a single pull request
	GetPullRequest(ctx context.Context, owner, repo string, number int) (*github.PullRequest, error)
}
