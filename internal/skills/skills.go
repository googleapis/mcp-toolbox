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

// Package skills implements the io.modelcontextprotocol/skills extension
// (SEP-2640).
package skills

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"unicode/utf8"
)

const DynamicMarker = "dynamic"

// Per-skill limits fixed by SEP-2640, both inclusive.
const (
	MaxRefs      = 512
	MaxTotalSize = 16 << 20 // 16 MiB, summed over every ref's Size
)

// Frontmatter limits from the Agent Skills specification, which SEP-2640
// adopts by reference.
const (
	maxNameLen        = 64
	maxDescriptionLen = 1024
)

// ResourceRef is one file in a skill's manifest.
type ResourceRef struct {
	URI    string `json:"uri"`
	Digest string `json:"digest"` // "sha256:" followed by 64 lowercase hex characters
	Size   int64  `json:"size"`
}

// Manifest is a skill's complete file list, or the marker: "dynamic".
type Manifest struct {
	Refs    []ResourceRef
	Dynamic bool
}

// MarshalJSON emits the file list, or the string "dynamic".
func (m Manifest) MarshalJSON() ([]byte, error) {
	if m.Dynamic {
		return json.Marshal(DynamicMarker)
	}
	// Empty Refs means unpopulated, not a skill with no files.
	if len(m.Refs) == 0 {
		return json.Marshal([]ResourceRef{})
	}
	return json.Marshal(m.Refs)
}

func (m *Manifest) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return fmt.Errorf("invalid skill manifest: no value")
	}

	switch data[0] {
	case '"': // dynamic
		var marker string
		if err := json.Unmarshal(data, &marker); err != nil {
			return fmt.Errorf("invalid skill manifest: %w", err)
		}
		if marker != DynamicMarker {
			return fmt.Errorf("invalid skill manifest %q: the only permitted string is %q", truncate(marker), DynamicMarker)
		}
		m.Dynamic, m.Refs = true, nil
		return nil

	case '[': // resources list for skill
		var refs []ResourceRef
		if err := json.Unmarshal(data, &refs); err != nil {
			return fmt.Errorf("invalid skill manifest: %w", err)
		}
		// A file list must be complete, and every skill has a SKILL.md.
		if len(refs) == 0 {
			return fmt.Errorf("invalid skill manifest: a file list names at least the skill's SKILL.md")
		}
		m.Dynamic, m.Refs = false, refs
		return nil

	default:
		return fmt.Errorf("invalid skill manifest: must be an array of {uri, digest, size} or the string %q", DynamicMarker)
	}
}

func (m Manifest) Validate() error {
	if m.Dynamic {
		if len(m.Refs) > 0 {
			return fmt.Errorf("invalid skill manifest: a dynamic skill publishes no file list, got %d refs", len(m.Refs))
		}
		// A dynamic entry offers nothing to count, so the limits do not apply.
		return nil
	}
	if len(m.Refs) == 0 {
		return fmt.Errorf("invalid skill manifest: a static skill lists at least its SKILL.md, or sets Dynamic")
	}
	if len(m.Refs) > MaxRefs {
		return fmt.Errorf("invalid skill manifest: %d refs exceeds the limit of %d", len(m.Refs), MaxRefs)
	}

	var total int64
	seen := make(map[string]struct{}, len(m.Refs))
	for i, r := range m.Refs {
		if r.URI == "" {
			return fmt.Errorf("invalid skill manifest: ref %d has no uri", i)
		}
		if _, dup := seen[r.URI]; dup {
			return fmt.Errorf("invalid skill manifest: %q is listed more than once", r.URI)
		}
		seen[r.URI] = struct{}{}

		if !validDigest(r.Digest) {
			return fmt.Errorf("invalid skill manifest: %q has digest %q, want sha256: followed by 64 lowercase hex characters", r.URI, r.Digest)
		}
		if r.Size < 0 {
			return fmt.Errorf("invalid skill manifest: %q has size %d, want a byte length", r.URI, r.Size)
		}
		// Subtraction, not addition: a size near math.MaxInt64 would wrap a
		// running total negative and pass.
		if r.Size > MaxTotalSize-total {
			return fmt.Errorf("invalid skill manifest: total size exceeds the limit of %d bytes", MaxTotalSize)
		}
		total += r.Size
	}
	return nil
}

// Entry is one skill as skills/list and skills/get publish it.
type Entry struct {
	// URI addresses the skill's SKILL.md, not its root directory.
	URI string `json:"uri"`
	// Frontmatter is the SKILL.md YAML frontmatter verbatim
	Frontmatter map[string]any `json:"frontmatter"`
	Resources   Manifest       `json:"resources"`
}

// UnmarshalJSON unmarshals a single skill entry. Rejects an entry carrying no resources.
func (e *Entry) UnmarshalJSON(data []byte) error {
	var fields struct {
		URI         string          `json:"uri"`
		Frontmatter map[string]any  `json:"frontmatter"`
		Resources   json.RawMessage `json:"resources"`
	}
	if err := json.Unmarshal(data, &fields); err != nil {
		return fmt.Errorf("invalid skill entry: %w", err)
	}
	if fields.Resources == nil {
		return fmt.Errorf("invalid skill entry %q: resources is required", truncate(fields.URI))
	}
	if err := e.Resources.UnmarshalJSON(fields.Resources); err != nil {
		return err
	}
	e.URI, e.Frontmatter = fields.URI, fields.Frontmatter
	return nil
}

// Validate checks the rules relating a manifest to the skill's own identity.
func (e Entry) Validate() error {
	scheme, segs, err := uriSegments(e.URI)
	if err != nil {
		return fmt.Errorf("invalid skill entry %q: uri %w", truncate(e.URI), err)
	}
	if len(segs) < 2 || segs[len(segs)-1] != "SKILL.md" {
		return fmt.Errorf("invalid skill entry %q: uri must address the skill's SKILL.md", truncate(e.URI))
	}
	root := segs[:len(segs)-1]
	if err := e.validateFrontmatter(root[len(root)-1]); err != nil {
		return err
	}
	if err := e.Resources.Validate(); err != nil {
		return fmt.Errorf("skill %q: %w", truncate(e.URI), err)
	}
	if e.Resources.Dynamic {
		return nil
	}
	return e.validateRefs(scheme, root)
}

// validateFrontmatter checks the fields the Agent Skills specification requires,
// and their agreement with name.
func (e Entry) validateFrontmatter(name string) error {
	// Check the frontmatter
	fmName, err := requiredString(e.Frontmatter, "name")
	if err != nil {
		return fmt.Errorf("invalid skill entry %q: %w", truncate(e.URI), err)
	}
	// Check the uri segment
	if err := validSkillName(fmName); err != nil {
		return fmt.Errorf("invalid skill entry %q: frontmatter name %w", truncate(e.URI), err)
	}
	desc, err := requiredString(e.Frontmatter, "description")
	if err != nil {
		return fmt.Errorf("invalid skill entry %q: %w", truncate(e.URI), err)
	}
	if n := utf8.RuneCountInString(desc); n > maxDescriptionLen {
		return fmt.Errorf("invalid skill entry %q: frontmatter description is %d characters, want at most %d", truncate(e.URI), n, maxDescriptionLen)
	}
	if fmName != name {
		return fmt.Errorf("invalid skill entry %q: frontmatter name %q does not match the uri's final skill-path segment %q", truncate(e.URI), truncate(fmName), truncate(name))
	}
	return nil
}

// validateRefs checks that a file list names only files inside the skill, and
// that it names the skill's own SKILL.md.
func (e Entry) validateRefs(scheme string, root []string) error {
	var listsItself bool
	for _, r := range e.Resources.Refs {
		if !underSkill(r.URI, scheme, root) {
			return fmt.Errorf("invalid skill entry %q: %q is not a file within the skill", truncate(e.URI), truncate(r.URI))
		}
		if r.URI == e.URI {
			listsItself = true
		}
	}
	if !listsItself {
		return fmt.Errorf("invalid skill entry %q: resources must list the skill's own SKILL.md", truncate(e.URI))
	}
	return nil
}

// uriSegments splits a URI into a flat list of path segments.
func uriSegments(raw string) (string, []string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", nil, fmt.Errorf("is not a valid uri")
	}
	if u.Scheme == "" {
		return "", nil, fmt.Errorf("has no scheme")
	}
	if u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return "", nil, fmt.Errorf("must be a bare path, with no query, fragment, or userinfo")
	}
	segs := []string{u.Host}
	if rest := strings.TrimPrefix(u.Path, "/"); rest != "" {
		segs = append(segs, strings.Split(rest, "/")...)
	}
	for _, s := range segs {
		if s == "" || s == "." || s == ".." {
			return "", nil, fmt.Errorf("has an empty or relative path segment")
		}
	}
	return u.Scheme, segs, nil
}

// underSkill reports whether ref names a file inside the skill rooted at the
// given scheme and skill path.
func underSkill(ref, scheme string, root []string) bool {
	s, segs, err := uriSegments(ref)
	return err == nil && s == scheme && len(segs) > len(root) && slices.Equal(segs[:len(root)], root)
}

// validSkillName applies the Agent Skills naming rules
func validSkillName(s string) error {
	for _, c := range s {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
			return fmt.Errorf("%q may only contain lowercase letters, digits, and hyphens", truncate(s))
		}
	}
	if s == "" || len(s) > maxNameLen {
		return fmt.Errorf("is %d characters, want 1 to %d", len(s), maxNameLen)
	}
	if strings.HasPrefix(s, "-") || strings.HasSuffix(s, "-") {
		return fmt.Errorf("%q starts or ends with a hyphen", truncate(s))
	}
	if strings.Contains(s, "--") {
		return fmt.Errorf("%q contains consecutive hyphens", truncate(s))
	}
	return nil
}

// validDigest matches SEP-2640's sha256:{hex} form, {hex} being 64 lowercase
// hex characters.
func validDigest(s string) bool {
	hex, ok := strings.CutPrefix(s, "sha256:")
	if !ok || len(hex) != 64 {
		return false
	}
	for _, c := range hex {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// requiredString reads a frontmatter field required by the Agent Skills specification.
func requiredString(fm map[string]any, key string) (string, error) {
	v, ok := fm[key]
	if !ok {
		return "", fmt.Errorf("frontmatter has no %s", key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("frontmatter %s is %T, want a string", key, v)
	}
	if s == "" {
		return "", fmt.Errorf("frontmatter %s is empty", key)
	}
	return s, nil
}

// truncate bounds a value from the wire before it reaches an error message.
func truncate(s string) string {
	const max = 64
	if len(s) <= max {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
