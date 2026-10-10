---
title: bigtable-create-table
type: docs
weight: 1
description: "The \"bigtable-create-table\" tool allows you to create a new bigtable table."
---

## About

Create a new Bigtable table.


## Compatible Sources

{{< compatible-sources >}}

## Parameters

`bigtable-create-table` accepts the following parameters:

- **`table_id`** (string): The ID of the table to create
- **`column_family`** (string): Optional column family name to create with the table

## Example

```yaml
kind: tool
name: bigtable_create_table
type: bigtable-create-table
source: my-bigtable-source
description: Create a new Bigtable table.
```

## Reference

| **field** | **type** | **required** | **description** |
|-----------|----------|--------------|-----------------|
| type | string | true | Must be `bigtable-create-table`. |
| source | string | true | Name of the source to execute on. |
| description | string | false | Description of the tool that is passed to the LLM. |
