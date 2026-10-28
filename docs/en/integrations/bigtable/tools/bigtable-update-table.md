---
title: bigtable-update-table
type: docs
weight: 1
description: "The \"bigtable-update-table\" tool allows you to update an existing bigtable table's configuration."
---

## About

Update an existing Bigtable table's configuration.


## Compatible Sources

{{< compatible-sources >}}

## Parameters

`bigtable-update-table` accepts the following parameters:

- **`table_id`** (string): The ID of the table to update
- **`disable_change_stream`** (boolean): Disable change stream

## Example

```yaml
kind: tool
name: bigtable_update_table
type: bigtable-update-table
source: my-bigtable-source
description: Update an existing Bigtable table's configuration.
```

## Reference

| **field** | **type** | **required** | **description** |
|-----------|----------|--------------|-----------------|
| type | string | true | Must be `bigtable-update-table`. |
| source | string | true | Name of the source to execute on. |
| description | string | false | Description of the tool that is passed to the LLM. |
