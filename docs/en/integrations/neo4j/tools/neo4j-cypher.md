---
title: "neo4j-cypher"
type: docs
weight: 1
description: >
  A "neo4j-cypher" tool executes a pre-defined cypher statement against a Neo4j
  database.
---

## About

A `neo4j-cypher` tool executes a pre-defined Cypher statement against a Neo4j
database.

The specified Cypher statement is executed as a [parameterized
statement][neo4j-parameters], and specified parameters will be used according to
their name: e.g. `$id`.

> **Note:** This tool uses parameterized queries to prevent SQL injections.
> Query parameters can be used as substitutes for arbitrary expressions.
> Parameters cannot be used as substitutes for identifiers, column names, table
> names, or other parts of the query.

[neo4j-parameters]:
    https://neo4j.com/docs/cypher-manual/current/syntax/parameters/


## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: search_movies_by_actor
type: neo4j-cypher
source: my-neo4j-movies-instance
statement: |
  MATCH (m:Movie)<-[:ACTED_IN]-(p:Person)
  WHERE p.name = $name AND m.year > $year
  RETURN m.title, m.year
  LIMIT 10
description: |
  Use this tool to get a list of movies for a specific actor and a given minimum release year.
  Takes a full actor name, e.g. "Tom Hanks" and a year e.g 1993 and returns a list of movie titles and release years.
  Do NOT use this tool with a movie title. Do NOT guess an actor name, Do NOT guess a year.
  A actor name is a fully qualified name with first and last name separated by a space.
  For example, if given "Hanks, Tom" the actor name is "Tom Hanks".
  If the tool returns more than one option choose the most recent movies.
  Example:
  {{
      "name": "Meg Ryan",
      "year": 1993
  }}
  Example:
  {{
      "name": "Clint Eastwood",
      "year": 2000
  }}
parameters:
  - name: name
    type: string
    description: Full actor name, "firstname lastname"
  - name: year
    type: integer
    description: 4 digit number starting in 1900 up to the current year
```

### Vector Search

Neo4j supports vector similarity search. When using an `embeddingModel` with a `neo4j-cypher` tool, the tool automatically converts text parameters into the vector format required by Neo4j.

#### Define the Embedding Model

See [EmbeddingModels](../../../documentation/configuration/embedding-models/_index.md) for more information.

```yaml
kind: embeddingModel
name: gemini-model
type: gemini
model: gemini-embedding-001
apiKey: ${GOOGLE_API_KEY}
dimension: 768
```

#### Vector Ingestion Tool

This tool stores both the raw text and its vector representation. It uses `valueFromParam` to hide the vector conversion logic from the LLM, ensuring the Agent only has to provide the content once.
```yaml
kind: tool
name: insert_doc_neo4j
type: neo4j-cypher
source: my-neo4j-source
statement: |
  CREATE (n:Document {content: $content, embedding: $text_to_embed})
  RETURN 1 as result
description: |
  Index new documents for semantic search in Neo4j.
parameters:
  - name: content
    type: string
    description: The text content to store.
  - name: text_to_embed
    type: string
    # Automatically copies 'content' and converts it to a vector array
    valueFromParam: content
    embeddedBy: gemini-model
```

#### Vector Search Tool

This tool allows the Agent to perform a natural language search. The query string provided by the Agent is converted into a vector before the Cypher statement is executed.

```yaml
kind: tool
name: search_docs_neo4j
type: neo4j-cypher
source: my-neo4j-source
statement: |
  MATCH (n:Document)
  WITH n, vector.similarity.cosine(n.embedding, $query) AS score
  WHERE score IS NOT NULL
  ORDER BY score DESC
  LIMIT 1
  RETURN n.content as content
description: |
  Search for documents in Neo4j using natural language. 
  Returns the most semantically similar result.
parameters:
  - name: query
    type: string
    description: The search query to be converted to a vector.
    embeddedBy: gemini-model
```

## Reference

| **field**   |                **type**                 | **required** | **description**                                                                              |
|-------------|:---------------------------------------:|:------------:|----------------------------------------------------------------------------------------------|
| type        |                 string                  |     true     | Must be "neo4j-cypher".                                                                      |
| source      |                 string                  |     true     | Name of the source the Cypher query should execute on.                                       |
| description |                 string                  |     true     | Description of the tool that is passed to the LLM.                                           |
| statement   |                 string                  |     true     | Cypher statement to execute                                                                  |
| parameters  | [parameters](../../../documentation/configuration/tools/_index.md#specifying-parameters) |    false     | List of [parameters](../../../documentation/configuration/tools/_index.md#specifying-parameters) that will be used with the Cypher statement. |
