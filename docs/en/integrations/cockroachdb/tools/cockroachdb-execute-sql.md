---
title: "cockroachdb-execute-sql"
type: docs
weight: 1
description: >
  Execute ad-hoc SQL statements against a CockroachDB database.

---

## About

A `cockroachdb-execute-sql` tool executes ad-hoc SQL statements against a CockroachDB database. This tool is designed for interactive workflows where the SQL query is provided dynamically at runtime, making it ideal for developer assistance and exploratory data analysis.

The tool takes a single `sql` parameter containing the SQL statement to execute and returns the query results.

> **Note:** This tool is intended for developer assistant workflows with human-in-the-loop and shouldn't be used for production agents. For production use cases with predefined queries, use [cockroachdb-sql](cockroachdb-sql.md) instead.


## Compatible Sources

{{< compatible-sources >}}

## Parameters

The tool accepts a single runtime parameter:

| Parameter | Type | Description |
|-----------|------|-------------|
| `sql` | string | The SQL statement to execute |

## Example

```yaml
kind: source
name: my_cockroachdb
type: cockroachdb
host: your-cluster.cockroachlabs.cloud
port: "26257"
user: myuser
password: mypassword
database: defaultdb
queryParams:
  sslmode: require
---
kind: tool
name: execute_sql
type: cockroachdb-execute-sql
source: my_cockroachdb
description: Execute any SQL statement against the CockroachDB database
```

### Usage Examples

#### Simple SELECT Query

```json
{
  "sql": "SELECT * FROM users LIMIT 10"
}
```

#### Query with Aggregations

```json
{
  "sql": "SELECT category, COUNT(*) as count, SUM(amount) as total FROM expenses GROUP BY category ORDER BY total DESC"
}
```

#### Database Introspection

```json
{
  "sql": "SHOW TABLES"
}
```

```json
{
  "sql": "SHOW COLUMNS FROM expenses"
}
```

#### Multi-Region Information

```json
{
  "sql": "SHOW REGIONS FROM DATABASE defaultdb"
}
```

```json
{
  "sql": "SHOW ZONE CONFIGURATIONS"
}
```

### CockroachDB-Specific Features

#### Check Cluster Version

```json
{
  "sql": "SELECT version()"
}
```

#### View Node Status

```json
{
  "sql": "SELECT node_id, address, locality, is_live FROM crdb_internal.gossip_nodes"
}
```

#### Check Replication Status

```json
{
  "sql": "SELECT range_id, start_key, end_key, replicas, lease_holder FROM crdb_internal.ranges LIMIT 10"
}
```

#### View Table Regions

```json
{
  "sql": "SHOW REGIONS FROM TABLE expenses"
}
```

## Reference

### Required Fields

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Must be `cockroachdb-execute-sql` |
| `source` | string | Name of the CockroachDB source to use |
| `description` | string | Human-readable description for the LLM |

### Optional Fields

| Field | Type | Description |
|-------|------|-------------|
| `authRequired` | array | List of authentication services required |

## Advanced Usage

### Best Practices

#### Use for Exploration, Not Production

This tool is ideal for:
- Interactive database exploration
- Ad-hoc analysis and reporting
- Debugging and troubleshooting
- Schema inspection

For production use cases, use [cockroachdb-sql](./cockroachdb-sql.md) with parameterized queries.

#### Be Cautious with Data Modification

While this tool can execute any SQL statement, be careful with:
- `INSERT`, `UPDATE`, `DELETE` statements
- `DROP` or `ALTER` statements
- Schema changes in production

#### Use LIMIT for Large Results

Always use `LIMIT` clauses when exploring data:

```sql
SELECT * FROM large_table LIMIT 100
```

#### Leverage CockroachDB's SQL Extensions

CockroachDB supports PostgreSQL syntax plus extensions:

```sql
-- Show database survival goal
SHOW SURVIVAL GOAL FROM DATABASE defaultdb;

-- View zone configurations
SHOW ZONE CONFIGURATION FOR TABLE expenses;

-- Check table localities
SHOW CREATE TABLE expenses;
```

### Security Considerations

#### SQL Injection Risk

Since this tool executes arbitrary SQL, it should only be used with:
- Trusted users in interactive sessions
- Human-in-the-loop workflows
- Development and testing environments

Never expose this tool directly to end users without proper authorization controls.

#### Use Authentication

Configure the `authRequired` field to restrict access:

```yaml
kind: tool
name: execute_sql
type: cockroachdb-execute-sql
source: my_cockroachdb
description: Execute SQL statements
authRequired:
  - my-auth-service
```

#### Read-Only Users

For safer exploration, create read-only database users:

```sql
CREATE USER readonly_user;
GRANT SELECT ON DATABASE defaultdb TO readonly_user;
```

### Common Use Cases

#### Database Administration

```sql
-- View database size
SELECT
  table_name,
  pg_size_pretty(pg_total_relation_size(table_name::regclass)) AS size
FROM information_schema.tables
WHERE table_schema = 'public'
ORDER BY pg_total_relation_size(table_name::regclass) DESC;
```

#### Performance Analysis

```sql
-- Find slow queries
SELECT query, count, mean_latency
FROM crdb_internal.statement_statistics
WHERE mean_latency > INTERVAL '1 second'
ORDER BY mean_latency DESC
LIMIT 10;
```

#### Data Quality Checks

```sql
-- Find NULL values
SELECT COUNT(*) as null_count
FROM expenses
WHERE description IS NULL OR amount IS NULL;

-- Find duplicates
SELECT user_id, email, COUNT(*) as count
FROM users
GROUP BY user_id, email
HAVING COUNT(*) > 1;
```

## Troubleshooting

The tool will return descriptive errors for:
- **Syntax errors**: Invalid SQL syntax
- **Permission errors**: Insufficient user privileges
- **Connection errors**: Network or authentication issues
- **Runtime errors**: Constraint violations, type mismatches, etc.

## Additional Resources

- [cockroachdb-sql](./cockroachdb-sql.md) - For parameterized, production-ready queries
- [cockroachdb-list-tables](./cockroachdb-list-tables.md) - List tables in the database
- [cockroachdb-list-schemas](./cockroachdb-list-schemas.md) - List database schemas
- [CockroachDB Source](../source.md) - Source configuration reference
- [CockroachDB SQL Reference](https://www.cockroachlabs.com/docs/stable/sql-statements.html) - Official SQL documentation
