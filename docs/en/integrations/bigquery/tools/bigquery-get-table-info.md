---
title: "bigquery-get-table-info"
type: docs
weight: 1
description: >
  A "bigquery-get-table-info" tool retrieves metadata for a BigQuery table.
---

## About

A `bigquery-get-table-info` tool retrieves metadata for a BigQuery table.

`bigquery-get-table-info` accepts the following parameters:

- **`table`** (required): The name of the table for which to retrieve metadata.
- **`dataset`** (required): The dataset containing the specified table.
- **`project`** (optional): The Google Cloud project ID. If not provided, the
  tool defaults to the project from the source configuration.

The tool's behavior regarding these parameters is influenced by the
`allowedDatasets` restriction on the `bigquery` source:

- **Without `allowedDatasets` restriction:** The tool can retrieve metadata for
  any table specified by the `table`, `dataset`, and `project` parameters.
- **With `allowedDatasets` restriction:** Before retrieving metadata, the tool
  verifies that the requested dataset is in `allowedDatasets`. If it is not, the
  request is denied. If only one dataset is specified in the `allowedDatasets`
  list, it will be used as the default value for the `dataset` parameter.

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: bigquery_get_table_info
type: bigquery-get-table-info
source: my-bigquery-source
description: Use this tool to get table metadata.
```

## Reference

| **field**   |                  **type**                  | **required** | **description**                                                                                  |
|-------------|:------------------------------------------:|:------------:|--------------------------------------------------------------------------------------------------|
| type        |                   string                   |     true     | Must be "bigquery-get-table-info".                                                               |
| source      |                   string                   |     true     | Name of the source the SQL should execute on.                                                    |
| description |                   string                   |     true     | Description of the tool that is passed to the LLM.                                               |
