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

package skills_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/log"
	"github.com/googleapis/mcp-toolbox/internal/resources"
	"github.com/googleapis/mcp-toolbox/internal/resources/skills"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/util"
)

// TestWithDocMetadata is the point of the change: a SKILL.md is published as the
// skill, not as a file called SKILL.md.
func TestWithDocMetadata(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatal(err)
	}

	const (
		skillURI    = "skill://analytics-guide/SKILL.md"
		refURI      = "skill://analytics-guide/references/queries.md"
		plainURI    = "file://project-docs"
		description = "Query and summarize the warehouse"
	)

	resourcesMap := map[string]resources.Resource{
		// The configured name and mime type are deliberately unhelpful, so the
		// assertions below cannot pass by accident.
		"guide":   textResource(t, ctx, "SKILL.md", skillURI, skillMD("analytics-guide", description)),
		"queries": textResource(t, ctx, "queries", refURI, "# Common queries\n"),
		"docs":    textResource(t, ctx, "docs", plainURI, "unrelated"),
	}

	entries, err := skills.Discover(ctx, skills.NewRegistry(resourcesMap))
	if err != nil {
		t.Fatalf("Discover() = %v, want nil", err)
	}

	docs, err := skills.WithDocMetadata(entries, resourcesMap)
	if err != nil {
		t.Fatalf("WithDocMetadata() = %v, want nil", err)
	}
	if len(docs) != 1 {
		t.Fatalf("got %d doc resources, want 1: %v", len(docs), docs)
	}

	doc, ok := docs["guide"]
	if !ok {
		t.Fatalf("no replacement for %q", skillURI)
	}
	if got := doc.GetName(); got != "analytics-guide" {
		t.Errorf("GetName() = %q, want the frontmatter name", got)
	}
	if got := doc.GetDescription(); got != description {
		t.Errorf("GetDescription() = %q, want %q", got, description)
	}
	if got := doc.GetMimeType(); got != "text/markdown" {
		t.Errorf("GetMimeType() = %q, want text/markdown", got)
	}

	// A supporting file and an unrelated resource keep their own identity.
	if _, ok := docs["queries"]; ok {
		t.Error("a supporting file was rewritten, want only SKILL.md")
	}
	if _, ok := docs["docs"]; ok {
		t.Error("a non-skill resource was rewritten")
	}
}

// TestWithDocMetadataMultipleSkills pins that each SKILL.md takes its own
// frontmatter identity rather than another skill's.
func TestWithDocMetadataMultipleSkills(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatal(err)
	}

	const (
		alphaURI = "skill://alpha-guide/SKILL.md"
		betaURI  = "skill://beta-guide/SKILL.md"
	)
	resourcesMap := map[string]resources.Resource{
		"alpha": textResource(t, ctx, "alpha", alphaURI, skillMD("alpha-guide", "Query the warehouse")),
		"notes": textResource(t, ctx, "notes", "skill://alpha-guide/references/notes.md", "# Notes\n"),
		"beta":  textResource(t, ctx, "beta", betaURI, skillMD("beta-guide", "Summarize the warehouse")),
	}

	entries, err := skills.Discover(ctx, skills.NewRegistry(resourcesMap))
	if err != nil {
		t.Fatalf("Discover() = %v, want nil", err)
	}

	docs, err := skills.WithDocMetadata(entries, resourcesMap)
	if err != nil {
		t.Fatalf("WithDocMetadata() = %v, want nil", err)
	}
	want := map[string]struct{ name, description string }{
		"alpha": {"alpha-guide", "Query the warehouse"},
		"beta":  {"beta-guide", "Summarize the warehouse"},
	}
	if len(docs) != len(want) {
		t.Fatalf("got %d doc resources, want %d: %v", len(docs), len(want), docs)
	}
	for key, w := range want {
		doc, ok := docs[key]
		if !ok {
			t.Errorf("no replacement for %q", key)
			continue
		}
		if got := doc.GetName(); got != w.name {
			t.Errorf("%s GetName() = %q, want %q", key, got, w.name)
		}
		if got := doc.GetDescription(); got != w.description {
			t.Errorf("%s GetDescription() = %q, want %q", key, got, w.description)
		}
	}
}

// TestWithDocMetadataForwards pins that only the three metadata methods change.
func TestWithDocMetadataForwards(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatal(err)
	}

	const skillURI = "skill://analytics-guide/SKILL.md"
	body := skillMD("analytics-guide", "Query and summarize the warehouse")
	backing := textResource(t, ctx, "SKILL.md", skillURI, body)

	entries, err := skills.Discover(ctx, skills.NewRegistry(map[string]resources.Resource{"guide": backing}))
	if err != nil {
		t.Fatalf("Discover() = %v, want nil", err)
	}
	docs, err := skills.WithDocMetadata(entries, map[string]resources.Resource{"guide": backing})
	if err != nil {
		t.Fatalf("WithDocMetadata() = %v, want nil", err)
	}
	doc := docs["guide"]
	if doc == nil {
		t.Fatal("no replacement for SKILL.md")
	}

	if got := doc.GetURI(); got != skillURI {
		t.Errorf("GetURI() = %q, want %q", got, skillURI)
	}
	if doc.ToConfig() == nil {
		t.Error("ToConfig() = nil, want the backing resource's config")
	}
	got, err := doc.Read(ctx, nil)
	if err != nil {
		t.Fatalf("Read() = %v, want nil", err)
	}
	if got != body {
		t.Errorf("Read() = %q, want the backing content unchanged", got)
	}
}

// TestWithDocMetadataNoSkills keeps the wrapper inert when nothing is a skill.
func TestWithDocMetadataNoSkills(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatal(err)
	}
	resourcesMap := map[string]resources.Resource{
		"docs": textResource(t, ctx, "docs", "file://project-docs", "unrelated"),
	}
	entries, err := skills.Discover(ctx, skills.NewRegistry(resourcesMap))
	if err != nil {
		t.Fatalf("Discover() = %v, want nil", err)
	}
	docs, err := skills.WithDocMetadata(entries, resourcesMap)
	if err != nil {
		t.Fatalf("WithDocMetadata() = %v, want nil", err)
	}
	if len(docs) != 0 {
		t.Errorf("got %v, want no replacements", docs)
	}
}

// TestWithDocMetadataRejectsBadFrontmatter covers the entries Discover cannot
// produce, since Entry.Validate rejects them first. A caller assembling entries
// by hand gets an error rather than a SKILL.md silently missing from the result.
func TestWithDocMetadataRejectsBadFrontmatter(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatal(err)
	}

	const skillURI = "skill://analytics-guide/SKILL.md"
	resourcesMap := map[string]resources.Resource{
		"guide": textResource(t, ctx, "guide", skillURI, skillMD("analytics-guide", "Query the warehouse")),
	}

	tcs := []struct {
		desc        string
		frontmatter map[string]any
		want        string
	}{
		{
			desc:        "no name",
			frontmatter: map[string]any{"description": "Query the warehouse"},
			want:        "frontmatter has no name",
		},
		{
			desc:        "no description",
			frontmatter: map[string]any{"name": "analytics-guide"},
			want:        "frontmatter has no description",
		},
		{
			desc:        "name is not a string",
			frontmatter: map[string]any{"name": 7, "description": "Query the warehouse"},
			want:        "frontmatter name is int, want a string",
		},
		{
			desc:        "description is empty",
			frontmatter: map[string]any{"name": "analytics-guide", "description": ""},
			want:        "frontmatter description is empty",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			entries := []skills.Entry{{URI: skillURI, Frontmatter: tc.frontmatter}}
			docs, err := skills.WithDocMetadata(entries, resourcesMap)
			if err == nil {
				t.Fatalf("WithDocMetadata() = %v, want an error", docs)
			}
			if docs != nil {
				t.Errorf("got %v alongside the error, want nil", docs)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("WithDocMetadata() = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

// TestWarnOnDocNameMismatch pins the signal an operator needs. resources/list
// publishes the config key, so a key that differs from the frontmatter name
// hides the skill from a client that reads the catalogue.
func TestWarnOnDocNameMismatch(t *testing.T) {
	var stderr bytes.Buffer
	logger, err := log.NewStdLogger(io.Discard, &stderr, "info")
	if err != nil {
		t.Fatal(err)
	}
	ctx := util.WithLogger(context.Background(), logger)

	resourcesMap := map[string]resources.Resource{
		"guide":   textResource(t, ctx, "guide", "skill://analytics-guide/SKILL.md", skillMD("analytics-guide", "Query the warehouse")),
		"other":   textResource(t, ctx, "other", "skill://other/SKILL.md", skillMD("other", "A skill named for its key")),
		"queries": textResource(t, ctx, "queries", "skill://analytics-guide/references/queries.md", "# Common queries\n"),
	}

	reg := skills.NewRegistry(resourcesMap)
	entries, err := skills.Discover(ctx, reg)
	if err != nil {
		t.Fatalf("Discover() = %v, want nil", err)
	}
	if err := skills.WarnOnDocNameMismatch(ctx, entries, reg); err != nil {
		t.Fatalf("WarnOnDocNameMismatch() = %v, want nil", err)
	}

	got := stderr.String()
	for _, want := range []string{`resource \"guide\"`, `skill \"analytics-guide\"`} {
		if !strings.Contains(got, want) {
			t.Errorf("warning %q does not mention %q", got, want)
		}
	}
	// A key that matches, and a supporting file, are not mismatches.
	for _, unwanted := range []string{`resource \"other\"`, `resource \"queries\"`} {
		if strings.Contains(got, unwanted) {
			t.Errorf("warning %q reports %q", got, unwanted)
		}
	}
}

// TestNoDocNameMismatchWarning guards the other direction: a key that matches
// must not warn, or the warning is noise an operator learns to ignore.
func TestNoDocNameMismatchWarning(t *testing.T) {
	var stderr bytes.Buffer
	logger, err := log.NewStdLogger(io.Discard, &stderr, "info")
	if err != nil {
		t.Fatal(err)
	}
	ctx := util.WithLogger(context.Background(), logger)

	resourcesMap := map[string]resources.Resource{
		"analytics-guide": textResource(t, ctx, "analytics-guide", "skill://analytics-guide/SKILL.md", skillMD("analytics-guide", "Query the warehouse")),
	}

	reg := skills.NewRegistry(resourcesMap)
	entries, err := skills.Discover(ctx, reg)
	if err != nil {
		t.Fatalf("Discover() = %v, want nil", err)
	}
	if err := skills.WarnOnDocNameMismatch(ctx, entries, reg); err != nil {
		t.Fatalf("WarnOnDocNameMismatch() = %v, want nil", err)
	}
	if got := stderr.String(); strings.Contains(got, "Rename the resource") {
		t.Errorf("unexpected name-mismatch warning: %q", got)
	}
}
