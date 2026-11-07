// Copyright 2025 Google LLC
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

package http_test

import (
	"context"
	nethttp "net/http"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	http "github.com/googleapis/mcp-toolbox/internal/tools/http"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestParseFromYamlHTTP(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	tcs := []struct {
		desc string
		in   string
		want server.ToolConfigs
	}{
		{
			desc: "basic example",
			in: `
			kind: tool
			name: example_tool
			type: http
			source: my-instance
			method: GET
			description: some description
			path: search
			`,
			want: server.ToolConfigs{
				"example_tool": http.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some description",
						AuthRequired: []string{},
					},
					Type:   "http",
					Source: "my-instance",
					Method: "GET",
					Path:   "search",
				},
			},
		},
		{
			desc: "advanced example",
			in: `
			kind: tool
			name: example_tool
			type: http
			source: my-instance
			method: GET
			path: "{{.pathParam}}?name=alice&pet=cat"
			description: some description
			authRequired:
				- my-google-auth-service
				- other-auth-service
			queryParams:
				- name: country
				  type: string
				  description: some description
				  authServices:
					- name: my-google-auth-service
					  field: user_id
					- name: other-auth-service
					  field: user_id
			pathParams:
			    - name: pathParam
			      type: string
			      description: path param
			requestBody: |
					{
						"age": {{.age}},
						"city": "{{.city}}",
						"food": {{.food}}
					}
			bodyParams:
				- name: age
				  type: integer
				  description: age num
				- name: city
				  type: string
				  description: city string
			headers:
				Authorization: API_KEY
				Content-Type: application/json
			headerParams:
				- name: Language
				  type: string
				  description: language string
			`,
			want: server.ToolConfigs{
				"example_tool": http.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some description",
						AuthRequired: []string{"my-google-auth-service", "other-auth-service"},
					},
					Type:   "http",
					Source: "my-instance",
					Method: "GET",
					Path:   "{{.pathParam}}?name=alice&pet=cat",
					QueryParams: []parameters.Parameter{
						parameters.NewStringParameter("country", "some description", parameters.WithStringAuth(
							[]parameters.ParamAuthService{{Name: "my-google-auth-service", Field: "user_id"},
								{Name: "other-auth-service", Field: "user_id"}})),
					},
					PathParams: parameters.Parameters{
						&parameters.StringParameter{
							CommonParameter: parameters.CommonParameter{Name: "pathParam", Type: "string", Desc: "path param"},
						},
					},
					RequestBody: `{
  "age": {{.age}},
  "city": "{{.city}}",
  "food": {{.food}}
}
`,
					BodyParams:   []parameters.Parameter{parameters.NewIntParameter("age", "age num"), parameters.NewStringParameter("city", "city string")},
					Headers:      map[string]string{"Authorization": "API_KEY", "Content-Type": "application/json"},
					HeaderParams: []parameters.Parameter{parameters.NewStringParameter("Language", "language string")},
				},
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, _, _, got, _, _, _, _, err := server.UnmarshalPrimitiveConfig(ctx, testutils.FormatYaml(tc.in))
			if err != nil {
				t.Fatalf("unable to unmarshal: %s", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("incorrect parse: diff %v", diff)
			}
		})
	}

}

func TestFailParseFromYamlHTTP(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	tcs := []struct {
		desc string
		in   string
		err  string
	}{
		{
			desc: "Invalid method",
			in: `
			kind: tool
			name: example_tool
			type: http
			source: my-instance
			method: GOT
			path: "search?name=alice&pet=cat"
			description: some description
			authRequired:
				- my-google-auth-service
				- other-auth-service
			queryParams:
				- name: country
				  type: string
				  description: some description
				  authServices:
					- name: my-google-auth-service
					  field: user_id
					- name: other-auth-service
					  field: user_id
			requestBody: |
					{
						"age": {{.age}},
						"city": "{{.city}}"
					}
			bodyParams:
				- name: age
				  type: integer
				  description: age num
				- name: city
				  type: string
				  description: city string
			headers:
				Authorization: API_KEY
				Content-Type: application/json
			headerParams:
				- name: Language
				  type: string
				  description: language string
			`,
			err: `GOT is not a valid http method`,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, _, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(ctx, testutils.FormatYaml(tc.in))
			if err == nil {
				t.Fatalf("expect parsing to fail")
			}
			errStr := err.Error()
			if !strings.Contains(errStr, tc.err) {
				t.Fatalf("unexpected error string: got %q, want substring %q", errStr, tc.err)
			}
		})
	}

}

// headerCaptureSource is a minimal HTTP source that records the headers of
// the request it is asked to run instead of sending it.
type headerCaptureSource struct {
	got nethttp.Header
}

func (s *headerCaptureSource) SourceType() string                 { return "http" }
func (s *headerCaptureSource) ToConfig() sources.SourceConfig     { return nil }
func (s *headerCaptureSource) IsReadOnly() bool                   { return false }
func (s *headerCaptureSource) HttpBaseURL() string                { return "http://example.com" }
func (s *headerCaptureSource) HttpQueryParams() map[string]string { return nil }
func (s *headerCaptureSource) HttpDefaultHeaders() map[string]string {
	return map[string]string{"Content-Type": "application/json"}
}

func (s *headerCaptureSource) RunRequest(_ context.Context, req *nethttp.Request) (any, error) {
	s.got = req.Header.Clone()
	return "ok", nil
}

func TestInvokeOptionalHeaderParams(t *testing.T) {
	cfg := http.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "example_tool",
			Description: "some description",
		},
		Type:   "http",
		Source: "my-instance",
		Method: "GET",
		Path:   "/test",
		HeaderParams: []parameters.Parameter{
			parameters.NewStringParameter("X-Trace-Id", "optional trace id", parameters.WithStringRequired(false)),
		},
	}
	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("unexpected error initializing tool: %s", err)
	}

	tcs := []struct {
		desc string
		in   map[string]any
		want map[string]string
	}{
		{
			desc: "optional header omitted",
			in:   map[string]any{},
			want: map[string]string{"Content-Type": "application/json"},
		},
		{
			desc: "optional header provided",
			in:   map[string]any{"X-Trace-Id": "abc123"},
			want: map[string]string{"Content-Type": "application/json", "X-Trace-Id": "abc123"},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			// Build the param values through ParseParams, as the server does
			// before calling Invoke.
			params, err := parameters.ParseParams(cfg.HeaderParams, tc.in, nil)
			if err != nil {
				t.Fatalf("unexpected error parsing params: %s", err)
			}
			src := &headerCaptureSource{}
			if _, toolErr := tool.Invoke(context.Background(), src, params, ""); toolErr != nil {
				t.Fatalf("unexpected error invoking tool: %s", toolErr)
			}
			got := make(map[string]string)
			for k := range src.got {
				got[k] = src.got.Get(k)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("incorrect request headers (-want +got):\n%s", diff)
			}
		})
	}
}
