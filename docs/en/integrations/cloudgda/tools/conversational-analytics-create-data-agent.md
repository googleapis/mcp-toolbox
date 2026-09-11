---
title: "conversational-analytics-create-data-agent"
type: docs
weight: 1
description: >
  A "conversational-analytics-create-data-agent" tool allows creating a new Conversational Analytics data agent.
aliases:
- /resources/tools/conversational-analytics-create-data-agent
---

## About

A `conversational-analytics-create-data-agent` tool allows you to create
a new data agent in Conversational Analytics.

## Compatible Sources

{{< compatible-sources >}}

## Parameters

`conversational-analytics-create-data-agent` accepts the following parameters:

- **`data_agent_id`:** The ID to use for the new data agent.
- **`agent_config`:** The JSON representation of the DataAgent resource to create. For the full schema, see the [DataAgent REST resource documentation](https://docs.cloud.google.com/gemini/data-agents/reference/rest/v1/projects.locations.dataAgents#resource:-dataagent).

  Example `agent_config`:
  ```json
  {
    "displayName": "My Support Agent",
    "description": "An agent that analyzes support tickets.",
    "dataAnalyticsAgent": {
      "datasourceReferences": {
        "bq": {
          "tableReferences": [
            {
              "projectId": "my-project",
              "datasetId": "support_data",
              "tableId": "tickets"
            }
          ]
        }
      }
    }
  }
  ```

## Example

```yaml
kind: tool
name: create_agent
type: conversational-analytics-create-data-agent
source: my-conversational-analytics-source
location: global
description: |
  Use this tool to create a new data agent with the specified configuration.
```

## Output Format

The tool returns the newly created DataAgent object after waiting for the operation to complete successfully. Note that the tool will block and poll for up to 60 seconds. A timeout error does not necessarily mean the creation failed; the operation may still be processing in the background.

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "conversational-analytics-create-data-agent". |
| source      |  string  |     true     | Name of the source.                                |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
| location    |  string  |    false     | The Google Cloud location (default: "global").     |
| authRequired| []string |    false     | List of auth services required to use the tool.    |
