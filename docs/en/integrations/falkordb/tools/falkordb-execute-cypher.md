---
title: "falkordb-execute-cypher"
type: docs
weight: 2
description: >
  A "falkordb-execute-cypher" tool executes an arbitrary Cypher query against
  a FalkorDB graph.
---

## About

A `falkordb-execute-cypher` tool executes a Cypher query provided by the
calling agent against the graph configured on a FalkorDB source.

When `readOnly` is set to `true`, queries are dispatched with FalkorDB's
`GRAPH.RO_QUERY` command, so write operations are rejected by the database
server itself rather than by client-side query inspection.

When `allowGraphOverride` is set to `true`, the tool exposes an additional
`graph` parameter that lets the agent target any graph on the instance
instead of the source's default graph. Leave this off (the default) when the
instance hosts graphs the agent should not reach.

Setting the `dry_run` parameter to `true` returns the query's execution plan
(via `GRAPH.EXPLAIN`) without running it.

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: execute_cypher
type: falkordb-execute-cypher
source: my-falkordb-instance
description: Use this tool to execute a Cypher query against the graph.
readOnly: true
```

## Reference

| **field**          | **type** | **required** | **description**                                                                        |
|--------------------|:--------:|:------------:|----------------------------------------------------------------------------------------|
| type               |  string  |     true     | Must be "falkordb-execute-cypher".                                                     |
| source             |  string  |     true     | Name of the source the Cypher query should execute on.                                 |
| description        |  string  |     true     | Description of the tool that is passed to the LLM.                                     |
| readOnly           |   bool   |    false     | Execute queries with GRAPH.RO_QUERY so the server rejects writes. Defaults to false.   |
| allowGraphOverride |   bool   |    false     | Expose a `graph` parameter to target other graphs on the instance. Defaults to false.  |
