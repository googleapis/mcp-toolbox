---
title: "SQL Server Source"
linkTitle: "Source"
type: docs
weight: 1
description: >
  SQL Server is a relational database management system (RDBMS).
no_list: true
---

## About

[SQL Server][mssql-docs] is a relational database management system (RDBMS)
developed by Microsoft that allows users to store, retrieve, and manage large
amount of data through a structured format.

[mssql-docs]: https://www.microsoft.com/en-us/sql-server



## Available Tools

{{< list-tools >}}

## Requirements

### Database User

This source uses standard authentication by default. You will need to [create a
SQL Server user][mssql-users] to login to the database with.

For an Azure SQL server with SQL authentication disabled, set `azureAuth` instead and
omit `user` and `password`. The identity you authenticate as still needs a database
user, created [from the external provider][mssql-entra-users].

To have each request run under the identity of the person who made it instead of
a shared one, see [Authenticating as the caller](#authenticating-as-the-caller).

[mssql-users]:
    https://learn.microsoft.com/en-us/sql/relational-databases/security/authentication-access/create-a-database-user?view=sql-server-ver16
[mssql-entra-users]:
    https://learn.microsoft.com/en-us/azure/azure-sql/database/authentication-aad-configure#create-contained-database-users-in-your-database-mapped-to-microsoft-entra-identities

## Example

```yaml
kind: source
name: my-mssql-source
type: mssql
host: 127.0.0.1
port: 1433
database: my_db
user: ${USER_NAME}
password: ${PASSWORD}
# encrypt: strict
```

### Microsoft Entra ID

```yaml
kind: source
name: my-azure-sql-source
type: mssql
host: my-server.database.windows.net
port: 1433
database: my_db
encrypt: strict
azureAuth:
  mode: workload-identity
  clientId: ${AZURE_CLIENT_ID}
```

{{< notice tip >}}
Use environment variable replacement with the format ${ENV_NAME}
instead of hardcoding your secrets into the configuration file.
{{< /notice >}}

## Reference

| **field**       | **type** | **required** | **description**                                                                                                                                                                                                                                                          |
|-----------------|:--------:|:------------:|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| type            |  string  |     true     | Must be "mssql".                                                                                                                                                                                                                                                         |
| host            |  string  |     true     | IP address to connect to (e.g. "127.0.0.1").                                                                                                                                                                                                                             |
| port            |  string  |     true     | Port to connect to (e.g. "1433").                                                                                                                                                                                                                                        |
| database        |  string  |     true     | Name of the SQL Server database to connect to (e.g. "my_db").                                                                                                                                                                                                            |
| user            |  string  |    true\*    | Name of the SQL Server user to connect as (e.g. "my-user"). Must be omitted when `azureAuth` or `useClientOAuth` is set.                                                                                                                                                 |
| password        |  string  |    true\*    | Password of the SQL Server user (e.g. "my-password"). Must be omitted when `useClientOAuth` is set. Not required when `azureAuth` is set; in `service-principal` mode it carries the client secret.                                                                     |
| encrypt         |  string  |    false     | Encryption level for data transmitted between the client and server (e.g., "strict"). If not specified, defaults to the [github.com/microsoft/go-mssqldb](https://github.com/microsoft/go-mssqldb?tab=readme-ov-file#common-parameters) package's default encrypt value. |
| azureAuth       |  object  |    false     | Authenticate to Azure SQL with a Microsoft Entra ID identity instead of a SQL login. Omit for standard authentication.                                                                                                                                                    |
| useClientOAuth  |  string  |    false     | Authenticate as the caller rather than as a configured identity. "true" reads the token from the `Authorization` header; any other non-empty value names the header to read instead. See [Authenticating as the caller](#authenticating-as-the-caller).                 |
| azureOnBehalfOf |  object  |    false     | Exchanges the caller's token for one Azure SQL will accept. Fields `clientId`, `clientSecret` and `tenantId`, all required. Only used with `useClientOAuth`.                                                                                                              |

### azureAuth

| **field**                  | **type** | **required** | **description**                                                                                                                                    |
|----------------------------|:--------:|:------------:|----------------------------------------------------------------------------------------------------------------------------------------------------|
| mode                       |  string  |     true     | One of `default`, `workload-identity`, `managed-identity` or `service-principal`. Selects the credential the [azuread][azuread-pkg] driver uses.    |
| clientId                   |  string  |    false     | Client ID of the identity. Set it where a host maps to more than one identity, so the credential is not left to be guessed. Required together with `tenantId`, and always in `service-principal` mode.                         |
| tenantId                   |  string  |    false     | Tenant ID, where it cannot be inferred from the server.                                                                                            |
| disableInstanceDiscovery   |   bool   |    false     | Skip the authority metadata request, which network-isolated and sovereign-cloud deployments cannot reach.                                           |
| additionallyAllowedTenants | []string |    false     | Accept tokens from tenants beyond the configured one, for multi-tenant credentials.                                                                 |

[azuread-pkg]: https://pkg.go.dev/github.com/microsoft/go-mssqldb/azuread

## Advanced Usage

### Authenticating as the caller

By default every tool call reaches the database over one connection pool, under
the identity in the configuration. Setting `useClientOAuth` changes that: Toolbox
takes the access token the client sent, connects as that person, and keeps a
separate pool per caller. The database then applies their own permissions and
row-level security, and refuses what they may not read — enforcement that does
not depend on the tool, the prompt or the model.

This requires a SQL Server that accepts Microsoft Entra ID authentication, such
as [Azure SQL Database or Azure SQL Managed Instance][azure-sql-entra], and a
database user (or Entra group) for each person who will query it.

[azure-sql-entra]:
    https://learn.microsoft.com/en-us/azure/azure-sql/database/authentication-aad-overview

Which token the database is given depends on who the client's token was issued
for:

- **The client already holds a token for the database.** Omit `azureOnBehalfOf`
  and the token is passed on as it arrived.
- **The client authenticated against Toolbox** — the usual case, since an MCP
  client requests a token for the server it is talking to. Configure
  `azureOnBehalfOf` and the driver exchanges that token for a database-scoped one
  using the [on-behalf-of flow][obo-flow]. The application it names must hold the
  delegated permission `user_impersonation` on Azure SQL Database, with consent
  granted, and the client's token must have been issued for that application.

[obo-flow]:
    https://learn.microsoft.com/en-us/entra/identity-platform/v2-oauth2-on-behalf-of-flow

```yaml
kind: source
name: my-mssql-source
type: mssql
host: my-server.database.windows.net
port: 1433
database: my_db
# No user or password: the identity comes from whoever called the tool.
useClientOAuth: "true"
azureOnBehalfOf:
    clientId: ${AZURE_CLIENT_ID}
    clientSecret: ${AZURE_CLIENT_SECRET}
    tenantId: ${AZURE_TENANT_ID}
```

{{< notice note >}}
A request that arrives without a token is refused rather than falling back to a
shared identity. Because there is no identity of its own to connect with, this
source does not verify the connection at startup: the first request carrying a
token is what proves the database is reachable.
{{< /notice >}}
