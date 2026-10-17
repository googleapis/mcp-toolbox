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

package mindsdb

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

var (
	MindsDBSourceType = "mindsdb"
	MindsDBToolType   = "mindsdb-sql"
	MindsDBDatabase   = os.Getenv("MINDSDB_DATABASE")
	MindsDBHost       = os.Getenv("MINDSDB_HOST")
	MindsDBPort       = os.Getenv("MINDSDB_PORT")
	MindsDBUser       = os.Getenv("MINDSDB_USER")
	MindsDBPass       = os.Getenv("MINDSDB_PASS")
)

func getMindsDBVars(t *testing.T) map[string]any {
	switch "" {
	case MindsDBDatabase:
		t.Fatal("'MINDSDB_DATABASE' not set")
	case MindsDBHost:
		t.Fatal("'MINDSDB_HOST' not set")
	case MindsDBPort:
		t.Fatal("'MINDSDB_PORT' not set")
	case MindsDBUser:
		t.Fatal("'MINDSDB_USER' not set")
	}

	// MindsDBPass can be empty, but the env var must exist
	if _, exists := os.LookupEnv("MINDSDB_PASS"); !exists {
		t.Fatal("'MINDSDB_PASS' not set (can be empty)")
	}

	// Handle no-password authentication
	mindsdbPassword := MindsDBPass
	if mindsdbPassword == "none" {
		mindsdbPassword = ""
	}

	return map[string]any{
		"type":     MindsDBSourceType,
		"host":     MindsDBHost,
		"port":     MindsDBPort,
		"database": MindsDBDatabase,
		"user":     MindsDBUser,
		"password": mindsdbPassword,
	}
}

// initMindsDBConnectionPool creates a connection pool using MySQL protocol
func initMindsDBConnectionPool(host, port, user, pass, dbname string) (*sql.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true", user, pass, host, port, dbname)
	pool, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	return pool, nil
}

// mindsDBCleanupTimeout bounds each cleanup query. Cleanups detach from the
// test context's cancellation (it is already cancelled when they run), so they
// need their own deadline to fail fast if the database stops responding.
const mindsDBCleanupTimeout = 30 * time.Second

// mindsDBSelect1Want is the expected result of "SELECT 1".
const mindsDBSelect1Want = "[{\"1\":1}]"

// getMindsDBToolsConfig returns the tools file shared by the MindsDB
// integration tests.
func getMindsDBToolsConfig(sourceConfig map[string]any, tableNameParam, tableNameAuth string) map[string]any {
	// Tool statements with ORDER BY for consistent results
	paramToolStmt := fmt.Sprintf("SELECT * FROM files.%s WHERE id = ? OR name = ? ORDER BY id", tableNameParam)
	idParamToolStmt := fmt.Sprintf("SELECT * FROM files.%s WHERE id = ? ORDER BY id", tableNameParam)
	nameParamToolStmt := fmt.Sprintf("SELECT * FROM files.%s WHERE name = ? ORDER BY id", tableNameParam)
	authToolStmt := fmt.Sprintf("SELECT name FROM files.%s WHERE email = ? ORDER BY name", tableNameAuth)

	return map[string]any{
		"sources": map[string]any{
			"my-instance": sourceConfig,
		},
		"authServices": map[string]any{
			"my-google-auth": map[string]any{
				"type":     "google",
				"clientId": tests.ClientId,
			},
		},
		"tools": map[string]any{
			"my-simple-tool": map[string]any{
				"type":        MindsDBToolType,
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
				"statement":   "SELECT 1",
			},
			"my-tool": map[string]any{
				"type":        MindsDBToolType,
				"source":      "my-instance",
				"description": "Tool to test invocation with params.",
				"statement":   paramToolStmt,
				"parameters": []map[string]any{
					{
						"name":        "id",
						"type":        "integer",
						"description": "user ID",
					},
					{
						"name":        "name",
						"type":        "string",
						"description": "user name",
					},
				},
			},
			"my-tool-by-id": map[string]any{
				"type":        MindsDBToolType,
				"source":      "my-instance",
				"description": "Tool to test invocation with params.",
				"statement":   idParamToolStmt,
				"parameters": []map[string]any{
					{
						"name":        "id",
						"type":        "integer",
						"description": "user ID",
					},
				},
			},
			"my-tool-by-name": map[string]any{
				"type":        MindsDBToolType,
				"source":      "my-instance",
				"description": "Tool to test invocation with params.",
				"statement":   nameParamToolStmt,
				"parameters": []map[string]any{
					{
						"name":        "name",
						"type":        "string",
						"description": "user name",
						"required":    false,
					},
				},
			},
			"my-array-tool": map[string]any{
				"type":        MindsDBToolType,
				"source":      "my-instance",
				"description": "Tool to test invocation with array params.",
				"statement":   "SELECT 1 as id, 'Alice' as name UNION SELECT 3 as id, 'Sid' as name",
			},
			"my-secure-tool": map[string]any{
				"type":        MindsDBToolType,
				"source":      "my-instance",
				"description": "Tool to test secure parameters.",
				"statement":   "SELECT 1 as id, 'Alice' as name UNION SELECT 3 as id, 'Sid' as name",
				"parameters": []map[string]any{
					{
						"name":        "id",
						"type":        "integer",
						"description": "user ID",
					},
					{
						"name":        "name",
						"type":        "string",
						"description": "user name",
						"secure":      true,
					},
				},
			},
			"my-auth-tool": map[string]any{
				"type":        MindsDBToolType,
				"source":      "my-instance",
				"description": "Tool to test authenticated parameters.",
				"statement":   authToolStmt,
				"parameters": []map[string]any{
					{
						"name":        "email",
						"type":        "string",
						"description": "user email",
						"authServices": []map[string]string{
							{
								"name":  "my-google-auth",
								"field": "email",
							},
						},
					},
				},
			},
			"my-auth-required-tool": map[string]any{
				"type":        MindsDBToolType,
				"source":      "my-instance",
				"description": "Tool to test auth required invocation.",
				"statement":   "SELECT 1",
				"authRequired": []string{
					"my-google-auth",
				},
			},
			"my-fail-tool": map[string]any{
				"type":        MindsDBToolType,
				"source":      "my-instance",
				"description": "Tool to test statement with incorrect syntax.",
				"statement":   "INVALID SQL STATEMENT",
			},
			"my-exec-sql-tool": map[string]any{
				"type":        "mindsdb-execute-sql",
				"source":      "my-instance",
				"description": "Tool to execute sql",
			},
			"my-auth-exec-sql-tool": map[string]any{
				"type":        "mindsdb-execute-sql",
				"source":      "my-instance",
				"description": "Tool to execute sql with auth",
				"authRequired": []string{
					"my-google-auth",
				},
			},
		},
	}
}

// setupMindsDBTest creates the param and auth tables and returns the tools
// file. The tables are dropped and the pool closed on cleanup.
func setupMindsDBTest(t *testing.T, ctx context.Context) map[string]any {
	t.Helper()
	sourceConfig := getMindsDBVars(t)

	// Create unique table names with UUID
	tableNameParam := "param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameAuth := "auth_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")

	pool, err := initMindsDBConnectionPool(MindsDBHost, MindsDBPort, MindsDBUser, MindsDBPass, MindsDBDatabase)
	if err != nil {
		t.Fatalf("unable to create MindsDB connection pool: %s", err)
	}
	t.Cleanup(func() {
		if err := pool.Close(); err != nil {
			t.Errorf("unable to close MindsDB connection pool: %s", err)
		}
	})

	// Registered before the tables are created so a partially failed setup is
	// still cleaned up. Runs before the pool is closed.
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), mindsDBCleanupTimeout)
		defer cancel()
		_, _ = pool.ExecContext(cleanupCtx, fmt.Sprintf("DROP TABLE IF EXISTS files.%s", tableNameParam))
		_, _ = pool.ExecContext(cleanupCtx, fmt.Sprintf("DROP TABLE IF EXISTS files.%s", tableNameAuth))
	})

	// Create param table: id=1:Alice, id=2:Jane, id=3:Sid, id=4:null
	createParamSQL := fmt.Sprintf("CREATE TABLE files.%s (SELECT 1 as id, 'Alice' as name UNION ALL SELECT 2, 'Jane' UNION ALL SELECT 3, 'Sid' UNION ALL SELECT 4, NULL)", tableNameParam)
	if _, err := pool.ExecContext(ctx, createParamSQL); err != nil {
		t.Fatalf("unable to create param table: %s", err)
	}

	// Create auth table: id=1:Alice:test@..., id=2:Jane:jane@...
	createAuthSQL := fmt.Sprintf("CREATE TABLE files.%s (SELECT 1 as id, 'Alice' as name, '%s' as email UNION ALL SELECT 2, 'Jane', 'janedoe@gmail.com')", tableNameAuth, tests.ServiceAccountEmail)
	if _, err := pool.ExecContext(ctx, createAuthSQL); err != nil {
		t.Fatalf("unable to create auth table: %s", err)
	}

	return getMindsDBToolsConfig(sourceConfig, tableNameParam, tableNameAuth)
}

// mindsDBTransport selects how the tests talk to the toolbox server: the
// legacy REST API, or the MCP endpoint when isMCP is set.
type mindsDBTransport struct {
	isMCP bool
}

// mindsDBResult is the outcome of a tool invocation. result holds the tool
// result as the REST API returns it: for MCP, the per-row content blocks are
// parsed and flattened into a single JSON array, or the error text when the
// call returned an error. toolErr is set when MCP reported an error.
type mindsDBResult struct {
	status  int
	result  string
	toolErr bool
}

// startServer starts the toolbox server with toolsFile and waits until it is
// ready to serve. The REST API is only enabled for the non-MCP transport.
func (tr mindsDBTransport) startServer(t *testing.T, ctx context.Context, toolsFile map[string]any) {
	t.Helper()
	var args []string
	if !tr.isMCP {
		args = append(args, "--enable-api")
	}
	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	t.Cleanup(cleanup)
	t.Cleanup(cmd.Close)

	waitCtx, waitCancel := context.WithTimeout(ctx, 10*time.Second)
	defer waitCancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}
}

// invoke calls toolName with args and request headers through the selected
// transport.
func (tr mindsDBTransport) invoke(t *testing.T, ctx context.Context, toolName string, args map[string]any, headers map[string]string) mindsDBResult {
	t.Helper()
	if tr.isMCP {
		statusCode, mcpResp, err := tests.InvokeMCPTool(t, toolName, args, headers)
		if err != nil {
			return mindsDBResult{status: statusCode, result: err.Error(), toolErr: true}
		}
		if mcpResp.Error != nil {
			return mindsDBResult{status: statusCode, result: mcpResp.Error.Message, toolErr: true}
		}
		if mcpResp.Result.IsError {
			var errText strings.Builder
			for _, content := range mcpResp.Result.Content {
				errText.WriteString(content.Text)
			}
			return mindsDBResult{status: statusCode, result: errText.String(), toolErr: true}
		}
		rows := []any{}
		for _, content := range mcpResp.Result.Content {
			var item any
			if err := json.Unmarshal([]byte(content.Text), &item); err != nil {
				rows = append(rows, content.Text)
				continue
			}
			if slice, ok := item.([]any); ok {
				rows = append(rows, slice...)
			} else {
				rows = append(rows, item)
			}
		}
		b, err := json.Marshal(rows)
		if err != nil {
			t.Fatalf("error marshaling MCP result: %s", err)
		}
		return mindsDBResult{status: statusCode, result: string(b)}
	}

	reqBytes, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("error marshaling request body: %s", err)
	}
	api := fmt.Sprintf("http://127.0.0.1:5000/api/tool/%s/invoke", toolName)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, api, bytes.NewBuffer(reqBytes))
	if err != nil {
		t.Fatalf("error creating request: %s", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("error when sending request: %s", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("error reading response body: %s", err)
	}
	if resp.StatusCode != http.StatusOK {
		return mindsDBResult{status: resp.StatusCode, result: string(respBody)}
	}
	var body map[string]any
	if err := json.Unmarshal(respBody, &body); err != nil {
		t.Fatalf("error parsing response body %q: %s", string(respBody), err)
	}
	result, ok := body["result"].(string)
	if !ok {
		t.Fatalf("unable to find result in response body: %s", string(respBody))
	}
	return mindsDBResult{status: resp.StatusCode, result: result}
}

// expectResult invokes toolName in a subtest and checks for a successful
// response whose result contains want. Like the shared REST helpers it replaces,
// it doesn't fail on a tool error when want is empty.
func (tr mindsDBTransport) expectResult(t *testing.T, ctx context.Context, toolName string, args map[string]any, want string) {
	t.Helper()
	t.Run(fmt.Sprintf("invoke %s", toolName), func(t *testing.T) {
		res := tr.invoke(t, ctx, toolName, args, nil)
		if res.status != http.StatusOK {
			t.Fatalf("response status code is not 200, got %d: %s", res.status, res.result)
		}
		if !strings.Contains(res.result, want) {
			t.Fatalf("unexpected value: got %q, want %q", res.result, want)
		}
	})
}

func TestMindsDBToolEndpoints(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	toolsFile := setupMindsDBTest(t, ctx)

	tr := mindsDBTransport{}
	tr.startServer(t, ctx, toolsFile)

	// Run standard tool tests with MindsDB-specific expectations
	tests.RunToolGetTest(t)
	tests.RunToolInvokeTest(t, mindsDBSelect1Want,
		tests.DisableArrayTest(), // MindsDB doesn't support array parameters
	)

	runMindsDBTests(t, ctx, tr)
}

// runMindsDBTests runs the MindsDB SQL, integration, error handling and auth
// checks.
func runMindsDBTests(t *testing.T, ctx context.Context, tr mindsDBTransport) {
	t.Run("mindsdb_core_functionality", func(t *testing.T) {
		tr.expectResult(t, ctx, "my-simple-tool", map[string]any{}, mindsDBSelect1Want)
		tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": "SELECT 1"}, mindsDBSelect1Want)
		tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": "SELECT 1+1 as result"}, "[{\"result\":2}]")
		tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": "SELECT 'hello' as greeting"}, "[{\"greeting\":\"hello\"}]")
	})

	t.Run("mindsdb_sql_tests", func(t *testing.T) {
		tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": "SELECT 1"}, mindsDBSelect1Want)
		tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": "SHOW DATABASES"}, "")
		tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": "SHOW TABLES"}, "")
		tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": "SELECT TABLE_NAME FROM information_schema.TABLES LIMIT 1"}, "")
		tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": "SELECT 1+1 as result"}, "")
		tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": "SELECT UPPER('hello') as result"}, "")
		tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": "SELECT NOW() as current_time"}, "")
	})

	// Test CREATE DATABASE (MindsDB's federated database capability)
	t.Run("mindsdb_create_database", func(t *testing.T) {
		tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": "DROP DATABASE IF EXISTS test_postgres_db"}, "")

		// Create external database integration using MindsDB's demo database
		createDBSQL := `CREATE DATABASE test_postgres_db WITH ENGINE = 'postgres', PARAMETERS = {'user': 'demo_user', 'password': 'demo_password', 'host': 'samples.mindsdb.com', 'port': '5432', 'database': 'demo', 'schema': 'demo_data'}`
		tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": createDBSQL}, "")

		tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": "SHOW DATABASES"}, "")

		tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": "SHOW TABLES FROM test_postgres_db"}, "")

		tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": "DROP DATABASE IF EXISTS test_postgres_db"}, "")
	})

	// Test MindsDB integration capabilities with product/review data
	t.Run("mindsdb_integration_demo", func(t *testing.T) {
		tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": "DROP TABLE IF EXISTS files.test_products"}, "")
		tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": "DROP TABLE IF EXISTS files.test_reviews"}, "")

		// Create test tables with sample data
		createProductsSQL := `CREATE TABLE files.test_products (SELECT 'PROD001' as product_id, 'Laptop Computer' as product_name, 'Electronics' as category UNION ALL SELECT 'PROD002', 'Office Chair', 'Furniture' UNION ALL SELECT 'PROD003', 'Coffee Maker', 'Appliances' UNION ALL SELECT 'PROD004', 'Desk Lamp', 'Furniture')`
		tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": createProductsSQL}, "")

		createReviewsSQL := `CREATE TABLE files.test_reviews (SELECT 'PROD001' as product_id, 'Great laptop, very fast!' as review, 5 as rating UNION ALL SELECT 'PROD001', 'Good value for money', 4 UNION ALL SELECT 'PROD002', 'Very comfortable chair', 5 UNION ALL SELECT 'PROD002', 'Nice design but expensive', 3 UNION ALL SELECT 'PROD003', 'Makes excellent coffee', 5 UNION ALL SELECT 'PROD004', 'Bright light, perfect for reading', 4)`
		tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": createReviewsSQL}, "")

		t.Run("query_created_tables", func(t *testing.T) {
			tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": "SELECT * FROM files.test_products ORDER BY product_id"}, "")
			tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": "SELECT * FROM files.test_reviews ORDER BY product_id, rating DESC"}, "")
			tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": "SELECT category, COUNT(*) as product_count FROM files.test_products GROUP BY category ORDER BY category"}, "")
			tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": "SELECT product_id, AVG(rating) as avg_rating FROM files.test_reviews GROUP BY product_id ORDER BY avg_rating DESC"}, "")
		})

		t.Run("cross_database_join", func(t *testing.T) {
			joinSQL := `SELECT p.product_name, p.category, r.review, r.rating FROM files.test_products p JOIN files.test_reviews r ON p.product_id = r.product_id WHERE r.rating >= 4 ORDER BY p.product_name, r.rating DESC`
			tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": joinSQL}, "")

			aggSQL := `SELECT p.category, COUNT(DISTINCT p.product_id) as product_count, COUNT(r.review) as review_count, AVG(r.rating) as avg_rating FROM files.test_products p LEFT JOIN files.test_reviews r ON p.product_id = r.product_id GROUP BY p.category ORDER BY avg_rating DESC`
			tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": aggSQL}, "")
		})

		t.Run("advanced_sql_features", func(t *testing.T) {
			subquerySQL := `SELECT p.product_name, p.category, AVG(r.rating) as avg_rating FROM files.test_products p JOIN files.test_reviews r ON p.product_id = r.product_id GROUP BY p.product_id, p.product_name, p.category HAVING AVG(r.rating) >= 4 ORDER BY avg_rating DESC`
			tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": subquerySQL}, "")

			caseSQL := `SELECT product_id, review, rating, CASE WHEN rating >= 5 THEN 'Excellent' WHEN rating >= 4 THEN 'Good' WHEN rating >= 3 THEN 'Average' ELSE 'Poor' END as rating_category FROM files.test_reviews ORDER BY rating DESC, product_id`
			tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": caseSQL}, "")
		})

		tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": "DROP TABLE IF EXISTS files.test_product_summary"}, "")

		t.Run("data_manipulation", func(t *testing.T) {
			summarySQL := `CREATE TABLE files.test_product_summary (SELECT p.product_id, p.product_name, p.category, COUNT(r.review) as total_reviews, AVG(r.rating) as avg_rating, MAX(r.rating) as max_rating, MIN(r.rating) as min_rating FROM files.test_products p LEFT JOIN files.test_reviews r ON p.product_id = r.product_id GROUP BY p.product_id, p.product_name, p.category)`
			tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": summarySQL}, "")

			tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": "SELECT * FROM files.test_product_summary ORDER BY avg_rating DESC"}, "")
		})

		tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": "DROP TABLE IF EXISTS files.test_products"}, "")
		tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": "DROP TABLE IF EXISTS files.test_reviews"}, "")
		tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": "DROP TABLE IF EXISTS files.test_product_summary"}, "")
	})

	// Test database integration and cross-database joins
	t.Run("mindsdb_create_database_integration", func(t *testing.T) {
		showDBSQL := `SHOW DATABASES`
		tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": showDBSQL}, "")

		tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": "SHOW TABLES FROM files"}, "")

		tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": "DROP TABLE IF EXISTS files.test_integration_data"}, "")
		tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": "DROP TABLE IF EXISTS files.test_local_data"}, "")

		createIntegrationTableSQL := `CREATE TABLE files.test_integration_data (SELECT 1 as id, 'Data from integration' as description, CURDATE() as created_at UNION ALL SELECT 2, 'Another record', CURDATE() UNION ALL SELECT 3, 'Third record', CURDATE())`
		tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": createIntegrationTableSQL}, "")

		tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": "SELECT * FROM files.test_integration_data ORDER BY id"}, "")

		createLocalTableSQL := `CREATE TABLE files.test_local_data (SELECT 1 as id, 'Local metadata' as metadata UNION ALL SELECT 2, 'More metadata')`
		tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": createLocalTableSQL}, "")

		crossJoinSQL := `SELECT i.id, i.description, l.metadata FROM files.test_integration_data i LEFT JOIN files.test_local_data l ON i.id = l.id ORDER BY i.id`
		tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": crossJoinSQL}, "")

		tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": "DROP TABLE IF EXISTS files.test_integration_data"}, "")
		tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": "DROP TABLE IF EXISTS files.test_local_data"}, "")
	})

	// Test data transformation with customer order data
	t.Run("mindsdb_data_transformation", func(t *testing.T) {
		tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": "DROP TABLE IF EXISTS files.test_orders"}, "")
		tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": "DROP TABLE IF EXISTS files.test_customer_summary"}, "")

		createOrdersSQL := `CREATE TABLE files.test_orders (SELECT 1 as order_id, 'CUST001' as customer_id, 100.50 as amount, '2024-01-15' as order_date UNION ALL SELECT 2, 'CUST001', 250.00, '2024-02-20' UNION ALL SELECT 3, 'CUST002', 75.25, '2024-01-18' UNION ALL SELECT 4, 'CUST003', 500.00, '2024-03-10' UNION ALL SELECT 5, 'CUST002', 150.00, '2024-02-25')`
		tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": createOrdersSQL}, "")

		customerSummarySQL := `CREATE TABLE files.test_customer_summary (SELECT customer_id, COUNT(*) as total_orders, SUM(amount) as total_spent, AVG(amount) as avg_order_value, MIN(order_date) as first_order_date, MAX(order_date) as last_order_date FROM files.test_orders GROUP BY customer_id)`
		tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": customerSummarySQL}, "")

		tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": "SELECT * FROM files.test_customer_summary ORDER BY total_spent DESC"}, "")

		segmentSQL := `SELECT customer_id, total_spent, CASE WHEN total_spent >= 300 THEN 'High Value' WHEN total_spent >= 150 THEN 'Medium Value' ELSE 'Low Value' END as customer_segment FROM files.test_customer_summary ORDER BY total_spent DESC`
		tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": segmentSQL}, "")

		tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": "DROP TABLE IF EXISTS files.test_orders"}, "")
		tr.expectResult(t, ctx, "my-exec-sql-tool", map[string]any{"sql": "DROP TABLE IF EXISTS files.test_customer_summary"}, "")
	})

	// Test error handling - these are expected to fail but exercise error paths
	t.Run("mindsdb_error_handling", func(t *testing.T) {
		// Test invalid SQL - expect this to fail with 400
		res := tr.invoke(t, ctx, "my-exec-sql-tool", map[string]any{"sql": "INVALID SQL QUERY"}, nil)
		if res.status != http.StatusBadRequest {
			t.Logf("Expected 400 for invalid SQL, got %d (tool error: %v) (this exercises error handling)", res.status, res.toolErr)
		}

		// Test empty SQL - expect this to fail with 400
		res = tr.invoke(t, ctx, "my-exec-sql-tool", map[string]any{"sql": ""}, nil)
		if res.status != http.StatusBadRequest {
			t.Logf("Expected 400 for empty SQL, got %d (tool error: %v) (this exercises error handling)", res.status, res.toolErr)
		}
	})

	// Test authentication - these are expected to fail but exercise auth code paths
	t.Run("mindsdb_auth_tests", func(t *testing.T) {
		// Test auth-required tool without auth - expect this to fail with 401
		res := tr.invoke(t, ctx, "my-auth-exec-sql-tool", map[string]any{"sql": "SELECT 1"}, nil)
		if res.status != http.StatusUnauthorized {
			t.Logf("Expected 401 for missing auth, got %d (this exercises auth handling)", res.status)
		}
	})
}
