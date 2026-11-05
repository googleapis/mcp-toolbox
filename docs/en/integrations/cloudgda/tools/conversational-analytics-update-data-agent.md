---
title: "conversational-analytics-update-data-agent"
type: docs
weight: 1
description: >
  A "conversational-analytics-update-data-agent" tool allows updating an existing Conversational Analytics data agent.
aliases:
- /resources/tools/conversational-analytics-update-data-agent
---

## About

A `conversational-analytics-update-data-agent` tool allows you to update
an existing data agent in Conversational Analytics.

## Compatible Sources

{{< compatible-sources >}}

## Parameters

`conversational-analytics-update-data-agent` accepts the following parameters:

- **`data_agent_id`:** The ID of the data agent to update.
- **`agent_config`:** The JSON representation of the DataAgent resource fields to update. For the full schema, see the [DataAgent REST resource documentation](https://docs.cloud.google.com/gemini/data-agents/reference/rest/v1/projects.locations.dataAgents#resource:-dataagent).
- **`update_mask`:** Comma-separated list of fields to update, using the API's camelCase JSON field names (e.g. `"displayName,description"` or `"dataAnalyticsAgent.publishedContext.systemInstruction"`). Every field listed here must also be present in `agent_config`.

  Example `agent_config`:
  ```json
  {
    "displayName": "Updated Support Agent",
    "description": "Updated description",
    "dataAnalyticsAgent": {
      "publishedContext": {
        "systemInstruction": "You are a helpful support data analyst."
      }
    }
  }
  ```

## Example

```yaml
kind: tool
name: update_agent
type: conversational-analytics-update-data-agent
source: my-conversational-analytics-source
location: global
description: |
  Use this tool to update an existing data agent with the specified configuration and update mask.
```

## Output Format

The tool returns the updated DataAgent object after waiting for the operation to complete successfully. Note that the tool will block and poll for up to 60 seconds.

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "conversational-analytics-update-data-agent". |
| source      |  string  |     true     | Name of the source.                                |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
| location    |  string  |    false     | The Google Cloud location (default: "global").     |
| authRequired| []string |    false     | List of auth services required to use the tool.    |
