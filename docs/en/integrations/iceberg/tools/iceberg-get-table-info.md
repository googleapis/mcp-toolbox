---
title: "iceberg-get-table-info"
type: docs
weight: 3
description: >
  An "iceberg-get-table-info" tool returns an Iceberg table's metadata.
---

## About

An `iceberg-get-table-info` tool returns an Iceberg table's metadata from the
catalog: its schema, partition spec, sort order, table properties, location,
and, when the table has one, a summary of the current snapshot (operation,
timestamp, and the totals the writer recorded, such as row count, data size,
and file count).

Everything returned comes from the catalog's load-table response; the tool
never reads data files from the warehouse.

The tool takes two required parameters: `namespace` (levels separated by dots,
e.g. `accounting.tax`) and `table` (the table name).

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: get_table_info
type: iceberg-get-table-info
source: my-iceberg-source
description: Use this tool to get a table's schema, partitioning, properties, and current snapshot summary.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
| ----------- | :------: | :----------: | -------------------------------------------------- |
| type        |  string  |     true     | Must be "iceberg-get-table-info".                  |
| source      |  string  |     true     | Name of the source the tool should execute on.     |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
