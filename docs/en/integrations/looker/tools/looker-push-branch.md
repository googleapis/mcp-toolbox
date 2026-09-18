---
title: "looker-push-branch"
type: docs
weight: 1
description: >
  A "looker-push-branch" tool is used to push the current dev branch to the remote git repo for a LookML project.
---

## About

A `looker-push-branch` tool is used to push the current dev branch to the remote git repo for a LookML project.

## Compatible Sources

{{< compatible-sources >}}

## Parameters

| **field**  | **type** | **required** | **description**                      |
| ---------- | :------: | :----------: | ------------------------------------ |
| project_id |  string  |     true     | The unique ID of the LookML project. |

## Example

```yaml
kind: tool
name: push_branch
type: looker-push-branch
source: looker-source
description: |
  This tool pushes the current dev branch to the remote git repo for a LookML project. This only works in dev mode.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "looker-push-branch".                      |
| source      |  string  |     true     | Name of the source.                                |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
