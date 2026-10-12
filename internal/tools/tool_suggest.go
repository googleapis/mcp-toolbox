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
	// SuggestionsOff discloses nothing about the tools that exist, and is the
	// default. The error still points the caller at the list call, so an agent
	// retains a recovery path without the server naming anything: the list is
	// permission- and scope-filtered where the error string is not.
	SuggestionsOff SuggestionMode = "off"
)

// Recovery hints name the call that re-reads the tool list. They differ by
// surface, and the list they point at is the authoritative, filtered one.
const (
	hintMCP  = "tools/list"
	hintHTTP = "GET /api/toolset"
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
	return string(SuggestionsOff)
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

// resolve normalizes an unset or unrecognized mode to the default, which is
// the least disclosing one.
func (m SuggestionMode) resolve() SuggestionMode {
	normalized := SuggestionMode(strings.ToLower(string(m)))
	if validModes[normalized] {
		return normalized
	}
	return SuggestionsOff
}

// UnknownToolError returns the error for a tool name an MCP request could not
// resolve. Under the default mode it discloses nothing about the tools that
// exist and points the caller at tools/list; raising the mode appends a
// nearest-match suggestion and, at "full", the available names (capped at
// maxListedNames). MCP errors are consumed by LLM agents as prompt text, so
// either shape leaves the agent a way to self-correct rather than retrying a
// stale or misspelled name.
//
// available must already be scoped to what the caller is allowed to disclose
// on this endpoint; this function does no filtering of its own.
func UnknownToolError(toolName string, available []string, mode SuggestionMode) error {
	return unknownToolError(toolName, available, mode, hintMCP)
}

// UnknownToolErrorHTTP returns the error for the /api routes. They carry no
// group scope, so there is no set of names known to be disclosable there and
// the mode is not configurable: the error names nothing and points at the
// toolset endpoint.
func UnknownToolErrorHTTP(toolName string) error {
	return unknownToolError(toolName, nil, SuggestionsOff, hintHTTP)
}

func unknownToolError(toolName string, available []string, mode SuggestionMode, hint string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "invalid tool name: tool with name %q does not exist", toolName)

	// Whenever the error says nothing about what does exist, it says how to
	// find out. The prefix above is unchanged from the pre-existing message, so
	// anything matching on it keeps working.
	withHint := func() error {
		fmt.Fprintf(&b, ". Call %s to see the available tools.", hint)
		return fmt.Errorf("%s", b.String())
	}

	mode = mode.resolve()
	if len(available) == 0 || mode == SuggestionsOff {
		return withHint()
	}

	sorted := make([]string, len(available))
	copy(sorted, available)
	sort.Strings(sorted)

	suggestion, hasSuggestion := nearestName(toolName, sorted)
	if mode == SuggestionsNearest && !hasSuggestion {
		return withHint()
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
