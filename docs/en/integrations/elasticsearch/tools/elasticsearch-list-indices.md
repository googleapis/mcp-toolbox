---
title: "elasticsearch-list-indices"
type: docs
weight: 4
description: >
  List the indices of an Elasticsearch cluster.
---

## About

List the indices of an Elasticsearch cluster, so an agent with no prior
knowledge of the cluster can discover index names.

The tool is read-only. It calls the
[cat indices API](https://www.elastic.co/docs/api/doc/elasticsearch/operation/operation-cat-indices)
and returns one object per index, sorted by index name.

Indices whose names start with `.` (for example `.internal.alerts-*`) are left
out of the result, unless at least one pattern in the `patterns` parameter starts with `.`.

## Compatible Sources

{{< compatible-sources >}}

## Parameters

| **name**      | **type** | **required** | **description**                                                                                                                                                              |
|---------------|:--------:|:------------:|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| patterns      |  string  |    false     | One or more index names or wildcard patterns, comma-separated, e.g. `logs-*,metrics-*`. Default is `*`.                                                                      |
| output_format |  string  |    false     | `simple` (default) returns index, health, status, docs_count, store_size. `detailed` also returns pri, rep, docs_deleted, creation_date, uuid. Any other value is rejected. |

## Example

```yaml
kind: tool
name: list_indices
type: elasticsearch-list-indices
source: elasticsearch-source
description: Use this tool to list the indices of the Elasticsearch cluster.
```

## Output Format

A list of objects, one per index, sorted by index name. Values are returned as
strings, the way Elasticsearch returns them.

With `output_format: simple`:

```json
[
  {
    "index": "people",
    "health": "green",
    "status": "open",
    "docs_count": "5",
    "store_size": "12.3kb"
  }
]
```

With `output_format: detailed`:

```json
[
  {
    "index": "people",
    "health": "green",
    "status": "open",
    "docs_count": "5",
    "store_size": "12.3kb",
    "pri": "1",
    "rep": "1",
    "docs_deleted": "0",
    "creation_date": "2026-11-20T10:15:30.123Z",
    "uuid": "kZ1q2x3VQ0aXb9mXyZ_abc"
  }
]
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "elasticsearch-list-indices".              |
| source      |  string  |     true     | Name of the source to list the indices of.         |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
