// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package lookercommon

import (
	"github.com/googleapis/mcp-toolbox/internal/tools"
)

func newAnnotations(custom *tools.ToolAnnotations, readOnly, destructive bool) *tools.ToolAnnotations {
	ann := &tools.ToolAnnotations{}
	if custom != nil {
		*ann = *custom
	}
	openWorld := false
	ann.OpenWorldHint = &openWorld
	ann.ReadOnlyHint = &readOnly
	ann.DestructiveHint = &destructive
	return ann
}

// ReadOnlyAnnotations returns ToolAnnotations with openWorldHint=false,
// destructiveHint=false, and readOnlyHint=true for Looker read-only tools.
func ReadOnlyAnnotations(custom *tools.ToolAnnotations) *tools.ToolAnnotations {
	return newAnnotations(custom, true, false)
}

// WriteAnnotations returns ToolAnnotations with openWorldHint=false,
// destructiveHint=false, and readOnlyHint=false for Looker additive write tools.
func WriteAnnotations(custom *tools.ToolAnnotations) *tools.ToolAnnotations {
	return newAnnotations(custom, false, false)
}

// DestructiveAnnotations returns ToolAnnotations with openWorldHint=false,
// destructiveHint=true, and readOnlyHint=false for Looker tools that can change
// existing data or metadata in the system.
func DestructiveAnnotations(custom *tools.ToolAnnotations) *tools.ToolAnnotations {
	return newAnnotations(custom, false, true)
}

// NewReadOnlyAnnotations creates default annotations for a Looker read-only tool.
func NewReadOnlyAnnotations() *tools.ToolAnnotations {
	return ReadOnlyAnnotations(nil)
}

// NewWriteAnnotations creates default annotations for a Looker additive write tool.
func NewWriteAnnotations() *tools.ToolAnnotations {
	return WriteAnnotations(nil)
}

// NewDestructiveAnnotations creates default annotations for a Looker destructive tool.
func NewDestructiveAnnotations() *tools.ToolAnnotations {
	return DestructiveAnnotations(nil)
}
