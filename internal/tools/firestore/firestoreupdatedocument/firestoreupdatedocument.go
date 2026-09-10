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

package firestoreupdatedocument

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	firestoreapi "cloud.google.com/go/firestore"
	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	fsUtil "github.com/googleapis/mcp-toolbox/internal/tools/firestore/util"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

const resourceType string = "firestore-update-document"
const documentPathKey string = "documentPath"
const documentDataKey string = "documentData"
const updateMaskKey string = "updateMask"
const returnDocumentDataKey string = "returnData"

func init() {
	if !tools.Register(resourceType, newConfig) {
		panic(fmt.Sprintf("tool type %q already registered", resourceType))
	}
}

func newConfig(ctx context.Context, name string, decoder *yaml.Decoder) (tools.ToolConfig, error) {
	actual := Config{ConfigBase: tools.ConfigBase{Name: name}}
	if err := decoder.DecodeContext(ctx, &actual); err != nil {
		return nil, err
	}
	return actual, nil
}

type compatibleSource interface {
	FirestoreClient() *firestoreapi.Client
	FirestoreClientContext(context.Context) (*firestoreapi.Client, error)
	UpdateDocument(context.Context, string, []firestoreapi.Update, any, bool) (map[string]any, error)
}

type Config struct {
	tools.ConfigBase `yaml:",inline"`
	Type             string                 `yaml:"type" validate:"required"`
	Source           string                 `yaml:"source" validate:"required"`
	Annotations      *tools.ToolAnnotations `yaml:"annotations,omitempty"`
}

// validate interface
var _ tools.ToolConfig = Config{}

func (cfg Config) ToolConfigType() string {
	return resourceType
}

func (cfg Config) Initialize(context.Context) (tools.Tool, error) {
	if cfg.Description == "" {
		return nil, fmt.Errorf("description is required for tool %q", cfg.Name)
	}

	// Create parameters
	documentPathParameter := parameters.NewStringParameter(
		documentPathKey,
		"The relative path of the document which needs to be updated (e.g., 'users/userId' or 'users/userId/posts/postId'). Note: This is a relative path, NOT an absolute path like 'projects/{project_id}/databases/{database_id}/documents/...'",
	)

	documentDataParameter := parameters.NewMapParameter(
		documentDataKey,
		`The document data in Firestore's native JSON format. Each field must be wrapped with a type indicator:
- Strings: {"stringValue": "text"}
- Integers: {"integerValue": "123"} or {"integerValue": 123}
- Doubles: {"doubleValue": 123.45}
- Booleans: {"booleanValue": true}
- Timestamps: {"timestampValue": "2025-01-07T10:00:00Z"}
- GeoPoints: {"geoPointValue": {"latitude": 34.05, "longitude": -118.24}}
- Arrays: {"arrayValue": {"values": [{"stringValue": "item1"}, {"integerValue": "2"}]}}
- Maps: {"mapValue": {"fields": {"key1": {"stringValue": "value1"}, "key2": {"booleanValue": true}}}}
- Null: {"nullValue": null}
- Bytes: {"bytesValue": "base64EncodedString"}
- References: {"referenceValue": "collection/document"}`,
		"", // Empty string for generic map that accepts any value type
	)

	updateMaskParameter := parameters.NewArrayParameter(
		updateMaskKey,
		"The selective fields to update. If not provided, all fields in documentData will be updated. When provided, only the specified fields will be updated. Fields referenced in the mask but not present in documentData will be deleted from the document",
		parameters.NewStringParameter("field", "Field path to update or delete. Use dot notation to access nested fields within maps (e.g., 'address.city' to update the city field within an address map, or 'user.profile.name' for deeply nested fields). To delete a field, include it in the mask but omit it from documentData. Note: You cannot update individual array elements; you must update the entire array field"), parameters.WithArrayRequired(
			false))

	returnDataParameter := parameters.NewBooleanParameter(
		returnDocumentDataKey,
		"If set to true the output will have the data of the updated document. This flag if set to false will help avoid overloading the context of the agent.", parameters.WithBooleanDefault(
			false))

	params := parameters.Parameters{
		documentPathParameter,
		documentDataParameter,
		updateMaskParameter,
		returnDataParameter,
	}

	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, tools.NewDestructiveAnnotations),
			tools.Manifest{Description: cfg.Description, Parameters: params.Manifest(), AuthRequired: cfg.AuthRequired},
			params,
		),
	}, nil
}

// validate interface
var _ tools.Tool = Tool{}

type Tool struct {
	tools.BaseTool[Config]
}

func (t Tool) GetSourceName() string {
	return t.Cfg.Source
}

func (t Tool) ToConfig() tools.ToolConfig {
	return t.Cfg
}

func (t Tool) ValidateSource(source sources.Source) error {
	_, ok := source.(compatibleSource)
	if !ok {
		return fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	return nil
}

func (t Tool) Invoke(ctx context.Context, s sources.Source, params parameters.ParamValues, accessToken tools.AccessToken) (any, util.ToolboxError) {
	source, ok := s.(compatibleSource)
	if !ok {
		return nil, util.NewClientServerError("source used is not compatible with the tool", http.StatusInternalServerError, nil)
	}
	mapParams := params.AsMap()

	// Get document path
	documentPath, ok := mapParams[documentPathKey].(string)
	if !ok || documentPath == "" {
		return nil, util.NewAgentError(fmt.Sprintf("invalid or missing '%s' parameter", documentPathKey), nil)
	}

	// Validate document path
	if err := fsUtil.ValidateDocumentPath(documentPath); err != nil {
		return nil, util.NewAgentError(fmt.Sprintf("invalid document path: %v", err), err)
	}

	// Get document data
	documentDataRaw, ok := mapParams[documentDataKey]
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("invalid or missing '%s' parameter", documentDataKey), nil)
	}

	// Get update mask if provided
	var updatePaths []string
	if updateMaskRaw, ok := mapParams[updateMaskKey]; ok && updateMaskRaw != nil {
		if updateMaskArray, ok := updateMaskRaw.([]any); ok {
			// Use ConvertAnySliceToTyped to convert the slice
			typedSlice, err := parameters.ConvertAnySliceToTyped(updateMaskArray, "string")
			if err != nil {
				return nil, util.NewAgentError(fmt.Sprintf("failed to convert update mask: %v", err), err)
			}
			updatePaths, ok = typedSlice.([]string)
			if !ok {
				return nil, util.NewAgentError("unexpected type conversion error for update mask", nil)
			}
		}
	}
	// Use selective field update with update mask
	updates := make([]firestoreapi.Update, 0, len(updatePaths))
	var documentData any
	client, err := source.FirestoreClientContext(ctx)
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}
	if len(updatePaths) > 0 {

		// Convert document data without delete markers
		dataMap, err := fsUtil.JSONToFirestoreValue(documentDataRaw, client)
		if err != nil {
			return nil, util.NewAgentError(fmt.Sprintf("failed to convert document data: %v", err), err)
		}

		// Ensure it's a map
		dataMapTyped, ok := dataMap.(map[string]interface{})
		if !ok {
			return nil, util.NewAgentError("document data must be a map", nil)
		}

		for _, path := range updatePaths {
			// Get the value for this path from the document data
			value, exists := getFieldValue(dataMapTyped, path)
			if !exists {
				// Field not in document data but in mask - delete it
				value = firestoreapi.Delete
			}

			updates = append(updates, firestoreapi.Update{
				Path:  path,
				Value: value,
			})
		}
	} else {
		// Update all fields in the document data (merge)
		documentData, err = fsUtil.JSONToFirestoreValue(documentDataRaw, client)
		if err != nil {
			return nil, util.NewAgentError(fmt.Sprintf("failed to convert document data: %v", err), err)
		}
	}

	// Get return document data flag
	returnData := false
	if val, ok := mapParams[returnDocumentDataKey].(bool); ok {
		returnData = val
	}
	resp, err := source.UpdateDocument(ctx, documentPath, updates, documentData, returnData)
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}
	return resp, nil
}

// getFieldValue retrieves a value from a nested map using a dot-separated path
func getFieldValue(data map[string]interface{}, path string) (interface{}, bool) {
	// Split the path by dots for nested field access
	parts := strings.Split(path, ".")

	current := data
	for i, part := range parts {
		if i == len(parts)-1 {
			// Last part - return the value
			if value, exists := current[part]; exists {
				return value, true
			}
			return nil, false
		}

		// Navigate deeper into the structure
		if next, ok := current[part].(map[string]interface{}); ok {
			current = next
		} else {
			return nil, false
		}
	}

	return nil, false
}
