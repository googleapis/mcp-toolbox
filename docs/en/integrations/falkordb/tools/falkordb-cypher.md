---
title: "falkordb-cypher"
type: docs
weight: 1
description: >
  A "falkordb-cypher" tool executes a pre-defined Cypher statement against a
  FalkorDB graph.
---

## About

A `falkordb-cypher` tool executes a pre-defined Cypher statement against the
graph configured on a FalkorDB source. The specified Cypher statement is
executed as a [parameterized statement][falkordb-parameters], and specified
parameters will be used according to their name: e.g. `$id`.

> **Note:** This tool uses parameterized queries to prevent injections. Query
> parameters can be used as substitutes for arbitrary expressions. Parameters
> cannot be used as substitutes for identifiers, labels, relationship types,
> or other parts of the query.

[falkordb-parameters]: https://docs.falkordb.com/cypher/cypher_support.html

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: search_movies_by_actor
type: falkordb-cypher
source: my-falkordb-movies-instance
statement: |
  MATCH (m:Movie)<-[:ACTED_IN]-(p:Person)
  WHERE p.name = $name AND m.year > $year
  RETURN m.title, m.year
  LIMIT 10
description: |
  Use this tool to get a list of movies for a specific actor and a given minimum release year.
  Takes a full actor name, e.g. "Tom Hanks" and a year e.g 1993 and returns a list of movie titles and release years.
parameters:
  - name: name
    type: string
    description: Full name of the actor.
  - name: year
    type: integer
    description: Minimum release year.
```

## Reference

| **field**   |                  **type**                  | **required** | **description**                                                |
|-------------|:------------------------------------------:|:------------:|----------------------------------------------------------------|
| type        |                   string                   |     true     | Must be "falkordb-cypher".                                     |
| source      |                   string                   |     true     | Name of the source the Cypher statement should execute on.     |
| description |                   string                   |     true     | Description of the tool that is passed to the LLM.             |
| statement   |                   string                   |     true     | Cypher statement to execute.                                   |
| parameters  | [parameters](../../../documentation/configuration/tools/_index.md#specifying-parameters) |    false     | List of [parameters](../../../documentation/configuration/tools/_index.md#specifying-parameters) that will be used with the Cypher statement. |
