---
title: "bigtable-delete-materialized-view"
type: docs
weight: 1
description: >
  A "bigtable-delete-materialized-view" tool is used to delete an existing materialized view from a Google Cloud Bigtable instance.
---

## About

A `bigtable-delete-materialized-view` tool deletes an existing materialized view from a Google Cloud Bigtable instance.

## Compatible Sources

{{< compatible-sources >}}

## Parameters

| **field** | **type** | **required** | **description** |
|-----------|----------|--------------|-----------------|
| `instance_id` | string | true | The ID of the instance. |
| `materialized_view_id` | string | true | The ID of the materialized view. |

## Example

```yaml
kind: tool
name: delete_materialized_view
type: bigtable-delete-materialized-view
source: my-bigtable-source
description: Use this tool to delete a materialized view.
```

## Reference

| **field** | **type** | **required** | **description** |
|-----------|----------|--------------|-----------------|
| type | string | true | Must be `bigtable-delete-materialized-view`. |
| source | string | true | Name of the source to execute on. |
| description | string | false | Description of the tool that is passed to the LLM. |
