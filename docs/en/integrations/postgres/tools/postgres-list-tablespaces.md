---
title: "postgres-list-tablespaces"
type: docs
weight: 1
description: >
 The "postgres-list-tablespaces" tool lists tablespaces in a Postgres database.
---

## About

The `postgres-list-tablespaces` tool lists available tablespaces in the database.

`postgres-list-tablespaces` lists detailed information as JSON for tablespaces. The tool takes the following input parameters:

- `tablespace_name` (optional): A text to filter results by tablespace name. Default: `""`
- `limit` (optional): The maximum number of tablespaces to return. Default: `50`

## Compatible Sources

{{< compatible-sources others="integrations/alloydb, integrations/cloud-sql-pg">}}

## Example

```yaml
kind: tool
name: list_tablespaces
type: postgres-list-tablespaces
source: postgres-source
description: |
  Lists all tablespaces in the database. Returns the tablespace name,
  owner name, size in bytes (if the current user is allowed to read it,
  otherwise NULL), internal object ID, the access control
  list regarding permissions, and any specific tablespace options.
```
The response is a json array with the following elements:

```json
{
 "tablespace_name": "name of the tablespace",
 "owner_username": "owner of the tablespace",
 "size_in_bytes": "size in bytes if the current user has CREATE privilege on the tablespace, has the privileges of pg_read_all_stats, or the tablespace is the current database's default tablespace; otherwise NULL",
 "oid": "Object ID of the tablespace",
 "spcacl": "Access privileges",
 "spcoptions": "Tablespace-level options (e.g., seq_page_cost, random_page_cost)"
}
```

## Reference

| **field**   | **type** | **required**  | **description**                                      |
|-------------|:--------:|:-------------:|------------------------------------------------------|
| type        |  string  |     true      | Must be "postgres-list-tablespaces".                      |
| source      |  string  |     true      | Name of the source the SQL should execute on.        |
| description |  string  |     false     | Description of the tool that is passed to the agent. |
