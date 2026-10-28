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
package alloydbadmin

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	alloydbrestapi "google.golang.org/api/alloydb/v1"
	"google.golang.org/api/option"
)

const SourceType string = "alloydb-admin"

// validate interface
var _ sources.SourceConfig = Config{}

func init() {
	if !sources.Register(SourceType, newConfig) {
		panic(fmt.Sprintf("source type %q already registered", SourceType))
	}
}

func newConfig(ctx context.Context, name string, decoder *yaml.Decoder) (sources.SourceConfig, error) {
	actual := Config{Name: name}
	if err := decoder.DecodeContext(ctx, &actual); err != nil {
		return nil, err
	}
	return actual, nil
}

type Config struct {
	Name           string `yaml:"name" validate:"required"`
	Type           string `yaml:"type" validate:"required"`
	DefaultProject string `yaml:"defaultProject"`
	UseClientOAuth bool   `yaml:"useClientOAuth"`
	ReadOnly       bool   `yaml:"readOnly"`
}

func (r Config) SourceConfigType() string {
	return SourceType
}

func (r Config) Initialize(ctx context.Context, tracer trace.Tracer) (sources.Source, error) {
	ua, err := util.UserAgentFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("error in User Agent retrieval: %s", err)
	}

	var client *http.Client
	if r.UseClientOAuth {
		client = &http.Client{
			Transport: util.NewUserAgentRoundTripper(ua, http.DefaultTransport),
		}
	} else {
		// Use Application Default Credentials
		creds, err := google.FindDefaultCredentials(ctx, alloydbrestapi.CloudPlatformScope)
		if err != nil {
			return nil, fmt.Errorf("failed to find default credentials: %w", err)
		}
		baseClient := oauth2.NewClient(ctx, creds.TokenSource)
		baseClient.Transport = util.NewUserAgentRoundTripper(ua, baseClient.Transport)
		client = baseClient
	}

	service, err := alloydbrestapi.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("error creating new alloydb service: %w", err)
	}

	s := &Source{
		Config:  r,
		BaseURL: "https://alloydb.googleapis.com",
		Service: service,
	}

	return s, nil
}

var _ sources.Source = &Source{}

type Source struct {
	Config
	BaseURL string
	Service *alloydbrestapi.Service
}

func (s *Source) IsReadOnly() bool {
	return s.ReadOnly
}

func (s *Source) SourceType() string {
	return SourceType
}

func (s *Source) ToConfig() sources.SourceConfig {
	return s.Config
}

func (s *Source) GetDefaultProject() string {
	return s.DefaultProject
}

func (s *Source) getService(ctx context.Context, accessToken string) (*alloydbrestapi.Service, error) {
	if s.UseClientOAuth {
		token := &oauth2.Token{AccessToken: accessToken}
		client := oauth2.NewClient(ctx, oauth2.StaticTokenSource(token))
		service, err := alloydbrestapi.NewService(ctx, option.WithHTTPClient(client))
		if err != nil {
			return nil, fmt.Errorf("error creating new alloydb service: %w", err)
		}
		return service, nil
	}
	return s.Service, nil
}

func (s *Source) UseClientAuthorization() bool {
	return s.UseClientOAuth
}

func (s *Source) CreateCluster(ctx context.Context, project, location, network, user, password, cluster, accessToken string) (any, error) {
	// Build the request body using the type-safe Cluster struct.
	clusterBody := &alloydbrestapi.Cluster{
		NetworkConfig: &alloydbrestapi.NetworkConfig{
			Network: fmt.Sprintf("projects/%s/global/networks/%s", project, network),
		},
		InitialUser: &alloydbrestapi.UserPassword{
			User:     user,
			Password: password,
		},
	}

	service, err := s.getService(ctx, accessToken)
	if err != nil {
		return nil, err
	}
	urlString := fmt.Sprintf("projects/%s/locations/%s", project, location)

	// The Create API returns a long-running operation.
	resp, err := service.Projects.Locations.Clusters.Create(urlString, clusterBody).ClusterId(cluster).Do()
	if err != nil {
		return nil, fmt.Errorf("error creating AlloyDB cluster: %w", err)
	}
	return resp, nil
}

func (s *Source) CreateInstance(ctx context.Context, project, location, cluster, instanceID, instanceType, displayName string, nodeCount int, accessToken string) (any, error) {
	// Build the request body using the type-safe Instance struct.
	instance := &alloydbrestapi.Instance{
		InstanceType: instanceType,
		NetworkConfig: &alloydbrestapi.InstanceNetworkConfig{
			EnablePublicIp: true,
		},
		DatabaseFlags: map[string]string{
			"password.enforce_complexity": "on",
		},
	}

	if displayName != "" {
		instance.DisplayName = displayName
	}

	if instanceType == "READ_POOL" {
		instance.ReadPoolConfig = &alloydbrestapi.ReadPoolConfig{
			NodeCount: int64(nodeCount),
		}
	}

	service, err := s.getService(ctx, accessToken)
	if err != nil {
		return nil, err
	}

	urlString := fmt.Sprintf("projects/%s/locations/%s/clusters/%s", project, location, cluster)

	// The Create API returns a long-running operation.
	resp, err := service.Projects.Locations.Clusters.Instances.Create(urlString, instance).InstanceId(instanceID).Do()
	if err != nil {
		return nil, fmt.Errorf("error creating AlloyDB instance: %w", err)
	}
	return resp, nil
}

func (s *Source) CreateUser(ctx context.Context, userType, password string, roles []string, accessToken, project, location, cluster, userID string) (any, error) {
	// Build the request body using the type-safe User struct.
	user := &alloydbrestapi.User{
		UserType: userType,
	}

	if userType == "ALLOYDB_BUILT_IN" {
		user.Password = password
	}

	if len(roles) > 0 {
		user.DatabaseRoles = roles
	}

	service, err := s.getService(ctx, accessToken)
	if err != nil {
		return nil, err
	}

	urlString := fmt.Sprintf("projects/%s/locations/%s/clusters/%s", project, location, cluster)

	// The Create API returns a long-running operation.
	resp, err := service.Projects.Locations.Clusters.Users.Create(urlString, user).UserId(userID).Do()
	if err != nil {
		return nil, fmt.Errorf("error creating AlloyDB user: %w", err)
	}

	return resp, nil
}

func (s *Source) GetCluster(ctx context.Context, project, location, cluster, accessToken string) (any, error) {
	service, err := s.getService(ctx, accessToken)
	if err != nil {
		return nil, err
	}

	urlString := fmt.Sprintf("projects/%s/locations/%s/clusters/%s", project, location, cluster)

	resp, err := service.Projects.Locations.Clusters.Get(urlString).Do()
	if err != nil {
		return nil, fmt.Errorf("error getting AlloyDB cluster: %w", err)
	}

	return resp, nil
}

func (s *Source) GetInstance(ctx context.Context, project, location, cluster, instance, accessToken string) (any, error) {
	service, err := s.getService(ctx, accessToken)
	if err != nil {
		return nil, err
	}

	urlString := fmt.Sprintf("projects/%s/locations/%s/clusters/%s/instances/%s", project, location, cluster, instance)

	resp, err := service.Projects.Locations.Clusters.Instances.Get(urlString).Do()
	if err != nil {
		return nil, fmt.Errorf("error getting AlloyDB instance: %w", err)
	}
	return resp, nil
}

func (s *Source) GetUsers(ctx context.Context, project, location, cluster, user, accessToken string) (any, error) {
	service, err := s.getService(ctx, accessToken)
	if err != nil {
		return nil, err
	}

	urlString := fmt.Sprintf("projects/%s/locations/%s/clusters/%s/users/%s", project, location, cluster, user)

	resp, err := service.Projects.Locations.Clusters.Users.Get(urlString).Do()
	if err != nil {
		return nil, fmt.Errorf("error getting AlloyDB user: %w", err)
	}
	return resp, nil
}

func (s *Source) ListCluster(ctx context.Context, project, location, accessToken string) (any, error) {
	service, err := s.getService(ctx, accessToken)
	if err != nil {
		return nil, err
	}

	urlString := fmt.Sprintf("projects/%s/locations/%s", project, location)

	resp, err := service.Projects.Locations.Clusters.List(urlString).Do()
	if err != nil {
		return nil, fmt.Errorf("error listing AlloyDB clusters: %w", err)
	}
	return resp, nil
}

func (s *Source) ListInstance(ctx context.Context, project, location, cluster, accessToken string) (any, error) {
	service, err := s.getService(ctx, accessToken)
	if err != nil {
		return nil, err
	}

	urlString := fmt.Sprintf("projects/%s/locations/%s/clusters/%s", project, location, cluster)

	resp, err := service.Projects.Locations.Clusters.Instances.List(urlString).Do()
	if err != nil {
		return nil, fmt.Errorf("error listing AlloyDB instances: %w", err)
	}
	return resp, nil
}

func (s *Source) ListUsers(ctx context.Context, project, location, cluster, accessToken string) (any, error) {
	service, err := s.getService(ctx, accessToken)
	if err != nil {
		return nil, err
	}

	urlString := fmt.Sprintf("projects/%s/locations/%s/clusters/%s", project, location, cluster)

	resp, err := service.Projects.Locations.Clusters.Users.List(urlString).Do()
	if err != nil {
		return nil, fmt.Errorf("error listing AlloyDB users: %w", err)
	}
	return resp, nil
}

func (s *Source) GetOperations(ctx context.Context, project, location, operation, connectionMessageTemplate string, delay time.Duration, accessToken string) (any, error) {
	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		return nil, err
	}

	service, err := s.getService(ctx, accessToken)
	if err != nil {
		return nil, err
	}

	name := fmt.Sprintf("projects/%s/locations/%s/operations/%s", project, location, operation)

	op, err := service.Projects.Locations.Operations.Get(name).Do()
	if err != nil {
		logger.DebugContext(ctx, fmt.Sprintf("error getting operation: %s, retrying in %v\n", err, delay))
	} else {
		if op.Done {
			if op.Error != nil {
				var errorBytes []byte
				errorBytes, err = json.Marshal(op.Error)
				if err != nil {
					return nil, fmt.Errorf("operation finished with error but could not marshal error object: %w", err)
				}
				return nil, fmt.Errorf("operation finished with error: %s", string(errorBytes))
			}

			var opBytes []byte
			opBytes, err = op.MarshalJSON()
			if err != nil {
				return nil, fmt.Errorf("could not marshal operation: %w", err)
			}

			if op.Response != nil {
				var responseData map[string]any
				if err := json.Unmarshal(op.Response, &responseData); err == nil && responseData != nil {
					if msg, ok := generateAlloyDBConnectionMessage(responseData, connectionMessageTemplate); ok {
						return msg, nil
					}
				}
			}

			var result any
			if err := json.Unmarshal(opBytes, &result); err != nil {
				return nil, fmt.Errorf("failed to unmarshal operation bytes: %w", err)
			}
			return result, nil
		}
		logger.DebugContext(ctx, fmt.Sprintf("Operation not complete, retrying in %v\n", delay))
	}
	return nil, nil
}

func generateAlloyDBConnectionMessage(responseData map[string]any, connectionMessageTemplate string) (string, bool) {
	resourceName, ok := responseData["name"].(string)
	if !ok {
		return "", false
	}

	parts := strings.Split(resourceName, "/")
	var project, region, cluster, instance string

	// Expected format: projects/{project}/locations/{location}/clusters/{cluster}
	// or projects/{project}/locations/{location}/clusters/{cluster}/instances/{instance}
	if len(parts) < 6 || parts[0] != "projects" || parts[2] != "locations" || parts[4] != "clusters" {
		return "", false
	}

	project = parts[1]
	region = parts[3]
	cluster = parts[5]

	if len(parts) >= 8 && parts[6] == "instances" {
		instance = parts[7]
	} else {
		return "", false
	}

	tmpl, err := template.New("alloydb-connection").Parse(connectionMessageTemplate)
	if err != nil {
		// This should not happen with a static template
		return fmt.Sprintf("template parsing error: %v", err), false
	}

	data := struct {
		Project  string
		Region   string
		Cluster  string
		Instance string
	}{
		Project:  project,
		Region:   region,
		Cluster:  cluster,
		Instance: instance,
	}

	var b strings.Builder
	if err := tmpl.Execute(&b, data); err != nil {
		return fmt.Sprintf("template execution error: %v", err), false
	}

	return b.String(), true
}
