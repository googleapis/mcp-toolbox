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

package postgreslistroles

import (
	"context"
	"fmt"
	"net/http"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"github.com/jackc/pgx/v5/pgxpool"
)

const resourceType string = "postgres-list-roles"

const listRolesStatement = `
    WITH RoleDetails AS (
        SELECT
            r.rolname AS role_name,
            r.oid AS oid,
            r.rolconnlimit AS connection_limit,
            r.rolsuper AS is_superuser,
            r.rolinherit AS inherits_privileges,
            r.rolcreaterole AS can_create_roles,
            r.rolcreatedb AS can_create_db,
            r.rolcanlogin AS can_login,
            r.rolreplication AS is_replication_role,
            r.rolbypassrls AS bypass_rls,
            r.rolvaliduntil AS valid_until,
            -- List of roles that belong to this role (Direct Members)
            ARRAY(
                SELECT m_r.rolname
                FROM pg_auth_members pam
                JOIN pg_roles m_r ON pam.member = m_r.oid
                WHERE pam.roleid = r.oid
            ) AS direct_members,
            -- List of roles that this role belongs to (Member Of)
            ARRAY(
                SELECT g_r.rolname
                FROM pg_auth_members pam
                JOIN pg_roles g_r ON pam.roleid = g_r.oid
                WHERE pam.member = r.oid
            ) AS member_of
        FROM pg_roles r
    -- Exclude system and internal roles
        WHERE r.rolname NOT LIKE 'cloudsql%'
        AND r.rolname NOT LIKE 'alloydb_%'
        AND r.rolname NOT LIKE 'pg_%'
    )
    SELECT *
    FROM RoleDetails
    WHERE
        ($1::text IS NULL OR role_name LIKE '%' || $1 || '%')
    ORDER BY role_name
    LIMIT COALESCE($2::int, 50);
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
	PostgresPool() *pgxpool.Pool
	RunSQL(context.Context, string, []any) (any, error)
}

type Config struct {
	tools.ConfigBase `yaml:",inline"`
	Type             string `yaml:"type" validate:"required"`
	Source           string `yaml:"source" validate:"required"`
}

var _ tools.ToolConfig = Config{}

func (cfg Config) ToolConfigType() string {
	return resourceType
}

func (cfg Config) Initialize(context.Context) (tools.Tool, error) {
	allParameters := parameters.Parameters{
		parameters.NewStringParameter("role_name", "Optional: a text to filter results by role name. The input is used within a LIKE clause.", parameters.WithStringDefault("")),
		parameters.NewIntParameter("limit", "Optional: The maximum number of rows to return. Default is 50", parameters.WithIntDefault(50)),
	}

	if cfg.Description == "" {
		cfg.Description = "Lists all the user-created roles in the instance . It returns the role name, Object ID, the maximum number of concurrent connections the role can make, along with boolean indicators for: superuser status, privilege inheritance from member roles, ability to create roles, ability to create databases, ability to log in, replication privilege, and the ability to bypass row-level security, the password expiration timestamp, a list of direct members belonging to this role, and a list of other roles/groups that this role is a member of."
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

func (t Tool) Invoke(ctx context.Context, s sources.Source, params parameters.ParamValues, accessToken tools.AccessToken) (any, util.ToolboxError) {
	source, ok := s.(compatibleSource)
	if !ok {
		return nil, util.NewClientServerError("source used is not compatible with the tool", http.StatusInternalServerError, nil)
	}
	paramsMap := params.AsMap()

	newParams, err := parameters.GetParams(t.StaticParameters, paramsMap)
	if err != nil {
		return nil, util.NewAgentError("unable to extract standard params", err)
	}
	sliceParams := newParams.AsSlice()
	resp, err := source.RunSQL(ctx, listRolesStatement, sliceParams)
	if err != nil {
		return nil, util.ProcessGeneralError(err)
	}
	return resp, nil
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
