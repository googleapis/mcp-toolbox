---
title: "looker-delete-conversation"
type: docs
weight: 1
description: >
  "looker-delete-conversation" deletes a Looker Conversational Analytics conversation.
---

## About

The `looker-delete-conversation` tool allows deleting a conversation.

```json
{
  "name": "looker-delete-conversation",
  "parameters": {
    "conversation_id": "conv-123"
  }
}
```

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: delete_conversation
type: looker-delete-conversation
source: my-looker-instance
description: |
  Delete a conversation.
  - `conversation_id` (string): The ID of the conversation to delete.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "looker-delete-conversation".              |
| source      |  string  |     true     | Name of the Looker source.                         |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
