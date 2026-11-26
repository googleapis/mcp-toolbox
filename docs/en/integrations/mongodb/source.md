---
title: "MongoDB Source"
linkTitle: "Source"
type: docs
weight: 1
description: >
  MongoDB is a no-sql data platform that can not only serve general purpose data requirements also perform VectorSearch where both operational data and embeddings used of search can reside in same document.
no_list: true
---

## About

[MongoDB][mongodb-docs] is a popular NoSQL database that stores data in
flexible, JSON-like documents, making it easy to develop and scale applications.

[mongodb-docs]: https://www.mongodb.com/docs/atlas/getting-started/



## Available Tools

{{< list-tools >}}

## Example

```yaml
kind: source
name: my-mongodb
type: mongodb
uri: "mongodb+srv://username:password@host.mongodb.net"

```

Restrict every tool on the source to a fixed set of collections:

```yaml
kind: source
name: my-mongodb
type: mongodb
uri: "mongodb+srv://username:password@host.mongodb.net"
allowedCollections:
  - crm.customers
  - crm.orders
```

## Reference

| **field**          | **type** | **required** | **description**                                                                                                                                       |
|--------------------|:--------:|:------------:|-------------------------------------------------------------------------------------------------------------------------------------------------------|
| type               |  string  |     true     | Must be "mongodb".                                                                                                                                    |
| uri                |  string  |     true     | connection string to connect to MongoDB                                                                                                               |
| allowedCollections |   list   |    false     | An optional list of `database.collection` entries that every tool on this source is restricted to. If omitted, tools may use any collection.          |

## Advanced Usage

### Restricting collections

`allowedCollections` is the authoritative scope for every tool bound to the
source. Each entry must be fully qualified as `database.collection`, because a
MongoDB source has no default database -- each tool names its own. A database
name cannot contain a dot, so the name is split on the first one.

A tool's own `collectionAllowedValues` is an optional second layer that can only
*narrow* the source's list; it can never widen it. The effective set is the
intersection of the two, and it is advertised to the agent as the JSON Schema
`enum` of the runtime `collection` parameter.

| `source.allowedCollections` | tool config                                   | behaviour                                          |
|-----------------------------|-----------------------------------------------|----------------------------------------------------|
| unset                       | fixed `collection: c`                         | allowed, unrestricted                              |
| unset                       | omitted, no `collectionAllowedValues`         | runtime parameter, no `enum`, any collection       |
| unset                       | omitted + `collectionAllowedValues: [a,b]`    | runtime parameter, `enum: [a,b]`                   |
| `[a,b,c]`                   | fixed `collection: a`                         | allowed                                            |
| `[a,b,c]`                   | fixed `collection: z`                         | rejected at startup                                |
| `[a,b,c]`                   | omitted                                       | runtime parameter, `enum: [a,b,c]`                 |
| `[a,b,c]`                   | omitted + `collectionAllowedValues: [a,b]`    | runtime parameter, `enum: [a,b]`                   |
| `[a,b]`                     | omitted + `collectionAllowedValues: [c]`      | rejected at startup, the tool escapes the source   |

A configuration that can never resolve to an allowed collection fails when the
server starts rather than when the agent calls the tool. At invocation time the
collection is checked again against the source, so the `enum` is a hint to the
client and the source remains the guard.

An aggregation pipeline can reach collections other than the one it runs on, so
every collection named by a `$lookup`, `$graphLookup`, `$unionWith`, `$out`,
`$merge` or `$facet` stage, including nested pipelines, must also be allowed by
the source.
