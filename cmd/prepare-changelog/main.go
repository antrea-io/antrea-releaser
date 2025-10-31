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
	"strings"

	"github.com/antrea-io/antrea-releaser/pkg/changelog"
	"github.com/antrea-io/antrea-releaser/pkg/changelog/genai"
	"github.com/antrea-io/antrea-releaser/pkg/changelog/github"
	"github.com/joho/godotenv"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

func run() error {
	// Load .env file if it exists (optional)
	_ = godotenv.Load()

	// Parse command-line flags
	var (
		release     = flag.String("release", "", "Release version (e.g., 2.5.0)")
		fromRelease = flag.String("from-release", "", "Previous release version (optional, auto-calculated if not provided)")
		all         = flag.Bool("all", false, "Include all PRs (not just those with action/release-note label)")
		outputFile  = flag.String("output", "", "Output file (default: stdout)")
		model       = flag.String("model", "gemini-2.5-flash", "Gemini model to use")
	)
	flag.Parse()

	// Validate required flags
	if *release == "" {
		return fmt.Errorf("--release flag is required")
	}

	// Validate model name
	if !strings.HasPrefix(*model, "gemini-") {
		return fmt.Errorf("model must start with 'gemini-', got: %s", *model)
	}

	// Get API keys from environment
	googleAPIKey := os.Getenv("GOOGLE_API_KEY")
	if googleAPIKey == "" {
		return fmt.Errorf("GOOGLE_API_KEY environment variable is required")
	}

	githubToken := os.Getenv("GITHUB_TOKEN")
	// GITHUB_TOKEN is optional (improves rate limits if provided)

	// Create dependencies
	ctx := context.Background()
	modelCaller := genai.NewGeminiCaller(googleAPIKey)
	githubClient := github.NewClient(ctx, githubToken)

	// Create changelog generator
	generator := changelog.NewChangelogGenerator(
		*release,
		*fromRelease,
		*all,
		*model,
		modelCaller,
		githubClient,
	)

	// Generate changelog
	log.Println("Starting changelog generation...")
	changelogText, promptData, modelResponse, modelDetails, err := generator.Generate(ctx)
	if err != nil {
		return fmt.Errorf("failed to generate changelog: %w", err)
	}

	// Save prompt to file
	promptFilename := fmt.Sprintf("changelog-model-prompt-%s-%s.txt", *release, promptData.Timestamp)
	if err := os.WriteFile(promptFilename, []byte(promptData.Text), 0644); err != nil {
		return fmt.Errorf("failed to write prompt file: %w", err)
	}
	log.Printf("Saved prompt to %s", promptFilename)

	// Save model response to JSON file
	outputFilename := fmt.Sprintf("changelog-model-output-%s-%s.json", *release, modelDetails.Timestamp)
	outputJSON, err := json.MarshalIndent(modelResponse, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal model response: %w", err)
	}
	if err := os.WriteFile(outputFilename, outputJSON, 0644); err != nil {
		return fmt.Errorf("failed to write model output file: %w", err)
	}
	log.Printf("Saved model output to %s", outputFilename)

	// Save model details to JSON file
	detailsFilename := fmt.Sprintf("changelog-model-details-%s-%s.json", *release, modelDetails.Timestamp)
	detailsJSON, err := json.MarshalIndent(modelDetails, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal model details: %w", err)
	}
	if err := os.WriteFile(detailsFilename, detailsJSON, 0644); err != nil {
		return fmt.Errorf("failed to write model details file: %w", err)
	}
	log.Printf("Saved model details to %s", detailsFilename)
	log.Printf("Estimated cost: $%.4f", modelDetails.EstimatedCostUSD)

	// Output changelog
	if *outputFile != "" {
		if err := os.WriteFile(*outputFile, []byte(changelogText), 0644); err != nil {
			return fmt.Errorf("failed to write output file: %w", err)
		}
		log.Printf("Changelog written to %s", *outputFile)
	} else {
		fmt.Print(changelogText)
	}

	return nil
}
