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

// Package cloudsqlconnectgce provides a single tool that connects any
// Cloud SQL instance (PostgreSQL, MySQL, or SQL Server) to a Google
// Compute Engine VM. The engine is auto-detected from the
// DatabaseVersion returned by the Cloud SQL Admin API, so the caller
// does not have to specify it.
//
// The tool validates network reachability between the instance and the
// VM, recommends a connection method (Auth Proxy, Connector, or Direct
// Private IP) with ranked alternatives, and returns ready-to-paste
// setup steps and an optional language-specific code snippet with
// floor-pinned dependency versions.
//
// Identity boundary: both the Cloud SQL Admin call and the Compute
// Engine call now honor the caller's access token when the underlying
// cloud-sql-admin source is configured with useClientOAuth. When no
// caller token is supplied the tool falls back to Application Default
// Credentials for the Compute Engine call.
package cloudsqlconnectgce

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/cloudsqlconnect"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"google.golang.org/api/compute/v1"
	sqladmin "google.golang.org/api/sqladmin/v1"
)

const resourceType string = "cloud-sql-connect-gce"

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
	GetService(context.Context, string) (*sqladmin.Service, error)
	UseClientAuthorization() bool
}

// Config defines the configuration for the cloud-sql-connect-gce tool.
type Config struct {
	tools.ConfigBase `yaml:",inline"`
	Type             string `yaml:"type" validate:"required"`
	Source           string `yaml:"source" validate:"required"`
}

var _ tools.ToolConfig = Config{}

// ToolConfigType returns the type of the tool.
func (cfg Config) ToolConfigType() string { return resourceType }

// Initialize initializes the tool from the configuration.
func (cfg Config) Initialize(context.Context) (tools.Tool, error) {
	if cfg.Description == "" {
		cfg.Description = "Helps connect a Cloud SQL instance (PostgreSQL, MySQL, or SQL Server) to a GCE VM. " +
			"Auto-detects the engine from the instance, validates network connectivity, recommends the best " +
			"connection method (Auth Proxy, Connector, or Direct Private IP), and provides setup instructions " +
			"and optional code snippets."
	}
	allParameters := buildParams()
	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, tools.NewReadOnlyAnnotations),
			tools.Manifest{Description: cfg.Description, Parameters: allParameters.Manifest(), AuthRequired: cfg.AuthRequired},
			allParameters,
		),
	}, nil
}

func buildParams() parameters.Parameters {
	return parameters.Parameters{
		parameters.NewStringParameter(
			"instance_connection_name",
			"Cloud SQL instance connection name in the format: project:region:instance",
		),
		parameters.NewStringParameter(
			"vm_name",
			"Name of the GCE VM instance to connect from",
		),
		parameters.NewStringParameter(
			"vm_zone",
			"Zone of the VM (optional - will auto-discover if not provided)",
			parameters.WithStringDefault(""),
		),
		parameters.NewStringParameter(
			"database_name",
			"Database name to connect to (optional - defaults to the engine's conventional default: 'postgres', 'mysql', or 'master')",
			parameters.WithStringDefault(""),
		),
		parameters.NewStringParameter(
			"language",
			"Programming language for code snippet generation: python, nodejs, java, go (optional)",
			parameters.WithStringDefault(""),
		),
	}
}

var _ tools.Tool = Tool{}

// Tool represents the cloud-sql-connect-gce tool.
type Tool struct {
	tools.BaseTool[Config]
}

func (t Tool) GetSourceName() string {
	return t.Cfg.Source
}

func (t Tool) ValidateSource(src sources.Source) error {
	_, ok := src.(compatibleSource)
	if !ok {
		return fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	return nil
}

func (t Tool) ToConfig() tools.ToolConfig {
	return t.Cfg
}

// Invoke fetches the Cloud SQL instance, detects its engine, validates
// connectivity to the target VM, and returns connection recommendations.
func (t Tool) Invoke(ctx context.Context, s sources.Source, params parameters.ParamValues, accessToken tools.AccessToken) (any, util.ToolboxError) {
	source, ok := s.(compatibleSource)
	if !ok {
		return nil, util.NewClientServerError("source used is not compatible with the tool", http.StatusInternalServerError, nil)
	}

	sqlService, err := source.GetService(ctx, string(accessToken))
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}

	computeService, err := cloudsqlconnect.GetComputeService(ctx, string(accessToken))
	if err != nil {
		return nil, util.NewClientServerError("failed to initialize Compute Engine service", http.StatusInternalServerError, err)
	}

	paramsMap := params.AsMap()

	connName, ok := paramsMap["instance_connection_name"].(string)
	if !ok || connName == "" {
		return nil, util.NewAgentError("missing or empty 'instance_connection_name' parameter", nil)
	}

	vmName, ok := paramsMap["vm_name"].(string)
	if !ok || vmName == "" {
		return nil, util.NewAgentError("missing or empty 'vm_name' parameter", nil)
	}
	if err := cloudsqlconnect.ValidateGCEResourceName(vmName, "vm_name"); err != nil {
		return nil, util.NewAgentError(err.Error(), err)
	}

	vmZone, _ := paramsMap["vm_zone"].(string)
	if vmZone != "" {
		if err := cloudsqlconnect.ValidateGCEResourceName(vmZone, "vm_zone"); err != nil {
			return nil, util.NewAgentError(err.Error(), err)
		}
	}

	// database_name has no default here because the sensible default
	// depends on the auto-detected engine; the engine-derived default is
	// applied after Instances.Get. Validation runs against the final
	// value, whether caller-supplied or defaulted, matching the contract
	// the earlier per-engine tools enforced.
	dbName, _ := paramsMap["database_name"].(string)

	language, _ := paramsMap["language"].(string)
	if err := cloudsqlconnect.ValidateLanguage(language); err != nil {
		return nil, util.NewAgentError(err.Error(), err)
	}

	project, region, instanceName, err := cloudsqlconnect.ValidateInstanceConnectionName(connName)
	if err != nil {
		return nil, util.NewAgentError(err.Error(), err)
	}

	sqlInstance, err := sqlService.Instances.Get(project, instanceName).Context(ctx).Do()
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}
	sqlInfo := cloudsqlconnect.ExtractSQLInfo(sqlInstance)

	// Auto-detect the engine from the sqladmin response. Reject unknown
	// engines up front rather than silently emitting a Postgres-shaped
	// result for a future Cloud SQL variant.
	engine, err := cloudsqlconnect.ParseDatabaseTypeStrict(sqlInfo.DatabaseVersion)
	if err != nil {
		return nil, util.NewAgentError(fmt.Sprintf("instance %q: %s", instanceName, err.Error()), err)
	}
	sqlInfo.DatabaseType = engine

	if dbName == "" {
		dbName = cloudsqlconnect.DefaultDatabaseName(engine)
	}
	if err := cloudsqlconnect.ValidateDatabaseName(dbName); err != nil {
		return nil, util.NewAgentError(err.Error(), err)
	}

	var vmInstance *compute.Instance
	if vmZone == "" {
		vmInstance, vmZone, err = cloudsqlconnect.FindVM(ctx, computeService, project, vmName)
		if err != nil {
			return nil, util.ProcessGcpError(err)
		}
	} else {
		vmInstance, err = computeService.Instances.Get(project, vmZone, vmName).Context(ctx).Do()
		if err != nil {
			return nil, util.ProcessGcpError(err)
		}
	}
	vmInfo := cloudsqlconnect.ExtractVMInfo(vmInstance, vmZone)

	validation := cloudsqlconnect.ValidateGCEConnection(sqlInfo, vmInfo)
	sameVPC := cloudsqlconnect.IsSameVPC(sqlInfo.VPCNetwork, vmInfo.VPCNetwork)
	primary, alternatives := cloudsqlconnect.GetGCERecommendations(sqlInfo, vmInfo, sameVPC)

	port := cloudsqlconnect.GetDatabasePort(engine)
	setupSteps := cloudsqlconnect.GenerateGCESetupSteps(primary.Method, connName, port, sqlInfo.PrivateIPAddress, vmInfo.ServiceAccount)
	envConfig := cloudsqlconnect.GenerateEnvironmentConfig(
		primary.Method, cloudsqlconnect.ComputeGCE, connName, port,
		sqlInfo.PrivateIPAddress, dbName, project,
	)
	connStrings := cloudsqlconnect.BuildConnectionStrings(primary.Method, engine, sqlInfo, dbName, connName)

	result := &cloudsqlconnect.ConnectResult{
		InstanceConnectionName: connName,
		Project:                project,
		Region:                 region,
		DatabaseType:           engine,
		DatabaseVersion:        sqlInfo.DatabaseVersion,
		ComputeType:            cloudsqlconnect.ComputeGCE,
		ComputeResource:        vmName,
		ComputeLocation:        vmZone,
		Validation:             *validation,
		RecommendedMethod:      primary,
		AlternativeMethods:     alternatives,
		ConnectionStrings:      connStrings,
		EnvironmentConfig:      envConfig,
		SetupSteps:             setupSteps,
		AvailableLanguages:     cloudsqlconnect.AvailableLanguages,
		RequiredIAMRoles:       []string{"roles/cloudsql.client"},
		RequiredAPIs:           []string{"sqladmin.googleapis.com", "compute.googleapis.com"},
	}

	if language != "" {
		lang := cloudsqlconnect.Language(strings.ToLower(language))
		result.CodeSnippet = cloudsqlconnect.GenerateCodeSnippet(
			lang, primary.Method, engine,
			connName, dbName, port, sqlInfo.PrivateIPAddress,
		)
	}

	return result, nil
}

func (t Tool) RequiresClientAuthorization(source sources.Source) (bool, error) {
	s, ok := source.(compatibleSource)
	if !ok {
		return false, fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	return s.UseClientAuthorization(), nil
}
