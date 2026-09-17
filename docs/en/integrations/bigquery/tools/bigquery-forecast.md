---
title: "bigquery-forecast"
type: docs
weight: 1
description: >
  A "bigquery-forecast" tool forecasts time series data in BigQuery.
---

## About

A `bigquery-forecast` tool forecasts time series data in BigQuery.

`bigquery-forecast` constructs and executes a `SELECT * FROM AI.FORECAST(...)`
query based on the provided parameters:

- **history_data** (string, required): This specifies the source of the
  historical time series data. It can be either a fully qualified BigQuery table
  ID (e.g., my-project.my_dataset.my_table) or a SQL query that returns the
  data.
- **timestamp_col** (string, required): The name of the column in your
  history_data that contains the timestamps.
- **data_col** (string, required): The name of the column in your history_data
  that contains the numeric values to be forecasted.
- **id_cols** (array of strings, optional): If you are forecasting multiple time
  series at once (e.g., sales for different products), this parameter takes an
  array of column names that uniquely identify each series. It defaults to an
  empty array if not provided.
- **horizon** (integer, optional): The number of future time steps you want to
  predict. It defaults to 10 if not specified.

The behavior of this tool is influenced by the `writeMode` setting on its
`bigquery` source:

- **`allowed` (default) and `blocked`:** These modes do not impose any special
  restrictions on the `bigquery-forecast` tool.
- **`protected`:** This mode enables session-based execution. The tool will
  operate within the same BigQuery session as other tools using the same source.
  This allows the `history_data` parameter to be a query that references
  temporary resources (e.g., `TEMP` tables) created within that session.

The tool's behavior is also influenced by the `allowedDatasets` restriction on
the `bigquery` source:

- **Without `allowedDatasets` restriction:** The tool can use any table or query
  for the `history_data` parameter.
- **With `allowedDatasets` restriction:** The tool verifies that the
  `history_data` parameter only accesses tables within the allowed datasets.
  - If `history_data` is a table ID, the tool checks if the table's dataset is
    in the allowed list.
  - If `history_data` is a query, the tool performs a dry run to analyze the query.
    It will reject the query if it explicitly references any table outside the
    allowed list. Like [`bigquery-execute-sql`](../bigquery-execute-sql), authorized views are supported:
    if a referenced table is not explicitly named in the SQL text, it is
    permitted. Unanalyzable and dataset-level operations are rejected.


## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: forecast_tool
type: bigquery-forecast
source: my-bigquery-source
description: Use this tool to forecast time series data in BigQuery.
```

## Reference

| **field**   | **type** | **required** | **description**                                         |
|-------------|:--------:|:------------:|---------------------------------------------------------|
| type        |  string  |     true     | Must be "bigquery-forecast".                            |
| source      |  string  |     true     | Name of the source the forecast tool should execute on. |
| description |  string  |     true     | Description of the tool that is passed to the LLM.      |

## Advanced Usage

### Sample Prompt

You can use the following sample prompts to call this tool:

- Can you forecast the history time series data in bigquery table
  `bqml_tutorial.google_analytic`? Use project_id `myproject`.
- What are the future `total_visits` in bigquery table
  `bqml_tutorial.google_analytic`?
