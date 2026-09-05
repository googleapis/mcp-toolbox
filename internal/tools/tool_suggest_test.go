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
	"testing"
)

func TestUnknownToolError(t *testing.T) {
	tcs := []struct {
		desc      string
		toolName  string
		available []string
		mode      SuggestionMode
		want      string
	}{
		{
			desc:      "no available tools keeps the base message",
			toolName:  "foo",
			available: nil,
			want:      `invalid tool name: tool with name "foo" does not exist`,
		},
		{
			desc:      "unset mode defaults to full",
			toolName:  "lookup_sensor",
			available: []string{"list_sensors", "latest_observation"},
			mode:      "",
			want:      `invalid tool name: tool with name "lookup_sensor" does not exist. Did you mean "list_sensors"? Available tools: latest_observation, list_sensors`,
		},
		{
			desc:      "off returns the bare message",
			toolName:  "lookup_sensor",
			available: []string{"list_sensors", "latest_observation"},
			mode:      SuggestionsOff,
			want:      `invalid tool name: tool with name "lookup_sensor" does not exist`,
		},
		{
			desc:      "nearest suggests without listing the inventory",
			toolName:  "lookup_sensor",
			available: []string{"list_sensors", "latest_observation"},
			mode:      SuggestionsNearest,
			want:      `invalid tool name: tool with name "lookup_sensor" does not exist. Did you mean "list_sensors"?`,
		},
		{
			desc:      "nearest with no plausible match discloses nothing",
			toolName:  "zzzzzzzzzzzz",
			available: []string{"search_hotels", "book_hotel"},
			mode:      SuggestionsNearest,
			want:      `invalid tool name: tool with name "zzzzzzzzzzzz" does not exist`,
		},
		{
			desc:      "close match yields a suggestion and the list",
			toolName:  "lookup_sensor",
			available: []string{"list_sensors", "latest_observation"},
			want:      `invalid tool name: tool with name "lookup_sensor" does not exist. Did you mean "list_sensors"? Available tools: latest_observation, list_sensors`,
		},
		{
			desc:      "typo in a single tool name is suggested",
			toolName:  "serach_hotels",
			available: []string{"search_hotels", "book_hotel"},
			want:      `invalid tool name: tool with name "serach_hotels" does not exist. Did you mean "search_hotels"? Available tools: book_hotel, search_hotels`,
		},
		{
			desc:      "case-insensitive match is suggested",
			toolName:  "Search-Hotels",
			available: []string{"search-hotels"},
			want:      `invalid tool name: tool with name "Search-Hotels" does not exist. Did you mean "search-hotels"? Available tools: search-hotels`,
		},
		{
			desc:      "no plausible match lists names without a suggestion",
			toolName:  "zzzzzzzzzzzz",
			available: []string{"search_hotels", "book_hotel"},
			want:      `invalid tool name: tool with name "zzzzzzzzzzzz" does not exist. Available tools: book_hotel, search_hotels`,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			got := UnknownToolError(tc.toolName, tc.available, tc.mode).Error()
			if got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestUnknownToolErrorCapsList(t *testing.T) {
	available := make([]string, 40)
	for i := range available {
		available[i] = fmt.Sprintf("tool_%02d", i)
	}
	got := UnknownToolError("nonexistent_name", available, SuggestionsFull).Error()
	if want := "(and 15 more)"; !strings.Contains(got, want) {
		t.Errorf("expected %q in error, got %q", want, got)
	}
	if strings.Contains(got, "tool_25") {
		t.Errorf("expected names past the cap to be omitted, got %q", got)
	}
	if !strings.Contains(got, "tool_24") {
		t.Errorf("expected names within the cap to be listed, got %q", got)
	}
}

func TestSuggestionModeSet(t *testing.T) {
	tcs := []struct {
		in      string
		want    SuggestionMode
		wantErr bool
	}{
		{in: "full", want: SuggestionsFull},
		{in: "FULL", want: SuggestionsFull},
		{in: "nearest", want: SuggestionsNearest},
		{in: "off", want: SuggestionsOff},
		{in: "", wantErr: true},
		{in: "verbose", wantErr: true},
	}
	for _, tc := range tcs {
		t.Run(tc.in, func(t *testing.T) {
			var mode SuggestionMode
			err := mode.Set(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error for %q, got mode %q", tc.in, mode)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if mode != tc.want {
				t.Errorf("got %q, want %q", mode, tc.want)
			}
		})
	}
}

func TestSuggestionModeStringDefaultsToFull(t *testing.T) {
	var mode SuggestionMode
	if got, want := mode.String(), string(SuggestionsFull); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNearestName(t *testing.T) {
	tcs := []struct {
		desc       string
		name       string
		candidates []string
		want       string
		wantOK     bool
	}{
		{
			desc:       "exact rename-style match",
			name:       "lookup_sensor",
			candidates: []string{"latest_observation", "list_sensors"},
			want:       "list_sensors",
			wantOK:     true,
		},
		{
			desc:       "ties resolve to the lexicographically first candidate",
			name:       "tool_x",
			candidates: []string{"tool_a", "tool_b"},
			want:       "tool_a",
			wantOK:     true,
		},
		{
			desc:       "distant names are not suggested",
			name:       "abcdefgh",
			candidates: []string{"zyxwvuts"},
			wantOK:     false,
		},
		{
			desc:       "empty candidate list",
			name:       "foo",
			candidates: nil,
			wantOK:     false,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			got, ok := nearestName(tc.name, tc.candidates)
			if ok != tc.wantOK {
				t.Fatalf("ok: got %v, want %v (suggestion %q)", ok, tc.wantOK, got)
			}
			if ok && got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLevenshtein(t *testing.T) {
	tcs := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"", "abc", 3},
		{"abc", "", 3},
		{"abc", "abc", 0},
		{"kitten", "sitting", 3},
		{"flaw", "lawn", 2},
	}
	for _, tc := range tcs {
		if got := levenshtein(tc.a, tc.b, -1); got != tc.want {
			t.Errorf("levenshtein(%q, %q, -1) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

// TestLevenshteinCutoff checks the early abort: a cutoff never changes a
// result that is genuinely below it, and saturates at the cutoff otherwise.
// Callers only compare against an incumbent, so saturating is sufficient.
func TestLevenshteinCutoff(t *testing.T) {
	tcs := []struct {
		a, b   string
		cutoff int
		want   int
	}{
		{"kitten", "sitting", -1, 3},
		{"kitten", "sitting", 10, 3},   // cutoff above the true distance is exact
		{"kitten", "sitting", 4, 3},    // still exact: 3 < 4
		{"kitten", "sitting", 3, 3},    // saturates: not better than the incumbent
		{"kitten", "sitting", 1, 1},    // saturates early
		{"abcdefgh", "zyxwvuts", 2, 2}, // hopeless pair abandoned immediately
		{"abc", "abc", 5, 0},           // identical strings still reach 0
	}
	for _, tc := range tcs {
		if got := levenshtein(tc.a, tc.b, tc.cutoff); got != tc.want {
			t.Errorf("levenshtein(%q, %q, %d) = %d, want %d", tc.a, tc.b, tc.cutoff, got, tc.want)
		}
	}
}

// naiveNearestName is the unoptimized reference: score every candidate with a
// full distance computation, then apply the same threshold. TestNearestName-
// MatchesNaive pins the optimized scan to it.
func naiveNearestName(name string, candidates []string) (string, bool) {
	lowered := strings.ToLower(name)
	best, bestDist := "", -1
	for _, c := range candidates {
		d := levenshtein(lowered, strings.ToLower(c), -1)
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

// TestNearestNameMatchesNaive proves the length pre-filter, the row-abort and
// the exact-match break are pure optimizations: over a deterministic sweep of
// name/candidate combinations, the optimized scan agrees with the reference on
// every input. Candidate lists are sorted, as nearestName requires.
func TestNearestNameMatchesNaive(t *testing.T) {
	pool := []string{
		"a", "ab", "abc", "list_sensors", "list_sensor", "latest_observation",
		"get_weather", "get_weather_data", "search_flights", "zyxwvuts",
		"tool_a", "tool_b", "TOOL_A", "", "list_tables", "execute_sql",
	}
	sorted := make([]string, len(pool))
	copy(sorted, pool)
	sort.Strings(sorted)

	names := append([]string{"lookup_sensor", "listsensors", "get_wether", "tool_x", "", "EXECUTE_SQL"}, pool...)

	// Sweep every prefix of the sorted pool so candidates arrive in varying
	// orders and sizes, including the empty list.
	for i := 0; i <= len(sorted); i++ {
		candidates := sorted[:i]
		for _, name := range names {
			gotName, gotOK := nearestName(name, candidates)
			wantName, wantOK := naiveNearestName(name, candidates)
			if gotOK != wantOK || gotName != wantName {
				t.Fatalf("nearestName(%q, %v) = (%q, %v), naive = (%q, %v)",
					name, candidates, gotName, gotOK, wantName, wantOK)
			}
		}
	}
}
