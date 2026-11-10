---
title: bigtable-create-cluster
type: docs
weight: 1
description: "The \"bigtable-create-cluster\" tool allows you to create a new bigtable cluster in an instance."
---

## About

Create a new Bigtable cluster in an instance.


## Compatible Sources

{{< compatible-sources >}}

## Parameters

`bigtable-create-cluster` accepts the following parameters:

- **`instance_id`** (string): The ID of the instance
- **`cluster_id`** (string): The ID of the cluster
- **`zone`** (string): The zone for the cluster (e.g. us-central1-b)
- **`num_nodes`** (integer): The number of nodes to allocate

## Example

```yaml
kind: tool
name: bigtable_create_cluster
type: bigtable-create-cluster
source: my-bigtable-source
description: Create a new Bigtable cluster in an instance.
```

## Reference

| **field** | **type** | **required** | **description** |
|-----------|----------|--------------|-----------------|
| type | string | true | Must be `bigtable-create-cluster`. |
| source | string | true | Name of the source to execute on. |
| description | string | false | Description of the tool that is passed to the LLM. |
