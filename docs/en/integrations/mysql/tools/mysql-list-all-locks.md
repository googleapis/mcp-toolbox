---
title: "mysql-list-all-locks"
type: docs
weight: 1
description: >
  A "mysql-list-all-locks" tool list all active locks including lock type, lock mode, locked object, lock status, transaction state, query and process id for all objects or specified objects within a designated database or across all databases as requested. 
---

## About
`mysql-list-all-locks` tool retrieves active database locks by joining performance_schema.data_locks with information_schema.innodb_trx, providing a comprehensive view of blocked threads, transaction states, and the specific queries causing contention.

`mysql-list-all-locks` outputs a detailed view of data locks including lock type, lock mode, lock status, transaction state, current operation and query for all threads running on specified object or all objects in a database. The output is a JSON formatted array of top 10 data locks ordered by longest running transaction time.

## Compatible Sources

{{< compatible-sources others="integrations/cloud-sql-mysql">}}

## Requirements

- `performance_schema` should be turned ON for this tool to work. 

## Parameters

This tool takes 3 optional input parameters:
- `table_schema` (optional): The target database for active locks. If not specified the results will be displayed for all databases. 
- `table_name` (optional): Name of the table to be checked. Check all tables visible to the current user if not specified.
- `limit` (optional): Max rows to return, default 10.
- `connected_schema` (optional): The database user is connected to, the value is set from env variable `CLOUD_SQL_MYSQL_DATABASE` or `MYSQL_DATABASE`.

## Example

```yaml
kind: tool
name: list_all_locks
type: mysql-list-all-locks
source: my-mysql-instance
description: list all active locks including lock type, lock mode, locked object, lock status, transaction state, query and process id for all objects or specified objects within a designated database or across all databases as requested.
```

## Output Format

The response is a json array with the following fields:
```json
[
  {
  "thread_id": "The internal MySQL server thread identifier associated with the lock",
  "process_id": "MySQL Process ID",
  "db": "The database schema where the locked object is located",
  "table_name": "The name of the specific table affected by the lock",
  "lock_type": "The target of the lock, such as a table or an individual record",
  "lock_mode": "The specific permission level of the lock (e.g., Shared or Exclusive)",
  "lock_status": "Whether the lock has been successfully granted or is currently waiting",
  "transaction_state": "The current lifecycle phase of the transaction",
  "current_operation": "The specific internal task the transaction is currently performing",
  "query": "The trimmed text of the SQL statement"
  }
]
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "mysql-list-all-locks".                    |
| source      |  string  |     true     | Name of the source the SQL should execute on.      |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |

