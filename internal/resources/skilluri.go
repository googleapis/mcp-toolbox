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

package resources

import (
	"fmt"
	"net/url"
	"strings"
)

// skillFile is the document at the root of every Agent Skill.
const skillFile = "SKILL.md"

// maxSkillNameLen is the Agent Skills limit on a skill name.
const maxSkillNameLen = 64

// ValidateSkillURI applies the SEP-2640 rules to a skill:// URI: a bare path
// with no empty or relative segments and, for a SKILL.md, a valid skill name
// as its parent segment. It returns nil for any other scheme.
func ValidateSkillURI(uri string) error {
	u, err := url.Parse(uri)
	if err != nil {
		return fmt.Errorf("is not a valid uri")
	}
	if !strings.EqualFold(u.Scheme, SkillScheme) {
		return nil
	}
	_, segs, err := SkillURISegments(uri)
	if err != nil {
		return err
	}
	if n := len(segs); n >= 2 && segs[n-1] == skillFile {
		if err := ValidSkillName(segs[n-2]); err != nil {
			return fmt.Errorf("skill name %w", err)
		}
	}
	return nil
}

// SkillURISegments splits a URI into its scheme and a flat list of path
// segments, the host first.
func SkillURISegments(raw string) (string, []string, error) {
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

// ValidSkillName applies the Agent Skills naming rules.
func ValidSkillName(s string) error {
	for _, c := range s {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
			return fmt.Errorf("%q may only contain lowercase letters, digits, and hyphens", s)
		}
	}
	if s == "" || len(s) > maxSkillNameLen {
		return fmt.Errorf("is %d characters, want 1 to %d", len(s), maxSkillNameLen)
	}
	if strings.HasPrefix(s, "-") || strings.HasSuffix(s, "-") {
		return fmt.Errorf("%q starts or ends with a hyphen", s)
	}
	if strings.Contains(s, "--") {
		return fmt.Errorf("%q contains consecutive hyphens", s)
	}
	return nil
}
