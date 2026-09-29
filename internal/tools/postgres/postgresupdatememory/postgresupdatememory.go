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
	"github.com/googleapis/mcp-toolbox/internal/embeddingmodels"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/memory"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

const resourceType string = "postgres-update-memory"

// defaultDescription gives LLMs clear prompt guidance on when to invoke this tool, and is reused across postgres prebuilts.
// The category list is derived from DefaultCategories so it stays the single source of truth.
var defaultDescription = fmt.Sprintf(`Modify the content, category, or pinned status of an existing memory by its memory_id. Use this when a stored fact changes or needs correcting, instead of creating a new, conflicting memory.
Before calling this tool, use search_memory to locate the target memory_id and verify that the updated fact does not duplicate another existing memory. Only memories created by the current user can be updated.

Provide at least one of content, category or is_pinned; omitted fields are left unchanged. Keep content short, self-contained and in the third person. When changing category, choose exactly one of: %q.`, memory.DefaultCategories)

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
	if embParam := cfg.EmbeddingParameter("content_embedding", "content"); embParam != nil {
		allParameters = append(allParameters, embParam)
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

func (t Tool) EmbedParams(ctx context.Context, paramValues parameters.ParamValues, pMgr tools.PrimitiveManagerI) (parameters.ParamValues, error) {
	return parameters.EmbedParams(ctx, t.StaticParameters, paramValues, pMgr, embeddingmodels.FormatVectorForPgvector)
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

	// Omitted optional parameters arrive as nil and leave the current column value unchanged.
	var content, category *string
	var isPinned *bool
	var fields []string
	if v, ok := p["content"].(string); ok {
		v = strings.TrimSpace(v)
		if v == "" {
			return nil, util.NewAgentError("content must not be empty", nil)
		}
		content = &v
		fields = append(fields, "content")
	}
	if v, ok := p["category"].(string); ok {
		v = strings.TrimSpace(v)
		category = &v
		fields = append(fields, "category")
	}
	if v, ok := p["is_pinned"].(bool); ok {
		isPinned = &v
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

	useVector := t.Cfg.EmbeddingModel != ""
	var contentEmbedding, embeddingModel *string
	if useVector && content != nil {
		emb := strings.TrimSpace(asString(p["content_embedding"]))
		if emb == "" {
			return nil, util.NewClientServerError("missing content_embedding parameter value", http.StatusInternalServerError, nil)
		}
		contentEmbedding = &emb
		modelName := t.Cfg.EmbeddingModel
		embeddingModel = &modelName
	}

	if logger != nil {
		logger.InfoContext(ctx, fmt.Sprintf("update_memory: preparing update for user=%q, memory_id=%s, fields=%v, vector=%t", userID, memoryID, fields, useVector))
	}

	// Lazy bootstrap to create the memory table (and vector extension/column when configured) if it doesn't exist yet.
	if err := memory.EnsureMemoryTable(ctx, pool, table, useVector); err != nil {
		if logger != nil {
			logger.ErrorContext(ctx, fmt.Sprintf("update_memory: table bootstrap failed: %v", err))
		}
		return nil, util.NewAgentError(fmt.Sprintf("database setup error: %v", err), err)
	}

	// Apply the provided fields and refresh salience; omitted fields are nil and left unchanged via COALESCE.
	// tsv is a generated column and is recomputed automatically; when vector search is enabled and content
	// changed, embedding and embedding_model are updated. In FTS mode, changing content resets embedding_model
	// to NULL so any stale embedding from a prior model run is invalidated.
	var (
		update     string
		updateArgs []any
	)
	if useVector {
		update = fmt.Sprintf(`UPDATE %s
SET content = COALESCE($3, content),
    category = COALESCE($4, category),
    is_pinned = COALESCE($5, is_pinned),
    embedding = COALESCE($6::vector, embedding),
    embedding_model = COALESCE($7, embedding_model),
    updated_at = NOW(),
    last_accessed_at = NOW(),
    access_count = access_count + 1
WHERE memory_id = $1::uuid AND user_id = $2
RETURNING %s`, table, memory.Columns)
		updateArgs = []any{memoryID, userID, content, category, isPinned, contentEmbedding, embeddingModel}
	} else {
		update = fmt.Sprintf(`UPDATE %s
SET content = COALESCE($3, content),
    category = COALESCE($4, category),
    is_pinned = COALESCE($5, is_pinned),
    embedding_model = CASE WHEN $3 IS NOT NULL THEN NULL ELSE embedding_model END,
    updated_at = NOW(),
    last_accessed_at = NOW(),
    access_count = access_count + 1
WHERE memory_id = $1::uuid AND user_id = $2
RETURNING %s`, table, memory.Columns)
		updateArgs = []any{memoryID, userID, content, category, isPinned}
	}

	rows, err := pool.Query(ctx, update, updateArgs...)
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
