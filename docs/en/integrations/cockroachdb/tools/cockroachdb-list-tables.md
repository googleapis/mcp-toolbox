---
title: "cockroachdb-list-tables"
type: docs
weight: 1
description: >
  List tables in a CockroachDB database with schema details.

---

## About

The `cockroachdb-list-tables` tool retrieves a list of tables from a CockroachDB database. It provides detailed information about table structure, including columns, constraints, indexes, and foreign key relationships.

This tool is useful for:
- Database schema discovery
- Understanding table relationships
- Generating context for AI-powered database queries
- Documentation and analysis


## Compatible Sources

{{< compatible-sources >}}

## Parameters

The tool accepts optional runtime parameters:

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `table_names` | array | all tables | List of specific table names to retrieve |
| `output_format` | string | "detailed" | Output format: "simple" or "detailed" |

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
name: list_all_tables
type: cockroachdb-list-tables
source: my_cockroachdb
description: List all user tables in the database with their structure
```

### Usage Examples

#### List All Tables

```json
{}
```

#### List Specific Tables

```json
{
  "table_names": ["users", "orders", "expenses"]
}
```

#### Simple Output

```json
{
  "output_format": "simple"
}
```

#### Output Structure

##### Simple Format Output

```json
{
  "table_name": "users",
  "estimated_rows": 1000,
  "size": "128 KB"
}
```

##### Detailed Format Output

```json
{
  "table_name": "users",
  "schema": "public",
  "columns": [
    {
      "name": "id",
      "type": "UUID",
      "nullable": false,
      "default": "gen_random_uuid()"
    },
    {
      "name": "email",
      "type": "STRING",
      "nullable": false,
      "default": null
    },
    {
      "name": "created_at",
      "type": "TIMESTAMP",
      "nullable": false,
      "default": "now()"
    }
  ],
  "primary_key": ["id"],
  "indexes": [
    {
      "name": "users_pkey",
      "columns": ["id"],
      "unique": true,
      "primary": true
    },
    {
      "name": "users_email_idx",
      "columns": ["email"],
      "unique": true,
      "primary": false
    }
  ],
  "foreign_keys": [],
  "constraints": [
    {
      "name": "users_email_check",
      "type": "CHECK",
      "definition": "email ~* '^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\\.[A-Z|a-z]{2,}$'"
    }
  ]
}
```

#### CockroachDB-Specific Information

##### UUID Primary Keys

The tool recognizes CockroachDB's recommended UUID primary key pattern:

```sql
CREATE TABLE users (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  ...
);
```

##### Multi-Region Tables

For multi-region tables, the output includes locality information:

```json
{
  "table_name": "users",
  "locality": "REGIONAL BY ROW",
  "regions": ["us-east-1", "us-west-2", "eu-west-1"]
}
```

##### Interleaved Tables

The tool shows parent-child relationships for interleaved tables (legacy feature):

```json
{
  "table_name": "order_items",
  "interleaved_in": "orders"
}
```

## Output Format

### Simple Format

Returns basic table information:
- Table name
- Row count estimate
- Size information

```json
{
  "table_names": ["users"],
  "output_format": "simple"
}
```

### Detailed Format (Default)

Returns comprehensive table information:
- Table name and schema
- All columns with types and constraints
- Primary keys
- Foreign keys and relationships
- Indexes
- Check constraints
- Table size and row counts

```json
{
  "table_names": ["users", "orders"],
  "output_format": "detailed"
}
```

## Reference

### Required Fields

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Must be `cockroachdb-list-tables` |
| `source` | string | Name of the CockroachDB source to use |
| `description` | string | Human-readable description for the LLM |

### Optional Fields

| Field | Type | Description |
|-------|------|-------------|
| `authRequired` | array | List of authentication services required |

## Advanced Usage

### Best Practices

#### Use for Schema Discovery

The tool is ideal for helping AI assistants understand your database structure:

```yaml
kind: tool
name: discover_schema
type: cockroachdb-list-tables
source: my_cockroachdb
description: |
  Use this tool first to understand the database schema before generating queries.
  It shows all tables, their columns, data types, and relationships.
```

#### Filter Large Schemas

For databases with many tables, specify relevant tables:

```json
{
  "table_names": ["users", "orders", "products"],
  "output_format": "detailed"
}
```

#### Use Simple Format for Overviews

When you need just table names and sizes:

```json
{
  "output_format": "simple"
}
```

### Excluded Tables

The tool automatically excludes system tables and schemas:
- `pg_catalog.*` - PostgreSQL system catalog
- `information_schema.*` - SQL standard information schema
- `crdb_internal.*` - CockroachDB internal tables
- `pg_extension.*` - PostgreSQL extension tables

Only user-created tables in the public schema (and other user schemas) are returned.

### Integration with AI Assistants

#### Prompt Example

```yaml
kind: tool
name: list_tables
type: cockroachdb-list-tables
source: my_cockroachdb
description: |
  Lists all tables in the database with detailed schema information.
  Use this tool to understand:
  - What tables exist
  - What columns each table has
  - Data types and constraints
  - Relationships between tables (foreign keys)
  - Available indexes

  Always call this tool before generating SQL queries to ensure
  you use correct table and column names.
```

### Common Use Cases

#### Generate Context for Queries

```json
{}
```

This provides comprehensive schema information that helps AI assistants generate accurate SQL queries.

#### Analyze Table Structure

```json
{
  "table_names": ["users"],
  "output_format": "detailed"
}
```

Perfect for understanding a specific table's structure, constraints, and relationships.

#### Quick Schema Overview

```json
{
  "output_format": "simple"
}
```

Gets a quick list of tables with basic statistics.

### Performance Considerations

- **Simple format** is faster for large databases
- **Detailed format** queries system tables extensively
- Specifying `table_names` reduces query time
- Results are fetched in a single query for efficiency


## Troubleshooting

The tool handles common errors:
- **Table not found**: Returns empty result for non-existent tables
- **Permission errors**: Returns error if user lacks SELECT privileges
- **Connection errors**: Returns connection failure details

## Additional Resources

- [cockroachdb-sql](./cockroachdb-sql.md) - Execute parameterized queries
- [cockroachdb-execute-sql](./cockroachdb-execute-sql.md) - Execute ad-hoc SQL
- [cockroachdb-list-schemas](./cockroachdb-list-schemas.md) - List database schemas
- [CockroachDB Source](../source.md) - Source configuration reference
- [CockroachDB Schema Design](https://www.cockroachlabs.com/docs/stable/schema-design-overview.html) - Best practices
