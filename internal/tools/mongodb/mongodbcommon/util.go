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
	"slices"

	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

// CollectionKey is the runtime parameter name used to pick a collection.
const CollectionKey string = "collection"

func anySlice(vs []string) []any {
	out := make([]any, len(vs))
	for i, v := range vs {
		out[i] = v
	}
	return out
}

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
		opts = append(opts, parameters.WithStringAllowedValues(anySlice(allowedValues)))
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

// CollectionScopedSource is the part of a MongoDB source that collection scoping needs.
type CollectionScopedSource interface {
	IsCollectionAllowed(database, collection string) bool
	MongoDBAllowedCollections(database string) []string
}

// EffectiveCollections narrows the source's allow-list with the tool's collectionAllowedValues, which may hold regexes.
// A nil sourceAllowed means the source is unrestricted; an empty non-nil one means it allows nothing in this database.
func EffectiveCollections(sourceAllowed, toolAllowed []string) []string {
	if sourceAllowed == nil {
		return toolAllowed
	}
	if len(toolAllowed) == 0 {
		return sourceAllowed
	}
	effective := make([]string, 0, len(sourceAllowed))
	for _, c := range sourceAllowed {
		for _, pattern := range toolAllowed {
			if parameters.MatchStringOrRegex(c, pattern) {
				effective = append(effective, c)
				break
			}
		}
	}
	return effective
}

// ValidateCollectionScope checks a tool's collection config against the source's allow-list so a bad config fails at startup.
func ValidateCollectionScope(src CollectionScopedSource, database, collection string, toolAllowed []string) error {
	sourceAllowed := src.MongoDBAllowedCollections(database)
	if sourceAllowed == nil {
		return nil
	}
	if collection != "" {
		if !src.IsCollectionAllowed(database, collection) {
			return fmt.Errorf("collection %q is not in the allowedCollections of database %q on this source", collection, database)
		}
		return nil
	}
	if len(EffectiveCollections(sourceAllowed, toolAllowed)) == 0 {
		return fmt.Errorf("no collection in database %q is both allowed by the source and listed in collectionAllowedValues", database)
	}
	return nil
}

// ScopeCollectionParam returns a copy of params with the runtime collection parameter restricted to allowed.
// The tools narrow their already-built parameters rather than rebuilding them, because some construct their
// non-collection parameters locally in Initialize and could not reproduce them from config alone.
func ScopeCollectionParam(params parameters.Parameters, allowed []string) parameters.Parameters {
	if len(allowed) == 0 {
		return params
	}
	scoped := slices.Clone(params)
	for i, p := range scoped {
		sp, ok := p.(*parameters.StringParameter)
		if !ok || sp.GetName() != CollectionKey {
			continue
		}
		narrowed := *sp
		narrowed.AllowedValues = anySlice(allowed)
		scoped[i] = &narrowed
	}
	return scoped
}
