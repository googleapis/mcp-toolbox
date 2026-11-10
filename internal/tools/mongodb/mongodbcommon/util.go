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

// Package mongodbcommon holds helpers shared across the MongoDB tools.
package mongodbcommon

import (
	"fmt"

	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

// CollectionKey is the runtime parameter name used to pick a collection.
const CollectionKey string = "collection"

// ValidateCollectionConfig rejects setting collection and collectionAllowedValues together.
func ValidateCollectionConfig(collection string, allowedValues []string) error {
	if collection != "" && len(allowedValues) > 0 {
		return fmt.Errorf("only one of 'collection' or 'collectionAllowedValues' can be set, not both")
	}
	return nil
}

// WithRuntimeCollectionParam adds a required collection parameter when none is set in config.
func WithRuntimeCollectionParam(collection string, allowedValues []string, params parameters.Parameters) parameters.Parameters {
	if collection != "" {
		return params
	}
	opts := []parameters.StringParameterOption{parameters.WithStringRequired(true)}
	if len(allowedValues) > 0 {
		allowed := make([]any, len(allowedValues))
		for i, v := range allowedValues {
			allowed[i] = v
		}
		opts = append(opts, parameters.WithStringAllowedValues(allowed))
	}
	collectionParam := parameters.NewStringParameter(CollectionKey, "The name of the collection to operate on.", opts...)
	return append(params, collectionParam)
}

// ResolveCollection returns the configured collection, falling back to the runtime parameter.
func ResolveCollection(collection string, paramsMap map[string]any) (string, util.ToolboxError) {
	if collection != "" {
		return collection, nil
	}
	c, ok := paramsMap[CollectionKey].(string)
	if !ok || c == "" {
		return "", util.NewAgentError("collection must be set in the tool config or provided as a parameter", nil)
	}
	return c, nil
}
