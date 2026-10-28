---
title: "mongodb-find-one"
type: docs
weight: 1
description: >
  A "mongodb-find-one" tool finds and retrieves a single document from a MongoDB collection.
---

## About

A `mongodb-find-one` tool is used to retrieve the **first single document** that
matches a specified filter from a MongoDB collection. If multiple documents
match the filter, you can use `sort` options to control which document is
returned. Otherwise, the selection is not guaranteed.

The tool returns a single JSON object representing the document, wrapped in a
JSON array.

## Compatible Sources

{{< compatible-sources >}}
---

## Example

Here's a common use case: finding a specific user by their unique email address
and returning their profile information, while excluding sensitive fields like
the password hash.

```yaml
kind: tool
name: get_user_profile
type: mongodb-find-one
source: my-mongo-source
description: Retrieves a user's profile by their email address.
database: user_data
collection: profiles
filterPayload: |
    { "email": {{json .email}} }
filterParams:
  - name: email
    type: string
    description: The email address of the user to find.
projectPayload: |
    { 
      "password_hash": 0,
      "login_history": 0
    }
```

## Reference

| **field**      | **type** | **required** | **description**                                                                                                                              |
|:---------------|:---------|:-------------|:---------------------------------------------------------------------------------------------------------------------------------------------|
| type           | string   | true         | Must be `mongodb-find-one`.                                                                                                                  |
| source         | string   | true         | The name of the `mongodb` source to use.                                                                                                     |
| description    | string   | true         | A description of the tool that is passed to the LLM.                                                                                         |
| database       | string   | true         | The name of the MongoDB database to query.                                                                                                   |
| collection     | string   | false         | The name of the MongoDB collection to query. Mutually exclusive with `collectionAllowedValues`. If omitted, it must be supplied at runtime as a `collection` parameter, and can be restricted with `collectionAllowedValues`. |
| collectionAllowedValues | list     | false        | An optional list of collection names the agent may choose from when `collection` is provided at runtime. Only configure this if collection is omitted. |
| filterPayload  | string   | true         | The MongoDB query filter document to select the document. Uses `{{json .param_name}}` for templating.                                        |
| filterParams   | list     | false        | A list of parameter objects that define the variables used in the `filterPayload`.                                                           |
| projectPayload | string   | false        | An optional MongoDB projection document to specify which fields to include (1) or exclude (0) in the result.                                 |
| projectParams  | list     | false        | A list of parameter objects for the `projectPayload`.                                                                                        |
