---
title: "bigtable-update-materialized-view"
type: docs
weight: 1
description: >
  A "bigtable-update-materialized-view" tool is used to update an existing materialized view on a Google Cloud Bigtable instance.
---

## About

A `bigtable-update-materialized-view` tool updates an existing materialized view on a Google Cloud Bigtable instance.

## Compatible Sources

{{< compatible-sources >}}

## Parameters

| **field** | **type** | **required** | **description** |
|-----------|----------|--------------|-----------------|
| `instance_id` | string | true | The ID of the instance. |
| `materialized_view_id` | string | true | The ID of the materialized view. |
| `query` | string | true | The updated materialized view query. |

## Example

```yaml
kind: tool
name: update_materialized_view
type: bigtable-update-materialized-view
source: my-bigtable-source
description: Use this tool to update a materialized view.
```

## Reference

| **field** | **type** | **required** | **description** |
|-----------|----------|--------------|-----------------|
| type | string | true | Must be `bigtable-update-materialized-view`. |
| source | string | true | Name of the source to execute on. |
| description | string | false | Description of the tool that is passed to the LLM. |
