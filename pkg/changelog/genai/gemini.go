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

package genai

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"google.golang.org/genai"

	"github.com/antrea-io/antrea-releaser/pkg/changelog/types"
)

// GeminiCaller implements ModelCaller for Google's Gemini API
type GeminiCaller struct {
	apiKey string
}

// NewGeminiCaller creates a new GeminiCaller with the provided API key
func NewGeminiCaller(apiKey string) *GeminiCaller {
	return &GeminiCaller{
		apiKey: apiKey,
	}
}

// Call sends a prompt to Gemini and returns the structured response and metadata
func (g *GeminiCaller) Call(ctx context.Context, prompt, version, modelName string) (*types.ModelResponse, *types.ModelDetails, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  g.apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}

	// Prepare the generation config
	genConfig := &genai.GenerateContentConfig{
		Temperature:      genai.Ptr(float32(0.2)),
		ResponseMIMEType: "application/json",
	}

	// Prepare the content parts
	parts := []*genai.Part{
		{Text: prompt},
	}
	content := []*genai.Content{{Parts: parts}}

	// Measure latency
	startTime := time.Now()
	resp, err := client.Models.GenerateContent(ctx, modelName, content, genConfig)
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
		if part.Text != "" {
			jsonStr += part.Text
		}
	}

	// Parse JSON response
	var modelResponse types.ModelResponse
	if err := json.Unmarshal([]byte(jsonStr), &modelResponse); err != nil {
		return nil, nil, fmt.Errorf("failed to parse model response: %w\nResponse: %s", err, jsonStr)
	}

	// Extract usage metadata
	var promptTokens, candidatesTokens, totalTokens int32
	var estimatedCost float64

	if resp.UsageMetadata != nil {
		promptTokens = int32(resp.UsageMetadata.PromptTokenCount)
		candidatesTokens = int32(resp.UsageMetadata.CandidatesTokenCount)
		totalTokens = int32(resp.UsageMetadata.TotalTokenCount)

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

	details := &types.ModelDetails{
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
