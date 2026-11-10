---
title: "cockroachdb-list-schemas"
type: docs
weight: 1
description: >
  List schemas in a CockroachDB database.

---

## About

The `cockroachdb-list-schemas` tool retrieves a list of schemas (namespaces) in a CockroachDB database. Schemas are used to organize database objects such as tables, views, and functions into logical groups.

This tool is useful for:
- Understanding database organization
- Discovering available schemas
- Multi-tenant application analysis
- Schema-level access control planning

## Compatible Sources

{{< compatible-sources >}}

### Requirements

To list schemas, the user needs:
- `CONNECT` privilege on the database
- No specific schema privileges required for listing

To query objects within schemas, the user needs:
- `USAGE` privilege on the schema
- Appropriate object privileges (SELECT, INSERT, etc.)

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
name: list_schemas
type: cockroachdb-list-schemas
source: my_cockroachdb
description: List all schemas in the database
```
### Usage Example

```json
{}
```

No parameters are required. The tool automatically lists all user-defined schemas.

## Output Format

The tool returns a list of schemas with the following information:

```json
[
  {
    "catalog_name": "defaultdb",
    "schema_name": "public",
    "is_user_defined": true
  },
  {
    "catalog_name": "defaultdb",
    "schema_name": "analytics",
    "is_user_defined": true
  }
]
```

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `catalog_name` | string | The database (catalog) name |
| `schema_name` | string | The schema name |
| `is_user_defined` | boolean | Whether this is a user-created schema (excludes system schemas) |


## Reference

### Required Fields

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Must be `cockroachdb-list-schemas` |
| `source` | string | Name of the CockroachDB source to use |
| `description` | string | Human-readable description for the LLM |

### Optional Fields

| Field | Type | Description |
|-------|------|-------------|
| `authRequired` | array | List of authentication services required |


## Advanced Usage

### Default Schemas

CockroachDB includes several standard schemas:

- **`public`**: The default schema for user objects
- **`pg_catalog`**: PostgreSQL system catalog (excluded from results)
- **`information_schema`**: SQL standard metadata views (excluded from results)
- **`crdb_internal`**: CockroachDB internal metadata (excluded from results)
- **`pg_extension`**: PostgreSQL extension objects (excluded from results)

The tool filters out system schemas and only returns user-defined schemas.

### Schema Management in CockroachDB

#### Creating Schemas

```sql
CREATE SCHEMA analytics;
```

#### Using Schemas

```sql
-- Create table in specific schema
CREATE TABLE analytics.revenue (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  amount DECIMAL(10,2),
  date DATE
);

-- Query from specific schema
SELECT * FROM analytics.revenue;
```

#### Schema Search Path

The search path determines which schemas are searched for unqualified object names:

```sql
-- Show current search path
SHOW search_path;

-- Set search path
SET search_path = analytics, public;
```

### Multi-Tenant Applications

Schemas are commonly used for multi-tenant applications:

```sql
-- Create schema per tenant
CREATE SCHEMA tenant_acme;
CREATE SCHEMA tenant_globex;

-- Create same table structure in each schema
CREATE TABLE tenant_acme.orders (...);
CREATE TABLE tenant_globex.orders (...);
```

The `cockroachdb-list-schemas` tool helps discover all tenant schemas:

```yaml
kind: tool
name: list_tenants
type: cockroachdb-list-schemas
source: my_cockroachdb
description: |
  List all tenant schemas in the database.
  Each schema represents a separate tenant's data namespace.
```

### Best Practices

#### Use Schemas for Organization

Group related tables into schemas:

```sql
CREATE SCHEMA sales;
CREATE SCHEMA inventory;
CREATE SCHEMA hr;

CREATE TABLE sales.orders (...);
CREATE TABLE inventory.products (...);
CREATE TABLE hr.employees (...);
```

#### Schema Naming Conventions

Use clear, descriptive schema names:
- Lowercase names
- Use underscores for multi-word names
- Avoid reserved keywords
- Use prefixes for grouped schemas (e.g., `tenant_`, `app_`)

#### Schema-Level Permissions

Schemas enable fine-grained access control:

```sql
-- Grant access to specific schema
GRANT USAGE ON SCHEMA analytics TO analyst_role;
GRANT SELECT ON ALL TABLES IN SCHEMA analytics TO analyst_role;

-- Revoke access
REVOKE ALL ON SCHEMA hr FROM public;
```

### Integration with Other Tools

#### Combined with List Tables

```yaml
kind: tool
name: list_schemas
type: cockroachdb-list-schemas
source: my_cockroachdb
description: List all schemas first
---
kind: tool
name: list_tables
type: cockroachdb-list-tables
source: my_cockroachdb
description: |
  List tables in the database.
  Use list_schemas first to understand schema organization.
```

#### Schema Discovery Workflow

1. Call `cockroachdb-list-schemas` to discover schemas
2. Call `cockroachdb-list-tables` to see tables in each schema
3. Generate queries using fully qualified names: `schema.table`

### Common Use Cases

#### Discover Database Structure

```yaml
kind: tool
name: discover_schemas
type: cockroachdb-list-schemas
source: my_cockroachdb
description: |
  Discover how the database is organized into schemas.
  Use this to understand the logical grouping of tables.
```

#### Multi-Tenant Analysis

```yaml
kind: tool
name: list_tenant_schemas
type: cockroachdb-list-schemas
source: my_cockroachdb
description: |
  List all tenant schemas (each tenant has their own schema).
  Schema names follow the pattern: tenant_<company_name>
```

#### Schema Migration Planning

```yaml
kind: tool
name: audit_schemas
type: cockroachdb-list-schemas
source: my_cockroachdb
description: |
  Audit existing schemas before migration.
  Identifies all schemas that need to be migrated.
```

### CockroachDB-Specific Features

#### System Schemas

CockroachDB includes PostgreSQL-compatible system schemas plus CockroachDB-specific ones:

- `crdb_internal.*`: CockroachDB internal metadata and statistics
- `pg_catalog.*`: PostgreSQL system catalog
- `information_schema.*`: SQL standard information schema

These are automatically filtered from the results.

### User-Defined Flag

The `is_user_defined` field helps distinguish:
- `true`: User-created schemas
- `false`: System schemas (already filtered out)


## Troubleshooting

The tool handles common errors:
- **Connection errors**: Returns connection failure details
- **Permission errors**: Returns error if user lacks USAGE privilege
- **Empty results**: Returns empty array if no user schemas exist

## Additional Resources

- [cockroachdb-sql](./cockroachdb-sql.md) - Execute parameterized queries
- [cockroachdb-execute-sql](./cockroachdb-execute-sql.md) - Execute ad-hoc SQL
- [cockroachdb-list-tables](./cockroachdb-list-tables.md) - List tables in the database
- [CockroachDB Source](../source.md) - Source configuration reference
- [CockroachDB Schema Design](https://www.cockroachlabs.com/docs/stable/schema-design-overview.html) - Official documentation
