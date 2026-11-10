---
title: "falkordb-list-graphs"
type: docs
weight: 4
description: >
  A "falkordb-list-graphs" tool lists the graphs stored on a FalkorDB
  instance.
---

## About

A `falkordb-list-graphs` tool lists the names of all graphs stored on the
FalkorDB instance of a source (via `GRAPH.LIST`). A FalkorDB instance can
host many independent graphs, so this tool is typically paired with a
`falkordb-execute-cypher` tool configured with `allowGraphOverride: true`.

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: list_graphs
type: falkordb-list-graphs
source: my-falkordb-instance
description: Use this tool to list the graphs stored on the instance.
```

## Output Format

```json
{"graphs": ["my_graph", "another_graph"]}
```

## Reference

| **field**   | **type** | **required** | **description**                                     |
|-------------|:--------:|:------------:|-----------------------------------------------------|
| type        |  string  |     true     | Must be "falkordb-list-graphs".                     |
| source      |  string  |     true     | Name of the source to list graphs from.             |
| description |  string  |     true     | Description of the tool that is passed to the LLM.  |
