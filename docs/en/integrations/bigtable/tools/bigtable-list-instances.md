---
title: bigtable-list-instances
type: docs
weight: 1
description: "The \"bigtable-list-instances\" tool allows you to list all bigtable instances in the project."
---

## About

List all Bigtable instances in the project.

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: bigtable_list_instances
type: bigtable-list-instances
source: my-bigtable-source
description: List all Bigtable instances in the project.
```

## Reference

| **field** | **type** | **required** | **description** |
|-----------|----------|--------------|-----------------|
| type | string | true | Must be `bigtable-list-instances`. |
| source | string | true | Name of the source to execute on. |
| description | string | false | Description of the tool that is passed to the LLM. |
