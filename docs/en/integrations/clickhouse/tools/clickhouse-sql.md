---
title: "clickhouse-sql"
type: docs
weight: 2
description: >
  A "clickhouse-sql" tool executes SQL queries as prepared statements in ClickHouse.
---

## About

A `clickhouse-sql` tool executes SQL queries as prepared statements against a
ClickHouse database.

This tool supports both template parameters (for SQL statement customization)
and regular parameters (for prepared statement values), providing flexible
query execution capabilities.

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: my_analytics_query
type: clickhouse-sql
source: my-clickhouse-instance
description: Get user analytics for a specific date range
statement: |
  SELECT 
    user_id,
    count(*) as event_count,
    max(timestamp) as last_event
  FROM events 
  WHERE date >= ? AND date <= ?
  GROUP BY user_id
  ORDER BY event_count DESC
  LIMIT ?
parameters:
  - name: start_date
    description: Start date for the query (YYYY-MM-DD format)
  - name: end_date  
    description: End date for the query (YYYY-MM-DD format)
  - name: limit
    description: Maximum number of results to return
```

### Template Parameters Example

```yaml
kind: tool
name: flexible_table_query
type: clickhouse-sql
source: my-clickhouse-instance
description: Query any table with flexible columns
statement: |
  SELECT {{columns}}
  FROM {{table_name}}
  WHERE created_date >= ?
  LIMIT ?
templateParameters:
  - name: columns
    description: Comma-separated list of columns to select
  - name: table_name
    description: Name of the table to query
parameters:
  - name: start_date
    description: Start date filter
  - name: limit
    description: Maximum number of results
```

### Vector Search Example

The `clickhouse-sql` tool can transparently embed string parameters into vectors
via Toolbox's native [embedding models](../../../documentation/configuration/embedding-models/_index.md).
The vector is bound to the prepared-statement placeholder as a native
`Array(Float32)`, so you can write SQL against ClickHouse's vector functions
(e.g. `cosineDistance`, `L2Distance`) without doing any string parsing yourself.

Assume the following destination table:

```sql
CREATE TABLE documents (
  id UUID DEFAULT generateUUIDv4(),
  content String,
  embedding Array(Float32)
) ENGINE = MergeTree ORDER BY tuple();
```

Define the embedding model:

```yaml
kind: embeddingModel
name: gemini-model
type: gemini
apiKey: ${GOOGLE_API_KEY}
model: gemini-embedding-001
dimension: 768
```

Define an ingestion tool that embeds `content` before insert by mirroring it
into a second parameter (`text_to_embed`) that carries the `embeddedBy` hint:

```yaml
kind: tool
name: insert_doc
type: clickhouse-sql
source: my-clickhouse-instance
description: Indexes a new document and its vector embedding.
statement: |
  INSERT INTO documents (content, embedding) VALUES (?, ?)
parameters:
  - name: content
    type: string
    description: The text content to store.
  - name: text_to_embed
    type: string
    description: The text content used to generate the vector.
    valueFromParam: content
    embeddedBy: gemini-model
```

Define a search tool that embeds the LLM-supplied `query` and ranks rows by
cosine distance:

```yaml
kind: tool
name: search_docs
type: clickhouse-sql
source: my-clickhouse-instance
description: Finds the most semantically similar document to a query.
statement: |
  SELECT content, cosineDistance(embedding, ?) AS distance
  FROM documents
  ORDER BY distance ASC
  LIMIT 1
parameters:
  - name: query
    type: string
    description: The search query.
    embeddedBy: gemini-model
```

Only `string`-typed parameters may declare `embeddedBy`. The embedding model
must be defined as an `embeddingModel` kind of the same
configuration file.

## Reference

| **field**          |      **type**      | **required** | **description**                                       |
|--------------------|:------------------:|:------------:|-------------------------------------------------------|
| type               |       string       |     true     | Must be "clickhouse-sql".                             |
| source             |       string       |     true     | Name of the ClickHouse source to execute SQL against. |
| description        |       string       |     true     | Description of the tool that is passed to the LLM.    |
| statement          |       string       |     true     | The SQL statement template to execute.                |
| parameters         | array of Parameter |    false     | Parameters for prepared statement values.             |
| templateParameters | array of Parameter |    false     | Parameters for SQL statement template customization.  |
