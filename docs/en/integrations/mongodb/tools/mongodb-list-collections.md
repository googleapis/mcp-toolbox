---
title: "mongodb-list-collections"
type: docs
weight: 2
description: >
  A "mongodb-list-collections" tool lists collection names in a MongoDB database.
---

## About

A `mongodb-list-collections` tool lists the names of collections in a configured
MongoDB database. It helps agents discover available collections before invoking
tools that operate on a collection.

The tool returns a JSON array of collection names and does not require invocation
parameters.

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: list_collections
type: mongodb-list-collections
source: my-mongo-source
database: app
description: List collections in the app database.
```

## Reference

| **field**   | **type** | **required** | **description**                                             |
|:------------|:---------|:-------------|:------------------------------------------------------------|
| type        | string   | true         | Must be `mongodb-list-collections`.                         |
| source      | string   | true         | The name of the `mongodb` source to use.                    |
| database    | string   | true         | The name of the MongoDB database whose collections to list. |
| description | string   | true         | A description of the tool that is passed to the LLM.         |
