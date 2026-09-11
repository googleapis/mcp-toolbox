---
name: database-schema-discovery
description: Automatically discover database schemas and generate a tools.yaml configuration file for MCP Toolbox. Supports PostgreSQL, SQLite, MySQL, and SQL Server.
---

# Database Schema Discovery

This skill connects to a relational database, introspects its tables and columns, and generates a `tools.yaml` file containing the standard CRUD tools (`list`, `get`, `insert`, `update`, `delete`) for each table. 

## Requirements

The script is a Node.js script. Before using it, ensure you have installed its dependencies:
```bash
cd skills/user/database-schema-discovery
npm install
```

## Usage

You can invoke this skill using Node.js. Select the appropriate script for your database engine:

```bash
# For PostgreSQL
node scripts/postgres.js --source-name="my_postgres" --connection-string="postgresql://user:password@localhost:5432/mydb"

# For SQLite
node scripts/sqlite.js --source-name="my_sqlite" --db-path="/path/to/database.db"

# For MySQL
node scripts/mysql.js --source-name="my_mysql" --connection-string="mysql://user:password@localhost:3306/mydb"

# For SQL Server
node scripts/mssql.js --source-name="my_mssql" --connection-string="mssql://user:password@localhost:1433/mydb"
```

### Parameters

| Name | Description | Required | Supported Scripts |
|------|-------------|----------|---------|
| `--source-name` | The name of the database source to reference in `tools.yaml`. | Yes | All |
| `--connection-string` | The connection URL for the database. | Yes | postgres, mysql, mssql |
| `--db-path` | The local file path to the SQLite database. | Yes | sqlite |

### Output

The script generates or updates a `tools.yaml` file in the current working directory. It will append to the `tools:` block if the file already exists.
