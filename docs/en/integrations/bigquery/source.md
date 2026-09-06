---
title: "BigQuery Source"
type: docs
linkTitle: "Source"
weight: 1
description: >
  BigQuery is Google Cloud's fully managed, petabyte-scale, and cost-effective
  analytics data warehouse that lets you run analytics over vast amounts of
  data in near real time. With BigQuery, there's no infrastructure to set
  up or manage, letting you focus on finding meaningful insights using
  GoogleSQL and taking advantage of flexible pricing models across on-demand
  and flat-rate options.
no_list: true
---

## About

[BigQuery][bigquery-docs] is Google Cloud's fully managed, petabyte-scale,
and cost-effective analytics data warehouse that lets you run analytics
over vast amounts of data in near real time. With BigQuery, there's no
infrastructure to set up or manage, letting you focus on finding meaningful
insights using GoogleSQL and taking advantage of flexible pricing models
across on-demand and flat-rate options.

If you are new to BigQuery, you can try to
[load and query data with the bq tool][bigquery-quickstart-cli].

BigQuery uses [GoogleSQL][bigquery-googlesql] for querying data. GoogleSQL
is an ANSI-compliant structured query language (SQL) that is also implemented
for other Google Cloud services. SQL queries are handled by cluster nodes
in the same way as NoSQL data requests. Therefore, the same best practices
apply when creating SQL queries to run against your BigQuery data, such as
avoiding full table scans or complex filters.

[bigquery-docs]: https://cloud.google.com/bigquery/docs
[bigquery-quickstart-cli]:
    https://cloud.google.com/bigquery/docs/quickstarts/quickstart-command-line
[bigquery-googlesql]:
    https://cloud.google.com/bigquery/docs/reference/standard-sql/

## Available Tools

{{< list-tools >}}

### Pre-built Configurations

- [BigQuery using
  MCP](https://mcp-toolbox.dev/documentation/connect-to/ides/bigquery_mcp/)
  Connect your IDE to BigQuery using Toolbox.

## Requirements

### IAM Permissions

BigQuery uses [Identity and Access Management (IAM)][iam-overview] to control
user and group access to BigQuery resources like projects, datasets, and tables.

### Authentication via Application Default Credentials (ADC)

By **default**, Toolbox will use your [Application Default Credentials
(ADC)][adc] to authorize and authenticate when interacting with
[BigQuery][bigquery-docs].

When using this method, you need to ensure the IAM identity associated with your
ADC (such as a service account) has the correct permissions for the queries you
intend to run. Common roles include `roles/bigquery.user` (which includes
permissions to run jobs and read data) or `roles/bigquery.dataViewer`.
Follow this [guide][set-adc] to set up your ADC.

If you are running on Google Compute Engine (GCE) or Google Kubernetes Engine
(GKE), you might need to explicitly set the access scopes for the service
account. While you can configure scopes when creating the VM or node pool, you
can also specify them in the source configuration using the `scopes` field.
Common scopes include `https://www.googleapis.com/auth/bigquery` or
`https://www.googleapis.com/auth/cloud-platform`.

### Authentication via User's OAuth Access Token

If the `useClientOAuth` parameter is set to `true`, Toolbox will instead use the
OAuth access token for authentication. By default, this token is parsed from the
`Authorization` header passed in with the tool invocation request.

If you need to use a non-standard header for the access token (e.g., to avoid
conflicts with other services like Cloud Run), you can specify the header name
in the `useClientOAuth` field (e.g., `useClientOAuth: X-BigQuery-Auth`).

This method allows Toolbox to make queries to [BigQuery][bigquery-docs] on behalf
of the client or the end-user.

When using this on-behalf-of authentication, you must ensure that the
identity used has been granted the correct IAM permissions.

[iam-overview]: <https://cloud.google.com/bigquery/docs/access-control>
[adc]: <https://cloud.google.com/docs/authentication#adc>
[set-adc]: <https://cloud.google.com/docs/authentication/provide-credentials-adc>

## Example

Initialize a BigQuery source that uses ADC:

```yaml
kind: source
name: my-bigquery-source
type: "bigquery"
project: "my-project-id"
# location: "US" # Optional: Specifies the location for query jobs.
# readOnly: false # Optional: Enforces read-only mode across all tools (defaults writeMode to "blocked").
# writeMode: "allowed" # One of: allowed, blocked, protected. Defaults to "allowed".
# allowedDatasets: # Optional: Restricts tool access to a specific list of datasets.
#   - "my_dataset_1"
#   - "other_project.my_dataset_2"
# impersonateServiceAccount: "service-account@project-id.iam.gserviceaccount.com" # Optional: Service account to impersonate
# scopes: # Optional: List of OAuth scopes to request.
#   - "https://www.googleapis.com/auth/bigquery"
#   - "https://www.googleapis.com/auth/drive.readonly"
# maxQueryResultRows: 50 # Optional: Limits the number of rows returned by queries. Defaults to 50.
# maximumBytesBilled: 10737418240 # Optional: Per-query bytes scanned cap (in bytes).
# apiEndpoint: "http://localhost:9050" # Optional: Override the BigQuery API endpoint (proxy or local emulator).
```

Initialize a BigQuery source that uses the client's access token:

```yaml
kind: source
name: my-bigquery-client-auth-source
type: "bigquery"
project: "my-project-id"
useClientOAuth: true
# location: "US" # Optional: Specifies the location for query jobs.
# readOnly: false # Optional: Enforces read-only mode across all tools (defaults writeMode to "blocked").
# writeMode: "allowed" # One of: allowed, blocked, protected. Defaults to "allowed".
# allowedDatasets: # Optional: Restricts tool access to a specific list of datasets.
#   - "my_dataset_1"
#   - "other_project.my_dataset_2"
# impersonateServiceAccount: "service-account@project-id.iam.gserviceaccount.com" # Optional: Service account to impersonate
# scopes: # Optional: List of OAuth scopes to request.
#   - "https://www.googleapis.com/auth/bigquery"
#   - "https://www.googleapis.com/auth/drive.readonly"
# maxQueryResultRows: 50 # Optional: Limits the number of rows returned by queries. Defaults to 50.
# maximumBytesBilled: 10737418240 # Optional: Per-query bytes scanned cap (in bytes).
# apiEndpoint: "http://localhost:9050" # Optional: Override the BigQuery API endpoint (proxy or local emulator).
```

## Reference

| **field**                 | **type** | **required** | **description**                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
|---------------------------|:--------:|:------------:|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| type                      |  string  |     true     | Must be "bigquery".                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| project                   |  string  |     true     | Id of the Google Cloud project to use for billing and as the default project for BigQuery resources.                                                                                                                                                                                                                                                                                                                                                                                                                |
| location                  |  string  |    false     | Specifies the location (e.g., 'us', 'asia-northeast1') in which to run the query job. This location must match the location of any tables referenced in the query. Defaults to the table's location or 'US' if the location cannot be determined. [Learn More](https://cloud.google.com/bigquery/docs/locations)                                                                                                                                                                                                    |
| readOnly                  | boolean  |    false     | Controls whether the source operates in read-only mode across Toolbox. Setting `readOnly: true` marks the source as read-only, coordinating with MCP read-only annotations and tool suppression, and defaults `writeMode` to `blocked` (or allows `protected`). If `writeMode` is also provided, it must be consistent with `readOnly` (`readOnly: true` with `blocked` or `protected`, `readOnly: false` with `allowed`), otherwise a startup configuration error is returned. |
| writeMode                 |  string  |    false     | Controls the write behavior for tools. `allowed` (default): All queries are permitted. `blocked`: Enforces strict read-only mode, coordinating with MCP read-only annotations and tool suppression (suppresses write-capable tools during registration, while dynamic tools like `bigquery-execute-sql` dynamically report `readOnlyHint: true`), and allows only `SELECT` statements for execution. `protected`: Enables session-based execution where all tools associated with this source instance share the same [BigQuery session](https://cloud.google.com/bigquery/docs/sessions-intro). This allows for stateful operations using temporary tables (e.g., `CREATE TEMP TABLE`), while protecting permanent datasets from modification (also classified as read-only at the Toolbox layer). For `bigquery-execute-sql`, `SELECT` statements can be used on all tables, but write operations are restricted to the session's temporary dataset. For tools like `bigquery-sql`, `bigquery-forecast`, and `bigquery-analyze-contribution`, the `writeMode` restrictions do not apply, but they will operate within the shared session. **Note:** The `protected` mode cannot be used with `useClientOAuth: true`. It is also not recommended for multi-user server environments, as all users would share the same session. A session is terminated automatically after 24 hours of inactivity or after 7 days, whichever comes first. A new session is created on the next request, and any temporary data from the previous session will be lost. |
| allowedDatasets           | []string |    false     | An optional list of dataset IDs that tools using this source are allowed to access. If provided, any tool operation attempting to access a dataset not in this list will be rejected. To enforce this, two types of operations are also disallowed: 1) Dataset-level operations (e.g., `CREATE SCHEMA`), and 2) operations where table access cannot be statically analyzed (e.g., `EXECUTE IMMEDIATE`, `CREATE PROCEDURE`). If a single dataset is provided, it will be treated as the default for prebuilt tools. |
| useClientOAuth            |  string  |    false     | If set to `'true'`, forwards the client's OAuth access token from the default `Authorization` header. If set to a custom header name (e.g., `X-My-Auth`), that header will be used instead. An empty string or `'false'` disables this feature. Defaults to `""` (disabled). |
| scopes                    | []string |    false     | A list of OAuth 2.0 scopes to use for the credentials. If not provided, default scopes are used.                                                                                                                                                                                                                                                                                                                                                                                                                     |
| impersonateServiceAccount |  string  |    false     | Service account email to impersonate when making BigQuery and Dataplex API calls. The authenticated principal must have the `roles/iam.serviceAccountTokenCreator` role on the target service account. [Learn More](https://cloud.google.com/iam/docs/service-account-impersonation)                                                                                                                                                                                                                                |
| maxQueryResultRows             |   int    |    false     | The maximum number of rows to return from a query. Defaults to 50. |
| maximumBytesBilled             |  int64   |    false     | The maximum bytes billed per query. When set, queries that exceed this limit fail before executing. |
| apiEndpoint                    |  string  |    false     | Override the BigQuery API endpoint. |
| sqlCommenter                   | boolean  |    false     | Overrides the global `--sql-commenter` flag for this source. When set, it takes priority; when omitted, the global flag applies. See [SQL Commenter: Job Labels](#sql-commenter-job-labels). |

## Advanced Usage

### SQL Commenter: Job Labels

BigQuery does not retain SQL-text comments in its logs, so instead of
prepending a comment, [SQL Commenter](../../documentation/monitoring/sql_commenter.md)
attaches the same attributes as native
[job labels](https://cloud.google.com/bigquery/docs/adding-labels#job-label)
on every query job executed by the `bigquery-execute-sql` and `bigquery-sql`
tools. Labels appear in `INFORMATION_SCHEMA.JOBS`, the Jobs API, audit logs,
and billing exports — no query text parsing is needed to recover them.

Attribute names and values are sanitized to satisfy BigQuery label
constraints (lowercase letters, digits, underscores, and dashes; at most 63
characters): dots in attribute names become underscores (`tool.name` →
`tool_name`), and any other disallowed character in a value is replaced with
an underscore (`genai-toolbox/1.1.0` → `genai-toolbox_1_1_0`). Labels set
explicitly by tools (such as `mcp-toolbox-tool`) always take precedence over
SQLCommenter attributes on key collisions. BigQuery's limit of 64 labels per
job is left to the API to enforce: a job that would exceed it fails with a
clear error rather than silently dropping telemetry.

For example, to see per-tool bytes billed for agent-issued queries:

```sql
SELECT
  (SELECT value FROM UNNEST(labels) WHERE key = 'tool_name') AS tool_name,
  (SELECT value FROM UNNEST(labels) WHERE key = 'client') AS client,
  SUM(total_bytes_billed) AS total_bytes_billed
FROM `region-us`.INFORMATION_SCHEMA.JOBS
WHERE EXISTS (SELECT 1 FROM UNNEST(labels) WHERE key = 'tool_name')
GROUP BY tool_name, client
ORDER BY total_bytes_billed DESC;
```
