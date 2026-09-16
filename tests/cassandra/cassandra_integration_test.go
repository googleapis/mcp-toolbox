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

package cassandra

import (
	"context"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	gocql "github.com/apache/cassandra-gocql-driver/v2"
	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

var (
	CassandraSourceType = "cassandra"
	CassandraToolType   = "cassandra-cql"
	Hosts               = os.Getenv("CASSANDRA_HOST")
	Keyspace            = "example_keyspace"
	Username            = os.Getenv("CASSANDRA_USER")
	Password            = os.Getenv("CASSANDRA_PASS")
)

func getCassandraVars(t *testing.T) map[string]any {
	switch "" {
	case Hosts:
		t.Fatal("'Hosts' not set")
	case Username:
		t.Fatal("'Username' not set")
	case Password:
		t.Fatal("'Password' not set")
	}
	return map[string]any{
		"type":     CassandraSourceType,
		"hosts":    strings.Split(Hosts, ","),
		"keyspace": Keyspace,
		"username": Username,
		"password": Password,
	}
}

func initCassandraSession() (*gocql.Session, error) {
	hostStrings := strings.Split(Hosts, ",")

	var hosts []string
	for _, h := range hostStrings {
		trimmedHost := strings.TrimSpace(h)
		if trimmedHost != "" {
			hosts = append(hosts, trimmedHost)
		}
	}
	if len(hosts) == 0 {
		return nil, fmt.Errorf("no valid hosts found in CASSANDRA_HOSTS env var")
	}
	// Configure cluster connection
	cluster := gocql.NewCluster(hosts...)
	cluster.Consistency = gocql.Quorum
	cluster.ProtoVersion = 4
	cluster.DisableInitialHostLookup = true
	cluster.ConnectTimeout = 10 * time.Second
	cluster.NumConns = 2
	cluster.Authenticator = gocql.PasswordAuthenticator{
		Username: Username,
		Password: Password,
	}
	cluster.RetryPolicy = &gocql.ExponentialBackoffRetryPolicy{
		NumRetries: 3,
		Min:        200 * time.Millisecond,
		Max:        2 * time.Second,
	}

	// Create session
	session, err := cluster.CreateSession()
	if err != nil {
		return nil, fmt.Errorf("Failed to create session: %v", err)
	}

	// Create keyspace
	err = session.Query(fmt.Sprintf(`
		CREATE KEYSPACE IF NOT EXISTS %s
		WITH replication = {'class': 'SimpleStrategy', 'replication_factor': 1}
	`, Keyspace)).Exec()
	if err != nil {
		return nil, fmt.Errorf("Failed to create keyspace: %v", err)
	}

	return session, nil
}

func initTable(tableName string, session *gocql.Session) error {

	// Create table with additional columns
	err := session.Query(fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s.%s (
			id int PRIMARY KEY,
			name text,
			email text,
			age int,
			is_active boolean,
			created_at timestamp
		)
	`, Keyspace, tableName)).Exec()
	if err != nil {
		return fmt.Errorf("Failed to create table: %v", err)
	}

	// Use fixed timestamps for reproducibility
	fixedTime, _ := time.Parse(time.RFC3339, "2025-07-25T12:00:00Z")
	dayAgo := fixedTime.Add(-24 * time.Hour)
	twelveHoursAgo := fixedTime.Add(-12 * time.Hour)

	// Insert minimal diverse data with fixed time.Time for timestamps
	err = session.Query(fmt.Sprintf(`
		INSERT INTO %s.%s (id, name,email, age, is_active, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`, Keyspace, tableName),
		3, "Alice", tests.ServiceAccountEmail, 25, true, dayAgo,
	).Exec()
	if err != nil {
		return fmt.Errorf("Failed to insert user: %v", err)
	}
	err = session.Query(fmt.Sprintf(`
		INSERT INTO %s.%s (id, name,email, age, is_active, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`, Keyspace, tableName),
		2, "Alex", "janedoe@gmail.com", 30, false, twelveHoursAgo,
	).Exec()
	if err != nil {
		return fmt.Errorf("Failed to insert user: %v", err)
	}
	err = session.Query(fmt.Sprintf(`
		INSERT INTO %s.%s (id, name,email, age, is_active, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`, Keyspace, tableName),
		1, "Sid", "sid@gmail.com", 10, true, fixedTime,
	).Exec()
	if err != nil {
		return fmt.Errorf("Failed to insert user: %v", err)
	}
	err = session.Query(fmt.Sprintf(`
		INSERT INTO %s.%s (id, name,email, age, is_active, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`, Keyspace, tableName),
		4, nil, "a@gmail.com", 40, false, fixedTime,
	).Exec()
	if err != nil {
		return fmt.Errorf("Failed to insert user: %v", err)
	}
	return nil
}

func dropTable(session *gocql.Session, tableName string) {
	err := session.Query(fmt.Sprintf("drop table %s.%s", Keyspace, tableName)).Exec()
	if err != nil {
		log.Printf("Failed to drop table %s: %v", tableName, err)
	}
}

func TestCassandra(t *testing.T) {
	session, err := initCassandraSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	sourceConfig := getCassandraVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	args := []string{"--enable-api"}
	paramTableName := "param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameAuth := "auth_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameTemplateParam := "template_param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	err = initTable(paramTableName, session)
	if err != nil {
		t.Fatal(err)
	}
	defer dropTable(session, paramTableName)

	err = initTable(tableNameAuth, session)
	if err != nil {
		t.Fatal(err)
	}
	defer dropTable(session, tableNameAuth)

	err = initTable(tableNameTemplateParam, session)
	if err != nil {
		t.Fatal(err)
	}
	defer dropTable(session, tableNameTemplateParam)

	paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt := createParamToolInfo(paramTableName)
	_, _, authToolStmt := getCassandraAuthToolInfo(tableNameAuth)
	toolsFile := tests.GetToolsConfig(sourceConfig, CassandraToolType, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, authToolStmt)

	tmplSelectCombined, tmplSelectFilterCombined := getCassandraTmplToolInfo()
	tmpSelectAll := "SELECT * FROM {{.tableName}} where id = 1"

	toolsFile = tests.AddTemplateParamConfig(t, toolsFile, CassandraToolType, tmplSelectCombined, tmplSelectFilterCombined, tmpSelectAll)

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()

	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}
	selectIdNameWant, selectIdNullWant, selectArrayParamWant, mcpMyFailToolWant, mcpSelect1Want, mcpMyToolIdWant := getCassandraWants()
	selectAllWant, selectIdWant, selectNameWant := getCassandraTmplWants()

	tests.RunToolGetTest(t)
	tests.RunToolInvokeTest(t, "", tests.DisableSelect1Test(),
		tests.DisableOptionalNullParamTest(),
		tests.WithMyToolId3NameAliceWant(selectIdNameWant),
		tests.WithMyToolById4Want(selectIdNullWant),
		tests.WithMyArrayToolWant(selectArrayParamWant),
		tests.DisableSelect1AuthTest())
	tests.RunToolInvokeWithTemplateParameters(t, tableNameTemplateParam,
		tests.DisableSelectFilterTest(),
		tests.WithSelectAllWant(selectAllWant),
		tests.DisableDdlTest(), tests.DisableInsertTest(), tests.WithTmplSelectId1Want(selectIdWant), tests.WithTmplSelectNameWant(selectNameWant))

	tests.RunMCPToolCallMethod(t, mcpMyFailToolWant, mcpSelect1Want,
		tests.WithMcpMyToolId3NameAliceWant(mcpMyToolIdWant),
		tests.WithMcpMySecureToolWant(selectIdNameWant),
		tests.DisableMcpSelect1AuthTest())

}

func createParamToolInfo(tableName string) (string, string, string, string) {
	toolStatement := fmt.Sprintf("SELECT id, name FROM %s WHERE id = ? AND name = ? ALLOW FILTERING;", tableName)
	idParamStatement := fmt.Sprintf("SELECT id,name FROM %s WHERE id = ?;", tableName)
	nameParamStatement := fmt.Sprintf("SELECT id, name FROM %s WHERE name = ? ALLOW FILTERING;", tableName)
	arrayToolStatement := fmt.Sprintf("SELECT id, name FROM %s WHERE id IN ? AND name IN ? ALLOW FILTERING;", tableName)
	return toolStatement, idParamStatement, nameParamStatement, arrayToolStatement

}

func getCassandraAuthToolInfo(tableName string) (string, string, string) {
	createStatement := fmt.Sprintf("CREATE TABLE %s (id UUID PRIMARY KEY, name TEXT, email TEXT);", tableName)
	insertStatement := fmt.Sprintf("INSERT INTO %s (id, name, email) VALUES (uuid(), ?, ?), (uuid(), ?, ?);", tableName)
	toolStatement := fmt.Sprintf("SELECT name FROM %s WHERE email = ? ALLOW FILTERING;", tableName)
	return createStatement, insertStatement, toolStatement
}

func getCassandraTmplToolInfo() (string, string) {
	selectAllTemplateStmt := "SELECT age, id, name FROM {{.tableName}} where id = ?;"
	selectByIdTemplateStmt := "SELECT id, name FROM {{.tableName}} WHERE name = ? ALLOW FILTERING;"
	return selectAllTemplateStmt, selectByIdTemplateStmt
}

func getCassandraWants() (string, string, string, string, string, string) {
	selectIdNameWant := "[{\"id\":3,\"name\":\"Alice\"}]"
	selectIdNullWant := "[{\"id\":4,\"name\":\"\"}]"
	selectArrayParamWant := "[{\"id\":1,\"name\":\"Sid\"},{\"id\":3,\"name\":\"Alice\"}]"
	mcpMyFailToolWant := "{\"jsonrpc\":\"2.0\",\"id\":\"invoke-fail-tool\",\"result\":{\"content\":[{\"type\":\"text\",\"text\":\"error processing request: unable to parse rows: line 1:0 no viable alternative at input 'SELEC' ([SELEC]...)\"}],\"isError\":true}}"
	mcpMyToolIdWant := "{\"jsonrpc\":\"2.0\",\"id\":\"my-tool\",\"result\":{\"content\":[{\"type\":\"text\",\"text\":\"[{\\\"id\\\":3,\\\"name\\\":\\\"Alice\\\"}]\"}]}}"
	return selectIdNameWant, selectIdNullWant, selectArrayParamWant, mcpMyFailToolWant, "nil", mcpMyToolIdWant
}

func getCassandraTmplWants() (string, string, string) {
	selectAllWant := "[{\"age\":10,\"created_at\":\"2025-07-25T12:00:00Z\",\"email\":\"sid@gmail.com\",\"id\":1,\"is_active\":true,\"name\":\"Sid\"}]"
	selectIdWant := "[{\"age\":10,\"id\":1,\"name\":\"Sid\"}]"
	selectNameWant := "[{\"id\":2,\"name\":\"Alex\"}]"
	return selectAllWant, selectIdWant, selectNameWant
}
