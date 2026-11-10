---
title: "firestore-mongodb-get-schema"
type: docs
weight: 1
description: >
  A "firestore-mongodb-get-schema" tool introspects Firestore collections to infer field types and schema definitions.
---

## About

A `firestore-mongodb-get-schema` tool retrieves schema information and inferred document structures for Firestore collections.

This tool introspects collections to detect field names, nested maps, and data types (such as string, integer, double, boolean, timestamp, geopoint, and reference), returning structured schema definitions.

## Compatible Sources

{{< compatible-sources >}}

## Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `collection` | string | false | Optional name or relative path of a specific collection. If omitted, schemas for all root collections are returned. |

## Example

```yaml
kind: tool
name: get_schema
type: firestore-mongodb-get-schema
source: my-firestore-source
description: Use this tool to retrieve schemas for Firestore collections.
```

## Reference

| **field**   | **type** | **required** | **description** |
|-------------|:--------:|:------------:|-----------------|
| type | string | true | Must be "firestore-mongodb-get-schema". |
| source | string | true | Name of the Firestore source to introspect. |
| description | string | true | Description of the tool that is passed to the LLM. |
