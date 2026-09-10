---
title: "iceberg-list-tables"
type: docs
weight: 2
description: >
  An "iceberg-list-tables" tool lists the tables in a namespace of an Iceberg
  catalog.
---

## About

An `iceberg-list-tables` tool lists the tables in a namespace of an Iceberg
catalog. It takes one required parameter, `namespace`, with namespace levels
separated by dots (e.g. `accounting.tax`).

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: list_tables
type: iceberg-list-tables
source: my-iceberg-source
description: Use this tool to list the tables in a namespace.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
| ----------- | :------: | :----------: | -------------------------------------------------- |
| type        |  string  |     true     | Must be "iceberg-list-tables".                     |
| source      |  string  |     true     | Name of the source the tool should execute on.     |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
