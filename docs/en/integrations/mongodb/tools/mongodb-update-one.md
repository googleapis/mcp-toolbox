---
title: "mongodb-update-one"
type: docs
weight: 1
description: >
  A "mongodb-update-one" tool updates a single document in a MongoDB collection.
---

## About

A `mongodb-update-one` tool updates a single document within a specified MongoDB
collection. It locates the document to be updated using a `filterPayload` and
applies modifications defined in an `updatePayload`. If the filter matches
multiple documents, only the first one found will be updated.

## Compatible Sources

{{< compatible-sources >}}

---

## Example

Here's an example of a `mongodb-update-one` tool configuration. This tool
updates the `stock` and `status` fields of a document in the `inventory`
collection where the `item` field matches a provided value. If no matching
document is found, the `upsert: true` option will create a new one.

```yaml
kind: tool
name: update_inventory_item
type: mongodb-update-one
source: my-mongo-source
description: Use this tool to update an item's stock and status in the inventory.
database: products
collection: inventory
filterPayload: |
    { "item": {{json .item_name}} }
filterParams:
  - name: item_name
    type: string
    description: The name of the item to update.
updatePayload: |
    { "$set": { "stock": {{json .new_stock}}, "status": {{json .new_status}} } }
updateParams:
  - name: new_stock
    type: integer
    description: The new stock quantity.
  - name: new_status
    type: string
    description: The new status of the item (e.g., "In Stock", "Backordered").
canonical: false
upsert: true
```

## Reference

| **field**     | **type** | **required** | **description**                                                                                                                                                                                                                                                        |
|:--------------|:---------|:-------------|:-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| type          | string   | true         | Must be `mongodb-update-one`.                                                                                                                                                                                                                                          |
| source        | string   | true         | The name of the `mongodb` source to use.                                                                                                                                                                                                                               |
| description   | string   | true         | A description of the tool that is passed to the LLM.                                                                                                                                                                                                                   |
| database      | string   | true         | The name of the MongoDB database containing the collection.                                                                                                                                                                                                            |
| collection    | string   | false         | The name of the MongoDB collection to update a document in. Mutually exclusive with `collectionAllowedValues`. If omitted, it must be supplied at runtime as a `collection` parameter, and can be restricted with `collectionAllowedValues`. |
| collectionAllowedValues | list     | false        | An optional list of collection names the agent may choose from when `collection` is provided at runtime. Only configure this if collection is omitted. |
| filterPayload | string   | true         | The MongoDB query filter document to select the document for updating. It's written as a Go template, using `{{json .param_name}}` to insert parameters.                                                                                                               |
| filterParams  | list     | false        | A list of parameter objects that define the variables used in the `filterPayload`.                                                                                                                                                                                     |
| updatePayload | string   | true         | The MongoDB update document, which specifies the modifications. This often uses update operators like `$set`. It's written as a Go template, using `{{json .param_name}}` to insert parameters.                                                                        |
| updateParams  | list     | true         | A list of parameter objects that define the variables used in the `updatePayload`.                                                                                                                                                                                     |
| canonical     | bool     | false        | Determines if the `updatePayload` string is parsed using MongoDB's Canonical or Relaxed Extended JSON format. **Canonical** is stricter about type representation (e.g., `{"$numberInt": "42"}`), while **Relaxed** is more lenient (e.g., `42`). Defaults to `false`. |
| upsert        | bool     | false        | If `true`, a new document is created if no document matches the `filterPayload`. Defaults to `false`.                                                                                                                                                                  |
