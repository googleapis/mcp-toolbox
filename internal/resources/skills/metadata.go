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
	"github.com/googleapis/mcp-toolbox/internal/resources"
)

// docMimeType is what SEP-2640 fixes for a SKILL.md, whichever resource type
// backs it.
const docMimeType = "text/markdown"

// skillDoc presents a SKILL.md under the identity its frontmatter declares,
// rather than the one its backing resource was configured with — a file
// resource would otherwise be listed as "SKILL.md", which tells a client
// nothing about which skill it belongs to.
//
// Wrapping rather than setting these at construction keeps SKILL.md's shape out
// of every resource type that can back one, and applies on every protocol
// version, so a client that has not negotiated the skills extension still sees
// a sensibly named resource.
type skillDoc struct {
	resources.Resource
	name        string
	description string
}

func (s skillDoc) GetName() string        { return s.name }
func (s skillDoc) GetDescription() string { return s.description }
func (s skillDoc) GetMimeType() string    { return docMimeType }

// WithDocMetadata returns a replacement SKILL.md resource for each entry, keyed
// by URI, carrying the name and description that entry's frontmatter declares.
// Resources that are not a skill's SKILL.md are absent from the result.
//
// Entries must have passed Entry.Validate, which is what guarantees frontmatter
// carries a non-empty name and description.
func WithDocMetadata(entries []Entry, resourcesMap map[string]resources.Resource) map[string]resources.Resource {
	if len(entries) == 0 {
		return nil
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
		name, nameOK := e.Frontmatter["name"].(string)
		desc, descOK := e.Frontmatter["description"].(string)
		if !nameOK || !descOK {
			continue
		}
		docs[res.GetURI()] = skillDoc{Resource: res, name: name, description: desc}
	}
	return docs
}
