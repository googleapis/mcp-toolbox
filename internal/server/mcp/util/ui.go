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
	"fmt"

	"github.com/googleapis/mcp-toolbox/internal/server/primitives"
	"github.com/googleapis/mcp-toolbox/internal/tools"
)

// ResolveToolUIMetadata resolves a tool's UI resource and returns the UI metadata map.
func ResolveToolUIMetadata(pMgr *primitives.PrimitiveManager, tool tools.Tool) (map[string]any, error) {
	uiMetaOrig := tool.GetToolUIMetadata()
	if uiMetaOrig == nil || uiMetaOrig.Resource == "" {
		return nil, nil
	}

	var uri string
	if res, hasRes := pMgr.GetResource(uiMetaOrig.Resource); hasRes {
		uri = res.GetURI()
	} else if tmpl, hasTmpl := pMgr.GetResourceTemplate(uiMetaOrig.Resource); hasTmpl {
		uri = tmpl.GetURITemplate()
	} else if res, hasRes := pMgr.GetUIResourceFromURI(uiMetaOrig.Resource); hasRes {
		uri = res.GetURI()
	} else {
		return nil, fmt.Errorf("UI resource %q for tool %q is not registered", uiMetaOrig.Resource, tool.GetName())
	}

	uiMeta := map[string]any{
		"resourceUri": uri,
	}
	if len(uiMetaOrig.Visibility) > 0 {
		vis := make([]string, len(uiMetaOrig.Visibility))
		for i, v := range uiMetaOrig.Visibility {
			vis[i] = string(v)
		}
		uiMeta["visibility"] = vis
	}
	return uiMeta, nil
}
