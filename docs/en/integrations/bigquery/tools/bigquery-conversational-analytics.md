---
title: "bigquery-conversational-analytics"
type: docs
weight: 1
description: >
  A "bigquery-conversational-analytics" tool allows conversational interaction with a BigQuery source.
---

## About

A `bigquery-conversational-analytics` tool allows you to ask questions about
your data in natural language.

This function takes a user's question (which can include conversational history
for context) and references to specific BigQuery tables, and sends them to a
stateless conversational API.

The API uses a GenAI agent to understand the question, generate and execute SQL
queries and Python code, and formulate an answer. This function returns a
detailed, sequential log of this entire process, which includes any generated
SQL or Python code, the data retrieved, and the final text answer.

**Note**: This tool requires additional setup in your project. Please refer to
the official [Conversational Analytics API
documentation](https://cloud.google.com/gemini/docs/conversational-analytics-api/overview)
for instructions.

`bigquery-conversational-analytics` accepts the following parameters:

- **`user_query_with_context`:** The user's question, potentially including
  conversation history and system instructions for context.
- **`table_references`:** A JSON string of a list of BigQuery tables to use as
  context. Each object in the list must contain `projectId`, `datasetId`, and
  `tableId`. Example: `'[{"projectId": "my-gcp-project", "datasetId":
  "my_dataset", "tableId": "my_table"}]'`

The tool's behavior regarding these parameters is influenced by the
`allowedDatasets` restriction on the `bigquery` source:

- **Without `allowedDatasets` restriction:** The tool can use tables from any
dataset specified in the `table_references` parameter.
- **With `allowedDatasets` restriction:** Before processing the request, the
  tool verifies that every table in `table_references` belongs to a dataset in
  `allowedDatasets`. If any table is from a dataset that is not listed, the
  request is denied.


## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: ask_data_insights
type: bigquery-conversational-analytics
source: my-bigquery-source
description: |
  Use this tool to perform data analysis, get insights, or answer complex 
  questions about the contents of specific BigQuery tables.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "bigquery-conversational-analytics".       |
| source      |  string  |     true     | Name of the source for chat.                       |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
