---
title: "Oracle Source"
linkTitle: "Source"
type: docs
weight: 1
description: >
  Oracle Database is a widely-used relational database management system.
no_list: true
---

## About

[Oracle Database][oracle-docs] is a multi-model database management system
produced and marketed by Oracle Corporation. It is commonly used for running
online transaction processing (OLTP), data warehousing (DW), and mixed (OLTP &
DW) database workloads.

[oracle-docs]: https://www.oracle.com/database/


## Available Tools

{{< list-tools >}}

## Requirements

### Database User

This source uses standard authentication. You will need to [create an Oracle
user][oracle-users] to log in to the database with the necessary permissions.

[oracle-users]:
    https://docs.oracle.com/en/database/oracle/oracle-database/21/sqlrf/CREATE-USER.html

### Oracle Driver Requirement (Conditional)

The Oracle source offers two connection drivers:

1. **Pure Go Driver (`useOCI: false`, default):** Uses the `go-ora` library.
   This driver is simpler and does not require any local Oracle software
   installation, but it **lacks support for advanced features** like Oracle
   Wallets or Kerberos authentication.

2. **OCI-Based Driver (`useOCI: true`):** Uses the `godror` library, which
   provides access to **advanced Oracle features** like Digital Wallet support.

If you set `useOCI: true`, you **must** install the **Oracle Instant Client**
libraries on the machine where this tool runs.

You can download the Instant Client from the official Oracle website: [Oracle
Instant Client
Downloads](https://www.oracle.com/database/technologies/instant-client/downloads.html)

### Connection Methods

You can configure the connection to your Oracle database using one of the
following three methods. **You should only use one method** in your source
configuration.

#### Basic Connection (Host/Port/Service Name)

This is the most straightforward method, where you provide the connection
details as separate fields:

- `host`: The IP address or hostname of the database server.
- `port`: The port number the Oracle listener is running on (typically 1521).
- `serviceName`: The service name for the database instance you wish to connect
  to.

#### Connection String

As an alternative, you can provide all the connection details in a single
`connectionString`. This is a convenient way to consolidate the connection
information. The typical format is `hostname:port/servicename`.

#### TNS Alias

For environments that use a `tnsnames.ora` configuration file, you can connect
using a TNS (Transparent Network Substrate) alias.

- `tnsAlias`: Specify the alias name defined in your `tnsnames.ora` file.
- `tnsAdmin` (Optional): If your configuration file is not in a standard
  location, you can use this field to provide the path to the directory
  containing it. This setting will override the `TNS_ADMIN` environment
  variable.

### Session Context & Security Configuration (Optional)

If your organization uses Oracle Virtual Private Database (VPD), PeopleSoft
session security packages, or custom session initialization:

- **Optional Caller Authentication:** Authentication is optional. When incoming
  tool requests contain verified JWT claims, the caller's identity is extracted
  and available for bind variables. If no claims are present, requests proceed
  without HTTP 401 errors.
- **Dynamic Bind Parameters:** Any bind parameters referenced in `sessionContextBlock`
  or `sessionResetBlock` (such as `:user_id`, `:user_identity`, or other tool
  arguments) are resolved automatically. Values can be provided as tool input
  parameters or extracted from JWT claims.
- **Identity Extraction:** If using JWT claims, the caller's identity is extracted
  by default from the `email` claim (with an automatic fallback to `sub`). You can
  configure a custom claim key using `sessionContextClaim`.
- **Dedicated Connections:** When per-request session contexts are initialized,
  connection pooling should be disabled using `disablePooling: true` to guarantee
  connection isolation and prevent cross-session contamination.

## Example

This example demonstrates the four connection methods you could choose from:

```yaml
kind: source
name: my-oracle-source
type: oracle

# --- Choose one connection method ---
# 1. Host, Port, and Service Name
host: 127.0.0.1
port: 1521
serviceName: XEPDB1

# 2. Direct Connection String
connectionString: "127.0.0.1:1521/XEPDB1"

# 3. TNS Alias (requires tnsnames.ora)
tnsAlias: "MY_DB_ALIAS"
tnsAdmin: "/opt/oracle/network/admin" # Optional: overrides TNS_ADMIN env var

user: ${USER_NAME}
password: ${PASSWORD}

# Optional: Set to true to use the OCI-based driver for advanced features (Requires Oracle Instant Client)
```

### Using an Oracle Wallet

Oracle Wallet allows you to store credentails used for database connection. Depending whether you are using an OCI-based driver, the wallet configuration is different.

#### Pure Go Driver (`useOCI: false`) - Oracle Wallet

The `go-ora` driver uses the `walletLocation` field to connect to a database secured with an Oracle Wallet without standard username and password.

```yaml
kind: source
name: pure-go-wallet
type: oracle
connectionString: "127.0.0.1:1521/XEPDB1"
user: ${USER_NAME}
password: ${PASSWORD}
# The TNS Alias is often required to connect to a service registered in tnsnames.ora
tnsAlias: "SECURE_DB_ALIAS"
walletLocation: "/path/to/my/wallet/directory"
```

#### OCI-Based Driver (`useOCI: true`) - Oracle Wallet

For the OCI-based driver, wallet authentication is triggered by setting tnsAdmin to the wallet directory and connecting via a tnsAlias. 

```yaml
kind: source
name: oci-wallet
type: oracle
connectionString: "127.0.0.1:1521/XEPDB1"
user: ${USER_NAME}
password: ${PASSWORD}
tnsAlias: "WALLET_DB_ALIAS"
tnsAdmin: "/opt/oracle/wallet" # Directory containing tnsnames.ora, sqlnet.ora, and wallet files
useOCI: true
```

### Session Context Initialization with Dedicated Connection Isolation

This example demonstrates configuring an Oracle source with dedicated
connection isolation, PL/SQL session context setup, and session reset blocks.
This pattern works for Oracle Virtual Private Database (VPD), PeopleSoft session
initialization packages, or custom schema setup:

```yaml
kind: source
name: my-oracle-vpd-source
type: oracle
connectionString: "127.0.0.1:1521/XEPDB1"
user: ${USER_NAME}
password: ${PASSWORD}

# Dedicated unpooled connection: physical sockets terminate immediately upon closure
disablePooling: true

# PL/SQL session context setup executed on checkout before running queries
# Bind variables (such as :user_identity or :user_id) are automatically resolved
# from tool input parameters or caller JWT claims
sessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;"

# JWT claim used to extract caller identity (defaults to "email", falling back to "sub")
sessionContextClaim: "email"

# PL/SQL session context reset executed prior to closing the dedicated connection
sessionResetBlock: "BEGIN DBMS_SESSION.CLEAR_IDENTIFIER; END;"
```

For PeopleSoft session initialization (where `:user_id` is supplied as a tool parameter):

```yaml
kind: source
name: peoplesoft-oracle
type: oracle
connectionString: "127.0.0.1:1521/XEPDB1"
user: ${USER_NAME}
password: ${PASSWORD}
disablePooling: true
sessionContextBlock: |
  declare 
    l_ret varchar2(100);
  begin
    l_ret := sysadm.ps_security_pkg1.initialize_session(:user_id);
  end;
sessionResetBlock: "BEGIN DBMS_SESSION.CLEAR_IDENTIFIER; END;"
```

{{< notice tip >}}
Use environment variable replacement with the format ${ENV_NAME}
instead of hardcoding your secrets into the configuration file.
{{< /notice >}}

## Reference

| **field**             | **type** | **required** | **description**                                                                                                                                                                         |
|-----------------------|:--------:|:------------:|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| type                  |  string  |     true     | Must be "oracle".                                                                                                                                                                       |
| user                  |  string  |     true     | Name of the Oracle user to connect as (e.g. "my-oracle-user").                                                                                                                          |
| password              |  string  |     true     | Password of the Oracle user (e.g. "my-password").                                                                                                                                       |
| host                  |  string  |    false     | IP address or hostname to connect to (e.g. "127.0.0.1"). Required if not using `connectionString` or `tnsAlias`.                                                                        |
| port                  | integer  |    false     | Port to connect to (e.g. "1521"). Required if not using `connectionString` or `tnsAlias`.                                                                                               |
| serviceName           |  string  |    false     | The Oracle service name of the database to connect to. Required if not using `connectionString` or `tnsAlias`.                                                                          |
| connectionString      |  string  |    false     | A direct connection string (e.g. "hostname:port/servicename"). Use as an alternative to `host`, `port`, and `serviceName`.                                                              |
| tnsAlias              |  string  |    false     | A TNS alias from a `tnsnames.ora` file. Use as an alternative to `host`/`port` or `connectionString`.                                                                                   |
| tnsAdmin              |  string  |    false     | Path to the directory containing the `tnsnames.ora` file. This overrides the `TNS_ADMIN` environment variable if it is set.                                                             |
| useOCI                |   bool   |    false     | If true, uses the OCI-based driver (godror) which supports Oracle Wallet/Kerberos but requires the Oracle Instant Client libraries to be installed. Defaults to false (pure Go driver). |
| walletLocation        |  string  |    false     | Path to the directory containing the wallet files for the pure Go driver (`useOCI: false`).                                                                                             |
| disablePooling        |   bool   |    false     | If true, disables database connection pooling (`SetMaxIdleConns(0)` and `SetConnMaxLifetime(0)`), ensuring dedicated physical sockets terminate immediately upon connection closure.   |
| sessionContextBlock   |  string  |    false     | A PL/SQL or SQL statement to execute immediately on dedicated connection checkout before query execution. Bind variables (e.g. `:user_id`, `:user_identity`, or any tool parameters) are automatically resolved from tool input parameters or JWT claims. Authorization is optional. |
| sessionContextClaim   |  string  |    false     | The JWT claim from which to extract the caller's identity (defaults to `"email"` with fallback to `"sub"`). Used when binding `:user_identity` or `:user_id` if not provided via tool parameters. |
| sessionResetBlock     |  string  |    false     | A PL/SQL statement to execute immediately prior to closing the dedicated connection to clear session context. Executed with a detached 5-second timeout. Requires `sessionContextBlock` to be configured. Errors during reset execution are suppressed and do not fail the query. |

## Advanced Usage

### Oracle Session Context & Connection Isolation

Organizations often enforce fine-grained access control or tenant isolation
directly in Oracle Database—such as via Virtual Private Database (VPD) policies
inspecting `SYS_CONTEXT('USERENV', 'CLIENT_IDENTIFIER')`, PeopleSoft session
security packages (e.g. `sysadm.ps_security_pkg1.initialize_session(:user_id)`),
or session-level parameters.

#### Dedicated Connection Isolation (`disablePooling: true`)

In standard database applications, connection pools reuse physical database
connections across different requests to minimize connection establishment
overhead. In multi-tenant AI agent architectures with session-level contexts,
connection reuse poses a security risk: if a pooled connection is returned
without a complete context purge, a subsequent query from a different user could
inadvertently execute with residual session privileges.

Setting `disablePooling: true` disables connection pooling on the underlying
database connection handle by setting maximum idle connections to 0 and
connection lifetime to 0. Every query acquires a dedicated physical connection
that is closed and terminated immediately after execution.

#### Flexible Context & Parameter Binding

When `sessionContextBlock` is defined:

1. **Tool Parameter & Claim Resolution:** Any bind variables in `sessionContextBlock`
   and `sessionResetBlock` (e.g., `:user_id`, `:user_identity`, or other parameter
   names) are matched against tool invocation parameters or extracted JWT identity
   claims.
2. **Optional Authorization:** Authorization is not mandatory. Unauthenticated
   tool calls proceed normally; if an identity claim is configured and present, it is
   bound to the identity placeholders. Tool parameters (such as `user_id`) take
   precedence when provided.
3. **Static Session Setup:** If the statement contains no bind parameters (for example,
   `ALTER SESSION SET CURRENT_SCHEMA = HR`), it is executed without bind arguments.

#### Dedicated Execution Pipeline

When a request executes against an Oracle source with `sessionContextBlock`:

1. **Dedicated Checkout:** A dedicated `*sql.Conn` is acquired from the database pool.
2. **Context Injection:** The `sessionContextBlock` PL/SQL statement is executed
   on the connection with resolved bind parameters. This binding is supported
   identically across both pure Go (`go-ora`) and OCI-based (`godror`) drivers.
3. **Autocommit Execution:** The user query or DML operation executes directly on
   the dedicated connection in autocommit mode without wrapping in an explicit
   transaction (`*sql.Tx`), ensuring transactional isolation and query safety.
4. **Deferred Teardown & Reset:** When execution completes (even in the event of
   a query error or panic), the `sessionResetBlock` (if configured) executes using
   a detached 5-second context timeout to clear session state, followed by immediate
   connection closure and physical socket destruction. Errors occurring during
   reset execution are suppressed to ensure cleanup completes and the original
   query outcome is returned.

