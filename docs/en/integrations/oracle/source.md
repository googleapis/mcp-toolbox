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

### Read-Only Access

A tool's `readOnly: true` setting runs its statement in an Oracle read-only
transaction, so writes and locking reads fail with `ORA-01456`. It does not stop
DDL: Oracle commits implicitly before and after every DDL statement, so `CREATE`,
`TRUNCATE` and `ALTER` still take effect, and a statement that performs DDL
before writing leaves the read-only transaction entirely.

Where a source backs tools that take untrusted or model-generated SQL, such as
`oracle-execute-sql`, enforce read-only in the database instead. On Oracle
Database 23ai and later, mark the connecting user read-only:

```sql
CREATE USER toolbox_ro IDENTIFIED BY "<password>" QUOTA 0 ON users READ ONLY;
GRANT CREATE SESSION TO toolbox_ro;
GRANT READ ON app.orders TO toolbox_ro;
```

Every write from that user's sessions then fails with `ORA-28194: Can perform
read operations only` — DDL, DML, `SELECT ... FOR UPDATE`, and writes inside
PL/SQL blocks, including autonomous transactions and definer's-rights
procedures. `ALTER USER toolbox_ro READ WRITE` reverses it.

The attribute applies to that user's sessions on this database, so it does not
constrain a procedure invoked over a database link, which runs on the target
database as the link's user. Grant `EXECUTE` on remote procedures and
database-link access sparingly.

Before 23ai there is no equivalent, so grant only `CREATE SESSION` plus the
specific `READ` object privileges the user needs, and no `RESOURCE`, `DBA`, or
`ANY` privileges.

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

{{< notice tip >}}
Use environment variable replacement with the format ${ENV_NAME}
instead of hardcoding your secrets into the configuration file.
{{< /notice >}}

## Reference

| **field**        | **type** | **required** | **description**                                                                                                                                                                         |
|------------------|:--------:|:------------:|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| type             |  string  |     true     | Must be "oracle".                                                                                                                                                                       |
| user             |  string  |     true     | Name of the Oracle user to connect as (e.g. "my-oracle-user").                                                                                                                          |
| password         |  string  |     true     | Password of the Oracle user (e.g. "my-password").                                                                                                                                       |
| host             |  string  |    false     | IP address or hostname to connect to (e.g. "127.0.0.1"). Required if not using `connectionString` or `tnsAlias`.                                                                        |
| port             | integer  |    false     | Port to connect to (e.g. "1521"). Required if not using `connectionString` or `tnsAlias`.                                                                                               |
| serviceName      |  string  |    false     | The Oracle service name of the database to connect to. Required if not using `connectionString` or `tnsAlias`.                                                                          |
| connectionString |  string  |    false     | A direct connection string (e.g. "hostname:port/servicename"). Use as an alternative to `host`, `port`, and `serviceName`.                                                              |
| tnsAlias         |  string  |    false     | A TNS alias from a `tnsnames.ora` file. Use as an alternative to `host`/`port` or `connectionString`.                                                                                   |
| tnsAdmin         |  string  |    false     | Path to the directory containing the `tnsnames.ora` file. This overrides the `TNS_ADMIN` environment variable if it is set.                                                             |
| useOCI           |   bool   |    false     | If true, uses the OCI-based driver (godror) which supports Oracle Wallet/Kerberos but requires the Oracle Instant Client libraries to be installed. Defaults to false (pure Go driver). |
| walletLocation   |  string  |    false     | Path to the directory containing the wallet files for the pure Go driver (`useOCI: false`). |
