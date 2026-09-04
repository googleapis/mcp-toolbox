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

package internal

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/auth/generic"
	"github.com/googleapis/mcp-toolbox/internal/auth/google"
	"github.com/googleapis/mcp-toolbox/internal/embeddingmodels/gemini"
	"github.com/googleapis/mcp-toolbox/internal/group"
	"github.com/googleapis/mcp-toolbox/internal/prebuiltconfigs"
	"github.com/googleapis/mcp-toolbox/internal/prompts"
	"github.com/googleapis/mcp-toolbox/internal/prompts/custom"
	"github.com/googleapis/mcp-toolbox/internal/server"
	cloudsqlpgsrc "github.com/googleapis/mcp-toolbox/internal/sources/cloudsqlpg"
	httpsrc "github.com/googleapis/mcp-toolbox/internal/sources/http"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/http"
	"github.com/googleapis/mcp-toolbox/internal/tools/postgres/postgressql"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestParseEnv(t *testing.T) {
	tcs := []struct {
		desc         string
		env          map[string]string
		in           string
		want         string
		err          bool
		errString    string
		wantOptional []string
		wantRequired []string
		lenient      bool
	}{
		{
			desc:         "without default without env",
			in:           "${FOO}",
			want:         "",
			err:          true,
			errString:    `environment variable not found: "FOO" (line 1, column 1)`,
			wantRequired: []string{"FOO"},
		},
		{
			desc:         "multiple missing required vars reported together",
			in:           "host: ${HOST}, user: ${USER_NAME}, pass: ${PASSWORD}",
			want:         "host: , user: , pass: ",
			err:          true,
			errString:    "environment variables not found:\n  - \"HOST\" (line 1, column 7)\n  - \"USER_NAME\" (line 1, column 22)\n  - \"PASSWORD\" (line 1, column 42)",
			wantRequired: []string{"HOST", "USER_NAME", "PASSWORD"},
		},
		{
			desc:         "repeated missing required var reported once",
			in:           "a: ${HOST}, b: ${HOST}",
			want:         "a: , b: ",
			err:          true,
			errString:    `environment variable not found: "HOST" (line 1, column 4)`,
			wantRequired: []string{"HOST", "HOST"},
		},
		{
			desc:         "legacy and non-empty errors reported together",
			in:           "${TEST_LEGACY_UNSET_3635} ${TEST_REQUIRED_UNSET_3635:?must be set}",
			want:         " ",
			err:          true,
			errString:    "environment variable expansion errors:\n  - environment variable not found: \"TEST_LEGACY_UNSET_3635\" (line 1, column 1)\n  - required environment variable is unset or empty: \"TEST_REQUIRED_UNSET_3635\": must be set (line 1, column 27)",
			wantRequired: []string{"TEST_LEGACY_UNSET_3635", "TEST_REQUIRED_UNSET_3635"},
		},
		{
			desc:         "without default without env, lenient",
			in:           "${FOO}",
			want:         "FOO",
			wantRequired: []string{"FOO"},
			lenient:      true,
		},
		{
			desc:         "missing required mixed with env, lenient",
			in:           "project: ${PROJECT}, region: ${REGION}",
			env:          map[string]string{"REGION": "us-central1"},
			want:         "project: PROJECT, region: us-central1",
			wantRequired: []string{"PROJECT", "REGION"},
			lenient:      true,
		},
		{
			desc: "without default with env",
			env: map[string]string{
				"FOO": "bar",
			},
			in:           "${FOO}",
			want:         "bar",
			wantRequired: []string{"FOO"},
		},
		{
			desc:         "without default with empty env",
			env:          map[string]string{"FOO": ""},
			in:           "${FOO}",
			want:         "",
			wantRequired: []string{"FOO"},
		},
		{
			desc: "skip commented out env var",
			in:   "# ${FOO}",
			want: "# ${FOO}",
		},
		{
			desc: "skip commented out env var with preceding whitespace",
			in:   "  # ${FOO}",
			want: "  # ${FOO}",
		},
		{
			desc: "skip commented out env var in inline comment",
			in:   "port: 8080 # default is ${DEFAULT_PORT}",
			want: "port: 8080 # default is ${DEFAULT_PORT}",
		},
		{
			desc: "skip commented out env var in inline comment but parse active env var",
			in:   "port: ${PORT} # default is ${DEFAULT_PORT}",
			env: map[string]string{
				"PORT": "9090",
			},
			want:         "port: 9090 # default is ${DEFAULT_PORT}",
			wantRequired: []string{"PORT"},
		},
		{
			desc: "do not skip env var with hash inside double quotes",
			in:   `url: "http://example.com/#${ANCHOR}"`,
			env: map[string]string{
				"ANCHOR": "section1",
			},
			want:         `url: "http://example.com/#section1"`,
			wantRequired: []string{"ANCHOR"},
		},
		{
			desc: "do not skip env var with hash inside single quotes",
			in:   `url: 'http://example.com/#${ANCHOR}'`,
			env: map[string]string{
				"ANCHOR": "section1",
			},
			want:         `url: 'http://example.com/#section1'`,
			wantRequired: []string{"ANCHOR"},
		},
		{
			desc: "skip commented out env var but parse non-commented ones",
			in:   "foo: ${BAR}\n# ${FOO}\nbaz: ${QUX}",
			env: map[string]string{
				"BAR": "bar_val",
				"QUX": "qux_val",
			},
			want:         "foo: bar_val\n# ${FOO}\nbaz: qux_val",
			wantRequired: []string{"BAR", "QUX"},
		},
		{
			desc: "parse env vars after a comment containing multi-byte characters",
			in: "# " + strings.Repeat("é", 30) + "\n" +
				"foo: ${BAR}\n" +
				"# " + strings.Repeat("é", 30) + "\n" +
				"baz: ${QUX}\n",
			env: map[string]string{
				"BAR": "bar_val",
				"QUX": "qux_val",
			},
			want: "# " + strings.Repeat("é", 30) + "\n" +
				"foo: bar_val\n" +
				"# " + strings.Repeat("é", 30) + "\n" +
				"baz: qux_val\n",
			wantRequired: []string{"BAR", "QUX"},
		},
		{
			desc: "skip commented out env var when comment contains multi-byte characters",
			in:   "# " + strings.Repeat("é", 30) + " ${FOO}\nbar: ${BAR}\n",
			env: map[string]string{
				"BAR": "bar_val",
			},
			want:         "# " + strings.Repeat("é", 30) + " ${FOO}\nbar: bar_val\n",
			wantRequired: []string{"BAR"},
		},
		{
			desc: "multiline yaml with mixed comments and env vars",
			in: "database: my-db # Another comment in line ${SHOULD_BE_IGNORED}\n" +
				"host: \"localhost\"\n" +
				"# This is a comment, ${SHOULD_BE_IGNORED} won't be replaced!\n" +
				"port: ${DB_PORT:5432}\n" +
				"user: ${DB_USER}\n",
			env: map[string]string{
				"DB_USER": "my_user",
			},
			want: "database: my-db # Another comment in line ${SHOULD_BE_IGNORED}\n" +
				"host: \"localhost\"\n" +
				"# This is a comment, ${SHOULD_BE_IGNORED} won't be replaced!\n" +
				"port: 5432\n" +
				"user: my_user\n",
			wantOptional: []string{"DB_PORT"},
			wantRequired: []string{"DB_USER"},
		},
		{
			desc:         "with default without env",
			in:           "${FOO:}",
			want:         "",
			wantOptional: []string{"FOO"},
		},
		{
			desc:         "with default",
			in:           "${FOO:bar}",
			want:         "bar",
			wantOptional: []string{"FOO"},
		},
		{
			desc: "with default with env",
			env: map[string]string{
				"FOO": "hello",
			},
			in:           "${FOO:bar}",
			want:         "hello",
			wantOptional: []string{"FOO"},
		},
		{
			desc:         "legacy default preserves empty env",
			env:          map[string]string{"FOO": ""},
			in:           "${FOO:bar}",
			want:         "",
			wantOptional: []string{"FOO"},
		},
		{
			desc:         "non-empty default without env",
			in:           "${TEST_NONEMPTY_DEFAULT_UNSET_3635:-bar}",
			want:         "bar",
			wantOptional: []string{"TEST_NONEMPTY_DEFAULT_UNSET_3635"},
		},
		{
			desc:         "non-empty default with empty env",
			env:          map[string]string{"FOO": ""},
			in:           "${FOO:-bar}",
			want:         "bar",
			wantOptional: []string{"FOO"},
		},
		{
			desc:         "non-empty default with env",
			env:          map[string]string{"FOO": "hello"},
			in:           "${FOO:-bar}",
			want:         "hello",
			wantOptional: []string{"FOO"},
		},
		{
			desc:         "required non-empty without env",
			in:           "${TEST_REQUIRED_UNSET_3635:?}",
			want:         "",
			err:          true,
			errString:    `required environment variable is unset or empty: "TEST_REQUIRED_UNSET_3635" (line 1, column 1)`,
			wantRequired: []string{"TEST_REQUIRED_UNSET_3635"},
		},
		{
			desc:         "required non-empty with empty env",
			env:          map[string]string{"FOO": ""},
			in:           "${FOO:?}",
			want:         "",
			err:          true,
			errString:    `required environment variable is unset or empty: "FOO" (line 1, column 1)`,
			wantRequired: []string{"FOO"},
		},
		{
			desc:         "required non-empty with custom error",
			env:          map[string]string{"FOO": ""},
			in:           "value: ${FOO:?FOO must be set}",
			want:         "value: ",
			err:          true,
			errString:    `required environment variable is unset or empty: "FOO": FOO must be set (line 1, column 8)`,
			wantRequired: []string{"FOO"},
		},
		{
			desc:         "required non-empty with env",
			env:          map[string]string{"FOO": "hello"},
			in:           "${FOO:?FOO must be set}",
			want:         "hello",
			wantRequired: []string{"FOO"},
		},
		{
			desc:         "required non-empty with empty env, lenient",
			env:          map[string]string{"FOO": ""},
			in:           "${FOO:?FOO must be set}",
			want:         "FOO",
			wantRequired: []string{"FOO"},
			lenient:      true,
		},
		{
			desc: "multiple variables",
			in:   "user: ${USER_NAME:}, password: ${PASSWORD:}, ip: ${IP:public}, region: ${REGION}",
			env: map[string]string{
				"REGION": "us-central1",
			},
			want:         "user: , password: , ip: public, region: us-central1",
			wantOptional: []string{"USER_NAME", "PASSWORD", "IP"},
			wantRequired: []string{"REGION"},
		},
		{
			desc: "variable required in one place and optional in another",
			in:   "project_req: ${PROJECT_ID}, project_opt: ${PROJECT_ID:default}",
			env: map[string]string{
				"PROJECT_ID": "my_project",
			},
			want:         "project_req: my_project, project_opt: my_project",
			wantOptional: []string{}, // Because it was marked required at least once
			wantRequired: []string{"PROJECT_ID"},
		},
		{
			desc: "variable required non-empty in one place and optional in another",
			in:   "project_req: ${PROJECT_ID:?}, project_opt: ${PROJECT_ID:-default}",
			env: map[string]string{
				"PROJECT_ID": "my_project",
			},
			want:         "project_req: my_project, project_opt: my_project",
			wantOptional: []string{},
			wantRequired: []string{"PROJECT_ID"},
		},
		{
			desc: "unsupported non-colon default remains unchanged",
			in:   "${FOO-default}",
			want: "${FOO-default}",
		},
		{
			desc: "unsupported non-colon required remains unchanged",
			in:   "${FOO?message}",
			want: "${FOO?message}",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			if tc.env != nil {
				for k, v := range tc.env {
					t.Setenv(k, v)
				}
			}
			parser := &ConfigParser{AllowMissingEnvVars: tc.lenient}
			got, err := parser.parseEnv(tc.in)
			if tc.err {
				if err == nil {
					t.Fatalf("expected error not found")
				}
				if tc.errString != err.Error() {
					t.Fatalf("incorrect error string: got %s, want %s", err, tc.errString)
				}
			}
			if tc.want != got {
				t.Fatalf("unexpected want: got %s, want %s", got, tc.want)
			}
			if len(parser.OptionalEnvVars) != len(tc.wantOptional) {
				t.Fatalf("OptionalEnvVars length mismatch: got %d, want %d. Got: %v, Want: %v", len(parser.OptionalEnvVars), len(tc.wantOptional), parser.OptionalEnvVars, tc.wantOptional)
			}
			for i, v := range parser.OptionalEnvVars {
				if v != tc.wantOptional[i] {
					t.Errorf("OptionalEnvVars element %d mismatch: got %q, want %q", i, v, tc.wantOptional[i])
				}
			}
			if diff := cmp.Diff(tc.wantRequired, parser.requiredEnvVars); diff != "" {
				t.Errorf("requiredEnvVars mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParseConfigEnvExpansion(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	t.Run("expands non-empty default and required value", func(t *testing.T) {
		t.Setenv("EMPTY_BASE_URL", "")
		t.Setenv("REQUIRED_HEADER", "actual-header")
		parser := ConfigParser{}
		config, err := parser.ParseConfig(ctx, []byte(`
kind: source
name: my-http-instance
type: http
baseUrl: ${EMPTY_BASE_URL:-https://example.com}
headers:
  Required: ${REQUIRED_HEADER:?header is required}
`))
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		source, ok := config.Sources["my-http-instance"].(httpsrc.Config)
		if !ok {
			t.Fatalf("unexpected source type: %T", config.Sources["my-http-instance"])
		}
		if source.BaseURL != "https://example.com" {
			t.Errorf("BaseURL mismatch: got %q, want %q", source.BaseURL, "https://example.com")
		}
		if got := source.DefaultHeaders["Required"]; got != "actual-header" {
			t.Errorf("required header mismatch: got %q, want %q", got, "actual-header")
		}
	})

	t.Run("rejects empty required value", func(t *testing.T) {
		t.Setenv("REQUIRED_HEADER", "")
		parser := ConfigParser{}
		_, err := parser.ParseConfig(ctx, []byte(`
kind: source
name: my-http-instance
type: http
baseUrl: https://example.com
headers:
  Required: ${REQUIRED_HEADER:?header is required}
`))
		if err == nil {
			t.Fatal("expected error")
		}
		want := `required environment variable is unset or empty: "REQUIRED_HEADER": header is required (line 7, column 13)`
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error mismatch: got %q, want substring %q", err, want)
		}
	})
}

func TestConvertConfig(t *testing.T) {
	tcs := []struct {
		desc   string
		in     string
		want   string
		isErr  bool
		errStr string
	}{
		{
			desc: "basic convert",
			in: `
            sources:
                my-pg-instance:
                    kind: cloud-sql-postgres
                    project: my-project
                    region: my-region
                    instance: my-instance
                    database: my_db
                    user: my_user
                    password: my_pass
            authServices:
                my-google-auth:
                    kind: google
                    clientId: testing-id
            tools:
                example_tool:
                    kind: postgres-sql
                    source: my-pg-instance
                    description: some description
                    statement: SELECT * FROM SQL_STATEMENT;
                    parameters:
                        - name: country
                          type: string
                          description: some description
            toolsets:
                example_toolset:
                    - example_tool
            prompts:
                code_review:
                    description: ask llm to analyze code quality
                    messages:
                      - content: "please review the following code for quality: {{.code}}"
                    arguments:
                        - name: code
                          description: the code to review
            embeddingModels:
                gemini-model:
                    kind: gemini
                    model: gemini-embedding-001
                    apiKey: some-key
                    dimension: 768`,
			want: `
kind: source
name: my-pg-instance
type: cloud-sql-postgres
project: my-project
region: my-region
instance: my-instance
database: my_db
user: my_user
password: my_pass
---
kind: authService
name: my-google-auth
type: google
clientId: testing-id
---
kind: tool
name: example_tool
type: postgres-sql
source: my-pg-instance
description: some description
statement: SELECT * FROM SQL_STATEMENT;
parameters:
- name: country
  type: string
  description: some description
---
kind: group
name: example_toolset
tools:
- example_tool
---
kind: prompt
name: code_review
description: ask llm to analyze code quality
messages:
- content: "please review the following code for quality: {{.code}}"
arguments:
- name: code
  description: the code to review
---
kind: embeddingModel
name: gemini-model
type: gemini
model: gemini-embedding-001
apiKey: some-key
dimension: 768
`,
		},
		{
			desc: "convert nested groups",
			in: `
            groups:
                my_group:
                    description: my group description
                    tools:
                        - example_tool
                    prompts:
                        - code_review`,
			want: `
kind: group
name: my_group
description: my group description
tools:
- example_tool
prompts:
- code_review
`,
		},
		{
			desc: "preserve resource order",
			in: `
            tools:
                example_tool:
                    kind: postgres-sql
                    source: my-pg-instance
                    description: some description
                    statement: SELECT * FROM SQL_STATEMENT;
                    parameters:
                        - name: country
                          type: string
                          description: some description
            sources:
                my-pg-instance:
                    kind: cloud-sql-postgres
                    project: my-project
                    region: my-region
                    instance: my-instance
                    database: my_db
                    user: my_user
                    password: my_pass
            authServices:
                my-google-auth:
                    kind: google
                    clientId: testing-id
            toolsets:
                example_toolset:
                    - example_tool`,
			want: `
kind: tool
name: example_tool
type: postgres-sql
source: my-pg-instance
description: some description
statement: SELECT * FROM SQL_STATEMENT;
parameters:
- name: country
  type: string
  description: some description
---
kind: source
name: my-pg-instance
type: cloud-sql-postgres
project: my-project
region: my-region
instance: my-instance
database: my_db
user: my_user
password: my_pass
---
kind: authService
name: my-google-auth
type: google
clientId: testing-id
---
kind: group
name: example_toolset
tools:
- example_tool
`,
		},
		{
			desc: "convert combination of v1 and v2",
			in: `
            sources:
                my-pg-instance:
                    kind: cloud-sql-postgres
                    project: my-project
                    region: my-region
                    instance: my-instance
                    database: my_db
                    user: my_user
                    password: my_pass
            authServices:
                my-google-auth:
                    kind: google
                    clientId: testing-id
            tools:
                example_tool:
                    kind: postgres-sql
                    source: my-pg-instance
                    description: some description
                    statement: SELECT * FROM SQL_STATEMENT;
                    parameters:
                        - name: country
                          type: string
                          description: some description
            toolsets:
                example_toolset:
                    - example_tool
            prompts:
                code_review:
                    description: ask llm to analyze code quality
                    messages:
                      - content: "please review the following code for quality: {{.code}}"
                    arguments:
                        - name: code
                          description: the code to review
            embeddingModels:
                gemini-model:
                    kind: gemini
                    model: gemini-embedding-001
                    apiKey: some-key
                    dimension: 768
---
            kind: source
            name: my-pg-instance2
            type: cloud-sql-postgres
            project: my-project
            region: my-region
            instance: my-instance
---
            kind: authService
            name: my-google-auth2
            type: google
            clientId: testing-id
---
            kind: tool
            name: example_tool2
            type: postgres-sql
            source: my-pg-instance
            description: some description
            statement: SELECT * FROM SQL_STATEMENT;
            parameters:
            - name: country
              type: string
              description: some description
---
            kind: toolset
            name: example_toolset2
            tools:
            - example_tool
---
            tools:
            - example_tool
            kind: toolset
            name: example_toolset3
---
            kind: prompt
            name: code_review2
            description: ask llm to analyze code quality
            messages:
            - content: "please review the following code for quality: {{.code}}"
            arguments:
            - name: code
              description: the code to review
---
            kind: embeddingModel
            name: gemini-model2
            type: gemini`,
			want: `
kind: source
name: my-pg-instance
type: cloud-sql-postgres
project: my-project
region: my-region
instance: my-instance
database: my_db
user: my_user
password: my_pass
---
kind: authService
name: my-google-auth
type: google
clientId: testing-id
---
kind: tool
name: example_tool
type: postgres-sql
source: my-pg-instance
description: some description
statement: SELECT * FROM SQL_STATEMENT;
parameters:
- name: country
  type: string
  description: some description
---
kind: group
name: example_toolset
tools:
- example_tool
---
kind: prompt
name: code_review
description: ask llm to analyze code quality
messages:
- content: "please review the following code for quality: {{.code}}"
arguments:
- name: code
  description: the code to review
---
kind: embeddingModel
name: gemini-model
type: gemini
model: gemini-embedding-001
apiKey: some-key
dimension: 768
---
kind: source
name: my-pg-instance2
type: cloud-sql-postgres
project: my-project
region: my-region
instance: my-instance
---
kind: authService
name: my-google-auth2
type: google
clientId: testing-id
---
kind: tool
name: example_tool2
type: postgres-sql
source: my-pg-instance
description: some description
statement: SELECT * FROM SQL_STATEMENT;
parameters:
- name: country
  type: string
  description: some description
---
kind: group
name: example_toolset2
tools:
- example_tool
---
tools:
- example_tool
kind: group
name: example_toolset3
---
kind: prompt
name: code_review2
description: ask llm to analyze code quality
messages:
- content: "please review the following code for quality: {{.code}}"
arguments:
- name: code
  description: the code to review
---
kind: embeddingModel
name: gemini-model2
type: gemini
`,
		},
		{
			desc: "flat format only rewrites the toolset kind",
			in: `
kind: source
name: my-pg-instance
type: cloud-sql-postgres
project: my-project
region: my-region
instance: my-instance
database: my_db
user: my_user
password: my_pass
---
kind: tool
name: example_tool
type: postgres-sql
source: my-pg-instance
description: some description
statement: SELECT * FROM SQL_STATEMENT;
parameters:
- name: country
  type: string
  description: some description
---
kind: toolset
name: example_toolset
tools:
- example_tool`,
			want: `
kind: source
name: my-pg-instance
type: cloud-sql-postgres
project: my-project
region: my-region
instance: my-instance
database: my_db
user: my_user
password: my_pass
---
kind: tool
name: example_tool
type: postgres-sql
source: my-pg-instance
description: some description
statement: SELECT * FROM SQL_STATEMENT;
parameters:
- name: country
  type: string
  description: some description
---
kind: group
name: example_toolset
tools:
- example_tool
`,
		},
		{
			desc:   "invalid source",
			in:     `sources: invalid`,
			isErr:  true,
			errStr: `doc 1: invalid config format at key "sources": expected nested format keys and type map`,
		},
		{
			desc:   "invalid toolset",
			in:     `toolsets: invalid`,
			isErr:  true,
			errStr: `doc 1: invalid config format at key "toolsets": expected nested format keys and type map`,
		},
		{
			// Toolsets are emitted as groups, but the error must still name the
			// key the user wrote.
			desc: "invalid toolset entry",
			in: `
            toolsets:
                example_toolset: not-a-list`,
			isErr:  true,
			errStr: `doc 1: invalid config format at key "toolsets": unable to convert entryBody to MapSlice`,
		},
		{
			// A toolset has no description of its own, so converting must not turn
			// one into a group description.
			desc: "convert flat toolset to group drops the description",
			in: `
kind: toolset
name: example_toolset
description: some description
tools:
- example_tool`,
			want: `
kind: group
name: example_toolset
tools:
- example_tool
`,
		},
		{
			// A nested toolset body may be written as a map rather than a bare
			// list, in which case it can carry a description too. Both
			// spellings must produce the same group.
			desc: "convert nested map-form toolset to group drops the description",
			in: `
toolsets:
    example_toolset:
        description: some description
        tools:
        - example_tool`,
			want: `
kind: group
name: example_toolset
tools:
- example_tool
`,
		},
		{
			desc: "convert flat toolset to group with kind not first",
			in: `
tools:
- example_tool
kind: toolset
name: example_toolset`,
			want: `
tools:
- example_tool
kind: group
name: example_toolset
`,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			output, err := ConvertConfig(context.Background(), []byte(tc.in))
			if tc.isErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				if tc.errStr != "" && err.Error() != tc.errStr {
					t.Fatalf("incorrect error string: got %s, want %s", err, tc.errStr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			if diff := cmp.Diff(string(output), tc.want); diff != "" {
				t.Fatalf("incorrect toolsets parse: diff %v", diff)
			}
		})
	}
}

func TestParseConfig(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	tcs := []struct {
		description string
		in          string
		wantConfig  Config
	}{
		{
			description: "basic example config file v1",
			in: `
			sources:
				my-pg-instance:
					kind: cloud-sql-postgres
					project: my-project
					region: my-region
					instance: my-instance
					database: my_db
					user: my_user
					password: my_pass
			tools:
				example_tool:
					kind: postgres-sql
					source: my-pg-instance
					description: some description
					statement: |
						SELECT * FROM SQL_STATEMENT;
					parameters:
						- name: country
							type: string
							description: some description
			toolsets:
				example_toolset:
					- example_tool
			`,
			wantConfig: Config{
				Sources: server.SourceConfigs{
					"my-pg-instance": cloudsqlpgsrc.Config{
						Name:     "my-pg-instance",
						Type:     cloudsqlpgsrc.SourceType,
						Project:  "my-project",
						Region:   "my-region",
						Instance: "my-instance",
						IPType:   "public",
						Database: "my_db",
						User:     "my_user",
						Password: "my_pass",
					},
				},
				Tools: server.ToolConfigs{
					"example_tool": postgressql.Config{
						ConfigBase: tools.ConfigBase{
							Name:         "example_tool",
							Description:  "some description",
							AuthRequired: []string{},
						},
						Type:      "postgres-sql",
						Source:    "my-pg-instance",
						Statement: "SELECT * FROM SQL_STATEMENT;\n",
						Parameters: []parameters.Parameter{
							parameters.NewStringParameter("country", "some description"),
						},
					},
				},
				Groups: server.GroupConfigs{
					"example_toolset": group.GroupConfig{
						Name:      "example_toolset",
						ToolNames: []string{"example_tool"},
					},
				},
				AuthServices: nil,
				Prompts:      nil,
			},
		},
		{
			description: "basic example config file v2",
			in: `
			kind: source
			name: my-pg-instance
			type: cloud-sql-postgres
			project: my-project
			region: my-region
			instance: my-instance
			database: my_db
			user: my_user
			password: my_pass
---
			kind: authService
			name: my-google-auth
			type: google
			clientId: testing-id
---
      kind: authService
      name: my-generic-auth
      type: generic
      audience: testings
      authorizationServer: https://testings
      mcpEnabled: true
      scopesRequired:
        - read:files
        - write:files
---
      kind: embeddingModel
      name: gemini-model
      type: gemini
      model: gemini-embedding-001
      apiKey: some-key
      dimension: 768
---
      kind: tool
      name: example_tool
      type: postgres-sql
      source: my-pg-instance
      description: some description
      statement: |
        SELECT * FROM SQL_STATEMENT;
      parameters:
      - name: country
        type: string
        description: some description
---
      kind: toolset
      name: example_toolset
      tools:
      - example_tool
---
      kind: prompt
      name: code_review
      description: ask llm to analyze code quality
      messages:
      - content: "please review the following code for quality: {{.code}}"
      arguments:
      - name: code
        description: the code to review
			`,
			wantConfig: Config{
				Sources: server.SourceConfigs{
					"my-pg-instance": cloudsqlpgsrc.Config{
						Name:     "my-pg-instance",
						Type:     cloudsqlpgsrc.SourceType,
						Project:  "my-project",
						Region:   "my-region",
						Instance: "my-instance",
						IPType:   "public",
						Database: "my_db",
						User:     "my_user",
						Password: "my_pass",
					},
				},
				AuthServices: server.AuthServiceConfigs{
					"my-google-auth": google.Config{
						Name:     "my-google-auth",
						Type:     google.AuthServiceType,
						ClientID: "testing-id",
					},
					"my-generic-auth": generic.Config{
						Name:                "my-generic-auth",
						Type:                generic.AuthServiceType,
						Audience:            "testings",
						McpEnabled:          true,
						AuthorizationServer: "https://testings",
						ScopesRequired:      []string{"read:files", "write:files"},
					},
				},
				EmbeddingModels: server.EmbeddingModelConfigs{
					"gemini-model": gemini.Config{
						Name:      "gemini-model",
						Type:      gemini.EmbeddingModelType,
						Model:     "gemini-embedding-001",
						ApiKey:    "some-key",
						Dimension: 768,
					},
				},
				Tools: server.ToolConfigs{
					"example_tool": postgressql.Config{
						ConfigBase: tools.ConfigBase{
							Name:         "example_tool",
							Description:  "some description",
							AuthRequired: []string{},
						},
						Type:      "postgres-sql",
						Source:    "my-pg-instance",
						Statement: "SELECT * FROM SQL_STATEMENT;\n",
						Parameters: []parameters.Parameter{
							parameters.NewStringParameter("country", "some description"),
						},
					},
				},
				Groups: server.GroupConfigs{
					"example_toolset": group.GroupConfig{
						Name:      "example_toolset",
						ToolNames: []string{"example_tool"},
					},
				},
				Prompts: server.PromptConfigs{
					"code_review": &custom.Config{
						Name:        "code_review",
						Description: "ask llm to analyze code quality",
						Arguments: prompts.Arguments{
							{Parameter: parameters.NewStringParameter("code", "the code to review")},
						},
						Messages: []prompts.Message{
							{Role: "user", Content: "please review the following code for quality: {{.code}}"},
						},
					},
				},
			},
		},
		{
			description: "only prompts",
			in: `
            kind: prompt
            name: my-prompt
            description: A prompt template for data analysis.
            arguments:
                - name: country
                  description: The country to analyze.
            messages:
                - content: Analyze the data for {{.country}}.
            `,
			wantConfig: Config{
				Sources:      nil,
				AuthServices: nil,
				Tools:        nil,
				Groups:       nil,
				Prompts: server.PromptConfigs{
					"my-prompt": &custom.Config{
						Name:        "my-prompt",
						Description: "A prompt template for data analysis.",
						Arguments: prompts.Arguments{
							{Parameter: parameters.NewStringParameter("country", "The country to analyze.")},
						},
						Messages: []prompts.Message{
							{Role: "user", Content: "Analyze the data for {{.country}}."},
						},
					},
				},
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.description, func(t *testing.T) {
			parser := ConfigParser{}
			configFile, err := parser.ParseConfig(ctx, testutils.FormatYaml(tc.in))
			if err != nil {
				t.Fatalf("failed to parse input: %v", err)
			}
			if diff := cmp.Diff(tc.wantConfig.Sources, configFile.Sources); diff != "" {
				t.Fatalf("incorrect sources parse: diff %v", diff)
			}
			if diff := cmp.Diff(tc.wantConfig.AuthServices, configFile.AuthServices); diff != "" {
				t.Fatalf("incorrect authServices parse: diff %v", diff)
			}
			if diff := cmp.Diff(tc.wantConfig.Tools, configFile.Tools); diff != "" {
				t.Fatalf("incorrect tools parse: diff %v", diff)
			}
			if diff := cmp.Diff(tc.wantConfig.Groups, configFile.Groups); diff != "" {
				t.Fatalf("incorrect toolsets parse: diff %v", diff)
			}
			if diff := cmp.Diff(tc.wantConfig.Prompts, configFile.Prompts); diff != "" {
				t.Fatalf("incorrect prompts parse: diff %v", diff)
			}
		})
	}
}

func TestParseConfigFailure(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	tcs := []struct {
		description string
		in          string
		wantError   string
	}{
		{
			description: "invalid special character in tool name",
			in: `
			kind: source
			name: my-pg-instance
			type: cloud-sql-postgres
			project: my-project
			region: my-region
			instance: my-instance
			database: my_db
			user: my_user
			password: my_pass
---
			kind: tool
			name: invalid_toolname_?
			type: postgres-sql
			source: my-pg-instance
			description: some description
			statement: SELECT *;
			`,
			wantError: "invalid character for resource name; only uppercase and lowercase ASCII letters (A-Z, a-z), digits (0-9), underscore (_), hyphen (-), and dot (.) is allowed",
		},
		{
			description: "invalid comma in tool name",
			in: `
			kind: source
			name: my-pg-instance
			type: cloud-sql-postgres
			project: my-project
			region: my-region
			instance: my-instance
			database: my_db
			user: my_user
			password: my_pass
---
			kind: tool
			name: invalid_toolname,
			type: postgres-sql
			source: my-pg-instance
			description: some description
			statement: SELECT *;
			`,
			wantError: "invalid character for resource name; only uppercase and lowercase ASCII letters (A-Z, a-z), digits (0-9), underscore (_), hyphen (-), and dot (.) is allowed",
		},
		{
			// Toolsets are parsed as groups, so unknown fields are now rejected by
			// the group's strict decoder instead of being silently dropped.
			description: "unknown field in toolset",
			in: `
			kind: toolset
			name: my_toolset
			tools:
			- example_tool
			descriptoin: typo
			`,
			wantError: "unable to unmarshal group",
		},
		{
			// Toolsets are parsed as groups, so a toolset and a group of the same
			// name now collide instead of the group silently winning.
			description: "toolset and group share a name",
			in: `
			kind: toolset
			name: my_collection
			tools:
			- example_tool
---
			kind: group
			name: my_collection
			tools:
			- example_tool
			`,
			wantError: `group "my_collection" declared more than once`,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.description, func(t *testing.T) {
			parser := ConfigParser{}
			_, err := parser.ParseConfig(ctx, testutils.FormatYaml(tc.in))
			if err == nil {
				t.Fatalf("expected error containing %s, got nil", tc.wantError)
			}
			if !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("want error: %s, got error: %s", tc.wantError, err.Error())
			}
		})
	}
}

func TestParseConfigWithAuth(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	tcs := []struct {
		description string
		in          string
		wantConfig  Config
	}{
		{
			description: "basic example",
			in: `
			kind: source
			name: my-pg-instance
			type: cloud-sql-postgres
			project: my-project
			region: my-region
			instance: my-instance
			database: my_db
			user: my_user
			password: my_pass
---
			kind: authService
			name: my-google-service
			type: google
			clientId: my-client-id
---
			kind: authService
			name: other-google-service
			type: google
			clientId: other-client-id
---
			kind: tool
			name: example_tool
			type: postgres-sql
			source: my-pg-instance
			description: some description
			statement: |
				SELECT * FROM SQL_STATEMENT;
			parameters:
				- name: country
					type: string
					description: some description
				- name: id
					type: integer
					description: user id
					authServices:
					- name: my-google-service
						field: user_id
				- name: email
					type: string
					description: user email
					authServices:
					- name: my-google-service
						field: email
					- name: other-google-service
						field: other_email
---
			kind: toolset
			name: example_toolset
			tools:
				- example_tool
			`,
			wantConfig: Config{
				Sources: server.SourceConfigs{
					"my-pg-instance": cloudsqlpgsrc.Config{
						Name:     "my-pg-instance",
						Type:     cloudsqlpgsrc.SourceType,
						Project:  "my-project",
						Region:   "my-region",
						Instance: "my-instance",
						IPType:   "public",
						Database: "my_db",
						User:     "my_user",
						Password: "my_pass",
					},
				},
				AuthServices: server.AuthServiceConfigs{
					"my-google-service": google.Config{
						Name:     "my-google-service",
						Type:     google.AuthServiceType,
						ClientID: "my-client-id",
					},
					"other-google-service": google.Config{
						Name:     "other-google-service",
						Type:     google.AuthServiceType,
						ClientID: "other-client-id",
					},
				},
				Tools: server.ToolConfigs{
					"example_tool": postgressql.Config{
						ConfigBase: tools.ConfigBase{
							Name:         "example_tool",
							Description:  "some description",
							AuthRequired: []string{},
						},
						Type:      "postgres-sql",
						Source:    "my-pg-instance",
						Statement: "SELECT * FROM SQL_STATEMENT;\n",
						Parameters: []parameters.Parameter{
							parameters.NewStringParameter("country", "some description"),
							parameters.NewIntParameter("id", "user id", parameters.WithIntAuth([]parameters.ParamAuthService{{Name: "my-google-service", Field: "user_id"}})),
							parameters.NewStringParameter("email", "user email", parameters.WithStringAuth([]parameters.ParamAuthService{{Name: "my-google-service", Field: "email"}, {Name: "other-google-service", Field: "other_email"}})),
						},
					},
				},
				Groups: server.GroupConfigs{
					"example_toolset": group.GroupConfig{
						Name:      "example_toolset",
						ToolNames: []string{"example_tool"},
					},
				},
				Prompts: nil,
			},
		},
		{
			description: "basic example with authRequired",
			in: `
			kind: source
			name: my-pg-instance
			type: cloud-sql-postgres
			project: my-project
			region: my-region
			instance: my-instance
			database: my_db
			user: my_user
			password: my_pass
---
			kind: authService
			name: my-google-service
			type: google
			clientId: my-client-id
---
			kind: authService
			name: other-google-service
			type: google
			clientId: other-client-id
---
			kind: tool
			name: example_tool
			type: postgres-sql
			source: my-pg-instance
			description: some description
			statement: |
				SELECT * FROM SQL_STATEMENT;
			authRequired:
				- my-google-service
			parameters:
				- name: country
					type: string
					description: some description
				- name: id
					type: integer
					description: user id
					authServices:
					- name: my-google-service
						field: user_id
				- name: email
					type: string
					description: user email
					authServices:
					- name: my-google-service
						field: email
					- name: other-google-service
						field: other_email
---
			kind: toolset
			name: example_toolset
			tools:
				- example_tool
			`,
			wantConfig: Config{
				Sources: server.SourceConfigs{
					"my-pg-instance": cloudsqlpgsrc.Config{
						Name:     "my-pg-instance",
						Type:     cloudsqlpgsrc.SourceType,
						Project:  "my-project",
						Region:   "my-region",
						Instance: "my-instance",
						IPType:   "public",
						Database: "my_db",
						User:     "my_user",
						Password: "my_pass",
					},
				},
				AuthServices: server.AuthServiceConfigs{
					"my-google-service": google.Config{
						Name:     "my-google-service",
						Type:     google.AuthServiceType,
						ClientID: "my-client-id",
					},
					"other-google-service": google.Config{
						Name:     "other-google-service",
						Type:     google.AuthServiceType,
						ClientID: "other-client-id",
					},
				},
				Tools: server.ToolConfigs{
					"example_tool": postgressql.Config{
						ConfigBase: tools.ConfigBase{
							Name:         "example_tool",
							Description:  "some description",
							AuthRequired: []string{"my-google-service"},
						},
						Type:      "postgres-sql",
						Source:    "my-pg-instance",
						Statement: "SELECT * FROM SQL_STATEMENT;\n",
						Parameters: []parameters.Parameter{
							parameters.NewStringParameter("country", "some description"),
							parameters.NewIntParameter("id", "user id", parameters.WithIntAuth([]parameters.ParamAuthService{{Name: "my-google-service", Field: "user_id"}})),
							parameters.NewStringParameter("email", "user email", parameters.WithStringAuth([]parameters.ParamAuthService{{Name: "my-google-service", Field: "email"}, {Name: "other-google-service", Field: "other_email"}})),
						},
					},
				},
				Groups: server.GroupConfigs{
					"example_toolset": group.GroupConfig{
						Name:      "example_toolset",
						ToolNames: []string{"example_tool"},
					},
				},
				Prompts: nil,
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.description, func(t *testing.T) {
			parser := ConfigParser{}
			configFile, err := parser.ParseConfig(ctx, testutils.FormatYaml(tc.in))
			if err != nil {
				t.Fatalf("failed to parse input: %v", err)
			}
			if diff := cmp.Diff(tc.wantConfig.Sources, configFile.Sources); diff != "" {
				t.Fatalf("incorrect sources parse: diff %v", diff)
			}
			if diff := cmp.Diff(tc.wantConfig.AuthServices, configFile.AuthServices); diff != "" {
				t.Fatalf("incorrect authServices parse: diff %v", diff)
			}
			if diff := cmp.Diff(tc.wantConfig.Tools, configFile.Tools); diff != "" {
				t.Fatalf("incorrect tools parse: diff %v", diff)
			}
			if diff := cmp.Diff(tc.wantConfig.Groups, configFile.Groups); diff != "" {
				t.Fatalf("incorrect toolsets parse: diff %v", diff)
			}
			if diff := cmp.Diff(tc.wantConfig.Prompts, configFile.Prompts); diff != "" {
				t.Fatalf("incorrect prompts parse: diff %v", diff)
			}
		})
	}

}

func TestEnvVarReplacement(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	t.Setenv("TestHeader", "ACTUAL_HEADER")
	t.Setenv("API_KEY", "ACTUAL_API_KEY")
	t.Setenv("clientId", "ACTUAL_CLIENT_ID")
	t.Setenv("clientId2", "ACTUAL_CLIENT_ID_2")
	t.Setenv("toolset_name", "ACTUAL_TOOLSET_NAME")
	t.Setenv("cat_string", "cat")
	t.Setenv("food_string", "food")
	t.Setenv("TestHeader", "ACTUAL_HEADER")
	t.Setenv("prompt_name", "ACTUAL_PROMPT_NAME")
	t.Setenv("prompt_content", "ACTUAL_CONTENT")

	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	tcs := []struct {
		description string
		in          string
		wantConfig  Config
	}{
		{
			description: "file with env var example",
			in: `
			sources:
				my-http-instance:
					kind: http
					baseUrl: http://test_server/
					timeout: 10s
					headers:
						Authorization: ${TestHeader}
					queryParams:
						api-key: ${API_KEY}
			authServices:
				my-google-service:
					kind: google
					clientId: ${clientId}
				other-google-service:
					kind: google
					clientId: ${clientId2}

			tools:
				example_tool:
					kind: http
					source: my-instance
					method: GET
					path: "search?name=alice&pet=${cat_string}"
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
								"city": "{{.city}}",
								"food": "${food_string}",
								"other": "$OTHER"
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

			toolsets:
				${toolset_name}:
					- example_tool


			prompts:
				${prompt_name}:
					description: A test prompt for {{.name}}.
					messages:
						- role: user
						  content: ${prompt_content}
			`,
			wantConfig: Config{
				Sources: server.SourceConfigs{
					"my-http-instance": httpsrc.Config{
						Name:           "my-http-instance",
						Type:           httpsrc.SourceType,
						BaseURL:        "http://test_server/",
						Timeout:        "10s",
						DefaultHeaders: map[string]string{"Authorization": "ACTUAL_HEADER"},
						QueryParams:    map[string]string{"api-key": "ACTUAL_API_KEY"},
					},
				},
				AuthServices: server.AuthServiceConfigs{
					"my-google-service": google.Config{
						Name:     "my-google-service",
						Type:     google.AuthServiceType,
						ClientID: "ACTUAL_CLIENT_ID",
					},
					"other-google-service": google.Config{
						Name:     "other-google-service",
						Type:     google.AuthServiceType,
						ClientID: "ACTUAL_CLIENT_ID_2",
					},
				},
				Tools: server.ToolConfigs{
					"example_tool": http.Config{
						ConfigBase: tools.ConfigBase{
							Name:         "example_tool",
							Description:  "some description",
							AuthRequired: []string{"my-google-auth-service", "other-auth-service"},
						},
						Type:   "http",
						Source: "my-instance",
						Method: "GET",
						Path:   "search?name=alice&pet=cat",
						QueryParams: []parameters.Parameter{
							parameters.NewStringParameter("country", "some description", parameters.WithStringAuth(
								[]parameters.ParamAuthService{{Name: "my-google-auth-service", Field: "user_id"},
									{Name: "other-auth-service", Field: "user_id"}})),
						},
						RequestBody: `{
  "age": {{.age}},
  "city": "{{.city}}",
  "food": "food",
  "other": "$OTHER"
}
`,
						BodyParams:   []parameters.Parameter{parameters.NewIntParameter("age", "age num"), parameters.NewStringParameter("city", "city string")},
						Headers:      map[string]string{"Authorization": "API_KEY", "Content-Type": "application/json"},
						HeaderParams: []parameters.Parameter{parameters.NewStringParameter("Language", "language string")},
					},
				},
				Groups: server.GroupConfigs{
					"ACTUAL_TOOLSET_NAME": group.GroupConfig{
						Name:      "ACTUAL_TOOLSET_NAME",
						ToolNames: []string{"example_tool"},
					},
				},
				Prompts: server.PromptConfigs{
					"ACTUAL_PROMPT_NAME": &custom.Config{
						Name:        "ACTUAL_PROMPT_NAME",
						Description: "A test prompt for {{.name}}.",
						Messages: []prompts.Message{
							{
								Role:    "user",
								Content: "ACTUAL_CONTENT",
							},
						},
						Arguments: nil,
					},
				},
			},
		},
		{
			description: "file with env var example configFile v2",
			in: `
			kind: source
			name: my-http-instance
			type: http
			baseUrl: http://test_server/
			timeout: 10s
			headers:
				Authorization: ${TestHeader}
			queryParams:
				api-key: ${API_KEY}
---
			kind: authService
			name: my-google-service
			type: google
			clientId: ${clientId}
---
			kind: authService
			name: other-google-service
			type: google
			clientId: ${clientId2}
---
			kind: tool
			name: example_tool
			type: http
			source: my-instance
			method: GET
			path: "search?name=alice&pet=${cat_string}"
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
						"city": "{{.city}}",
						"food": "${food_string}",
						"other": "$OTHER"
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
---
			kind: toolset
			name: ${toolset_name}
			tools:
				- example_tool
---
			kind: prompt
			name: ${prompt_name}
			description: A test prompt for {{.name}}.
			messages:
				- role: user
					content: ${prompt_content}
			`,
			wantConfig: Config{
				Sources: server.SourceConfigs{
					"my-http-instance": httpsrc.Config{
						Name:           "my-http-instance",
						Type:           httpsrc.SourceType,
						BaseURL:        "http://test_server/",
						Timeout:        "10s",
						DefaultHeaders: map[string]string{"Authorization": "ACTUAL_HEADER"},
						QueryParams:    map[string]string{"api-key": "ACTUAL_API_KEY"},
					},
				},
				AuthServices: server.AuthServiceConfigs{
					"my-google-service": google.Config{
						Name:     "my-google-service",
						Type:     google.AuthServiceType,
						ClientID: "ACTUAL_CLIENT_ID",
					},
					"other-google-service": google.Config{
						Name:     "other-google-service",
						Type:     google.AuthServiceType,
						ClientID: "ACTUAL_CLIENT_ID_2",
					},
				},
				Tools: server.ToolConfigs{
					"example_tool": http.Config{
						ConfigBase: tools.ConfigBase{
							Name:         "example_tool",
							Description:  "some description",
							AuthRequired: []string{"my-google-auth-service", "other-auth-service"},
						},
						Type:   "http",
						Source: "my-instance",
						Method: "GET",
						Path:   "search?name=alice&pet=cat",
						QueryParams: []parameters.Parameter{
							parameters.NewStringParameter("country", "some description", parameters.WithStringAuth(
								[]parameters.ParamAuthService{{Name: "my-google-auth-service", Field: "user_id"},
									{Name: "other-auth-service", Field: "user_id"}})),
						},
						RequestBody: `{
  "age": {{.age}},
  "city": "{{.city}}",
  "food": "food",
  "other": "$OTHER"
}
`,
						BodyParams:   []parameters.Parameter{parameters.NewIntParameter("age", "age num"), parameters.NewStringParameter("city", "city string")},
						Headers:      map[string]string{"Authorization": "API_KEY", "Content-Type": "application/json"},
						HeaderParams: []parameters.Parameter{parameters.NewStringParameter("Language", "language string")},
					},
				},
				Groups: server.GroupConfigs{
					"ACTUAL_TOOLSET_NAME": group.GroupConfig{
						Name:      "ACTUAL_TOOLSET_NAME",
						ToolNames: []string{"example_tool"},
					},
				},
				Prompts: server.PromptConfigs{
					"ACTUAL_PROMPT_NAME": &custom.Config{
						Name:        "ACTUAL_PROMPT_NAME",
						Description: "A test prompt for {{.name}}.",
						Messages: []prompts.Message{
							{
								Role:    "user",
								Content: "ACTUAL_CONTENT",
							},
						},
						Arguments: nil,
					},
				},
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.description, func(t *testing.T) {
			parser := ConfigParser{}
			configFile, err := parser.ParseConfig(ctx, testutils.FormatYaml(tc.in))
			if err != nil {
				t.Fatalf("failed to parse input: %v", err)
			}
			if diff := cmp.Diff(tc.wantConfig.Sources, configFile.Sources); diff != "" {
				t.Fatalf("incorrect sources parse: diff %v", diff)
			}
			if diff := cmp.Diff(tc.wantConfig.AuthServices, configFile.AuthServices); diff != "" {
				t.Fatalf("incorrect authServices parse: diff %v", diff)
			}
			if diff := cmp.Diff(tc.wantConfig.Tools, configFile.Tools); diff != "" {
				t.Fatalf("incorrect tools parse: diff %v", diff)
			}
			if diff := cmp.Diff(tc.wantConfig.Groups, configFile.Groups); diff != "" {
				t.Fatalf("incorrect toolsets parse: diff %v", diff)
			}
			if diff := cmp.Diff(tc.wantConfig.Prompts, configFile.Prompts); diff != "" {
				t.Fatalf("incorrect prompts parse: diff %v", diff)
			}
		})
	}

}

func TestPrebuiltTools(t *testing.T) {
	// Get prebuilt configs
	alloydb_omni_config, _ := prebuiltconfigs.Get("alloydb-omni")
	alloydb_admin_config, _ := prebuiltconfigs.Get("alloydb-postgres-admin")
	alloydb_config, _ := prebuiltconfigs.Get("alloydb-postgres")
	alloydbobsvconfig, _ := prebuiltconfigs.Get("alloydb-postgres-observability")
	bigquery_config, _ := prebuiltconfigs.Get("bigquery")
	clickhouse_config, _ := prebuiltconfigs.Get("clickhouse")
	cloudhealthcare_config, _ := prebuiltconfigs.Get("cloud-healthcare")
	cloudsqlmssql_config, _ := prebuiltconfigs.Get("cloud-sql-mssql")
	cloudsqlmssql_admin_config, _ := prebuiltconfigs.Get("cloud-sql-mssql-admin")
	cloudsqlmssqlobsvconfig, _ := prebuiltconfigs.Get("cloud-sql-mssql-observability")
	cloudsqlmysql_config, _ := prebuiltconfigs.Get("cloud-sql-mysql")
	cloudsqlmysql_admin_config, _ := prebuiltconfigs.Get("cloud-sql-mysql-admin")
	cloudsqlmysqlobsvconfig, _ := prebuiltconfigs.Get("cloud-sql-mysql-observability")
	cloudsqlpg_config, _ := prebuiltconfigs.Get("cloud-sql-postgres")
	cloudsqlpg_admin_config, _ := prebuiltconfigs.Get("cloud-sql-postgres-admin")
	cloudsqlpgobsvconfig, _ := prebuiltconfigs.Get("cloud-sql-postgres-observability")
	conversationalanalytics_config, _ := prebuiltconfigs.Get("conversational-analytics-with-data-agent")
	dataplex_config, _ := prebuiltconfigs.Get("dataplex")
	dataproc_config, _ := prebuiltconfigs.Get("dataproc")
	elasticsearch_config, _ := prebuiltconfigs.Get("elasticsearch")
	firestoreconfig, _ := prebuiltconfigs.Get("firestore")
	looker_config, _ := prebuiltconfigs.Get("looker")
	looker_dev_config, _ := prebuiltconfigs.Get("looker-dev")
	lookerca_config, _ := prebuiltconfigs.Get("looker-conversational-analytics")
	mindsdb_config, _ := prebuiltconfigs.Get("mindsdb")
	mssql_config, _ := prebuiltconfigs.Get("mssql")
	mysql_config, _ := prebuiltconfigs.Get("mysql")
	neo4jconfig, _ := prebuiltconfigs.Get("neo4j")
	oceanbase_config, _ := prebuiltconfigs.Get("oceanbase")
	oracle_config, _ := prebuiltconfigs.Get("oracledb")
	postgresconfig, _ := prebuiltconfigs.Get("postgres")
	serverless_spark_config, _ := prebuiltconfigs.Get("serverless-spark")
	cloudstorage_config, _ := prebuiltconfigs.Get("cloud-storage")
	singlestore_config, _ := prebuiltconfigs.Get("singlestore")
	snowflake_config, _ := prebuiltconfigs.Get("snowflake")
	spanner_config, _ := prebuiltconfigs.Get("spanner")
	spannerpg_config, _ := prebuiltconfigs.Get("spanner-postgres")
	sqlite_config, _ := prebuiltconfigs.Get("sqlite")

	// Set environment variables
	t.Setenv("API_KEY", "your_api_key")

	t.Setenv("BIGQUERY_PROJECT", "your_gcp_project_id")
	t.Setenv("DATAPLEX_PROJECT", "your_gcp_project_id")
	t.Setenv("FIRESTORE_PROJECT", "your_gcp_project_id")
	t.Setenv("FIRESTORE_DATABASE", "your_firestore_db_name")

	t.Setenv("SPANNER_PROJECT", "your_gcp_project_id")
	t.Setenv("SPANNER_INSTANCE", "your_spanner_instance")
	t.Setenv("SPANNER_DATABASE", "your_spanner_db")

	t.Setenv("ALLOYDB_POSTGRES_PROJECT", "your_gcp_project_id")
	t.Setenv("ALLOYDB_POSTGRES_REGION", "your_gcp_region")
	t.Setenv("ALLOYDB_POSTGRES_CLUSTER", "your_alloydb_cluster")
	t.Setenv("ALLOYDB_POSTGRES_INSTANCE", "your_alloydb_instance")
	t.Setenv("ALLOYDB_POSTGRES_DATABASE", "your_alloydb_db")
	t.Setenv("ALLOYDB_POSTGRES_USER", "your_alloydb_user")
	t.Setenv("ALLOYDB_POSTGRES_PASSWORD", "your_alloydb_password")

	t.Setenv("ALLOYDB_OMNI_HOST", "localhost")
	t.Setenv("ALLOYDB_OMNI_PORT", "5432")
	t.Setenv("ALLOYDB_OMNI_DATABASE", "your_alloydb_db")
	t.Setenv("ALLOYDB_OMNI_USER", "your_alloydb_user")
	t.Setenv("ALLOYDB_OMNI_PASSWORD", "your_alloydb_password")

	t.Setenv("CLICKHOUSE_PROTOCOL", "your_clickhouse_protocol")
	t.Setenv("CLICKHOUSE_DATABASE", "your_clickhouse_database")
	t.Setenv("CLICKHOUSE_PASSWORD", "your_clickhouse_password")
	t.Setenv("CLICKHOUSE_USER", "your_clickhouse_user")
	t.Setenv("CLICKHOUSE_HOST", "your_clickhosue_host")
	t.Setenv("CLICKHOUSE_PORT", "8123")

	t.Setenv("CLOUD_SQL_POSTGRES_PROJECT", "your_pg_project")
	t.Setenv("CLOUD_SQL_POSTGRES_INSTANCE", "your_pg_instance")
	t.Setenv("CLOUD_SQL_POSTGRES_DATABASE", "your_pg_db")
	t.Setenv("CLOUD_SQL_POSTGRES_REGION", "your_pg_region")
	t.Setenv("CLOUD_SQL_POSTGRES_USER", "your_pg_user")
	t.Setenv("CLOUD_SQL_POSTGRES_PASS", "your_pg_pass")

	t.Setenv("CLOUD_SQL_MYSQL_PROJECT", "your_gcp_project_id")
	t.Setenv("CLOUD_SQL_MYSQL_REGION", "your_gcp_region")
	t.Setenv("CLOUD_SQL_MYSQL_INSTANCE", "your_instance")
	t.Setenv("CLOUD_SQL_MYSQL_DATABASE", "your_cloudsql_mysql_db")
	t.Setenv("CLOUD_SQL_MYSQL_USER", "your_cloudsql_mysql_user")
	t.Setenv("CLOUD_SQL_MYSQL_PASSWORD", "your_cloudsql_mysql_password")

	t.Setenv("CLOUD_SQL_MSSQL_PROJECT", "your_gcp_project_id")
	t.Setenv("CLOUD_SQL_MSSQL_REGION", "your_gcp_region")
	t.Setenv("CLOUD_SQL_MSSQL_INSTANCE", "your_cloudsql_mssql_instance")
	t.Setenv("CLOUD_SQL_MSSQL_DATABASE", "your_cloudsql_mssql_db")
	t.Setenv("CLOUD_SQL_MSSQL_IP_ADDRESS", "127.0.0.1")
	t.Setenv("CLOUD_SQL_MSSQL_USER", "your_cloudsql_mssql_user")
	t.Setenv("CLOUD_SQL_MSSQL_PASSWORD", "your_cloudsql_mssql_password")
	t.Setenv("CLOUD_SQL_POSTGRES_PASSWORD", "your_cloudsql_pg_password")

	t.Setenv("CLOUD_GDA_PROJECT", "your_gcp_project_id")

	t.Setenv("ELASTICSEARCH_HOST", "your_elasticsearch_host")
	t.Setenv("ELASTICSEARCH_APIKEY", "your_api_key")

	t.Setenv("SERVERLESS_SPARK_PROJECT", "your_gcp_project_id")
	t.Setenv("SERVERLESS_SPARK_LOCATION", "your_gcp_location")

	t.Setenv("DATAPROC_PROJECT", "your_gcp_project_id")
	t.Setenv("DATAPROC_REGION", "your_gcp_location")

	t.Setenv("POSTGRES_HOST", "localhost")
	t.Setenv("POSTGRES_PORT", "5432")
	t.Setenv("POSTGRES_DATABASE", "your_postgres_db")
	t.Setenv("POSTGRES_USER", "your_postgres_user")
	t.Setenv("POSTGRES_PASSWORD", "your_postgres_password")

	t.Setenv("MYSQL_HOST", "localhost")
	t.Setenv("MYSQL_PORT", "3306")
	t.Setenv("MYSQL_DATABASE", "your_mysql_db")
	t.Setenv("MYSQL_USER", "your_mysql_user")
	t.Setenv("MYSQL_PASSWORD", "your_mysql_password")

	t.Setenv("MSSQL_HOST", "localhost")
	t.Setenv("MSSQL_PORT", "1433")
	t.Setenv("MSSQL_DATABASE", "your_mssql_db")
	t.Setenv("MSSQL_USER", "your_mssql_user")
	t.Setenv("MSSQL_PASSWORD", "your_mssql_password")

	t.Setenv("MINDSDB_HOST", "localhost")
	t.Setenv("MINDSDB_PORT", "47334")
	t.Setenv("MINDSDB_DATABASE", "your_mindsdb_db")
	t.Setenv("MINDSDB_USER", "your_mindsdb_user")
	t.Setenv("MINDSDB_PASS", "your_mindsdb_password")

	t.Setenv("LOOKER_BASE_URL", "https://your_company.looker.com")
	t.Setenv("LOOKER_CLIENT_ID", "your_looker_client_id")
	t.Setenv("LOOKER_CLIENT_SECRET", "your_looker_client_secret")
	t.Setenv("LOOKER_VERIFY_SSL", "true")

	t.Setenv("LOOKER_PROJECT", "your_project_id")
	t.Setenv("LOOKER_LOCATION", "us")

	t.Setenv("SQLITE_DATABASE", "test.db")

	t.Setenv("NEO4J_URI", "bolt://localhost:7687")
	t.Setenv("NEO4J_DATABASE", "neo4j")
	t.Setenv("NEO4J_USERNAME", "your_neo4j_user")
	t.Setenv("NEO4J_PASSWORD", "your_neo4j_password")

	t.Setenv("CLOUD_HEALTHCARE_PROJECT", "your_gcp_project_id")
	t.Setenv("CLOUD_HEALTHCARE_REGION", "your_gcp_region")
	t.Setenv("CLOUD_HEALTHCARE_DATASET", "your_healthcare_dataset")

	t.Setenv("CLOUD_STORAGE_PROJECT", "your_gcp_project_id")

	t.Setenv("SNOWFLAKE_ACCOUNT", "your_account")
	t.Setenv("SNOWFLAKE_USER", "your_username")
	t.Setenv("SNOWFLAKE_PASSWORD", "your_pass")
	t.Setenv("SNOWFLAKE_DATABASE", "your_db")
	t.Setenv("SNOWFLAKE_SCHEMA", "your_schema")
	t.Setenv("SNOWFLAKE_WAREHOUSE", "your_wh")
	t.Setenv("SNOWFLAKE_ROLE", "your_role")

	t.Setenv("ORACLE_USERNAME", "your_oracle_db_username")
	t.Setenv("ORACLE_CONNECTION_STRING", "your_oracle_connection_string")
	t.Setenv("ORACLE_PASSWORD", "your_oracle_db_password")
	t.Setenv("ORACLE_HOST", "your_oracle_db_host")
	t.Setenv("ORACLE_PORT", "your_oracle_db_port")
	t.Setenv("ORACLE_USE_OCI", "false")
	t.Setenv("ORACLE_WALLET", "your_path_to_oracldb_wallet")
	t.Setenv("ORACLE_TNS_ADMIN", "your_path_to_tns_admin")

	t.Setenv("OCEANBASE_HOST", "your_oceanbase_host")
	t.Setenv("OCEANBASE_PORT", "your_oceanbase_port")
	t.Setenv("OCEANBASE_DATABASE", "your_oceanbase_db")
	t.Setenv("OCEANBASE_USER", "your_oceanbase_user")
	t.Setenv("OCEANBASE_PASSWORD", "your_oceanbase_pass")

	t.Setenv("SINGLESTORE_HOST", "your_singlestore_host")
	t.Setenv("SINGLESTORE_PORT", "your_singlestore_port")
	t.Setenv("SINGLESTORE_DATABASE", "your_singlestore_db")
	t.Setenv("SINGLESTORE_USER", "your_singlestore_user")
	t.Setenv("SINGLESTORE_PASSWORD", "your_singlestore_pass")

	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	tcs := []struct {
		name       string
		in         []byte
		wantGroups server.GroupConfigs
	}{
		{
			name: "alloydb omni prebuilt tools",
			in:   alloydb_omni_config,
			wantGroups: server.GroupConfigs{
				"data": group.GroupConfig{
					Name:        "data",
					Description: "Use these skills when you need to explore the database structure, identify schema objects like views and triggers, and execute SQL queries to interact with your data.",
					ToolNames:   []string{"execute_sql", "list_tables", "list_views", "list_schemas", "list_triggers", "list_indexes", "list_sequences", "list_stored_procedure"},
				},
				"performance": group.GroupConfig{
					Name:        "performance",
					Description: "Use these skills when you need to analyze query performance, generate execution plans, check table/column statistics, and monitor overall database activity.",
					ToolNames:   []string{"execute_sql", "get_query_plan", "list_query_stats", "get_column_cardinality", "list_table_stats", "list_database_stats", "list_active_queries"},
				},
				"monitor": group.GroupConfig{
					Name:        "monitor",
					Description: "Use these skills when you need to troubleshoot production issues by identifying locks, tracking long-running transactions, and getting a high-level view of server state.",
					ToolNames:   []string{"database_overview", "list_active_queries", "long_running_transactions", "list_locks", "list_database_stats", "list_pg_settings"},
				},
				"optimize": group.GroupConfig{
					Name:        "optimize",
					Description: "Use these skills when you need to fine-tune the database engine settings, manage extensions, or optimize the columnar engine for better analytical performance.",
					ToolNames:   []string{"list_pg_settings", "list_memory_configurations", "list_available_extensions", "list_installed_extensions", "list_autovacuum_configurations", "list_columnar_configurations", "list_columnar_recommended_columns"},
				},
				"health": group.GroupConfig{
					Name:        "health",
					Description: "Use these skills when you need to audit database health, identify storage bloat, find broken indexes, and verify tablespace or maintenance configurations.",
					ToolNames:   []string{"list_top_bloated_tables", "list_invalid_indexes", "list_table_stats", "list_tablespaces", "database_overview", "list_autovacuum_configurations"},
				},
				"replication": group.GroupConfig{
					Name:        "replication",
					Description: "Use these skills when you need to monitor the health of database replication, manage sync states between nodes, and audit publication tables for distributed setups.",
					ToolNames:   []string{"replication_stats", "list_replication_slots", "list_publication_tables", "database_overview"},
				},
				"access-control": group.GroupConfig{
					Name:        "access-control",
					Description: "Use these skills when you need to manage user roles, inspect permissions, and verify security-related configuration parameters.",
					ToolNames:   []string{"list_roles", "list_pg_settings", "database_overview"},
				},
			},
		},
		{
			name: "alloydb postgres admin prebuilt tools",
			in:   alloydb_admin_config,
			wantGroups: server.GroupConfigs{
				"alloydb_postgres_admin_tools": group.GroupConfig{
					Name:      "alloydb_postgres_admin_tools",
					ToolNames: []string{"create_cluster", "wait_for_operation", "create_instance", "list_clusters", "list_instances", "list_users", "create_user", "get_cluster", "get_instance", "get_user"},
				},
			},
		},
		{
			name: "cloudsql pg admin prebuilt tools",
			in:   cloudsqlpg_admin_config,
			wantGroups: server.GroupConfigs{
				"cloud_sql_postgres_admin_tools": group.GroupConfig{
					Name:      "cloud_sql_postgres_admin_tools",
					ToolNames: []string{"create_instance", "get_instance", "list_instances", "create_database", "list_databases", "create_user", "wait_for_operation", "postgres_upgrade_precheck", "clone_instance", "create_backup", "restore_backup"},
				},
			},
		},
		{
			name: "cloudsql mysql admin prebuilt tools",
			in:   cloudsqlmysql_admin_config,
			wantGroups: server.GroupConfigs{
				"cloud_sql_mysql_admin_tools": group.GroupConfig{
					Name:      "cloud_sql_mysql_admin_tools",
					ToolNames: []string{"create_instance", "get_instance", "list_instances", "create_database", "list_databases", "create_user", "wait_for_operation", "clone_instance", "create_backup", "restore_backup"},
				},
			},
		},
		{
			name: "cloudsql mssql admin prebuilt tools",
			in:   cloudsqlmssql_admin_config,
			wantGroups: server.GroupConfigs{
				"cloud_sql_mssql_admin_tools": group.GroupConfig{
					Name:      "cloud_sql_mssql_admin_tools",
					ToolNames: []string{"create_instance", "get_instance", "list_instances", "create_database", "list_databases", "create_user", "wait_for_operation", "clone_instance", "create_backup", "restore_backup"},
				},
			},
		},
		{
			name: "alloydb prebuilt tools",
			in:   alloydb_config,
			wantGroups: server.GroupConfigs{
				"admin": group.GroupConfig{
					Name:        "admin",
					Description: "Use these skills when you need to provision new AlloyDB clusters and instances, monitor their creation status, and retrieve high-level configuration or health data for the environment.",
					ToolNames:   []string{"create_cluster", "get_cluster", "list_clusters", "create_instance", "get_instance", "list_instances", "database_overview", "wait_for_operation"},
				},
				"access-management": group.GroupConfig{
					Name:        "access-management",
					Description: "Use these skills when you need to manage database users, inspect permissions and roles, and verify global configuration parameters related to security and access control.",
					ToolNames:   []string{"create_user", "list_users", "get_user", "list_roles", "list_pg_settings", "database_overview"},
				},
				"data": group.GroupConfig{
					Name:        "data",
					Description: "Use these skills when you need to explore the database schema, identify objects like views and triggers, and execute custom SQL queries to interact with your data.",
					ToolNames:   []string{"execute_sql", "list_tables", "list_views", "list_schemas", "list_triggers", "list_indexes", "list_sequences", "list_stored_procedure"},
				},
				"monitor": group.GroupConfig{
					Name:        "monitor",
					Description: "Use these skills when you need to troubleshoot slow performance, analyze query execution plans, identify resource-heavy processes, and monitor system-level PromQL metrics.",
					ToolNames:   []string{"list_active_queries", "list_query_stats", "get_query_plan", "get_query_metrics", "get_system_metrics", "long_running_transactions", "list_locks", "list_database_stats"},
				},
				"health": group.GroupConfig{
					Name:        "health",
					Description: "Use these skills when you need to optimize storage, identify index issues, analyze table statistics, or manage autovacuum and tablespace configurations to maintain peak database health.",
					ToolNames:   []string{"list_top_bloated_tables", "list_invalid_indexes", "list_table_stats", "get_column_cardinality", "list_autovacuum_configurations", "list_tablespaces", "database_overview", "get_instance"},
				},
				"optimize": group.GroupConfig{
					Name:        "optimize",
					Description: "Use these skills when you need to discover and manage PostgreSQL extensions or fine-tune engine-level settings such as memory allocation and server configuration parameters.",
					ToolNames:   []string{"list_available_extensions", "list_installed_extensions", "list_memory_configurations", "list_pg_settings", "database_overview", "get_cluster"},
				},
				"replication": group.GroupConfig{
					Name:        "replication",
					Description: "Use these skills when you need to monitor replication health, manage sync states between nodes, and ensure the high availability and data distribution of your AlloyDB cluster.",
					ToolNames:   []string{"replication_stats", "list_replication_slots", "list_publication_tables", "list_instances", "get_instance", "database_overview"},
				},
			},
		},
		{
			name: "bigquery prebuilt tools",
			in:   bigquery_config,
			wantGroups: server.GroupConfigs{
				"data": group.GroupConfig{
					Name:        "data",
					Description: "Use these skills when you need to handle large-scale data exploration and dataset management. Use when users need to find data assets or run SQL at scale. Provides metadata discovery and query execution across the data warehouse.",
					ToolNames:   []string{"execute_sql", "list_dataset_ids", "list_table_ids", "get_dataset_info", "get_table_info", "search_catalog"},
				},
				"analytics": group.GroupConfig{
					Name:        "analytics",
					Description: `Use these skills when you need to handle advanced data intelligence and predictive tasks. Use when a user asks "why" data changed or needs future projections. Provides automated insight generation and time-series forecasting.`,
					ToolNames:   []string{"analyze_contribution", "ask_data_insights", "forecast", "search_catalog"},
				},
			},
		},
		{
			name: "clickhouse prebuilt tools",
			in:   clickhouse_config,
			wantGroups: server.GroupConfigs{
				"clickhouse_database_tools": group.GroupConfig{
					Name:      "clickhouse_database_tools",
					ToolNames: []string{"execute_sql", "list_databases", "list_tables"},
				},
			},
		},
		{
			name: "cloudsqlpg prebuilt tools",
			in:   cloudsqlpg_config,
			wantGroups: server.GroupConfigs{
				"admin": group.GroupConfig{
					Name:        "admin",
					Description: "Use these skills when you need to provision new Cloud SQL instances, create databases and users, clone existing environments, and monitor the progress of long-running operations.",
					ToolNames:   []string{"create_instance", "get_instance", "list_instances", "create_database", "list_databases", "create_user", "wait_for_operation", "clone_instance"},
				},
				"lifecycle": group.GroupConfig{
					Name:        "lifecycle",
					Description: "Use these skills when you need to manage the lifecycle of your instances, including performing backups and restores, checking major version upgrade compatibility, and monitoring overall instance status.",
					ToolNames:   []string{"create_backup", "restore_backup", "postgres_upgrade_precheck", "wait_for_operation", "database_overview", "get_instance", "list_instances"},
				},
				"data": group.GroupConfig{
					Name:        "data",
					Description: "Use these skills when you need to explore the database structure, discover schema objects like views or stored procedures, and execute custom SQL queries to interact with your data.",
					ToolNames:   []string{"execute_sql", "list_tables", "list_views", "list_schemas", "list_triggers", "list_indexes", "list_sequences", "list_stored_procedure"},
				},
				"monitor": group.GroupConfig{
					Name:        "monitor",
					Description: "Use these skills when you need to troubleshoot performance bottlenecks, analyze query execution plans, identify resource-heavy processes, and monitor system-level PromQL metrics.",
					ToolNames:   []string{"get_system_metrics", "get_query_metrics", "list_query_stats", "get_query_plan", "list_database_stats", "list_active_queries", "long_running_transactions", "list_locks"},
				},
				"health": group.GroupConfig{
					Name:        "health",
					Description: "Use these skills when you need to audit database health, identify storage bloat, find invalid indexes, analyze table statistics, and manage maintenance configurations like autovacuum.",
					ToolNames:   []string{"list_top_bloated_tables", "list_invalid_indexes", "list_table_stats", "get_column_cardinality", "list_autovacuum_configurations", "list_tablespaces", "database_overview", "list_pg_settings"},
				},
				"view-config": group.GroupConfig{
					Name:        "view-config",
					Description: "Use these skills when you need to discover and manage PostgreSQL extensions or fine-tune engine-level settings such as memory allocation and server configuration parameters.",
					ToolNames:   []string{"list_available_extensions", "list_installed_extensions", "list_memory_configurations", "list_pg_settings", "database_overview", "get_instance"},
				},
				"replication": group.GroupConfig{
					Name:        "replication",
					Description: "Use these skills when you need to monitor replication health, manage sync states between nodes, and audit database roles and security settings to ensure environment integrity.",
					ToolNames:   []string{"replication_stats", "list_replication_slots", "list_publication_tables", "list_roles", "list_pg_settings", "database_overview"},
				},
				"vectorassist": {
					Name:        "vectorassist",
					Description: "Use these skills to set up and optimize production-ready vector workloads by simply expressing your intent and performance requirements.",
					ToolNames:   []string{"execute_sql", "define_spec", "modify_spec", "apply_spec", "generate_query", "improve_query_recall", "list_specs", "get_spec", "delete_spec"},
				},
			},
		},
		{
			name: "cloudsqlmysql prebuilt tools",
			in:   cloudsqlmysql_config,
			wantGroups: server.GroupConfigs{
				"admin": group.GroupConfig{
					Name:        "admin",
					Description: "Use these skills when you need to provision new Cloud SQL for MySQL instances, create databases and users, and monitor the progress of infrastructure operations.",
					ToolNames:   []string{"create_instance", "get_instance", "list_instances", "create_database", "list_databases", "create_user", "wait_for_operation"},
				},
				"data": group.GroupConfig{
					Name:        "data",
					Description: "Use these skills when you need to explore your database schema, execute SQL queries to interact with your data, and inspect how MySQL plans to execute your statements.",
					ToolNames:   []string{"execute_sql", "list_tables", "get_query_plan", "list_active_queries"},
				},
				"monitor": group.GroupConfig{
					Name:        "monitor",
					Description: "Use these skills when you need to troubleshoot slow queries, analyze system-level PromQL metrics, and identify structural performance issues like table fragmentation or missing unique indexes.",
					ToolNames:   []string{"get_query_plan", "list_active_queries", "list_all_locks", "get_query_metrics", "get_system_metrics", "list_table_fragmentation", "list_table_stats", "list_tables_missing_unique_indexes", "show_query_stats"},
				},
				"lifecycle": group.GroupConfig{
					Name:        "lifecycle",
					Description: "Use these skills when you need to manage the durability and safety of your data by creating backups, restoring from previous states, or cloning instances for recovery and testing.",
					ToolNames:   []string{"create_backup", "restore_backup", "clone_instance", "list_instances", "get_instance", "wait_for_operation"},
				},
			},
		},
		{
			name: "cloudsqlmssql prebuilt tools",
			in:   cloudsqlmssql_config,
			wantGroups: server.GroupConfigs{
				"admin": group.GroupConfig{
					Name:        "admin",
					Description: "Use these skills when you need to provision new Cloud SQL for SQL Server instances, create databases and users, and monitor the progress of long-running operations.",
					ToolNames:   []string{"create_instance", "get_instance", "list_instances", "create_database", "list_databases", "create_user", "wait_for_operation"},
				},
				"data": group.GroupConfig{
					Name:        "data",
					Description: "Use these skills when you need to explore the database schema and execute SQL queries to interact with your data.",
					ToolNames:   []string{"execute_sql", "list_tables"},
				},
				"monitor": group.GroupConfig{
					Name:        "monitor",
					Description: "Use these skills when you need to troubleshoot slow queries and analyze system-level PromQL metrics.",
					ToolNames:   []string{"get_system_metrics"},
				},
				"lifecycle": group.GroupConfig{
					Name:        "lifecycle",
					Description: "Use these skills when you need to manage the lifecycle and durability of your data, including creating backups, restoring from existing backups, and cloning instances for testing or migration.",
					ToolNames:   []string{"create_backup", "restore_backup", "clone_instance", "list_instances", "get_instance", "wait_for_operation"},
				},
			},
		},
		{
			name: "dataplex prebuilt tools",
			in:   dataplex_config,
			wantGroups: server.GroupConfigs{
				"discovery": group.GroupConfig{
					Name:      "discovery",
					ToolNames: []string{"search_entries", "lookup_entry", "search_aspect_types", "lookup_context", "search_dq_scans"},
				},
				"data-products": group.GroupConfig{
					Name:      "data-products",
					ToolNames: []string{"search_entries", "lookup_entry", "search_aspect_types", "lookup_context", "list_data_products", "get_data_product", "list_data_assets", "get_data_asset", "create_data_product", "update_data_product", "create_data_asset", "update_data_asset", "update_data_product_aspects", "get_operation"},
				},
				"enrich": group.GroupConfig{
					Name:      "enrich",
					ToolNames: []string{"search_entries", "lookup_entry", "lookup_context", "generate_data_insights", "get_data_insights", "generate_data_profile", "get_data_profile", "discover_metadata", "get_discovery_results", "check_data_quality", "get_data_quality_results", "get_operation", "get_run_status"},
				},
			},
		},
		{
			name: "dataproc prebuilt tools",
			in:   dataproc_config,
			wantGroups: server.GroupConfigs{
				"dataproc_tools": group.GroupConfig{
					Name:        "dataproc_tools",
					Description: "Skills to interact with your Dataproc clusters and jobs.",
					ToolNames:   []string{"list_clusters", "get_cluster", "list_jobs", "get_job"},
				},
			},
		},
		{
			name: "serverless spark prebuilt tools",
			in:   serverless_spark_config,
			wantGroups: server.GroupConfigs{
				"serverless_spark_tools": group.GroupConfig{
					Name:      "serverless_spark_tools",
					ToolNames: []string{"list_batches", "get_batch", "cancel_batch", "create_pyspark_batch", "create_spark_batch", "get_session_template", "list_sessions", "get_session"},
				},
			},
		},
		{
			name: "firestore prebuilt tools",
			in:   firestoreconfig,
			wantGroups: server.GroupConfigs{
				"data": group.GroupConfig{
					Name:        "data",
					Description: "Handles NoSQL document operations and collection hierarchy exploration. Use for CRUD tasks and data retrieval. Provides flexible document manipulation and structured querying.",
					ToolNames:   []string{"get_documents", "add_documents", "update_document", "delete_documents", "query_collection", "list_collections"},
				},
				"security": group.GroupConfig{
					Name:      "security",
					ToolNames: []string{"get_rules", "validate_rules"},
				},
			},
		},
		{
			name: "mysql prebuilt tools",
			in:   mysql_config,
			wantGroups: server.GroupConfigs{
				"data": group.GroupConfig{
					Name:      "data",
					ToolNames: []string{"execute_sql", "list_tables", "get_query_plan", "list_active_queries"},
				},
				"monitor": group.GroupConfig{
					Name:      "monitor",
					ToolNames: []string{"get_query_plan", "list_active_queries", "list_all_locks", "list_table_fragmentation", "list_table_stats", "list_tables_missing_unique_indexes", "show_query_stats"},
				},
			},
		},
		{
			name: "mssql prebuilt tools",
			in:   mssql_config,
			wantGroups: server.GroupConfigs{
				"data": group.GroupConfig{
					Name:      "data",
					ToolNames: []string{"execute_sql", "list_tables"},
				},
			},
		},
		{
			name: "looker prebuilt tools",
			in:   looker_config,
			wantGroups: server.GroupConfigs{
				"looker_tools": group.GroupConfig{
					Name:        "looker_tools",
					Description: "These skills are designed for data discovery and business intelligence.",
					ToolNames:   []string{"get_models", "get_explores", "get_dimensions", "get_measures", "get_filters", "get_parameters", "get_field_value_suggestions", "query", "query_sql", "query_url", "get_looks", "run_look", "make_look", "get_dashboards", "run_dashboard", "make_dashboard", "add_dashboard_element", "add_dashboard_filter", "generate_embed_url", "get_dashboard", "update_dashboard_element", "create_dashboard_layout", "update_dashboard_layout_component"},
				},
			},
		},
		{
			name: "looker dev prebuilt tools",
			in:   looker_dev_config,
			wantGroups: server.GroupConfigs{
				"looker_dev_tools": group.GroupConfig{
					Name:        "looker_dev_tools",
					Description: "These skills are built for LookML developers, data engineers, and administrators who manage the backbone of Looker.",
					ToolNames:   []string{"health_pulse", "health_analyze", "health_vacuum", "dev_mode", "get_projects", "get_project_files", "get_project_file", "create_project_file", "update_project_file", "delete_project_file", "get_project_directories", "create_project_directory", "delete_project_directory", "validate_project", "get_connections", "get_connection_schemas", "get_connection_databases", "get_connection_tables", "get_connection_table_columns", "get_lookml_tests", "run_lookml_tests", "create_view_from_table", "list_git_branches", "get_git_branch", "create_git_branch", "switch_git_branch", "delete_git_branch"},
				},
			},
		},
		{
			name: "looker-conversational-analytics prebuilt tools",
			in:   lookerca_config,
			wantGroups: server.GroupConfigs{
				"looker_conversational_analytics_tools": group.GroupConfig{
					Name:      "looker_conversational_analytics_tools",
					ToolNames: []string{"ask_data_insights", "get_models", "get_explores"},
				},
			},
		},
		{
			name: "postgres prebuilt tools",
			in:   postgresconfig,
			wantGroups: server.GroupConfigs{
				"data": group.GroupConfig{
					Name:      "data",
					ToolNames: []string{"execute_sql", "list_tables", "list_views", "list_schemas", "list_triggers", "list_indexes", "list_sequences", "list_stored_procedure"},
				},
				"monitor": group.GroupConfig{
					Name:      "monitor",
					ToolNames: []string{"list_query_stats", "get_query_plan", "list_database_stats", "list_active_queries", "long_running_transactions", "list_locks"},
				},
				"health": group.GroupConfig{
					Name:      "health",
					ToolNames: []string{"list_top_bloated_tables", "list_invalid_indexes", "list_table_stats", "get_column_cardinality", "list_autovacuum_configurations", "list_tablespaces", "database_overview", "list_pg_settings"},
				},
				"view-config": group.GroupConfig{
					Name:      "view-config",
					ToolNames: []string{"list_available_extensions", "list_installed_extensions", "list_memory_configurations", "list_pg_settings", "database_overview"},
				},
				"replication": group.GroupConfig{
					Name:      "replication",
					ToolNames: []string{"replication_stats", "list_replication_slots", "list_publication_tables", "list_roles", "list_pg_settings", "database_overview"},
				},
			},
		},
		{
			name: "spanner prebuilt tools",
			in:   spanner_config,
			wantGroups: server.GroupConfigs{
				"data": group.GroupConfig{
					Name:        "data",
					Description: "Use these skills when you need to explore the database structure, discover schema objects like tables and graphs, and execute custom SQL queries to interact with your data.",
					ToolNames:   []string{"execute_sql", "execute_sql_readonly", "list_tables", "list_graphs"},
				},
				"data_with_discovery": group.GroupConfig{
					Name:      "data_with_discovery",
					ToolNames: []string{"execute_sql", "execute_sql_readonly", "list_tables", "list_graphs", "search_catalog"},
				},
			},
		},
		{
			name: "spanner pg prebuilt tools",
			in:   spannerpg_config,
			wantGroups: server.GroupConfigs{
				"data": group.GroupConfig{
					Name:      "data",
					ToolNames: []string{"execute_sql", "execute_sql_readonly", "list_tables"},
				},
				"data_with_discovery": group.GroupConfig{
					Name:      "data_with_discovery",
					ToolNames: []string{"execute_sql", "execute_sql_readonly", "list_tables", "search_catalog"},
				},
			},
		},
		{
			name: "mindsdb prebuilt tools",
			in:   mindsdb_config,
			wantGroups: server.GroupConfigs{
				"mindsdb-tools": group.GroupConfig{
					Name:      "mindsdb-tools",
					ToolNames: []string{"execute_sql", "parameterized_sql"},
				},
			},
		},
		{
			name: "sqlite prebuilt tools",
			in:   sqlite_config,
			wantGroups: server.GroupConfigs{
				"sqlite_database_tools": group.GroupConfig{
					Name:      "sqlite_database_tools",
					ToolNames: []string{"execute_sql", "list_tables"},
				},
			},
		},
		{
			name: "neo4j prebuilt tools",
			in:   neo4jconfig,
			wantGroups: server.GroupConfigs{
				"neo4j_database_tools": group.GroupConfig{
					Name:      "neo4j_database_tools",
					ToolNames: []string{"execute_cypher", "get_schema"},
				},
			},
		},
		{
			name: "alloydb postgres observability prebuilt tools",
			in:   alloydbobsvconfig,
			wantGroups: server.GroupConfigs{
				"alloydb_postgres_cloud_monitoring_tools": group.GroupConfig{
					Name:      "alloydb_postgres_cloud_monitoring_tools",
					ToolNames: []string{"get_system_metrics", "get_query_metrics"},
				},
				"alloydb_postgres_database_insights_tools": group.GroupConfig{
					Name:      "alloydb_postgres_database_insights_tools",
					ToolNames: []string{"get_advanced_aggregated_query_stats", "get_advanced_aggregated_wait_event_stats", "get_advanced_time_series_query_stats", "get_advanced_time_series_wait_event_stats", "get_index_recommendations"},
				},
				"alloydb_postgres_observability_tools": group.GroupConfig{
					Name:      "alloydb_postgres_observability_tools",
					ToolNames: []string{"get_system_metrics", "get_query_metrics", "get_advanced_aggregated_query_stats", "get_advanced_aggregated_wait_event_stats", "get_advanced_time_series_query_stats", "get_advanced_time_series_wait_event_stats", "get_index_recommendations"},
				},
			},
		},
		{
			name: "cloudsql postgres observability prebuilt tools",
			in:   cloudsqlpgobsvconfig,
			wantGroups: server.GroupConfigs{
				"cloud_sql_postgres_cloud_monitoring_tools": group.GroupConfig{
					Name:      "cloud_sql_postgres_cloud_monitoring_tools",
					ToolNames: []string{"get_system_metrics", "get_query_metrics"},
				},
			},
		},
		{
			name: "cloudsql mysql observability prebuilt tools",
			in:   cloudsqlmysqlobsvconfig,
			wantGroups: server.GroupConfigs{
				"cloud_sql_mysql_cloud_monitoring_tools": group.GroupConfig{
					Name:      "cloud_sql_mysql_cloud_monitoring_tools",
					ToolNames: []string{"get_system_metrics", "get_query_metrics"},
				},
			},
		},
		{
			name: "cloudsql mssql observability prebuilt tools",
			in:   cloudsqlmssqlobsvconfig,
			wantGroups: server.GroupConfigs{
				"cloud_sql_mssql_cloud_monitoring_tools": group.GroupConfig{
					Name:      "cloud_sql_mssql_cloud_monitoring_tools",
					ToolNames: []string{"get_system_metrics"},
				},
			},
		},
		{
			name: "cloud healthcare prebuilt tools",
			in:   cloudhealthcare_config,
			wantGroups: server.GroupConfigs{
				"cloud_healthcare_dataset_tools": group.GroupConfig{
					Name:      "cloud_healthcare_dataset_tools",
					ToolNames: []string{"get_dataset", "list_dicom_stores", "list_fhir_stores"},
				},
				"cloud_healthcare_fhir_tools": group.GroupConfig{
					Name:      "cloud_healthcare_fhir_tools",
					ToolNames: []string{"get_fhir_store", "get_fhir_store_metrics", "get_fhir_resource", "fhir_patient_search", "fhir_patient_everything", "fhir_fetch_page"},
				},
				"cloud_healthcare_dicom_tools": group.GroupConfig{
					Name:      "cloud_healthcare_dicom_tools",
					ToolNames: []string{"get_dicom_store", "get_dicom_store_metrics", "search_dicom_studies", "search_dicom_series", "search_dicom_instances", "retrieve_rendered_dicom_instance"},
				},
			},
		},
		{
			name: "cloud storage prebuilt tools",
			in:   cloudstorage_config,
			wantGroups: server.GroupConfigs{
				"cloud-storage-buckets": group.GroupConfig{
					Name:        "cloud-storage-buckets",
					Description: "Use these tools when you need to administer cloud storage buckets, including listing and creating buckets, inspecting bucket metadata and access control policies, and deleting buckets.",
					ToolNames:   []string{"list_buckets", "create_bucket", "get_bucket_metadata", "get_bucket_iam_policy", "delete_bucket"},
				},
				"cloud-storage-objects": group.GroupConfig{
					Name:        "cloud-storage-objects",
					Description: "Use these tools when you need to manage files and objects in cloud storage — listing, reading, writing, copying, moving, or deleting objects and retrieving their metadata.",
					ToolNames:   []string{"list_objects", "get_object_metadata", "read_object", "download_object", "write_object", "upload_object", "copy_object", "move_object", "delete_object"},
				},
			},
		},
		{
			name: "Snowflake prebuilt tool",
			in:   snowflake_config,
			wantGroups: server.GroupConfigs{
				"snowflake_tools": group.GroupConfig{
					Name:      "snowflake_tools",
					ToolNames: []string{"execute_sql", "list_tables"},
				},
			},
		},
		{
			name: "Oracle prebuilt tools",
			in:   oracle_config,
			wantGroups: server.GroupConfigs{
				"oracle_database_tools": group.GroupConfig{
					Name:        "oracle_database_tools",
					Description: "Use these skills to manage and monitor Oracle databases by executing SQL statements, exploring schema metadata, analyzing query performance, monitoring active sessions and resource consumption, and managing storage and object health.",
					ToolNames:   []string{"execute_sql", "list_tables", "list_active_sessions", "get_query_plan", "list_top_sql_by_resource", "list_tablespace_usage", "list_invalid_objects"},
				},
			},
		},
		{
			name: "Conversational Analytics with Data Agent prebuilt tools",
			in:   conversationalanalytics_config,
			wantGroups: server.GroupConfigs{
				"conversational_analytics_tools": group.GroupConfig{
					Name:      "conversational_analytics_tools",
					ToolNames: []string{"list_accessible_data_agents", "get_data_agent_info", "ask_data_agent"},
				},
			},
		},
		{
			name: "Elasticsearch prebuilt tools",
			in:   elasticsearch_config,
			wantGroups: server.GroupConfigs{
				"elasticsearch-tools": group.GroupConfig{
					Name:      "elasticsearch-tools",
					ToolNames: []string{"execute_esql_query"},
				},
			},
		},
		{
			name: "Oceanbase prebuilt tools",
			in:   oceanbase_config,
			wantGroups: server.GroupConfigs{
				"oceanbase_database_tools": group.GroupConfig{
					Name:      "oceanbase_database_tools",
					ToolNames: []string{"execute_sql", "list_tables"},
				},
			},
		},
		{
			name: "Singlestore prebuilt tools",
			in:   singlestore_config,
			wantGroups: server.GroupConfigs{
				"singlestore-database-tools": group.GroupConfig{
					Name:      "singlestore-database-tools",
					ToolNames: []string{"execute_sql", "list_tables"},
				},
			},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			parser := ConfigParser{}
			configFile, err := parser.ParseConfig(ctx, tc.in)
			if err != nil {
				t.Fatalf("failed to parse input: %v", err)
			}
			if diff := cmp.Diff(tc.wantGroups, configFile.Groups); diff != "" {
				t.Fatalf("incorrect tools parse: diff %v", diff)
			}
			// Prebuilt configs do not have prompts, so assert empty maps.
			if len(configFile.Prompts) != 0 {
				t.Fatalf("expected empty prompts map for prebuilt config, got: %v", configFile.Prompts)
			}

			t.Run("check toolset sizes", func(t *testing.T) {
				for tsName, ts := range configFile.Groups {
					if len(ts.ToolNames) > 10 {
						t.Logf("WARNING: Group %q in config %q has %d tools, which is larger than the recommended maximum of 10.", tsName, tc.name, len(ts.ToolNames))
					}
				}
			})
		})
	}
}

func TestMergeConfigs(t *testing.T) {
	file1 := Config{
		Sources:         server.SourceConfigs{"source1": httpsrc.Config{Name: "source1"}},
		Tools:           server.ToolConfigs{"tool1": http.Config{ConfigBase: tools.ConfigBase{Name: "tool1"}}},
		Groups:          server.GroupConfigs{"set1": group.GroupConfig{Name: "set1"}},
		EmbeddingModels: server.EmbeddingModelConfigs{"model1": gemini.Config{Name: "gemini-text"}},
	}
	file2 := Config{
		AuthServices: server.AuthServiceConfigs{"auth1": google.Config{Name: "auth1"}},
		Tools:        server.ToolConfigs{"tool2": http.Config{ConfigBase: tools.ConfigBase{Name: "tool2"}}},
		Groups:       server.GroupConfigs{"set2": group.GroupConfig{Name: "set2"}},
	}
	fileWithConflicts := Config{
		Sources: server.SourceConfigs{"source1": httpsrc.Config{Name: "source1"}},
		Tools:   server.ToolConfigs{"tool2": http.Config{ConfigBase: tools.ConfigBase{Name: "tool2"}}},
	}
	fileMcp1 := Config{
		AuthServices: server.AuthServiceConfigs{"generic1": generic.Config{Name: "generic1", McpEnabled: true}},
	}
	fileMcp2 := Config{
		AuthServices: server.AuthServiceConfigs{"generic2": generic.Config{Name: "generic2", McpEnabled: true}},
	}

	testCases := []struct {
		name      string
		files     []Config
		want      Config
		wantErr   bool
		errString string
	}{
		{
			name:  "merge two distinct files",
			files: []Config{file1, file2},
			want: Config{
				Sources:         server.SourceConfigs{"source1": httpsrc.Config{Name: "source1"}},
				AuthServices:    server.AuthServiceConfigs{"auth1": google.Config{Name: "auth1"}},
				Tools:           server.ToolConfigs{"tool1": http.Config{ConfigBase: tools.ConfigBase{Name: "tool1"}}, "tool2": http.Config{ConfigBase: tools.ConfigBase{Name: "tool2"}}},
				Prompts:         server.PromptConfigs{},
				Groups:          server.GroupConfigs{"set1": group.GroupConfig{Name: "set1"}, "set2": group.GroupConfig{Name: "set2"}},
				EmbeddingModels: server.EmbeddingModelConfigs{"model1": gemini.Config{Name: "gemini-text"}},
			},
			wantErr: false,
		},
		{
			name:    "merge with conflicts",
			files:   []Config{file1, file2, fileWithConflicts},
			wantErr: true,
		},
		{
			name:      "merge multiple mcp enabled generic",
			files:     []Config{fileMcp1, fileMcp2},
			wantErr:   true,
			errString: "multiple authServices with mcpEnabled=true detected",
		},
		{
			name:  "merge single file",
			files: []Config{file1},
			want: Config{
				Sources:         file1.Sources,
				AuthServices:    make(server.AuthServiceConfigs),
				EmbeddingModels: server.EmbeddingModelConfigs{"model1": gemini.Config{Name: "gemini-text"}},
				Tools:           file1.Tools,
				Prompts:         server.PromptConfigs{},
				Groups:          file1.Groups,
			},
		},
		{
			name:  "merge empty list",
			files: []Config{},
			want: Config{
				Sources:         make(server.SourceConfigs),
				AuthServices:    make(server.AuthServiceConfigs),
				EmbeddingModels: make(server.EmbeddingModelConfigs),
				Tools:           make(server.ToolConfigs),
				Prompts:         server.PromptConfigs{},
				Groups:          make(server.GroupConfigs),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := mergeConfigs(tc.files...)
			if (err != nil) != tc.wantErr {
				t.Fatalf("mergeConfigs() error = %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr {
				if diff := cmp.Diff(tc.want, got); diff != "" {
					t.Errorf("mergeConfigs() mismatch (-want +got):\n%s", diff)
				}
			} else {
				if err == nil {
					t.Fatal("expected an error for conflicting files but got none")
				}
				if tc.errString != "" && !strings.Contains(err.Error(), tc.errString) {
					t.Errorf("expected error %q, but got: %v", tc.errString, err)
				} else if tc.errString == "" && !strings.Contains(err.Error(), "resource conflicts detected") {
					t.Errorf("expected conflict error, but got: %v", err)
				}
			}
		})
	}
}

func TestParameterReferenceValidation(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	// Base template
	baseYaml := `
sources:
  dummy-source:
    kind: http
    baseUrl: http://example.com
tools:
  test-tool:
    kind: postgres-sql
    source: dummy-source
    description: test tool
    statement: SELECT 1;
    parameters:
%s`

	tcs := []struct {
		desc      string
		params    string
		wantErr   bool
		errSubstr string
	}{
		{
			desc: "valid backward reference",
			params: `
      - name: source_param
        type: string
        description: source
      - name: copy_param
        type: string
        description: copy
        valueFromParam: source_param`,
			wantErr: false,
		},
		{
			desc: "valid forward reference (out of order)",
			params: `
      - name: copy_param
        type: string
        description: copy
        valueFromParam: source_param
      - name: source_param
        type: string
        description: source`,
			wantErr: false,
		},
		{
			desc: "invalid missing reference",
			params: `
      - name: copy_param
        type: string
        description: copy
        valueFromParam: non_existent_param`,
			wantErr:   true,
			errSubstr: "references '\"non_existent_param\"' in the 'valueFromParam' field",
		},
		{
			desc: "invalid self reference",
			params: `
      - name: myself
        type: string
        description: self
        valueFromParam: myself`,
			wantErr:   true,
			errSubstr: "parameter \"myself\" cannot copy value from itself",
		},
		{
			desc: "multiple valid references",
			params: `
      - name: a
        type: string
        description: a
      - name: b
        type: string
        description: b
        valueFromParam: a
      - name: c
        type: string
        description: c
        valueFromParam: a`,
			wantErr: false,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			// Indent parameters to match YAML structure
			yamlContent := fmt.Sprintf(baseYaml, tc.params)
			parser := ConfigParser{}
			_, err := parser.ParseConfig(ctx, []byte(yamlContent))

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tc.errSubstr) {
					t.Errorf("error %q does not contain expected substring %q", err.Error(), tc.errSubstr)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestParseConfigGroupNameValidation(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	tcs := []struct {
		description string
		in          string
		wantErr     bool
		errContains string
	}{
		{
			description: "group with integer name is rejected",
			in: `
kind: group
name: 123
`,
			wantErr:     true,
			errContains: "missing 'name' field or it is not a string",
		},
		{
			description: "group with boolean name is rejected",
			in: `
kind: group
name: true
`,
			wantErr:     true,
			errContains: "missing 'name' field or it is not a string",
		},
		{
			description: "group with absent name is accepted as default group",
			in: `
kind: group
description: the default group
`,
			wantErr: false,
		},
		{
			description: "group with null name is accepted as default group",
			in: `
kind: group
name: ~
description: the default group
`,
			wantErr: false,
		},
		{
			description: "group with quoted numeric name is accepted",
			in: `
kind: group
name: "123"
`,
			wantErr: false,
		},
		{
			description: "non-group resource with integer name is rejected",
			in: `
kind: tool
name: 42
type: postgres-sql
source: my-pg-instance
description: some description
statement: SELECT 1
`,
			wantErr:     true,
			errContains: "missing 'name' field or it is not a string",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.description, func(t *testing.T) {
			parser := ConfigParser{}
			_, err := parser.ParseConfig(ctx, testutils.FormatYaml(tc.in))
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
					t.Errorf("error %q does not contain %q", err.Error(), tc.errContains)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}
