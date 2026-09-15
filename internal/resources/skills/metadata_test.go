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
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/resources"
	"github.com/googleapis/mcp-toolbox/internal/skills"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
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

	entries, err := skills.Discover(ctx, resourcesMap)
	if err != nil {
		t.Fatalf("Discover() = %v, want nil", err)
	}

	docs := skills.WithDocMetadata(entries, resourcesMap)
	if len(docs) != 1 {
		t.Fatalf("got %d doc resources, want 1: %v", len(docs), docs)
	}

	doc, ok := docs[skillURI]
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
	if _, ok := docs[refURI]; ok {
		t.Error("a supporting file was rewritten, want only SKILL.md")
	}
	if _, ok := docs[plainURI]; ok {
		t.Error("a non-skill resource was rewritten")
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

	entries, err := skills.Discover(ctx, map[string]resources.Resource{"guide": backing})
	if err != nil {
		t.Fatalf("Discover() = %v, want nil", err)
	}
	doc := skills.WithDocMetadata(entries, map[string]resources.Resource{"guide": backing})[skillURI]
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
	entries, err := skills.Discover(ctx, resourcesMap)
	if err != nil {
		t.Fatalf("Discover() = %v, want nil", err)
	}
	if docs := skills.WithDocMetadata(entries, resourcesMap); len(docs) != 0 {
		t.Errorf("got %v, want no replacements", docs)
	}
}
