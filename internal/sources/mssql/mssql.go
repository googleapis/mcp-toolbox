// Copyright 2025 Google LLC
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

package mssql

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/orderedmap"
	_ "github.com/microsoft/go-mssqldb"
	"github.com/microsoft/go-mssqldb/azuread"
	"go.opentelemetry.io/otel/trace"
)

const SourceType string = "mssql"

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

// azureFedAuth maps the `azureAuth.mode` field onto the fedauth workflow the
// azuread driver expects. The keys are the modes allowed by the oneof tag below.
var azureFedAuth = map[string]string{
	"default":           azuread.ActiveDirectoryDefault,
	"workload-identity": azuread.ActiveDirectoryWorkloadIdentity,
	"managed-identity":  azuread.ActiveDirectoryManagedIdentity,
	"service-principal": azuread.ActiveDirectoryServicePrincipal,
}

// AzureAuthConfig configures Microsoft Entra ID authentication for the database
// connection, for Azure SQL servers where SQL authentication is disabled. Omit the
// whole block to keep using a user and password.
type AzureAuthConfig struct {
	Mode string `yaml:"mode" validate:"required,oneof=default workload-identity managed-identity service-principal"`
	// ClientID identifies the credential to use, and is required when a host maps to
	// more than one identity — a pod with several user-assigned managed identities,
	// for instance — so the credential is not left to be guessed. The driver only
	// reads a tenant alongside a client id, so one without the other is refused
	// rather than silently dropped; a service principal always needs one.
	ClientID string `yaml:"clientId" validate:"required_with=TenantID,required_if=Mode service-principal"`
	// TenantID is needed where the tenant cannot be inferred from the server.
	TenantID string `yaml:"tenantId"`
	// DisableInstanceDiscovery skips the authority metadata request, which
	// network-isolated and sovereign-cloud deployments cannot reach.
	DisableInstanceDiscovery bool `yaml:"disableInstanceDiscovery"`
	// AdditionallyAllowedTenants permits tokens from tenants beyond the configured
	// one, for multi-tenant credentials.
	AdditionallyAllowedTenants []string `yaml:"additionallyAllowedTenants"`
}

type Config struct {
	// Cloud SQL MSSQL configs
	Name string `yaml:"name" validate:"required"`
	Type string `yaml:"type" validate:"required"`
	Host string `yaml:"host" validate:"required"`
	Port string `yaml:"port" validate:"required"`
	// User and Password are the SQL login. They are not required when azureAuth is
	// set, since an Entra-only server has no SQL logins to hand out. In
	// service-principal mode Password carries the client secret.
	User     string `yaml:"user" validate:"required_without=AzureAuth"`
	Password string `yaml:"password" validate:"required_without=AzureAuth"`
	Database string `yaml:"database" validate:"required"`
	Encrypt  string `yaml:"encrypt"`
	// AzureAuth switches the connection to Microsoft Entra ID authentication.
	AzureAuth *AzureAuthConfig `yaml:"azureAuth"`
}

func (r Config) SourceConfigType() string {
	// Returns Cloud SQL MSSQL source type
	return SourceType
}

func (r Config) Initialize(ctx context.Context, tracer trace.Tracer) (sources.Source, error) {
	if r.AzureAuth != nil {
		// The identity is the Entra credential, so a SQL user has no meaning here
		// and is refused rather than silently ignored.
		if r.User != "" {
			return nil, fmt.Errorf("azureAuth authenticates with an Entra identity, so 'user' must not be set")
		}
		// The driver reads a service principal's client secret from 'password';
		// without it the failure would surface as a login error at connect time.
		if r.AzureAuth.Mode == "service-principal" && r.Password == "" {
			return nil, fmt.Errorf("azureAuth mode service-principal needs the client secret in 'password'")
		}
	}

	// Initializes a MSSQL source
	db, err := initMssqlConnection(ctx, tracer, r.Name, r.Host, r.Port, r.User, r.Password, r.Database, r.Encrypt, r.AzureAuth)
	if err != nil {
		return nil, fmt.Errorf("unable to create db connection: %w", err)
	}

	// Verify db connection
	err = db.PingContext(ctx)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("unable to connect successfully: %w", err)
	}

	s := &Source{
		Config: r,
		Db:     db,
	}
	return s, nil
}

var _ sources.Source = &Source{}

type Source struct {
	Config
	Db *sql.DB
}

func (s *Source) IsReadOnly() bool {
	return false
}

func (s *Source) SourceType() string {
	// Returns Cloud SQL MSSQL source type
	return SourceType
}

func (s *Source) ToConfig() sources.SourceConfig {
	return s.Config
}

func (s *Source) MSSQLDB() *sql.DB {
	// Returns a Cloud SQL MSSQL database connection pool
	return s.Db
}

func (s *Source) RunSQL(ctx context.Context, statement string, params []any) (any, error) {
	results, err := s.MSSQLDB().QueryContext(ctx, statement, params...)
	if err != nil {
		return nil, fmt.Errorf("unable to execute query: %w", err)
	}
	defer results.Close()

	cols, err := results.Columns()
	// If Columns() errors, it might be a DDL/DML without an OUTPUT clause.
	// We proceed, and results.Err() will catch actual query execution errors.
	// 'out' will remain an empty slice if cols is empty or err is not nil here.
	out := []any{}
	if err == nil && len(cols) > 0 {
		// create an array of values for each column, which can be re-used to scan each row
		rawValues := make([]any, len(cols))
		values := make([]any, len(cols))
		for i := range rawValues {
			values[i] = &rawValues[i]
		}

		for results.Next() {
			scanErr := results.Scan(values...)
			if scanErr != nil {
				return nil, fmt.Errorf("unable to parse row: %w", scanErr)
			}
			row := orderedmap.Row{}
			for i, name := range cols {
				row.Add(name, rawValues[i])
			}
			out = append(out, row)
		}
	}

	// Check for errors from iterating over rows or from the query execution itself.
	// results.Close() is handled by defer.
	if err := results.Err(); err != nil {
		return nil, fmt.Errorf("errors encountered during query execution or row processing: %w", err)
	}

	return out, nil
}

func initMssqlConnection(
	ctx context.Context,
	tracer trace.Tracer,
	name, host, port, user, pass, dbname, encrypt string,
	azure *AzureAuthConfig,
) (
	*sql.DB,
	error,
) {
	//nolint:all // Reassigned ctx
	ctx, span := sources.InitConnectionSpan(ctx, tracer, SourceType, name)
	defer span.End()

	userAgent, err := util.UserAgentFromContext(ctx)
	if err != nil {
		userAgent = "genai-toolbox"
	}
	// Create dsn
	query := url.Values{}
	query.Add("app name", userAgent)
	query.Add("database", dbname)
	if encrypt != "" {
		query.Add("encrypt", encrypt)
	}

	// SQL authentication carries the login in the DSN user info. Entra
	// authentication leaves it empty and drives the credential from query
	// parameters instead, so the two cannot disagree about the identity.
	driverName := "sqlserver"
	var userInfo *url.Userinfo
	if azure == nil {
		userInfo = url.UserPassword(user, pass)
	} else {
		workflow, ok := azureFedAuth[azure.Mode]
		if !ok {
			return nil, fmt.Errorf("unsupported azureAuth mode %q", azure.Mode)
		}
		driverName = azuread.DriverName
		query.Add("fedauth", workflow)
		// The driver reads the client id, and optionally its tenant, from
		// 'user id' in clientID@tenantID form.
		if azure.ClientID != "" {
			userID := azure.ClientID
			if azure.TenantID != "" {
				userID = fmt.Sprintf("%s@%s", azure.ClientID, azure.TenantID)
			}
			query.Add("user id", userID)
		}
		if azure.DisableInstanceDiscovery {
			query.Add("disableinstancediscovery", "true")
		}
		if len(azure.AdditionallyAllowedTenants) > 0 {
			query.Add("additionallyallowedtenants", strings.Join(azure.AdditionallyAllowedTenants, ","))
		}
		// A service principal authenticates with a client secret, which the driver
		// reads from 'password'. No other mode uses it, so it is not sent otherwise.
		if azure.Mode == "service-principal" {
			query.Add("password", pass)
		}
	}

	url := &url.URL{
		Scheme:   "sqlserver",
		User:     userInfo,
		Host:     fmt.Sprintf("%s:%s", host, port),
		RawQuery: query.Encode(),
	}

	// Open database connection
	db, err := sql.Open(driverName, url.String())
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	return db, nil
}
