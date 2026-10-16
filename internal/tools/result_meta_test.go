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

package tools_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/tools"
)

func TestSplitResultMeta(t *testing.T) {
	meta := map[string]any{"k": "v"}
	tcs := []struct {
		name     string
		in       any
		wantRes  any
		wantMeta map[string]any
	}{
		{name: "plain value", in: []any{"a"}, wantRes: []any{"a"}},
		{name: "nil", in: nil, wantRes: nil},
		{name: "value wrapper", in: tools.ResultWithMeta{Result: "r", Meta: meta}, wantRes: "r", wantMeta: meta},
		{name: "pointer wrapper", in: &tools.ResultWithMeta{Result: "r", Meta: meta}, wantRes: "r", wantMeta: meta},
		{name: "nil pointer wrapper", in: (*tools.ResultWithMeta)(nil), wantRes: nil},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			gotRes, gotMeta := tools.SplitResultMeta(tc.in)
			if diff := cmp.Diff(tc.wantRes, gotRes); diff != "" {
				t.Errorf("result mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.wantMeta, gotMeta); diff != "" {
				t.Errorf("meta mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
