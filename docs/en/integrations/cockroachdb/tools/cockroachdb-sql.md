---
title: "cockroachdb-sql"
type: docs
weight: 1
description: >
  Execute parameterized SQL queries in CockroachDB.

---

## About

The `cockroachdb-sql` tool allows you to execute parameterized SQL queries against a CockroachDB database. This tool supports prepared statements with parameter binding, template parameters for dynamic query construction, and automatic transaction retry for resilience against serialization conflicts.

## Compatible Sources

{{< compatible-sources >}}

## Parameters

Parameters allow you to safely pass values into your SQL queries using prepared statements. CockroachDB uses PostgreSQL-style parameter placeholders: `$1`, `$2`, etc.

### Parameter Types

- `string`: Text values
- `number`: Numeric values (integers or decimals)
- `boolean`: True/false values
- `array`: Array of values

### Example with Multiple Parameters

```yaml
kind: tool
name: filter_expenses
type: cockroachdb-sql
source: my_cockroachdb
description: Filter expenses by category and date range
statement: |
  SELECT id, description, amount, category, expense_date
  FROM expenses
  WHERE user_id = $1
    AND category = $2
    AND expense_date >= $3
    AND expense_date <= $4
  ORDER BY expense_date DESC
parameters:
  - name: user_id
    type: string
    description: The user's UUID
  - name: category
    type: string
    description: Expense category (e.g., "Food", "Transport")
  - name: start_date
    type: string
    description: Start date in YYYY-MM-DD format
  - name: end_date
    type: string
    description: End date in YYYY-MM-DD format
```

### Template Parameters

Template parameters enable dynamic query construction by replacing placeholders in the SQL statement before parameter binding. This is useful for dynamic table names, column names, or query structure.

#### Example with Template Parameters

```yaml
kind: tool
name: get_column_data
type: cockroachdb-sql
source: my_cockroachdb
description: Get data from a specific column
statement: |
  SELECT {{column_name}}
  FROM {{table_name}}
  WHERE user_id = $1
  LIMIT 100
templateParameters:
  - name: table_name
    type: string
    description: The table to query
  - name: column_name
    type: string
    description: The column to retrieve
parameters:
  - name: user_id
    type: string
    description: The user's UUID
```

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
name: get_user_orders
type: cockroachdb-sql
source: my_cockroachdb
description: Get all orders for a specific user
statement: |
  SELECT o.id, o.order_date, o.total_amount, o.status
  FROM orders o
  WHERE o.user_id = $1
  ORDER BY o.order_date DESC
parameters:
  - name: user_id
    type: string
    description: The UUID of the user
```

## Reference

### Required Fields

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Must be `cockroachdb-sql` |
| `source` | string | Name of the CockroachDB source to use |
| `description` | string | Human-readable description of what the tool does |
| `statement` | string | The SQL query to execute |

### Optional Fields

| Field | Type | Description |
|-------|------|-------------|
| `parameters` | array | List of parameter definitions for the query |
| `templateParameters` | array | List of template parameters for dynamic query construction |
| `authRequired` | array | List of authentication services required |


## Advanced Usage

### Use UUID Primary Keys

CockroachDB performs best with UUID primary keys to avoid transaction hotspots:

```sql
CREATE TABLE orders (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL,
  order_date TIMESTAMP DEFAULT now(),
  total_amount DECIMAL(10,2)
);
```

### Use Indexes for Performance

Create indexes on frequently queried columns:

```sql
CREATE INDEX idx_orders_user_id ON orders(user_id);
CREATE INDEX idx_orders_date ON orders(order_date DESC);
```

### Use JOINs Efficiently

CockroachDB supports standard SQL JOINs. Keep joins efficient by:
- Adding appropriate indexes
- Using UUIDs for foreign keys
- Limiting result sets with WHERE clauses

```yaml
kind: tool
name: get_user_with_orders
type: cockroachdb-sql
source: my_cockroachdb
description: Get user details with their recent orders
statement: |
  SELECT u.name, u.email, o.id as order_id, o.order_date, o.total_amount
  FROM users u
  LEFT JOIN orders o ON u.id = o.user_id
  WHERE u.id = $1
  ORDER BY o.order_date DESC
  LIMIT 10
parameters:
  - name: user_id
    type: string
    description: The user's UUID
```

### Handle NULL Values

Use COALESCE or NULL checks when dealing with nullable columns:

```sql
SELECT id, description, COALESCE(notes, 'No notes') as notes
FROM expenses
WHERE user_id = $1
```

### Aggregations

```yaml
kind: tool
name: expense_summary
type: cockroachdb-sql
source: my_cockroachdb
description: Get expense summary by category for a user
statement: |
  SELECT 
    category,
    COUNT(*) as count,
    SUM(amount) as total_amount,
    AVG(amount) as avg_amount
  FROM expenses
  WHERE user_id = $1
    AND expense_date >= $2
  GROUP BY category
  ORDER BY total_amount DESC
parameters:
  - name: user_id
    type: string
    description: The user's UUID
  - name: start_date
    type: string
    description: Start date in YYYY-MM-DD format
```

### Window Functions

```yaml
kind: tool
name: running_total
type: cockroachdb-sql
source: my_cockroachdb
description: Get running total of expenses
statement: |
  SELECT 
    expense_date,
    amount,
    SUM(amount) OVER (ORDER BY expense_date) as running_total
  FROM expenses
  WHERE user_id = $1
  ORDER BY expense_date
parameters:
  - name: user_id
    type: string
    description: The user's UUID
```

### Common Table Expressions (CTEs)

```yaml
kind: tool
name: top_spenders
type: cockroachdb-sql
source: my_cockroachdb
description: Find top spending users
statement: |
  WITH user_totals AS (
    SELECT 
      user_id,
      SUM(amount) as total_spent
    FROM expenses
    WHERE expense_date >= $1
    GROUP BY user_id
  )
  SELECT 
    u.name,
    u.email,
    ut.total_spent
  FROM user_totals ut
  JOIN users u ON ut.user_id = u.id
  ORDER BY ut.total_spent DESC
  LIMIT 10
parameters:
  - name: start_date
    type: string
    description: Start date in YYYY-MM-DD format
```


## Troubleshooting

The tool automatically handles:
- **Connection errors**: Retried with exponential backoff
- **Serialization conflicts**: Automatically retried using cockroach-go library
- **Invalid parameters**: Returns descriptive error messages
- **SQL syntax errors**: Returns database error details

## Additional Resources

- [cockroachdb-execute-sql](./cockroachdb-execute-sql.md) - For ad-hoc SQL execution
- [cockroachdb-list-tables](./cockroachdb-list-tables.md) - List tables in the database
- [cockroachdb-list-schemas](./cockroachdb-list-schemas.md) - List database schemas
- [CockroachDB Source](../source.md) - Source configuration reference
