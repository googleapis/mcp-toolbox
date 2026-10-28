---
title: bigtable-list-tables
type: docs
weight: 1
description: "The \"bigtable-list-tables\" tool allows you to list all bigtable tables in the instance."
---

## About

List all Bigtable tables in the instance.

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: bigtable_list_tables
type: bigtable-list-tables
source: my-bigtable-source
description: List all Bigtable tables in the instance.
```

## Reference

| **field** | **type** | **required** | **description** |
|-----------|----------|--------------|-----------------|
| type | string | true | Must be `bigtable-list-tables`. |
| source | string | true | Name of the source to execute on. |
| description | string | false | Description of the tool that is passed to the LLM. |
