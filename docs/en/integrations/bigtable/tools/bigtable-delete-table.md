---
title: bigtable-delete-table
type: docs
weight: 1
description: "The \"bigtable-delete-table\" tool allows you to delete a bigtable table."
---

## About

Delete a Bigtable table.


## Compatible Sources

{{< compatible-sources >}}

## Parameters

`bigtable-delete-table` accepts the following parameters:

- **`table_id`** (string): The ID of the table to delete

## Example

```yaml
kind: tool
name: bigtable_delete_table
type: bigtable-delete-table
source: my-bigtable-source
description: Delete a Bigtable table.
```

## Reference

| **field** | **type** | **required** | **description** |
|-----------|----------|--------------|-----------------|
| type | string | true | Must be `bigtable-delete-table`. |
| source | string | true | Name of the source to execute on. |
| description | string | false | Description of the tool that is passed to the LLM. |
