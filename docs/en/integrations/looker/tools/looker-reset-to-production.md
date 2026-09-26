---
title: "looker-reset-to-production"
type: docs
weight: 1
description: >
  A "looker-reset-to-production" tool is used to reset the local dev branch to the commit of production.
---

## About

A `looker-reset-to-production` tool is used to reset the local dev branch to the commit of production.

## Compatible Sources

{{< compatible-sources >}}

## Parameters

| **field**  | **type** | **required** | **description**                           |
| ---------- | :------: | :----------: | ----------------------------------------- |
| project_id |  string  |     true     | The unique ID of the LookML project.      |

## Example

```yaml
kind: tool
name: reset_to_production
type: looker-reset-to-production
source: looker-source
description: |
  This tool resets the local dev branch to the commit of production.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "looker-reset-to-production".              |
| source      |  string  |     true     | Name of the source.                                |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
