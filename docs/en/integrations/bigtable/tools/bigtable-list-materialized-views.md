---
title: "bigtable-list-materialized-views"
type: docs
weight: 1
description: >
  A "bigtable-list-materialized-views" tool is used to list all materialized views on a Google Cloud Bigtable instance.
---

## About

A `bigtable-list-materialized-views` tool lists all materialized views on a Google Cloud Bigtable instance.

## Compatible Sources

{{< compatible-sources >}}

## Parameters

| **field** | **type** | **required** | **description** |
|-----------|----------|--------------|-----------------|
| `instance_id` | string | true | The ID of the instance. |

## Example

```yaml
kind: tool
name: list_materialized_views
type: bigtable-list-materialized-views
source: my-bigtable-source
description: Use this tool to list materialized views.
```

## Reference

| **field** | **type** | **required** | **description** |
|-----------|----------|--------------|-----------------|
| type | string | true | Must be `bigtable-list-materialized-views`. |
| source | string | true | Name of the source to execute on. |
| description | string | false | Description of the tool that is passed to the LLM. |
