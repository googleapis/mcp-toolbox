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

package postgressearchmemory

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
	"github.com/jackc/pgx/v5"
)

const resourceType string = "postgres-search-memory"

// defaultDescription gives LLMs clear prompt guidance on when to invoke this tool, and is reused across postgres prebuilts.
// The category list is derived from DefaultCategories so it stays the single source of truth.
var defaultDescription = fmt.Sprintf(`Search for stored facts, user preferences, coding conventions, and prior constraints based on relevance to a natural language query.
Use this tool before performing tasks, answering questions about preferences, or when relevant context might have been stored in previous sessions.

Optional filters:
- category: narrow search to one of %q.
- top_k: maximum number of relevant memories to return (default 5).
- threshold: minimum relevance score threshold.`, memory.DefaultCategories)

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

// Config represents the YAML configuration for postgres-search-memory, embedding shared table/auth settings.
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
			"query",
			"Natural language query to search relevant memories.",
			parameters.WithStringRequired(true),
		),
		parameters.NewStringParameter(
			"category",
			fmt.Sprintf("Optional classification tag to filter memories. Must be one of: %q.", memory.DefaultCategories),
			parameters.WithStringRequired(false),
			parameters.WithStringAllowedValues(memory.DefaultCategories),
		),
		parameters.NewIntParameter(
			"top_k",
			"Maximum number of top relevant memory records to return (defaults to 5).",
			parameters.WithIntDefault(5),
		),
		parameters.NewFloatParameter(
			"threshold",
			"Minimum relevance score threshold range (0.0 to 1.0, defaults to 0.0).",
			parameters.WithFloatDefault(0.0),
		),
	}

	if userParam := cfg.UserIDParameter(); userParam != nil {
		allParameters = append(allParameters, userParam)
	}

	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, tools.NewReadOnlyAnnotations),
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
	Status   string          `json:"status"`
	Message  string          `json:"message"`
	Memories []memory.Memory `json:"memories"`
}

func (t Tool) Invoke(ctx context.Context, s sources.Source, params parameters.ParamValues, accessToken tools.AccessToken) (any, util.ToolboxError) {
	logger, _ := util.LoggerFromContext(ctx)
	source, ok := s.(memory.CompatibleSource)
	if !ok {
		err := fmt.Errorf("source %q is not compatible with postgres-search-memory (requires PostgresPool)", t.Cfg.Source)
		if logger != nil {
			logger.ErrorContext(ctx, err.Error())
		}
		return nil, util.NewClientServerError(err.Error(), http.StatusInternalServerError, err)
	}
	pool := source.PostgresPool()
	table := t.Cfg.TableName
	p := params.AsMap()

	query := strings.TrimSpace(asString(p["query"]))
	if query == "" {
		return nil, util.NewAgentError("query must not be empty", nil)
	}

	userID, err := t.Cfg.ResolveUserID(params)
	if err != nil {
		if logger != nil {
			logger.ErrorContext(ctx, err.Error())
		}
		return nil, util.NewClientServerError(err.Error(), http.StatusBadRequest, err)
	}

	category := strings.TrimSpace(asString(p["category"]))
	topK := asInt(p["top_k"], 5)
	threshold := asFloat(p["threshold"], 0.0)

	if logger != nil {
		logger.InfoContext(ctx, fmt.Sprintf("search_memory: querying table=%q user=%q category=%q top_k=%d threshold=%f", table, userID, category, topK, threshold))
	}

	// Lazy bootstrap to ensure the memory table exists before querying.
	if err := memory.EnsureMemoryTable(ctx, pool, table); err != nil {
		if logger != nil {
			logger.ErrorContext(ctx, fmt.Sprintf("search_memory: table bootstrap failed: %v", err))
		}
		return nil, util.NewAgentError(fmt.Sprintf("database setup error: %v", err), err)
	}

	args := []any{query, userID}
	filters := ""
	if category != "" {
		args = append(args, category)
		filters += fmt.Sprintf(" AND category = $%d", len(args))
	}
	if threshold > 0 {
		args = append(args, threshold)
		filters += fmt.Sprintf(" AND ts_rank(m.tsv, q.tsq) >= $%d", len(args))
	}
	args = append(args, topK)
	limitParam := fmt.Sprintf("$%d", len(args))

	// The q CTE ORs the query's normalized lexemes, so a memory matches when it shares any
	// non-stopword with the query and ts_rank favors memories that cover more query terms.
	// An AND query would drop any memory missing a single term, which penalizes the verbose,
	// synonym-rich queries agents tend to write. plainto_tsquery is used instead of
	// websearch_to_tsquery so the & -> | rewrite cannot turn a "-term" negation into "| !term".
	// Hits are ordered by relevance before pinned status so a weakly matching pinned memory
	// cannot outrank a strong match.
	// Select the top matches and atomically bump their salience counters in a single query,
	// so returned access_count and last_accessed_at always match the database.
	querySQL := fmt.Sprintf(`WITH q AS (
    SELECT replace(plainto_tsquery('english', $1)::text, ' & ', ' | ')::tsquery AS tsq
), hits AS (
    SELECT m.memory_id, ts_rank(m.tsv, q.tsq) AS score, m.last_accessed_at AS prev_accessed_at
    FROM %[1]s AS m, q
    WHERE %[2]s AND m.tsv @@ q.tsq%[3]s
    ORDER BY score DESC, m.is_pinned DESC, m.last_accessed_at DESC
    LIMIT %[4]s
), refreshed AS (
    UPDATE %[1]s AS m
    SET access_count = m.access_count + 1, last_accessed_at = NOW()
    FROM hits
    WHERE m.memory_id = hits.memory_id
    RETURNING m.memory_id::text AS memory_id, m.user_id, m.is_global, m.category, m.content, m.is_pinned, m.created_at, m.updated_at, m.last_accessed_at, m.access_count, hits.score, hits.prev_accessed_at
)
SELECT %[5]s, score FROM refreshed ORDER BY score DESC, is_pinned DESC, prev_accessed_at DESC`,
		table, memory.ScopeClause("$2"), filters, limitParam, memory.Columns)

	rows, err := pool.Query(ctx, querySQL, args...)
	if err != nil {
		if logger != nil {
			logger.ErrorContext(ctx, fmt.Sprintf("search_memory: query failed: %v", err))
		}
		return nil, util.ProcessGeneralError(fmt.Errorf("unable to search memories: %w", err))
	}
	defer rows.Close()

	memories := []memory.Memory{}
	for rows.Next() {
		m, err := scanMemoryWithScore(rows)
		if err != nil {
			return nil, util.ProcessGeneralError(fmt.Errorf("unable to parse memory search row: %w", err))
		}
		memories = append(memories, m)
	}
	if err := rows.Err(); err != nil {
		return nil, util.ProcessGeneralError(fmt.Errorf("unable to read memories: %w", err))
	}

	if len(memories) == 0 {
		return Result{
			Status:   "empty",
			Message:  "No relevant memories found.",
			Memories: memories,
		}, nil
	}
	return Result{
		Status:   "ok",
		Message:  fmt.Sprintf("Found %d relevant memories.", len(memories)),
		Memories: memories,
	}, nil
}

func scanMemoryWithScore(rows pgx.Rows) (memory.Memory, error) {
	var m memory.Memory
	var score float64
	dest := []any{
		&m.MemoryID,
		&m.UserID,
		&m.IsGlobal,
		&m.Category,
		&m.Content,
		&m.IsPinned,
		&m.CreatedAt,
		&m.UpdatedAt,
		&m.LastAccessedAt,
		&m.AccessCount,
		&score,
	}
	if err := rows.Scan(dest...); err != nil {
		return m, err
	}
	m.Score = &score
	return m, nil
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func asInt(v any, defaultVal int) int {
	switch n := v.(type) {
	case int:
		if n > 0 {
			return n
		}
	case int64:
		if n > 0 {
			return int(n)
		}
	case float64:
		if n > 0 {
			return int(n)
		}
	}
	return defaultVal
}

func asFloat(v any, defaultVal float64) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	}
	return defaultVal
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
