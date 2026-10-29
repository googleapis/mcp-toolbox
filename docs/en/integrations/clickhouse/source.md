---
title: "ClickHouse Source"
type: docs
weight: 1
linkTitle: "Source"
description: >
  ClickHouse is an open-source, OLTP database.
no_list: true
---

## About

[ClickHouse][clickhouse-docs] is a fast, open-source, column-oriented database

[clickhouse-docs]: https://clickhouse.com/docs

## Available Tools

{{< list-tools >}}

## Requirements

### Database User

This source uses standard ClickHouse authentication. You will need to [create a
ClickHouse user][clickhouse-users] (or with [ClickHouse
Cloud][clickhouse-cloud]) to connect to the database with. The user should have
appropriate permissions for the operations you plan to perform.

[clickhouse-cloud]:
    https://clickhouse.com/docs/getting-started/quick-start/cloud#connect-with-your-app
[clickhouse-users]: https://clickhouse.com/docs/en/sql-reference/statements/create/user

### Network Access

ClickHouse supports multiple protocols:

- **HTTPS protocol** (default port 8443) - Secure HTTP access (default)
- **HTTP protocol** (default port 8123) - Good for web-based access

## Example

### Secure Connection Example

```yaml
kind: source
name: secure-clickhouse-source
type: clickhouse
host: clickhouse.example.com
port: "8443"
database: analytics
user: ${CLICKHOUSE_USER}
password: ${CLICKHOUSE_PASSWORD}
protocol: https
secure: true
```

### HTTP Protocol Example

```yaml
kind: source
name: http-clickhouse-source
type: clickhouse
host: localhost
port: "8123"
database: logs
user: ${CLICKHOUSE_USER}
password: ${CLICKHOUSE_PASSWORD}
protocol: http
secure: false
```

### Connection Pool Example

The source keeps a pool of connections to ClickHouse. You can tune the pool
for your workload; any field you omit keeps its default.

```yaml
kind: source
name: tuned-clickhouse-source
type: clickhouse
host: clickhouse.example.com
port: "8443"
database: analytics
user: ${CLICKHOUSE_USER}
password: ${CLICKHOUSE_PASSWORD}
maxOpenConns: 50
maxIdleConns: 10
connMaxLifetime: 10m
```

{{< notice tip >}}
Use environment variable replacement with the format ${ENV_NAME}
instead of hardcoding your secrets into the configuration file.
{{< /notice >}}

## Reference

| **field**       | **type** | **required** | **description**                                                                                                              |
|-----------------|:--------:|:------------:|------------------------------------------------------------------------------------------------------------------------------|
| type            |  string  |     true     | Must be "clickhouse".                                                                                                        |
| host            |  string  |     true     | IP address or hostname to connect to (e.g. "127.0.0.1" or "clickhouse.example.com")                                          |
| port            |  string  |     true     | Port to connect to (e.g. "8443" for HTTPS, "8123" for HTTP)                                                                  |
| database        |  string  |     true     | Name of the ClickHouse database to connect to (e.g. "my_database").                                                          |
| user            |  string  |     true     | Name of the ClickHouse user to connect as (e.g. "analytics_user").                                                           |
| password        |  string  |    false     | Password of the ClickHouse user (e.g. "my-password").                                                                        |
| protocol        |  string  |    false     | Connection protocol: "https" (default) or "http".                                                                            |
| secure          | boolean  |    false     | Whether to use a secure connection (TLS). Default: false.                                                                    |
| maxOpenConns    | integer  |    false     | Maximum number of open connections to the database. Must be at least 1. Default: 25.                                         |
| maxIdleConns    | integer  |    false     | Maximum number of idle connections kept in the pool. Must not exceed `maxOpenConns`. Default: 5, or `maxOpenConns` if lower. |
| connMaxLifetime |  string  |    false     | Maximum time a connection may be reused before it is closed (e.g. "10m", "1h"). Default: "5m".                               |
