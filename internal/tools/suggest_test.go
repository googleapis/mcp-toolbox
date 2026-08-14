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
	"strings"
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/util"
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

func TestSuggestionModeAtMost(t *testing.T) {
	tcs := []struct {
		desc    string
		mode    SuggestionMode
		ceiling SuggestionMode
		want    SuggestionMode
	}{
		{desc: "ceiling lowers full", mode: SuggestionsFull, ceiling: SuggestionsNearest, want: SuggestionsNearest},
		{desc: "ceiling does not raise off", mode: SuggestionsOff, ceiling: SuggestionsNearest, want: SuggestionsOff},
		{desc: "ceiling does not raise nearest", mode: SuggestionsNearest, ceiling: SuggestionsFull, want: SuggestionsNearest},
		{desc: "unset mode is treated as full", mode: "", ceiling: SuggestionsNearest, want: SuggestionsNearest},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			if got := tc.mode.AtMost(tc.ceiling); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSuggestionModeFromContext(t *testing.T) {
	tcs := []struct {
		desc string
		ctx  context.Context
		want SuggestionMode
	}{
		{desc: "unset context defaults to full", ctx: context.Background(), want: SuggestionsFull},
		{desc: "empty value defaults to full", ctx: util.WithToolSuggestions(context.Background(), ""), want: SuggestionsFull},
		{desc: "unrecognized value defaults to full", ctx: util.WithToolSuggestions(context.Background(), "loud"), want: SuggestionsFull},
		{desc: "off is honored", ctx: util.WithToolSuggestions(context.Background(), "off"), want: SuggestionsOff},
		{desc: "nearest is honored", ctx: util.WithToolSuggestions(context.Background(), "nearest"), want: SuggestionsNearest},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			if got := SuggestionModeFromContext(tc.ctx); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
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
		if got := levenshtein(tc.a, tc.b); got != tc.want {
			t.Errorf("levenshtein(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
