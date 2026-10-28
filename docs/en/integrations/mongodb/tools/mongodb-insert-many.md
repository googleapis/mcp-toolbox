---
title: "mongodb-insert-many"
type: docs
weight: 1
description: >
  A "mongodb-insert-many" tool inserts multiple new documents into a MongoDB collection.
---

## About

The `mongodb-insert-many` tool inserts **multiple new documents** into a
specified MongoDB collection in a single bulk operation. This is highly
efficient for adding large amounts of data at once.

This tool takes one required parameter named `data`. This `data` parameter must
be a string containing a **JSON array of document objects**. Upon successful
insertion, the tool returns a JSON array containing the unique `_id` of **each**
new document that was created.

## Compatible Sources

{{< compatible-sources >}}

---

## Example

Here is an example configuration for a tool that logs multiple events at once.

```yaml
kind: tool
name: log_batch_events
type: mongodb-insert-many
source: my-mongo-source
description: Inserts a batch of event logs into the database.
database: logging
collection: events
canonical: true
```

An LLM would call this tool by providing an array of documents as a JSON string
in the `data` parameter, like this:
`tool_code: log_batch_events(data='[{"event": "login", "user": "user1"}, {"event": "click", "user": "user2"}, {"event": "logout", "user": "user1"}]')`

---

## Reference

| **field**   | **type** | **required** | **description**                                                                                                         |
|:------------|:---------|:-------------|:------------------------------------------------------------------------------------------------------------------------|
| type        | string   | true         | Must be `mongodb-insert-many`.                                                                                          |
| source      | string   | true         | The name of the `mongodb` source to use.                                                                                |
| description | string   | true         | A description of the tool that is passed to the LLM.                                                                    |
| database    | string   | true         | The name of the MongoDB database containing the collection.                                                             |
| collection  | string   | false         | The name of the MongoDB collection into which the documents will be inserted. Mutually exclusive with `collectionAllowedValues`. If omitted, it must be supplied at runtime as a `collection` parameter, and can be restricted with `collectionAllowedValues`. |
| collectionAllowedValues | list     | false        | An optional list of collection names the agent may choose from when `collection` is provided at runtime. Only configure this if collection is omitted. |
| canonical   | bool     | false        | Determines if the data string is parsed using MongoDB's Canonical or Relaxed Extended JSON format. Defaults to `false`. |
