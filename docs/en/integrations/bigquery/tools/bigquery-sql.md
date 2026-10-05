---
title: "bigquery-sql"
type: docs
weight: 1
description: >
  A "bigquery-sql" tool executes a pre-defined SQL statement.
---

## About

A `bigquery-sql` tool executes a pre-defined SQL statement.

The behavior of this tool is influenced by the `readOnly` and `writeMode`
settings on its `bigquery` source:

- **Read-Only Mode:** When a source is configured with
  `readOnly: true` (or `writeMode: blocked` / `writeMode: protected`),
  `bigquery-sql` tools default to write-capable annotations (`readOnlyHint: false`)
  and are automatically suppressed from registration during server startup to
  protect data integrity and optimize agent context window. To keep a pre-defined
  read-only query active in read-only mode, explicitly annotate the tool with
  `readOnlyHint: true`.
- **`protected`:** This mode enables session-based
  execution. The tool will operate within the same BigQuery session as other
  tools using the same source, allowing it to interact with temporary resources
  like `TEMP` tables created within that session.

## Compatible Sources

{{< compatible-sources >}}

### GoogleSQL

BigQuery uses [GoogleSQL][bigquery-googlesql] for querying data. The integration
with Toolbox supports this dialect. The specified SQL statement is executed, and
parameters can be inserted into the query. BigQuery supports both named
parameters (e.g., `@name`) and positional parameters (`?`), but they cannot be
mixed in the same query.

[bigquery-googlesql]:
  https://cloud.google.com/bigquery/docs/reference/standard-sql/

## Example

> **Note:** This tool uses
> [parameterized queries](https://cloud.google.com/bigquery/docs/parameterized-queries)
> to prevent SQL injections. Query parameters can be used as substitutes for
> arbitrary expressions. Parameters cannot be used as substitutes for
> identifiers, column names, table names, or other parts of the query.

```yaml
# Example: Querying a user table in BigQuery
kind: tool
name: search_users_bq
type: bigquery-sql
source: my-bigquery-source
statement: |
  SELECT
    id,
    name,
    email
  FROM
    `my-project.my-dataset.users`
  WHERE
    id = @id OR email = @email;
description: |
  Use this tool to get information for a specific user.
  Takes an id number or a name and returns info on the user.

  Example:
  {{
      "id": 123,
      "name": "Alice",
  }}
parameters:
  - name: id
    type: integer
    description: User ID
  - name: email
    type: string
    description: Email address of the user
```

### Example with Vector Search

BigQuery supports vector similarity search using the `ML.DISTANCE` function.
When using an embeddingModel with a `bigquery-sql` tool, the tool automatically
converts text parameters into the native ARRAY<FLOAT64> format required by
BigQuery.

#### Define the Embedding Model

See
[EmbeddingModels](../../../documentation/configuration/embedding-models/_index.md)
for more information.

```yaml
kind: embeddingModel
name: gemini-model
type: gemini
model: gemini-embedding-001
apiKey: ${GOOGLE_API_KEY}
dimension: 768
```

#### Vector Ingestion Tool

This tool stores both the raw text and its vector representation. It uses
`valueFromParam` to hide the vector conversion logic from the LLM, ensuring the
Agent only has to provide the content once.

```yaml
kind: tool
name: insert_doc
type: bigquery-sql
source: my-bigquery-source
statement: |
  INSERT INTO `my-project.my-dataset.vector_table` (id, content, embedding)
  VALUES (1, @content, @text_to_embed)
description: |
  Internal tool to index new documents for future search.
parameters:
  - name: content
    type: string
    description: The text content to store.
  - name: text_to_embed
    type: string
    # Automatically copies 'content' and converts it to a FLOAT64 array
    valueFromParam: content
    embeddedBy: gemini-model
```

#### Vector Search Tool

This tool allows the Agent to perform a natural language search. The query
string provided by the Agent is converted into a vector before the SQL is
executed.

```yaml
kind: tool
name: search_docs
type: bigquery-sql
source: my-bigquery-source
statement: |
  SELECT 
    id, 
    content, 
    ML.DISTANCE(embedding, @query, 'COSINE') AS distance 
  FROM 
    `my-project.my-dataset.vector_table` 
  ORDER BY 
    distance 
  LIMIT 1
description: |
  Search for documents using natural language. 
  Returns the most semantically similar result.
parameters:
  - name: query
    type: string
    description: The search terms or question.
    embeddedBy: gemini-model
```

### Example with Template Parameters

> **Note:** This tool allows direct modifications to the SQL statement,
> including identifiers, column names, and table names. **This makes it more
> vulnerable to SQL injections**. Using basic parameters only (see above) is
> recommended for performance and safety reasons. For more details, please check
> [templateParameters](../../../documentation/configuration/tools/_index.md#template-parameters).

```yaml
kind: tool
name: list_table
type: bigquery-sql
source: my-bigquery-source
statement: |
  SELECT * FROM {{.tableName}};
description: |
  Use this tool to list all information from a specific table.
  Example:
  {{
      "tableName": "flights",
  }}
templateParameters:
  - name: tableName
    type: string
    description: Table to select from
```

## Reference

| **field**          |                                            **type**                                            | **required** | **description**                                                                                                                                                                          |
| ------------------ | :--------------------------------------------------------------------------------------------: | :----------: | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| type               |                                             string                                             |     true     | Must be "bigquery-sql".                                                                                                                                                                  |
| source             |                                             string                                             |     true     | Name of the source the GoogleSQL should execute on.                                                                                                                                      |
| description        |                                             string                                             |     true     | Description of the tool that is passed to the LLM.                                                                                                                                       |
| statement          |                                             string                                             |     true     | The GoogleSQL statement to execute.                                                                                                                                                      |
| parameters         |    [parameters](../../../documentation/configuration/tools/_index.md#specifying-parameters)    |    false     | List of [parameters](../../../documentation/configuration/tools/_index.md#specifying-parameters) that will be inserted into the SQL statement.                                           |
| templateParameters | [templateParameters](../../../documentation/configuration/tools/_index.md#template-parameters) |    false     | List of [templateParameters](../../../documentation/configuration/tools/_index.md#template-parameters) that will be inserted into the SQL statement before executing prepared statement. |
