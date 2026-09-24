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

// Discover builds one Entry per skill. A skill can have 1 or more supporting files.
func Discover(ctx context.Context, reg *Registry) ([]Entry, error) {
	if reg.Len() == 0 {
		return nil, nil
	}

	entries := make([]Entry, 0, reg.Len())
	for _, skillURI := range reg.URIs() {
		var e Entry
		var err error
		if reg.IsDynamic(skillURI) {
			// A dynamic skill publishes no digests, so Discover does not read its
			// supporting files. It reads only the SKILL.md, for the frontmatter
			// every entry carries.
			doc, ok := reg.Doc(skillURI)
			if !ok {
				return nil, fmt.Errorf("skill %q: no %s resource is registered", skillURI, resources.SkillFile)
			}
			e, err = buildDynamicEntry(ctx, skillURI, doc)
		} else {
			members, _ := reg.Members(skillURI)
			e, err = buildEntry(ctx, skillURI, members)
		}
		if err != nil {
			return nil, err
		}
		if err := e.Validate(); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// buildDynamicEntry assembles the entry for a skill that publishes the
// "dynamic" marker in place of a file list.
//
// The file count does not apply: SEP-2640 counts it over the entries of a
// manifest, and a dynamic skill has none. The size limit still bounds the one
// file this reads, because Discover runs on every request.
func buildDynamicEntry(ctx context.Context, skillURI string, doc resources.Resource) (Entry, error) {
	content, err := readBounded(ctx, doc, MaxTotalSize)
	if err != nil {
		return Entry{}, fmt.Errorf("skill %q: %w", skillURI, err)
	}
	frontmatter, err := parseFrontmatter(content)
	if err != nil {
		return Entry{}, fmt.Errorf("skill %q: %w", skillURI, err)
	}
	return Entry{URI: skillURI, Frontmatter: frontmatter, Resources: Manifest{Dynamic: true}}, nil
}

// buildEntry hashes every file of one skill and assembles its entry.
func buildEntry(ctx context.Context, skillURI string, members []resources.Resource) (Entry, error) {
	if len(members) > MaxRefs {
		return Entry{}, fmt.Errorf("skill %q: %d files exceeds the limit of %d", skillURI, len(members), MaxRefs)
	}

	refs := make([]ResourceRef, 0, len(members))
	var frontmatter map[string]any
	// Manifest.Validate enforces the same limit, but only once every file is in
	// memory. We sum as we read: this bounds what each skill loads.
	var total int64
	for _, res := range members {
		content, err := readBounded(ctx, res, MaxTotalSize-total)
		if err != nil {
			return Entry{}, fmt.Errorf("skill %q: %w", skillURI, err)
		}
		size := int64(len(content))
		total += size
		sum := sha256.Sum256([]byte(content))
		refs = append(refs, ResourceRef{
			URI:    res.GetURI(),
			Digest: "sha256:" + hex.EncodeToString(sum[:]),
			Size:   size,
		})
		if res.GetURI() == skillURI {
			frontmatter, err = parseFrontmatter(content)
			if err != nil {
				return Entry{}, fmt.Errorf("skill %q: %w", skillURI, err)
			}
		}
	}

	return Entry{URI: skillURI, Frontmatter: frontmatter, Resources: Manifest{Refs: refs}}, nil
}

// readBounded reads one resource, and rejects content larger than remaining
// bytes. Callers pass the budget they have left, not the limit, so a huge size
// hint cannot wrap a running total negative.
func readBounded(ctx context.Context, res resources.Resource, remaining int64) (string, error) {
	if sz := res.GetSize(); sz != nil && *sz > remaining {
		return "", fmt.Errorf("total size exceeds the limit of %d bytes", MaxTotalSize)
	}
	content, err := readString(ctx, res)
	if err != nil {
		return "", err
	}
	// GetSize above is only a hint. This check is authoritative.
	if int64(len(content)) > remaining {
		return "", fmt.Errorf("total size exceeds the limit of %d bytes", MaxTotalSize)
	}
	return content, nil
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
		return nil, fmt.Errorf("%s must open with YAML frontmatter delimited by ---", resources.SkillFile)
	}
	body, ok := cutAtDelimiter(rest)
	if !ok {
		return nil, fmt.Errorf("%s frontmatter is not closed by --- on a line of its own", resources.SkillFile)
	}

	fm := map[string]any{}
	if err := yaml.Unmarshal([]byte(body), &fm); err != nil {
		return nil, fmt.Errorf("unable to parse %s frontmatter: %w", resources.SkillFile, err)
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

// WarnOnDuplicateNames reports the skills that share a frontmatter name. Call
// it one time, at startup. Discover also runs one time for each request.
func WarnOnDuplicateNames(ctx context.Context, entries []Entry) error {
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
