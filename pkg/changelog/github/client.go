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

package github

import (
	"context"
	"fmt"

	gogithub "github.com/google/go-github/v76/github"
	"golang.org/x/oauth2"
)

// RealClient wraps the go-github client and implements the GitHubClient interface
type RealClient struct {
	client *gogithub.Client
}

// NewClient creates a new GitHub client
func NewClient(ctx context.Context, token string) *RealClient {
	var client *gogithub.Client
	if token != "" {
		ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
		tc := oauth2.NewClient(ctx, ts)
		client = gogithub.NewClient(tc)
	} else {
		client = gogithub.NewClient(nil)
	}

	return &RealClient{client: client}
}

// GetDirectoryContents lists contents of a directory in a repository
func (c *RealClient) GetDirectoryContents(ctx context.Context, owner, repo, path string) ([]*gogithub.RepositoryContent, error) {
	_, dirContent, _, err := c.client.Repositories.GetContents(ctx, owner, repo, path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get directory contents: %w", err)
	}
	return dirContent, nil
}

// GetFileContent gets the content of a file from a repository
func (c *RealClient) GetFileContent(ctx context.Context, owner, repo, path string) (string, error) {
	fileContent, _, _, err := c.client.Repositories.GetContents(ctx, owner, repo, path, nil)
	if err != nil {
		return "", fmt.Errorf("failed to get file content: %w", err)
	}

	content, err := fileContent.GetContent()
	if err != nil {
		return "", fmt.Errorf("failed to decode file content: %w", err)
	}

	return content, nil
}

// GetTagRef gets a Git reference for a tag
func (c *RealClient) GetTagRef(ctx context.Context, owner, repo, tag string) (*gogithub.Reference, error) {
	ref, _, err := c.client.Git.GetRef(ctx, owner, repo, "tags/"+tag)
	if err != nil {
		return nil, fmt.Errorf("failed to get tag ref: %w", err)
	}
	return ref, nil
}

// GetCommit gets a Git commit
func (c *RealClient) GetCommit(ctx context.Context, owner, repo, sha string) (*gogithub.Commit, error) {
	commit, _, err := c.client.Git.GetCommit(ctx, owner, repo, sha)
	if err != nil {
		return nil, fmt.Errorf("failed to get commit: %w", err)
	}
	return commit, nil
}

// ListPullRequests lists pull requests with pagination
func (c *RealClient) ListPullRequests(ctx context.Context, owner, repo string, opts *gogithub.PullRequestListOptions) ([]*gogithub.PullRequest, *gogithub.Response, error) {
	pulls, resp, err := c.client.PullRequests.List(ctx, owner, repo, opts)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list pull requests: %w", err)
	}
	return pulls, resp, nil
}

// GetPullRequest gets a single pull request
func (c *RealClient) GetPullRequest(ctx context.Context, owner, repo string, number int) (*gogithub.PullRequest, error) {
	pr, _, err := c.client.PullRequests.Get(ctx, owner, repo, number)
	if err != nil {
		return nil, fmt.Errorf("failed to get pull request: %w", err)
	}
	return pr, nil
}
