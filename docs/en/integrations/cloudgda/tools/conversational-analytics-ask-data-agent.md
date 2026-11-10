---
title: "conversational-analytics-ask-data-agent"
type: docs
weight: 1
description: >
  A "conversational-analytics-ask-data-agent" tool allows conversational interaction with a Conversational Analytics source.
aliases:
- /resources/tools/conversational-analytics-ask-data-agent
---

## About

A `conversational-analytics-ask-data-agent` tool allows you to ask questions about
your data in natural language.

This function takes a user's question (which can include conversational history
for context) and references to a specific BigQuery Data Agent, and sends them to a
stateless conversational API.

The API uses a GenAI agent to understand the question, generate and execute SQL
queries and Python code, and formulate an answer. This function returns a
detailed, sequential log of this entire process, which includes any generated
SQL or Python code, the data retrieved, and the final text answer.

**Note**: This tool requires additional setup in your project. Please refer to
the official Conversational Analytics API
documentation
for instructions.

It's compatible with the following sources:

- cloud-gemini-data-analytics

`conversational-analytics-ask-data-agent` accepts the following parameters:

- **`user_query_with_context`:** The question to ask the agent, potentially 
  including conversation history for context.
- **`data_agent_id`:** The ID of the data agent to ask.

## Example

```yaml
kind: tool
name: ask_data_agent
type: conversational-analytics-ask-data-agent
source: my-conversational-analytics-source
location: global
maxResults: 50
description: |
  Perform natural language data analysis and get insights by interacting 
  with a specific BigQuery Data Agent. This tool allows for conversational 
  queries and provides detailed responses based on the agent's configured 
  data sources.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "conversational-analytics-ask-data-agent". |
| source      |  string  |     true     | Name of the source for chat.                       |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
| location    |  string  |    false     | The Google Cloud location (default: "global").     |
| maxResults  |  integer |    false     | The maximum number of data rows to return in the tool's final response (default: 50). This only limits the amount of data included in the final tool return to prevent excessive token consumption, and does not affect the internal analytical process or intermediate steps. |