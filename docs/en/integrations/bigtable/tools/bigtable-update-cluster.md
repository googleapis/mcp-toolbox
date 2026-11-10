---
title: bigtable-update-cluster
type: docs
weight: 1
description: "The \"bigtable-update-cluster\" tool allows you to update the number of nodes in a bigtable cluster."
---

## About

Update the number of nodes in a Bigtable cluster.


## Compatible Sources

{{< compatible-sources >}}

## Parameters

`bigtable-update-cluster` accepts the following parameters:

- **`instance_id`** (string): The ID of the instance
- **`cluster_id`** (string): The ID of the cluster
- **`serve_nodes`** (integer): The new number of nodes to allocate

## Example

```yaml
kind: tool
name: bigtable_update_cluster
type: bigtable-update-cluster
source: my-bigtable-source
description: Update the number of nodes in a Bigtable cluster.
```

## Reference

| **field** | **type** | **required** | **description** |
|-----------|----------|--------------|-----------------|
| type | string | true | Must be `bigtable-update-cluster`. |
| source | string | true | Name of the source to execute on. |
| description | string | false | Description of the tool that is passed to the LLM. |
