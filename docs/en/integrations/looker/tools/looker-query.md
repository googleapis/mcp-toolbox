---
title: "looker-query"
type: docs
weight: 1
description: >
  "looker-query" runs an inline query using the Looker
  semantic model or executes a saved Looker query by ID/slug.
---

## About

The `looker-query` tool runs an inline query using the Looker
semantic model or executes a saved Looker query by ID/slug.

`looker-query` takes the following parameters (provide either `query_id` OR `model` + `explore` + `fields`):

1. an optional `query_id` (numeric ID or slug of a saved query)
2. the `model` (required if `query_id` is not provided)
3. the `explore` (required if `query_id` is not provided)
4. the `fields` list (required if `query_id` is not provided)
5. an optional set of `filters`
6. an optional `filter_expression`
7. an optional `dynamic_fields`
8. an optional set of `pivots`
9. an optional set of `sorts`
10. an optional `limit`
11. an optional `tz`
12. an optional `result_format` (`json` or `json_detail`, default `json`)
13. an optional `vis_config`
14. an optional `generate_drill_links`

Starting in Looker v25.18, these queries can be identified in Looker's
System Activity. In the History explore, use the field API Client Name
to find MCP Toolbox queries.

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: query
type: looker-query
source: looker-source
description: |
  This tool runs a query against a LookML model or executes a saved query by ID/slug and returns the results in JSON format.

  Parameters (provide either `query_id` OR `model` + `explore` + `fields`):
  - query_id (optional): The unique identifier (numeric ID or slug) of a saved Looker query to execute. If provided, `model`, `explore`, and `fields` are not required.
  - model: The name of the LookML model (from `get_models`). Required if `query_id` is not provided.
  - explore: The name of the explore (from `get_explores`). Required if `query_id` is not provided.
  - fields: A list of field names (dimensions, measures, filters, or parameters) to include in the query. Required if `query_id` is not provided.

  Optional Parameters:
  - pivots: A list of fields to pivot the results by. These fields must also be included in the `fields` list.
  - filters: A map of filter expressions, e.g., `{"view_name.field_name": "value", "view_name.date": "7 days"}`.
    - Each key must be a fully-scoped `view_name.field_name`, copied verbatim from
      `get_dimensions`, `get_measures`, `get_filters`, or `get_parameters`. The view
      prefix and the dot are required.
    - Each value is a Looker filter expression. Pass values bare: do not wrap them in
      extra quote characters. For LookML `parameter` fields, use the raw allowed_value
      (e.g. `first_touch`, not `"first_touch"`).
    - Use `not null` instead of `-NULL`.
    - If a value contains a comma, enclose it in single quotes (e.g., `'New York, NY'`).
    - To retrieve valid filter values for a suggestible field, use the 'get_field_value_suggestions' tool.
  - filter_expression: A Looker expression filter string (custom filter). This allows complex logic and comparing fields.
    - Reference fields using `${view.field_name}` syntax.
    - Supports logical operators (`AND`, `OR`, `NOT`) and comparison operators.
    - Supports Looker functions (e.g., `matches_filter`, `now`, `add_days`, `diff_days`).
    - Examples:
      - `${orders.order_date} < add_years(-1, now())`
      - `${activity.email} != ${activity_drive_facts.current_owner_email}`
      - `matches_filter(${order.order_month}, '24 months') AND matches_filter(${order.order_month}, 'before 2024/07/01')`
  - dynamic_fields: An optional array of dynamic fields (table calculations, custom measures, custom dimensions) defined as JSON objects.
    - Useful for ad-hoc calculations that are not defined in the LookML model.
    - Reference fields using `${view.field_name}` syntax.
    - Examples:
      - Table Calculation: `[{"table_calculation": "test", "label": "test", "expression": "${order_items.total_sale_price} * 0.8", "_type_hint": "number"}]`
      - Custom Dimension: `[{"dimension": "days_since_order", "label": "days since order", "expression": "diff_days(${order.order_date}, now())", "_type_hint": "number"}]`
      - Custom Measure: `[{"measure": "sum_of_revenue", "label": "Sum of Revenue", "based_on": "training.revenue", "type": "sum", "_type_hint": "number"}]`
  - sorts: A list of fields to sort by, optionally including direction (e.g., `["view.field desc"]`).
  - limit: Row limit (default 500). Use "-1" for unlimited.
  - tz: Specific timezone for the query (e.g. `America/Los_Angeles`).
  - result_format: Output format (`'json'` or `'json_detail'`, default `'json'`).
  - vis_config: Optional visualization configuration JSON string to associate with the query response.
  - generate_drill_links: Optional flag indicating whether to generate drill links in the query result.

  Note: Use `get_dimensions`, `get_measures`, `get_filters`, and `get_parameters` to find valid fields.

  The result of the query tool is JSON
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "looker-query"                             |
| source      |  string  |     true     | Name of the source the SQL should execute on.      |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
