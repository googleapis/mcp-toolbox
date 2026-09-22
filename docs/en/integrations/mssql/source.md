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

| **field** | **type** | **required** | **description**                                                                                                                                                                                                                                                          |
|-----------|:--------:|:------------:|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| type      |  string  |     true     | Must be "mssql".                                                                                                                                                                                                                                                         |
| host      |  string  |     true     | IP address to connect to (e.g. "127.0.0.1").                                                                                                                                                                                                                             |
| port      |  string  |     true     | Port to connect to (e.g. "1433").                                                                                                                                                                                                                                        |
| database  |  string  |     true     | Name of the SQL Server database to connect to (e.g. "my_db").                                                                                                                                                                                                            |
| user      |  string  |    true\*    | Name of the SQL Server user to connect as (e.g. "my-user"). Must be omitted when `azureAuth` is set.                                                                                                                                                                          |
| password  |  string  |    true\*    | Password of the SQL Server user (e.g. "my-password"). Not required when `azureAuth` is set; in `service-principal` mode it carries the client secret.                                                                                                                      |
| encrypt   |  string  |    false     | Encryption level for data transmitted between the client and server (e.g., "strict"). If not specified, defaults to the [github.com/microsoft/go-mssqldb](https://github.com/microsoft/go-mssqldb?tab=readme-ov-file#common-parameters) package's default encrypt value. |
| azureAuth |  object  |    false     | Authenticate to Azure SQL with a Microsoft Entra ID identity instead of a SQL login. Omit for standard authentication.                                                                                                                                                    |

### azureAuth

| **field**                  | **type** | **required** | **description**                                                                                                                                    |
|----------------------------|:--------:|:------------:|----------------------------------------------------------------------------------------------------------------------------------------------------|
| mode                       |  string  |     true     | One of `default`, `workload-identity`, `managed-identity` or `service-principal`. Selects the credential the [azuread][azuread-pkg] driver uses.    |
| clientId                   |  string  |    false     | Client ID of the identity. Set it where a host maps to more than one identity, so the credential is not left to be guessed. Required together with `tenantId`, and always in `service-principal` mode.                         |
| tenantId                   |  string  |    false     | Tenant ID, where it cannot be inferred from the server.                                                                                            |

[azuread-pkg]: https://pkg.go.dev/github.com/microsoft/go-mssqldb/azuread
