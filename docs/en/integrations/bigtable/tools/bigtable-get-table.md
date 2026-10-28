---
title: bigtable-get-table
type: docs
weight: 1
description: "The \"bigtable-get-table\" tool allows you to get details of a bigtable table."
---

## About

Get details of a Bigtable table.


## Compatible Sources

{{< compatible-sources >}}

## Parameters

`bigtable-get-table` accepts the following parameters:

- **`table_id`** (string): The ID of the table to get

## Example

```yaml
kind: tool
name: bigtable_get_table
type: bigtable-get-table
source: my-bigtable-source
description: Get details of a Bigtable table.
```

## Reference

| **field** | **type** | **required** | **description** |
|-----------|----------|--------------|-----------------|
| type | string | true | Must be `bigtable-get-table`. |
| source | string | true | Name of the source to execute on. |
| description | string | false | Description of the tool that is passed to the LLM. |
