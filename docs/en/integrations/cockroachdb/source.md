---
title: "CockroachDB Source"
linkTitle: "Source"
type: docs
weight: 1
description: >
  CockroachDB is a distributed SQL database built for cloud applications.
no_list: true
---

## About

[CockroachDB][crdb-docs] is a distributed SQL database designed for cloud-native applications. It provides strong consistency, horizontal scalability, and built-in resilience with automatic failover and recovery. CockroachDB uses the PostgreSQL wire protocol, making it compatible with many PostgreSQL tools and drivers while providing unique features like multi-region deployments and distributed transactions.

**Minimum Version:** CockroachDB v25.1 or later is recommended for full tool compatibility.

[crdb-docs]: https://www.cockroachlabs.com/docs/


## Available Tools

{{< list-tools >}}

## Requirements

### Database User

This source uses standard authentication. You will need to [create a CockroachDB user][crdb-users] to login to the database with. For CockroachDB Cloud deployments, SSL/TLS is required.

[crdb-users]: https://www.cockroachlabs.com/docs/stable/create-user.html

### SSL/TLS Configuration

CockroachDB Cloud clusters require SSL/TLS connections. Use the `queryParams` section to configure SSL settings:

- **For CockroachDB Cloud**: Use `sslmode: require` at minimum
- **For self-hosted with certificates**: Use `sslmode: verify-full` with certificate paths
- **For local development only**: Use `sslmode: disable` (not recommended for production)

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
maxRetries: 5
retryBaseDelay: 500ms
queryParams:
  sslmode: require
  application_name: my-app

# MCP Security Settings (recommended for production)
readOnlyMode: true          # Read-only by default (MCP best practice)
enableWriteMode: false      # Set to true to allow write operations
maxRowLimit: 1000           # Limit query results
queryTimeoutSec: 30         # Prevent long-running queries
enableTelemetry: true       # Enable observability
telemetryVerbose: false     # Set true for detailed logs
clusterID: "my-cluster"     # Optional identifier
---
kind: tool
name: list_expenses
type: cockroachdb-sql
source: my_cockroachdb
description: List all expenses
statement: SELECT id, description, amount, category FROM expenses WHERE user_id = $1
parameters:
  - name: user_id
    type: string
    description: The user's ID
---
kind: tool
name: describe_expenses
type: cockroachdb-describe-table
source: my_cockroachdb
description: Describe the expenses table schema
---
kind: tool
name: list_expenses_indexes
type: cockroachdb-list-indexes
source: my_cockroachdb
description: List indexes on the expenses table
```

## Reference

### Required Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `type` | string | Must be `cockroachdb` |
| `host` | string | The hostname or IP address of the CockroachDB cluster |
| `port` | string | The port number (typically "26257") |
| `user` | string | The database user name |
| `database` | string | The database name to connect to |

### Optional Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `password` | string | "" | The database password (can be empty for certificate-based auth) |
| `maxRetries` | integer | 5 | Maximum number of connection retry attempts |
| `retryBaseDelay` | string | "500ms" | Base delay between retry attempts (exponential backoff) |
| `queryParams` | map | {} | Additional connection parameters (e.g., SSL configuration) |

### MCP Security Parameters

CockroachDB integration includes security features following the [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) specification:

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `readOnlyMode` | boolean | true | Enables read-only mode by default (MCP requirement) |
| `enableWriteMode` | boolean | false | Explicitly enable write operations (INSERT/UPDATE/DELETE/CREATE/DROP) |
| `maxRowLimit` | integer | 1000 | Maximum rows returned per SELECT query (auto-adds LIMIT clause) |
| `queryTimeoutSec` | integer | 30 | Query timeout in seconds to prevent long-running queries |
| `enableTelemetry` | boolean | true | Enable structured logging of tool invocations |
| `telemetryVerbose` | boolean | false | Enable detailed JSON telemetry output |
| `clusterID` | string | "" | Optional cluster identifier for telemetry |

### Query Parameters

Common query parameters for CockroachDB connections:

| Parameter | Values | Description |
|-----------|--------|-------------|
| `sslmode` | `disable`, `require`, `verify-ca`, `verify-full` | SSL/TLS mode (CockroachDB Cloud requires `require` or higher) |
| `sslrootcert` | file path | Path to root certificate for SSL verification |
| `sslcert` | file path | Path to client certificate |
| `sslkey` | file path | Path to client key |
| `application_name` | string | Application name for connection tracking |

## Advanced Usage

### Security and MCP Compliance

**Read-Only by Default**: The integration follows MCP best practices by defaulting to read-only mode. This prevents accidental data modifications:

```yaml
kind: source
name: my_cockroachdb
readOnlyMode: true        # Default behavior
enableWriteMode: false    # Explicit write opt-in required
```

To enable write operations:

```yaml
kind: source
name: my_cockroachdb
readOnlyMode: false       # Disable read-only protection
enableWriteMode: true     # Explicitly allow writes
```

**Query Limits**: Automatic row limits prevent excessive data retrieval:
- SELECT queries automatically get `LIMIT 1000` appended (configurable via `maxRowLimit`)
- Queries are terminated after 30 seconds (configurable via `queryTimeoutSec`)

**Observability**: Structured telemetry provides visibility into tool usage:
- Tool invocations are logged with status, latency, and row counts
- SQL queries are redacted to protect sensitive values
- Set `telemetryVerbose: true` for detailed JSON logs

### Use UUID Primary Keys

CockroachDB performs best with UUID primary keys rather than sequential integers to avoid transaction hotspots:

```sql
CREATE TABLE expenses (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  description TEXT,
  amount DECIMAL(10,2)
);
```

### Automatic Transaction Retry

This source uses the official `cockroach-go/v2` library which provides automatic transaction retry for serialization conflicts. For write operations requiring explicit transaction control, tools can use the `ExecuteTxWithRetry` method.

### Multi-Region Deployments

CockroachDB supports multi-region deployments with automatic data distribution. Configure your cluster's regions and survival goals separately from the Toolbox configuration. The source will connect to any node in the cluster.

### Connection Pooling

The source maintains a connection pool to the CockroachDB cluster. The pool automatically handles:
- Load balancing across cluster nodes
- Connection retry with exponential backoff
- Health checking of connections

## Troubleshooting

### SSL/TLS Errors

If you encounter "server requires encryption" errors:

1. For CockroachDB Cloud, ensure `sslmode` is set to `require` or higher:
   ```yaml
   queryParams:
     sslmode: require
   ```

2. For certificate verification, download your cluster's root certificate and configure:
   ```yaml
   queryParams:
     sslmode: verify-full
     sslrootcert: /path/to/ca.crt
   ```

### Connection Timeouts

If experiencing connection timeouts:

1. Check network connectivity to the CockroachDB cluster
2. Verify firewall rules allow connections on port 26257
3. For CockroachDB Cloud, ensure IP allowlisting is configured
4. Increase `maxRetries` or `retryBaseDelay` if needed

### Transaction Retry Errors

CockroachDB may encounter serializable transaction conflicts. The integration automatically handles these retries using the cockroach-go library. If you see retry-related errors, check:

1. Database load and contention
2. Query patterns that might cause conflicts
3. Consider using `SELECT FOR UPDATE` for explicit locking

## Additional Resources

- [CockroachDB Documentation](https://www.cockroachlabs.com/docs/)
- [CockroachDB Best Practices](https://www.cockroachlabs.com/docs/stable/performance-best-practices-overview.html)
- [Multi-Region Capabilities](https://www.cockroachlabs.com/docs/stable/multiregion-overview.html)
- [Connection Parameters](https://www.cockroachlabs.com/docs/stable/connection-parameters.html)
