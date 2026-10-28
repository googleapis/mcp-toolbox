---
title: "conversational-analytics-get-data-agent-info"
type: docs
weight: 1
description: >
  A "conversational-analytics-get-data-agent-info" tool allows retrieving information about a specific Conversational Analytics data agent.
aliases:
- /resources/tools/conversational-analytics-get-data-agent-info
---

## About

A `conversational-analytics-get-data-agent-info` tool allows you to retrieve
details about a specific data agent.

It's compatible with the following sources:

- cloud-gemini-data-analytics

`conversational-analytics-get-data-agent-info` accepts the following parameters:

- **`data_agent_id`:** The ID of the data agent to retrieve information for.

## Example

```yaml
kind: tool
name: get_agent_info
type: conversational-analytics-get-data-agent-info
source: my-conversational-analytics-source
location: global
description: |
  Use this tool to get details about a specific data agent.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "conversational-analytics-get-data-agent-info". |
| source      |  string  |     true     | Name of the source.                                |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
| location    |  string  |    false     | The Google Cloud location (default: "global").     |