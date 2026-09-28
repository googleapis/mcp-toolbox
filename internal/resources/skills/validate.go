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
	"context"
	"fmt"

	"github.com/googleapis/mcp-toolbox/internal/resources"
)

// Skill is what startup validation learns about one skill. It carries no
// digests or sizes: skills/list and skills/get compute those per request
// through Discover, so nothing content-derived outlives the request.
type Skill struct {
	// URI addresses the skill's SKILL.md.
	URI string
	// Frontmatter is the SKILL.md YAML frontmatter verbatim.
	Frontmatter map[string]any
}

// Validate checks every skill in reg so a misconfigured skill fails
// the load instead of a later skills/list. It hashes nothing and reads only
// each skill's SKILL.md: membership comes from URIs, and sizes come from each
// resource's GetSize, which is a stat for a file resource.
//
// It applies the same rules as Discover apart from the digest format, and warns
// once when two skills share a frontmatter name.
func Validate(ctx context.Context, reg *Registry) ([]Skill, error) {
	if reg.Len() == 0 {
		return nil, nil
	}

	found := make([]Skill, 0, reg.Len())
	for _, skillURI := range reg.URIs() {
		var s Skill
		var err error
		if reg.IsDynamic(skillURI) {
			doc, ok := reg.Doc(skillURI)
			if !ok {
				return nil, fmt.Errorf("skill %q: no %s resource is registered", skillURI, resources.SkillFile)
			}
			s, err = validateDynamicSkill(ctx, skillURI, doc)
		} else {
			members, _ := reg.Members(skillURI)
			s, err = validateSkill(ctx, skillURI, members)
		}
		if err != nil {
			return nil, err
		}
		found = append(found, s)
	}

	if err := warnOnDuplicateNames(ctx, found); err != nil {
		return nil, err
	}
	return found, nil
}

// validateSkill checks one skill's limits and structure, then reads and checks
// its SKILL.md.
func validateSkill(ctx context.Context, skillURI string, members []resources.Resource) (Skill, error) {
	if len(members) > MaxRefs {
		return Skill{}, fmt.Errorf("skill %q: %d files exceeds the limit of %d", skillURI, len(members), MaxRefs)
	}

	refs := make([]ResourceRef, 0, len(members))
	var doc resources.Resource
	var total int64
	for _, res := range members {
		// A resource that reports no size is not counted here. Discover still
		// enforces the limit on the bytes it reads at request time.
		var size int64
		if sz := res.GetSize(); sz != nil {
			size = *sz
		}
		// Subtraction, not addition: a huge size would wrap the total negative.
		if size > MaxTotalSize-total {
			return Skill{}, fmt.Errorf("skill %q: total size exceeds the limit of %d bytes", skillURI, MaxTotalSize)
		}
		total += size
		refs = append(refs, ResourceRef{URI: res.GetURI(), Size: size})
		if res.GetURI() == skillURI {
			doc = res
		}
	}
	if doc == nil {
		return Skill{}, fmt.Errorf("skill %q: %s is not among the skill's resources", skillURI, resources.SkillFile)
	}

	return checkDoc(ctx, skillURI, doc, Manifest{Refs: refs})
}

// validateDynamicSkill checks a dynamic skill the way Discover builds its
// entry: the file count does not apply, and only SKILL.md is read, bounded by
// the size limit.
func validateDynamicSkill(ctx context.Context, skillURI string, doc resources.Resource) (Skill, error) {
	return checkDoc(ctx, skillURI, doc, Manifest{Dynamic: true})
}

// checkDoc reads and parses SKILL.md, then applies every entry rule except the
// digest format.
func checkDoc(ctx context.Context, skillURI string, doc resources.Resource, m Manifest) (Skill, error) {
	content, err := readBounded(ctx, doc, MaxTotalSize)
	if err != nil {
		return Skill{}, fmt.Errorf("skill %q: %w", skillURI, err)
	}
	frontmatter, err := parseFrontmatter(content)
	if err != nil {
		return Skill{}, fmt.Errorf("skill %q: %w", skillURI, err)
	}

	e := Entry{URI: skillURI, Frontmatter: frontmatter, Resources: m}
	if err := e.validate(false); err != nil {
		return Skill{}, err
	}
	return Skill{URI: skillURI, Frontmatter: frontmatter}, nil
}
