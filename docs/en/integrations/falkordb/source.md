---
title: "FalkorDB Source"
linkTitle: "Source"
type: docs
weight: 1
description: >
  FalkorDB is a low-latency, open source graph database built for GenAI workloads
no_list: true
---

## About

[FalkorDB][falkordb-docs] is a low-latency graph database that speaks the
openCypher query language over the Redis protocol. A single FalkorDB instance
can host many independent graphs, and its native vector and full-text indexes
make it a popular knowledge-graph backend for GenAI applications.

The source connects to one instance and designates a default graph; tools can
optionally target other graphs on the same instance.

[falkordb-docs]: https://docs.falkordb.com/

## Available Tools

{{< list-tools >}}

## Requirements

### Database User

FalkorDB instances without access control can be used with no credentials
(e.g. a local `docker run -p 6379:6379 falkordb/falkordb` instance). For
instances with authentication enabled, such as FalkorDB Cloud, provide the
`username` and `password` fields, and enable `tls` for encrypted endpoints.

## Example

```yaml
kind: source
name: my-falkordb-source
type: falkordb
host: 127.0.0.1
port: "6379"
username: ${FALKORDB_USERNAME}
password: ${FALKORDB_PASSWORD}
graph: my_graph
```

{{< notice tip >}}
Use environment variable replacement with the format ${ENV_NAME}
instead of hardcoding your secrets into the configuration file.
{{< /notice >}}

## Reference

| **field**      | **type** | **required** | **description**                                                                  |
|----------------|:--------:|:------------:|----------------------------------------------------------------------------------|
| type           |  string  |     true     | Must be "falkordb".                                                              |
| host           |  string  |     true     | Host of the FalkorDB instance (e.g. "127.0.0.1").                                |
| port           |  string  |     true     | Port of the FalkorDB instance (e.g. "6379").                                     |
| username       |  string  |    false     | Name of the user to connect as, if authentication is enabled.                    |
| password       |  string  |    false     | Password of the user, if authentication is enabled.                              |
| graph          |  string  |     true     | Name of the default graph tools operate on (e.g. "my_graph").                    |
| queryTimeoutMs |   int    |    false     | Per-query timeout in milliseconds. No timeout when unset.                        |
| tls.enabled    |   bool   |    false     | Enable TLS for the connection. Defaults to false.                                |
| tls.insecureSkipVerify | bool | false    | Skip server certificate verification (not recommended). Defaults to false.       |
