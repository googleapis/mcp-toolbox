---
title: "looker-deploy-to-production"
type: docs
weight: 1
description: >
  A "looker-deploy-to-production" tool is used to deploy the current revision of the dev branch to production.
---

## About

A `looker-deploy-to-production` tool is used to deploy the current revision of the dev branch to production.

## Compatible Sources

{{< compatible-sources >}}

## Parameters

| **field**  | **type** | **required** | **description**                           |
| ---------- | :------: | :----------: | ----------------------------------------- |
| project_id |  string  |     true     | The unique ID of the LookML project.      |

## Example

```yaml
kind: tool
name: deploy_to_production
type: looker-deploy-to-production
source: looker-source
description: |
  This tool deploys the current revision of the dev branch to production.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "looker-deploy-to-production".             |
| source      |  string  |     true     | Name of the source.                                |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
