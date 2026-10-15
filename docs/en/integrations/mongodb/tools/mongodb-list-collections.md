---
title: "mongodb-list-collections"
type: docs
weight: 1
description: >
  A "mongodb-list-collections" tool lists the collections in a MongoDB database.
---

## About

A `mongodb-list-collections` tool lists the collections that exist in a MongoDB
database, so an agent can discover what it may query instead of guessing a
collection name.

If the source is configured with
[`allowedCollections`](../source.md), the tool returns only the allowed
collections that actually exist, and the server refuses to start if the source
allows no collections in the tool's database. The tool takes no parameters; the
database comes from its configuration.

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: list_collections
type: mongodb-list-collections
source: my-mongo-source
database: crm
description: List the collections available in the database.
```

## Output Format

The tool returns a JSON array of collection names, sorted alphabetically.

```json
["customers", "orders"]
```

## Reference

| **field**   | **type** | **required** | **description**                                       |
|:------------|:---------|:-------------|:--------------------------------------------------------|
| type        | string   | true         | Must be `mongodb-list-collections`.                   |
| source      | string   | true         | The name of the `mongodb` source to use.              |
| description | string   | true         | A description of the tool that is passed to the LLM.  |
| database    | string   | true         | The name of the MongoDB database whose collections are listed. |
