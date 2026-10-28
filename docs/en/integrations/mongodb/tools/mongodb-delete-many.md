---
title: "mongodb-delete-many"
type: docs
weight: 1
description: >
  A "mongodb-delete-many" tool deletes all documents from a MongoDB collection that match a filter.
---

## About

The `mongodb-delete-many` tool performs a **bulk destructive operation**,
deleting **ALL** documents from a collection that match a specified filter.

The tool returns the total count of documents that were deleted. If the filter
does not match any documents (i.e., the deleted count is 0), the tool will
return an error.

## Compatible Sources

{{< compatible-sources >}}

---

## Example

Here is an example that performs a cleanup task by deleting all products from
the `inventory` collection that belong to a discontinued brand.

```yaml
kind: tool
name: retire_brand_products
type: mongodb-delete-many
source: my-mongo-source
description: Deletes all products from a specified discontinued brand.
database: ecommerce
collection: inventory
filterPayload: |
    { "brand_name": {{json .brand_to_delete}} }
filterParams:
  - name: brand_to_delete
    type: string
    description: The name of the discontinued brand whose products should be deleted.
```

## Reference

| **field**     | **type** | **required** | **description**                                                                                                     |
|:--------------|:---------|:-------------|:--------------------------------------------------------------------------------------------------------------------|
| type          | string   | true         | Must be `mongodb-delete-many`.                                                                                      |
| source        | string   | true         | The name of the `mongodb` source to use.                                                                            |
| description   | string   | true         | A description of the tool that is passed to the LLM.                                                                |
| database      | string   | true         | The name of the MongoDB database containing the collection.                                                         |
| collection    | string   | false         | The name of the MongoDB collection from which to delete documents. Mutually exclusive with `collectionAllowedValues`. If omitted, it must be supplied at runtime as a `collection` parameter, and can be restricted with `collectionAllowedValues`. |
| collectionAllowedValues | list     | false        | An optional list of collection names the agent may choose from when `collection` is provided at runtime. Only configure this if collection is omitted. |
| filterPayload | string   | true         | The MongoDB query filter document to select the documents for deletion. Uses `{{json .param_name}}` for templating. |
| filterParams  | list     | false        | A list of parameter objects that define the variables used in the `filterPayload`.                                  |
