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

package looker_test

import (
	"context"
	"strings"
	"testing"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/tools"

	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookeradddashboardelement"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookeradddashboardfilter"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookerconversationalanalytics"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookercreateagent"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookercreatedashboardlayout"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookercreategitbranch"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookercreateprojectdirectory"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookercreateprojectfile"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookercreateviewfromtable"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookerdeleteagent"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookerdeletegitbranch"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookerdeleteprojectdirectory"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookerdeleteprojectfile"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookerdevmode"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookergenerateembedurl"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookergetagent"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookergetconnectiondatabases"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookergetconnections"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookergetconnectionschemas"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookergetconnectiontablecolumns"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookergetconnectiontables"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookergetdashboard"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookergetdashboards"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookergetdimensions"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookergetexplores"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookergetfieldvaluesuggestions"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookergetfilters"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookergetgitbranch"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookergetlookmltests"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookergetlooks"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookergetmeasures"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookergetmodels"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookergetparameters"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookergetprojectdirectories"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookergetprojectfile"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookergetprojectfiles"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookergetprojects"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookerhealthanalyze"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookerhealthpulse"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookerhealthvacuum"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookerlistagents"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookerlistgitbranches"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookermakedashboard"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookermakelook"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookerquery"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookerquerysql"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookerqueryurl"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookerrundashboard"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookerrunlook"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookerrunlookmltests"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookerswitchgitbranch"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookerupdateagent"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookerupdatedashboardelement"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookerupdatedashboardlayoutcomponent"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookerupdateprojectfile"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookervalidateproject"
)

func TestAllLookerToolsAnnotations(t *testing.T) {
	tcs := []struct {
		resourceType    string
		wantReadOnly    bool
		wantDestructive bool
	}{
		{
			resourceType:    "looker-add-dashboard-element",
			wantReadOnly:    false,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-add-dashboard-filter",
			wantReadOnly:    false,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-conversational-analytics",
			wantReadOnly:    true,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-create-agent",
			wantReadOnly:    false,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-create-dashboard-layout",
			wantReadOnly:    false,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-create-git-branch",
			wantReadOnly:    false,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-create-project-directory",
			wantReadOnly:    false,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-create-project-file",
			wantReadOnly:    false,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-create-view-from-table",
			wantReadOnly:    false,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-delete-agent",
			wantReadOnly:    false,
			wantDestructive: true,
		},
		{
			resourceType:    "looker-delete-git-branch",
			wantReadOnly:    false,
			wantDestructive: true,
		},
		{
			resourceType:    "looker-delete-project-directory",
			wantReadOnly:    false,
			wantDestructive: true,
		},
		{
			resourceType:    "looker-delete-project-file",
			wantReadOnly:    false,
			wantDestructive: true,
		},
		{
			resourceType:    "looker-dev-mode",
			wantReadOnly:    true,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-generate-embed-url",
			wantReadOnly:    true,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-get-agent",
			wantReadOnly:    true,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-get-connection-databases",
			wantReadOnly:    true,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-get-connections",
			wantReadOnly:    true,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-get-connection-schemas",
			wantReadOnly:    true,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-get-connection-table-columns",
			wantReadOnly:    true,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-get-connection-tables",
			wantReadOnly:    true,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-get-dashboard",
			wantReadOnly:    true,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-get-dashboards",
			wantReadOnly:    true,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-get-dimensions",
			wantReadOnly:    true,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-get-explores",
			wantReadOnly:    true,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-get-field-value-suggestions",
			wantReadOnly:    true,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-get-filters",
			wantReadOnly:    true,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-get-git-branch",
			wantReadOnly:    true,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-get-lookml-tests",
			wantReadOnly:    true,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-get-looks",
			wantReadOnly:    true,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-get-measures",
			wantReadOnly:    true,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-get-models",
			wantReadOnly:    true,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-get-parameters",
			wantReadOnly:    true,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-get-project-directories",
			wantReadOnly:    true,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-get-project-file",
			wantReadOnly:    true,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-get-project-files",
			wantReadOnly:    true,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-get-projects",
			wantReadOnly:    true,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-health-analyze",
			wantReadOnly:    true,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-health-pulse",
			wantReadOnly:    true,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-health-vacuum",
			wantReadOnly:    true,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-list-agents",
			wantReadOnly:    true,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-list-git-branches",
			wantReadOnly:    true,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-make-dashboard",
			wantReadOnly:    false,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-make-look",
			wantReadOnly:    false,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-query",
			wantReadOnly:    true,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-query-sql",
			wantReadOnly:    true,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-query-url",
			wantReadOnly:    true,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-run-dashboard",
			wantReadOnly:    true,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-run-look",
			wantReadOnly:    true,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-run-lookml-tests",
			wantReadOnly:    true,
			wantDestructive: false,
		},
		{
			resourceType:    "looker-switch-git-branch",
			wantReadOnly:    false,
			wantDestructive: true,
		},
		{
			resourceType:    "looker-update-agent",
			wantReadOnly:    false,
			wantDestructive: true,
		},
		{
			resourceType:    "looker-update-dashboard-element",
			wantReadOnly:    false,
			wantDestructive: true,
		},
		{
			resourceType:    "looker-update-dashboard-layout-component",
			wantReadOnly:    false,
			wantDestructive: true,
		},
		{
			resourceType:    "looker-update-project-file",
			wantReadOnly:    false,
			wantDestructive: true,
		},
		{
			resourceType:    "looker-validate-project",
			wantReadOnly:    true,
			wantDestructive: false,
		},
	}

	ctx := context.Background()

	for _, tc := range tcs {
		t.Run(tc.resourceType, func(t *testing.T) {
			yamlConfig := `
source: my-looker-source
description: test description
`
			decoder := yaml.NewDecoder(strings.NewReader(yamlConfig))
			cfg, err := tools.DecodeConfig(ctx, tc.resourceType, "test_tool", decoder)
			if err != nil {
				t.Fatalf("failed to decode config: %v", err)
			}

			tool, err := cfg.Initialize(ctx)
			if err != nil {
				t.Fatalf("failed to initialize tool: %v", err)
			}

			annotations := tool.GetAnnotations(nil)
			if annotations == nil {
				t.Fatal("expected non-nil annotations")
			}

			// OpenWorldHint must always be false for all Looker tools
			if annotations.OpenWorldHint == nil {
				t.Error("expected openWorldHint to be set, got nil")
			} else if *annotations.OpenWorldHint != false {
				t.Errorf("expected openWorldHint=false, got %v", *annotations.OpenWorldHint)
			}

			// DestructiveHint must be true only for tools that can change existing data/metadata
			if annotations.DestructiveHint == nil {
				t.Error("expected destructiveHint to be set, got nil")
			} else if *annotations.DestructiveHint != tc.wantDestructive {
				t.Errorf("expected destructiveHint=%v, got %v", tc.wantDestructive, *annotations.DestructiveHint)
			}

			// ReadOnlyHint must match expected read-only state
			if annotations.ReadOnlyHint == nil {
				t.Error("expected readOnlyHint to be set, got nil")
			} else if *annotations.ReadOnlyHint != tc.wantReadOnly {
				t.Errorf("expected readOnlyHint=%v, got %v", tc.wantReadOnly, *annotations.ReadOnlyHint)
			}
		})
	}
}

func TestAllLookerToolsCustomAnnotations(t *testing.T) {
	// Verify that custom annotations (e.g. idempotentHint) are preserved,
	// while openWorldHint=false and destructiveHint are enforced.
	ctx := context.Background()
	yamlConfig := `
source: my-looker-source
description: test description
annotations:
  idempotentHint: true
  openWorldHint: true
`
	decoder := yaml.NewDecoder(strings.NewReader(yamlConfig))
	cfg, err := tools.DecodeConfig(ctx, "looker-get-models", "test_tool", decoder)
	if err != nil {
		t.Fatalf("failed to decode config: %v", err)
	}

	tool, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}

	annotations := tool.GetAnnotations(nil)
	if annotations == nil {
		t.Fatal("expected non-nil annotations")
	}
	if annotations.IdempotentHint == nil || *annotations.IdempotentHint != true {
		t.Errorf("expected idempotentHint=true preserved, got %v", annotations.IdempotentHint)
	}
	if annotations.OpenWorldHint == nil || *annotations.OpenWorldHint != false {
		t.Errorf("expected openWorldHint=false enforced, got %v", annotations.OpenWorldHint)
	}
	if annotations.DestructiveHint == nil || *annotations.DestructiveHint != false {
		t.Errorf("expected destructiveHint=false, got %v", annotations.DestructiveHint)
	}
}
