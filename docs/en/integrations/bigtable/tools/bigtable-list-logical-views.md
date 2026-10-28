---
title: bigtable-list-logical-views
type: docs
weight: 1
description: "The \"bigtable-list-logical-views\" tool allows you to list all bigtable logical views in the instance."
---

## About

List all Bigtable logical views in the instance.


## Compatible Sources

{{< compatible-sources >}}

## Parameters

`bigtable-list-logical-views` accepts the following parameters:

- **`instance_id`** (string): The ID of the instance

## Example

```yaml
kind: tool
name: bigtable_list_logical_views
type: bigtable-list-logical-views
source: my-bigtable-source
description: List all Bigtable logical views in the instance.
```

## Reference

| **field** | **type** | **required** | **description** |
|-----------|----------|--------------|-----------------|
| type | string | true | Must be `bigtable-list-logical-views`. |
| source | string | true | Name of the source to execute on. |
| description | string | false | Description of the tool that is passed to the LLM. |
