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

package lookercommon_test

import (
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/looker/lookercommon"
)

func TestReadOnlyAnnotations(t *testing.T) {
	t.Run("nil custom", func(t *testing.T) {
		ann := lookercommon.ReadOnlyAnnotations(nil)
		if ann == nil {
			t.Fatal("expected non-nil annotations")
		}
		if ann.OpenWorldHint == nil || *ann.OpenWorldHint != false {
			t.Errorf("expected openWorldHint=false, got %v", ann.OpenWorldHint)
		}
		if ann.DestructiveHint == nil || *ann.DestructiveHint != false {
			t.Errorf("expected destructiveHint=false, got %v", ann.DestructiveHint)
		}
		if ann.ReadOnlyHint == nil || *ann.ReadOnlyHint != true {
			t.Errorf("expected readOnlyHint=true, got %v", ann.ReadOnlyHint)
		}
	})

	t.Run("custom with idempotent", func(t *testing.T) {
		idempotentTrue := true
		openWorldTrue := true
		destructiveTrue := true
		custom := &tools.ToolAnnotations{
			IdempotentHint:  &idempotentTrue,
			OpenWorldHint:   &openWorldTrue,
			DestructiveHint: &destructiveTrue,
		}
		ann := lookercommon.ReadOnlyAnnotations(custom)
		if ann.IdempotentHint == nil || *ann.IdempotentHint != true {
			t.Errorf("expected idempotentHint=true preserved, got %v", ann.IdempotentHint)
		}
		if ann.OpenWorldHint == nil || *ann.OpenWorldHint != false {
			t.Errorf("expected openWorldHint=false, got %v", ann.OpenWorldHint)
		}
		if ann.DestructiveHint == nil || *ann.DestructiveHint != false {
			t.Errorf("expected destructiveHint=false, got %v", ann.DestructiveHint)
		}
		if ann.ReadOnlyHint == nil || *ann.ReadOnlyHint != true {
			t.Errorf("expected readOnlyHint=true, got %v", ann.ReadOnlyHint)
		}
	})
}

func TestWriteAnnotations(t *testing.T) {
	t.Run("nil custom", func(t *testing.T) {
		ann := lookercommon.WriteAnnotations(nil)
		if ann == nil {
			t.Fatal("expected non-nil annotations")
		}
		if ann.OpenWorldHint == nil || *ann.OpenWorldHint != false {
			t.Errorf("expected openWorldHint=false, got %v", ann.OpenWorldHint)
		}
		if ann.DestructiveHint == nil || *ann.DestructiveHint != false {
			t.Errorf("expected destructiveHint=false, got %v", ann.DestructiveHint)
		}
		if ann.ReadOnlyHint == nil || *ann.ReadOnlyHint != false {
			t.Errorf("expected readOnlyHint=false, got %v", ann.ReadOnlyHint)
		}
	})
}

func TestDestructiveAnnotations(t *testing.T) {
	t.Run("nil custom", func(t *testing.T) {
		ann := lookercommon.DestructiveAnnotations(nil)
		if ann == nil {
			t.Fatal("expected non-nil annotations")
		}
		if ann.OpenWorldHint == nil || *ann.OpenWorldHint != false {
			t.Errorf("expected openWorldHint=false, got %v", ann.OpenWorldHint)
		}
		if ann.DestructiveHint == nil || *ann.DestructiveHint != true {
			t.Errorf("expected destructiveHint=true, got %v", ann.DestructiveHint)
		}
		if ann.ReadOnlyHint == nil || *ann.ReadOnlyHint != false {
			t.Errorf("expected readOnlyHint=false, got %v", ann.ReadOnlyHint)
		}
	})
}

func TestNewAnnotations(t *testing.T) {
	ro := lookercommon.NewReadOnlyAnnotations()
	if *ro.ReadOnlyHint != true || *ro.DestructiveHint != false || *ro.OpenWorldHint != false {
		t.Errorf("unexpected NewReadOnlyAnnotations: %+v", ro)
	}

	w := lookercommon.NewWriteAnnotations()
	if *w.ReadOnlyHint != false || *w.DestructiveHint != false || *w.OpenWorldHint != false {
		t.Errorf("unexpected NewWriteAnnotations: %+v", w)
	}

	d := lookercommon.NewDestructiveAnnotations()
	if *d.ReadOnlyHint != false || *d.DestructiveHint != true || *d.OpenWorldHint != false {
		t.Errorf("unexpected NewDestructiveAnnotations: %+v", d)
	}
}
