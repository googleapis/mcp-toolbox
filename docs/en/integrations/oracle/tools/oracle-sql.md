---
title: "oracle-sql"
type: docs
weight: 1
description: > 
  An "oracle-sql" tool executes a pre-defined SQL statement against an Oracle database.
---

## About

An `oracle-sql` tool executes a pre-defined SQL statement against an
Oracle database.

The specified SQL statement is executed using [prepared statements][oracle-stmt]
for security and performance. It expects parameter placeholders in the SQL query
to be in the native Oracle format (e.g., `:1`, `:2`).

By default, the statement runs as a query. Set `readOnly` to `true` to run it as
a read-only transaction, so the database itself rejects writes and locking reads.
Set it to `false` to run the statement as DML, which reports the number of
affected rows instead of returning rows.

> **Note:** A read-only transaction does not stop DDL, which commits implicitly,
> or PL/SQL that writes in its own autonomous transaction
> (`PRAGMA AUTONOMOUS_TRANSACTION`). See [Read-Only Access][ro-access] for
> enforcing this in the database instead.

[ro-access]: ../source.md#read-only-access

[oracle-stmt]: https://docs.oracle.com/javase/tutorial/jdbc/basics/prepared.html

## Compatible Sources

{{< compatible-sources >}}

## Example

> **Note:** This tool uses parameterized queries to prevent SQL injections.
> Query parameters can be used as substitutes for arbitrary expressions.
> Parameters cannot be used as substitutes for identifiers, column names, table
> names, or other parts of the query.

```yaml
kind: tool
name: search_flights_by_number
type: oracle-sql
source: my-oracle-instance
statement: |
  SELECT * FROM flights
  WHERE airline = :1
  AND flight_number = :2
  FETCH FIRST 10 ROWS ONLY
description: |
  Use this tool to get information for a specific flight.
  Takes an airline code and flight number and returns info on the flight.
  Do NOT use this tool with a flight id. Do NOT guess an airline code or flight number.
  Example:
  {{
      "airline": "CY",
      "flight_number": "888",
  }}
parameters:
  - name: airline
    type: string
    description: Airline unique 2 letter identifier
  - name: flight_number
    type: string
    description: 1 to 4 digit number
---
kind: tool
name: update_flight_status
type: oracle-sql
source: my-oracle-instance
readOnly: false  # Required for INSERT/UPDATE/DELETE
statement: |
  UPDATE flights 
  SET status = :1 
  WHERE airline = :2 AND flight_number = :3
description: Updates the status of a specific flight.
parameters:
  - name: status
    type: string
  - name: airline
    type: string
  - name: flight_number
    type: string
```
