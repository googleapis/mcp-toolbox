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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/log"
	"github.com/googleapis/mcp-toolbox/internal/resources"
	"github.com/googleapis/mcp-toolbox/internal/resources/text"
	"github.com/googleapis/mcp-toolbox/internal/skills"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/util"
)

// textResource builds a real text resource rather than a mock, so discovery is
// exercised against the same Read path a hand-declared skill uses.
func textResource(t *testing.T, ctx context.Context, name, uri, content string) resources.Resource {
	t.Helper()
	cfg := &text.Config{
		ResourceConfigBase: resources.ResourceConfigBase{
			ConfigBase: resources.ConfigBase{Name: name, Type: "text", MimeType: "text/markdown"},
			URI:        uri,
		},
		Text: content,
	}
	res, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("unable to initialize %q: %s", uri, err)
	}
	return res
}

func skillMD(name, description string) string {
	return fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n\n# %s\n", name, description, name)
}

func digestOf(s string) string {
	sum := sha256.Sum256([]byte(s))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func TestDiscover(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatal(err)
	}

	const (
		mdBody      = "# Common queries\n"
		unrelatedMD = "not part of any skill"
	)
	skillDoc := skillMD("analytics-guide", "Query and summarize the warehouse")

	resourcesMap := map[string]resources.Resource{
		"guide/SKILL.md": textResource(t, ctx, "guide/SKILL.md",
			"skill://analytics-guide/SKILL.md", skillDoc),
		"guide/queries": textResource(t, ctx, "guide/queries",
			"skill://analytics-guide/references/queries.md", mdBody),
		// Neither of these belongs to the skill: one is an ordinary resource,
		// the other shares a name prefix but not a path prefix.
		"docs": textResource(t, ctx, "docs", "file://project-docs", unrelatedMD),
		"decoy": textResource(t, ctx, "decoy",
			"skill://analytics-guide-v2/SKILL.md", skillMD("analytics-guide-v2", "A different skill")),
	}

	entries, err := skills.Discover(ctx, resourcesMap)
	if err != nil {
		t.Fatalf("Discover() = %v, want nil", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2 (analytics-guide and analytics-guide-v2)", len(entries))
	}

	got := entries[0]
	if got.URI != "skill://analytics-guide/SKILL.md" {
		t.Errorf("URI = %q, want the SKILL.md URI", got.URI)
	}
	if name := got.Frontmatter["name"]; name != "analytics-guide" {
		t.Errorf("frontmatter name = %v, want analytics-guide", name)
	}
	if got.Resources.Dynamic {
		t.Error("Dynamic = true, want a static manifest")
	}

	// Two files, sorted by URI, each digested over the bytes Read returns.
	want := []skills.ResourceRef{
		{URI: "skill://analytics-guide/SKILL.md", Digest: digestOf(skillDoc), Size: int64(len(skillDoc))},
		{URI: "skill://analytics-guide/references/queries.md", Digest: digestOf(mdBody), Size: int64(len(mdBody))},
	}
	if len(got.Resources.Refs) != len(want) {
		t.Fatalf("got %d refs, want %d: %+v", len(got.Resources.Refs), len(want), got.Resources.Refs)
	}
	for i, w := range want {
		if got.Resources.Refs[i] != w {
			t.Errorf("ref %d = %+v, want %+v", i, got.Resources.Refs[i], w)
		}
	}

	// The decoy shares a name prefix but not a path prefix, so it must be its
	// own skill rather than a file of the first.
	if entries[1].URI != "skill://analytics-guide-v2/SKILL.md" {
		t.Errorf("second entry = %q, want the decoy as its own skill", entries[1].URI)
	}
	if n := len(entries[1].Resources.Refs); n != 1 {
		t.Errorf("decoy has %d refs, want 1 — analytics-guide's files must not leak in", n)
	}
}

// TestDiscoverNestedSkill pins the SEP rule that a nested skill is published as
// its own entry while its files stay listed in the enclosing skill's manifest.
func TestDiscoverNestedSkill(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatal(err)
	}

	resourcesMap := map[string]resources.Resource{
		"parent": textResource(t, ctx, "parent", "skill://acme/billing/SKILL.md",
			skillMD("billing", "Billing workflows")),
		"child": textResource(t, ctx, "child", "skill://acme/billing/refunds/SKILL.md",
			skillMD("refunds", "Refund workflows")),
	}

	entries, err := skills.Discover(ctx, resourcesMap)
	if err != nil {
		t.Fatalf("Discover() = %v, want nil", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}

	byURI := map[string]skills.Entry{}
	for _, e := range entries {
		byURI[e.URI] = e
	}

	parent, ok := byURI["skill://acme/billing/SKILL.md"]
	if !ok {
		t.Fatal("enclosing skill missing from the entries")
	}
	if n := len(parent.Resources.Refs); n != 2 {
		t.Errorf("enclosing manifest has %d refs, want 2 — a nested skill's files stay listed in it", n)
	}

	child, ok := byURI["skill://acme/billing/refunds/SKILL.md"]
	if !ok {
		t.Fatal("nested skill missing from the entries")
	}
	if n := len(child.Resources.Refs); n != 1 {
		t.Errorf("nested manifest has %d refs, want 1", n)
	}
}

func TestDiscoverErrors(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatal(err)
	}

	tcs := []struct {
		desc    string
		uri     string
		content string
		wantErr string
	}{
		{
			desc:    "SKILL.md without frontmatter",
			uri:     "skill://guide/SKILL.md",
			content: "# Just a heading\n",
			wantErr: "must open with YAML frontmatter",
		},
		{
			desc:    "frontmatter never closed",
			uri:     "skill://guide/SKILL.md",
			content: "---\nname: guide\n",
			wantErr: "not closed by ---",
		},
		{
			desc:    "frontmatter missing description",
			uri:     "skill://guide/SKILL.md",
			content: "---\nname: guide\n---\n",
			wantErr: "description",
		},
		{
			desc:    "frontmatter name disagrees with the URI",
			uri:     "skill://guide/SKILL.md",
			content: skillMD("something-else", "Mismatched"),
			wantErr: "name",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			resourcesMap := map[string]resources.Resource{
				"s": textResource(t, ctx, "s", tc.uri, tc.content),
			}
			_, err := skills.Discover(ctx, resourcesMap)
			if err == nil {
				t.Fatalf("Discover() = nil, want error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("Discover() = %v, want error containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestDiscoverNoSkills(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatal(err)
	}

	resourcesMap := map[string]resources.Resource{
		"docs": textResource(t, ctx, "docs", "file://project-docs", "hello"),
		// A skill:// resource that is not a SKILL.md does not make a skill on
		// its own; without a SKILL.md there is nothing to key an entry on.
		"orphan": textResource(t, ctx, "orphan", "skill://guide/references/orphan.md", "hello"),
	}

	entries, err := skills.Discover(ctx, resourcesMap)
	if err != nil {
		t.Fatalf("Discover() = %v, want nil", err)
	}
	if len(entries) != 0 {
		t.Errorf("got %d entries, want none", len(entries))
	}
}

// TestDiscoverCRLFFrontmatter covers a SKILL.md checked out with Windows line
// endings. Only the delimiters are normalised, so the digest still covers the
// raw bytes the resource returns.
func TestDiscoverCRLFFrontmatter(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatal(err)
	}

	content := "---\r\nname: guide\r\ndescription: Windows line endings\r\n---\r\n\r\n# guide\r\n"
	resourcesMap := map[string]resources.Resource{
		"s": textResource(t, ctx, "s", "skill://guide/SKILL.md", content),
	}

	entries, err := skills.Discover(ctx, resourcesMap)
	if err != nil {
		t.Fatalf("Discover() = %v, want nil", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if name := entries[0].Frontmatter["name"]; name != "guide" {
		t.Errorf("frontmatter name = %v, want guide", name)
	}
	if got, want := entries[0].Resources.Refs[0].Digest, digestOf(content); got != want {
		t.Errorf("digest = %s, want %s — normalising must not change what is hashed", got, want)
	}
}

// TestDiscoverNoLogger covers the boot-time contract: discovery needs a logger
// to report duplicate names, and a context without one is a wiring error
// rather than a condition to skip past silently.
func TestDiscoverNoLogger(t *testing.T) {
	resourcesMap := map[string]resources.Resource{
		"s": textResource(t, mustLoggerCtx(t), "s", "skill://guide/SKILL.md",
			"---\nname: guide\ndescription: A guide\n---\n\n# guide\n"),
	}

	_, err := skills.Discover(context.Background(), resourcesMap)
	if err == nil {
		t.Fatal("Discover() with no logger in context = nil, want an error")
	}
	if !strings.Contains(err.Error(), "duplicate skill names") {
		t.Errorf("error = %q, want it to name the operation that failed", err)
	}
}

func mustLoggerCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

// TestDiscoverFrontmatterDelimiters pins the closing delimiter to a line of its
// own. A line merely starting with --- must not end the frontmatter, or a file
// that never closes it is accepted with a silently truncated header.
func TestDiscoverFrontmatterDelimiters(t *testing.T) {
	const header = "---\nname: guide\ndescription: A guide\n"
	tcs := []struct {
		desc    string
		content string
		wantErr string
	}{
		{"closed and followed by a body", header + "---\n\n# guide\n", ""},
		{"closed at end of file", header + "---", ""},
		{"horizontal rule in the body", header + "---\n\n# guide\n\n---\n\ntext\n", ""},
		{"trailing whitespace on the delimiter", header + "--- \n\n# guide\n", ""},
		{"trailing whitespace on the opening delimiter", "--- \n" + header[4:] + "---\n", ""},
		{"utf-8 bom before the opening delimiter", "\ufeff" + header + "---\n", ""},
		{"never closed", header + "---extra stuff\n", "not closed by ---"},
		{"four dashes do not open frontmatter", "----\n" + header[4:] + "---\n", "must open with YAML frontmatter"},
		{"leading blank line", "\n" + header + "---\n", "must open with YAML frontmatter"},
		{"run of dashes is not a delimiter", header + "----------\n---\n", "unable to parse"},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			ctx := mustLoggerCtx(t)
			m := map[string]resources.Resource{
				"s": textResource(t, ctx, "s", "skill://guide/SKILL.md", tc.content),
			}
			_, err := skills.Discover(ctx, m)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("Discover() = %v, want nil", err)
			case tc.wantErr != "" && err == nil:
				t.Fatalf("Discover() = nil, want an error containing %q", tc.wantErr)
			case tc.wantErr != "" && !strings.Contains(err.Error(), tc.wantErr):
				t.Errorf("Discover() = %v, want an error containing %q", err, tc.wantErr)
			}
		})
	}
}

// badResource stands in for a resource type whose Read does not yield text.
// Every other method is unused by discovery.
type badResource struct {
	uri     string
	content any
	err     error
}

func (r badResource) GetURI() string                                    { return r.uri }
func (r badResource) GetName() string                                   { return r.uri }
func (r badResource) GetTitle() string                                  { return "" }
func (r badResource) GetDescription() string                            { return "" }
func (r badResource) GetMimeType() string                               { return "text/markdown" }
func (r badResource) GetAnnotations() *resources.ResourceAnnotations    { return nil }
func (r badResource) GetSize() *int64                                   { return nil }
func (r badResource) GetResourceUIMetadata() any                        { return nil }
func (r badResource) IsUI() bool                                        { return false }
func (r badResource) ToConfig() resources.ResourceConfig                { return nil }
func (r badResource) Read(context.Context, map[string]any) (any, error) { return r.content, r.err }

// TestDiscoverWarnsOnDuplicateNames pins the one thing warnOnDuplicateNames
// does. Entry.Validate ties the frontmatter name to the final skill-path
// segment, so a duplicate can only arise from differing parent paths.
func TestDiscoverWarnsOnDuplicateNames(t *testing.T) {
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

	entries, err := skills.Discover(ctx, resourcesMap)
	if err != nil {
		t.Fatalf("Discover() = %v, want nil", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}

	got := stderr.String()
	for _, want := range []string{
		"skill://acme/guide/SKILL.md",
		"skill://other/guide/SKILL.md",
		`share the name \"guide\"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("warning %q does not mention %q", got, want)
		}
	}
}

// TestDiscoverNoDuplicateWarning guards the other direction: distinct names
// must not warn, or the warning is noise an operator learns to ignore.
func TestDiscoverNoDuplicateWarning(t *testing.T) {
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
	if _, err := skills.Discover(ctx, resourcesMap); err != nil {
		t.Fatalf("Discover() = %v, want nil", err)
	}
	if got := stderr.String(); strings.Contains(got, "share the name") {
		t.Errorf("unexpected duplicate-name warning: %q", got)
	}
}

// TestDiscoverUnreadableResource covers the two ways a member can fail to
// produce text. A skill's files must be textual, so both are errors rather
// than a skipped file.
func TestDiscoverUnreadableResource(t *testing.T) {
	tcs := []struct {
		desc    string
		res     resources.Resource
		wantErr string
	}{
		{
			desc:    "Read fails",
			res:     badResource{uri: "skill://guide/refs/data.md", err: fmt.Errorf("backend is down")},
			wantErr: "unable to read",
		},
		{
			desc:    "Read returns non-text content",
			res:     badResource{uri: "skill://guide/refs/data.md", content: []byte("raw")},
			wantErr: "want text content",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			ctx := mustLoggerCtx(t)
			resourcesMap := map[string]resources.Resource{
				"s": textResource(t, ctx, "s", "skill://guide/SKILL.md", skillMD("guide", "A guide")),
				"d": tc.res,
			}
			_, err := skills.Discover(ctx, resourcesMap)
			if err == nil {
				t.Fatalf("Discover() = nil, want an error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("Discover() = %v, want an error containing %q", err, tc.wantErr)
			}
			if !strings.Contains(err.Error(), "skill://guide/SKILL.md") {
				t.Errorf("Discover() = %v, want the error to name the skill", err)
			}
		})
	}
}

// TestDiscoverTooManyFiles pins the ref-count limit being applied before the
// files are read, so an oversized skill is rejected without being pulled into
// memory first.
func TestDiscoverTooManyFiles(t *testing.T) {
	ctx := mustLoggerCtx(t)
	resourcesMap := map[string]resources.Resource{
		"s": textResource(t, ctx, "s", "skill://guide/SKILL.md", skillMD("guide", "A guide")),
	}
	// One over the limit once the SKILL.md itself is counted. Each would fail
	// the read if it were reached, which is what makes the ordering visible.
	for i := 0; i < skills.MaxRefs; i++ {
		uri := fmt.Sprintf("skill://guide/refs/%03d.md", i)
		resourcesMap[uri] = badResource{uri: uri, content: []byte("unreadable")}
	}

	_, err := skills.Discover(ctx, resourcesMap)
	if err == nil {
		t.Fatal("Discover() = nil, want an error")
	}
	if !strings.Contains(err.Error(), "exceeds the limit") {
		t.Errorf("Discover() = %v, want the ref-count limit error", err)
	}
	if strings.Contains(err.Error(), "want text content") {
		t.Errorf("Discover() = %v, want the limit checked before any file is read", err)
	}
}
