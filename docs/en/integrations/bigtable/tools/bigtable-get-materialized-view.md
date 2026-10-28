---
title: "bigtable-get-materialized-view"
type: docs
weight: 1
description: >
  A "bigtable-get-materialized-view" tool is used to retrieve information about a materialized view on a Google Cloud Bigtable instance.
---

## About

A `bigtable-get-materialized-view` tool retrieves metadata and information about a materialized view on a Google Cloud Bigtable instance.

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
name: get_materialized_view
type: bigtable-get-materialized-view
source: my-bigtable-source
description: Use this tool to get materialized view info.
```

## Reference

| **field** | **type** | **required** | **description** |
|-----------|----------|--------------|-----------------|
| type | string | true | Must be `bigtable-get-materialized-view`. |
| source | string | true | Name of the source to execute on. |
| description | string | false | Description of the tool that is passed to the LLM. |
