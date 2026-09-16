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
	"fmt"

	"github.com/googleapis/mcp-toolbox/internal/resources"
)

// docMimeType is what SEP-2640 fixes for a SKILL.md, whichever resource type
// backs it.
const docMimeType = "text/markdown"

// skillDoc presents a SKILL.md under the name and description its frontmatter
// declares. Otherwise a client sees a file resource named "SKILL.md" which
// does not identify the skill.
type skillDoc struct {
	resources.Resource
	name        string
	description string
}

func (s skillDoc) GetName() string        { return s.name }
func (s skillDoc) GetDescription() string { return s.description }
func (s skillDoc) GetMimeType() string    { return docMimeType }

// WithDocMetadata returns a replacement SKILL.md per each entry, keyed
// by URI. Omits any resource that is not a skill's SKILL.md.
func WithDocMetadata(entries []Entry, resourcesMap map[string]resources.Resource) (map[string]resources.Resource, error) {
	if len(entries) == 0 {
		return nil, nil
	}

	byURI := make(map[string]Entry, len(entries))
	for _, e := range entries {
		byURI[e.URI] = e
	}

	docs := make(map[string]resources.Resource, len(entries))
	for _, res := range resourcesMap {
		e, ok := byURI[res.GetURI()]
		if !ok {
			continue
		}
		name, err := requiredString(e.Frontmatter, "name")
		if err != nil {
			return nil, fmt.Errorf("invalid skill entry %q: %w", truncate(e.URI), err)
		}
		desc, err := requiredString(e.Frontmatter, "description")
		if err != nil {
			return nil, fmt.Errorf("invalid skill entry %q: %w", truncate(e.URI), err)
		}
		docs[res.GetURI()] = skillDoc{Resource: res, name: name, description: desc}
	}
	return docs, nil
}
