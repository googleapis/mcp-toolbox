---
title: cloud-sql-connect-gke
type: docs
weight: 13
description: Plan the connection from a Google Kubernetes Engine workload to a Cloud SQL instance (PostgreSQL, MySQL, or SQL Server). The engine is auto-detected from the instance.
---

## About

The `cloud-sql-connect-gke` tool helps an agent (or user) connect a workload
running on Google Kubernetes Engine to any Cloud SQL instance. The engine
(PostgreSQL, MySQL, or SQL Server) is auto-detected from the
`DatabaseVersion` returned by the Cloud SQL Admin API, so the caller does not
have to specify it.

Most GKE-to-Cloud SQL failures come from cluster facts that are invisible
from inside a pod. The tool reads those facts and tailors its output to
them:

| Cluster fact                                       | Why it matters                                                                       |
| -------------------------------------------------- | ------------------------------------------------------------------------------------ |
| Workload Identity disabled                         | Pods cannot authenticate as a Google service account without exported keys.          |
| Node pools not on `GKE_METADATA`                   | Pods silently use the node's service account instead of Workload Identity.           |
| Routes-based (non VPC-native) cluster              | Pod traffic cannot reach a private services access IP.                               |
| Different VPC, or different Shared VPC host        | Private IP is unreachable; peering is not transitive.                                |
| Private nodes                                      | A public IP path needs Cloud NAT.                                                    |
| Private Service Connect only                       | Needs an allowed consumer project, a PSC endpoint, and DNS for the instance.         |
| Kubernetes 1.29 or newer                           | The proxy can run as a native sidecar, removing startup races and unblocking Jobs.   |

The tool:

1. Reads the Cloud SQL instance via the `cloud-sql-admin` source and the
   cluster via the GKE API. If `cluster_location` is omitted, the cluster is
   found by listing all locations in `cluster_project`.
2. Resolves the network path, preferring private IP, then public IP, then
   Private Service Connect. Private IP is chosen only when the cluster is
   VPC-native and on the instance's network (Shared VPC host networks are
   compared by host project).
3. Validates the cluster and instance (cluster status, network path, PSC
   allowed projects, Workload Identity, node pool metadata mode, IAM
   database authentication, SSL enforcement). The result is invalid only
   when there is no network path or the cluster is not running; everything
   else is reported as a warning with a matching setup step.
4. Recommends the Cloud SQL Auth Proxy sidecar, with the Cloud SQL
   Connector library (PostgreSQL and MySQL) and direct private IP (when
   reachable) as alternatives.
5. Returns a one-paragraph `summary`, setup steps tailored to the cluster's
   current state, ready-to-apply Kubernetes manifests, troubleshooting tips
   for the chosen method, and, when `language` is set, a code snippet.

The tool is read-only. It never changes the cluster, IAM, or the instance.
The `summary` tells the agent to get the user's approval before running
steps that do.

### Generated setup steps

Steps are numbered without gaps and only include work that is still needed:
for example, enabling Workload Identity is skipped when it is already on,
namespace creation is skipped for `default`, and PSC endpoint or Cloud NAT
steps appear only on those paths. Every command carries explicit
`--project`, `--location`, and `-n` flags so it targets the right resources
regardless of local `gcloud` and `kubectl` defaults. Commands contain no
pipes, heredocs, or command substitution, so an agent can show and run each
line as is.

The generated sidecar uses Cloud SQL Auth Proxy v2 with the IP-type flag
that matches the detected path (`--private-ip`, `--psc`, or none for public
IP), an HTTP health check with startup and liveness probes, and
`--exit-zero-on-sigterm`. It does not set `--auto-iam-authn`, because that
breaks built-in database users; the troubleshooting tips explain when to
add it.

### Identity boundary

Both the Cloud SQL Admin call and the GKE call honor the caller's OAuth
access token when the `cloud-sql-admin` source is configured with
`useClientOAuth: true`, so IAM decisions on both APIs are evaluated as the
caller. When no caller token is passed, the GKE call falls back to
Application Default Credentials.

### Input validation

Caller-supplied identifiers that flow into shell commands, manifests,
DSN/JDBC templates, or IAM member strings (`instance_connection_name`,
`cluster_name`, `cluster_location`, `cluster_project`, `namespace`,
`kubernetes_service_account`, `database_name`) are validated against GCP and
Kubernetes naming rules before any API call is made.

## Compatible Sources

{{< compatible-sources >}}

## Requirements

- The identity used for the tool call (caller OAuth or the Toolbox
  service account, per source configuration) needs permission to read the
  Cloud SQL instance (for example `roles/cloudsql.viewer`) and the cluster
  (for example `roles/container.clusterViewer` on the cluster's project).
- The Cloud SQL Admin API (`sqladmin.googleapis.com`) and Kubernetes Engine
  API (`container.googleapis.com`) must be enabled.

## Parameters

| **parameter**              | **type** | **required** | **description**                                                                                                                          |
| -------------------------- | :------: | :----------: | ---------------------------------------------------------------------------------------------------------------------------------------- |
| instance_connection_name   |  string  |     true     | Cloud SQL instance connection name in the format `project:region:instance`.                                                              |
| cluster_name               |  string  |     true     | Name of the GKE cluster the workload runs in.                                                                                            |
| cluster_location           |  string  |    false     | Region or zone of the cluster (for example `us-central1` or `us-central1-a`). If omitted, all locations in `cluster_project` are searched. |
| cluster_project            |  string  |    false     | Project that owns the cluster. Defaults to the instance's project; set it when the cluster runs in a different project.                  |
| namespace                  |  string  |    false     | Kubernetes namespace of the workload. Defaults to `default`.                                                                             |
| kubernetes_service_account |  string  |    false     | Kubernetes service account the workload runs as. Defaults to `cloudsql-ksa`, which the setup steps create.                               |
| database_name              |  string  |    false     | Database name to connect to. Defaults to the engine's conventional default: `postgres` (Postgres), `mysql` (MySQL), or `master` (MSSQL). |
| language                   |  string  |    false     | Programming language for a code snippet: `python`, `nodejs`, `java`, or `go`. Omit to skip snippet output.                               |

## Example

```yaml
kind: source
name: my-cloud-sql-admin-source
type: cloud-sql-admin
---
kind: tool
name: connect_to_gke
type: cloud-sql-connect-gke
source: my-cloud-sql-admin-source
description: Help me connect a GKE workload to a Cloud SQL instance.
```

## Output Format

The tool returns a JSON object containing:

- `summary` — a plain-language verdict to relay first: either the
  connectivity blocker, or the chosen method and network path plus any
  cluster changes needed
- `instanceConnectionName`, `project`, `region`
- `databaseType` (`postgres`, `mysql`, or `sqlserver`) and `databaseVersion`
- `computeType` (`gke`), `computeResource` (the cluster name),
  `computeLocation` (the cluster's region or zone)
- `validation` — checks performed and their status
- `recommendedMethod` and `alternativeMethods` — one of `auth_proxy`,
  `connector`, or `direct_private_ip`, with rationale and requirements
- `connectionStrings` — host/port/DSN/JDBC templates
- `environmentConfig`:
  - `environmentVariables` — what the app container reads (`DB_HOST`,
    `DB_PORT`, and `DB_USER` / `DB_PASS` / `DB_NAME` from the Secret)
  - `serviceAccountYaml` — the Kubernetes service account annotated for
    Workload Identity
  - `sidecarYaml` — the Auth Proxy container to merge into an existing pod
    spec (as an init container with `restartPolicy: Always` on 1.29+)
  - `deploymentYaml` — a complete example Deployment wiring it all together
  - `secretYaml` — the Secret shape (the setup steps create it
    with `kubectl create secret` instead, to keep passwords out of files)
- `setupSteps` — ordered actions the user needs to take
- `troubleshooting` — symptom, cause, and fix for the failures users hit
  most often with the chosen method
- `codeSnippet` — populated only when `language` was set
- `requiredIamRoles` (for the workload's Google service account) and
  `requiredApis`

## Reference

### Tool Configuration

| **field**   | **type** | **required** | **description**                                  |
| ----------- | :------: | :----------: | ------------------------------------------------ |
| type        |  string  |     true     | Must be `cloud-sql-connect-gke`.                 |
| source      |  string  |     true     | The name of the `cloud-sql-admin` source to use. |
| description |  string  |    false     | Overrides the default tool description.          |
