---
title: "looker-project-commit"
type: docs
weight: 1
description: >
  A "looker-project-commit" tool is used to commit changes in a LookML project.
---

## About

A `looker-project-commit` tool is used to commit changes in a LookML project.

## Compatible Sources

{{< compatible-sources >}}

## Parameters

| **field**  | **type** | **required** | **description**                        |
| ---------- | :------: | :----------: | -------------------------------------- |
| project_id |  string  |     true     | The unique ID of the LookML project.   |
| message    |  string  |     false    | The commit message.                    |
| files      |  array   |     false    | List of specific file paths to commit. |
| amend      | boolean  |     false    | Amend the last commit.                 |

## Example

```yaml
kind: tool
name: project_commit
type: looker-project-commit
source: looker-source
description: |
  This tool is used to commit changes in a LookML project. This only works in dev mode.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "looker-project-commit".                   |
| source      |  string  |     true     | Name of the source.                                |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
