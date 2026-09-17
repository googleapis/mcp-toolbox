---
title: "bigquery-analyze-contribution"
type: docs
weight: 1
description: >
  A "bigquery-analyze-contribution" tool performs contribution analysis in BigQuery.
---

## About

A `bigquery-analyze-contribution` tool performs contribution analysis in
BigQuery by creating a temporary `CONTRIBUTION_ANALYSIS` model and then querying
it with `ML.GET_INSIGHTS` to find top contributors for a given metric.

`bigquery-analyze-contribution` takes the following parameters:

- **input_data** (string, required): The data that contain the test and control
  data to analyze. This can be a fully qualified BigQuery table ID (e.g.,
  `my-project.my_dataset.my_table`) or a SQL query that returns the data.
- **contribution_metric** (string, required): The name of the column that
  contains the metric to analyze. This can be SUM(metric_column_name),
  SUM(numerator_metric_column_name)/SUM(denominator_metric_column_name) or
  SUM(metric_sum_column_name)/COUNT(DISTINCT categorical_column_name) depending
  the type of metric to analyze.
- **is_test_col** (string, required): The name of the column that identifies
  whether a row is in the test or control group. The column must contain boolean
  values.
- **dimension_id_cols** (array of strings, optional): An array of column names
  that uniquely identify each dimension.
- **top_k_insights_by_apriori_support** (integer, optional): The number of top
  insights to return, ranked by apriori support. Default to '30'.
- **pruning_method** (string, optional): The method to use for pruning redundant
  insights. Can be `'NO_PRUNING'` or `'PRUNE_REDUNDANT_INSIGHTS'`. Defaults to
  `'PRUNE_REDUNDANT_INSIGHTS'`.

The behavior of this tool is influenced by the `writeMode` setting on its
`bigquery` source:

- **`allowed` (default) and `blocked`:** These modes do not impose any special
  restrictions on the `bigquery-analyze-contribution` tool.
- **`protected`:** This mode enables session-based execution. The tool will
  operate within the same BigQuery session as other tools using the same source.
  This allows the `input_data` parameter to be a query that references temporary
  resources (e.g., `TEMP` tables) created within that session.

The tool's behavior is also influenced by the `allowedDatasets` restriction on
the `bigquery` source:

- **Without `allowedDatasets` restriction:** The tool can use any table or query
  for the `input_data` parameter.
- **With `allowedDatasets` restriction:** The tool verifies that the
  `input_data` parameter only accesses tables within the allowed datasets.
  - If `input_data` is a table ID, the tool checks if the table's dataset is in
    the allowed list.
  - If `input_data` is a query, the tool performs a dry run to analyze the query.
    It will reject the query if it explicitly references any table outside the
    allowed list. Like [`bigquery-execute-sql`](../bigquery-execute-sql), authorized views are supported:
    if a referenced table is not explicitly named in the SQL text, it is
    permitted. Unanalyzable and dataset-level operations are rejected.


## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: contribution_analyzer
type: bigquery-analyze-contribution
source: my-bigquery-source
description: Use this tool to run contribution analysis on a dataset in BigQuery.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "bigquery-analyze-contribution".           |
| source      |  string  |     true     | Name of the source the tool should execute on.     |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |

## Advanced Usage

### Sample Prompt

You can prepare a sample table following
https://cloud.google.com/bigquery/docs/get-contribution-analysis-insights.
And use the following sample prompts to call this tool:

- What drives the changes in sales in the table
  `bqml_tutorial.iowa_liquor_sales_sum_data`? Use the project id myproject.
- Analyze the contribution for the `total_sales` metric in the table
  `bqml_tutorial.iowa_liquor_sales_sum_data`. The test group is identified by
  the `is_test` column. The dimensions are `store_name`, `city`, `vendor_name`,
  `category_name` and `item_description`.
