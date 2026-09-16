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
	"slices"
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/resources"
	"github.com/googleapis/mcp-toolbox/internal/skills"
)

// memberURIs reads back one skill's member list as URIs, so a test can compare
// against a literal rather than against resource values.
func memberURIs(t *testing.T, reg *skills.Registry, skillURI string) []string {
	t.Helper()
	members, ok := reg.Members(skillURI)
	if !ok {
		t.Fatalf("Members(%q) reported the skill is not registered", skillURI)
	}
	uris := make([]string, 0, len(members))
	for _, m := range members {
		uris = append(uris, m.GetURI())
	}
	return uris
}

func TestNewRegistry(t *testing.T) {
	ctx := mustLoggerCtx(t)

	resourcesMap := map[string]resources.Resource{
		"guide": textResource(t, ctx, "guide", "skill://analytics-guide/SKILL.md",
			skillMD("analytics-guide", "Query the warehouse")),
		"queries": textResource(t, ctx, "queries",
			"skill://analytics-guide/references/queries.md", "# Common queries\n"),
		// Shares a name prefix with the skill above but not a path prefix, so it
		// is its own skill rather than a file of that one.
		"decoy": textResource(t, ctx, "decoy", "skill://analytics-guide-v2/SKILL.md",
			skillMD("analytics-guide-v2", "A different skill")),
		// Not addressed by skill://, so no skill owns it.
		"docs": textResource(t, ctx, "docs", "file://project-docs", "unrelated"),
	}

	reg := skills.NewRegistry(resourcesMap)

	if got := reg.Len(); got != 2 {
		t.Fatalf("Len() = %d, want 2", got)
	}

	// Ordered by skill root: "analytics-guide" precedes "analytics-guide-v2",
	// which sorting the SKILL.md URIs would reverse.
	wantURIs := []string{
		"skill://analytics-guide/SKILL.md",
		"skill://analytics-guide-v2/SKILL.md",
	}
	if got := reg.URIs(); !slices.Equal(got, wantURIs) {
		t.Errorf("URIs() = %v, want %v", got, wantURIs)
	}

	wantMembers := []string{
		"skill://analytics-guide/SKILL.md",
		"skill://analytics-guide/references/queries.md",
	}
	if got := memberURIs(t, reg, "skill://analytics-guide/SKILL.md"); !slices.Equal(got, wantMembers) {
		t.Errorf("Members() = %v, want %v", got, wantMembers)
	}

	wantDecoy := []string{"skill://analytics-guide-v2/SKILL.md"}
	if got := memberURIs(t, reg, "skill://analytics-guide-v2/SKILL.md"); !slices.Equal(got, wantDecoy) {
		t.Errorf("Members() = %v, want %v — the other skill's files must not leak in", got, wantDecoy)
	}
}

// TestNewRegistryNestedSkill pins the SEP's completeness rule: a file inside a
// nested skill belongs to every enclosing skill, so it appears in more than one
// member list.
func TestNewRegistryNestedSkill(t *testing.T) {
	ctx := mustLoggerCtx(t)

	resourcesMap := map[string]resources.Resource{
		"parent": textResource(t, ctx, "parent", "skill://acme/billing/SKILL.md",
			skillMD("billing", "Billing workflows")),
		"child": textResource(t, ctx, "child", "skill://acme/billing/refunds/SKILL.md",
			skillMD("refunds", "Refund workflows")),
		"note": textResource(t, ctx, "note",
			"skill://acme/billing/refunds/notes.md", "# Refund notes\n"),
	}

	reg := skills.NewRegistry(resourcesMap)

	if got := reg.Len(); got != 2 {
		t.Fatalf("Len() = %d, want 2", got)
	}

	wantParent := []string{
		"skill://acme/billing/SKILL.md",
		"skill://acme/billing/refunds/SKILL.md",
		"skill://acme/billing/refunds/notes.md",
	}
	if got := memberURIs(t, reg, "skill://acme/billing/SKILL.md"); !slices.Equal(got, wantParent) {
		t.Errorf("Members() = %v, want %v — a nested skill's files stay listed in the enclosing skill", got, wantParent)
	}

	wantChild := []string{
		"skill://acme/billing/refunds/SKILL.md",
		"skill://acme/billing/refunds/notes.md",
	}
	if got := memberURIs(t, reg, "skill://acme/billing/refunds/SKILL.md"); !slices.Equal(got, wantChild) {
		t.Errorf("Members() = %v, want %v", got, wantChild)
	}
}

func TestNewRegistryNoSkills(t *testing.T) {
	ctx := mustLoggerCtx(t)

	reg := skills.NewRegistry(map[string]resources.Resource{
		"docs": textResource(t, ctx, "docs", "file://project-docs", "unrelated"),
	})

	if got := reg.Len(); got != 0 {
		t.Errorf("Len() = %d, want 0", got)
	}
	if got := reg.URIs(); len(got) != 0 {
		t.Errorf("URIs() = %v, want empty", got)
	}
	if _, ok := reg.Members("skill://analytics-guide/SKILL.md"); ok {
		t.Error("Members() reported a skill, want none registered")
	}
}

// TestNilRegistry pins the nil receiver as usable. PR9 and PR11 each hold this
// pointer, and one the caller never built must report no skills, not panic.
func TestNilRegistry(t *testing.T) {
	var reg *skills.Registry

	if got := reg.Len(); got != 0 {
		t.Errorf("Len() = %d, want 0", got)
	}
	if got := reg.URIs(); len(got) != 0 {
		t.Errorf("URIs() = %v, want empty", got)
	}
	if members, ok := reg.Members("skill://analytics-guide/SKILL.md"); ok {
		t.Errorf("Members() = %v, true, want false", members)
	}
}

// TestRegistryReturnsCopies pins the accessor results as safe to modify. Several
// callers share one registry, so a write to one result must not change what the
// next caller reads.
func TestRegistryReturnsCopies(t *testing.T) {
	ctx := mustLoggerCtx(t)

	reg := skills.NewRegistry(map[string]resources.Resource{
		"guide": textResource(t, ctx, "guide", "skill://analytics-guide/SKILL.md",
			skillMD("analytics-guide", "Query the warehouse")),
		"queries": textResource(t, ctx, "queries",
			"skill://analytics-guide/references/queries.md", "# Common queries\n"),
	})

	wantURIs := slices.Clone(reg.URIs())
	reg.URIs()[0] = "skill://tampered/SKILL.md"
	if got := reg.URIs(); !slices.Equal(got, wantURIs) {
		t.Errorf("URIs() = %v after a caller wrote to an earlier result, want %v", got, wantURIs)
	}

	wantMembers := memberURIs(t, reg, "skill://analytics-guide/SKILL.md")
	members, _ := reg.Members("skill://analytics-guide/SKILL.md")
	members[0] = members[len(members)-1]
	if got := memberURIs(t, reg, "skill://analytics-guide/SKILL.md"); !slices.Equal(got, wantMembers) {
		t.Errorf("Members() = %v after a caller wrote to an earlier result, want %v", got, wantMembers)
	}
}

// TestRegistryMembersUnknownURI covers the lookup a caller makes with a URI from
// the wire, which names no registered skill.
func TestRegistryMembersUnknownURI(t *testing.T) {
	ctx := mustLoggerCtx(t)

	reg := skills.NewRegistry(map[string]resources.Resource{
		"guide": textResource(t, ctx, "guide", "skill://analytics-guide/SKILL.md",
			skillMD("analytics-guide", "Query the warehouse")),
	})

	tcs := []struct {
		desc string
		uri  string
	}{
		{desc: "no such skill", uri: "skill://nope/SKILL.md"},
		{desc: "a skill root rather than its SKILL.md", uri: "skill://analytics-guide"},
		{desc: "a supporting file rather than a skill", uri: "skill://analytics-guide/references/queries.md"},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			if members, ok := reg.Members(tc.uri); ok {
				t.Errorf("Members(%q) = %v, true, want false", tc.uri, members)
			}
		})
	}
}
