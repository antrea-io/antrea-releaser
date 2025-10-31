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
	"testing"
	"time"

	gogithub "github.com/google/go-github/v76/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/antrea-io/antrea-releaser/pkg/changelog/mocks"
	"github.com/antrea-io/antrea-releaser/pkg/changelog/types"
)

func TestGenerate_MinorRelease(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockModelCaller := mocks.NewMockModelCaller(ctrl)
	mockGitHubClient := mocks.NewMockGitHubClient(ctrl)

	// Setup expectations for GitHub API calls
	setupMinorReleaseExpectations(t, mockGitHubClient, mockModelCaller)

	generator := NewChangelogGenerator(
		"2.5.0",
		"",
		false,
		"gemini-2.5-flash",
		mockModelCaller,
		mockGitHubClient,
	)

	ctx := context.Background()
	changelogText, promptData, modelResponse, modelDetails, err := generator.Generate(ctx)

	require.NoError(t, err, "Generate() should not fail")

	// Verify prompt data
	assert.Equal(t, "2.5.0", promptData.Version, "Prompt version should match")
	assert.Contains(t, promptData.Text, "PULL REQUESTS FOR THIS RELEASE", "Prompt should contain PR section")

	// Verify model response
	assert.Len(t, modelResponse.Changes, 2, "Should have 2 changes")

	// Verify model details
	assert.Equal(t, "2.5.0", modelDetails.Version, "Model details version should match")
	assert.Equal(t, "gemini-2.5-flash", modelDetails.Model, "Model should match")

	// Verify changelog text
	assert.Contains(t, changelogText, "# Changelog 2.5", "Changelog should contain title for minor release")
	assert.Contains(t, changelogText, "## 2.5.0 -", "Changelog should contain release header")
	assert.Contains(t, changelogText, "### Added", "Changelog should contain ADDED section")
	assert.Contains(t, changelogText, "Add new feature X", "Changelog should contain first PR description")
	assert.Contains(t, changelogText, "[#1234]", "Changelog should contain PR link")
}

func TestGenerate_PatchRelease(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockModelCaller := mocks.NewMockModelCaller(ctrl)
	mockGitHubClient := mocks.NewMockGitHubClient(ctrl)

	// Setup expectations for patch release
	setupPatchReleaseExpectations(t, mockGitHubClient, mockModelCaller)

	generator := NewChangelogGenerator(
		"2.4.1",
		"",
		false,
		"gemini-2.5-flash",
		mockModelCaller,
		mockGitHubClient,
	)

	ctx := context.Background()
	changelogText, _, _, _, err := generator.Generate(ctx)

	require.NoError(t, err, "Generate() should not fail")

	// Patch release should NOT have the major title
	assert.NotContains(t, changelogText, "# Changelog 2.4", "Patch release should not have major title")
	// But should have release header
	assert.Contains(t, changelogText, "## 2.4.1 -", "Changelog should contain release header")
}

func TestGenerate_AllFlagBehavior(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockModelCaller := mocks.NewMockModelCaller(ctrl)
	mockGitHubClient := mocks.NewMockGitHubClient(ctrl)

	// When all=true, should fetch ALL PRs (not just those with action/release-note label)
	setupAllFlagExpectations(t, mockGitHubClient, mockModelCaller)

	generator := NewChangelogGenerator(
		"2.5.0",
		"",
		true, // all=true
		"gemini-2.5-flash",
		mockModelCaller,
		mockGitHubClient,
	)

	ctx := context.Background()
	_, promptData, _, _, err := generator.Generate(ctx)

	require.NoError(t, err, "Generate() should not fail")

	// Should include PR without action/release-note label
	assert.Contains(t, promptData.Text, "PR #5678", "With all=true, should include all PRs")
}

func TestGenerate_BotFiltering(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockModelCaller := mocks.NewMockModelCaller(ctrl)
	mockGitHubClient := mocks.NewMockGitHubClient(ctrl)

	setupBotFilteringExpectations(t, mockGitHubClient, mockModelCaller)

	generator := NewChangelogGenerator(
		"2.5.0",
		"",
		false,
		"gemini-2.5-flash",
		mockModelCaller,
		mockGitHubClient,
	)

	ctx := context.Background()
	_, promptData, _, _, err := generator.Generate(ctx)

	require.NoError(t, err, "Generate() should not fail")

	// Should NOT include bot-authored PRs
	assert.NotContains(t, promptData.Text, "renovate[bot]", "Should filter out renovate[bot] PRs")
	assert.NotContains(t, promptData.Text, "dependabot", "Should filter out dependabot PRs")
	assert.NotContains(t, promptData.Text, "antrea-bot", "Should filter out antrea-bot PRs")
}

func TestGenerate_OptionalPrefix(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockModelCaller := mocks.NewMockModelCaller(ctrl)
	mockGitHubClient := mocks.NewMockGitHubClient(ctrl)

	// Setup expectations with a low-confidence change
	setupOptionalPrefixExpectations(t, mockGitHubClient, mockModelCaller)

	generator := NewChangelogGenerator(
		"2.5.0",
		"",
		false,
		"gemini-2.5-flash",
		mockModelCaller,
		mockGitHubClient,
	)

	ctx := context.Background()
	changelogText, _, _, _, err := generator.Generate(ctx)

	require.NoError(t, err, "Generate() should not fail")

	// Should include *OPTIONAL* prefix for include_score 25-49
	assert.Contains(t, changelogText, "*OPTIONAL*", "Should include *OPTIONAL* prefix for low-confidence changes")
}

func TestGenerate_ExcludeLowScore(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockModelCaller := mocks.NewMockModelCaller(ctrl)
	mockGitHubClient := mocks.NewMockGitHubClient(ctrl)

	// Setup expectations with a very low score change
	setupExcludeLowScoreExpectations(t, mockGitHubClient, mockModelCaller)

	generator := NewChangelogGenerator(
		"2.5.0",
		"",
		false,
		"gemini-2.5-flash",
		mockModelCaller,
		mockGitHubClient,
	)

	ctx := context.Background()
	changelogText, _, modelResponse, _, err := generator.Generate(ctx)

	require.NoError(t, err, "Generate() should not fail")

	// Model should return the change
	assert.Len(t, modelResponse.Changes, 2, "Model should return 2 changes")

	// But changelog should NOT include PR #9999 (score < 25)
	assert.NotContains(t, changelogText, "#9999", "Should exclude changes with include_score < 25 from changelog")
}

func TestFilterBotPRs(t *testing.T) {
	prs := []types.PRInfo{
		{Number: 1, Author: "user1"},
		{Number: 2, Author: "renovate[bot]"},
		{Number: 3, Author: "dependabot"},
		{Number: 4, Author: "dependabot[bot]"},
		{Number: 5, Author: "antrea-bot"},
		{Number: 6, Author: "user2"},
	}

	filtered := filterBotPRs(prs)

	assert.Len(t, filtered, 2, "Should have 2 PRs after filtering")

	for _, pr := range filtered {
		assert.NotContains(t, []string{"renovate[bot]", "dependabot", "dependabot[bot]", "antrea-bot"},
			pr.Author, "Bot PR should be filtered out")
	}
}

// Helper functions to setup mock expectations

func setupMinorReleaseExpectations(t *testing.T, mockGitHub *mocks.MockGitHubClient, mockModel *mocks.MockModelCaller) {
	t.Helper()

	// Mock GetDirectoryContents for CHANGELOG directory
	changelog := "CHANGELOG-2.4.md"
	mockGitHub.EXPECT().
		GetDirectoryContents(gomock.Any(), "antrea-io", "antrea", "CHANGELOG").
		Return([]*gogithub.RepositoryContent{
			{Name: &changelog},
		}, nil)

	// Mock GetFileContent for historical CHANGELOG
	historicalContent := `### Added
- Add feature Y. ([#1111](https://github.com/antrea-io/antrea/pull/1111), [@oldauthor])

### Fixed
- Fix bug Z. ([#2222](https://github.com/antrea-io/antrea/pull/2222), [@oldauthor2])`

	mockGitHub.EXPECT().
		GetFileContent(gomock.Any(), "antrea-io", "antrea", "CHANGELOG/CHANGELOG-2.4.md").
		Return(historicalContent, nil).
		Times(2) // Called once for parsing PR cache, once for including in prompt

	// Mock GetTagRef for from-release
	sha := "abc123"
	mockGitHub.EXPECT().
		GetTagRef(gomock.Any(), "antrea-io", "antrea", "v2.4.0").
		Return(&gogithub.Reference{
			Object: &gogithub.GitObject{SHA: &sha},
		}, nil)

	// Mock GetCommit
	commitDate := time.Now().Add(-30 * 24 * time.Hour)
	mockGitHub.EXPECT().
		GetCommit(gomock.Any(), "antrea-io", "antrea", "abc123").
		Return(&gogithub.Commit{
			Committer: &gogithub.CommitAuthor{
				Date: &gogithub.Timestamp{Time: commitDate},
			},
		}, nil)

	// Mock ListPullRequests
	prNum1 := 1234
	prTitle1 := "Add new feature X"
	prBody1 := "This adds feature X"
	prUser1 := "author1"
	prLabel1 := "action/release-note"
	mergedAt := time.Now()

	prNum2 := 5678
	prTitle2 := "Fix bug Y"
	prBody2 := "This fixes bug Y"
	prUser2 := "author2"
	prLabel2 := "action/release-note"

	mockGitHub.EXPECT().
		ListPullRequests(gomock.Any(), "antrea-io", "antrea", gomock.Any()).
		Return([]*gogithub.PullRequest{
			{
				Number:   &prNum1,
				Title:    &prTitle1,
				Body:     &prBody1,
				User:     &gogithub.User{Login: &prUser1},
				MergedAt: &gogithub.Timestamp{Time: mergedAt},
				Labels: []*gogithub.Label{
					{Name: &prLabel1},
				},
			},
			{
				Number:   &prNum2,
				Title:    &prTitle2,
				Body:     &prBody2,
				User:     &gogithub.User{Login: &prUser2},
				MergedAt: &gogithub.Timestamp{Time: mergedAt},
				Labels: []*gogithub.Label{
					{Name: &prLabel2},
				},
			},
		}, &gogithub.Response{NextPage: 0}, nil)

	// Mock model call
	mockModel.EXPECT().
		Call(gomock.Any(), gomock.Any(), "2.5.0", "gemini-2.5-flash").
		Return(&types.ModelResponse{
			Changes: []types.ChangeEntry{
				{
					PRNumber:          1234,
					Category:          "ADDED",
					Description:       "Add new feature X",
					IncludeScore:      100,
					ImportanceScore:   90,
					ReusedFromHistory: false,
				},
				{
					PRNumber:          5678,
					Category:          "FIXED",
					Description:       "Fix bug Y",
					IncludeScore:      100,
					ImportanceScore:   85,
					ReusedFromHistory: false,
				},
			},
		}, &types.ModelDetails{
			Version:          "2.5.0",
			Timestamp:        time.Now().Format("20060102-150405"),
			Model:            "gemini-2.5-flash",
			LatencySeconds:   1.5,
			TotalTokens:      1000,
			EstimatedCostUSD: 0.001,
		}, nil)
}

func setupPatchReleaseExpectations(t *testing.T, mockGitHub *mocks.MockGitHubClient, mockModel *mocks.MockModelCaller) {
	t.Helper()

	// Mock GetDirectoryContents
	changelog := "CHANGELOG-2.4.md"
	mockGitHub.EXPECT().
		GetDirectoryContents(gomock.Any(), "antrea-io", "antrea", "CHANGELOG").
		Return([]*gogithub.RepositoryContent{
			{Name: &changelog},
		}, nil)

	// Mock GetFileContent
	mockGitHub.EXPECT().
		GetFileContent(gomock.Any(), "antrea-io", "antrea", gomock.Any()).
		Return("", nil).
		Times(2) // Called once for parsing PR cache, once for including in prompt

	// Mock GetTagRef
	sha := "def456"
	mockGitHub.EXPECT().
		GetTagRef(gomock.Any(), "antrea-io", "antrea", "v2.4.0").
		Return(&gogithub.Reference{
			Object: &gogithub.GitObject{SHA: &sha},
		}, nil)

	// Mock GetCommit
	commitDate := time.Now().Add(-10 * 24 * time.Hour)
	mockGitHub.EXPECT().
		GetCommit(gomock.Any(), "antrea-io", "antrea", "def456").
		Return(&gogithub.Commit{
			Committer: &gogithub.CommitAuthor{
				Date: &gogithub.Timestamp{Time: commitDate},
			},
		}, nil)

	// Mock ListPullRequests (twice: once for action/release-note, once for cherry-picks)
	prNum := 3333
	prTitle := "Fix critical bug"
	prBody := "This fixes a critical bug"
	prUser := "author3"
	prLabel := "action/release-note"
	mergedAt := time.Now()

	mockGitHub.EXPECT().
		ListPullRequests(gomock.Any(), "antrea-io", "antrea", gomock.Any()).
		Return([]*gogithub.PullRequest{
			{
				Number:   &prNum,
				Title:    &prTitle,
				Body:     &prBody,
				User:     &gogithub.User{Login: &prUser},
				MergedAt: &gogithub.Timestamp{Time: mergedAt},
				Labels: []*gogithub.Label{
					{Name: &prLabel},
				},
			},
		}, &gogithub.Response{NextPage: 0}, nil).Times(2)

	// Mock model call
	mockModel.EXPECT().
		Call(gomock.Any(), gomock.Any(), "2.4.1", "gemini-2.5-flash").
		Return(&types.ModelResponse{
			Changes: []types.ChangeEntry{
				{
					PRNumber:          3333,
					Category:          "FIXED",
					Description:       "Fix critical bug",
					IncludeScore:      100,
					ImportanceScore:   95,
					ReusedFromHistory: false,
				},
			},
		}, &types.ModelDetails{
			Version:          "2.4.1",
			Timestamp:        time.Now().Format("20060102-150405"),
			Model:            "gemini-2.5-flash",
			LatencySeconds:   1.2,
			TotalTokens:      800,
			EstimatedCostUSD: 0.0008,
		}, nil)
}

func setupAllFlagExpectations(t *testing.T, mockGitHub *mocks.MockGitHubClient, mockModel *mocks.MockModelCaller) {
	t.Helper()

	// Mock GetDirectoryContents
	changelog := "CHANGELOG-2.4.md"
	mockGitHub.EXPECT().
		GetDirectoryContents(gomock.Any(), "antrea-io", "antrea", "CHANGELOG").
		Return([]*gogithub.RepositoryContent{
			{Name: &changelog},
		}, nil)

	// Mock GetFileContent
	mockGitHub.EXPECT().
		GetFileContent(gomock.Any(), "antrea-io", "antrea", gomock.Any()).
		Return("", nil).
		Times(2) // Called once for parsing PR cache, once for including in prompt

	// Mock GetTagRef
	sha := "ghi789"
	mockGitHub.EXPECT().
		GetTagRef(gomock.Any(), "antrea-io", "antrea", "v2.4.0").
		Return(&gogithub.Reference{
			Object: &gogithub.GitObject{SHA: &sha},
		}, nil)

	// Mock GetCommit
	commitDate := time.Now().Add(-30 * 24 * time.Hour)
	mockGitHub.EXPECT().
		GetCommit(gomock.Any(), "antrea-io", "antrea", "ghi789").
		Return(&gogithub.Commit{
			Committer: &gogithub.CommitAuthor{
				Date: &gogithub.Timestamp{Time: commitDate},
			},
		}, nil)

	// Mock ListPullRequests - should return ALL PRs
	prNum := 5678
	prTitle := "Some random change"
	prBody := "This is a change without action/release-note label"
	prUser := "author4"
	mergedAt := time.Now()

	mockGitHub.EXPECT().
		ListPullRequests(gomock.Any(), "antrea-io", "antrea", gomock.Any()).
		Return([]*gogithub.PullRequest{
			{
				Number:   &prNum,
				Title:    &prTitle,
				Body:     &prBody,
				User:     &gogithub.User{Login: &prUser},
				MergedAt: &gogithub.Timestamp{Time: mergedAt},
				Labels:   []*gogithub.Label{},
			},
		}, &gogithub.Response{NextPage: 0}, nil)

	// Mock model call
	mockModel.EXPECT().
		Call(gomock.Any(), gomock.Any(), "2.5.0", "gemini-2.5-flash").
		Return(&types.ModelResponse{
			Changes: []types.ChangeEntry{
				{
					PRNumber:          5678,
					Category:          "CHANGED",
					Description:       "Some random change",
					IncludeScore:      60,
					ImportanceScore:   50,
					ReusedFromHistory: false,
				},
			},
		}, &types.ModelDetails{
			Version:          "2.5.0",
			Timestamp:        time.Now().Format("20060102-150405"),
			Model:            "gemini-2.5-flash",
			LatencySeconds:   1.0,
			TotalTokens:      500,
			EstimatedCostUSD: 0.0005,
		}, nil)
}

func setupBotFilteringExpectations(t *testing.T, mockGitHub *mocks.MockGitHubClient, mockModel *mocks.MockModelCaller) {
	t.Helper()

	// Mock GetDirectoryContents
	changelog := "CHANGELOG-2.4.md"
	mockGitHub.EXPECT().
		GetDirectoryContents(gomock.Any(), "antrea-io", "antrea", "CHANGELOG").
		Return([]*gogithub.RepositoryContent{
			{Name: &changelog},
		}, nil)

	// Mock GetFileContent
	mockGitHub.EXPECT().
		GetFileContent(gomock.Any(), "antrea-io", "antrea", gomock.Any()).
		Return("", nil).
		Times(2) // Called once for parsing PR cache, once for including in prompt

	// Mock GetTagRef
	sha := "jkl012"
	mockGitHub.EXPECT().
		GetTagRef(gomock.Any(), "antrea-io", "antrea", "v2.4.0").
		Return(&gogithub.Reference{
			Object: &gogithub.GitObject{SHA: &sha},
		}, nil)

	// Mock GetCommit
	commitDate := time.Now().Add(-30 * 24 * time.Hour)
	mockGitHub.EXPECT().
		GetCommit(gomock.Any(), "antrea-io", "antrea", "jkl012").
		Return(&gogithub.Commit{
			Committer: &gogithub.CommitAuthor{
				Date: &gogithub.Timestamp{Time: commitDate},
			},
		}, nil)

	// Mock ListPullRequests with bot PRs that should be filtered
	prNum1 := 1111
	prTitle1 := "User PR"
	prBody1 := "Real user PR"
	prUser1 := "realuser"
	prLabel1 := "action/release-note"
	mergedAt := time.Now()

	prNum2 := 2222
	prTitle2 := "Bot PR"
	prBody2 := "Renovate update"
	prUser2 := "renovate[bot]"

	mockGitHub.EXPECT().
		ListPullRequests(gomock.Any(), "antrea-io", "antrea", gomock.Any()).
		Return([]*gogithub.PullRequest{
			{
				Number:   &prNum1,
				Title:    &prTitle1,
				Body:     &prBody1,
				User:     &gogithub.User{Login: &prUser1},
				MergedAt: &gogithub.Timestamp{Time: mergedAt},
				Labels: []*gogithub.Label{
					{Name: &prLabel1},
				},
			},
			{
				Number:   &prNum2,
				Title:    &prTitle2,
				Body:     &prBody2,
				User:     &gogithub.User{Login: &prUser2},
				MergedAt: &gogithub.Timestamp{Time: mergedAt},
				Labels: []*gogithub.Label{
					{Name: &prLabel1},
				},
			},
		}, &gogithub.Response{NextPage: 0}, nil)

	// Mock model call - should only receive non-bot PRs
	mockModel.EXPECT().
		Call(gomock.Any(), gomock.Any(), "2.5.0", "gemini-2.5-flash").
		Return(&types.ModelResponse{
			Changes: []types.ChangeEntry{
				{
					PRNumber:          1111,
					Category:          "ADDED",
					Description:       "User change",
					IncludeScore:      100,
					ImportanceScore:   80,
					ReusedFromHistory: false,
				},
			},
		}, &types.ModelDetails{
			Version:          "2.5.0",
			Timestamp:        time.Now().Format("20060102-150405"),
			Model:            "gemini-2.5-flash",
			LatencySeconds:   1.0,
			TotalTokens:      500,
			EstimatedCostUSD: 0.0005,
		}, nil)
}

func setupOptionalPrefixExpectations(t *testing.T, mockGitHub *mocks.MockGitHubClient, mockModel *mocks.MockModelCaller) {
	t.Helper()

	// Mock GetDirectoryContents
	changelog := "CHANGELOG-2.4.md"
	mockGitHub.EXPECT().
		GetDirectoryContents(gomock.Any(), "antrea-io", "antrea", "CHANGELOG").
		Return([]*gogithub.RepositoryContent{
			{Name: &changelog},
		}, nil)

	// Mock GetFileContent
	mockGitHub.EXPECT().
		GetFileContent(gomock.Any(), "antrea-io", "antrea", gomock.Any()).
		Return("", nil).
		Times(2) // Called once for parsing PR cache, once for including in prompt

	// Mock GetTagRef
	sha := "mno345"
	mockGitHub.EXPECT().
		GetTagRef(gomock.Any(), "antrea-io", "antrea", "v2.4.0").
		Return(&gogithub.Reference{
			Object: &gogithub.GitObject{SHA: &sha},
		}, nil)

	// Mock GetCommit
	commitDate := time.Now().Add(-30 * 24 * time.Hour)
	mockGitHub.EXPECT().
		GetCommit(gomock.Any(), "antrea-io", "antrea", "mno345").
		Return(&gogithub.Commit{
			Committer: &gogithub.CommitAuthor{
				Date: &gogithub.Timestamp{Time: commitDate},
			},
		}, nil)

	// Mock ListPullRequests
	prNum := 7777
	prTitle := "Minor change"
	prBody := "This is a minor change"
	prUser := "author5"
	prLabel := "action/release-note"
	mergedAt := time.Now()

	mockGitHub.EXPECT().
		ListPullRequests(gomock.Any(), "antrea-io", "antrea", gomock.Any()).
		Return([]*gogithub.PullRequest{
			{
				Number:   &prNum,
				Title:    &prTitle,
				Body:     &prBody,
				User:     &gogithub.User{Login: &prUser},
				MergedAt: &gogithub.Timestamp{Time: mergedAt},
				Labels: []*gogithub.Label{
					{Name: &prLabel},
				},
			},
		}, &gogithub.Response{NextPage: 0}, nil)

	// Mock model call with low confidence (25-49)
	mockModel.EXPECT().
		Call(gomock.Any(), gomock.Any(), "2.5.0", "gemini-2.5-flash").
		Return(&types.ModelResponse{
			Changes: []types.ChangeEntry{
				{
					PRNumber:          7777,
					Category:          "CHANGED",
					Description:       "Minor change",
					IncludeScore:      35, // Low confidence
					ImportanceScore:   30,
					ReusedFromHistory: false,
				},
			},
		}, &types.ModelDetails{
			Version:          "2.5.0",
			Timestamp:        time.Now().Format("20060102-150405"),
			Model:            "gemini-2.5-flash",
			LatencySeconds:   1.0,
			TotalTokens:      500,
			EstimatedCostUSD: 0.0005,
		}, nil)
}

func setupExcludeLowScoreExpectations(t *testing.T, mockGitHub *mocks.MockGitHubClient, mockModel *mocks.MockModelCaller) {
	t.Helper()

	// Mock GetDirectoryContents
	changelog := "CHANGELOG-2.4.md"
	mockGitHub.EXPECT().
		GetDirectoryContents(gomock.Any(), "antrea-io", "antrea", "CHANGELOG").
		Return([]*gogithub.RepositoryContent{
			{Name: &changelog},
		}, nil)

	// Mock GetFileContent
	mockGitHub.EXPECT().
		GetFileContent(gomock.Any(), "antrea-io", "antrea", gomock.Any()).
		Return("", nil).
		Times(2) // Called once for parsing PR cache, once for including in prompt

	// Mock GetTagRef
	sha := "pqr678"
	mockGitHub.EXPECT().
		GetTagRef(gomock.Any(), "antrea-io", "antrea", "v2.4.0").
		Return(&gogithub.Reference{
			Object: &gogithub.GitObject{SHA: &sha},
		}, nil)

	// Mock GetCommit
	commitDate := time.Now().Add(-30 * 24 * time.Hour)
	mockGitHub.EXPECT().
		GetCommit(gomock.Any(), "antrea-io", "antrea", "pqr678").
		Return(&gogithub.Commit{
			Committer: &gogithub.CommitAuthor{
				Date: &gogithub.Timestamp{Time: commitDate},
			},
		}, nil)

	// Mock ListPullRequests
	prNum1 := 8888
	prTitle1 := "Good change"
	prBody1 := "This is a good change"
	prUser1 := "author6"
	prLabel1 := "action/release-note"
	mergedAt := time.Now()

	prNum2 := 9999
	prTitle2 := "Trivial change"
	prBody2 := "This is trivial"
	prUser2 := "author7"

	mockGitHub.EXPECT().
		ListPullRequests(gomock.Any(), "antrea-io", "antrea", gomock.Any()).
		Return([]*gogithub.PullRequest{
			{
				Number:   &prNum1,
				Title:    &prTitle1,
				Body:     &prBody1,
				User:     &gogithub.User{Login: &prUser1},
				MergedAt: &gogithub.Timestamp{Time: mergedAt},
				Labels: []*gogithub.Label{
					{Name: &prLabel1},
				},
			},
			{
				Number:   &prNum2,
				Title:    &prTitle2,
				Body:     &prBody2,
				User:     &gogithub.User{Login: &prUser2},
				MergedAt: &gogithub.Timestamp{Time: mergedAt},
				Labels: []*gogithub.Label{
					{Name: &prLabel1},
				},
			},
		}, &gogithub.Response{NextPage: 0}, nil)

	// Mock model call with one very low score
	mockModel.EXPECT().
		Call(gomock.Any(), gomock.Any(), "2.5.0", "gemini-2.5-flash").
		Return(&types.ModelResponse{
			Changes: []types.ChangeEntry{
				{
					PRNumber:          8888,
					Category:          "CHANGED",
					Description:       "Good change",
					IncludeScore:      80,
					ImportanceScore:   70,
					ReusedFromHistory: false,
				},
				{
					PRNumber:          9999,
					Category:          "CHANGED",
					Description:       "Trivial change",
					IncludeScore:      10, // Very low - should be excluded
					ImportanceScore:   5,
					ReusedFromHistory: false,
				},
			},
		}, &types.ModelDetails{
			Version:          "2.5.0",
			Timestamp:        time.Now().Format("20060102-150405"),
			Model:            "gemini-2.5-flash",
			LatencySeconds:   1.0,
			TotalTokens:      500,
			EstimatedCostUSD: 0.0005,
		}, nil)
}
