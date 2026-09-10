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

package iceberggettableinfo

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/apache/iceberg-go/catalog"
	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

const resourceType string = "iceberg-get-table-info"
const namespaceKey string = "namespace"
const tableKey string = "table"

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
	IcebergCatalog() catalog.Catalog
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

	namespaceParameter := parameters.NewStringParameter(namespaceKey,
		"The namespace the table belongs to, with levels separated by dots.")
	tableParameter := parameters.NewStringParameter(tableKey, "The name of the table.")
	params := parameters.Parameters{namespaceParameter, tableParameter}

	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, tools.NewReadOnlyAnnotations),
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
	namespace, ok := mapParams[namespaceKey].(string)
	if !ok || namespace == "" {
		return nil, util.NewAgentError(fmt.Sprintf("invalid or missing '%s' parameter; expected a non-empty string", namespaceKey), nil)
	}
	tableName, ok := mapParams[tableKey].(string)
	if !ok || tableName == "" {
		return nil, util.NewAgentError(fmt.Sprintf("invalid or missing '%s' parameter; expected a non-empty string", tableKey), nil)
	}

	identifier := append(strings.Split(namespace, "."), tableName)
	tbl, err := source.IcebergCatalog().LoadTable(ctx, identifier)
	if err != nil {
		return nil, util.ProcessGeneralError(err)
	}

	// Everything below comes back inline with the load-table response; nothing
	// reads data files from the warehouse.
	info := map[string]any{
		"table":             strings.Join(identifier, "."),
		"location":          tbl.Location(),
		"metadata-location": tbl.MetadataLocation(),
		"schema":            tbl.Schema(),
		"partition-spec":    tbl.Spec(),
		"sort-order":        tbl.SortOrder(),
		"properties":        tbl.Properties(),
	}

	// A freshly created table has no snapshot yet; only the operation key of a
	// snapshot summary is guaranteed by the Iceberg spec, so the summary is
	// passed through as-is rather than requiring specific metric keys.
	if snapshot := tbl.CurrentSnapshot(); snapshot != nil {
		currentSnapshot := map[string]any{
			"snapshot-id":  snapshot.SnapshotID,
			"timestamp-ms": snapshot.TimestampMs,
		}
		if snapshot.Summary != nil {
			currentSnapshot["summary"] = snapshot.Summary
		}
		info["current-snapshot"] = currentSnapshot
	}

	return info, nil
}
