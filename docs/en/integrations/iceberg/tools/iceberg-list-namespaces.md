---
title: "iceberg-list-namespaces"
type: docs
weight: 1
description: >
  An "iceberg-list-namespaces" tool lists the namespaces in an Iceberg
  catalog.
---

## About

An `iceberg-list-namespaces` tool lists the namespaces in an Iceberg catalog.
It takes one optional parameter, `parent`: when set, only the namespaces
nested under that namespace are listed, with levels separated by dots (e.g.
`accounting.tax`); when empty, the top-level namespaces are listed.

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: list_namespaces
type: iceberg-list-namespaces
source: my-iceberg-source
description: Use this tool to list the namespaces in the Iceberg catalog.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
| ----------- | :------: | :----------: | -------------------------------------------------- |
| type        |  string  |     true     | Must be "iceberg-list-namespaces".                 |
| source      |  string  |     true     | Name of the source the tool should execute on.     |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
