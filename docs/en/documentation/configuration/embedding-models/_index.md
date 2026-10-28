---
title: "EmbeddingModels"
type: docs
weight: 7
description: >
  EmbeddingModels represent services that transform text into vector embeddings
  for semantic search.
---

EmbeddingModels represent services that generate vector representations of text
data. In the MCP Toolbox, these models enable **Semantic Queries**, allowing
[Tools](../tools/_index.md) to automatically convert human-readable text into numerical
vectors before using them in a query.

This is primarily used in two scenarios:

- **Vector Ingestion**: Converting a text parameter into a vector string during
  an `INSERT` operation.

- **Semantic Search**: Converting a natural language query into a vector to
  perform similarity searches.

## Hidden Parameter Duplication (valueFromParam)

When building tools for vector ingestion, you often need the same input string
twice:

1. To store the original text in a TEXT column.
1. To generate the vector embedding for a VECTOR column.

Requesting an Agent (LLM) to output the exact same string twice is inefficient
and error-prone. The `valueFromParam` field solves this by allowing a parameter
to inherit its value from another parameter in the same tool.

### Key Behaviors

1. Hidden from Manifest: Parameters with valueFromParam set are excluded from
   the tool definition sent to the Agent. The Agent does not know this parameter
   exists.
1. Auto-Filled: When the tool is executed, the Toolbox automatically copies the
   value from the referenced parameter before processing embeddings.

## Example

The following configuration defines an embedding model and applies it to
specific tool parameters.

{{< notice tip >}} Use environment variable replacement with the format
${ENV_NAME} instead of hardcoding your API keys into the configuration file.
{{< /notice >}}

### Step 1 - Define an Embedding Model

Define an embedding model with the `embeddingModel` kind:

```yaml
kind: embeddingModel
name: gemini-model # Name of the embedding model
type: gemini
model: gemini-embedding-001
apiKey: ${GOOGLE_API_KEY}
dimension: 768
```

### Step 2 - Embed Tool Parameters

Use the defined embedding model, embed your query parameters using the
`embeddedBy` field. Only string-typed parameters can be embedded:

```yaml
# Vector ingestion tool
kind: tool
name: insert_embedding
type: postgres-sql
source: my-pg-instance
description: Insert a new document into the database.
statement: |
  INSERT INTO documents (content, embedding)
  VALUES ($1, $2);
parameters:
  - name: content
    type: string
    description: The raw text content to be stored in the database.
  - name: vector_string
    type: string
    description: A hidden parameter holding a copy of 'content' to be embedded as a vector.
    # This parameter is hidden from the LLM.
    # It automatically copies the value from 'content' and embeds it.
    valueFromParam: content
    embeddedBy: gemini-model
---
# Semantic search tool
kind: tool
name: search_embedding
type: postgres-sql
source: my-pg-instance
description: Search for documents in the database.
statement: |
  SELECT id, content, embedding <-> $1 AS distance
  FROM documents
  ORDER BY distance LIMIT 1
parameters:
  - name: semantic_search_string
    type: string
    description: The search query that will be converted to a vector.
    embeddedBy: gemini-model # refers to the name of a defined embedding model
```

## Types of Embedding Models
