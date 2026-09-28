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

package postgresdeletememory

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	yaml "github.com/goccy/go-yaml"
	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/memory"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

const resourceType string = "postgres-delete-memory"

// defaultDescription gives LLMs clear prompt guidance on when to invoke this tool, and is reused across postgres prebuilts.
const defaultDescription = `Permanently delete an obsolete, incorrect, or user-retracted memory by its memory_id.
Before calling this tool, use search_memory to locate the target memory_id. Prefer update_memory when a stored fact has changed rather than deleting and recreating it. Only memories created by the current user can be deleted.`

func init() {
	if !tools.Register(resourceType, newConfig) {
		panic(fmt.Sprintf("tool type %q already registered", resourceType))
	}
}

func newConfig(ctx context.Context, name string, decoder *yaml.Decoder) (tools.ToolConfig, error) {
	actual := Config{Config: memory.Config{ConfigBase: tools.ConfigBase{Name: name}}}
	if err := decoder.DecodeContext(ctx, &actual); err != nil {
		return nil, err
	}
	return actual, nil
}

// Config represents the YAML configuration for postgres-delete-memory, embedding shared table/auth settings.
type Config struct {
	memory.Config `yaml:",inline"`
}

var _ tools.ToolConfig = Config{}

func (cfg Config) ToolConfigType() string {
	return resourceType
}

func (cfg Config) Initialize(context.Context) (tools.Tool, error) {
	resolved, err := cfg.Resolve()
	if err != nil {
		return nil, err
	}
	cfg.Config = resolved
	if cfg.Description == "" {
		cfg.Description = defaultDescription
	}

	allParameters := parameters.Parameters{
		parameters.NewStringParameter(
			"memory_id",
			"The unique ID of the memory to delete.",
			parameters.WithStringRequired(true),
		),
	}
	if userParam := cfg.UserIDParameter(); userParam != nil {
		allParameters = append(allParameters, userParam)
	}

	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, tools.NewDestructiveAnnotations),
			tools.Manifest{Description: cfg.Description, Parameters: allParameters.Manifest(), AuthRequired: cfg.AuthRequired},
			allParameters,
		),
	}, nil
}

var _ tools.Tool = Tool{}

type Tool struct {
	tools.BaseTool[Config]
}

type Result struct {
	Status  string         `json:"status"`
	Message string         `json:"message"`
	Memory  *memory.Memory `json:"memory,omitempty"`
}

func (t Tool) Invoke(ctx context.Context, s sources.Source, params parameters.ParamValues, accessToken tools.AccessToken) (any, util.ToolboxError) {
	logger, _ := util.LoggerFromContext(ctx)
	source, ok := s.(memory.CompatibleSource)
	if !ok {
		err := fmt.Errorf("source %q is not compatible with postgres-delete-memory (requires PostgresPool)", t.Cfg.Source)
		if logger != nil {
			logger.ErrorContext(ctx, err.Error())
		}
		return nil, util.NewClientServerError(err.Error(), http.StatusInternalServerError, err)
	}
	pool := source.PostgresPool()
	table := t.Cfg.TableName
	p := params.AsMap()

	rawID := strings.TrimSpace(asString(p["memory_id"]))
	parsedID, err := uuid.Parse(rawID)
	if err != nil {
		return nil, util.NewAgentError(fmt.Sprintf("memory_id %q is not a valid UUID", rawID), nil)
	}
	memoryID := parsedID.String()

	userID, err := t.Cfg.ResolveUserID(params)
	if err != nil {
		if logger != nil {
			logger.ErrorContext(ctx, err.Error())
		}
		return nil, util.NewClientServerError(err.Error(), http.StatusBadRequest, err)
	}

	if logger != nil {
		logger.InfoContext(ctx, fmt.Sprintf("delete_memory: preparing delete for user=%q, memory_id=%s", userID, memoryID))
	}

	// Lazy bootstrap to create the memory table if it doesn't exist yet.
	if err := memory.EnsureMemoryTable(ctx, pool, table); err != nil {
		if logger != nil {
			logger.ErrorContext(ctx, fmt.Sprintf("delete_memory: table bootstrap failed: %v", err))
		}
		return nil, util.NewAgentError(fmt.Sprintf("database setup error: %v", err), err)
	}

	deleteSQL := fmt.Sprintf(`DELETE FROM %s
WHERE memory_id = $1::uuid AND user_id = $2
RETURNING %s`, table, memory.Columns)

	rows, err := pool.Query(ctx, deleteSQL, memoryID, userID)
	if err != nil {
		if logger != nil {
			logger.ErrorContext(ctx, fmt.Sprintf("delete_memory: DELETE failed: %v", err))
		}
		return nil, util.ProcessGeneralError(fmt.Errorf("unable to delete memory: %w", err))
	}
	deleted, err := memory.CollectMemories(rows)
	if err != nil {
		if logger != nil {
			logger.ErrorContext(ctx, fmt.Sprintf("delete_memory: reading deleted memory row failed: %v", err))
		}
		return nil, util.ProcessGeneralError(err)
	}
	if len(deleted) == 0 {
		return nil, util.NewAgentError(fmt.Sprintf("memory %q was not found or was not created by the current user", rawID), nil)
	}

	if logger != nil {
		logger.InfoContext(ctx, fmt.Sprintf("delete_memory: successfully deleted memory id=%s", deleted[0].MemoryID))
	}

	return Result{
		Status:  "deleted",
		Message: "Memory deleted.",
		Memory:  &deleted[0],
	}, nil
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func (t Tool) GetSourceName() string {
	return t.Cfg.Source
}

func (t Tool) ToConfig() tools.ToolConfig {
	return t.Cfg
}

func (t Tool) ValidateSource(source sources.Source) error {
	return t.Cfg.Validate(source)
}
