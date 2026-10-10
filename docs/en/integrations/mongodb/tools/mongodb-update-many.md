---
title: "mongodb-update-many"
type: docs
weight: 1
description: >
  A "mongodb-update-many" tool updates all documents in a MongoDB collection that match a filter.
---

## About

A `mongodb-update-many` tool updates **all** documents within a specified
MongoDB collection that match a given filter. It locates the documents using a
`filterPayload` and applies the modifications defined in an `updatePayload`.

The tool returns an array of three integers: `[ModifiedCount, UpsertedCount,
MatchedCount]`.

## Compatible Sources

{{< compatible-sources >}}

---

## Example

Here's an example configuration. This tool applies a discount to all items
within a specific category and also marks them as being on sale.

```yaml
kind: tool
name: apply_category_discount
type: mongodb-update-many
source: my-mongo-source
description: Use this tool to apply a discount to all items in a given category.
database: products
collection: inventory
filterPayload: |
    { "category": {{json .category_name}} }
filterParams:
  - name: category_name
    type: string
    description: The category of items to update.
updatePayload: |
    { 
      "$mul": { "price": {{json .discount_multiplier}} },
      "$set": { "on_sale": true }
    }
updateParams:
  - name: discount_multiplier
    type: number
    description: The multiplier to apply to the price (e.g., 0.8 for a 20% discount).
canonical: false
upsert: false
```

## Reference

| **field**     | **type** | **required** | **description**                                                                                                                                                                                                                                  |
|:--------------|:---------|:-------------|:-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| type          | string   | true         | Must be `mongodb-update-many`.                                                                                                                                                                                                                   |
| source        | string   | true         | The name of the `mongodb` source to use.                                                                                                                                                                                                         |
| description   | string   | true         | A description of the tool that is passed to the LLM.                                                                                                                                                                                             |
| database      | string   | true         | The name of the MongoDB database containing the collection.                                                                                                                                                                                      |
| collection    | string   | false         | The name of the MongoDB collection in which to update documents. Mutually exclusive with `collectionAllowedValues`. If omitted, it must be supplied at runtime as a `collection` parameter, and can be restricted with `collectionAllowedValues`. |
| collectionAllowedValues | list     | false        | An optional list of collection names the agent may choose from when `collection` is provided at runtime. Only configure this if collection is omitted. |
| filterPayload | string   | true         | The MongoDB query filter document to select the documents for updating. It's written as a Go template, using `{{json .param_name}}` to insert parameters.                                                                                        |
| filterParams  | list     | false        | A list of parameter objects that define the variables used in the `filterPayload`.                                                                                                                                                               |
| updatePayload | string   | true         | The MongoDB update document, It's written as a Go template, using `{{json .param_name}}` to insert parameters.                                                                                                                                   |
| updateParams  | list     | true         | A list of parameter objects that define the variables used in the `updatePayload`.                                                                                                                                                               |
| canonical     | bool     | false        | Determines if the `filterPayload` and `updatePayload` strings are parsed using MongoDB's Canonical or Relaxed Extended JSON format. **Canonical** is stricter about type representation, while **Relaxed** is more lenient. Defaults to `false`. |
| upsert        | bool     | false        | If `true`, a new document is created if no document matches the `filterPayload`. Defaults to `false`.                                                                                                                                            |
