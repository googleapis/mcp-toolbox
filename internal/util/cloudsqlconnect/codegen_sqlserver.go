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

package cloudsqlconnect

import "fmt"

// generateSQLServerPython returns a Python code snippet for connecting to
// Cloud SQL for SQL Server. SQL Server's recommended path is the Cloud SQL
// Auth Proxy (Connector libraries only fully support Postgres and MySQL).
func generateSQLServerPython(method ConnectionMethod, connectionName, dbName string, port int, privateIP string) *CodeSnippet {
	snippet := &CodeSnippet{Language: LangPython}

	switch method {
	case MethodAuthProxy:
		snippet.Dependencies = []string{"pyodbc>=5.0", "sqlalchemy>=2.0"}
		snippet.Code = fmt.Sprintf(`import os
import sqlalchemy

def connect_with_auth_proxy():
    """Connect via Cloud SQL Auth Proxy (must be running on localhost:%d)."""
    user = os.environ["DB_USER"]
    password = os.environ["DB_PASS"]
    db_name = os.environ.get("DB_NAME", "%s")
    host = os.environ.get("DB_HOST", "127.0.0.1")

    url = sqlalchemy.engine.URL.create(
        drivername="mssql+pyodbc",
        username=user,
        password=password,
        host=host,
        port=%d,
        database=db_name,
        query={"driver": "ODBC Driver 18 for SQL Server", "TrustServerCertificate": "yes"},
    )
    return sqlalchemy.create_engine(url)

# Start Auth Proxy: ./cloud-sql-proxy %s

if __name__ == "__main__":
    engine = connect_with_auth_proxy()
    with engine.connect() as conn:
        result = conn.execute(sqlalchemy.text("SELECT 1"))
        print(result.fetchone())
`, port, dbName, port, connectionName)
	case MethodDirectPrivateIP:
		snippet.Dependencies = []string{"pyodbc>=5.0", "sqlalchemy>=2.0"}
		snippet.Code = fmt.Sprintf(`import os
import sqlalchemy

def connect_with_private_ip():
    """Connect directly using Cloud SQL private IP (same VPC required)."""
    user = os.environ["DB_USER"]
    password = os.environ["DB_PASS"]
    db_name = os.environ.get("DB_NAME", "%s")

    url = sqlalchemy.engine.URL.create(
        drivername="mssql+pyodbc",
        username=user,
        password=password,
        host="%s",
        port=%d,
        database=db_name,
        query={"driver": "ODBC Driver 18 for SQL Server", "TrustServerCertificate": "yes"},
    )
    return sqlalchemy.create_engine(url)

if __name__ == "__main__":
    engine = connect_with_private_ip()
    with engine.connect() as conn:
        result = conn.execute(sqlalchemy.text("SELECT 1"))
        print(result.fetchone())
`, dbName, privateIP, port)
	default:
		snippet.Code = "# For Cloud SQL for SQL Server, use the Cloud SQL Auth Proxy (recommended)."
		snippet.Notes = []string{"Cloud SQL Auth Proxy is the recommended connection method for SQL Server."}
	}
	return snippet
}

// generateSQLServerNodeJS returns a Node.js snippet for SQL Server.
func generateSQLServerNodeJS(method ConnectionMethod, connectionName, dbName string, port int, privateIP string) *CodeSnippet {
	snippet := &CodeSnippet{Language: LangNodeJS}

	switch method {
	case MethodAuthProxy:
		snippet.Dependencies = []string{"mssql@^11.0"}
		snippet.Code = fmt.Sprintf(`const sql = require('mssql');

async function connectWithAuthProxy() {
    // Auth Proxy must be running: ./cloud-sql-proxy %s
    const pool = await sql.connect({
        user: process.env.DB_USER,
        password: process.env.DB_PASS,
        database: process.env.DB_NAME || '%s',
        server: process.env.DB_HOST || '127.0.0.1',
        port: %d,
        options: { encrypt: true, trustServerCertificate: true },
        pool: { max: 5 },
    });
    return pool;
}

(async () => {
    const pool = await connectWithAuthProxy();
    const result = await pool.request().query('SELECT 1 AS value');
    console.log(result.recordset);
    await pool.close();
})();
`, connectionName, dbName, port)
	case MethodDirectPrivateIP:
		snippet.Dependencies = []string{"mssql@^11.0"}
		snippet.Code = fmt.Sprintf(`const sql = require('mssql');

async function connectWithPrivateIP() {
    const pool = await sql.connect({
        user: process.env.DB_USER,
        password: process.env.DB_PASS,
        database: process.env.DB_NAME || '%s',
        server: '%s',
        port: %d,
        options: { encrypt: true, trustServerCertificate: true },
        pool: { max: 5 },
    });
    return pool;
}

(async () => {
    const pool = await connectWithPrivateIP();
    const result = await pool.request().query('SELECT 1 AS value');
    console.log(result.recordset);
    await pool.close();
})();
`, dbName, privateIP, port)
	default:
		snippet.Code = "// For Cloud SQL for SQL Server, use the Cloud SQL Auth Proxy (recommended)."
		snippet.Notes = []string{"Cloud SQL Auth Proxy is the recommended connection method for SQL Server."}
	}
	return snippet
}

// generateSQLServerJava returns a Java/JDBC snippet for SQL Server.
func generateSQLServerJava(method ConnectionMethod, connectionName, dbName string, port int, privateIP string) *CodeSnippet {
	snippet := &CodeSnippet{
		Language: LangJava,
		Dependencies: []string{
			"com.microsoft.sqlserver:mssql-jdbc:12.6.1.jre11",
			"com.zaxxer:HikariCP:5.0.1",
		},
	}

	host := "127.0.0.1"
	hostNote := fmt.Sprintf("// Auth Proxy must be running: ./cloud-sql-proxy %s", connectionName)
	if method == MethodDirectPrivateIP {
		host = privateIP
		hostNote = "// Requires same VPC as Cloud SQL"
	}

	snippet.Code = fmt.Sprintf(`import com.zaxxer.hikari.HikariConfig;
import com.zaxxer.hikari.HikariDataSource;
import javax.sql.DataSource;

public class CloudSQLConnector {
    public static DataSource createConnectionPool() {
        %s
        HikariConfig config = new HikariConfig();
        config.setJdbcUrl("jdbc:sqlserver://%s:%d;databaseName=%s;encrypt=true;trustServerCertificate=true");
        config.setUsername(System.getenv("DB_USER"));
        config.setPassword(System.getenv("DB_PASS"));
        config.setMaximumPoolSize(5);
        return new HikariDataSource(config);
    }

    public static void main(String[] args) throws Exception {
        DataSource pool = createConnectionPool();
        try (var conn = pool.getConnection();
             var stmt = conn.createStatement();
             var rs = stmt.executeQuery("SELECT 1")) {
            if (rs.next()) {
                System.out.println("Connected: " + rs.getInt(1));
            }
        }
    }
}
`, hostNote, host, port, dbName)

	return snippet
}

// generateSQLServerGo returns a Go snippet for SQL Server using
// github.com/microsoft/go-mssqldb.
func generateSQLServerGo(method ConnectionMethod, connectionName, dbName string, port int, privateIP string) *CodeSnippet {
	snippet := &CodeSnippet{
		Language:     LangGo,
		Dependencies: []string{"github.com/microsoft/go-mssqldb@v1.10.0"},
	}

	host := "127.0.0.1"
	hostNote := fmt.Sprintf("// Auth Proxy must be running: ./cloud-sql-proxy %s", connectionName)
	if method == MethodDirectPrivateIP {
		host = privateIP
		hostNote = "// Requires same VPC as Cloud SQL"
	}

	snippet.Code = fmt.Sprintf(`package main

import (
    "database/sql"
    "fmt"
    "net/url"
    "os"

    _ "github.com/microsoft/go-mssqldb"
)

func connect() (*sql.DB, error) {
    %s
    query := url.Values{}
    query.Add("database", getEnv("DB_NAME", "%s"))
    query.Add("encrypt", "true")
    query.Add("TrustServerCertificate", "true")

    u := &url.URL{
        Scheme:   "sqlserver",
        User:     url.UserPassword(os.Getenv("DB_USER"), os.Getenv("DB_PASS")),
        Host:     "%s:%d",
        RawQuery: query.Encode(),
    }
    db, err := sql.Open("sqlserver", u.String())
    if err != nil {
        return nil, err
    }
    db.SetMaxOpenConns(5)
    return db, nil
}

func getEnv(key, fallback string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return fallback
}

func main() {
    db, err := connect()
    if err != nil {
        panic(err)
    }
    defer db.Close()

    var result int
    if err := db.QueryRow("SELECT 1").Scan(&result); err != nil {
        panic(err)
    }
    fmt.Println("Connected:", result)
}
`, hostNote, dbName, host, port)

	return snippet
}
