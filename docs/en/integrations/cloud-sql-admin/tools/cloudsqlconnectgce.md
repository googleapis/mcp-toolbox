---
title: cloud-sql-connect-gce
type: docs
weight: 12
description: Connect a Cloud SQL instance (PostgreSQL, MySQL, or SQL Server) to a Google Compute Engine VM. The engine is auto-detected from the instance.
---

## About

The `cloud-sql-connect-gce` tool helps an agent (or user) wire up any Cloud
SQL instance to a Google Compute Engine VM. The engine (PostgreSQL, MySQL,
or SQL Server) is auto-detected from the `DatabaseVersion` returned by the
Cloud SQL Admin API, so the caller does not have to specify it.

The tool:

1. Reads the Cloud SQL instance via the `cloud-sql-admin` source and the
   target GCE VM via the Compute Engine API (auto-discovering the zone
   with an `AggregatedList` filter when `vm_zone` is omitted).
2. Validates network reachability between the instance and the VM
   (same-VPC, private-IP, public-IP posture, SSL requirements).
3. Recommends a primary connection method (Cloud SQL Auth Proxy, Cloud SQL
   Connector library, or Direct Private IP) plus ranked alternatives.
4. Returns ready-to-paste setup steps, environment/DSN/JDBC snippets, and,
   when `language` is set, a Python / Node.js / Java / Go code snippet
   with floor-pinned dependency versions.

### Identity boundary

Both the Cloud SQL Admin call and the Compute Engine call honor the
caller's OAuth access token when the `cloud-sql-admin` source is
configured with `useClientOAuth: true`, so IAM decisions on both APIs are
evaluated as the caller. When no caller token is passed, the Compute
Engine call falls back to Application Default Credentials.

### Input validation

Caller-supplied identifiers that flow into shell commands, DSN/JDBC
templates, or the GCE list filter (`instance_connection_name`, `vm_name`,
`vm_zone`, `database_name`) are validated against GCP naming rules via
regex whitelists before use.

## Compatible Sources

{{< compatible-sources >}}

## Requirements

- The identity used for the tool call (caller OAuth or the Toolbox
  service account, per source configuration) needs `roles/cloudsql.client`
  on the Cloud SQL instance and `roles/compute.viewer` on the project.
- The Cloud SQL Admin API (`sqladmin.googleapis.com`) and Compute Engine
  API (`compute.googleapis.com`) must be enabled.

## Parameters

| **parameter**            | **type** | **required** | **description**                                                                                                                          |
| ------------------------ | :------: | :----------: | ---------------------------------------------------------------------------------------------------------------------------------------- |
| instance_connection_name |  string  |     true     | Cloud SQL instance connection name in the format `project:region:instance`.                                                              |
| vm_name                  |  string  |     true     | Name of the GCE VM instance to connect from.                                                                                             |
| vm_zone                  |  string  |    false     | Zone of the VM. If omitted, the tool searches all zones in the project via an aggregated list filter.                                    |
| database_name            |  string  |    false     | Database name to connect to. Defaults to the engine's conventional default: `postgres` (Postgres), `mysql` (MySQL), or `master` (MSSQL). |
| language                 |  string  |    false     | Programming language for a code snippet: `python`, `nodejs`, `java`, or `go`. Omit to skip snippet output.                               |

## Example

```yaml
kind: source
name: my-cloud-sql-admin-source
type: cloud-sql-admin
---
kind: tool
name: connect_to_gce
type: cloud-sql-connect-gce
source: my-cloud-sql-admin-source
description: Help me connect a Cloud SQL instance to a GCE VM.
```

## Output Format

The tool returns a JSON object containing:

- `instanceConnectionName`, `project`, `region`
- `databaseType` (`postgres`, `mysql`, or `sqlserver`) and `databaseVersion`
  (the raw `DatabaseVersion` string from the Cloud SQL Admin API)
- `computeType` (`gce`), `computeResource` (the VM name), `computeLocation`
  (the resolved zone)
- `validation` — checks performed and their status
- `recommendedMethod` and `alternativeMethods` — one of `auth_proxy`,
  `connector`, or `direct_private_ip`, with rationale and requirements
  (SQL Server prefers `auth_proxy` since the Connector library covers
  Postgres and MySQL only)
- `connectionStrings` — host/port/DSN/JDBC templates (credential
  placeholders are `USER` / `PASS`)
- `environmentConfig` — env vars and, where relevant, the Auth Proxy launch
  command
- `setupSteps` — ordered actions the user needs to take
- `codeSnippet` — populated only when `language` was set; carries a
  floor-pinned `dependencies` list and an install-command note derived
  from it (`pip install ...`, `npm install ...`, `go get ...`)
- `requiredIamRoles`, `requiredApis`

### Dependencies contract

The `codeSnippet.dependencies` field returns each library with a floor
pin (e.g., `sqlalchemy>=2.0`, `pg@^8.11`, `com.google.cloud.sql:...:1.15.0`)
representing the minimum version the tool has been tested against. Newer
minor releases should work. For production use, pin exact versions in
your project's package manifest.

## Reference

### Tool Configuration

| **field**   | **type** | **required** | **description**                                  |
| ----------- | :------: | :----------: | ------------------------------------------------ |
| type        |  string  |     true     | Must be `cloud-sql-connect-gce`.                 |
| source      |  string  |     true     | The name of the `cloud-sql-admin` source to use. |
| description |  string  |    false     | Overrides the default tool description.          |
