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
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/orderedmap"
	mssql "github.com/microsoft/go-mssqldb"
	"github.com/microsoft/go-mssqldb/azuread"
	"go.opentelemetry.io/otel/trace"
)

const SourceType string = "mssql"

// clientPoolIdleTimeout closes idle connections in a per-caller pool. These pools
// are created one per caller, so idle connections accumulate with the number of
// people connected rather than being bounded by the configuration.
const clientPoolIdleTimeout = 5 * time.Minute

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

// AzureOnBehalfOfConfig names the confidential client that exchanges the caller's
// token for one Azure SQL will accept, using the OAuth 2.0 on-behalf-of flow. It
// is needed whenever the token the client sent was issued for this server rather
// than for the database — the usual case, since an MCP client authenticates
// against Toolbox, so its token's audience is Toolbox. Omit the block to send the
// caller's token to the database as it arrived.
//
// The driver performs the exchange itself, and the server tells it which resource
// to request the token for, so there is no scope to configure here.
type AzureOnBehalfOfConfig struct {
	ClientID     string `yaml:"clientId" validate:"required"`
	ClientSecret string `yaml:"clientSecret" validate:"required"`
	TenantID     string `yaml:"tenantId" validate:"required"`
}

type Config struct {
	// Cloud SQL MSSQL configs
	Name string `yaml:"name" validate:"required"`
	Type string `yaml:"type" validate:"required"`
	Host string `yaml:"host" validate:"required"`
	Port string `yaml:"port" validate:"required"`
	// User and Password are the SQL login every request shares. They are not
	// required when the identity comes from elsewhere: azureAuth, an Entra identity
	// of the server's own, or useClientOAuth, the caller's. In service-principal
	// mode Password carries the client secret.
	User     string `yaml:"user" validate:"required_without_all=AzureAuth UseClientOAuth"`
	Password string `yaml:"password" validate:"required_without_all=AzureAuth UseClientOAuth"`
	Database string `yaml:"database" validate:"required"`
	Encrypt  string `yaml:"encrypt"`
	// AzureAuth switches the connection to Microsoft Entra ID authentication with
	// an identity of the server's own.
	AzureAuth *AzureAuthConfig `yaml:"azureAuth"`
	// UseClientOAuth authenticates to SQL Server as the caller instead of as one
	// configured identity, so the database applies that person's own permissions
	// and row-level security. Set it to "true" to take the token from the standard
	// Authorization header, or to a header name to take it from elsewhere.
	UseClientOAuth string `yaml:"useClientOAuth"`
	// AzureOnBehalfOf turns the caller's token into one the database will accept.
	// Only meaningful together with useClientOAuth.
	AzureOnBehalfOf *AzureOnBehalfOfConfig `yaml:"azureOnBehalfOf"`
}

func (r Config) SourceConfigType() string {
	// Returns Cloud SQL MSSQL source type
	return SourceType
}

// useClientOAuth reports whether the source authenticates as the caller. The field
// is a string so that it can name a header, so "false" and "" both mean off.
func (r Config) useClientOAuth() bool {
	return r.UseClientOAuth != "" && strings.ToLower(r.UseClientOAuth) != "false"
}

func (r Config) Initialize(ctx context.Context, tracer trace.Tracer) (sources.Source, error) {
	if r.useClientOAuth() {
		return r.initializeForClients(tracer)
	}
	if r.AzureOnBehalfOf != nil {
		return nil, fmt.Errorf("azureOnBehalfOf exchanges the caller's token, so it needs useClientOAuth")
	}
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
	} else if r.User == "" || r.Password == "" {
		// The validate tags let both be omitted when useClientOAuth is set, which
		// includes it being set to "false". Say so plainly rather than letting the
		// driver report a login failure for an empty user.
		return nil, fmt.Errorf("'user' and 'password' are required unless azureAuth or useClientOAuth is set")
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
		Config:              r,
		Db:                  db,
		AuthTokenHeaderName: "Authorization",
		tracer:              tracer,
	}
	return s, nil
}

// initializeForClients builds a source with no identity of its own: every
// connection is opened as the caller who asked. There is nothing to verify at
// startup, so nothing is; the first request carrying a token proves the database
// is reachable.
func (r Config) initializeForClients(tracer trace.Tracer) (sources.Source, error) {
	if r.User != "" || r.Password != "" || r.AzureAuth != nil {
		return nil, fmt.Errorf("useClientOAuth authenticates as the caller, so 'user', 'password' and 'azureAuth' must not be set")
	}
	s := &Source{
		Config:              r,
		AuthTokenHeaderName: "Authorization",
		tracer:              tracer,
		dbCache: sources.NewCache(func(_ string, value any) {
			if db, ok := value.(*sql.DB); ok && db != nil {
				db.Close()
			}
		}),
	}
	if strings.ToLower(r.UseClientOAuth) != "true" {
		s.AuthTokenHeaderName = r.UseClientOAuth
	}
	return s, nil
}

var _ sources.Source = &Source{}

type Source struct {
	Config
	Db                  *sql.DB
	AuthTokenHeaderName string

	tracer trace.Tracer
	// dbCache holds one connection pool per caller, keyed by a digest of their
	// token, and closes a pool when the entry expires. Nil unless useClientOAuth.
	dbCache *sources.Cache
	// mu serialises pool creation so that two requests from the same caller cannot
	// both open one.
	mu sync.Mutex
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

// UseClientAuthorization reports whether tools using this source must be given the
// caller's access token.
func (s *Source) UseClientAuthorization() bool {
	return s.useClientOAuth()
}

// GetAuthTokenHeaderName returns the header the caller's token is read from.
func (s *Source) GetAuthTokenHeaderName() string {
	return s.AuthTokenHeaderName
}

func (s *Source) RunSQL(ctx context.Context, statement string, params []any) (any, error) {
	return runSQL(ctx, s.MSSQLDB(), statement, params)
}

// RunSQLForClient runs a statement on a connection authenticated as the caller who
// sent accessToken, so SQL Server applies their permissions rather than a shared
// login's.
func (s *Source) RunSQLForClient(ctx context.Context, accessToken tools.AccessToken, statement string, params []any) (any, error) {
	db, err := s.DBForClient(ctx, accessToken)
	if err != nil {
		return nil, err
	}
	return runSQL(ctx, db, statement, params)
}

// DBForClient returns a connection pool that authenticates as the caller who sent
// accessToken. Pools are cached per token, so a caller's later requests reuse
// their own connections, and expire with the cache entry.
func (s *Source) DBForClient(ctx context.Context, accessToken tools.AccessToken) (*sql.DB, error) {
	if !s.UseClientAuthorization() {
		return s.MSSQLDB(), nil
	}
	assertion, err := accessToken.ParseBearerToken()
	if err != nil {
		return nil, fmt.Errorf("error parsing access token: %w", err)
	}

	// Key on a digest so that bearer tokens are not held as map keys for the life
	// of the entry.
	sum := sha256.Sum256([]byte(assertion))
	key := hex.EncodeToString(sum[:])

	if cached, ok := s.dbCache.Get(key); ok {
		return cached.(*sql.DB), nil
	}

	// A caller's first two requests can arrive together. Without this lock both
	// would open a pool, and the second Set would evict — and so close — the pool
	// the first request is still using. Opening a pool performs no I/O, so the
	// critical section stays cheap.
	s.mu.Lock()
	defer s.mu.Unlock()
	if cached, ok := s.dbCache.Get(key); ok {
		return cached.(*sql.DB), nil
	}

	db, err := s.openForClient(ctx, assertion)
	if err != nil {
		return nil, err
	}
	s.dbCache.Set(key, db)
	return db, nil
}

// openForClient builds a pool whose connections authenticate with a token belonging
// to the caller rather than with credentials from the configuration.
func (s *Source) openForClient(ctx context.Context, assertion string) (*sql.DB, error) {
	//nolint:all // Reassigned ctx
	ctx, span := sources.InitConnectionSpan(ctx, s.tracer, SourceType, s.Name)
	defer span.End()

	var connector *mssql.Connector
	var err error
	if obo := s.AzureOnBehalfOf; obo != nil {
		// The driver runs the on-behalf-of exchange itself: the confidential client
		// goes in 'user id' and 'password', and the caller's token is the user
		// assertion. It requests the resource the server names, so no scope is set.
		// NewConnector parses the DSN now, so a mistake in it is reported here
		// rather than on the first query.
		query := url.Values{}
		query.Add("fedauth", azuread.ActiveDirectoryOnBehalfOf)
		query.Add("user id", fmt.Sprintf("%s@%s", obo.ClientID, obo.TenantID))
		query.Add("password", obo.ClientSecret)
		query.Add("userassertion", assertion)
		connector, err = azuread.NewConnector(buildDSN(ctx, s.Host, s.Port, s.Database, s.Encrypt, nil, query))
	} else {
		// The caller already holds a token for the database, so it is presented as
		// it arrived. Nothing here can confirm its audience is right; the server
		// refuses it if it is not.
		connector, err = mssql.NewConnectorWithAccessTokenProvider(
			buildDSN(ctx, s.Host, s.Port, s.Database, s.Encrypt, nil, url.Values{}),
			func(context.Context) (string, error) { return assertion, nil },
		)
	}
	if err != nil {
		return nil, fmt.Errorf("unable to create connector for the caller: %w", err)
	}

	db := sql.OpenDB(connector)
	// One pool per caller, so the package defaults would multiply by the number of
	// people connected.
	db.SetMaxIdleConns(1)
	db.SetConnMaxIdleTime(clientPoolIdleTimeout)
	return db, nil
}

func runSQL(ctx context.Context, db *sql.DB, statement string, params []any) (any, error) {
	if db == nil {
		return nil, fmt.Errorf("no database connection available")
	}
	results, err := db.QueryContext(ctx, statement, params...)
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

// buildDSN assembles the connection string. userInfo carries a SQL login, or is
// nil when the credential travels in query parameters instead — the Entra modes,
// and the per-caller path — so the two can never disagree about the identity.
// query must be non-nil; the common parameters are added to it.
func buildDSN(ctx context.Context, host, port, dbname, encrypt string, userInfo *url.Userinfo, query url.Values) string {
	userAgent, err := util.UserAgentFromContext(ctx)
	if err != nil {
		userAgent = "genai-toolbox"
	}
	query.Add("app name", userAgent)
	query.Add("database", dbname)
	if encrypt != "" {
		query.Add("encrypt", encrypt)
	}

	dsn := &url.URL{
		Scheme:   "sqlserver",
		User:     userInfo,
		Host:     fmt.Sprintf("%s:%s", host, port),
		RawQuery: query.Encode(),
	}
	return dsn.String()
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

	driverName := "sqlserver"
	query := url.Values{}
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

	// Open database connection
	db, err := sql.Open(driverName, buildDSN(ctx, host, port, dbname, encrypt, userInfo, query))
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	return db, nil
}
