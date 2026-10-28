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

package tools

// ResultWithMeta lets a tool's Invoke return out-of-band metadata (for example
// token usage or debug traces from an upstream service) that is meant for the
// calling application rather than the model. Servers serialize Result as the
// tool output and surface Meta separately: in the MCP CallToolResult `_meta`
// and in the `meta` field of the HTTP API response.
type ResultWithMeta struct {
	Result any
	Meta   map[string]any
}

// SplitResultMeta unwraps a ResultWithMeta. Any other value is returned
// unchanged with nil metadata.
func SplitResultMeta(res any) (any, map[string]any) {
	switch r := res.(type) {
	case ResultWithMeta:
		return r.Result, r.Meta
	case *ResultWithMeta:
		if r != nil {
			return r.Result, r.Meta
		}
		return nil, nil
	}
	return res, nil
}
