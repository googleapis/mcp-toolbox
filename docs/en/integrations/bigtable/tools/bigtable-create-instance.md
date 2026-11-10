---
title: bigtable-create-instance
type: docs
weight: 1
description: "The \"bigtable-create-instance\" tool allows you to create a new bigtable instance."
---

## About

Create a new Bigtable instance.


## Compatible Sources

{{< compatible-sources >}}

## Parameters

`bigtable-create-instance` accepts the following parameters:

- **`instance_id`** (string): The ID of the instance to create
- **`display_name`** (string): Display name for the instance
- **`cluster_id`** (string): The ID of the primary cluster
- **`zone`** (string): The zone for the cluster (e.g. us-central1-b)
- **`num_nodes`** (integer): The number of nodes for the cluster

## Example

```yaml
kind: tool
name: bigtable_create_instance
type: bigtable-create-instance
source: my-bigtable-source
description: Create a new Bigtable instance.
```

## Reference

| **field** | **type** | **required** | **description** |
|-----------|----------|--------------|-----------------|
| type | string | true | Must be `bigtable-create-instance`. |
| source | string | true | Name of the source to execute on. |
| description | string | false | Description of the tool that is passed to the LLM. |
