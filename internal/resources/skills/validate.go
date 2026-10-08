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
	"strings"

	"github.com/googleapis/mcp-toolbox/internal/resources"
	"github.com/googleapis/mcp-toolbox/internal/util"
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

// Validate checks every skill in resourcesMap so a misconfigured skill fails
// the load instead of a later skills/list. It hashes nothing and reads only
// each skill's SKILL.md: membership comes from URIs, and sizes come from each
// resource's GetSize, which is a stat for a file resource.
//
// It applies the same rules as Discover apart from the digest format. It warns
// once when two skills share a frontmatter name, and once for skill:// resources
// that belong to no skill.
func Validate(ctx context.Context, resourcesMap map[string]resources.Resource) ([]Skill, error) {
	roots := skillRoots(resourcesMap)
	// Before the early return: a config whose only skill:// files are typos has
	// no skills, and is exactly the case the warning is for.
	members, orphans := skillMembers(resourcesMap, roots)
	if len(orphans) > 0 {
		logger, err := util.LoggerFromContext(ctx)
		if err != nil {
			return nil, fmt.Errorf("checking for orphaned skill files: %w", err)
		}
		logger.WarnContext(ctx, fmt.Sprintf("resources %s use the %s:// scheme but no %s is above them, so they belong to no skill; check the URI for a typo", strings.Join(orphans, ", "), resources.SkillScheme, resources.SkillFile))
	}
	if len(roots) == 0 {
		return nil, nil
	}

	found := make([]Skill, 0, len(roots))
	for _, root := range roots {
		s, err := validateSkill(ctx, root, members[root])
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
func validateSkill(ctx context.Context, root string, members []resources.Resource) (Skill, error) {
	skillURI := root + "/" + resources.SkillFile
	if doc, dynamic := skillDoc(members, skillURI); dynamic {
		// A dynamic skill is checked the way Discover builds its entry: the file
		// count does not apply, and only SKILL.md is read.
		if doc == nil {
			return Skill{}, fmt.Errorf("skill %q: no %s resource is registered", skillURI, resources.SkillFile)
		}
		return checkDoc(ctx, skillURI, doc, Manifest{Dynamic: true})
	}
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
	if err := e.Validate(false); err != nil {
		return Skill{}, err
	}
	return Skill{URI: skillURI, Frontmatter: frontmatter}, nil
}

// WarnOnDocNameMismatch reports the SKILL.md resources whose config key differs
// from the frontmatter name. Validate already publishes the SKILL.md under the
// frontmatter name. A group lists its resources by config key, so a key that
// differs from the skill is hard to maintain. Call it one time, at startup.
func WarnOnDocNameMismatch(ctx context.Context, found []Skill, resourcesMap map[string]resources.Resource) error {
	if len(found) == 0 {
		return nil
	}
	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		return fmt.Errorf("checking the names of the skill documents: %w", err)
	}

	keys := make(map[string]string, len(found))
	for key, res := range resourcesMap {
		uri := res.GetURI()
		if _, ok := resources.SkillRoot(uri); !ok {
			continue
		}
		// Two config keys can address one URI. Keep the lowest, so the warning
		// does not depend on map order.
		if prev, seen := keys[uri]; !seen || key < prev {
			keys[uri] = key
		}
	}

	for _, s := range found {
		key, ok := keys[s.URI]
		if !ok {
			continue
		}
		name, ok := s.Frontmatter["name"].(string)
		if !ok || name == key {
			continue
		}
		logger.WarnContext(ctx, fmt.Sprintf("resource %q is the %s of skill %q. Rename the resource to %q, so that a group lists it under the skill's name", key, resources.SkillFile, name, name))
	}
	return nil
}
