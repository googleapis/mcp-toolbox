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

package arcadedb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools/neo4j/neo4jexecutecypher/classifier"
	"github.com/googleapis/mcp-toolbox/internal/tools/neo4j/neo4jschema/helpers"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	neo4jconf "github.com/neo4j/neo4j-go-driver/v6/neo4j/config"

	"go.opentelemetry.io/otel/trace"
)

const SourceType string = "arcadedb"

var sourceClassifier *classifier.QueryClassifier = classifier.NewQueryClassifier()

var arcadeHTTPClient = &http.Client{
	Timeout: 60 * time.Second,
}

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
	Name       string `yaml:"name" validate:"required"`
	Type       string `yaml:"type" validate:"required"`
	Uri        string `yaml:"uri" validate:"required"`
	User       string `yaml:"user" validate:"required"`
	Password   string `yaml:"password" validate:"required"`
	Database   string `yaml:"database" validate:"required"`
	HTTPUri    string `yaml:"httpUri"`
	HTTPScheme string `yaml:"httpScheme"`
	HTTPPort   int    `yaml:"httpPort"`
}

func (r Config) SourceConfigType() string {
	return SourceType
}

func (r Config) Initialize(ctx context.Context, tracer trace.Tracer, deferConnect bool) (sources.Source, error) {
	s := r.newSource(ctx, tracer)
	if deferConnect {
		return s, nil
	}
	if _, err := s.driver(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (r Config) newSource(ctx context.Context, tracer trace.Tracer) *Source {
	return &Source{
		Config: r,
		conn:   sources.NewConnectOnce[neo4j.Driver](ctx, r.Name, SourceType, tracer),
	}
}

var _ sources.Source = &Source{}

type Source struct {
	Config
	conn *sources.ConnectOnce[neo4j.Driver]
}

func (s *Source) driver(ctx context.Context) (neo4j.Driver, error) {
	return s.conn.Do(ctx, func(ctx context.Context) (neo4j.Driver, error) {
		driver, err := initArcadeDBDriver(ctx, s.Uri, s.User, s.Password)
		if err != nil {
			return nil, fmt.Errorf("unable to create driver: %w", err)
		}
		if err := driver.VerifyConnectivity(ctx); err != nil {
			driver.Close(ctx)
			return nil, fmt.Errorf("unable to connect successfully: %w", err)
		}
		return driver, nil
	})
}

func (s *Source) IsReadOnly() bool {
	return false
}

func (s *Source) SourceType() string {
	return SourceType
}

func (s *Source) ToConfig() sources.SourceConfig {
	return s.Config
}

// ArcadeDBDriver reports the driver once connected.
func (s *Source) ArcadeDBDriver() neo4j.Driver {
	driver, _ := s.conn.Get()
	return driver
}

func (s *Source) ArcadeDBDatabase() string {
	return s.Database
}

func (s *Source) RunCypher(ctx context.Context, cypherStr string, params map[string]any, readOnly, dryRun bool) (any, error) {
	cf := sourceClassifier.Classify(cypherStr)
	if cf.Error != nil {
		return nil, cf.Error
	}

	if cf.Type == classifier.WriteQuery && readOnly {
		return nil, fmt.Errorf("this tool is read-only and cannot execute write queries")
	}

	if dryRun {
		cypherStr = "EXPLAIN " + cypherStr
	}

	driver, err := s.driver(ctx)
	if err != nil {
		return nil, err
	}

	config := neo4j.ExecuteQueryWithDatabase(s.ArcadeDBDatabase())
	results, err := neo4j.ExecuteQuery[*neo4j.EagerResult](ctx, driver, cypherStr, params,
		neo4j.EagerResultTransformer, config)
	if err != nil {
		return nil, fmt.Errorf("unable to execute query: %w", err)
	}

	if dryRun {
		summary := results.Summary
		plan := summary.Plan()
		if plan == nil {
			return nil, fmt.Errorf("dry-run produced no execution plan")
		}

		node, incomplete, operatorCount := buildPlanNode(plan)
		if operatorCount == 0 {
			return nil, fmt.Errorf("dry-run produced an empty execution plan")
		}

		execPlan := map[string]any{
			"queryType":     cf.Type.String(),
			"statementType": summary.QueryType(),
		}
		for k, v := range node {
			execPlan[k] = v
		}
		if incomplete {
			execPlan["plan_incomplete"] = true
			execPlan["warning"] = "Execution plan appears partial; server may not provide complete EXPLAIN details for this query/version."
		}
		return []map[string]any{execPlan}, nil
	}

	return formatRecords(results.Keys, results.Records), nil
}

func (s *Source) RunSQL(ctx context.Context, sqlStr string, params map[string]any, readOnly bool) (any, error) {
	endpoint := "command"
	if readOnly {
		endpoint = "query"
	}
	httpURL, err := s.arcadeHTTPEndpointURL(endpoint)
	if err != nil {
		return nil, err
	}

	requestBody := map[string]any{
		"language": "sql",
		"command":  sqlStr,
	}
	if params != nil {
		requestBody["params"] = params
	}

	payload, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("unable to serialize SQL request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, httpURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("unable to create SQL request: %w", err)
	}
	req.SetBasicAuth(s.User, s.Password)
	req.Header.Set("Content-Type", "application/json")

	resp, err := arcadeHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("unable to execute SQL request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("unable to read SQL response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unable to execute SQL request: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("unable to parse SQL response: %w", err)
	}

	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(sqlStr)), "EXPLAIN") {
		resp := map[string]any{}
		if plan, ok := decoded["explainPlan"]; ok {
			resp["executionPlan"] = plan
		}
		if explainText, ok := decoded["explain"]; ok {
			resp["executionPlanAsString"] = explainText
		}
		if len(resp) > 0 {
			return []map[string]any{resp}, nil
		}
	}

	result, ok := decoded["result"]
	if !ok {
		return []map[string]any{}, nil
	}

	return result, nil
}

func formatRecords(keys []string, records []*neo4j.Record) []map[string]any {
	out := []map[string]any{}
	for _, record := range records {
		vMap := make(map[string]any)
		for col, value := range record.Values {
			vMap[keys[col]] = helpers.ConvertValue(value)
		}
		out = append(out, vMap)
	}
	return out
}

func buildPlanNode(p neo4j.Plan) (map[string]any, bool, int) {
	children := p.Children()
	node := map[string]any{
		"arguments":     p.Arguments(),
		"identifiers":   p.Identifiers(),
		"childrenCount": len(children),
	}

	incomplete := false
	operatorCount := 0
	if operator := p.Operator(); operator != "" {
		node["operator"] = operator
		operatorCount = 1
	} else {
		incomplete = true
	}

	if len(children) > 0 {
		childNodes := make([]map[string]any, 0, len(children))
		for _, child := range children {
			childNode, childIncomplete, childOperatorCount := buildPlanNode(child)
			childNodes = append(childNodes, childNode)
			if childIncomplete {
				incomplete = true
			}
			operatorCount += childOperatorCount
		}
		node["children"] = childNodes
	}

	return node, incomplete, operatorCount
}

func (s *Source) arcadeHTTPEndpointURL(endpoint string) (string, error) {
	base := s.HTTPUri
	if base == "" {
		parsed, err := url.Parse(s.Uri)
		if err != nil {
			return "", fmt.Errorf("unable to parse arcade uri %q: %w", s.Uri, err)
		}
		host := parsed.Hostname()
		if host == "" {
			return "", fmt.Errorf("unable to parse host from arcade uri %q", s.Uri)
		}
		scheme := s.HTTPScheme
		if scheme == "" {
			scheme = "http"
		}
		port := s.HTTPPort
		if port == 0 {
			port = 2480
		}
		base = fmt.Sprintf("%s://%s:%d", scheme, host, port)
	}
	return fmt.Sprintf("%s/api/v1/%s/%s",
		strings.TrimRight(base, "/"),
		endpoint,
		url.PathEscape(s.ArcadeDBDatabase())), nil
}

func initArcadeDBDriver(ctx context.Context, uri, user, password string) (neo4j.Driver, error) {
	auth := neo4j.BasicAuth(user, password, "")
	userAgent, err := util.UserAgentFromContext(ctx)
	if err != nil {
		return nil, err
	}
	driver, err := neo4j.NewDriver(uri, auth, func(config *neo4jconf.Config) {
		config.UserAgent = userAgent
	})
	if err != nil {
		return nil, fmt.Errorf("unable to create connection driver: %w", err)
	}
	return driver, nil
}
