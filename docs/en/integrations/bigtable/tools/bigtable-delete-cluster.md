---
title: bigtable-delete-cluster
type: docs
weight: 1
description: "The \"bigtable-delete-cluster\" tool allows you to delete a bigtable cluster."
---

## About

Delete a Bigtable cluster.


## Compatible Sources

{{< compatible-sources >}}

## Parameters

`bigtable-delete-cluster` accepts the following parameters:

- **`instance_id`** (string): The ID of the instance
- **`cluster_id`** (string): The ID of the cluster

## Example

```yaml
kind: tool
name: bigtable_delete_cluster
type: bigtable-delete-cluster
source: my-bigtable-source
description: Delete a Bigtable cluster.
```

## Reference

| **field** | **type** | **required** | **description** |
|-----------|----------|--------------|-----------------|
| type | string | true | Must be `bigtable-delete-cluster`. |
| source | string | true | Name of the source to execute on. |
| description | string | false | Description of the tool that is passed to the LLM. |
