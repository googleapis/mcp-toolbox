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

package util

import (
	"context"
	"encoding/json"
	"log/slog"
	"reflect"
	"strings"
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/log"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

type mockParameter struct {
	name string
	typ  string
}

func (m mockParameter) GetName() string                                { return m.name }
func (m mockParameter) GetDesc() string                                { return "" }
func (m mockParameter) GetType() string                                { return m.typ }
func (m mockParameter) GetDefault() any                                { return nil }
func (m mockParameter) GetRequired() bool                              { return false }
func (m mockParameter) GetAuthServices() []parameters.ParamAuthService { return nil }
func (m mockParameter) GetEmbeddedBy() string                          { return "" }
func (m mockParameter) GetValueFromParam() string                      { return "" }
func (m mockParameter) GetSecure() bool                                { return false }
func (m mockParameter) Parse(any) (any, error)                         { return nil, nil }
func (m mockParameter) Manifest() parameters.ParameterManifest         { return parameters.ParameterManifest{} }
func (m mockParameter) McpManifest() (parameters.ParameterMcpManifest, []string) {
	return parameters.ParameterMcpManifest{}, nil
}

type logEntry struct {
	level  string
	msg    string
	params []any
}

type mockLogger struct {
	logs []logEntry
}

func (m *mockLogger) DebugContext(ctx context.Context, msg string, keysAndValues ...any) {
	m.logs = append(m.logs, logEntry{level: "DEBUG", msg: msg, params: keysAndValues})
}
func (m *mockLogger) InfoContext(ctx context.Context, msg string, keysAndValues ...any) {
	m.logs = append(m.logs, logEntry{level: "INFO", msg: msg, params: keysAndValues})
}
func (m *mockLogger) WarnContext(ctx context.Context, msg string, keysAndValues ...any) {
	m.logs = append(m.logs, logEntry{level: "WARN", msg: msg, params: keysAndValues})
}
func (m *mockLogger) ErrorContext(ctx context.Context, msg string, keysAndValues ...any) {
	m.logs = append(m.logs, logEntry{level: "ERROR", msg: msg, params: keysAndValues})
}
func (m *mockLogger) SlogLogger() *slog.Logger {
	return nil
}

func assertLogs(t *testing.T, expected []logEntry, actual []logEntry) {
	t.Helper()
	if len(expected) != len(actual) {
		t.Fatalf("expected %d logs, got %d. Logs: %+v", len(expected), len(actual), actual)
	}
	for i := range expected {
		if expected[i].level != actual[i].level {
			t.Errorf("[%d] expected level %q, got %q", i, expected[i].level, actual[i].level)
		}
		if expected[i].msg != actual[i].msg {
			t.Errorf("[%d] expected msg %q, got %q", i, expected[i].msg, actual[i].msg)
		}
		// Check that the expected parameters (keys and values) are subset of actual parameters.
		for k := 0; k < len(expected[i].params); k += 2 {
			expectedKey := expected[i].params[k].(string)
			expectedVal := expected[i].params[k+1]

			// Find this key in actual params
			found := false
			for a := 0; a < len(actual[i].params); a += 2 {
				actualKey := actual[i].params[a].(string)
				if actualKey == expectedKey {
					found = true
					if expectedKey == "error" {
						if actual[i].params[a+1] == nil {
							t.Errorf("[%d] param %q: expected non-nil error, got nil", i, expectedKey)
						}
					} else {
						if !reflect.DeepEqual(expectedVal, actual[i].params[a+1]) {
							t.Errorf("[%d] param %q: expected %v, got %v", i, expectedKey, expectedVal, actual[i].params[a+1])
						}
					}
					break
				}
			}
			if !found {
				t.Errorf("[%d] expected param key %q not found in actual params", i, expectedKey)
			}
		}
	}
}

func TestPopulateUrlParams(t *testing.T) {
	tests := []struct {
		name         string
		setupCtx     func(ctx context.Context, logger log.Logger) context.Context
		initial      map[string]any
		toolParams   parameters.Parameters
		expected     map[string]any
		expectedLogs []logEntry
		wantErr      bool
		errContains  string
	}{
		{
			name: "no URL params in context",
			setupCtx: func(ctx context.Context, logger log.Logger) context.Context {
				return util.WithLogger(ctx, logger)
			},
			initial: map[string]any{
				"existing": "val",
			},
			toolParams: parameters.Parameters{},
			expected: map[string]any{
				"existing": "val",
			},
			expectedLogs: nil,
		},
		{
			name: "URL params present and key already exists in data - returns error",
			setupCtx: func(ctx context.Context, logger log.Logger) context.Context {
				ctx = util.WithLogger(ctx, logger)
				return util.WithUrlParams(ctx, map[string]string{
					"param1": "newValue",
				})
			},
			initial: map[string]any{
				"param1": "existingValue",
			},
			toolParams: parameters.Parameters{
				mockParameter{name: "param1", typ: "string"},
			},
			expected:     nil,
			expectedLogs: nil,
			wantErr:      true,
			errContains:  `parameter "param1" is bound by URL and cannot be provided in client arguments`,
		},
		{
			name: "URL params present and key not in data - string type",
			setupCtx: func(ctx context.Context, logger log.Logger) context.Context {
				ctx = util.WithLogger(ctx, logger)
				return util.WithUrlParams(ctx, map[string]string{
					"param1": "newValue",
				})
			},
			initial: map[string]any{
				"existing": "val",
			},
			toolParams: parameters.Parameters{
				mockParameter{name: "param1", typ: "string"},
			},
			expected: map[string]any{
				"existing": "val",
				"param1":   "newValue",
			},
			expectedLogs: nil,
		},
		{
			name: "URL params present and key not in data - integer type success",
			setupCtx: func(ctx context.Context, logger log.Logger) context.Context {
				ctx = util.WithLogger(ctx, logger)
				return util.WithUrlParams(ctx, map[string]string{
					"param1": "42",
				})
			},
			initial: map[string]any{},
			toolParams: parameters.Parameters{
				mockParameter{name: "param1", typ: "integer"},
			},
			expected: map[string]any{
				"param1": 42,
			},
			expectedLogs: nil,
		},
		{
			name: "URL params present and key not in data - integer type failure keeps string",
			setupCtx: func(ctx context.Context, logger log.Logger) context.Context {
				ctx = util.WithLogger(ctx, logger)
				return util.WithUrlParams(ctx, map[string]string{
					"param1": "not-an-int",
				})
			},
			initial: map[string]any{},
			toolParams: parameters.Parameters{
				mockParameter{name: "param1", typ: "integer"},
			},
			expected: map[string]any{
				"param1": "not-an-int",
			},
			expectedLogs: []logEntry{
				{
					level: "WARN",
					msg:   "failed to convert URL parameter to integer",
					params: []any{
						"parameter", "param1",
						"value", "not-an-int",
						"error", nil,
					},
				},
			},
		},
		{
			name: "URL params present and key not in data - boolean type success true",
			setupCtx: func(ctx context.Context, logger log.Logger) context.Context {
				ctx = util.WithLogger(ctx, logger)
				return util.WithUrlParams(ctx, map[string]string{
					"param1": "true",
				})
			},
			initial: map[string]any{},
			toolParams: parameters.Parameters{
				mockParameter{name: "param1", typ: "boolean"},
			},
			expected: map[string]any{
				"param1": true,
			},
			expectedLogs: nil,
		},
		{
			name: "URL params present and key not in data - boolean type success false",
			setupCtx: func(ctx context.Context, logger log.Logger) context.Context {
				ctx = util.WithLogger(ctx, logger)
				return util.WithUrlParams(ctx, map[string]string{
					"param1": "false",
				})
			},
			initial: map[string]any{},
			toolParams: parameters.Parameters{
				mockParameter{name: "param1", typ: "boolean"},
			},
			expected: map[string]any{
				"param1": false,
			},
			expectedLogs: nil,
		},
		{
			name: "URL params present and key not in data - boolean type failure keeps string",
			setupCtx: func(ctx context.Context, logger log.Logger) context.Context {
				ctx = util.WithLogger(ctx, logger)
				return util.WithUrlParams(ctx, map[string]string{
					"param1": "not-a-bool",
				})
			},
			initial: map[string]any{},
			toolParams: parameters.Parameters{
				mockParameter{name: "param1", typ: "boolean"},
			},
			expected: map[string]any{
				"param1": "not-a-bool",
			},
			expectedLogs: []logEntry{
				{
					level: "WARN",
					msg:   "failed to convert URL parameter to boolean",
					params: []any{
						"parameter", "param1",
						"value", "not-a-bool",
						"error", nil,
					},
				},
			},
		},
		{
			name: "URL params present and key not in data - float type success",
			setupCtx: func(ctx context.Context, logger log.Logger) context.Context {
				ctx = util.WithLogger(ctx, logger)
				return util.WithUrlParams(ctx, map[string]string{
					"param1": "3.14159",
				})
			},
			initial: map[string]any{},
			toolParams: parameters.Parameters{
				mockParameter{name: "param1", typ: "float"},
			},
			expected: map[string]any{
				"param1": 3.14159,
			},
			expectedLogs: nil,
		},
		{
			name: "URL params present and key not in data - float type failure keeps string",
			setupCtx: func(ctx context.Context, logger log.Logger) context.Context {
				ctx = util.WithLogger(ctx, logger)
				return util.WithUrlParams(ctx, map[string]string{
					"param1": "not-a-float",
				})
			},
			initial: map[string]any{},
			toolParams: parameters.Parameters{
				mockParameter{name: "param1", typ: "float"},
			},
			expected: map[string]any{
				"param1": "not-a-float",
			},
			expectedLogs: []logEntry{
				{
					level: "WARN",
					msg:   "failed to convert URL parameter to float",
					params: []any{
						"parameter", "param1",
						"value", "not-a-float",
						"error", nil,
					},
				},
			},
		},
		{
			name: "URL params present and key not in data - array type success",
			setupCtx: func(ctx context.Context, logger log.Logger) context.Context {
				ctx = util.WithLogger(ctx, logger)
				return util.WithUrlParams(ctx, map[string]string{
					"param1": `["foo", "bar", 123]`,
				})
			},
			initial: map[string]any{},
			toolParams: parameters.Parameters{
				mockParameter{name: "param1", typ: "array"},
			},
			expected: map[string]any{
				"param1": []any{"foo", "bar", json.Number("123")},
			},
			expectedLogs: nil,
		},
		{
			name: "URL params present and key not in data - array type failure keeps string",
			setupCtx: func(ctx context.Context, logger log.Logger) context.Context {
				ctx = util.WithLogger(ctx, logger)
				return util.WithUrlParams(ctx, map[string]string{
					"param1": `invalid-json-array`,
				})
			},
			initial: map[string]any{},
			toolParams: parameters.Parameters{
				mockParameter{name: "param1", typ: "array"},
			},
			expected: map[string]any{
				"param1": `invalid-json-array`,
			},
			expectedLogs: []logEntry{
				{
					level: "WARN",
					msg:   "failed to convert URL parameter to array",
					params: []any{
						"parameter", "param1",
						"value", "invalid-json-array",
						"error", nil,
					},
				},
			},
		},
		{
			name: "URL params present and key not in data - map type success",
			setupCtx: func(ctx context.Context, logger log.Logger) context.Context {
				ctx = util.WithLogger(ctx, logger)
				return util.WithUrlParams(ctx, map[string]string{
					"param1": `{"nested": "value", "num": 123}`,
				})
			},
			initial: map[string]any{},
			toolParams: parameters.Parameters{
				mockParameter{name: "param1", typ: "map"},
			},
			expected: map[string]any{
				"param1": map[string]any{
					"nested": "value",
					"num":    json.Number("123"),
				},
			},
			expectedLogs: nil,
		},
		{
			name: "URL params present and key not in data - map type failure keeps string",
			setupCtx: func(ctx context.Context, logger log.Logger) context.Context {
				ctx = util.WithLogger(ctx, logger)
				return util.WithUrlParams(ctx, map[string]string{
					"param1": `invalid-json-map`,
				})
			},
			initial: map[string]any{},
			toolParams: parameters.Parameters{
				mockParameter{name: "param1", typ: "map"},
			},
			expected: map[string]any{
				"param1": `invalid-json-map`,
			},
			expectedLogs: []logEntry{
				{
					level: "WARN",
					msg:   "failed to convert URL parameter to map",
					params: []any{
						"parameter", "param1",
						"value", "invalid-json-map",
						"error", nil,
					},
				},
			},
		},
		{
			name: "nil initial data map is allocated",
			setupCtx: func(ctx context.Context, logger log.Logger) context.Context {
				ctx = util.WithLogger(ctx, logger)
				return util.WithUrlParams(ctx, map[string]string{
					"param1": "foo",
				})
			},
			initial: nil,
			toolParams: parameters.Parameters{
				mockParameter{name: "param1", typ: "string"},
			},
			expected: map[string]any{
				"param1": "foo",
			},
			expectedLogs: nil,
		},
		{
			name: "URL params present but param name not in tool parameters list",
			setupCtx: func(ctx context.Context, logger log.Logger) context.Context {
				ctx = util.WithLogger(ctx, logger)
				return util.WithUrlParams(ctx, map[string]string{
					"paramUnknown": "foo",
				})
			},
			initial:    map[string]any{},
			toolParams: parameters.Parameters{},
			expected: map[string]any{
				"paramUnknown": "foo",
			},
			expectedLogs: []logEntry{
				{
					level: "WARN",
					msg:   "URL parameter not defined in tool parameters",
					params: []any{
						"parameter", "paramUnknown",
					},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logger := &mockLogger{}
			ctx := tc.setupCtx(context.Background(), logger)
			actual, err := PopulateUrlParams(ctx, tc.initial, tc.toolParams)
			if (err != nil) != tc.wantErr {
				t.Fatalf("PopulateUrlParams() error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr && tc.errContains != "" && (err == nil || !strings.Contains(err.Error(), tc.errContains)) {
				t.Errorf("PopulateUrlParams() error = %v, want error containing %q", err, tc.errContains)
			}
			if !reflect.DeepEqual(actual, tc.expected) {
				t.Errorf("PopulateUrlParams() = %v, want %v", actual, tc.expected)
			}
			assertLogs(t, tc.expectedLogs, logger.logs)
		})
	}
}
