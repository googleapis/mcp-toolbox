---
title: bigtable-delete-logical-view
type: docs
weight: 1
description: "The \"bigtable-delete-logical-view\" tool allows you to delete a bigtable logical view."
---

## About

Delete a Bigtable logical view.


## Compatible Sources

{{< compatible-sources >}}

## Parameters

`bigtable-delete-logical-view` accepts the following parameters:

- **`instance_id`** (string): The ID of the instance
- **`logical_view_id`** (string): The ID of the logical view

## Example

```yaml
kind: tool
name: bigtable_delete_logical_view
type: bigtable-delete-logical-view
source: my-bigtable-source
description: Delete a Bigtable logical view.
```

## Reference

| **field** | **type** | **required** | **description** |
|-----------|----------|--------------|-----------------|
| type | string | true | Must be `bigtable-delete-logical-view`. |
| source | string | true | Name of the source to execute on. |
| description | string | false | Description of the tool that is passed to the LLM. |
