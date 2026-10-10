---
title: "falkordb-schema"
type: docs
weight: 3
description: >
  A "falkordb-schema" tool extracts the schema of a FalkorDB graph.
---

## About

A `falkordb-schema` tool extracts a complete schema description of the graph
configured on a FalkorDB source: node labels and relationship types with
their observed property shapes (derived from sampling up to `sampleSize`
entities per label or type, 100 by default), indexes (including vector and
full-text indexes),
constraints, and graph statistics.

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: get_schema
type: falkordb-schema
source: my-falkordb-instance
description: Use this tool to get the schema of the graph.
```

## Output Format

```json
{
  "graphInfo": {"name": "my_graph", "nodeCount": 2, "edgeCount": 1},
  "nodeLabels": [
    {"name": "Person", "count": 1, "properties": [{"name": "name", "types": ["STRING"]}]}
  ],
  "relationships": [
    {"type": "ACTED_IN", "count": 1, "startNode": "Person", "endNode": "Movie", "properties": []}
  ],
  "indexes": [],
  "constraints": [],
  "statistics": {"totalNodes": 2, "totalRelationships": 1}
}
```

## Reference

| **field**          | **type** | **required** | **description**                                                       |
|--------------------|:--------:|:------------:|-----------------------------------------------------------------------|
| type               |  string  |     true     | Must be "falkordb-schema".                                            |
| source             |  string  |     true     | Name of the source to extract the schema from.                        |
| description        |  string  |     true     | Description of the tool that is passed to the LLM.                    |
| sampleSize         |   int    |    false     | Entities sampled per label or type to derive property shapes. Defaults to 100. |
