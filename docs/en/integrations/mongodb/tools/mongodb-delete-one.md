---
title: "mongodb-delete-one"
type: docs
weight: 1
description: >
  A "mongodb-delete-one" tool deletes a single document from a MongoDB collection.
---

## About

The `mongodb-delete-one` tool performs a destructive operation, deleting the
**first single document** that matches a specified filter from a MongoDB
collection.

If the filter matches multiple documents, only the first one found by the
database will be deleted. This tool is useful for removing specific entries,
such as a user account or a single item from an inventory based on a unique ID.

The tool returns the number of documents deleted, which will be either `1` if a
document was found and deleted, or `0` if no matching document was found.

## Compatible Sources

{{< compatible-sources >}}

---

## Example

Here is an example that deletes a specific user account from the `users`
collection by matching their unique email address. This is a permanent action.

```yaml
kind: tool
name: delete_user_account
type: mongodb-delete-one
source: my-mongo-source
description: Permanently deletes a user account by their email address.
database: user_data
collection: users
filterPayload: |
    { "email": {{json .email_address}} }
filterParams:
  - name: email_address
    type: string
    description: The email of the user account to delete.
```

## Reference

| **field**     | **type** | **required** | **description**                                                                                                    |
|:--------------|:---------|:-------------|:-------------------------------------------------------------------------------------------------------------------|
| type          | string   | true         | Must be `mongodb-delete-one`.                                                                                      |
| source        | string   | true         | The name of the `mongodb` source to use.                                                                           |
| description   | string   | true         | A description of the tool that is passed to the LLM.                                                               |
| database      | string   | true         | The name of the MongoDB database containing the collection.                                                        |
| collection    | string   | false         | The name of the MongoDB collection from which to delete a document. Mutually exclusive with `collectionAllowedValues`. If omitted, it must be supplied at runtime as a `collection` parameter, and can be restricted with `collectionAllowedValues`. |
| collectionAllowedValues | list     | false        | An optional list of collection names the agent may choose from when `collection` is provided at runtime. Only configure this if collection is omitted. |
| filterPayload | string   | true         | The MongoDB query filter document to select the document for deletion. Uses `{{json .param_name}}` for templating. |
| filterParams  | list     | false        | A list of parameter objects that define the variables used in the `filterPayload`.                                 |
