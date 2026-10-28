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

import (
	"fmt"
	"strings"
)

// appendDependencyNotes decorates snippet.Notes with a human-readable
// install command derived from Dependencies, plus a one-line reminder
// that the pinned versions are the floor tested against Cloud SQL. It
// preserves any Notes the per-language generator already set.
func appendDependencyNotes(snippet *CodeSnippet) *CodeSnippet {
	if snippet == nil || len(snippet.Dependencies) == 0 {
		return snippet
	}
	var installCmd string
	switch snippet.Language {
	case LangPython:
		installCmd = "pip install " + strings.Join(snippet.Dependencies, " ")
	case LangNodeJS:
		installCmd = "npm install " + strings.Join(snippet.Dependencies, " ")
	case LangGo:
		// Go modules install one at a time; join with a newline in the note.
		lines := make([]string, 0, len(snippet.Dependencies))
		for _, d := range snippet.Dependencies {
			lines = append(lines, "go get "+d)
		}
		installCmd = strings.Join(lines, "\n")
	case LangJava:
		installCmd = "See pom.xml or build.gradle: " + strings.Join(snippet.Dependencies, ", ")
	default:
		return snippet
	}
	extra := []string{
		"Install with: " + installCmd,
		"Versions above are the floor tested against Cloud SQL; newer minor releases should work. Pin exact versions in production.",
	}
	snippet.Notes = append(snippet.Notes, extra...)
	return snippet
}

// GenerateCodeSnippet generates a code snippet for the given configuration.
// SQL Server is routed to dedicated generators because the Cloud SQL
// Connector libraries fully support only Postgres and MySQL.
//
// Every returned snippet carries a Dependencies list floor-pinned to the
// minimum tested version of each library (pip `>=`, npm `^`, Maven exact,
// Go module `@version`), plus an install-command note derived from that
// list, so the caller can install a known-working environment without
// hitting the "which version?" question.
func GenerateCodeSnippet(lang Language, method ConnectionMethod, dbType DatabaseType, connectionName, dbName string, port int, privateIP string) *CodeSnippet {
	var snippet *CodeSnippet
	if dbType == SQLServer {
		switch lang {
		case LangPython:
			snippet = generateSQLServerPython(method, connectionName, dbName, port, privateIP)
		case LangNodeJS:
			snippet = generateSQLServerNodeJS(method, connectionName, dbName, port, privateIP)
		case LangJava:
			snippet = generateSQLServerJava(method, connectionName, dbName, port, privateIP)
		case LangGo:
			snippet = generateSQLServerGo(method, connectionName, dbName, port, privateIP)
		default:
			return &CodeSnippet{
				Language: lang,
				Code:     "// Unsupported language",
				Notes:    []string{"Supported languages: python, nodejs, java, go"},
			}
		}
	} else {
		switch lang {
		case LangPython:
			snippet = generatePythonCode(method, dbType, connectionName, dbName, port, privateIP)
		case LangNodeJS:
			snippet = generateNodeJSCode(method, dbType, connectionName, dbName, port, privateIP)
		case LangJava:
			snippet = generateJavaCode(method, dbType, connectionName, dbName, port, privateIP)
		case LangGo:
			snippet = generateGoCode(method, dbType, connectionName, dbName, port, privateIP)
		default:
			return &CodeSnippet{
				Language: lang,
				Code:     "// Unsupported language",
				Notes:    []string{"Supported languages: python, nodejs, java, go"},
			}
		}
	}
	return appendDependencyNotes(snippet)
}

func generatePythonCode(method ConnectionMethod, dbType DatabaseType, connectionName, dbName string, port int, privateIP string) *CodeSnippet {
	snippet := &CodeSnippet{Language: LangPython}

	switch method {
	case MethodConnector:
		if dbType == PostgreSQL {
			snippet.Dependencies = []string{"cloud-sql-python-connector[pg8000]>=1.13", "sqlalchemy>=2.0"}
			snippet.Code = fmt.Sprintf(`import os
from google.cloud.sql.connector import Connector
import sqlalchemy

def connect_with_connector():
    """Create connection using Cloud SQL Python Connector."""
    connector = Connector()

    def getconn():
        return connector.connect(
            "%s",  # Instance connection name
            "pg8000",
            user=os.environ["DB_USER"],
            password=os.environ["DB_PASS"],
            db=os.environ.get("DB_NAME", "%s"),
        )

    pool = sqlalchemy.create_engine(
        "postgresql+pg8000://",
        creator=getconn,
        pool_size=5,
        max_overflow=2,
        pool_timeout=30,
        pool_recycle=1800,
    )
    return pool

# Usage
if __name__ == "__main__":
    engine = connect_with_connector()
    with engine.connect() as conn:
        result = conn.execute(sqlalchemy.text("SELECT 1"))
        print(result.fetchone())
`, connectionName, dbName)
		} else {
			snippet.Dependencies = []string{"cloud-sql-python-connector[pymysql]>=1.13", "sqlalchemy>=2.0"}
			snippet.Code = fmt.Sprintf(`import os
from google.cloud.sql.connector import Connector
import sqlalchemy

def connect_with_connector():
    """Create connection using Cloud SQL Python Connector."""
    connector = Connector()

    def getconn():
        return connector.connect(
            "%s",  # Instance connection name
            "pymysql",
            user=os.environ["DB_USER"],
            password=os.environ["DB_PASS"],
            db=os.environ.get("DB_NAME", "%s"),
        )

    pool = sqlalchemy.create_engine(
        "mysql+pymysql://",
        creator=getconn,
        pool_size=5,
        max_overflow=2,
        pool_timeout=30,
        pool_recycle=1800,
    )
    return pool

# Usage
if __name__ == "__main__":
    engine = connect_with_connector()
    with engine.connect() as conn:
        result = conn.execute(sqlalchemy.text("SELECT 1"))
        print(result.fetchone())
`, connectionName, dbName)
		}

	case MethodAuthProxy:
		if dbType == PostgreSQL {
			snippet.Dependencies = []string{"psycopg2-binary>=2.9", "sqlalchemy>=2.0"}
			snippet.Code = fmt.Sprintf(`import os
import sqlalchemy

def connect_with_auth_proxy():
    """Connect via Cloud SQL Auth Proxy (must be running on localhost:%d)."""
    db_user = os.environ["DB_USER"]
    db_pass = os.environ["DB_PASS"]
    db_name = os.environ.get("DB_NAME", "%s")
    db_host = os.environ.get("DB_HOST", "127.0.0.1")

    url = f"postgresql+psycopg2://{db_user}:{db_pass}@{db_host}:%d/{db_name}"
    engine = sqlalchemy.create_engine(url)
    return engine

# Start Auth Proxy: ./cloud-sql-proxy %s

# Usage
if __name__ == "__main__":
    engine = connect_with_auth_proxy()
    with engine.connect() as conn:
        result = conn.execute(sqlalchemy.text("SELECT 1"))
        print(result.fetchone())
`, port, dbName, port, connectionName)
		} else {
			snippet.Dependencies = []string{"pymysql>=1.1", "sqlalchemy>=2.0"}
			snippet.Code = fmt.Sprintf(`import os
import sqlalchemy

def connect_with_auth_proxy():
    """Connect via Cloud SQL Auth Proxy (must be running on localhost:%d)."""
    db_user = os.environ["DB_USER"]
    db_pass = os.environ["DB_PASS"]
    db_name = os.environ.get("DB_NAME", "%s")
    db_host = os.environ.get("DB_HOST", "127.0.0.1")

    url = f"mysql+pymysql://{db_user}:{db_pass}@{db_host}:%d/{db_name}"
    engine = sqlalchemy.create_engine(url)
    return engine

# Start Auth Proxy: ./cloud-sql-proxy %s

# Usage
if __name__ == "__main__":
    engine = connect_with_auth_proxy()
    with engine.connect() as conn:
        result = conn.execute(sqlalchemy.text("SELECT 1"))
        print(result.fetchone())
`, port, dbName, port, connectionName)
		}

	case MethodDirectPrivateIP:
		if dbType == PostgreSQL {
			snippet.Dependencies = []string{"psycopg2-binary>=2.9", "sqlalchemy>=2.0"}
			snippet.Code = fmt.Sprintf(`import os
import sqlalchemy

def connect_with_private_ip():
    """Connect directly using Cloud SQL private IP."""
    db_user = os.environ["DB_USER"]
    db_pass = os.environ["DB_PASS"]
    db_name = os.environ.get("DB_NAME", "%s")

    url = f"postgresql+psycopg2://{db_user}:{db_pass}@%s:%d/{db_name}"
    engine = sqlalchemy.create_engine(url)
    return engine

# Note: Requires compute resource in same VPC as Cloud SQL

# Usage
if __name__ == "__main__":
    engine = connect_with_private_ip()
    with engine.connect() as conn:
        result = conn.execute(sqlalchemy.text("SELECT 1"))
        print(result.fetchone())
`, dbName, privateIP, port)
		} else {
			snippet.Dependencies = []string{"pymysql>=1.1", "sqlalchemy>=2.0"}
			snippet.Code = fmt.Sprintf(`import os
import sqlalchemy

def connect_with_private_ip():
    """Connect directly using Cloud SQL private IP."""
    db_user = os.environ["DB_USER"]
    db_pass = os.environ["DB_PASS"]
    db_name = os.environ.get("DB_NAME", "%s")

    url = f"mysql+pymysql://{db_user}:{db_pass}@%s:%d/{db_name}"
    engine = sqlalchemy.create_engine(url)
    return engine

# Note: Requires compute resource in same VPC as Cloud SQL

# Usage
if __name__ == "__main__":
    engine = connect_with_private_ip()
    with engine.connect() as conn:
        result = conn.execute(sqlalchemy.text("SELECT 1"))
        print(result.fetchone())
`, dbName, privateIP, port)
		}

	case MethodUnixSocket:
		if dbType == PostgreSQL {
			snippet.Dependencies = []string{"psycopg2-binary>=2.9", "sqlalchemy>=2.0"}
			snippet.Code = fmt.Sprintf(`import os
import sqlalchemy

def connect_with_unix_socket():
    """Connect using Unix socket (Cloud Run / App Engine)."""
    db_user = os.environ["DB_USER"]
    db_pass = os.environ["DB_PASS"]
    db_name = os.environ.get("DB_NAME", "%s")
    socket_path = os.environ.get("INSTANCE_UNIX_SOCKET", "/cloudsql/%s")

    url = f"postgresql+psycopg2://{db_user}:{db_pass}@/{db_name}?host={socket_path}"
    engine = sqlalchemy.create_engine(url)
    return engine

# Usage (Cloud Run / App Engine)
if __name__ == "__main__":
    engine = connect_with_unix_socket()
    with engine.connect() as conn:
        result = conn.execute(sqlalchemy.text("SELECT 1"))
        print(result.fetchone())
`, dbName, connectionName)
		} else {
			snippet.Dependencies = []string{"pymysql>=1.1", "sqlalchemy>=2.0"}
			snippet.Code = fmt.Sprintf(`import os
import sqlalchemy

def connect_with_unix_socket():
    """Connect using Unix socket (Cloud Run / App Engine)."""
    db_user = os.environ["DB_USER"]
    db_pass = os.environ["DB_PASS"]
    db_name = os.environ.get("DB_NAME", "%s")
    socket_path = os.environ.get("INSTANCE_UNIX_SOCKET", "/cloudsql/%s")

    url = f"mysql+pymysql://{db_user}:{db_pass}@/{db_name}?unix_socket={socket_path}"
    engine = sqlalchemy.create_engine(url)
    return engine

# Usage (Cloud Run / App Engine)
if __name__ == "__main__":
    engine = connect_with_unix_socket()
    with engine.connect() as conn:
        result = conn.execute(sqlalchemy.text("SELECT 1"))
        print(result.fetchone())
`, dbName, connectionName)
		}
	}

	return snippet
}

func generateNodeJSCode(method ConnectionMethod, dbType DatabaseType, connectionName, dbName string, port int, privateIP string) *CodeSnippet {
	snippet := &CodeSnippet{Language: LangNodeJS}

	switch method {
	case MethodConnector:
		if dbType == PostgreSQL {
			snippet.Dependencies = []string{"@google-cloud/cloud-sql-connector@^1.9", "pg@^8.11"}
			snippet.Code = fmt.Sprintf(`const { Connector } = require('@google-cloud/cloud-sql-connector');
const { Pool } = require('pg');

async function connectWithConnector() {
    const connector = new Connector();
    const clientOpts = await connector.getOptions({
        instanceConnectionName: '%s',
        ipType: 'PUBLIC',
    });

    const pool = new Pool({
        ...clientOpts,
        user: process.env.DB_USER,
        password: process.env.DB_PASS,
        database: process.env.DB_NAME || '%s',
        max: 5,
    });

    return pool;
}

// Usage
(async () => {
    const pool = await connectWithConnector();
    const result = await pool.query('SELECT 1 as value');
    console.log(result.rows);
    await pool.end();
})();
`, connectionName, dbName)
		} else {
			snippet.Dependencies = []string{"@google-cloud/cloud-sql-connector@^1.9", "mysql2@^3.11"}
			snippet.Code = fmt.Sprintf(`const { Connector } = require('@google-cloud/cloud-sql-connector');
const mysql = require('mysql2/promise');

async function connectWithConnector() {
    const connector = new Connector();
    const clientOpts = await connector.getOptions({
        instanceConnectionName: '%s',
        ipType: 'PUBLIC',
    });

    const pool = await mysql.createPool({
        ...clientOpts,
        user: process.env.DB_USER,
        password: process.env.DB_PASS,
        database: process.env.DB_NAME || '%s',
        connectionLimit: 5,
    });

    return pool;
}

// Usage
(async () => {
    const pool = await connectWithConnector();
    const [rows] = await pool.execute('SELECT 1 as value');
    console.log(rows);
    await pool.end();
})();
`, connectionName, dbName)
		}

	case MethodAuthProxy:
		if dbType == PostgreSQL {
			snippet.Dependencies = []string{"pg@^8.11"}
			snippet.Code = fmt.Sprintf(`const { Pool } = require('pg');

async function connectWithAuthProxy() {
    // Auth Proxy must be running: ./cloud-sql-proxy %s
    const pool = new Pool({
        user: process.env.DB_USER,
        password: process.env.DB_PASS,
        database: process.env.DB_NAME || '%s',
        host: process.env.DB_HOST || '127.0.0.1',
        port: %d,
        max: 5,
    });

    return pool;
}

// Usage
(async () => {
    const pool = await connectWithAuthProxy();
    const result = await pool.query('SELECT 1 as value');
    console.log(result.rows);
    await pool.end();
})();
`, connectionName, dbName, port)
		} else {
			snippet.Dependencies = []string{"mysql2@^3.11"}
			snippet.Code = fmt.Sprintf(`const mysql = require('mysql2/promise');

async function connectWithAuthProxy() {
    // Auth Proxy must be running: ./cloud-sql-proxy %s
    const pool = await mysql.createPool({
        user: process.env.DB_USER,
        password: process.env.DB_PASS,
        database: process.env.DB_NAME || '%s',
        host: process.env.DB_HOST || '127.0.0.1',
        port: %d,
        connectionLimit: 5,
    });

    return pool;
}

// Usage
(async () => {
    const pool = await connectWithAuthProxy();
    const [rows] = await pool.execute('SELECT 1 as value');
    console.log(rows);
    await pool.end();
})();
`, connectionName, dbName, port)
		}

	case MethodUnixSocket:
		if dbType == PostgreSQL {
			snippet.Dependencies = []string{"pg@^8.11"}
			snippet.Code = fmt.Sprintf(`const { Pool } = require('pg');

async function connectWithUnixSocket() {
    const pool = new Pool({
        user: process.env.DB_USER,
        password: process.env.DB_PASS,
        database: process.env.DB_NAME || '%s',
        host: process.env.INSTANCE_UNIX_SOCKET || '/cloudsql/%s',
    });

    return pool;
}

// Usage (Cloud Run / App Engine)
(async () => {
    const pool = await connectWithUnixSocket();
    const result = await pool.query('SELECT 1 as value');
    console.log(result.rows);
    await pool.end();
})();
`, dbName, connectionName)
		} else {
			snippet.Dependencies = []string{"mysql2@^3.11"}
			snippet.Code = fmt.Sprintf(`const mysql = require('mysql2/promise');

async function connectWithUnixSocket() {
    const pool = await mysql.createPool({
        user: process.env.DB_USER,
        password: process.env.DB_PASS,
        database: process.env.DB_NAME || '%s',
        socketPath: process.env.INSTANCE_UNIX_SOCKET || '/cloudsql/%s',
        connectionLimit: 5,
    });

    return pool;
}

// Usage (Cloud Run / App Engine)
(async () => {
    const pool = await connectWithUnixSocket();
    const [rows] = await pool.execute('SELECT 1 as value');
    console.log(rows);
    await pool.end();
})();
`, dbName, connectionName)
		}

	case MethodDirectPrivateIP:
		if dbType == PostgreSQL {
			snippet.Dependencies = []string{"pg@^8.11"}
			snippet.Code = fmt.Sprintf(`const { Pool } = require('pg');

async function connectWithPrivateIP() {
    const pool = new Pool({
        user: process.env.DB_USER,
        password: process.env.DB_PASS,
        database: process.env.DB_NAME || '%s',
        host: '%s',
        port: %d,
        max: 5,
    });

    return pool;
}

// Note: Requires compute resource in same VPC as Cloud SQL

// Usage
(async () => {
    const pool = await connectWithPrivateIP();
    const result = await pool.query('SELECT 1 as value');
    console.log(result.rows);
    await pool.end();
})();
`, dbName, privateIP, port)
		} else {
			snippet.Dependencies = []string{"mysql2@^3.11"}
			snippet.Code = fmt.Sprintf(`const mysql = require('mysql2/promise');

async function connectWithPrivateIP() {
    const pool = await mysql.createPool({
        user: process.env.DB_USER,
        password: process.env.DB_PASS,
        database: process.env.DB_NAME || '%s',
        host: '%s',
        port: %d,
        connectionLimit: 5,
    });

    return pool;
}

// Note: Requires compute resource in same VPC as Cloud SQL

// Usage
(async () => {
    const pool = await connectWithPrivateIP();
    const [rows] = await pool.execute('SELECT 1 as value');
    console.log(rows);
    await pool.end();
})();
`, dbName, privateIP, port)
		}
	}

	return snippet
}

func generateJavaCode(method ConnectionMethod, dbType DatabaseType, connectionName, dbName string, port int, privateIP string) *CodeSnippet {
	snippet := &CodeSnippet{Language: LangJava}

	if dbType == PostgreSQL {
		snippet.Dependencies = []string{
			"com.google.cloud.sql:postgres-socket-factory:1.15.0",
			"org.postgresql:postgresql:42.6.0",
			"com.zaxxer:HikariCP:5.0.1",
		}
	} else {
		snippet.Dependencies = []string{
			"com.google.cloud.sql:mysql-socket-factory-connector-j-8:1.15.0",
			"mysql:mysql-connector-java:8.0.33",
			"com.zaxxer:HikariCP:5.0.1",
		}
	}

	switch method {
	case MethodConnector:
		if dbType == PostgreSQL {
			snippet.Code = fmt.Sprintf(`import com.zaxxer.hikari.HikariConfig;
import com.zaxxer.hikari.HikariDataSource;
import javax.sql.DataSource;

public class CloudSQLConnector {

    public static DataSource createConnectionPool() {
        HikariConfig config = new HikariConfig();

        config.setJdbcUrl("jdbc:postgresql:///%s");
        config.setUsername(System.getenv("DB_USER"));
        config.setPassword(System.getenv("DB_PASS"));

        config.addDataSourceProperty("socketFactory",
            "com.google.cloud.sql.postgres.SocketFactory");
        config.addDataSourceProperty("cloudSqlInstance", "%s");

        config.setMaximumPoolSize(5);
        config.setMinimumIdle(2);
        config.setConnectionTimeout(30000);
        config.setIdleTimeout(600000);

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
`, dbName, connectionName)
		} else {
			snippet.Code = fmt.Sprintf(`import com.zaxxer.hikari.HikariConfig;
import com.zaxxer.hikari.HikariDataSource;
import javax.sql.DataSource;

public class CloudSQLConnector {

    public static DataSource createConnectionPool() {
        HikariConfig config = new HikariConfig();

        config.setJdbcUrl("jdbc:mysql:///%s");
        config.setUsername(System.getenv("DB_USER"));
        config.setPassword(System.getenv("DB_PASS"));

        config.addDataSourceProperty("socketFactory",
            "com.google.cloud.sql.mysql.SocketFactory");
        config.addDataSourceProperty("cloudSqlInstance", "%s");

        config.setMaximumPoolSize(5);
        config.setMinimumIdle(2);
        config.setConnectionTimeout(30000);
        config.setIdleTimeout(600000);

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
`, dbName, connectionName)
		}

	default:
		snippet.Code = fmt.Sprintf(`// For %s method, use standard JDBC connection
// Host: %s, Port: %d
// See connector method above for recommended approach`, method, privateIP, port)
	}

	return snippet
}

func generateGoCode(method ConnectionMethod, dbType DatabaseType, connectionName, dbName string, port int, privateIP string) *CodeSnippet {
	snippet := &CodeSnippet{Language: LangGo}

	switch method {
	case MethodConnector:
		if dbType == PostgreSQL {
			snippet.Dependencies = []string{
				"cloud.google.com/go/cloudsqlconn@v1.15.0",
				"github.com/jackc/pgx/v5@v5.7.0",
				"github.com/jackc/pgx/v5/stdlib@v5.7.0",
			}
			snippet.Code = fmt.Sprintf(`package main

import (
    "context"
    "database/sql"
    "fmt"
    "net"
    "os"

    "cloud.google.com/go/cloudsqlconn"
    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/stdlib"
)

func connectWithConnector() (*sql.DB, error) {
    dsn := fmt.Sprintf("user=%%s password=%%s dbname=%%s sslmode=disable",
        os.Getenv("DB_USER"),
        os.Getenv("DB_PASS"),
        getEnv("DB_NAME", "%s"),
    )

    config, err := pgx.ParseConfig(dsn)
    if err != nil {
        return nil, err
    }

    d, err := cloudsqlconn.NewDialer(context.Background())
    if err != nil {
        return nil, err
    }

    config.DialFunc = func(ctx context.Context, network, addr string) (net.Conn, error) {
        return d.Dial(ctx, "%s")
    }

    dbURI := stdlib.RegisterConnConfig(config)
    db, err := sql.Open("pgx", dbURI)
    if err != nil {
        return nil, err
    }

    db.SetMaxOpenConns(5)
    db.SetMaxIdleConns(2)

    return db, nil
}

func getEnv(key, fallback string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return fallback
}

func main() {
    db, err := connectWithConnector()
    if err != nil {
        panic(err)
    }
    defer db.Close()

    var result int
    err = db.QueryRow("SELECT 1").Scan(&result)
    if err != nil {
        panic(err)
    }
    fmt.Println("Connected:", result)
}
`, dbName, connectionName)
		} else {
			snippet.Dependencies = []string{
				"cloud.google.com/go/cloudsqlconn@v1.15.0",
				"github.com/go-sql-driver/mysql@v1.9.0",
			}
			snippet.Code = fmt.Sprintf(`package main

import (
    "context"
    "database/sql"
    "fmt"
    "net"
    "os"

    "cloud.google.com/go/cloudsqlconn"
    "github.com/go-sql-driver/mysql"
)

func connectWithConnector() (*sql.DB, error) {
    d, err := cloudsqlconn.NewDialer(context.Background())
    if err != nil {
        return nil, err
    }

    mysql.RegisterDialContext("cloudsql",
        func(ctx context.Context, addr string) (net.Conn, error) {
            return d.Dial(ctx, "%s")
        })

    cfg := mysql.Config{
        User:                 os.Getenv("DB_USER"),
        Passwd:               os.Getenv("DB_PASS"),
        DBName:               getEnv("DB_NAME", "%s"),
        Net:                  "cloudsql",
        Addr:                 "%s",
        AllowNativePasswords: true,
    }

    db, err := sql.Open("mysql", cfg.FormatDSN())
    if err != nil {
        return nil, err
    }

    db.SetMaxOpenConns(5)
    db.SetMaxIdleConns(2)

    return db, nil
}

func getEnv(key, fallback string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return fallback
}

func main() {
    db, err := connectWithConnector()
    if err != nil {
        panic(err)
    }
    defer db.Close()

    var result int
    err = db.QueryRow("SELECT 1").Scan(&result)
    if err != nil {
        panic(err)
    }
    fmt.Println("Connected:", result)
}
`, connectionName, dbName, connectionName)
		}

	default:
		// Leave Dependencies empty so appendDependencyNotes does not emit a
		// synthetic install command; direct the caller to pick a driver via
		// Notes instead.
		snippet.Notes = append(snippet.Notes, "Install the database/sql driver appropriate for your engine (pgx for Postgres, go-sql-driver/mysql for MySQL, microsoft/go-mssqldb for SQL Server).")
		snippet.Code = fmt.Sprintf(`// For %s method, use standard database/sql connection
// Host: %s, Port: %d
// See connector method above for recommended approach`, method, privateIP, port)
	}

	return snippet
}
