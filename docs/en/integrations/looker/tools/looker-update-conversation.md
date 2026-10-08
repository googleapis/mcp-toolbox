---
title: "looker-update-conversation"
type: docs
weight: 1
description: >
  "looker-update-conversation" updates an existing Looker Conversational Analytics conversation.
---

## About

The `looker-update-conversation` tool allows updating an existing conversation.

```json
{
  "name": "looker-update-conversation",
  "parameters": {
    "conversation_id": "conv-123",
    "name": "Updated Name",
    "agent_id": "new-agent-id"
  }
}
```

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: update_conversation
type: looker-update-conversation
source: my-looker-instance
description: |
  Update a conversation.
  - `conversation_id` (string): The ID of the conversation to update.
  - `name` (string): Optional. The new name of the conversation.
  - `agent_id` (string): Optional. The new agent ID.
  - `category` (string): Optional. The new category.
  - `sources` (array): Optional. A list of data sources.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "looker-update-conversation".              |
| source      |  string  |     true     | Name of the Looker source.                         |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
