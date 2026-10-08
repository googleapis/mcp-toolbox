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
	"path"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
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

// skillAt builds a valid SKILL.md at uri, named for its final skill-path segment.
func skillAt(t *testing.T, ctx context.Context, uri, description string) resources.Resource {
	t.Helper()
	name := path.Base(path.Dir(uri))
	return textResource(t, ctx, uri, uri, skillMD(name, description))
}

// withUnreadFiles adds n supporting files of the given size under root.
func withUnreadFiles(t *testing.T, m map[string]resources.Resource, root string, n int, size int64) map[string]resources.Resource {
	for i := range n {
		uri := fmt.Sprintf("%s/refs/%03d.md", root, i)
		m[uri] = unreadResource{badResource: badResource{uri: uri}, t: t, size: size}
	}
	return m
}

func TestValidate(t *testing.T) {
	ctx := mustLoggerCtx(t)
	fm := func(name, desc string) map[string]any {
		return map[string]any{"name": name, "description": desc}
	}
	tcs := []struct {
		desc      string
		resources func(t *testing.T) map[string]resources.Resource
		want      []skills.Skill
	}{
		{
			desc: "a skill with a supporting file, beside a non-skill resource",
			resources: func(t *testing.T) map[string]resources.Resource {
				return withUnreadFiles(t, map[string]resources.Resource{
					"guide": skillAt(t, ctx, "skill://analytics-guide/SKILL.md", "Query the warehouse"),
					"docs":  textResource(t, ctx, "docs", "file://project-docs", "not a skill"),
				}, "skill://analytics-guide", 1, 962)
			},
			want: []skills.Skill{
				{URI: "skill://analytics-guide/SKILL.md", Frontmatter: fm("analytics-guide", "Query the warehouse")},
			},
		},
		{
			desc: "no skills",
			resources: func(t *testing.T) map[string]resources.Resource {
				return map[string]resources.Resource{
					"docs": textResource(t, ctx, "docs", "file://project-docs", "hello"),
				}
			},
		},
		{
			// The child's SKILL.md also counts toward the parent's files.
			desc: "a nested skill validates as its own skill",
			resources: func(t *testing.T) map[string]resources.Resource {
				return map[string]resources.Resource{
					"parent": skillAt(t, ctx, "skill://acme/billing/SKILL.md", "Billing workflows"),
					"child":  skillAt(t, ctx, "skill://acme/billing/refunds/SKILL.md", "Refund workflows"),
				}
			},
			want: []skills.Skill{
				{URI: "skill://acme/billing/SKILL.md", Frontmatter: fm("billing", "Billing workflows")},
				{URI: "skill://acme/billing/refunds/SKILL.md", Frontmatter: fm("refunds", "Refund workflows")},
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			got, err := skills.Validate(ctx, tc.resources(t))
			if err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("Validate() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestValidateErrors(t *testing.T) {
	loggerCtx := mustLoggerCtx(t)
	const uri = "skill://guide/SKILL.md"
	guide := func(t *testing.T, content string) map[string]resources.Resource {
		return map[string]resources.Resource{"s": textResource(t, loggerCtx, "s", uri, content)}
	}
	tcs := []struct {
		desc      string
		ctx       context.Context // defaults to one with a logger
		resources func(t *testing.T) map[string]resources.Resource
		wantErr   []string
	}{
		{
			desc:      "SKILL.md without frontmatter",
			resources: func(t *testing.T) map[string]resources.Resource { return guide(t, "# Just a heading\n") },
			wantErr:   []string{"must open with YAML frontmatter"},
		},
		{
			desc:      "frontmatter never closed",
			resources: func(t *testing.T) map[string]resources.Resource { return guide(t, "---\nname: guide\n") },
			wantErr:   []string{"not closed by ---"},
		},
		{
			desc:      "frontmatter missing description",
			resources: func(t *testing.T) map[string]resources.Resource { return guide(t, "---\nname: guide\n---\n") },
			wantErr:   []string{"description"},
		},
		{
			desc: "frontmatter name disagrees with the URI",
			resources: func(t *testing.T) map[string]resources.Resource {
				return guide(t, skillMD("something-else", "Mismatched"))
			},
			wantErr: []string{"name"},
		},
		{
			// One over the limit once SKILL.md itself is counted.
			desc: "too many files",
			resources: func(t *testing.T) map[string]resources.Resource {
				return withUnreadFiles(t, guide(t, skillMD("guide", "A guide")), "skill://guide", skills.MaxRefs, 1)
			},
			wantErr: []string{"exceeds the limit", uri},
		},
		{
			// Four 4 MiB files exceed 16 MiB from reported sizes alone.
			desc: "total size over the limit",
			resources: func(t *testing.T) map[string]resources.Resource {
				return withUnreadFiles(t, guide(t, skillMD("guide", "A guide")), "skill://guide", 4, 4<<20)
			},
			wantErr: []string{"total size exceeds the limit", uri},
		},
		{
			desc: "unreadable SKILL.md",
			resources: func(t *testing.T) map[string]resources.Resource {
				return map[string]resources.Resource{"s": badResource{uri: uri, err: fmt.Errorf("backend is down")}}
			},
			wantErr: []string{"unable to read", uri},
		},
		{
			// Validation needs a logger to report duplicate names; a context
			// without one is a wiring error, not something to skip silently.
			desc:      "no logger in the context",
			ctx:       context.Background(),
			resources: func(t *testing.T) map[string]resources.Resource { return guide(t, skillMD("guide", "A guide")) },
			wantErr:   []string{"duplicate skill names"},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			ctx := tc.ctx
			if ctx == nil {
				ctx = loggerCtx
			}
			_, err := skills.Validate(ctx, tc.resources(t))
			if err == nil {
				t.Fatalf("Validate() = nil, want error containing %q", tc.wantErr)
			}
			for _, want := range tc.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("Validate() = %v, want error containing %q", err, want)
				}
			}
		})
	}
}

// TestValidateWarnings checks the two warnings Validate logs, and that each
// appears only when it should:
//   - Two skills share a frontmatter name. Entry validation ties the name to
//     the final skill-path segment, so a duplicate can only arise from
//     differing parent paths.
//   - A skill:// resource has no SKILL.md above it. Most often such a URI has
//     a typo.
func TestValidateWarnings(t *testing.T) {
	tcs := []struct {
		desc      string
		skills    []string          // SKILL.md URIs
		files     map[string]string // other resource URI to content
		wantFound int
		wantWarn  []string // substrings the log must contain
		noWarn    []string // substrings the log must not contain
	}{
		{
			desc:      "same name under different parents",
			skills:    []string{"skill://acme/guide/SKILL.md", "skill://other/guide/SKILL.md"},
			wantFound: 2,
			wantWarn:  []string{"skill://acme/guide/SKILL.md", "skill://other/guide/SKILL.md", `share the name \"guide\"`},
		},
		{
			desc:      "distinct names",
			skills:    []string{"skill://acme/guide/SKILL.md", "skill://acme/other/SKILL.md"},
			wantFound: 2,
			noWarn:    []string{"share the name"},
		},
		{
			desc:   "orphans from a typo and a name-prefix sibling",
			skills: []string{"skill://analytics-guide/SKILL.md"},
			files: map[string]string{
				// A typo in the skill path: close to the skill, under none.
				"skill://analytcs-guide/references/joins.md": "# Joins\n",
				// A name prefix of the skill, not a path prefix.
				"skill://analytics-guide-v2/notes.md": "# Notes\n",
				// Not addressed by skill://, so it is never an orphan.
				"file://project-docs": "unrelated",
			},
			wantFound: 1,
			wantWarn:  []string{"skill://analytcs-guide/references/joins.md", "skill://analytics-guide-v2/notes.md", "belong to no skill"},
			noWarn:    []string{"project-docs"},
		},
		{
			desc:      "every skill:// file is under a skill",
			skills:    []string{"skill://analytics-guide/SKILL.md"},
			files:     map[string]string{"skill://analytics-guide/references/queries.md": "# Common queries\n"},
			wantFound: 1,
			noWarn:    []string{"belong to no skill"},
		},
		{
			// With no SKILL.md at all, every skill:// file is an orphan.
			desc:     "orphans without any skill",
			files:    map[string]string{"skill://guide/notes.md": "# Notes\n"},
			wantWarn: []string{"skill://guide/notes.md", "belong to no skill"},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			ctx, stderr := bufferLoggerCtx(t)
			resourcesMap := map[string]resources.Resource{}
			for _, uri := range tc.skills {
				resourcesMap[uri] = skillAt(t, ctx, uri, "A skill")
			}
			for uri, content := range tc.files {
				resourcesMap[uri] = textResource(t, ctx, uri, uri, content)
			}
			found, err := skills.Validate(ctx, resourcesMap)
			if err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
			if len(found) != tc.wantFound {
				t.Errorf("Validate() found %d skills, want %d", len(found), tc.wantFound)
			}

			out := stderr.String()
			for _, want := range tc.wantWarn {
				if !strings.Contains(out, want) {
					t.Errorf("log %q does not mention %q", out, want)
				}
			}
			for _, unwanted := range tc.noWarn {
				if strings.Contains(out, unwanted) {
					t.Errorf("log %q unexpectedly mentions %q", out, unwanted)
				}
			}
		})
	}
}

// TestValidateOrphanWarning checks that Validate warns about skill:// resources
// no SKILL.md sits above, and only those. Most often such a URI has a typo.
func TestValidateOrphanWarning(t *testing.T) {
	tcs := []struct {
		desc     string
		extra    map[string]string // resource URI to content, beside one skill
		wantWarn []string          // nil means no warning
	}{
		{
			desc: "typo and name-prefix sibling",
			extra: map[string]string{
				// A typo in the skill path: close to the skill, under none.
				"skill://analytcs-guide/references/joins.md": "# Joins\n",
				// A name prefix of the skill, not a path prefix.
				"skill://analytics-guide-v2/notes.md": "# Notes\n",
				// Not addressed by skill://, so it is never an orphan.
				"file://project-docs": "unrelated",
			},
			wantWarn: []string{"skill://analytcs-guide/references/joins.md", "skill://analytics-guide-v2/notes.md", "belong to no skill"},
		},
		{
			desc:  "every skill:// file is under a skill",
			extra: map[string]string{"skill://analytics-guide/references/queries.md": "# Common queries\n"},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			ctx, stderr := bufferLoggerCtx(t)
			resourcesMap := map[string]resources.Resource{
				"guide": skillAt(t, ctx, "skill://analytics-guide/SKILL.md", "A skill"),
			}
			for uri, content := range tc.extra {
				resourcesMap[uri] = textResource(t, ctx, uri, uri, content)
			}
			if _, err := skills.Validate(ctx, resourcesMap); err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}

			out := stderr.String()
			if tc.wantWarn == nil {
				if strings.Contains(out, "belong to no skill") {
					t.Errorf("unexpected orphan warning: %q", out)
				}
				return
			}
			for _, want := range tc.wantWarn {
				if !strings.Contains(out, want) {
					t.Errorf("warning %q does not mention %q", out, want)
				}
			}
			if strings.Contains(out, "project-docs") {
				t.Errorf("warning %q names a resource outside the skill:// scheme", out)
			}
		})
	}
}

// TestValidateOrphanWarningWithoutSkills covers a config with no SKILL.md at
// all, whose skill:// files are therefore all orphans. Validate must still warn.
func TestValidateOrphanWarningWithoutSkills(t *testing.T) {
	ctx, stderr := bufferLoggerCtx(t)
	resourcesMap := map[string]resources.Resource{
		"notes": textResource(t, ctx, "notes", "skill://guide/notes.md", "# Notes\n"),
	}
	found, err := skills.Validate(ctx, resourcesMap)
	if err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
	if len(found) != 0 {
		t.Errorf("Validate() found %d skills, want none", len(found))
	}
	if out := stderr.String(); !strings.Contains(out, "skill://guide/notes.md") {
		t.Errorf("log %q does not warn about skill://guide/notes.md", out)
	}
}

// TestSkillDocIdentity checks that each SKILL.md reports the name and
// description from its frontmatter, and that other resources keep their
// config values. The text resource sets these in Initialize, so the test
// doesn't call Validate.
func TestSkillDocIdentity(t *testing.T) {
	ctx := mustLoggerCtx(t)

	// The config names differ from the frontmatter names on purpose, so the
	// test can't pass by accident.
	resourcesMap := map[string]resources.Resource{
		"alpha": textResource(t, ctx, "SKILL.md", "skill://alpha-guide/SKILL.md", skillMD("alpha-guide", "Query the warehouse")),
		"notes": textResource(t, ctx, "notes", "skill://alpha-guide/references/notes.md", "# Notes\n"),
		"beta":  textResource(t, ctx, "beta", "skill://beta-guide/SKILL.md", skillMD("beta-guide", "Summarize the warehouse")),
		"docs":  textResource(t, ctx, "docs", "file://project-docs", "unrelated"),
	}

	want := map[string]struct{ name, description string }{
		"alpha": {"alpha-guide", "Query the warehouse"},
		"beta":  {"beta-guide", "Summarize the warehouse"},
		"notes": {"notes", ""},
		"docs":  {"docs", ""},
	}
	for key, w := range want {
		res := resourcesMap[key]
		if got := res.GetName(); got != w.name {
			t.Errorf("%s GetName() = %q, want %q", key, got, w.name)
		}
		if got := res.GetDescription(); got != w.description {
			t.Errorf("%s GetDescription() = %q, want %q", key, got, w.description)
		}
	}
}

func TestIsDoc(t *testing.T) {
	tcs := []struct {
		uri  string
		want bool
	}{
		{"skill://analytics-guide/SKILL.md", true},
		{"skill://org/team/analytics-guide/SKILL.md", true},
		{"skill://analytics-guide/references/queries.md", false},
		{"skill://analytics-guide/NOTSKILL.md", false},
		{"file://analytics-guide/SKILL.md", false},
	}
	for _, tc := range tcs {
		if got := skills.IsDoc(tc.uri); got != tc.want {
			t.Errorf("IsDoc(%q) = %v, want %v", tc.uri, got, tc.want)
		}
	}
}

func TestDocIdentity(t *testing.T) {
	tcs := []struct {
		desc     string
		content  string
		wantName string
		wantDesc string
		wantOK   bool
	}{
		{desc: "valid", content: skillMD("guide", "A guide"), wantName: "guide", wantDesc: "A guide", wantOK: true},
		{desc: "no frontmatter", content: "# Guide\n"},
		{desc: "unclosed frontmatter", content: "---\nname: guide\n"},
		{desc: "missing name", content: "---\ndescription: A guide\n---\n"},
		{desc: "missing description", content: "---\nname: guide\n---\n"},
		{desc: "non-string name", content: "---\nname: 7\ndescription: A guide\n---\n"},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			name, desc, ok := skills.DocIdentity(tc.content)
			if name != tc.wantName || desc != tc.wantDesc || ok != tc.wantOK {
				t.Errorf("DocIdentity() = (%q, %q, %v), want (%q, %q, %v)", name, desc, ok, tc.wantName, tc.wantDesc, tc.wantOK)
			}
		})
	}
}

// TestValidateDynamicSkill checks that startup validation treats a dynamic skill
// the way Discover does: the file count and size sums do not apply, and only
// SKILL.md is read.
func TestValidateDynamicSkill(t *testing.T) {
	ctx := mustLoggerCtx(t)
	resourcesMap := map[string]resources.Resource{
		"doc": dynamicSkillDoc(t, ctx, "doc", "skill://big-skill/SKILL.md",
			skillMD("big-skill", "More files than a static skill may carry")),
	}
	for i := range skills.MaxRefs + 1 {
		uri := fmt.Sprintf("skill://big-skill/refs/f%d.md", i)
		resourcesMap[fmt.Sprintf("ref%d", i)] = unreadResource{
			badResource: badResource{uri: uri},
			t:           t,
			size:        skills.MaxTotalSize,
		}
	}

	got, err := skills.Validate(ctx, resourcesMap)
	if err != nil {
		t.Fatalf("Validate() = %v, want nil: a dynamic skill has no refs to count", err)
	}
	if len(got) != 1 || got[0].Frontmatter["name"] != "big-skill" {
		t.Fatalf("got %+v, want the dynamic skill with its frontmatter", got)
	}
	// A dynamic SKILL.md is published under its frontmatter name as well.
	if got := resourcesMap["doc"].GetName(); got != "big-skill" {
		t.Errorf("GetName() = %q, want the frontmatter name %q", got, "big-skill")
	}
}

// TestValidateDynamicSkillStillChecksItsDoc checks that a dynamic skill with a
// bad SKILL.md still fails the load.
func TestValidateDynamicSkillStillChecksItsDoc(t *testing.T) {
	ctx := mustLoggerCtx(t)
	resourcesMap := map[string]resources.Resource{
		"doc": dynamicSkillDoc(t, ctx, "doc", "skill://live-report/SKILL.md", "# no frontmatter\n"),
	}
	_, err := skills.Validate(ctx, resourcesMap)
	if err == nil || !strings.Contains(err.Error(), "must open with YAML frontmatter") {
		t.Fatalf("Validate() = %v, want the frontmatter error", err)
	}
}

// TestWarnOnDocNameMismatch pins the signal an operator needs. A group lists its
// resources by config key, so a key that differs from the frontmatter name is
// hard to maintain.
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

	found, err := skills.Validate(ctx, resourcesMap)
	if err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
	if err := skills.WarnOnDocNameMismatch(ctx, found, resourcesMap); err != nil {
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

	found, err := skills.Validate(ctx, resourcesMap)
	if err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
	if err := skills.WarnOnDocNameMismatch(ctx, found, resourcesMap); err != nil {
		t.Fatalf("WarnOnDocNameMismatch() = %v, want nil", err)
	}
	if got := stderr.String(); strings.Contains(got, "Rename the resource") {
		t.Errorf("unexpected name-mismatch warning: %q", got)
	}
}
