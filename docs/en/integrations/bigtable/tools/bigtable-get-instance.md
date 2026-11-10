---
title: bigtable-get-instance
type: docs
weight: 1
description: "The \"bigtable-get-instance\" tool allows you to get details of a bigtable instance."
---

## About

Get details of a Bigtable instance.


## Compatible Sources

{{< compatible-sources >}}

## Parameters

`bigtable-get-instance` accepts the following parameters:

- **`instance_id`** (string): The ID of the instance to get

## Example

```yaml
kind: tool
name: bigtable_get_instance
type: bigtable-get-instance
source: my-bigtable-source
description: Get details of a Bigtable instance.
```

## Reference

| **field** | **type** | **required** | **description** |
|-----------|----------|--------------|-----------------|
| type | string | true | Must be `bigtable-get-instance`. |
| source | string | true | Name of the source to execute on. |
| description | string | false | Description of the tool that is passed to the LLM. |
