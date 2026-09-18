---
title: "looker-create-conversation"
type: docs
weight: 1
description: >
  "looker-create-conversation" creates a Looker Conversational Analytics conversation.
---

## About

The `looker-create-conversation` tool allows creating a new conversation for Conversational Analytics.

```json
{
  "name": "looker-create-conversation",
  "parameters": {
    "name": "My Conversation",
    "agent_id": "optional-agent-id",
    "category": "optional-category",
    "sources": [{"model": "my_model", "explore": "my_explore"}]
  }
}
```

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: create_conversation
type: looker-create-conversation
source: my-looker-instance
description: |
  Create a new conversation.
  - `name` (string): The name of the conversation.
  - `agent_id` (string): Optional. The ID of the agent associated with this conversation.
  - `category` (string): Optional. The category of the conversation.
  - `sources` (array): Optional. A list of data sources (e.g., `[{"model": "my_model", "explore": "my_explore"}]`).
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "looker-create-conversation".              |
| source      |  string  |     true     | Name of the Looker source.                         |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
