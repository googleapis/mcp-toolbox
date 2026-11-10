---
title: "Cloud SQL for PostgreSQL Source"
linkTitle: "Source"
type: docs
weight: 1
description: >
  Cloud SQL for PostgreSQL is a fully-managed database service for Postgres.
no_list: true
---

## About

[Cloud SQL for PostgreSQL][csql-pg-docs] is a fully-managed database service
that helps you set up, maintain, manage, and administer your PostgreSQL
relational databases on Google Cloud Platform.

If you are new to Cloud SQL for PostgreSQL, you can try [creating and connecting
to a database by following these instructions][csql-pg-quickstart].

[csql-pg-docs]: https://cloud.google.com/sql/docs/postgres
[csql-pg-quickstart]:
    https://cloud.google.com/sql/docs/postgres/connect-instance-local-computer

## Available Tools

{{< list-tools dirs="/integrations/postgres/tools" >}}

### Pre-built Configurations

- [Cloud SQL for Postgres using
  MCP](https://mcp-toolbox.dev/documentation/connect-to/ides/cloud_sql_pg_mcp/)
Connect your IDE to Cloud SQL for Postgres using Toolbox.

## Requirements

### IAM Permissions

By default, this source uses the [Cloud SQL Go Connector][csql-go-conn] to
authorize and establish mTLS connections to your Cloud SQL instance. The Go
connector uses your [Application Default Credentials (ADC)][adc] to authorize
your connection to Cloud SQL.

In addition to [setting the ADC for your server][set-adc], you need to ensure
the IAM identity has been given the following IAM roles (or corresponding
permissions):

- `roles/cloudsql.client`

{{< notice tip >}}
If you are connecting from Compute Engine, make sure your VM
also has the [proper
scope](https://cloud.google.com/compute/docs/access/service-accounts#accesscopesiam)
to connect using the Cloud SQL Admin API.
{{< /notice >}}

[csql-go-conn]: <https://github.com/GoogleCloudPlatform/cloud-sql-go-connector>
[adc]: <https://cloud.google.com/docs/authentication#adc>
[set-adc]: <https://cloud.google.com/docs/authentication/provide-credentials-adc>

### Networking

Cloud SQL supports connecting over both from external networks via the internet
([public IP][public-ip]), and internal networks ([private IP][private-ip]).
For more information on choosing between the two options, see the Cloud SQL page
[Connection overview][conn-overview].

You can configure the `ipType` parameter in your source configuration to
`public` or `private` to match your cluster's configuration. Regardless of which
you choose, all connections use IAM-based authorization and are encrypted with
mTLS.

[private-ip]: https://cloud.google.com/sql/docs/postgres/configure-private-ip
[public-ip]: https://cloud.google.com/sql/docs/postgres/configure-ip
[conn-overview]: https://cloud.google.com/sql/docs/postgres/connect-overview

### Authentication

This source supports both password-based authentication and IAM
authentication (using your [Application Default Credentials][adc]).

#### Standard Authentication

To connect using user/password, [create
a PostgreSQL user][cloudsql-users] and input your credentials in the `user` and
`password` fields.

```yaml
user: ${USER_NAME}
password: ${PASSWORD}
```

#### IAM Authentication

To connect using IAM authentication:

1. Prepare your database instance and user following this [guide][iam-guide].
2. You could choose one of the two ways to log in:
    - Specify your IAM email as the `user`.
    - Leave your `user` field blank. Toolbox will fetch the [ADC][adc]
      automatically and log in using the email associated with it.

3. Leave the `password` field blank.

[iam-guide]: https://cloud.google.com/sql/docs/postgres/iam-logins
[cloudsql-users]: https://cloud.google.com/sql/docs/postgres/create-manage-users

## Example

```yaml
kind: source
name: my-cloud-sql-pg-source
type: cloud-sql-postgres
project: my-project-id
region: us-central1
instance: my-instance
database: my_db
user: ${USER_NAME}
password: ${PASSWORD}
# ipType: "private"
```

{{< notice tip >}}
Use environment variable replacement with the format ${ENV_NAME}
instead of hardcoding your secrets into the configuration file.
{{< /notice >}}

### Managed Connection Pooling

Toolbox automatically supports [Managed Connection Pooling][csql-mcp]. If your Cloud SQL for PostgreSQL instance has Managed Connection Pooling enabled, the connection will immediately benefit from increased throughput and reduced latency.

The interface is identical, so there's no additional configuration required on the client. For more information on configuring your instance, see the [Cloud SQL Managed Connection Pooling documentation][csql-mcp-docs].

[csql-mcp]: https://docs.cloud.google.com/sql/docs/postgres/managed-connection-pooling
[csql-mcp-docs]: https://docs.cloud.google.com/sql/docs/postgres/configure-mcp

## Reference

| **field** | **type** | **required** | **description**                                                                                                          |
|-----------|:--------:|:------------:|--------------------------------------------------------------------------------------------------------------------------|
| type      |  string  |     true     | Must be "cloud-sql-postgres".                                                                                            |
| project   |  string  |     true     | Id of the GCP project that the cluster was created in (e.g. "my-project-id").                                            |
| region    |  string  |     true     | Name of the GCP region that the cluster was created in (e.g. "us-central1").                                             |
| instance  |  string  |     true     | Name of the Cloud SQL instance within the cluster (e.g. "my-instance").                                                  |
| database  |  string  |     true     | Name of the Postgres database to connect to (e.g. "my_db").                                                              |
| user      |  string  |    false     | Name of the Postgres user to connect as (e.g. "my-pg-user"). Defaults to IAM auth using [ADC][adc] email if unspecified. |
| password  |  string  |    false     | Password of the Postgres user (e.g. "my-password"). Defaults to attempting IAM authentication if unspecified.            |
| ipType    |  string  |    false     | IP Type of the Cloud SQL instance; must be one of `public`, `private`, or `psc`. Default: `public`.                      |
| sqlCommenter | boolean |  false     | Overrides the global `--sql-commenter` flag for this source. When set, it takes priority; when omitted, the global flag applies. |
| readOnly  | boolean  |    false     | When set to `true`, enforces read-only execution at the database session level (`cloudsql_session_read_only=locked`) and suppresses write-capable tools. Default: `false`. |
