---
title: "looker-update-dashboard"
type: docs
weight: 1
description: >
  "looker-update-dashboard" updates an existing Looker dashboard container and its appearance settings.
---

## About

The `looker-update-dashboard` tool updates an existing Looker dashboard (`PATCH /api/4.0/dashboards/:id`). It allows updating top-level dashboard properties including `title`, `description`, navigation visibility, refresh interval, and dashboard appearance settings.

When publishing Looker Data Applications, this tool is used to configure the dashboard container for full-bleed rendering by hiding Looker's native top navigation header and zeroing outer page margins and tile spacing.

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: update_dashboard
type: looker-update-dashboard
source: looker-source
description: |
  This tool updates an existing Looker dashboard (`PATCH /api/4.0/dashboards/:id`).
  It allows updating top-level dashboard properties such as title, description,
  visibility, refresh interval, and dashboard appearance settings.

  FULL-BLEED DATA APP APPEARANCE:
  To configure a dashboard container for full-bleed Data Applications, pass:
  - `show_dashboard_header: false` (hides Looker's native top navigation bar, title, filter bar, reload icon, and menu)
  - `page_side_margins: 0` (removes outer dashboard margins)
  - `tile_space_between: 0` (removes gutter spacing between tiles)
  - `tile_shadow: false` (removes tile drop shadows)

  Parameters:
  - dashboard_id (required): The unique identifier of the dashboard to update.
  - title (optional): Updated dashboard title.
  - description (optional): Updated dashboard description.
  - show_dashboard_header (optional): Whether to display Looker's native header bar. Set to false for full-bleed Data Apps.
  - page_side_margins (optional): Page margin width in pixels (e.g. 0 for full bleed).
  - tile_space_between (optional): Spacing between tiles in pixels (e.g. 0 for full bleed).
  - tile_shadow (optional): Whether tiles have drop shadows (e.g. false for full bleed).
  - appearance (optional): Map of additional appearance styling settings.
  - hidden (optional): Whether to hide the dashboard in navigation.
  - refresh_interval (optional): Auto-refresh interval phrase (e.g. '1 hour').
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|:------------|:--------:|:------------:|----------------------------------------------------|
| type        | string   | true         | Must be "looker-update-dashboard".                 |
| source      | string   | true         | Name of the Looker source.                         |
| description | string   | true         | Description of the tool that is passed to the LLM. |
