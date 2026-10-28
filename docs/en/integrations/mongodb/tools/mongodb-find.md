---
title: "mongodb-find"
type: docs
weight: 1
description: >
  A "mongodb-find" tool finds and retrieves documents from a MongoDB collection.
---

## About

A `mongodb-find` tool is used to query a MongoDB collection and retrieve
documents that match a specified filter. It's a flexible tool that allows you to
shape the output by selecting specific fields (**projection**), ordering the
results (**sorting**), and restricting the number of documents returned
(**limiting**).

The tool returns a JSON array of the documents found.

## Compatible Sources

{{< compatible-sources >}}

## Example

Here's an example that finds up to 10 users from the `customers` collection who
live in a specific city. The results are sorted by their last name, and only
their first name, last name, and email are returned.

```yaml
kind: tool
name: find_local_customers
type: mongodb-find
source: my-mongo-source
description: Finds customers by city, sorted by last name.
database: crm
collection: customers
limit: 10
filterPayload: |
    { "address.city": {{json .city}} }
filterParams:
  - name: city
    type: string
    description: The city to search for customers in.
projectPayload: |
    { 
      "first_name": 1,
      "last_name": 1,
      "email": 1,
      "_id": 0
    }
sortPayload: |
    { "last_name": {{json .sort_order}} }
sortParams:
  - name: sort_order
    type: integer
    description: The sort order (1 for ascending, -1 for descending).
```

## Reference

| **field**      | **type** | **required** | **description**                                                                                                             |
|:---------------|:---------|:-------------|:----------------------------------------------------------------------------------------------------------------------------|
| type           | string   | true         | Must be `mongodb-find`.                                                                                                     |
| source         | string   | true         | The name of the `mongodb` source to use.                                                                                    |
| description    | string   | true         | A description of the tool that is passed to the LLM.                                                                        |
| database       | string   | true         | The name of the MongoDB database to query.                                                                                  |
| collection     | string   | false         | The name of the MongoDB collection to query. Mutually exclusive with `collectionAllowedValues`. If omitted, it must be supplied at runtime as a `collection` parameter, and can be restricted with `collectionAllowedValues`. |
| collectionAllowedValues | list     | false        | An optional list of collection names the agent may choose from when `collection` is provided at runtime. Only configure this if collection is omitted. |
| filterPayload  | string   | true         | The MongoDB query filter document to select which documents to return. Uses `{{json .param_name}}` for templating.          |
| filterParams   | list     | false        | A list of parameter objects that define the variables used in the `filterPayload`.                                          |
| projectPayload | string   | false        | An optional MongoDB projection document to specify which fields to include (1) or exclude (0) in the results.               |
| projectParams  | list     | false        | A list of parameter objects for the `projectPayload`.                                                                       |
| sortPayload    | string   | false        | An optional MongoDB sort document to define the order of the returned documents. Use 1 for ascending and -1 for descending. |
| sortParams     | list     | false        | A list of parameter objects for the `sortPayload`.                                                                          |
| limit          | integer  | false        | An optional integer specifying the maximum number of documents to return.                                                   |
