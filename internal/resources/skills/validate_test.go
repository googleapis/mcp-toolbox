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
	"context"
	"fmt"
	"path"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/resources"
	"github.com/googleapis/mcp-toolbox/internal/resources/skills"
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

// TestValidateDuplicateNameWarning checks that Validate warns when two skills
// share a frontmatter name, and only then. Entry validation ties the name to
// the final skill-path segment, so a duplicate can only arise from differing
// parent paths.
func TestValidateDuplicateNameWarning(t *testing.T) {
	tcs := []struct {
		desc     string
		uris     []string
		wantWarn []string // nil means no warning
	}{
		{
			desc:     "same name under different parents",
			uris:     []string{"skill://acme/guide/SKILL.md", "skill://other/guide/SKILL.md"},
			wantWarn: []string{"skill://acme/guide/SKILL.md", "skill://other/guide/SKILL.md", `share the name \"guide\"`},
		},
		{
			desc: "distinct names",
			uris: []string{"skill://acme/guide/SKILL.md", "skill://acme/other/SKILL.md"},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			ctx, stderr := bufferLoggerCtx(t)
			resourcesMap := map[string]resources.Resource{}
			for _, uri := range tc.uris {
				resourcesMap[uri] = skillAt(t, ctx, uri, "A skill")
			}
			if _, err := skills.Validate(ctx, resourcesMap); err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}

			out := stderr.String()
			if tc.wantWarn == nil {
				if strings.Contains(out, "share the name") {
					t.Errorf("unexpected duplicate-name warning: %q", out)
				}
				return
			}
			for _, want := range tc.wantWarn {
				if !strings.Contains(out, want) {
					t.Errorf("warning %q does not mention %q", out, want)
				}
			}
		})
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
