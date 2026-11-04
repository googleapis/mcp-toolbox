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
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	httpsource "github.com/googleapis/mcp-toolbox/internal/sources/http"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	http "github.com/googleapis/mcp-toolbox/internal/tools/http"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestInvokeHeaderPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name        string
		toolHeaders map[string]string
		headerParam string
		want        string
	}{
		{
			name: "source default",
			want: "text/plain; charset=UTF-8",
		},
		{
			name:        "tool overrides source",
			toolHeaders: map[string]string{"Content-Type": "application/json; charset=UTF-8"},
			want:        "application/json; charset=UTF-8",
		},
		{
			name:        "provided parameter overrides source and tool",
			toolHeaders: map[string]string{"Content-Type": "application/json; charset=UTF-8"},
			headerParam: "application/xml; charset=UTF-8",
			want:        "application/xml; charset=UTF-8",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			received := make(chan string, 1)
			receiver := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
				received <- r.Header.Get("Content-Type")
				w.WriteHeader(stdhttp.StatusNoContent)
			}))
			defer receiver.Close()

			ctx, err := testutils.ContextWithNewLogger()
			if err != nil {
				t.Fatal(err)
			}
			ctx = testutils.ContextWithUserAgent(ctx, "header-precedence-test")
			sourceCfg := httpsource.Config{
				Name:                 "http-source",
				Type:                 "http",
				BaseURL:              receiver.URL,
				Timeout:              "5s",
				DefaultHeaders:       map[string]string{"content-type": "text/plain; charset=UTF-8"},
				AllowPrivateNetworks: true,
			}
			source, err := sourceCfg.Initialize(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			cfg := http.Config{
				ConfigBase: tools.ConfigBase{Name: "http-tool", Description: "test header precedence"},
				Type:       "http",
				Source:     "http-source",
				Method:     "GET",
				Path:       "/",
				Headers:    tc.toolHeaders,
			}
			input := map[string]any{}
			if tc.headerParam != "" {
				cfg.HeaderParams = parameters.Parameters{parameters.NewStringParameter("CONTENT-type", "content type")}
				input["CONTENT-type"] = tc.headerParam
			}
			tool, err := cfg.Initialize(ctx)
			if err != nil {
				t.Fatal(err)
			}
			params, err := parameters.ParseParams(cfg.HeaderParams, input, nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := tool.Invoke(ctx, source, params, ""); err != nil {
				t.Fatal(err)
			}
			if got := <-received; got != tc.want {
				t.Fatalf("received Content-Type %q, want %q", got, tc.want)
			}
		})
	}
}

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
