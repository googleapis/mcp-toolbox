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

This source also connects to [Spanner Omni][spanner-omni-docs], a
deploy-anywhere version of Spanner. See [Spanner Omni](#spanner-omni) below, or
the [Spanner Omni quickstart][spanner-omni-quickstart] to set up a deployment.

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

Spanner Omni sources don't use ADC or IAM. See [Spanner Omni](#spanner-omni)
for the supported connection options.

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

| **field**                 | **type** | **required** | **description**                                                                                                                                     |
|---------------------------|:--------:|:------------:|-----------------------------------------------------------------------------------------------------------------------------------------------------|
| type                      |  string  |     true     | Must be "spanner".                                                                                                                                  |
| project                   |  string  |    false     | Id of the GCP project that the cluster was created in (e.g. "my-project-id"). Required for Spanner; optional for Spanner Omni (default: `default`). |
| instance                  |  string  |    false     | Name of the Spanner instance. Required for Spanner; optional for Spanner Omni (default: `default`).                                                 |
| database                  |  string  |     true     | Name of the database on the Spanner instance                                                                                                        |
| dialect                   |  string  |    false     | Name of the dialect type of the Spanner database, must be either `googlesql` or `postgresql`. Default: `googlesql`.                                 |
| instanceType              |  string  |    false     | Either `cloud` or `omni`. Set to `omni` to connect to a [Spanner Omni][spanner-omni-docs] deployment. Default: `cloud`.                             |
| omniEndpoint              |  string  |    false     | Spanner Omni API endpoint as `host:port` (e.g. "omni.example.com:15000"). Required when `instanceType` is `omni`.                                   |
| omniUsePlainText          |   bool   |    false     | Connect to Spanner Omni without TLS. Only for local development. Cannot be combined with certificates or username/password.                         |
| omniCaCertificateFile     |  string  |    false     | Path to the CA certificate that signed the Spanner Omni API certificate.                                                                            |
| omniClientCertificateFile |  string  |    false     | Path to a client certificate for Spanner Omni mTLS. Requires `omniClientKeyFile`.                                                                   |
| omniClientKeyFile         |  string  |    false     | Path to the private key of the client certificate. Requires `omniClientCertificateFile`.                                                            |
| omniUsername              |  string  |    false     | Spanner Omni username for password authentication. Requires `omniPassword`.                                                                         |
| omniPassword              |  string  |    false     | Spanner Omni password. Use an environment variable (e.g. `${OMNI_PASSWORD}`) rather than a literal value.                                           |

## Advanced Usage

### Spanner Omni

[Spanner Omni][spanner-omni-docs] is a deploy-anywhere version of Spanner. Set
`instanceType: omni` and `omniEndpoint` to connect to it. Spanner Omni does not
use Google Cloud credentials; instead, choose one of the following:

- **TLS:** set `omniCaCertificateFile`.
- **mTLS:** also set `omniClientCertificateFile` and `omniClientKeyFile`.
- **Password:** set `omniUsername` and `omniPassword`. Password authentication
  requires TLS, so also set `omniCaCertificateFile` (or rely on the system trust
  store) and don't set `omniUsePlainText`.
- **Plaintext (local development only):** set `omniUsePlainText: true`.

`project` and `instance` are optional for Spanner Omni and default to
`default`.

```yaml
kind: source
name: my-spanner-omni-source
type: "spanner"
database: "my_db"
instanceType: "omni"
omniEndpoint: "omni.example.com:15000"
omniCaCertificateFile: "/path/to/ca.crt"
omniClientCertificateFile: "/path/to/client.crt"
omniClientKeyFile: "/path/to/client.key"
```

The `spanner-search-catalog` tool uses Knowledge Catalog and is not supported
for Spanner Omni sources.

[spanner-omni-docs]: https://docs.cloud.google.com/spanner-omni/overview
[spanner-omni-quickstart]: https://docs.cloud.google.com/spanner-omni/quickstart
