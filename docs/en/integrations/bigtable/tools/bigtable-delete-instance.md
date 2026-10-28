---
title: bigtable-delete-instance
type: docs
weight: 1
description: "The \"bigtable-delete-instance\" tool allows you to delete a bigtable instance."
---

## About

Delete a Bigtable instance.


## Compatible Sources

{{< compatible-sources >}}

## Parameters

`bigtable-delete-instance` accepts the following parameters:

- **`instance_id`** (string): The ID of the instance to delete

## Example

```yaml
kind: tool
name: bigtable_delete_instance
type: bigtable-delete-instance
source: my-bigtable-source
description: Delete a Bigtable instance.
```

## Reference

| **field** | **type** | **required** | **description** |
|-----------|----------|--------------|-----------------|
| type | string | true | Must be `bigtable-delete-instance`. |
| source | string | true | Name of the source to execute on. |
| description | string | false | Description of the tool that is passed to the LLM. |
