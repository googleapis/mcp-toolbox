---
title: "Spanner Source"
linkTitle: "Source"
type: docs
weight: 1
description: >
  Spanner is a fully managed database service from Google Cloud that combines 
  relational, key-value, graph, and search capabilities.
no_list: true
---

## About

[Spanner][spanner-docs] is a fully managed, mission-critical database service
that brings together relational, graph, key-value, and search. It offers
transactional consistency at global scale, automatic, synchronous replication
for high availability, and support for two SQL dialects: GoogleSQL (ANSI 2011
with extensions) and PostgreSQL.

If you are new to Spanner, you can try to [create and query a database using
the Google Cloud console][spanner-quickstart].

[spanner-docs]: https://cloud.google.com/spanner/docs
[spanner-quickstart]:
    https://cloud.google.com/spanner/docs/create-query-database-console



## Available Tools

{{< list-tools >}}

### Pre-built Configurations

- [Spanner using MCP](../../documentation/connect-to/ides/spanner_mcp.md)
Connect your IDE to Spanner using Toolbox.

## Requirements

### IAM Permissions

Spanner uses [Identity and Access Management (IAM)][iam-overview] to control
user and group access to Spanner resources at the project, Spanner instance, and
Spanner database levels. Toolbox will use your [Application Default Credentials
(ADC)][adc] to authorize and authenticate when interacting with Spanner.

In addition to [setting the ADC for your server][set-adc], you need to ensure
the IAM identity has been given the correct IAM permissions for the query
provided. See [Apply IAM roles][grant-permissions] for more information on
applying IAM permissions and roles to an identity.

[iam-overview]: https://cloud.google.com/spanner/docs/iam
[adc]: https://cloud.google.com/docs/authentication#adc
[set-adc]: https://cloud.google.com/docs/authentication/provide-credentials-adc
[grant-permissions]: https://cloud.google.com/spanner/docs/grant-permissions

## Example

```yaml
kind: source
name: my-spanner-source
type: "spanner"
project: "my-project-id"
instance: "my-instance"
database: "my_db"
# dialect: "googlesql"
```

## Reference

| **field** | **type** | **required** | **description**                                                                                                     |
|-----------|:--------:|:------------:|---------------------------------------------------------------------------------------------------------------------|
| type      |  string  |     true     | Must be "spanner".                                                                                                  |
| project   |  string  |     true     | Id of the GCP project that the cluster was created in (e.g. "my-project-id").                                       |
| instance  |  string  |     true     | Name of the Spanner instance.                                                                                       |
| database  |  string  |     true     | Name of the database on the Spanner instance                                                                        |
| dialect   |  string  |    false     | Name of the dialect type of the Spanner database, must be either `googlesql` or `postgresql`. Default: `googlesql`. |
| readOnly  | boolean  |    false     | When set to `true`, enforces read-only execution at the database protocol level and suppresses write-capable tools. Default: `false`. |
