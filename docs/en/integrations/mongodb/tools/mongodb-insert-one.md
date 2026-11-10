---
title: "mongodb-insert-one"
type: docs
weight: 1
description: >
  A "mongodb-insert-one" tool inserts a single new document into a MongoDB collection.
---

## About

The `mongodb-insert-one` tool inserts a **single new document** into a specified
MongoDB collection.

This tool takes one required parameter named `data`, which must be a string
containing the JSON object you want to insert. Upon successful insertion, the
tool returns the unique `_id` of the newly created document.

## Compatible Sources

{{< compatible-sources >}}

## Example

Here is an example configuration for a tool that adds a new user to a `users`
collection.

```yaml
kind: tool
name: create_new_user
type: mongodb-insert-one
source: my-mongo-source
description: Creates a new user record in the database.
database: user_data
collection: users
canonical: false
```

An LLM would call this tool by providing the document as a JSON string in the
`data` parameter, like this:
`tool_code: create_new_user(data='{"email": "new.user@example.com", "name": "Jane Doe", "status": "active"}')`

## Reference

| **field**   | **type** | **required** | **description**                                                                                                         |
|:------------|:---------|:-------------|:------------------------------------------------------------------------------------------------------------------------|
| type        | string   | true         | Must be `mongodb-insert-one`.                                                                                           |
| source      | string   | true         | The name of the `mongodb` source to use.                                                                                |
| description | string   | true         | A description of the tool that is passed to the LLM.                                                                    |
| database    | string   | true         | The name of the MongoDB database containing the collection.                                                             |
| collection  | string   | false         | The name of the MongoDB collection into which the document will be inserted. Mutually exclusive with `collectionAllowedValues`. If omitted, it must be supplied at runtime as a `collection` parameter, and can be restricted with `collectionAllowedValues`. |
| collectionAllowedValues | list     | false        | An optional list of collection names the agent may choose from when `collection` is provided at runtime. Only configure this if collection is omitted. |
| canonical   | bool     | false        | Determines if the data string is parsed using MongoDB's Canonical or Relaxed Extended JSON format. Defaults to `false`. |
