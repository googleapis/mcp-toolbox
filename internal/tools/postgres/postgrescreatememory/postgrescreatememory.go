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

package postgrescreatememory

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/memory"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

const resourceType string = "postgres-create-memory"

// defaultDescription gives LLMs clear prompt guidance on when to invoke this tool, and is reused across postgres prebuilts.
// The category list is derived from DefaultCategories so it stays the single source of truth.
var defaultDescription = fmt.Sprintf(`Persist a single, discrete fact about the user, their project, or their preferences so it can be recalled in future sessions (e.g. "All timestamps in the orders DB are stored in EST", "User prefers tabs over spaces in Go", "Fix for build error X is Y").

Before inserting, the tool checks for existing memories in the same category with identical content:
- status "created": the memory was stored.
- status "duplicate": an equivalent memory already exists; it was NOT re-created and its salience was refreshed instead.

Keep content short, self-contained and in the third person. Choose exactly one category of the following: %q.`, memory.DefaultCategories)

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

// Config represents the YAML configuration for postgres-create-memory, embedding shared table/auth settings.
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
			"content",
			"The discrete fact or constraint to remember.",
			parameters.WithStringRequired(true),
		),
		parameters.NewStringParameter(
			"category",
			fmt.Sprintf("Classification tag for the memory. Must be one of: %q.", memory.DefaultCategories),
			parameters.WithStringRequired(true),
			parameters.WithStringAllowedValues(memory.DefaultCategories),
		),
		parameters.NewBooleanParameter(
			"is_global",
			"Set to true to make this memory visible to all users. Defaults to false (private to current user).",
			parameters.WithBooleanDefault(false),
		),
		parameters.NewBooleanParameter(
			"is_pinned",
			"Set to true to prioritize this memory permanently.",
			parameters.WithBooleanDefault(false),
		),
	}
	if userParam := cfg.UserIDParameter(); userParam != nil {
		allParameters = append(allParameters, userParam)
	}

	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, tools.NewWriteAnnotations),
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
		err := fmt.Errorf("source %q is not compatible with postgres-create-memory (requires PostgresPool)", t.Cfg.Source)
		if logger != nil {
			logger.ErrorContext(ctx, err.Error())
		}
		return nil, util.NewClientServerError(err.Error(), http.StatusInternalServerError, err)
	}
	pool := source.PostgresPool()
	table := t.Cfg.TableName
	p := params.AsMap()

	content := strings.TrimSpace(asString(p["content"]))
	if content == "" {
		return nil, util.NewAgentError("content must not be empty", nil)
	}
	category := strings.TrimSpace(asString(p["category"]))
	if category == "" {
		return nil, util.NewAgentError("category must not be empty", nil)
	}
	isGlobal, _ := p["is_global"].(bool)
	userID, err := t.Cfg.ResolveUserID(params)
	if err != nil {
		if logger != nil {
			logger.ErrorContext(ctx, err.Error())
		}
		return nil, util.NewClientServerError(err.Error(), http.StatusBadRequest, err)
	}
	isPinned, _ := p["is_pinned"].(bool)

	if logger != nil {
		logger.InfoContext(ctx, fmt.Sprintf("create_memory: preparing insert for user=%q, category=%q, is_global=%t, content_len=%d", userID, category, isGlobal, len(content)))
	}

	// Lazy bootstrap to create the memory table if it doesn't exist yet.
	if err := memory.EnsureMemoryTable(ctx, pool, table); err != nil {
		if logger != nil {
			logger.ErrorContext(ctx, fmt.Sprintf("create_memory: table bootstrap failed: %v", err))
		}
		return nil, util.NewAgentError(fmt.Sprintf("database setup error: %v", err), err)
	}

	// Check if identical content already exists in the same category within visible scope.
	query := fmt.Sprintf(`SELECT %s
FROM %s
WHERE %s AND category = $2 AND LOWER(TRIM(content)) = LOWER(TRIM($3))
LIMIT 1`, memory.Columns, table, memory.ScopeClause("$1"))

	rows, err := pool.Query(ctx, query, userID, category, content)
	if err != nil {
		if logger != nil {
			logger.ErrorContext(ctx, fmt.Sprintf("create_memory: duplicate check query failed: %v", err))
		}
		return nil, util.ProcessGeneralError(fmt.Errorf("unable to check for duplicate memories: %w", err))
	}
	existing, err := memory.CollectMemories(rows)
	if err != nil {
		if logger != nil {
			logger.ErrorContext(ctx, fmt.Sprintf("create_memory: reading duplicate rows failed: %v", err))
		}
		return nil, util.ProcessGeneralError(err)
	}
	if len(existing) > 0 {
		dup := existing[0]
		if logger != nil {
			logger.InfoContext(ctx, fmt.Sprintf("create_memory: duplicate found (id=%s); refreshing salience", dup.MemoryID))
		}

		// If a duplicate exists, we "refresh" its salience by bumping access_count and last_accessed_at, and return status "duplicate".
		refreshed, err := memory.Touch(ctx, pool, table, dup.MemoryID)
		if err != nil {
			if logger != nil {
				logger.ErrorContext(ctx, fmt.Sprintf("create_memory: refresh existing memory failed: %v", err))
			}
			return nil, util.ProcessGeneralError(err)
		}
		return Result{
			Status:  "duplicate",
			Message: "An equivalent memory already exists in this category. It was not re-created; its access count and last_accessed_at were refreshed.",
			Memory:  &refreshed,
		}, nil
	}

	// Insert the new memory record and scan the newly generated row using memory.Columns.
	insert := fmt.Sprintf(`INSERT INTO %s (user_id, is_global, category, content, is_pinned)
VALUES ($1, $2, $3, $4, $5)
RETURNING %s`, table, memory.Columns)

	rows, err = pool.Query(ctx, insert, userID, isGlobal, category, content, isPinned)
	if err != nil {
		if logger != nil {
			logger.ErrorContext(ctx, fmt.Sprintf("create_memory: INSERT failed: %v", err))
		}
		return nil, util.ProcessGeneralError(fmt.Errorf("unable to create memory: %w", err))
	}
	created, err := memory.CollectMemories(rows)
	if err != nil {
		if logger != nil {
			logger.ErrorContext(ctx, fmt.Sprintf("create_memory: reading created memory row failed: %v", err))
		}
		return nil, util.ProcessGeneralError(err)
	}
	if len(created) != 1 {
		return nil, util.NewClientServerError("memory insert returned no row", http.StatusInternalServerError, nil)
	}

	if logger != nil {
		logger.InfoContext(ctx, fmt.Sprintf("create_memory: successfully created memory id=%s", created[0].MemoryID))
	}

	// Return status "created" with the complete stored Memory entity.
	return Result{
		Status:  "created",
		Message: "Memory stored.",
		Memory:  &created[0],
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
