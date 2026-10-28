---
title: bigtable-list-clusters
type: docs
weight: 1
description: "The \"bigtable-list-clusters\" tool allows you to list all bigtable clusters in the instance."
---

## About

List all Bigtable clusters in the instance.


## Compatible Sources

{{< compatible-sources >}}

## Parameters

`bigtable-list-clusters` accepts the following parameters:

- **`instance_id`** (string): The ID of the instance

## Example

```yaml
kind: tool
name: bigtable_list_clusters
type: bigtable-list-clusters
source: my-bigtable-source
description: List all Bigtable clusters in the instance.
```

## Reference

| **field** | **type** | **required** | **description** |
|-----------|----------|--------------|-----------------|
| type | string | true | Must be `bigtable-list-clusters`. |
| source | string | true | Name of the source to execute on. |
| description | string | false | Description of the tool that is passed to the LLM. |
