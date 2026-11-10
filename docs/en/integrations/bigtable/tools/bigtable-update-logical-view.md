---
title: bigtable-update-logical-view
type: docs
weight: 1
description: "The \"bigtable-update-logical-view\" tool allows you to update an existing bigtable logical view."
---

## About

Update an existing Bigtable logical view.


## Compatible Sources

{{< compatible-sources >}}

## Parameters

`bigtable-update-logical-view` accepts the following parameters:

- **`instance_id`** (string): The ID of the instance
- **`logical_view_id`** (string): The ID of the logical view
- **`query`** (string): The new logical view query

## Example

```yaml
kind: tool
name: bigtable_update_logical_view
type: bigtable-update-logical-view
source: my-bigtable-source
description: Update an existing Bigtable logical view.
```

## Reference

| **field** | **type** | **required** | **description** |
|-----------|----------|--------------|-----------------|
| type | string | true | Must be `bigtable-update-logical-view`. |
| source | string | true | Name of the source to execute on. |
| description | string | false | Description of the tool that is passed to the LLM. |
