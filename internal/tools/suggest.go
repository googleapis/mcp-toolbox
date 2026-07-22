// Copyright 2026 Google LLC
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

package tools

import (
	"fmt"
	"sort"
	"strings"
)

// maxListedNames caps how many available tool names are embedded in an
// unknown-tool error so the payload stays bounded for large toolsets.
const maxListedNames = 25

// UnknownToolError returns the error for a tool name that could not be
// resolved. Beyond the base message, it lists the available tool names (capped
// at maxListedNames) and, when one is close enough, a nearest-match
// suggestion. MCP errors are consumed by LLM agents as prompt text; including
// the valid names lets an agent self-correct instead of retrying a stale or
// misspelled name.
func UnknownToolError(toolName string, available []string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "invalid tool name: tool with name %q does not exist", toolName)

	if len(available) == 0 {
		return fmt.Errorf("%s", b.String())
	}

	sorted := make([]string, len(available))
	copy(sorted, available)
	sort.Strings(sorted)

	b.WriteString(".")
	if suggestion, ok := nearestName(toolName, sorted); ok {
		fmt.Fprintf(&b, " Did you mean %q?", suggestion)
	}

	listed := sorted
	var remaining int
	if len(sorted) > maxListedNames {
		listed = sorted[:maxListedNames]
		remaining = len(sorted) - maxListedNames
	}
	fmt.Fprintf(&b, " Available tools: %s", strings.Join(listed, ", "))
	if remaining > 0 {
		fmt.Fprintf(&b, " (and %d more)", remaining)
	}
	return fmt.Errorf("%s", b.String())
}

// nearestName returns the candidate most similar to name, if any candidate is
// similar enough to be a plausible rename or typo. Similarity is
// case-insensitive Levenshtein distance; a candidate qualifies when the
// distance is at most half the longer name's length. Candidates must be
// sorted so ties resolve deterministically.
func nearestName(name string, candidates []string) (string, bool) {
	lowered := strings.ToLower(name)
	best, bestDist := "", -1
	for _, c := range candidates {
		d := levenshtein(lowered, strings.ToLower(c))
		if bestDist == -1 || d < bestDist {
			best, bestDist = c, d
		}
	}
	if bestDist == -1 {
		return "", false
	}
	longer := len([]rune(name))
	if l := len([]rune(best)); l > longer {
		longer = l
	}
	if bestDist*2 > longer {
		return "", false
	}
	return best, true
}

// levenshtein computes the edit distance between two strings by rune.
func levenshtein(a, b string) int {
	ar, br := []rune(a), []rune(b)
	if len(ar) == 0 {
		return len(br)
	}
	prev := make([]int, len(br)+1)
	curr := make([]int, len(br)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		curr[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			curr[j] = min(prev[j]+1, min(curr[j-1]+1, prev[j-1]+cost))
		}
		prev, curr = curr, prev
	}
	return prev[len(br)]
}
