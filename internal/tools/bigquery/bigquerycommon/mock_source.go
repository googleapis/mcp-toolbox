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

package bigquerycommon

import (
	"context"
	"fmt"

	bigqueryapi "cloud.google.com/go/bigquery"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	bigqueryds "github.com/googleapis/mcp-toolbox/internal/sources/bigquery"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	bigqueryrestapi "google.golang.org/api/bigquery/v2"
)

// MockSource is a reusable mock implementation of sources.Source for BigQuery tool tests.
type MockSource struct {
	sources.Source
	CalledSQL       string
	Client          *bigqueryapi.Client
	Service         *bigqueryrestapi.Service
	AllowedDatasets []string
	RunSQLResult    any
	RunSQLError     error
}

func (m *MockSource) BigQueryClient() *bigqueryapi.Client {
	return m.Client
}

func (m *MockSource) UseClientAuthorization() bool {
	return false
}

func (m *MockSource) BigQueryWriteMode() string {
	return "allowed"
}

func (m *MockSource) GetAuthTokenHeaderName() string {
	return ""
}

func (m *MockSource) GetMaximumBytesBilled() int64 {
	return 0
}

func (m *MockSource) IsDatasetAllowed(projectID, datasetID string) bool {
	if len(m.AllowedDatasets) == 0 {
		return true
	}
	target := fmt.Sprintf("%s.%s", projectID, datasetID)
	for _, allowed := range m.AllowedDatasets {
		if allowed == target || allowed == datasetID {
			return true
		}
	}
	return false
}

func (m *MockSource) BigQueryAllowedDatasets() []string {
	return m.AllowedDatasets
}

func (m *MockSource) BigQuerySession() bigqueryds.BigQuerySessionProvider {
	return func(ctx context.Context) (*bigqueryds.Session, error) {
		return &bigqueryds.Session{ID: "mock-session-id"}, nil
	}
}

func (m *MockSource) RetrieveClientAndService(tools.AccessToken) (*bigqueryapi.Client, *bigqueryrestapi.Service, error) {
	return m.Client, m.Service, nil
}

func (m *MockSource) RunSQL(ctx context.Context, client *bigqueryapi.Client, sql string, queryType string, params []bigqueryapi.QueryParameter, connProps []*bigqueryapi.ConnectionProperty, labels map[string]string) (any, error) {
	m.CalledSQL = sql
	return m.RunSQLResult, m.RunSQLError
}
