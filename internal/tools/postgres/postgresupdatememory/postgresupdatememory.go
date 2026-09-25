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

package postgresupdatememory

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

const resourceType string = "postgres-update-memory"

// defaultDescription gives LLMs clear prompt guidance on when to invoke this tool, and is reused across postgres prebuilts.
// The category list is derived from DefaultCategories so it stays the single source of truth.
var defaultDescription = fmt.Sprintf(`Modify the content, category, or pinned status of an existing memory by its memory_id. Use this when a stored fact changes or needs correcting, instead of creating a new, conflicting memory. Only memories created by the current user can be updated.

Provide at least one of content, category or is_pinned; omitted fields are left unchanged.
- status "updated": the memory was updated.
- status "duplicate": another memory in the resulting category already has identical content; the update was NOT applied, and that memory is returned with its salience refreshed.

Keep content short, self-contained and in the third person. When changing category, choose exactly one of: %q.`, memory.DefaultCategories)

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

// Config represents the YAML configuration for postgres-update-memory, embedding shared table/auth settings.
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
			"The unique ID of the memory to update.",
			parameters.WithStringRequired(true),
		),
		parameters.NewStringParameter(
			"content",
			"The updated discrete fact or constraint to remember. Omit to keep the current content.",
			parameters.WithStringRequired(false),
		),
		parameters.NewStringParameter(
			"category",
			fmt.Sprintf("The updated classification tag. Must be one of: %q. Omit to keep the current category.", memory.DefaultCategories),
			parameters.WithStringRequired(false),
			parameters.WithStringAllowedValues(memory.DefaultCategories),
		),
		parameters.NewBooleanParameter(
			"is_pinned",
			"The updated pinned status of the memory. Set to true to prioritize this memory permanently. Omit to keep the current status.",
			parameters.WithBooleanRequired(false),
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
		err := fmt.Errorf("source %q is not compatible with postgres-update-memory (requires PostgresPool)", t.Cfg.Source)
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

	// Omitted optional parameters arrive as nil; a nil value is passed as NULL so COALESCE keeps the current column value.
	var content, category, isPinned any
	var fields []string
	if v, ok := p["content"].(string); ok {
		v = strings.TrimSpace(v)
		if v == "" {
			return nil, util.NewAgentError("content must not be empty", nil)
		}
		content = v
		fields = append(fields, "content")
	}
	if v, ok := p["category"].(string); ok {
		category = v
		fields = append(fields, "category")
	}
	if v, ok := p["is_pinned"].(bool); ok {
		isPinned = v
		fields = append(fields, "is_pinned")
	}
	if len(fields) == 0 {
		return nil, util.NewAgentError("at least one of content, category or is_pinned must be provided", nil)
	}

	userID, err := t.Cfg.ResolveUserID(params)
	if err != nil {
		if logger != nil {
			logger.ErrorContext(ctx, err.Error())
		}
		return nil, util.NewClientServerError(err.Error(), http.StatusBadRequest, err)
	}

	if logger != nil {
		logger.InfoContext(ctx, fmt.Sprintf("update_memory: preparing update for user=%q, memory_id=%s, fields=%v", userID, memoryID, fields))
	}

	// Lazy bootstrap to create the memory table if it doesn't exist yet.
	if err := memory.EnsureMemoryTable(ctx, pool, table); err != nil {
		if logger != nil {
			logger.ErrorContext(ctx, fmt.Sprintf("update_memory: table bootstrap failed: %v", err))
		}
		return nil, util.NewAgentError(fmt.Sprintf("database setup error: %v", err), err)
	}

	// A new content or category must not duplicate another visible memory, mirroring create_memory's check.
	// The target CTE computes the post-update values and only matches a memory owned by the caller.
	if content != nil || category != nil {
		dupQuery := fmt.Sprintf(`WITH target AS (
    SELECT COALESCE($3, content) AS new_content, COALESCE($4, category) AS new_category
    FROM %[1]s
    WHERE memory_id = $1::uuid AND user_id = $2
)
SELECT %[2]s
FROM %[1]s, target
WHERE %[3]s AND memory_id <> $1::uuid
    AND category = target.new_category AND LOWER(TRIM(content)) = LOWER(TRIM(target.new_content))
LIMIT 1`, table, memory.Columns, memory.ScopeClause("$2"))

		rows, err := pool.Query(ctx, dupQuery, memoryID, userID, content, category)
		if err != nil {
			if logger != nil {
				logger.ErrorContext(ctx, fmt.Sprintf("update_memory: duplicate check query failed: %v", err))
			}
			return nil, util.ProcessGeneralError(fmt.Errorf("unable to check for duplicate memories: %w", err))
		}
		existing, err := memory.CollectMemories(rows)
		if err != nil {
			if logger != nil {
				logger.ErrorContext(ctx, fmt.Sprintf("update_memory: reading duplicate rows failed: %v", err))
			}
			return nil, util.ProcessGeneralError(err)
		}
		if len(existing) > 0 {
			dup := existing[0]
			if logger != nil {
				logger.InfoContext(ctx, fmt.Sprintf("update_memory: update of id=%s would duplicate id=%s; refreshing salience of the existing memory", memoryID, dup.MemoryID))
			}

			refreshed, err := memory.Touch(ctx, pool, table, dup.MemoryID)
			if err != nil {
				if logger != nil {
					logger.ErrorContext(ctx, fmt.Sprintf("update_memory: refresh existing memory failed: %v", err))
				}
				return nil, util.ProcessGeneralError(err)
			}
			return Result{
				Status:  "duplicate",
				Message: "An equivalent memory already exists in this category. The update was not applied; the existing memory's access count and last_accessed_at were refreshed.",
				Memory:  &refreshed,
			}, nil
		}
	}

	// Only the owner can update a memory, so other users' memories fail with the same not-found error as unknown IDs.
	// Editing a memory also refreshes its salience; tsv is a generated column and is recomputed automatically.
	update := fmt.Sprintf(`UPDATE %s
SET content = COALESCE($3, content),
    category = COALESCE($4, category),
    is_pinned = COALESCE($5, is_pinned),
    updated_at = NOW(),
    last_accessed_at = NOW(),
    access_count = access_count + 1
WHERE memory_id = $1::uuid AND user_id = $2
RETURNING %s`, table, memory.Columns)

	rows, err := pool.Query(ctx, update, memoryID, userID, content, category, isPinned)
	if err != nil {
		if logger != nil {
			logger.ErrorContext(ctx, fmt.Sprintf("update_memory: UPDATE failed: %v", err))
		}
		return nil, util.ProcessGeneralError(fmt.Errorf("unable to update memory: %w", err))
	}
	updated, err := memory.CollectMemories(rows)
	if err != nil {
		if logger != nil {
			logger.ErrorContext(ctx, fmt.Sprintf("update_memory: reading updated memory row failed: %v", err))
		}
		return nil, util.ProcessGeneralError(err)
	}
	if len(updated) == 0 {
		return nil, util.NewAgentError(fmt.Sprintf("memory %q was not found or was not created by the current user", rawID), nil)
	}

	if logger != nil {
		logger.InfoContext(ctx, fmt.Sprintf("update_memory: successfully updated memory id=%s", updated[0].MemoryID))
	}

	return Result{
		Status:  "updated",
		Message: "Memory updated.",
		Memory:  &updated[0],
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
