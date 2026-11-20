---
title: "looker-make-dashboard"
type: docs
weight: 1
description: >
  "looker-make-dashboard" generates a Looker dashboard in the users personal folder in
  Looker
---

## About

The `looker-make-dashboard` creates a dashboard in the user's
Looker personal folder.

`looker-make-dashboard` takes three parameters:

1. the `title`
2. the `description`
3. an optional `folder` id. If not provided, the user's default folder will be used.

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: make_dashboard
type: looker-make-dashboard
source: looker-source
description: |
  This tool creates a new, empty dashboard in Looker. Dashboards are stored
  in the user's personal folder, and the dashboard name must be unique.
  After creation, use `add_dashboard_filter` to add filters and
  `add_dashboard_element` to add content tiles.

  Required Parameters:
  - title (required): A unique title for the new dashboard.
  - description (required): A brief description of the dashboard's purpose.

  Output:
  A JSON object containing a link (`url`) to the newly created dashboard,
  its unique `id`, and its default `dashboard_layout_id` (when available). This
  `id` and `dashboard_layout_id` can be used in subsequent calls to
  `add_dashboard_filter`, `add_dashboard_element`, and `update_dashboard_layout`.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "looker-make-dashboard"                    |
| source      |  string  |     true     | Name of the source the SQL should execute on.      |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
