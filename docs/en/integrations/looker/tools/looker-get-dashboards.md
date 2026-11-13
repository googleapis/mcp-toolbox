---
title: "looker-get-dashboards"
type: docs
weight: 1
description: >
  "looker-get-dashboards" tool searches for a saved Dashboard by name or description.

---

## About

The `looker-get-dashboards` tool searches for a saved Dashboard by
name or description. It returns each dashboard's `id`, `title`, `description`,
and `certification_metadata` (if present).

`looker-get-dashboards` takes seven parameters, the `title`, `desc`, `id`,
`folder_id`, `user_id`, `limit` and `offset`.

Title and description use SQL style wildcards and are case insensitive.

Limit and offset are used to page through a larger set of matches and
default to 100 and 0.

LookML dashboards are matched on their exact LookML `id` (for example
`model_name::dashboard_name`) and on `folder_id`. Because LookML dashboards are
not created by a user, they are excluded from the results when `user_id` is set.

## Compatible Sources

{{< compatible-sources >}}

## Parameters

| **field** | **type** | **required** | **description**                                              |
|-----------|:--------:|:------------:|--------------------------------------------------------------|
| title     |  string  |    false     | The title of the dashboard.                                  |
| desc      |  string  |    false     | The description of the dashboard.                            |
| id        |  string  |    false     | The id of the dashboard.                                     |
| folder_id |  string  |    false     | The id of the folder containing the dashboard.               |
| user_id   |  string  |    false     | The id of the user who created the dashboard.                |
| limit     | integer  |    false     | The number of dashboards to fetch. Default 100               |
| offset    | integer  |    false     | The number of dashboards to skip before fetching. Default 0  |

## Example

```yaml
kind: tool
name: get_dashboards
type: looker-get-dashboards
source: looker-source
description: |
  This tool searches for saved dashboards in a Looker instance. It returns a list of JSON objects, each representing a dashboard, including its id, title, description, and certification_metadata (when certified or revoked).

  Search Parameters:
  - title (optional): Filter by dashboard title (supports wildcards).
  - folder_id (optional): Filter by the ID of the folder where the dashboard is saved.
  - user_id (optional): Filter by the ID of the user who created the dashboard.
  - desc (optional): Filter by description content (supports wildcards).
  - id (optional): Filter by specific dashboard ID.
  - limit (optional): Maximum number of results to return. Defaults to a system limit.
  - offset (optional): Starting point for pagination.

  String Search Behavior:
  - Case-insensitive matching.
  - Supports SQL LIKE pattern match wildcards:
    - `%`: Matches any sequence of zero or more characters. (e.g., `"finan%"` matches "financial", "finance")
    - `_`: Matches any single character. (e.g., `"s_les"` matches "sales")
  - Special expressions for null checks:
    - `"IS NULL"`: Matches dashboards where the field is null.
    - `"NOT NULL"`: Excludes dashboards where the field is null.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "looker-get-dashboards"                    |
| source      |  string  |     true     | Name of the source the SQL should execute on.      |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
