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
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/log"
	"github.com/googleapis/mcp-toolbox/internal/resources"
	"github.com/googleapis/mcp-toolbox/internal/resources/skills"
	"github.com/googleapis/mcp-toolbox/internal/util"
)

// unreadResource fails the test if Validate reads it. Startup validation reads
// only SKILL.md, so a supporting file must never reach Read.
type unreadResource struct {
	badResource
	t    *testing.T
	size int64
}

func (r unreadResource) GetSize() *int64 { return &r.size }

func (r unreadResource) Read(context.Context, map[string]any) (any, error) {
	r.t.Errorf("Read(%q) was called, want Validate to read only SKILL.md", r.uri)
	return "", nil
}

func TestValidate(t *testing.T) {
	ctx := mustLoggerCtx(t)
	resourcesMap := map[string]resources.Resource{
		"guide": textResource(t, ctx, "guide", "skill://analytics-guide/SKILL.md",
			skillMD("analytics-guide", "Query and summarize the warehouse")),
		"queries": unreadResource{
			badResource: badResource{uri: "skill://analytics-guide/references/queries.md"},
			t:           t,
			size:        962,
		},
		"docs": textResource(t, ctx, "docs", "file://project-docs", "not a skill"),
	}

	got, err := skills.Validate(ctx, skills.NewRegistry(resourcesMap))
	if err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d skills, want 1", len(got))
	}
	if got[0].URI != "skill://analytics-guide/SKILL.md" {
		t.Errorf("URI = %q, want the SKILL.md URI", got[0].URI)
	}
	if name := got[0].Frontmatter["name"]; name != "analytics-guide" {
		t.Errorf("frontmatter name = %v, want analytics-guide", name)
	}
}

func TestValidateNoSkills(t *testing.T) {
	ctx := mustLoggerCtx(t)
	resourcesMap := map[string]resources.Resource{
		"docs": textResource(t, ctx, "docs", "file://project-docs", "hello"),
	}
	got, err := skills.Validate(ctx, skills.NewRegistry(resourcesMap))
	if err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d skills, want 0", len(got))
	}
}

// TestValidateNestedSkill checks that a nested skill validates as its own
// skill and that its files count toward the enclosing one.
func TestValidateNestedSkill(t *testing.T) {
	ctx := mustLoggerCtx(t)
	resourcesMap := map[string]resources.Resource{
		"parent": textResource(t, ctx, "parent", "skill://acme/billing/SKILL.md",
			skillMD("billing", "Billing workflows")),
		"child": textResource(t, ctx, "child", "skill://acme/billing/refunds/SKILL.md",
			skillMD("refunds", "Refund workflows")),
	}
	got, err := skills.Validate(ctx, skills.NewRegistry(resourcesMap))
	if err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d skills, want 2", len(got))
	}
}

func TestValidateErrors(t *testing.T) {
	tcs := []struct {
		desc    string
		content string
		wantErr string
	}{
		{"SKILL.md without frontmatter", "# Just a heading\n", "must open with YAML frontmatter"},
		{"frontmatter never closed", "---\nname: guide\n", "not closed by ---"},
		{"frontmatter missing description", "---\nname: guide\n---\n", "description"},
		{"frontmatter name disagrees with the URI", skillMD("something-else", "Mismatched"), "name"},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			ctx := mustLoggerCtx(t)
			resourcesMap := map[string]resources.Resource{
				"s": textResource(t, ctx, "s", "skill://guide/SKILL.md", tc.content),
			}
			_, err := skills.Validate(ctx, skills.NewRegistry(resourcesMap))
			if err == nil {
				t.Fatalf("Validate() = nil, want error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("Validate() = %v, want error containing %q", err, tc.wantErr)
			}
		})
	}
}

// TestValidateTooManyFiles checks the ref-count limit without reading any
// supporting file.
func TestValidateTooManyFiles(t *testing.T) {
	ctx := mustLoggerCtx(t)
	resourcesMap := map[string]resources.Resource{
		"s": textResource(t, ctx, "s", "skill://guide/SKILL.md", skillMD("guide", "A guide")),
	}
	// One over the limit once the SKILL.md itself is counted.
	for i := 0; i < skills.MaxRefs; i++ {
		uri := fmt.Sprintf("skill://guide/refs/%03d.md", i)
		resourcesMap[uri] = unreadResource{badResource: badResource{uri: uri}, t: t, size: 1}
	}

	_, err := skills.Validate(ctx, skills.NewRegistry(resourcesMap))
	if err == nil {
		t.Fatal("Validate() = nil, want an error")
	}
	if !strings.Contains(err.Error(), "exceeds the limit") {
		t.Errorf("Validate() = %v, want the ref-count limit error", err)
	}
}

// TestValidateOversizeSkill checks the total-size limit from the reported
// sizes, without reading the files.
func TestValidateOversizeSkill(t *testing.T) {
	ctx := mustLoggerCtx(t)
	resourcesMap := map[string]resources.Resource{
		"guide": textResource(t, ctx, "guide", "skill://analytics-guide/SKILL.md",
			skillMD("analytics-guide", "Query and summarize the warehouse")),
	}
	const chunk = 4 << 20 // 4 MiB per file. Four files exceed the 16 MiB limit.
	for i := range 4 {
		uri := fmt.Sprintf("skill://analytics-guide/refs/%d.md", i)
		resourcesMap[uri] = unreadResource{badResource: badResource{uri: uri}, t: t, size: chunk}
	}

	_, err := skills.Validate(ctx, skills.NewRegistry(resourcesMap))
	if err == nil {
		t.Fatal("Validate() = nil, want a total-size error")
	}
	if !strings.Contains(err.Error(), "total size exceeds the limit") {
		t.Errorf("Validate() = %v, want a total-size error", err)
	}
	if !strings.Contains(err.Error(), "skill://analytics-guide/SKILL.md") {
		t.Errorf("Validate() = %v, want the error to name the skill", err)
	}
}

// TestValidateUnreadableSkillFile checks that a SKILL.md that yields no text
// fails the load and names the skill.
func TestValidateUnreadableSkillFile(t *testing.T) {
	ctx := mustLoggerCtx(t)
	resourcesMap := map[string]resources.Resource{
		"s": badResource{uri: "skill://guide/SKILL.md", err: fmt.Errorf("backend is down")},
	}
	_, err := skills.Validate(ctx, skills.NewRegistry(resourcesMap))
	if err == nil {
		t.Fatal("Validate() = nil, want an error")
	}
	if !strings.Contains(err.Error(), "unable to read") || !strings.Contains(err.Error(), "skill://guide/SKILL.md") {
		t.Errorf("Validate() = %v, want a read error naming the skill", err)
	}
}

// TestValidateNoLogger covers the boot-time contract: validation needs a logger
// to report duplicate names, and a context without one is a wiring error
// rather than a condition to skip past silently.
func TestValidateNoLogger(t *testing.T) {
	resourcesMap := map[string]resources.Resource{
		"s": textResource(t, mustLoggerCtx(t), "s", "skill://guide/SKILL.md",
			"---\nname: guide\ndescription: A guide\n---\n\n# guide\n"),
	}

	_, err := skills.Validate(context.Background(), skills.NewRegistry(resourcesMap))
	if err == nil {
		t.Fatal("Validate() with no logger in context = nil, want an error")
	}
	if !strings.Contains(err.Error(), "duplicate skill names") {
		t.Errorf("error = %q, want it to name the operation that failed", err)
	}
}

// TestValidateWarnsOnDuplicateNames pins the one thing warnOnDuplicateNames
// does. Entry validation ties the frontmatter name to the final skill-path
// segment, so a duplicate can only arise from differing parent paths.
func TestValidateWarnsOnDuplicateNames(t *testing.T) {
	var stderr bytes.Buffer
	logger, err := log.NewStdLogger(io.Discard, &stderr, "info")
	if err != nil {
		t.Fatal(err)
	}
	ctx := util.WithLogger(context.Background(), logger)

	resourcesMap := map[string]resources.Resource{
		"a": textResource(t, ctx, "a", "skill://acme/guide/SKILL.md", skillMD("guide", "One")),
		"b": textResource(t, ctx, "b", "skill://other/guide/SKILL.md", skillMD("guide", "Two")),
	}

	got, err := skills.Validate(ctx, skills.NewRegistry(resourcesMap))
	if err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d skills, want 2", len(got))
	}

	out := stderr.String()
	for _, want := range []string{
		"skill://acme/guide/SKILL.md",
		"skill://other/guide/SKILL.md",
		`share the name \"guide\"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("warning %q does not mention %q", out, want)
		}
	}
}

// TestValidateNoDuplicateWarning guards the other direction: distinct names
// must not warn, or the warning is noise an operator learns to ignore.
func TestValidateNoDuplicateWarning(t *testing.T) {
	var stderr bytes.Buffer
	logger, err := log.NewStdLogger(io.Discard, &stderr, "info")
	if err != nil {
		t.Fatal(err)
	}
	ctx := util.WithLogger(context.Background(), logger)

	resourcesMap := map[string]resources.Resource{
		"a": textResource(t, ctx, "a", "skill://acme/guide/SKILL.md", skillMD("guide", "One")),
		"b": textResource(t, ctx, "b", "skill://acme/other/SKILL.md", skillMD("other", "Two")),
	}
	if _, err := skills.Validate(ctx, skills.NewRegistry(resourcesMap)); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
	if got := stderr.String(); strings.Contains(got, "share the name") {
		t.Errorf("unexpected duplicate-name warning: %q", got)
	}
}

// TestDiscoverDoesNotWarn pins that Discover, which runs per request, leaves
// the duplicate-name warning to startup.
func TestDiscoverDoesNotWarn(t *testing.T) {
	var stderr bytes.Buffer
	logger, err := log.NewStdLogger(io.Discard, &stderr, "info")
	if err != nil {
		t.Fatal(err)
	}
	ctx := util.WithLogger(context.Background(), logger)

	resourcesMap := map[string]resources.Resource{
		"a": textResource(t, ctx, "a", "skill://acme/guide/SKILL.md", skillMD("guide", "One")),
		"b": textResource(t, ctx, "b", "skill://other/guide/SKILL.md", skillMD("guide", "Two")),
	}
	if _, err := skills.Discover(ctx, skills.NewRegistry(resourcesMap)); err != nil {
		t.Fatalf("Discover() = %v, want nil", err)
	}
	if got := stderr.String(); strings.Contains(got, "share the name") {
		t.Errorf("Discover() warned %q, want the warning left to Validate", got)
	}
}
