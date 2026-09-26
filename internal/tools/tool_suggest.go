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

// maxListedNames bounds the payload of an unknown-tool error for large groups.
const maxListedNames = 25

// SuggestionMode controls how much an unknown-tool error discloses about the
// tools that do exist. Errors reach sinks a tools/list response does not —
// telemetry spans, client logs, and gateways that filter tools/list but pass
// tools/call errors through — so deployments can narrow this independently.
type SuggestionMode string

const (
	// SuggestionsFull lists the available tool names and the nearest match.
	SuggestionsFull SuggestionMode = "full"
	// SuggestionsNearest includes only the nearest-match suggestion.
	SuggestionsNearest SuggestionMode = "nearest"
	// SuggestionsOff returns the bare "does not exist" message.
	SuggestionsOff SuggestionMode = "off"
)

// validModes is the set of recognized modes.
var validModes = map[SuggestionMode]bool{
	SuggestionsOff:     true,
	SuggestionsNearest: true,
	SuggestionsFull:    true,
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
	normalized := SuggestionMode(strings.ToLower(v))
	if !validModes[normalized] {
		return fmt.Errorf(`tool suggestions must be one of "full", "nearest", or "off"`)
	}
	*m = normalized
	return nil
}

// Type is used in Cobra help text.
func (m *SuggestionMode) Type() string {
	return "suggestionMode"
}

// resolve normalizes an unset or unrecognized mode to the default.
func (m SuggestionMode) resolve() SuggestionMode {
	normalized := SuggestionMode(strings.ToLower(string(m)))
	if validModes[normalized] {
		return normalized
	}
	return SuggestionsFull
}

// UnknownToolError returns the error for a tool name that could not be
// resolved. Depending on mode it appends a nearest-match suggestion and the
// available names (capped at maxListedNames). MCP errors are consumed by LLM
// agents as prompt text, so naming the valid alternatives lets an agent
// self-correct instead of retrying a stale or misspelled name.
//
// available must already be scoped to what the caller is allowed to disclose
// on this endpoint; this function does no filtering of its own.
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

// nearestName returns the candidate most similar to name, if any is close
// enough to be a plausible rename or typo: case-insensitive Levenshtein
// distance at most half the longer name's length. Candidates must be sorted so
// ties resolve deterministically.
//
// Only the closest candidate matters, so the scan never computes a full
// distance it cannot use. A candidate whose length alone puts it at or beyond
// the incumbent is skipped outright (length difference is a lower bound on
// distance), the matrix walk aborts as soon as every cell in a row reaches the
// incumbent, and an exact match ends the scan.
func nearestName(name string, candidates []string) (string, bool) {
	lowered := strings.ToLower(name)
	nameLen := len([]rune(lowered))

	best, bestDist := "", -1
	for _, c := range candidates {
		lc := strings.ToLower(c)
		if bestDist >= 0 {
			lenDiff := nameLen - len([]rune(lc))
			if lenDiff < 0 {
				lenDiff = -lenDiff
			}
			if lenDiff >= bestDist {
				continue
			}
		}
		d := levenshtein(lowered, lc, bestDist)
		if bestDist == -1 || d < bestDist {
			best, bestDist = c, d
			if bestDist == 0 {
				break
			}
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

// levenshtein computes the edit distance between two strings by rune. A cutoff
// of zero or more abandons the walk once no result below cutoff is reachable,
// returning cutoff; callers comparing against an incumbent distance only need
// to know the result is no better. A negative cutoff computes the exact
// distance.
func levenshtein(a, b string, cutoff int) int {
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
		rowMin := curr[0]
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			curr[j] = min(prev[j]+1, min(curr[j-1]+1, prev[j-1]+cost))
			if curr[j] < rowMin {
				rowMin = curr[j]
			}
		}
		prev, curr = curr, prev
		// Distances never decrease as rows advance, so once the whole row is
		// at the cutoff nothing below it remains reachable.
		if cutoff >= 0 && rowMin >= cutoff {
			return cutoff
		}
	}
	return prev[len(br)]
}
