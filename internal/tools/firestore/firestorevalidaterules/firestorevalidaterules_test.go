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

package firestorevalidaterules_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/firestore/firestorevalidaterules"
)

func TestParseFromYamlFirestoreValidateRules(t *testing.T) {
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
			name: validate_rules_tool
			type: firestore-validate-rules
			source: my-firestore-instance
			description: Validate Firestore security rules
			`,
			want: server.ToolConfigs{
				"validate_rules_tool": firestorevalidaterules.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "validate_rules_tool",
						Description:  "Validate Firestore security rules",
						AuthRequired: []string{},
					},
					Type:   "firestore-validate-rules",
					Source: "my-firestore-instance",
				},
			},
		},
		{
			desc: "with auth requirements",
			in: `
			kind: tool
			name: secure_validate_rules
			type: firestore-validate-rules
			source: prod-firestore
			description: Validate rules with authentication
			authRequired:
				- google-auth-service
				- api-key-service
			`,
			want: server.ToolConfigs{
				"secure_validate_rules": firestorevalidaterules.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "secure_validate_rules",
						Description:  "Validate rules with authentication",
						AuthRequired: []string{"google-auth-service", "api-key-service"},
					},
					Type:   "firestore-validate-rules",
					Source: "prod-firestore",
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

func TestParseFromYamlMultipleTools(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	in := `
	kind: tool
	name: validate_dev_rules
	type: firestore-validate-rules
	source: dev-firestore
	description: Validate development environment rules
	authRequired:
		- dev-auth
---
	kind: tool
	name: validate_staging_rules
	type: firestore-validate-rules
	source: staging-firestore
	description: Validate staging environment rules
---
	kind: tool
	name: validate_prod_rules
	type: firestore-validate-rules
	source: prod-firestore
	description: Validate production environment rules
	authRequired:
		- prod-auth
		- admin-auth
	`
	want := server.ToolConfigs{
		"validate_dev_rules": firestorevalidaterules.Config{
			ConfigBase: tools.ConfigBase{
				Name:         "validate_dev_rules",
				Description:  "Validate development environment rules",
				AuthRequired: []string{"dev-auth"},
			},
			Type:   "firestore-validate-rules",
			Source: "dev-firestore",
		},
		"validate_staging_rules": firestorevalidaterules.Config{
			ConfigBase: tools.ConfigBase{
				Name:         "validate_staging_rules",
				Description:  "Validate staging environment rules",
				AuthRequired: []string{},
			},
			Type:   "firestore-validate-rules",
			Source: "staging-firestore",
		},
		"validate_prod_rules": firestorevalidaterules.Config{
			ConfigBase: tools.ConfigBase{
				Name:         "validate_prod_rules",
				Description:  "Validate production environment rules",
				AuthRequired: []string{"prod-auth", "admin-auth"},
			},
			Type:   "firestore-validate-rules",
			Source: "prod-firestore",
		},
	}

	_, _, _, got, _, _, _, _, err := server.UnmarshalPrimitiveConfig(ctx, testutils.FormatYaml(in))
	if err != nil {
		t.Fatalf("unable to unmarshal: %s", err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("incorrect parse: diff %v", diff)
	}
}
