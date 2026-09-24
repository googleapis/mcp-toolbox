// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package alloydbwaitforoperation

import (
	"context"
	"fmt"
	"net/http"
	"time"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

const resourceType string = "alloydb-wait-for-operation"

var alloyDBConnectionMessageTemplate = `Your AlloyDB resource is ready.

To connect, please configure your environment. The method depends on how you are running the toolbox:

**If running locally via stdio:**
Update the MCP server configuration with the following environment variables:
` + "```json" + `
{
  "mcpServers": {
    "alloydb": {
      "command": "./PATH/TO/toolbox",
      "args": ["--prebuilt","alloydb-postgres","--stdio"],
      "env": {
          "ALLOYDB_POSTGRES_PROJECT": "{{.Project}}",
          "ALLOYDB_POSTGRES_REGION": "{{.Region}}",
          "ALLOYDB_POSTGRES_CLUSTER": "{{.Cluster}}",
{{if .Instance}}          "ALLOYDB_POSTGRES_INSTANCE": "{{.Instance}}",
{{end}}          "ALLOYDB_POSTGRES_DATABASE": "postgres",
          "ALLOYDB_POSTGRES_USER": "<your-user>",
          "ALLOYDB_POSTGRES_PASSWORD": "<your-password>"
      }
    }
  }
}
` + "```" + `

**If running remotely:**
For remote deployments, you will need to set the following environment variables in your deployment configuration:
` + "```" + `
ALLOYDB_POSTGRES_PROJECT={{.Project}}
ALLOYDB_POSTGRES_REGION={{.Region}}
ALLOYDB_POSTGRES_CLUSTER={{.Cluster}}
{{if .Instance}}ALLOYDB_POSTGRES_INSTANCE={{.Instance}}
{{end}}ALLOYDB_POSTGRES_DATABASE=postgres
ALLOYDB_POSTGRES_USER=<your-user>
ALLOYDB_POSTGRES_PASSWORD=<your-password>
` + "```" + `

Please refer to the official documentation for guidance on deploying the toolbox:
- Deploying the Toolbox: https://mcp-toolbox.dev/documentation/deploy-to/
- Deploying on GKE: https://mcp-toolbox.dev/documentation/deploy-to/kubernetes/
`

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
	GetDefaultProject() string
	UseClientAuthorization() bool
	GetOperations(context.Context, string, string, string, string, time.Duration, string) (any, error)
}

// Config defines the configuration for the wait-for-operation tool.
type Config struct {
	tools.ConfigBase `yaml:",inline"`
	Type             string `yaml:"type" validate:"required"`
	Source           string `yaml:"source" validate:"required"`

	// Polling configuration
	Delay      string  `yaml:"delay"`
	MaxDelay   string  `yaml:"maxDelay"`
	Multiplier float64 `yaml:"multiplier"`
	MaxRetries int     `yaml:"maxRetries"`
}

// validate interface
var _ tools.ToolConfig = Config{}

// ToolConfigType returns the type of the tool.
func (cfg Config) ToolConfigType() string {
	return resourceType
}

// Initialize initializes the tool from the configuration.
func (cfg Config) Initialize(context.Context) (tools.Tool, error) {

	if cfg.Description == "" {
		cfg.Description = "This will poll on operations API until the operation is done. For checking operation status we need projectId, locationID and operationId. Once instance is created give follow up steps on how to use the variables to bring data plane MCP server up in local and remote setup."
	}

	var delay time.Duration
	if cfg.Delay == "" {
		delay = 3 * time.Second
	} else {
		var err error
		delay, err = time.ParseDuration(cfg.Delay)
		if err != nil {
			return nil, fmt.Errorf("invalid value for delay: %w", err)
		}
	}

	var maxDelay time.Duration
	if cfg.MaxDelay == "" {
		maxDelay = 4 * time.Minute
	} else {
		var err error
		maxDelay, err = time.ParseDuration(cfg.MaxDelay)
		if err != nil {
			return nil, fmt.Errorf("invalid value for maxDelay: %w", err)
		}
	}

	multiplier := cfg.Multiplier
	if multiplier == 0 {
		multiplier = 2.0
	}

	maxRetries := cfg.MaxRetries
	if maxRetries == 0 {
		maxRetries = 10
	}

	params := buildParams("")
	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, tools.NewReadOnlyAnnotations),
			tools.Manifest{Description: cfg.Description, Parameters: params.Manifest(), AuthRequired: cfg.AuthRequired},
			params,
		),
		Delay:      delay,
		MaxDelay:   maxDelay,
		Multiplier: multiplier,
		MaxRetries: maxRetries,
	}, nil
}

// Tool represents the wait-for-operation tool.
type Tool struct {
	tools.BaseTool[Config]
	Client *http.Client

	// Polling configuration
	Delay      time.Duration
	MaxDelay   time.Duration
	Multiplier float64
	MaxRetries int
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

// Invoke executes the tool's logic.
func (t Tool) Invoke(ctx context.Context, s sources.Source, params parameters.ParamValues, accessToken tools.AccessToken) (any, util.ToolboxError) {
	source, ok := s.(compatibleSource)
	if !ok {
		return nil, util.NewClientServerError("source used is not compatible with the tool", http.StatusInternalServerError, nil)
	}
	paramsMap := params.AsMap()

	project, ok := paramsMap["project"].(string)
	if !ok {
		return nil, util.NewAgentError("missing 'project' parameter", nil)
	}
	location, ok := paramsMap["location"].(string)
	if !ok {
		return nil, util.NewAgentError("missing 'location' parameter", nil)
	}
	operation, ok := paramsMap["operation"].(string)
	if !ok {
		return nil, util.NewAgentError("missing 'operation' parameter", nil)
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	delay := t.Delay
	maxDelay := t.MaxDelay
	multiplier := t.Multiplier
	maxRetries := t.MaxRetries
	retries := 0

	for retries < maxRetries {
		select {
		case <-ctx.Done():
			return nil, util.NewAgentError("timed out waiting for operation", ctx.Err())
		default:
		}

		op, err := source.GetOperations(ctx, project, location, operation, alloyDBConnectionMessageTemplate, delay, string(accessToken))
		if err != nil {
			return nil, util.ProcessGeneralError(err)
		}
		if op != nil {
			return op, nil
		}

		time.Sleep(delay)
		delay = time.Duration(float64(delay) * multiplier)
		if delay > maxDelay {
			delay = maxDelay
		}
		retries++
	}
	return nil, util.NewAgentError("exceeded max retries waiting for operation", nil)
}

// Authorized checks if the tool is authorized.
func (t Tool) Authorized(verifiedAuthServices []string) bool {
	return true
}

func (t Tool) RequiresClientAuthorization(source sources.Source) (bool, error) {
	s, ok := source.(compatibleSource)
	if !ok {
		return false, fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	return s.UseClientAuthorization(), nil
}

// buildParams builds the tool's parameters. A non-empty project means the source has a
// configured default project, which is baked into the project param; otherwise the plain form is used.
func buildParams(project string) parameters.Parameters {
	projectParam := parameters.NewStringParameter("project", "The project ID")
	if project != "" {
		projectParam = parameters.NewStringParameter("project", "The GCP project ID. This is pre-configured; do not ask for it unless the user explicitly provides a different one.", parameters.WithStringDefault(project))
	}
	return parameters.Parameters{
		projectParam,
		parameters.NewStringParameter("location", "The location ID"),
		parameters.NewStringParameter("operation", "The operation ID"),
	}
}

// resolveParams builds the tool's parameters using the source's configured default GCP project.
func (t Tool) resolveParams(source sources.Source) (parameters.Parameters, error) {
	s, ok := source.(compatibleSource)
	if !ok {
		return nil, fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	return buildParams(s.GetDefaultProject()), nil
}

// GetParameters returns the tool's parameters, resolved against the source.
func (t Tool) GetParameters(source sources.Source) (parameters.Parameters, error) {
	return t.resolveParams(source)
}

// Manifest returns the tool's manifest, resolved against the source.
func (t Tool) Manifest(source sources.Source) (tools.Manifest, error) {
	allParameters, err := t.resolveParams(source)
	if err != nil {
		return tools.Manifest{}, err
	}
	return tools.Manifest{Description: t.Cfg.Description, Parameters: allParameters.Manifest(), AuthRequired: t.Cfg.AuthRequired}, nil
}
