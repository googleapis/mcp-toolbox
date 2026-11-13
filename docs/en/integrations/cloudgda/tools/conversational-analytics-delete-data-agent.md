---
title: "conversational-analytics-delete-data-agent"
type: docs
weight: 1
description: >
  A "conversational-analytics-delete-data-agent" tool allows deleting an existing Conversational Analytics data agent.
aliases:
- /resources/tools/conversational-analytics-delete-data-agent
---

## About

A `conversational-analytics-delete-data-agent` tool allows you to delete
an existing data agent in Conversational Analytics.

## Compatible Sources

{{< compatible-sources >}}

## Parameters

`conversational-analytics-delete-data-agent` accepts the following parameter:

- **`data_agent_id`:** The ID of the data agent to delete.

## Example

```yaml
kind: tool
name: delete_agent
type: conversational-analytics-delete-data-agent
source: my-conversational-analytics-source
location: global
description: |
  Use this tool to delete an existing data agent using its ID.
```

## Output Format

The tool returns the operation response after waiting for the deletion operation to complete successfully. Note that the tool will block and poll for up to 60 seconds.

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "conversational-analytics-delete-data-agent". |
| source      |  string  |     true     | Name of the source.                                |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
| location    |  string  |    false     | The Google Cloud location (default: "global").     |
| authRequired| []string |    false     | List of auth services required to use the tool.    |
