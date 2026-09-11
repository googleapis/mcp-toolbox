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

	"github.com/googleapis/mcp-toolbox/internal/resources"
)

// snapshot serves the bytes read when the skill was discovered, rather than
// re-reading the resource it wraps. A static skill publishes a digest per file,
// and a host must reject content that does not match it, so bytes that change
// under a live server would fail verification against a digest the server keeps
// republishing until the config is reloaded.
//
// Every method other than Read and GetSize is promoted from the embedded
// resource, so the wrapper stays transparent to everything else.
type snapshot struct {
	resources.Resource
	content string
	size    int64
}

func (s snapshot) Read(context.Context, map[string]any) (any, error) {
	return s.content, nil
}

// GetSize reports the length of the captured bytes. The wrapped file resource
// stats the disk on every resources/list, which would let a listing advertise a
// length the served content does not have.
func (s snapshot) GetSize() *int64 {
	size := s.size
	return &size
}
