---
title: bigtable-get-logical-view
type: docs
weight: 1
description: "The \"bigtable-get-logical-view\" tool allows you to get details of a bigtable logical view."
---

## About

Get details of a Bigtable logical view.


## Compatible Sources

{{< compatible-sources >}}

## Parameters

`bigtable-get-logical-view` accepts the following parameters:

- **`instance_id`** (string): The ID of the instance
- **`logical_view_id`** (string): The ID of the logical view

## Example

```yaml
kind: tool
name: bigtable_get_logical_view
type: bigtable-get-logical-view
source: my-bigtable-source
description: Get details of a Bigtable logical view.
```

## Reference

| **field** | **type** | **required** | **description** |
|-----------|----------|--------------|-----------------|
| type | string | true | Must be `bigtable-get-logical-view`. |
| source | string | true | Name of the source to execute on. |
| description | string | false | Description of the tool that is passed to the LLM. |
