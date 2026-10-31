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

// Package cloudsqlconnectgke provides a single tool that plans the
// connection from a Google Kubernetes Engine workload to any Cloud SQL
// instance (PostgreSQL, MySQL, or SQL Server). The engine is
// auto-detected from the DatabaseVersion returned by the Cloud SQL Admin
// API, so the caller does not have to specify it.
//
// GKE-to-Cloud SQL failures usually come from a handful of cluster facts
// that are invisible from inside the pod: Workload Identity off, node
// pools still on the Compute Engine metadata server, a routes-based
// cluster, private nodes without NAT, or a VPC (or Shared VPC host) that
// differs from the instance's. The tool reads those facts, resolves the
// network path (private IP, public IP, or Private Service Connect), and
// returns setup steps, manifests, and troubleshooting tailored to them.
//
// The tool is read-only: it never changes the cluster, IAM, or the
// instance. Identity boundary: when the cloud-sql-admin source sets
// useClientOAuth, both the Cloud SQL Admin and GKE calls run as the
// caller; otherwise the GKE call uses Application Default Credentials.
package cloudsqlconnectgke

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/cloudsqlconnect"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"google.golang.org/api/container/v1"
	sqladmin "google.golang.org/api/sqladmin/v1"
)

const resourceType string = "cloud-sql-connect-gke"

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

// Config defines the configuration for the cloud-sql-connect-gke tool.
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
		cfg.Description = "Helps connect a Cloud SQL instance (PostgreSQL, MySQL, or SQL Server) to a GKE cluster. " +
			"Auto-detects the engine from the instance, inspects the cluster (Workload Identity, node pool metadata, " +
			"VPC and Shared VPC, private nodes, Private Service Connect), recommends the best connection method " +
			"(Auth Proxy sidecar, Connector, or Direct Private IP), and provides a summary, setup instructions " +
			"tailored to the cluster, Kubernetes manifests, troubleshooting tips, and optional code snippets. " +
			"Relay the summary first and get the user's approval before running steps that change IAM, the " +
			"cluster, or the instance."
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
			"cluster_name",
			"Name of the GKE cluster to connect from",
		),
		parameters.NewStringParameter(
			"cluster_location",
			"Region or zone of the cluster (optional - will auto-discover if not provided)",
			parameters.WithStringDefault(""),
		),
		parameters.NewStringParameter(
			"cluster_project",
			"Project that owns the cluster (optional - defaults to the Cloud SQL instance's project)",
			parameters.WithStringDefault(""),
		),
		parameters.NewStringParameter(
			"namespace",
			"Kubernetes namespace of the workload (optional - defaults to 'default')",
			parameters.WithStringDefault(""),
		),
		parameters.NewStringParameter(
			"kubernetes_service_account",
			"Kubernetes service account the workload runs as (optional - defaults to 'cloudsql-ksa')",
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

// Tool represents the cloud-sql-connect-gke tool.
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

// Invoke fetches the Cloud SQL instance and GKE cluster, detects the
// engine, resolves how pods can reach the instance, and returns a
// connection plan tailored to the cluster's current state.
func (t Tool) Invoke(ctx context.Context, s sources.Source, params parameters.ParamValues, accessToken tools.AccessToken) (any, util.ToolboxError) {
	source, ok := s.(compatibleSource)
	if !ok {
		return nil, util.NewClientServerError("source used is not compatible with the tool", http.StatusInternalServerError, nil)
	}

	sqlService, err := source.GetService(ctx, string(accessToken))
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}

	containerService, err := getContainerService(ctx, string(accessToken))
	if err != nil {
		return nil, util.NewClientServerError("failed to initialize GKE service", http.StatusInternalServerError, err)
	}

	paramsMap := params.AsMap()

	connName, ok := paramsMap["instance_connection_name"].(string)
	if !ok || connName == "" {
		return nil, util.NewAgentError("missing or empty 'instance_connection_name' parameter", nil)
	}

	clusterName, ok := paramsMap["cluster_name"].(string)
	if !ok || clusterName == "" {
		return nil, util.NewAgentError("missing or empty 'cluster_name' parameter", nil)
	}
	if err := validateClusterName(clusterName); err != nil {
		return nil, util.NewAgentError(err.Error(), err)
	}

	clusterLocation, _ := paramsMap["cluster_location"].(string)
	if clusterLocation != "" {
		if err := validateLocation(clusterLocation, "cluster_location"); err != nil {
			return nil, util.NewAgentError(err.Error(), err)
		}
	}

	namespace, _ := paramsMap["namespace"].(string)
	if namespace != "" {
		if err := validateKubernetesName(namespace, "namespace"); err != nil {
			return nil, util.NewAgentError(err.Error(), err)
		}
	}

	ksa, _ := paramsMap["kubernetes_service_account"].(string)
	if ksa != "" {
		if err := validateKubernetesName(ksa, "kubernetes_service_account"); err != nil {
			return nil, util.NewAgentError(err.Error(), err)
		}
	}

	// database_name has no default here because the sensible default
	// depends on the auto-detected engine; it is applied after
	// Instances.Get, and validation runs against the final value.
	dbName, _ := paramsMap["database_name"].(string)

	language, _ := paramsMap["language"].(string)
	if err := cloudsqlconnect.ValidateLanguage(language); err != nil {
		return nil, util.NewAgentError(err.Error(), err)
	}

	project, region, instanceName, err := cloudsqlconnect.ValidateInstanceConnectionName(connName)
	if err != nil {
		return nil, util.NewAgentError(err.Error(), err)
	}

	// GKE commonly runs in a different project from the database, so the
	// cluster project is its own parameter.
	clusterProject, _ := paramsMap["cluster_project"].(string)
	if clusterProject == "" {
		clusterProject = project
	}
	if err := validateProjectID(clusterProject, "cluster_project"); err != nil {
		return nil, util.NewAgentError(err.Error(), err)
	}

	sqlInstance, err := sqlService.Instances.Get(project, instanceName).Context(ctx).Do()
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}
	sqlInfo := extractSQLInfo(sqlInstance)

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

	var cluster *container.Cluster
	if clusterLocation == "" {
		cluster, err = findCluster(ctx, containerService, clusterProject, clusterName)
		if err != nil {
			// Not-found and ambiguous-name errors are fixable by the
			// caller, so they are surfaced as agent errors with the
			// clusters that do exist.
			if errors.Is(err, errClusterLookup) {
				return nil, util.NewAgentError(err.Error(), err)
			}
			return nil, util.ProcessGcpError(err)
		}
	} else {
		cluster, err = containerService.Projects.Locations.Clusters.Get(clusterResourceName(clusterProject, clusterLocation, clusterName)).Context(ctx).Do()
		if err != nil {
			return nil, util.ProcessGcpError(err)
		}
	}
	gkeInfo := extractClusterInfo(cluster, clusterProject)

	validation := validateGKEConnection(sqlInfo, gkeInfo)
	path := resolveNetworkPath(sqlInfo, gkeInfo)
	primary, alternatives := getGKERecommendations(sqlInfo, gkeInfo)

	port := cloudsqlconnect.GetDatabasePort(engine)
	identity := newGKEIdentity(clusterProject, namespace, ksa)
	nativeSidecar := supportsNativeSidecar(gkeInfo.MasterVersion)

	envConfig := generateGKEEnvironmentConfig(manifestOptions{
		Method:         primary.Method,
		Path:           path,
		ConnectionName: connName,
		Port:           port,
		PrivateIP:      sqlInfo.PrivateIPAddress,
		DBName:         dbName,
		Identity:       identity,
		NativeSidecar:  nativeSidecar,
	})
	setup := setupParams{
		SQL:           sqlInfo,
		Cluster:       gkeInfo,
		Identity:      identity,
		Method:        primary.Method,
		Path:          path,
		Manifests:     envConfig,
		NativeSidecar: nativeSidecar,
	}
	setupSteps := generateGKESetupSteps(setup)
	connStrings := cloudsqlconnect.BuildConnectionStrings(primary.Method, engine, sqlInfo.CloudSQLInstanceInfo, dbName, connName)

	result := &connectResult{
		Summary: summarizeGKE(setup, validation, primary, len(setupSteps)),
		ConnectResult: &cloudsqlconnect.ConnectResult{
			InstanceConnectionName: connName,
			Project:                project,
			Region:                 region,
			DatabaseType:           engine,
			DatabaseVersion:        sqlInfo.DatabaseVersion,
			ComputeType:            cloudsqlconnect.ComputeGKE,
			ComputeResource:        clusterName,
			ComputeLocation:        gkeInfo.Location,
			Validation:             *validation,
			RecommendedMethod:      primary,
			AlternativeMethods:     alternatives,
			ConnectionStrings:      connStrings,
			SetupSteps:             setupSteps,
			AvailableLanguages:     cloudsqlconnect.AvailableLanguages,
			RequiredIAMRoles:       []string{"roles/cloudsql.client", "roles/iam.workloadIdentityUser"},
			RequiredAPIs:           []string{"sqladmin.googleapis.com", "container.googleapis.com", "iamcredentials.googleapis.com"},
		},
		EnvironmentConfig: envConfig,
		Troubleshooting:   gkeTroubleshooting(setup),
	}

	if language != "" {
		lang := cloudsqlconnect.Language(strings.ToLower(language))
		result.CodeSnippet = cloudsqlconnect.GenerateCodeSnippet(
			lang, primary.Method, engine,
			connName, dbName, port, sqlInfo.PrivateIPAddress,
		)
		if primary.Method == cloudsqlconnect.MethodAuthProxy {
			result.CodeSnippet.Notes = append(result.CodeSnippet.Notes, "On GKE the proxy runs as the sidecar in environmentConfig.sidecarYaml; ignore any host-level proxy command in the snippet. DB_* values come from the Kubernetes Secret.")
		}
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
