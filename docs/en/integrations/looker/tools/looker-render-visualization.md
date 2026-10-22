---
title: "looker-render-visualization"
type: docs
weight: 1
description: >
  "looker-render-visualization" executes a query using the Looker
  semantic model and returns an interactive UI visualization response adhering to the MCP Apps specification.
---

## About

The `looker-render-visualization` tool executes a query using the Looker
semantic model and returns an interactive visualization interface adhering to the
Model Context Protocol (MCP) Apps specification.

`looker-render-visualization` takes twelve parameters:

1. an optional `query_id` (Looker query ID or slug string)
2. the `model` (optional if `query_id` is provided)
3. the `explore` (optional if `query_id` is provided)
4. the `fields` list (optional if `query_id` is provided)
5. an optional set of `filters`
6. an optional `filter_expression`
7. an optional `dynamic_fields`
8. an optional set of `pivots`
9. an optional set of `sorts`
10. an optional `limit`
11. an optional `tz`
12. an optional `vis_config`

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: render_visualization
type: looker-render-visualization
source: looker-source
description: |
  This tool executes a query against a LookML model or fetches a saved query by ID/slug and returns an interactive UI visualization response adhering to the MCP Apps specification.

  Parameters:
  - query_id (optional): Looker query ID (integer) or slug string to fetch and render a saved query visualization directly. If provided, model and explore are not required.
  - model (optional): The name of the LookML model (required if query_id is not provided).
  - explore (optional): The name of the explore (required if query_id is not provided).
  - fields (optional): A list of field names (dimensions, measures, filters, or parameters) to include in the query.
  - pivots (optional): A list of fields to pivot the results by.
  - filters (optional): A map of filter expressions, e.g., `{"view_name.field_name": "value"}`.
  - filter_expression (optional): A Looker expression filter string (custom filter).
  - dynamic_fields (optional): An optional array of dynamic fields (table calculations, custom measures, custom dimensions).
  - sorts (optional): A list of fields to sort by (e.g. `["users.created_date desc"]`).
  - limit (optional): Maximum number of rows to return (default: 500).
  - tz (optional): Timezone for the query.
  - vis_config (optional): Optional JSON string or object specifying the visualization configuration (e.g. chart type, legend, colors).
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "looker-render-visualization"               |
| source      |  string  |     true     | Name of the source the query should execute on.    |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
| ui          |  object  |    false     | Optional MCP App UI configuration (e.g. resource, visibility). |
