// Copyright 2025 Google LLC
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

package cloudgda

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"cloud.google.com/go/geminidataanalytics/apiv1beta/geminidataanalyticspb"
	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"google.golang.org/protobuf/encoding/protojson"
)

const resourceType string = "cloud-gemini-data-analytics-query"

// Guidance is the tool guidance string.
const Guidance = `Tool guidance:
  Inputs:
    1. query: A natural language formulation of a database query.
  Outputs: (all optional)
    1. disambiguation_question: Clarification questions or comments where the tool needs the users' input.
    2. generated_query: The generated query for the user query.
    3. intent_explanation: An explanation for why the tool produced ` + "`generated_query`" + `.
    4. query_result: The result of executing ` + "`generated_query`" + `.
    5. natural_language_answer: The natural language answer that summarizes the ` + "`query`" + ` and ` + "`query_result`" + `.

Usage guidance:
  1. If ` + "`disambiguation_question`" + ` is produced, then solicit the needed inputs from the user and try the tool with a new ` + "`query`" + ` that has the needed clarification.
  2. If ` + "`natural_language_answer`" + ` is produced, use ` + "`intent_explanation`" + ` and ` + "`generated_query`" + ` to see if you need to clarify any assumptions for the user.`

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
	GetProjectID() string
	UseClientAuthorization() bool
	RunQuery(context.Context, string, *geminidataanalyticspb.QueryDataRequest) (*geminidataanalyticspb.QueryDataResponse, error)
}

// QueryDataContext wraps geminidataanalyticspb.QueryDataContext to support YAML decoding via protojson.
type QueryDataContext struct {
	*geminidataanalyticspb.QueryDataContext
}

var _ yaml.BytesUnmarshalerContext = &QueryDataContext{}

func (q *QueryDataContext) UnmarshalYAML(ctx context.Context, b []byte) error {
	var raw map[string]any
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return fmt.Errorf("failed to unmarshal context from yaml: %w", err)
	}
	if dropSpannerRegion(raw) {
		// Warning is best effort: a caller without a logger in context still
		// gets the config loaded.
		if logger, err := util.LoggerFromContext(ctx); err == nil {
			logger.WarnContext(ctx, "ignoring context.datasourceReferences.spannerReference.databaseReference.region, which the Gemini Data Analytics API no longer supports; remove it from your configuration")
		}
	}
	jsonBytes, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("failed to marshal context map: %w", err)
	}
	q.QueryDataContext = &geminidataanalyticspb.QueryDataContext{}
	if err := protojson.Unmarshal(jsonBytes, q.QueryDataContext); err != nil {
		return fmt.Errorf("failed to unmarshal context to proto: %w", err)
	}
	return nil
}

// dropSpannerRegion deletes datasourceReferences.spannerReference.databaseReference.region
// from a raw context and reports whether it was present. The API removed the
// field, and strict proto decoding would otherwise reject configs that set it.
func dropSpannerRegion(raw map[string]any) bool {
	refs := childMap(raw, "datasourceReferences", "datasource_references")
	spanner := childMap(refs, "spannerReference", "spanner_reference")
	db := childMap(spanner, "databaseReference", "database_reference")
	if _, ok := db["region"]; !ok {
		return false
	}
	delete(db, "region")
	return true
}

// childMap returns the map stored under the first of keys present in m. It
// accepts both JSON and proto field names, as protojson does.
func childMap(m map[string]any, keys ...string) map[string]any {
	for _, k := range keys {
		if child, ok := m[k].(map[string]any); ok {
			return child
		}
	}
	return nil
}

// GenerationOptions wraps geminidataanalyticspb.GenerationOptions to support YAML decoding via protojson.
type GenerationOptions struct {
	*geminidataanalyticspb.GenerationOptions
}

func (g *GenerationOptions) UnmarshalYAML(b []byte) error {
	var raw map[string]any
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return fmt.Errorf("failed to unmarshal generation options from yaml: %w", err)
	}
	jsonBytes, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("failed to marshal generation options map: %w", err)
	}
	g.GenerationOptions = &geminidataanalyticspb.GenerationOptions{}
	if err := protojson.Unmarshal(jsonBytes, g.GenerationOptions); err != nil {
		return fmt.Errorf("failed to unmarshal generation options to proto: %w", err)
	}
	return nil
}

type Config struct {
	tools.ConfigBase  `yaml:",inline"`
	Type              string             `yaml:"type" validate:"required"`
	Source            string             `yaml:"source" validate:"required"`
	Location          string             `yaml:"location" validate:"required"`
	Context           *QueryDataContext  `yaml:"context" validate:"required"`
	GenerationOptions *GenerationOptions `yaml:"generationOptions,omitempty"`
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

	// Define the parameters for the Gemini Data Analytics Query API
	// The query is the only input parameter.
	allParameters := parameters.Parameters{
		parameters.NewStringParameter("query", "A natural language formulation of a database query.", parameters.WithStringRequired(true)),
	}
	// The input and outputs are for tool guidance, usage guidance is for multi-turn interaction.
	guidance := Guidance
	cfg.Description += "\n\n" + guidance

	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, tools.NewReadOnlyAnnotations),
			tools.Manifest{Description: cfg.Description, Parameters: allParameters.Manifest(), AuthRequired: cfg.AuthRequired},
			allParameters,
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
	paramsMap := params.AsMap()
	query, ok := paramsMap["query"].(string)
	if !ok {
		return nil, util.NewAgentError("query parameter not found or not a string", nil)
	}

	// Parse the access token if provided
	var tokenStr string
	if source.UseClientAuthorization() {
		var err error
		tokenStr, err = accessToken.ParseBearerToken()
		if err != nil {
			return nil, util.NewClientServerError("error parsing access token", http.StatusUnauthorized, err)
		}
	}

	// The parent in the request payload uses the tool's configured location.
	payloadParent := fmt.Sprintf("projects/%s/locations/%s", source.GetProjectID(), t.Cfg.Location)

	req := &geminidataanalyticspb.QueryDataRequest{
		Parent: payloadParent,
		Prompt: query,
	}

	if t.Cfg.Context != nil {
		req.Context = t.Cfg.Context.QueryDataContext
	}

	if t.Cfg.GenerationOptions != nil {
		req.GenerationOptions = t.Cfg.GenerationOptions.GenerationOptions
	}

	resp, err := source.RunQuery(ctx, tokenStr, req)
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}
	return resp, nil
}

func (t Tool) RequiresClientAuthorization(source sources.Source) (bool, error) {
	s, ok := source.(compatibleSource)
	if !ok {
		return false, fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	return s.UseClientAuthorization(), nil
}
