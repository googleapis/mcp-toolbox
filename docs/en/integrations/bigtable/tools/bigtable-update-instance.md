---
title: bigtable-update-instance
type: docs
weight: 1
description: "The \"bigtable-update-instance\" tool allows you to update an existing bigtable instance."
---

## About

Update an existing Bigtable instance.


## Compatible Sources

{{< compatible-sources >}}

## Parameters

`bigtable-update-instance` accepts the following parameters:

- **`instance_id`** (string): The ID of the instance to update
- **`display_name`** (string): The new display name

## Example

```yaml
kind: tool
name: bigtable_update_instance
type: bigtable-update-instance
source: my-bigtable-source
description: Update an existing Bigtable instance.
```

## Reference

| **field** | **type** | **required** | **description** |
|-----------|----------|--------------|-----------------|
| type | string | true | Must be `bigtable-update-instance`. |
| source | string | true | Name of the source to execute on. |
| description | string | false | Description of the tool that is passed to the LLM. |
