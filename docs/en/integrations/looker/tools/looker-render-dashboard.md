---
title: "looker-render-dashboard"
type: docs
weight: 1
description: >
  "looker-render-dashboard" retrieves a Looker dashboard definition and returns an interactive UI dashboard response adhering to the MCP Apps specification, allowing tiles to load queries asynchronously.
---

## About

The `looker-render-dashboard` tool retrieves a dashboard definition by its ID
and returns an interactive dashboard interface adhering to the Model Context
Protocol (MCP) Apps specification. Tile queries are loaded asynchronously
by the dashboard client, ensuring fast response times and eliminating large payload limits.

`looker-render-dashboard` takes two parameters:

1. a required `dashboard_id` (the unique identifier or slug of the dashboard)
2. an optional set of `filters` (key-value filter overrides)

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: render_dashboard
type: looker-render-dashboard
source: looker-source
description: |
  This tool retrieves a Looker dashboard by ID and returns an interactive dashboard UI response adhering to the MCP Apps specification, allowing dashboard tiles to load and execute queries asynchronously.

  Parameters:
  - dashboard_id (required): The unique identifier of the dashboard to render.
  - filters (optional): Optional dashboard filter overrides as key-value pairs.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "looker-render-dashboard"                   |
| source      |  string  |     true     | Name of the source the query should execute on.    |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
| ui          |  object  |    false     | Optional MCP App UI configuration (e.g. resource, visibility). |
