---
title: "looker-conversational-analytics-chat"
type: docs
weight: 1
description: >
  "looker-conversational-analytics-chat" sends a message to a Looker Conversational Analytics conversation.
---

## About

The `looker-conversational-analytics-chat` tool allows sending a message to a conversation and getting the generated response from the agent.

```json
{
  "name": "looker-conversational-analytics-chat",
  "parameters": {
    "conversation_id": "conv-123",
    "user_message": "What were the sales last month?"
  }
}
```

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: conversational_analytics_chat
type: looker-conversational-analytics-chat
source: my-looker-instance
description: |
  Send a message to a conversation and get the response.
  - `conversation_id` (string): The ID of the conversation.
  - `user_message` (string): The text content of the message to send.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "looker-conversational-analytics-chat".    |
| source      |  string  |     true     | Name of the Looker source.                         |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
