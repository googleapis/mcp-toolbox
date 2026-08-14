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
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/googleapis/mcp-toolbox/internal/util"
)

// maxListedNames caps how many available tool names are embedded in an
// unknown-tool error so the payload stays bounded for large toolsets.
const maxListedNames = 25

// SuggestionMode controls how much an unknown-tool error discloses about the
// tools that do exist. Tool listing is not authorization-filtered, so on a
// given endpoint the names in these errors are a subset of what a `tools/list`
// on that same endpoint already returns. The mode exists for deployments that
// still want to keep the inventory out of error strings, which reach sinks a
// list response does not: telemetry spans, client logs, and any gateway that
// filters `tools/list` but passes `tools/call` errors through.
type SuggestionMode string

const (
	// SuggestionsFull lists the available tool names and the nearest match.
	SuggestionsFull SuggestionMode = "full"
	// SuggestionsNearest includes only the nearest-match suggestion, so the
	// agent can still self-correct without the error carrying an inventory.
	SuggestionsNearest SuggestionMode = "nearest"
	// SuggestionsOff returns the bare "does not exist" message.
	SuggestionsOff SuggestionMode = "off"
)

// suggestionRank orders the modes from least to most disclosure so AtMost can
// compare them.
var suggestionRank = map[SuggestionMode]int{
	SuggestionsOff:     0,
	SuggestionsNearest: 1,
	SuggestionsFull:    2,
}

// String is used by both fmt.Print and by Cobra in help text.
func (m *SuggestionMode) String() string {
	if string(*m) != "" {
		return strings.ToLower(string(*m))
	}
	return string(SuggestionsFull)
}

// Set validates the tool-suggestions flag.
func (m *SuggestionMode) Set(v string) error {
	switch SuggestionMode(strings.ToLower(v)) {
	case SuggestionsFull, SuggestionsNearest, SuggestionsOff:
		*m = SuggestionMode(strings.ToLower(v))
		return nil
	default:
		return fmt.Errorf(`tool suggestions must be one of "full", "nearest", or "off"`)
	}
}

// Type is used in Cobra help text.
func (m *SuggestionMode) Type() string {
	return "suggestionMode"
}

// resolve normalizes an unset or unrecognized mode to the default.
func (m SuggestionMode) resolve() SuggestionMode {
	normalized := SuggestionMode(strings.ToLower(string(m)))
	if _, ok := suggestionRank[normalized]; ok {
		return normalized
	}
	return SuggestionsFull
}

// AtMost returns the less disclosing of the two modes. Callers that lack a
// group scope use it to bound what an error can reveal regardless of the
// server-wide setting.
func (m SuggestionMode) AtMost(ceiling SuggestionMode) SuggestionMode {
	if suggestionRank[ceiling.resolve()] < suggestionRank[m.resolve()] {
		return ceiling.resolve()
	}
	return m.resolve()
}

// SuggestionModeFromContext retrieves the server's configured suggestion mode,
// defaulting to SuggestionsFull when unset.
func SuggestionModeFromContext(ctx context.Context) SuggestionMode {
	mode, ok := util.ToolSuggestionsFromContext(ctx)
	if !ok {
		return SuggestionsFull
	}
	return SuggestionMode(mode).resolve()
}

// UnknownToolError returns the error for a tool name that could not be
// resolved. Beyond the base message, and depending on mode, it lists the
// available tool names (capped at maxListedNames) and, when one is close
// enough, a nearest-match suggestion. MCP errors are consumed by LLM agents as
// prompt text; including the valid names lets an agent self-correct instead of
// retrying a stale or misspelled name.
func UnknownToolError(toolName string, available []string, mode SuggestionMode) error {
	var b strings.Builder
	fmt.Fprintf(&b, "invalid tool name: tool with name %q does not exist", toolName)

	mode = mode.resolve()
	if len(available) == 0 || mode == SuggestionsOff {
		return fmt.Errorf("%s", b.String())
	}

	sorted := make([]string, len(available))
	copy(sorted, available)
	sort.Strings(sorted)

	suggestion, hasSuggestion := nearestName(toolName, sorted)
	if mode == SuggestionsNearest && !hasSuggestion {
		return fmt.Errorf("%s", b.String())
	}

	b.WriteString(".")
	if hasSuggestion {
		fmt.Fprintf(&b, " Did you mean %q?", suggestion)
	}
	if mode == SuggestionsNearest {
		return fmt.Errorf("%s", b.String())
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
