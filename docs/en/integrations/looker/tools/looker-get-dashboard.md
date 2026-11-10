---
title: "looker-get-dashboard"
type: docs
weight: 1
description: >
  "looker-get-dashboard" retrieves the JSON design of a Looker dashboard.
---

## About

The `looker-get-dashboard` tool retrieves the JSON design of a Looker dashboard by its ID. The output is trimmed to essential fields (including `certification_metadata`), excluding alerts and scheduled plans, making it suitable for replicating or analyzing the dashboard structure.

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: get_dashboard
type: looker-get-dashboard
source: looker-source
description: |
  This tool retrieves the JSON design of a Looker dashboard by its ID.
  The output is trimmed to essential fields (including certification_metadata), excluding alerts and scheduled plans,
  making it suitable for replicating or analyzing the dashboard structure.

  Parameters:
  - dashboard_id (required): The unique identifier of the dashboard.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|:------------|:--------:|:------------:|----------------------------------------------------|
| type        | string   | true         | Must be "looker-get-dashboard".                    |
| source      | string   | true         | Name of the Looker source.                         |
| description | string   | true         | Description of the tool that is passed to the LLM. |
