---
title: "looker-reset-to-remote"
type: docs
weight: 1
description: >
  A "looker-reset-to-remote" tool is used to reset the current dev branch to the state of the last commit pushed to the remote git server.
---

## About

A `looker-reset-to-remote` tool is used to reset the current dev branch to the state of the last commit pushed to the remote git server.

## Compatible Sources

{{< compatible-sources >}}

## Parameters

| **field**  | **type** | **required** | **description**                           |
| ---------- | :------: | :----------: | ----------------------------------------- |
| project_id |  string  |     true     | The unique ID of the LookML project.      |

## Example

```yaml
kind: tool
name: reset_to_remote
type: looker-reset-to-remote
source: looker-source
description: |
  This tool resets the current dev branch to the state of the last commit pushed to the remote git server.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "looker-reset-to-remote".                  |
| source      |  string  |     true     | Name of the source.                                |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
