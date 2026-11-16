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

package server

import (
	"context"
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/resources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
)

type mockResourceProviderTool struct {
	tools.Tool
	name      string
	resources []resources.Resource
}

func (m *mockResourceProviderTool) GetName() string {
	return m.name
}

func (m *mockResourceProviderTool) GetResources() []resources.Resource {
	return m.resources
}

func TestRegisterDynamicResources(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("failed to create logger context: %v", err)
	}
	l, err := util.LoggerFromContext(ctx)
	if err != nil {
		t.Fatalf("failed to get logger from context: %v", err)
	}

	readFn := func(ctx context.Context) (string, error) {
		return "<html></html>", nil
	}

	existingRes := resources.NewDynamicUIResource("existing_res", "ui://test/existing.html", "existing", nil, nil, readFn)
	validRes := resources.NewDynamicUIResource("valid_res", "ui://test/valid.html", "valid", nil, nil, readFn)
	emptyNameRes := resources.NewDynamicUIResource("", "ui://test/empty_name.html", "empty name", nil, nil, readFn)
	emptyURIRes := resources.NewDynamicUIResource("empty_uri_res", "", "empty uri", nil, nil, readFn)
	invalidURIRes := resources.NewDynamicUIResource("invalid_uri_res", "://bad-uri", "invalid uri", nil, nil, readFn)
	wrongSchemeRes := resources.NewDynamicUIResource("wrong_scheme_res", "https://example.com/ui.html", "wrong scheme", nil, nil, readFn)
	duplicateRes := resources.NewDynamicUIResource("existing_res", "ui://test/dup.html", "duplicate", nil, nil, readFn)

	toolsMap := map[string]tools.Tool{
		"mock_tool": &mockResourceProviderTool{
			name: "mock_tool",
			resources: []resources.Resource{
				validRes,
				emptyNameRes,
				emptyURIRes,
				invalidURIRes,
				wrongSchemeRes,
				duplicateRes,
			},
		},
	}

	resourcesMap := map[string]resources.Resource{
		"existing_res": existingRes,
	}

	registerDynamicResources(ctx, toolsMap, resourcesMap, l)

	if len(resourcesMap) != 2 {
		t.Fatalf("expected 2 resources in resourcesMap, got %d: %v", len(resourcesMap), resourcesMap)
	}
	if resourcesMap["valid_res"] != validRes {
		t.Errorf("expected valid_res to be registered")
	}
	if resourcesMap["existing_res"] != existingRes {
		t.Errorf("expected existing_res not to be overwritten by duplicate")
	}
}
