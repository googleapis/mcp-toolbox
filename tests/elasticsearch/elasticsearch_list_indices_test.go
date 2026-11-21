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

package elasticsearch

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/esapi"

	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

func TestElasticsearchListIndices(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	address, containerCleanup := setupElasticsearchContainer(ctx, t)
	defer containerCleanup()

	indexA, indexB, hidden := "list-indices-a", "list-indices-b", ".list-indices-hidden"
	esClient, err := elasticsearch.NewBase(
		elasticsearch.WithAddresses(address),
		elasticsearch.WithBasicAuth(EsUser, EsPass),
	)
	if err != nil {
		t.Fatalf("error creating the Elasticsearch client: %s", err)
	}
	defer func() {
		if _, err := (esapi.IndicesDeleteRequest{Index: []string{indexA, indexB, hidden}}).Do(ctx, esClient); err != nil {
			t.Errorf("error deleting indices: %s", err)
		}
	}()
	for _, name := range []string{indexA, indexB, hidden} {
		if res, err := (esapi.IndicesCreateRequest{Index: name}).Do(ctx, esClient); err != nil || res.IsError() {
			t.Fatalf("error creating index %s: %v %v", name, err, res)
		}
	}
	for _, doc := range []string{`{"n":1}`, `{"n":2}`} {
		res, err := (esapi.IndexRequest{Index: indexA, Body: strings.NewReader(doc), Refresh: "true"}).Do(ctx, esClient)
		if err != nil || res.IsError() {
			t.Fatalf("error indexing document: %v %v", err, res)
		}
	}

	toolsFile := map[string]any{
		"sources": map[string]any{
			"my-instance": map[string]any{
				"type":      ElasticsearchSourceType,
				"addresses": []string{address},
				"username":  EsUser,
				"password":  EsPass,
			},
		},
		"tools": map[string]any{
			"my-list-indices-tool": map[string]any{
				"type":        "elasticsearch-list-indices",
				"source":      "my-instance",
				"description": "List Elasticsearch indices.",
			},
		},
	}
	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile)
	if err != nil {
		t.Fatalf("failed to start cmd: %v", err)
	}
	defer cleanup()
	waitCtx, cancelWait := context.WithTimeout(ctx, 10*time.Second)
	defer cancelWait()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	simpleKeys := []string{"docs_count", "health", "index", "status", "store_size"}
	detailedKeys := append([]string{"creation_date", "docs_deleted", "pri", "rep", "uuid"}, simpleKeys...)
	slices.Sort(detailedKeys)

	tcs := []struct {
		name      string
		args      map[string]any
		wantNames []string
		wantKeys  []string
		wantErr   string
		exact     bool
	}{
		{name: "default", args: map[string]any{}, wantNames: []string{indexA, indexB}, wantKeys: simpleKeys},
		{name: "detailed", args: map[string]any{"output_format": "detailed"}, wantNames: []string{indexA, indexB}, wantKeys: detailedKeys},
		{name: "prefix pattern", args: map[string]any{"patterns": "list-indices-*"}, wantNames: []string{indexA, indexB}, wantKeys: simpleKeys, exact: true},
		{name: "dot pattern", args: map[string]any{"patterns": ".list-indices-*"}, wantNames: []string{hidden}, wantKeys: simpleKeys, exact: true},
		{name: "comma separated patterns", args: map[string]any{"patterns": "list-indices-a, .list-indices-*"}, wantNames: []string{hidden, indexA}, wantKeys: simpleKeys, exact: true},
		{name: "no match", args: map[string]any{"patterns": "no-such-prefix-*"}, wantNames: []string{}, exact: true},
		{name: "missing index", args: map[string]any{"patterns": "no-such-index"}, wantErr: "index_not_found_exception"},
		{name: "invalid output_format", args: map[string]any{"output_format": "verbose"}, wantErr: "must be 'simple' or 'detailed'"},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			status, resp, err := tests.InvokeMCPTool(t, "my-list-indices-tool", tc.args, nil)
			if err != nil {
				t.Fatalf("native error executing tool: %s", err)
			}
			if status != http.StatusOK {
				t.Fatalf("expected status 200, got %d", status)
			}
			if tc.wantErr != "" {
				if !resp.Result.IsError {
					t.Fatalf("expected isError result, got %v", resp.Result)
				}
				tests.AssertMCPError(t, resp, tc.wantErr)
				return
			}
			if resp.Result.IsError {
				t.Fatalf("unexpected error result: %v", resp.Result)
			}
			rows := parseIndexRows(t, resp)
			var names []string
			for _, row := range rows {
				name := row["index"].(string)
				if strings.HasPrefix(name, ".") && !strings.HasPrefix(name, ".list-indices-") {
					continue
				}
				if !tc.exact && !strings.HasPrefix(name, "list-indices-") {
					continue
				}
				names = append(names, name)
				keys := make([]string, 0, len(row))
				for k := range row {
					keys = append(keys, k)
				}
				slices.Sort(keys)
				if !slices.Equal(keys, tc.wantKeys) {
					t.Errorf("row %s keys = %v, want %v", name, keys, tc.wantKeys)
				}
				if name == indexA && row["docs_count"] != "2" {
					t.Errorf("docs_count of %s = %v, want 2", name, row["docs_count"])
				}
			}
			if !slices.IsSorted(names) {
				t.Errorf("rows not sorted by index name: %v", names)
			}
			if !slices.Equal(names, tc.wantNames) {
				t.Errorf("index names = %v, want %v", names, tc.wantNames)
			}
		})
	}
}

func parseIndexRows(t *testing.T, resp *tests.MCPCallToolResponse) []map[string]any {
	t.Helper()
	rows := []map[string]any{}
	for _, c := range resp.Result.Content {
		var batch []map[string]any
		if err := json.Unmarshal([]byte(c.Text), &batch); err != nil {
			var row map[string]any
			if err := json.Unmarshal([]byte(c.Text), &row); err != nil {
				t.Fatalf("unable to parse result text %q: %s", c.Text, err)
			}
			batch = []map[string]any{row}
		}
		rows = append(rows, batch...)
	}
	return rows
}
