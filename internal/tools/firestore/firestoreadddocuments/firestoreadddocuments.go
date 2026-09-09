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

package firestoreadddocuments

import (
	"context"
	"fmt"
	"net/http"

	firestoreapi "cloud.google.com/go/firestore"
	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	fsUtil "github.com/googleapis/mcp-toolbox/internal/tools/firestore/util"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

const resourceType string = "firestore-add-documents"
const collectionPathKey string = "collectionPath"
const documentDataKey string = "documentData"
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
	AddDocuments(context.Context, string, any, bool) (map[string]any, error)
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
	collectionPathParameter := parameters.NewStringParameter(
		collectionPathKey,
		"The relative path of the collection where the document will be added to (e.g., 'users' or 'users/userId/posts'). Note: This is a relative path, NOT an absolute path like 'projects/{project_id}/databases/{database_id}/documents/...'",
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

	returnDataParameter := parameters.NewBooleanParameter(
		returnDocumentDataKey,
		"If set to true the output will have the data of the created document. This flag if set to false will help avoid overloading the context of the agent.", parameters.WithBooleanDefault(
			false))

	params := parameters.Parameters{
		collectionPathParameter,
		documentDataParameter,
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
	// Get collection path
	collectionPath, ok := mapParams[collectionPathKey].(string)
	if !ok || collectionPath == "" {
		return nil, util.NewAgentError(fmt.Sprintf("invalid or missing '%s' parameter", collectionPathKey), nil)
	}
	// Validate collection path
	if err := fsUtil.ValidateCollectionPath(collectionPath); err != nil {
		return nil, util.NewAgentError(fmt.Sprintf("invalid collection path: %v", err), err)
	}
	// Get document data
	documentDataRaw, ok := mapParams[documentDataKey]
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("invalid or missing '%s' parameter", documentDataKey), nil)
	}
	// Convert the document data from JSON format to Firestore format
	// The client is passed to handle referenceValue types
	client, err := source.FirestoreClientContext(ctx)
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}
	documentData, err := fsUtil.JSONToFirestoreValue(documentDataRaw, client)
	if err != nil {
		return nil, util.NewAgentError(fmt.Sprintf("failed to convert document data: %v", err), err)
	}

	// Get return document data flag
	returnData := false
	if val, ok := mapParams[returnDocumentDataKey].(bool); ok {
		returnData = val
	}
	resp, err := source.AddDocuments(ctx, collectionPath, documentData, returnData)
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}
	return resp, nil
}
