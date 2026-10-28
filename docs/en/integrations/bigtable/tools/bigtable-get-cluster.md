---
title: bigtable-get-cluster
type: docs
weight: 1
description: "The \"bigtable-get-cluster\" tool allows you to get details of a bigtable cluster."
---

## About

Get details of a Bigtable cluster.


## Compatible Sources

{{< compatible-sources >}}

## Parameters

`bigtable-get-cluster` accepts the following parameters:

- **`instance_id`** (string): The ID of the instance
- **`cluster_id`** (string): The ID of the cluster

## Example

```yaml
kind: tool
name: bigtable_get_cluster
type: bigtable-get-cluster
source: my-bigtable-source
description: Get details of a Bigtable cluster.
```

## Reference

| **field** | **type** | **required** | **description** |
|-----------|----------|--------------|-----------------|
| type | string | true | Must be `bigtable-get-cluster`. |
| source | string | true | Name of the source to execute on. |
| description | string | false | Description of the tool that is passed to the LLM. |
