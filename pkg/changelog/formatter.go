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
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/antrea-io/antrea-releaser/pkg/changelog/types"
	"github.com/antrea-io/antrea-releaser/pkg/changelog/version"
)

// formatChangelog formats the AI response into a CHANGELOG
func formatChangelog(ver *version.Version, response *types.ModelResponse) string {
	var sb strings.Builder

	// Title for minor releases only
	if ver.Patch() == 0 {
		sb.WriteString(fmt.Sprintf("# Changelog %d.%d\n\n", ver.Major(), ver.Minor()))
	}

	// Release header
	sb.WriteString(fmt.Sprintf("## %d.%d.%d - %s\n\n", ver.Major(), ver.Minor(), ver.Patch(), time.Now().Format("2006-01-02")))

	// Group changes by category based on include_score
	// >= 50: include normally
	// 25-49: include with *OPTIONAL* prefix
	// < 25: exclude from CHANGELOG
	categories := []string{"ADDED", "CHANGED", "FIXED"}
	changesByCategory := make(map[string][]types.ChangeEntry)

	for _, change := range response.Changes {
		// Skip PRs with include_score < 25
		if change.IncludeScore < 25 {
			continue
		}

		category := strings.ToUpper(change.Category)
		if category == "ADDED" || category == "CHANGED" || category == "FIXED" {
			changesByCategory[category] = append(changesByCategory[category], change)
		}
	}

	// Sort changes within each category by importance_score (descending)
	for category := range changesByCategory {
		changes := changesByCategory[category]
		sort.Slice(changes, func(i, j int) bool {
			return changes[i].ImportanceScore > changes[j].ImportanceScore
		})
		changesByCategory[category] = changes
	}

	// Collect authors
	authorSet := make(map[string]bool)

	// Output each category
	for _, category := range categories {
		// Use simple capitalization for category headers (e.g., "Added", "Changed", "Fixed")
		categoryTitle := strings.ToUpper(category[:1]) + strings.ToLower(category[1:])
		sb.WriteString(fmt.Sprintf("### %s\n\n", categoryTitle))

		changes := changesByCategory[category]
		if len(changes) > 0 {
			for _, change := range changes {
				prefix := ""
				if change.IncludeScore >= 25 && change.IncludeScore < 50 {
					prefix = "*OPTIONAL* "
				}
				sb.WriteString(fmt.Sprintf("- %s%s. ([#%d](https://github.com/antrea-io/antrea/pull/%d), [@%s])\n",
					prefix, change.Description, change.PRNumber, change.PRNumber, change.Author))
				authorSet[change.Author] = true
			}
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
