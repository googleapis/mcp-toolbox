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
	"fmt"
	"strconv"
	"strings"

	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

// PopulateUrlParams injects bound URL query parameters into the data arguments
// and performs automatic type conversion for integer, boolean, and float parameters.
func PopulateUrlParams(ctx context.Context, data map[string]any, toolParams parameters.Parameters) (map[string]any, error) {
	urlParams, ok := util.UrlParamsFromContext(ctx)
	if !ok {
		return data, nil
	}
	if data == nil {
		data = make(map[string]any)
	}
	logger, _ := util.LoggerFromContext(ctx)

	for name, val := range urlParams {
		// Reject if client supplied a parameter bound to URL.
		if _, exists := data[name]; exists {
			return nil, util.NewAgentError(fmt.Sprintf("parameter %q is bound by URL and cannot be provided in client arguments", name), nil)
		}
		data[name] = val

		// Attempt type conversion for known parameters
		found := false
		for _, p := range toolParams {
			if p.GetName() == name {
				found = true
				switch p.GetType() {
				case "integer":
					if i, err := strconv.Atoi(val); err == nil {
						data[name] = i
					} else if logger != nil {
						logger.WarnContext(ctx, "failed to convert URL parameter to integer", "parameter", name, "value", val, "error", err)
					}
				case "boolean":
					if b, err := strconv.ParseBool(val); err == nil {
						data[name] = b
					} else if logger != nil {
						logger.WarnContext(ctx, "failed to convert URL parameter to boolean", "parameter", name, "value", val, "error", err)
					}
				case "float":
					if f, err := strconv.ParseFloat(val, 64); err == nil {
						data[name] = f
					} else if logger != nil {
						logger.WarnContext(ctx, "failed to convert URL parameter to float", "parameter", name, "value", val, "error", err)
					}
				case "array":
					var arr []any
					if err := util.DecodeJSON(strings.NewReader(val), &arr); err == nil {
						data[name] = arr
					} else if logger != nil {
						logger.WarnContext(ctx, "failed to convert URL parameter to array", "parameter", name, "value", val, "error", err)
					}
				case "map":
					var m map[string]any
					if err := util.DecodeJSON(strings.NewReader(val), &m); err == nil {
						data[name] = m
					} else if logger != nil {
						logger.WarnContext(ctx, "failed to convert URL parameter to map", "parameter", name, "value", val, "error", err)
					}
				}
				break
			}
		}
		if !found && logger != nil {
			logger.WarnContext(ctx, "URL parameter not defined in tool parameters", "parameter", name)
		}
	}
	return data, nil
}
