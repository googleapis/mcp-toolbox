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
type Registry struct {
	members map[string][]resources.Resource
	uris    []string
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
	if len(isRoot) == 0 {
		return &Registry{}
	}

	members := make(map[string][]resources.Resource, len(isRoot))
	for _, res := range resourcesMap {
		uri := res.GetURI()
		if !strings.HasPrefix(uri, prefix) {
			continue
		}
		// Walk the URI's ancestors rather than every root, so the scan costs
		// path depth instead of the number of skills.
		for i := strings.LastIndex(uri, "/"); i > 0; i = strings.LastIndex(uri[:i], "/") {
			if root := uri[:i]; isRoot[root] {
				skillURI := root + "/" + skillFile
				members[skillURI] = append(members[skillURI], res)
			}
		}
	}

	uris := make([]string, 0, len(members))
	for skillURI, m := range members {
		uris = append(uris, skillURI)
		sort.Slice(m, func(i, j int) bool { return m[i].GetURI() < m[j].GetURI() })
	}
	sort.Strings(uris)

	return &Registry{members: members, uris: uris}
}

// URIs returns every skill's SKILL.md URI, sorted.
func (r *Registry) URIs() []string {
	return r.uris
}

// Members returns the resources making up one skill, sorted by URI, reporting
// whether the skill is registered.
func (r *Registry) Members(skillURI string) ([]resources.Resource, bool) {
	m, ok := r.members[skillURI]
	return m, ok
}

// Len reports how many skills are registered.
func (r *Registry) Len() int {
	return len(r.uris)
}
