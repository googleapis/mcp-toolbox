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
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	gocql "github.com/apache/cassandra-gocql-driver/v2"
	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// If CASSANDRA_HOST is set, use that cluster; otherwise start an ephemeral container.
var (
	CassandraSourceType = "cassandra"
	CassandraToolType   = "cassandra-cql"
	Hosts               = os.Getenv("CASSANDRA_HOST")
	// Per-run keyspace (dropped on cleanup) so runs never share or leak one.
	Keyspace = "toolbox_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	Username = os.Getenv("CASSANDRA_USER")
	Password = os.Getenv("CASSANDRA_PASS")
)

const (
	cassandraImage         = "cassandra:5.0.9"
	cassandraContainerUser = "toolbox"
	// Enable password auth (default is AllowAllAuthenticator); grep aborts startup if the sed stops matching.
	cassandraStartCmd = `sed -i 's/^authenticator: AllowAllAuthenticator$/authenticator: PasswordAuthenticator/' /etc/cassandra/cassandra.yaml && ` +
		`grep -q '^authenticator: PasswordAuthenticator$' /etc/cassandra/cassandra.yaml && ` +
		`exec docker-entrypoint.sh cassandra -f`
)

// setupCassandraContainer starts an ephemeral Cassandra container and returns its connection details.
func setupCassandraContainer(ctx context.Context, t *testing.T) (hosts, user, pass string) {
	t.Helper()

	req := testcontainers.ContainerRequest{
		Image:        cassandraImage,
		Entrypoint:   []string{"sh", "-c"},
		Cmd:          []string{cassandraStartCmd},
		ExposedPorts: []string{"9042/tcp"},
		// Keep the JVM small enough for CI workers.
		Env: map[string]string{"MAX_HEAP_SIZE": "512M", "HEAP_NEWSIZE": "128M"},
		// Cassandra can take over a minute to start, past each strategy's 60s default.
		WaitingFor: wait.ForAll(
			wait.ForListeningPort("9042/tcp").WithStartupTimeout(4*time.Minute),
			wait.ForLog("Created default superuser role 'cassandra'").WithStartupTimeout(4*time.Minute),
		).WithDeadline(4 * time.Minute),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	// Register before checking err: a container can be returned alongside an error.
	testcontainers.CleanupContainer(t, container)
	if err != nil {
		t.Fatalf("failed to start Cassandra container: %s", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get Cassandra container host: %s", err)
	}
	mappedPort, err := container.MappedPort(ctx, "9042/tcp")
	if err != nil {
		t.Fatalf("failed to get Cassandra container mapped port: %s", err)
	}
	hosts = fmt.Sprintf("%s:%s", host, mappedPort.Port())

	// Log in as the default superuser, create the test role, then rotate the superuser's password.
	cluster := gocql.NewCluster(hosts)
	cluster.ProtoVersion = 4
	cluster.DisableInitialHostLookup = true
	cluster.ConnectTimeout = 10 * time.Second
	cluster.Authenticator = gocql.PasswordAuthenticator{Username: "cassandra", Password: "cassandra"}
	admin, err := cluster.CreateSession()
	if err != nil {
		t.Fatalf("unable to connect to Cassandra container as superuser: %s", err)
	}
	defer admin.Close()

	user = cassandraContainerUser
	pass = strings.ReplaceAll(uuid.New().String(), "-", "")
	superPass := strings.ReplaceAll(uuid.New().String(), "-", "")
	// Values are constants or hex UUIDs, so Sprintf is safe here.
	bootstrapStmts := []string{
		fmt.Sprintf("CREATE ROLE %s WITH PASSWORD = '%s' AND LOGIN = true", user, pass),
		fmt.Sprintf("ALTER ROLE cassandra WITH PASSWORD = '%s'", superPass),
	}
	for _, stmt := range bootstrapStmts {
		if err := admin.Query(stmt).Exec(); err != nil {
			t.Fatalf("failed to bootstrap Cassandra container: %s", err)
		}
	}

	return hosts, user, pass
}

// useCassandra points the connection globals at an ephemeral container unless CASSANDRA_HOST is set.
func useCassandra(ctx context.Context, t *testing.T) {
	t.Helper()
	if Hosts != "" {
		return
	}
	t.Logf("CASSANDRA_HOST not set; starting ephemeral %s container", cassandraImage)
	hosts, user, pass := setupCassandraContainer(ctx, t)
	// Restore globals so repeated runs (-count>1) start a fresh container.
	origHosts, origUser, origPass := Hosts, Username, Password
	t.Cleanup(func() { Hosts, Username, Password = origHosts, origUser, origPass })
	Hosts, Username, Password = hosts, user, pass
}

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

func dropTable(t *testing.T, session *gocql.Session, tableName string) {
	t.Helper()
	err := session.Query(fmt.Sprintf("DROP TABLE IF EXISTS %s.%s", Keyspace, tableName)).Exec()
	if err != nil {
		t.Errorf("failed to drop table %s: %v", tableName, err)
	}
}

// setupTable registers the drop before creating the table, so a failed insert can't leak it.
func setupTable(t *testing.T, session *gocql.Session, tableName string) {
	t.Helper()
	t.Cleanup(func() { dropTable(t, session, tableName) })
	if err := initTable(tableName, session); err != nil {
		t.Fatal(err)
	}
}

func dropKeyspace(t *testing.T, session *gocql.Session) {
	t.Helper()
	if err := session.Query(fmt.Sprintf("DROP KEYSPACE IF EXISTS %s", Keyspace)).Exec(); err != nil {
		t.Errorf("failed to drop keyspace %s: %v", Keyspace, err)
	}
}

func TestCassandra(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	useCassandra(ctx, t)

	session, err := initCassandraSession()
	if err != nil {
		t.Fatal(err)
	}
	// t.Cleanup, not defer: the table drops below run as cleanups and need the session.
	t.Cleanup(session.Close)
	// Runs after the table drops and before the session closes (cleanups are LIFO).
	t.Cleanup(func() { dropKeyspace(t, session) })
	sourceConfig := getCassandraVars(t)

	args := []string{"--enable-api"}
	paramTableName := "param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameAuth := "auth_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameTemplateParam := "template_param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	setupTable(t, session, paramTableName)
	setupTable(t, session, tableNameAuth)
	setupTable(t, session, tableNameTemplateParam)

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
