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

package skills

import (
	"slices"
	"sort"
	"strings"

	"github.com/googleapis/mcp-toolbox/internal/resources"
)

// Registry records which resources make up which skill.
//
// It holds structure, never content. Which URIs belong to a skill follows from
// the URIs alone, needs no I/O, and changes only when the resources map does —
// that is, at reload. Digests, sizes, and frontmatter come from file content and
// are recomputed per request instead: a digest cached for the process lifetime
// would hand a host back the same value it already failed to verify, leaving the
// refresh path SEP-2640 specifies with nothing to refresh to.
//
// A nil *Registry reports no skills. A caller that holds one before the config
// loads needs no nil check.
type Registry struct {
	members map[string][]resources.Resource
	uris    []string
	orphans []string
}

// NewRegistry groups resourcesMap into skills. A skill is any resource at
// skill://<skill-path>/SKILL.md, and everything sharing that prefix is one of
// its supporting files.
//
// Membership is one-to-many in both directions: per the SEP's completeness rule
// a file inside a nested skill also belongs to every enclosing skill, so it
// appears in more than one member list.
func NewRegistry(resourcesMap map[string]resources.Resource) *Registry {
	prefix := resources.SkillScheme + "://"

	isRoot := make(map[string]bool)
	for _, res := range resourcesMap {
		uri := res.GetURI()
		if !strings.HasPrefix(uri, prefix) {
			continue
		}
		if root, ok := strings.CutSuffix(uri, "/"+skillFile); ok {
			isRoot[root] = true
		}
	}

	// Segments per root, so membership can apply the same test the manifest
	// validation applies. A root that is not a valid URI owns no files.
	rootSegs := make(map[string][]string, len(isRoot))
	for root := range isRoot {
		if _, segs, err := uriSegments(root); err == nil {
			rootSegs[root] = segs
		}
	}

	members := make(map[string][]resources.Resource, len(isRoot))
	var orphans []string
	for _, res := range resourcesMap {
		uri := res.GetURI()
		if !strings.HasPrefix(uri, prefix) {
			continue
		}
		matched := false
		// Walk the URI's ancestors rather than every root, so the scan costs
		// path depth instead of the number of skills.
		for i := strings.LastIndex(uri, "/"); i > 0; i = strings.LastIndex(uri[:i], "/") {
			// underSkill is the test Entry.Validate applies to every ref. A
			// looser rule here admits a member the validation then rejects,
			// which fails startup for the whole config.
			root := uri[:i]
			if segs, ok := rootSegs[root]; ok && underSkill(uri, resources.SkillScheme, segs) {
				skillURI := root + "/" + skillFile
				members[skillURI] = append(members[skillURI], res)
				matched = true
			}
		}
		// Skip SKILL.md files: each one defines a skill rather than belonging to one.
		if !matched && !strings.HasSuffix(uri, "/"+skillFile) {
			orphans = append(orphans, uri)
		}
	}
	sort.Strings(orphans)

	// Order by skill root, not by SKILL.md URI: "guide-v2" sorts before "guide"
	// once "/SKILL.md" is appended, because "-" precedes "/".
	roots := make([]string, 0, len(isRoot))
	for root := range isRoot {
		roots = append(roots, root)
	}
	sort.Strings(roots)

	uris := make([]string, 0, len(roots))
	for _, root := range roots {
		uris = append(uris, root+"/"+skillFile)
	}
	for _, m := range members {
		sort.Slice(m, func(i, j int) bool { return m[i].GetURI() < m[j].GetURI() })
	}

	return &Registry{members: members, uris: uris, orphans: orphans}
}

// URIs returns a copy of every skill's SKILL.md URI, sorted.
func (r *Registry) URIs() []string {
	if r == nil {
		return nil
	}
	return slices.Clone(r.uris)
}

// Members returns a copy of one skill's resources, sorted by URI. The second
// result reports whether the skill is registered.
func (r *Registry) Members(skillURI string) ([]resources.Resource, bool) {
	if r == nil {
		return nil, false
	}
	m, ok := r.members[skillURI]
	return slices.Clone(m), ok
}

// Len reports how many skills are registered.
func (r *Registry) Len() int {
	if r == nil {
		return 0
	}
	return len(r.uris)
}

// Orphans returns a copy of every skill:// URI that belongs to no skill, sorted.
func (r *Registry) Orphans() []string {
	if r == nil {
		return nil
	}
	return slices.Clone(r.orphans)
}
