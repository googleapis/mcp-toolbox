---
title: "mongodb-aggregate"
type: docs
weight: 1
description: >
  A "mongodb-aggregate" tool executes a multi-stage aggregation pipeline against a MongoDB collection.
---

## About

The `mongodb-aggregate` tool is the most powerful query tool for MongoDB,
allowing you to process data through a multi-stage pipeline. Each stage
transforms the documents as they pass through, enabling complex operations like
grouping, filtering, reshaping documents, and performing calculations.

The core of this tool is the `pipelinePayload`, which must be a string
containing a **JSON array of pipeline stage documents**. The tool returns a JSON
array of documents produced by the final stage of the pipeline.

A `readOnly` flag can be set to `true` as a safety measure to ensure the
pipeline does not contain any write stages (like `$out` or `$merge`).

## Compatible Sources

{{< compatible-sources >}}

## Example

Here is an example that calculates the average price and total count of products
for each category, but only for products with an "active" status.

```yaml
kind: tool
name: get_category_stats
type: mongodb-aggregate
source: my-mongo-source
description: Calculates average price and count of products, grouped by category.
database: ecommerce
collection: products
readOnly: true
pipelinePayload: |
  [
    {
      "$match": {
        "status": {{json .status_filter}}
      }
    },
    {
      "$group": {
        "_id": "$category",
        "average_price": { "$avg": "$price" },
        "item_count": { "$sum": 1 }
      }
    },
    {
      "$sort": {
        "average_price": -1
      }
    }
  ]
pipelineParams:
  - name: status_filter
    type: string
    description: The product status to filter by (e.g., "active").
```

## Reference

| **field**       | **type** | **required** | **description**                                                                                                |
|:----------------|:---------|:-------------|:---------------------------------------------------------------------------------------------------------------|
| type            | string   | true         | Must be `mongodb-aggregate`.                                                                                   |
| source          | string   | true         | The name of the `mongodb` source to use.                                                                       |
| description     | string   | true         | A description of the tool that is passed to the LLM.                                                           |
| database        | string   | true         | The name of the MongoDB database containing the collection.                                                    |
| collection      | string   | false         | The name of the MongoDB collection to run the aggregation on. Mutually exclusive with `collectionAllowedValues`. If omitted, it must be supplied at runtime as a `collection` parameter, and can be restricted with `collectionAllowedValues`. |
| collectionAllowedValues | list     | false        | An optional list of collection names the agent may choose from when `collection` is provided at runtime. Only configure this if collection is omitted. |
| pipelinePayload | string   | true         | A JSON array of aggregation stage documents, provided as a string. Uses `{{json .param_name}}` for templating. |
| pipelineParams  | list     | true         | A list of parameter objects that define the variables used in the `pipelinePayload`.                           |
| canonical       | bool     | false        | Determines if the pipeline string is parsed using MongoDB's Canonical or Relaxed Extended JSON format.         |
| readOnly        | bool     | false        | If `true`, the tool will fail if the pipeline contains write stages (`$out` or `$merge`). Defaults to `false`. |
