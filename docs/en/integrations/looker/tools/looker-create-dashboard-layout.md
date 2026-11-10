---
title: "looker-create-dashboard-layout"
type: docs
weight: 1
description: >
  "looker-create-dashboard-layout" creates a new dashboard layout (tab).
---

## About

The `looker-create-dashboard-layout` tool creates a new dashboard layout, which typically represents a tab in modern Looker dashboards.

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: create_dashboard_layout
type: looker-create-dashboard-layout
source: looker-source
description: |
  This tool creates a new dashboard layout, which typically represents a tab
  in modern Looker dashboards.

  Parameters:
  - dashboard_id (required): The ID of the dashboard.
  - label (required): The label (title) of the new tab.
  - type (optional): The type of layout (defaults to 'newspaper').
  - active (optional): Whether to make this layout active.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|:------------|:--------:|:------------:|----------------------------------------------------|
| type        | string   | true         | Must be "looker-create-dashboard-layout".          |
| source      | string   | true         | Name of the Looker source.                         |
| description | string   | true         | Description of the tool that is passed to the LLM. |
