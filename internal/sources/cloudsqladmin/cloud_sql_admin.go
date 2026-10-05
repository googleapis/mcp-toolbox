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
package cloudsqladmin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/log"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	sqladmin "google.golang.org/api/sqladmin/v1"
)

const SourceType string = "cloud-sql-admin"

var (
	targetLinkRegex = regexp.MustCompile(`/projects/([^/]+)/instances/([^/]+)/databases/([^/]+)`)
	backupDRRegex   = regexp.MustCompile(`^projects/([^/]+)/locations/([^/]+)/backupVaults/([^/]+)/dataSources/([^/]+)/backups/([^/]+)$`)
)

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

// Initialize initializes a CloudSQL Admin Source instance.
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
		creds, err := google.FindDefaultCredentials(ctx, sqladmin.SqlserviceAdminScope)
		if err != nil {
			return nil, fmt.Errorf("failed to find default credentials: %w", err)
		}
		baseClient := oauth2.NewClient(ctx, creds.TokenSource)
		baseClient.Transport = util.NewUserAgentRoundTripper(ua, baseClient.Transport)
		client = baseClient
	}

	service, err := sqladmin.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("error creating new sqladmin service: %w", err)
	}

	s := &Source{
		Config:  r,
		BaseURL: "https://sqladmin.googleapis.com",
		Service: service,
	}
	return s, nil
}

var _ sources.Source = &Source{}

type Source struct {
	Config
	BaseURL string
	Service *sqladmin.Service
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

// GetService returns a new Cloud SQL Admin service for the given access token.
func (s *Source) GetService(ctx context.Context, accessToken string) (*sqladmin.Service, error) {
	if s.UseClientOAuth {
		token := &oauth2.Token{AccessToken: accessToken}
		client := oauth2.NewClient(ctx, oauth2.StaticTokenSource(token))
		service, err := sqladmin.NewService(ctx, option.WithHTTPClient(client))
		if err != nil {
			return nil, fmt.Errorf("error creating new sqladmin service: %w", err)
		}
		return service, nil
	}
	return s.Service, nil
}

func (s *Source) UseClientAuthorization() bool {
	return s.UseClientOAuth
}

func (s *Source) CloneInstance(ctx context.Context, project, sourceInstanceName, destinationInstanceName, pointInTime, preferredZone, preferredSecondaryZone, accessToken string) (any, error) {
	cloneContext := &sqladmin.CloneContext{
		DestinationInstanceName: destinationInstanceName,
	}

	if pointInTime != "" {
		cloneContext.PointInTime = pointInTime
	}
	if preferredZone != "" {
		cloneContext.PreferredZone = preferredZone
	}
	if preferredSecondaryZone != "" {
		cloneContext.PreferredSecondaryZone = preferredSecondaryZone
	}

	rb := &sqladmin.InstancesCloneRequest{
		CloneContext: cloneContext,
	}
	service, err := s.GetService(ctx, accessToken)
	if err != nil {
		return nil, err
	}
	resp, err := service.Instances.Clone(project, sourceInstanceName, rb).Do()
	if err != nil {
		return nil, fmt.Errorf("error cloning instance: %w", err)
	}
	return resp, nil
}

func (s *Source) CreateDatabase(ctx context.Context, name, project, instance, accessToken string) (any, error) {
	database := sqladmin.Database{
		Name:     name,
		Project:  project,
		Instance: instance,
	}

	service, err := s.GetService(ctx, accessToken)
	if err != nil {
		return nil, err
	}

	resp, err := service.Databases.Insert(project, instance, &database).Do()
	if err != nil {
		return nil, fmt.Errorf("error creating database: %w", err)
	}
	return resp, nil
}

func (s *Source) CreateUsers(ctx context.Context, project, instance, name, password string, iamUser bool, accessToken string) (any, error) {
	service, err := s.GetService(ctx, accessToken)
	if err != nil {
		return nil, err
	}

	user := sqladmin.User{
		Name: name,
	}

	if iamUser {
		user.Type = "CLOUD_IAM_USER"
	} else {
		user.Type = "BUILT_IN"
		if password == "" {
			return nil, fmt.Errorf("missing 'password' parameter for non-IAM user")
		}
		user.Password = password
	}

	resp, err := service.Users.Insert(project, instance, &user).Do()
	if err != nil {
		return nil, fmt.Errorf("error creating user: %w", err)
	}

	return resp, nil
}

func (s *Source) GetInstance(ctx context.Context, projectId, instanceId, accessToken string) (any, error) {
	service, err := s.GetService(ctx, accessToken)
	if err != nil {
		return nil, err
	}

	resp, err := service.Instances.Get(projectId, instanceId).Do()
	if err != nil {
		return nil, fmt.Errorf("error getting instance: %w", err)
	}
	return resp, nil
}

func (s *Source) ListDatabase(ctx context.Context, project, instance, accessToken string) (any, error) {
	service, err := s.GetService(ctx, accessToken)
	if err != nil {
		return nil, err
	}

	resp, err := service.Databases.List(project, instance).Do()
	if err != nil {
		return nil, fmt.Errorf("error listing databases: %w", err)
	}

	if resp.Items == nil {
		return []any{}, nil
	}

	type databaseInfo struct {
		Name      string `json:"name"`
		Charset   string `json:"charset"`
		Collation string `json:"collation"`
	}

	var databases []databaseInfo
	for _, item := range resp.Items {
		databases = append(databases, databaseInfo{
			Name:      item.Name,
			Charset:   item.Charset,
			Collation: item.Collation,
		})
	}
	return databases, nil
}

func (s *Source) ListInstance(ctx context.Context, project, accessToken string) (any, error) {
	service, err := s.GetService(ctx, accessToken)
	if err != nil {
		return nil, err
	}

	resp, err := service.Instances.List(project).Do()
	if err != nil {
		return nil, fmt.Errorf("error listing instances: %w", err)
	}

	if resp.Items == nil {
		return []any{}, nil
	}

	type instanceInfo struct {
		Name         string `json:"name"`
		InstanceType string `json:"instanceType"`
	}

	var instances []instanceInfo
	for _, item := range resp.Items {
		instances = append(instances, instanceInfo{
			Name:         item.Name,
			InstanceType: item.InstanceType,
		})
	}
	return instances, nil
}

func (s *Source) ExecuteSql(ctx context.Context, project, instance, database, sql string, accessToken string) (any, error) {
	service, err := s.GetService(ctx, accessToken)
	if err != nil {
		return nil, err
	}

	req := &sqladmin.ExecuteSqlPayload{
		Database:     database,
		SqlStatement: sql,
	}

	resp, err := service.Instances.ExecuteSql(project, instance, req).Do()
	if err != nil {
		return nil, fmt.Errorf("error executing sql: %w", err)
	}
	return resp, nil
}

func (s *Source) CreateInstance(ctx context.Context, project, name, dbVersion, rootPassword string, settings sqladmin.Settings, accessToken string) (any, error) {
	instance := sqladmin.DatabaseInstance{
		Name:            name,
		DatabaseVersion: dbVersion,
		RootPassword:    rootPassword,
		Settings:        &settings,
		Project:         project,
	}

	service, err := s.GetService(ctx, accessToken)
	if err != nil {
		return nil, err
	}

	resp, err := service.Instances.Insert(project, &instance).Do()
	if err != nil {
		return nil, fmt.Errorf("error creating instance: %w", err)
	}

	return resp, nil
}

func (s *Source) GetWaitForOperations(ctx context.Context, service *sqladmin.Service, project, operation, connectionMessageTemplate string, delay time.Duration) (any, error) {
	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		return nil, err
	}
	op, err := service.Operations.Get(project, operation).Do()
	if err != nil {
		logger.DebugContext(ctx, fmt.Sprintf("error getting operation: %s, retrying in %v", err, delay))
	} else {
		if op.Status == "DONE" {
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

			var data map[string]any
			if err := json.Unmarshal(opBytes, &data); err != nil {
				return nil, fmt.Errorf("could not unmarshal operation: %w", err)
			}

			if msg, ok := generateCloudSQLConnectionMessage(ctx, s, logger, data, connectionMessageTemplate); ok {
				return msg, nil
			}
			return string(opBytes), nil
		}
		logger.DebugContext(ctx, fmt.Sprintf("operation not complete, retrying in %v", delay))
	}
	return nil, nil
}

func (s *Source) InsertBackupRun(ctx context.Context, project, instance, location, backupDescription, accessToken string) (any, error) {
	backupRun := &sqladmin.BackupRun{}
	if location != "" {
		backupRun.Location = location
	}
	if backupDescription != "" {
		backupRun.Description = backupDescription
	}

	service, err := s.GetService(ctx, string(accessToken))
	if err != nil {
		return nil, err
	}

	resp, err := service.BackupRuns.Insert(project, instance, backupRun).Do()
	if err != nil {
		return nil, fmt.Errorf("error creating backup: %w", err)
	}

	return resp, nil
}

func (s *Source) RestoreBackup(ctx context.Context, targetProject, targetInstance, sourceProject, sourceInstance, backupID, accessToken string) (any, error) {
	request := &sqladmin.InstancesRestoreBackupRequest{}

	// There are 3 scenarios for the backup identifier:
	// 1. The identifier is an int64 containing the timestamp of the BackupRun.
	//    This is used to restore standard backups, and the RestoreBackupContext
	//    field should be populated with the backup ID and source instance info.
	// 2. The identifier is a string of the format
	//    'projects/{project-id}/locations/{location}/backupVaults/{backupvault}/dataSources/{datasource}/backups/{backup-uid}'.
	//    This is used to restore BackupDR backups, and the BackupdrBackup field
	//    should be populated.
	// 3. The identifer is a string of the format
	//    'projects/{project-id}/backups/{backup-uid}'. In this case, the Backup
	//    field should be populated.
	if backupRunID, err := strconv.ParseInt(backupID, 10, 64); err == nil {
		if sourceProject == "" || sourceInstance == "" {
			return nil, fmt.Errorf("source project and instance are required when restoring via backup ID")
		}
		request.RestoreBackupContext = &sqladmin.RestoreBackupContext{
			Project:     sourceProject,
			InstanceId:  sourceInstance,
			BackupRunId: backupRunID,
		}
	} else if backupDRRegex.MatchString(backupID) {
		request.BackupdrBackup = backupID
	} else {
		request.Backup = backupID
	}

	service, err := s.GetService(ctx, string(accessToken))
	if err != nil {
		return nil, err
	}

	resp, err := service.Instances.RestoreBackup(targetProject, targetInstance, request).Do()
	if err != nil {
		return nil, fmt.Errorf("error restoring backup: %w", err)
	}

	return resp, nil
}

func generateCloudSQLConnectionMessage(ctx context.Context, source *Source, logger log.Logger, opResponse map[string]any, connectionMessageTemplate string) (string, bool) {
	operationType, ok := opResponse["operationType"].(string)
	if !ok || operationType != "CREATE_DATABASE" {
		return "", false
	}

	targetLink, ok := opResponse["targetLink"].(string)
	if !ok {
		return "", false
	}

	matches := targetLinkRegex.FindStringSubmatch(targetLink)
	if len(matches) < 4 {
		return "", false
	}
	project := matches[1]
	instance := matches[2]
	database := matches[3]

	dbInstance, err := fetchInstanceData(ctx, source, project, instance)
	if err != nil {
		logger.DebugContext(ctx, fmt.Sprintf("error fetching instance data: %v", err))
		return "", false
	}

	region := dbInstance.Region
	if region == "" {
		return "", false
	}

	databaseVersion := dbInstance.DatabaseVersion
	if databaseVersion == "" {
		return "", false
	}

	var dbType string
	if strings.Contains(databaseVersion, "POSTGRES") {
		dbType = "postgres"
	} else if strings.Contains(databaseVersion, "MYSQL") {
		dbType = "mysql"
	} else if strings.Contains(databaseVersion, "SQLSERVER") {
		dbType = "mssql"
	} else {
		return "", false
	}

	tmpl, err := template.New("cloud-sql-connection").Parse(connectionMessageTemplate)
	if err != nil {
		return fmt.Sprintf("template parsing error: %v", err), false
	}

	data := struct {
		Project     string
		Region      string
		Instance    string
		DBType      string
		DBTypeUpper string
		Database    string
	}{
		Project:     project,
		Region:      region,
		Instance:    instance,
		DBType:      dbType,
		DBTypeUpper: strings.ToUpper(dbType),
		Database:    database,
	}

	var b strings.Builder
	if err := tmpl.Execute(&b, data); err != nil {
		return fmt.Sprintf("template execution error: %v", err), false
	}

	return b.String(), true
}

func fetchInstanceData(ctx context.Context, source *Source, project, instance string) (*sqladmin.DatabaseInstance, error) {
	service, err := source.GetService(ctx, "")
	if err != nil {
		return nil, err
	}

	resp, err := service.Instances.Get(project, instance).Do()
	if err != nil {
		return nil, fmt.Errorf("error getting instance: %w", err)
	}
	return resp, nil
}
