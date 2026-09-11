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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/resources"
	"github.com/googleapis/mcp-toolbox/internal/util"
)

const skillFile = "SKILL.md"

// Discover builds one Entry per skill. A skill can have 1 or more supporting files.
//
// The second return value maps a URI to a resource serving the bytes read here,
// for every file belonging to a skill. Callers are expected to serve those in
// place of the originals: the digests published alongside them are computed from
// these bytes, and a host rejects content that does not match.
func Discover(ctx context.Context, resourcesMap map[string]resources.Resource) ([]Entry, map[string]resources.Resource, error) {
	roots := skillRoots(resourcesMap)
	if len(roots) == 0 {
		return nil, nil, nil
	}
	members := skillMembers(resourcesMap, roots)

	entries := make([]Entry, 0, len(roots))
	snapshots := make(map[string]resources.Resource)
	for _, root := range roots {
		e, snaps, err := buildEntry(ctx, root, members[root])
		if err != nil {
			return nil, nil, err
		}
		if err := e.Validate(); err != nil {
			return nil, nil, err
		}
		entries = append(entries, e)
		// A file nested inside two skills is read once per skill; both reads
		// yield the same bytes, so the later write is a no-op.
		for _, s := range snaps {
			snapshots[s.GetURI()] = s
		}
	}

	if err := warnOnDuplicateNames(ctx, entries); err != nil {
		return nil, nil, err
	}
	return entries, snapshots, nil
}

// A list of root dir of every skill in the map, sorted
func skillRoots(resourcesMap map[string]resources.Resource) []string {
	var roots []string
	for _, res := range resourcesMap {
		uri := res.GetURI()
		if !strings.HasPrefix(uri, resources.SkillScheme+"://") {
			continue
		}
		if root, ok := strings.CutSuffix(uri, "/"+skillFile); ok {
			roots = append(roots, root)
		}
	}
	sort.Strings(roots)
	return roots
}

// skillMembers groups every resource under the skills that contain it, sorted by
// URI.
func skillMembers(resourcesMap map[string]resources.Resource, roots []string) map[string][]resources.Resource {
	isRoot := make(map[string]bool, len(roots))
	for _, root := range roots {
		isRoot[root] = true
	}

	members := make(map[string][]resources.Resource, len(roots))
	for _, res := range resourcesMap {
		uri := res.GetURI()
		if !strings.HasPrefix(uri, resources.SkillScheme+"://") {
			continue
		}
		// Walk the URI's ancestors rather than every root, so the scan costs
		// path depth instead of the number of skills.
		for i := strings.LastIndex(uri, "/"); i > 0; i = strings.LastIndex(uri[:i], "/") {
			if isRoot[uri[:i]] {
				members[uri[:i]] = append(members[uri[:i]], res)
			}
		}
	}
	for _, m := range members {
		sort.Slice(m, func(i, j int) bool { return m[i].GetURI() < m[j].GetURI() })
	}
	return members
}

// buildEntry hashes every file under root and assembles its entry, returning a
// snapshot of each file alongside it.
func buildEntry(ctx context.Context, root string, members []resources.Resource) (Entry, []snapshot, error) {
	skillURI := root + "/" + skillFile
	if len(members) > MaxRefs {
		return Entry{}, nil, fmt.Errorf("skill %q: %d files exceeds the limit of %d", skillURI, len(members), MaxRefs)
	}

	refs := make([]ResourceRef, 0, len(members))
	snaps := make([]snapshot, 0, len(members))
	var frontmatter map[string]any
	var total int64
	for _, res := range members {
		content, err := readString(ctx, res)
		if err != nil {
			return Entry{}, nil, fmt.Errorf("skill %q: %w", skillURI, err)
		}
		size := int64(len(content))
		// Manifest.Validate enforces the same limit, but only once every file is
		// in memory. Checking as we read caps what a single skill can allocate.
		total += size
		if total > MaxTotalSize {
			return Entry{}, nil, fmt.Errorf("skill %q: total size exceeds the limit of %d bytes", skillURI, MaxTotalSize)
		}
		sum := sha256.Sum256([]byte(content))
		refs = append(refs, ResourceRef{
			URI:    res.GetURI(),
			Digest: "sha256:" + hex.EncodeToString(sum[:]),
			Size:   size,
		})
		snaps = append(snaps, snapshot{Resource: res, content: content, size: size})
		if res.GetURI() == skillURI {
			frontmatter, err = parseFrontmatter(content)
			if err != nil {
				return Entry{}, nil, fmt.Errorf("skill %q: %w", skillURI, err)
			}
		}
	}

	return Entry{URI: skillURI, Frontmatter: frontmatter, Resources: Manifest{Refs: refs}}, snaps, nil
}

func readString(ctx context.Context, res resources.Resource) (string, error) {
	got, err := res.Read(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("unable to read %q: %w", res.GetURI(), err)
	}
	content, ok := got.(string)
	if !ok {
		return "", fmt.Errorf("%q returned %T, want text content", res.GetURI(), got)
	}
	return content, nil
}

// Extracts the leading YAML frontmatter of a SKILL.md.
func parseFrontmatter(content string) (map[string]any, error) {
	// Normalise invisible bytes for windows; the digest covers the bytes as read.
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.TrimPrefix(content, "\ufeff")

	opening, rest, ok := strings.Cut(content, "\n")
	if !ok || strings.TrimRight(opening, " \t") != "---" {
		return nil, fmt.Errorf("%s must open with YAML frontmatter delimited by ---", skillFile)
	}
	body, ok := cutAtDelimiter(rest)
	if !ok {
		return nil, fmt.Errorf("%s frontmatter is not closed by --- on a line of its own", skillFile)
	}

	fm := map[string]any{}
	if err := yaml.Unmarshal([]byte(body), &fm); err != nil {
		return nil, fmt.Errorf("unable to parse %s frontmatter: %w", skillFile, err)
	}
	return fm, nil
}

// Returns everything before the first line consisting only of
// ---, reporting whether such a line exists.
func cutAtDelimiter(rest string) (string, bool) {
	for offset := 0; ; {
		line, tail, more := strings.Cut(rest[offset:], "\n")
		if strings.TrimRight(line, " \t") == "---" {
			return rest[:offset], true
		}
		if !more {
			return "", false
		}
		offset = len(rest) - len(tail)
	}
}

// warnOnDuplicateNames reports skills sharing a frontmatter name.
func warnOnDuplicateNames(ctx context.Context, entries []Entry) error {
	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		return fmt.Errorf("checking for duplicate skill names: %w", err)
	}
	byName := map[string][]string{}
	for _, e := range entries {
		if name, ok := e.Frontmatter["name"].(string); ok {
			byName[name] = append(byName[name], e.URI)
		}
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if uris := byName[name]; len(uris) > 1 {
			logger.WarnContext(ctx, fmt.Sprintf("skills %s share the name %q; hosts must disambiguate them", strings.Join(uris, ", "), name))
		}
	}
	return nil
}
