---
title: bigtable-create-logical-view
type: docs
weight: 1
description: "The \"bigtable-create-logical-view\" tool allows you to create a new bigtable logical view."
---

## About

Create a new Bigtable logical view.


## Compatible Sources

{{< compatible-sources >}}

## Parameters

`bigtable-create-logical-view` accepts the following parameters:

- **`instance_id`** (string): The ID of the instance
- **`logical_view_id`** (string): The ID of the logical view
- **`query`** (string): The logical view query

## Example

```yaml
kind: tool
name: bigtable_create_logical_view
type: bigtable-create-logical-view
source: my-bigtable-source
description: Create a new Bigtable logical view.
```

## Reference

| **field** | **type** | **required** | **description** |
|-----------|----------|--------------|-----------------|
| type | string | true | Must be `bigtable-create-logical-view`. |
| source | string | true | Name of the source to execute on. |
| description | string | false | Description of the tool that is passed to the LLM. |
