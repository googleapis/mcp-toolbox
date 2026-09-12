---
title: "looker-delete-conversation-message"
type: docs
weight: 1
description: >
  "looker-delete-conversation-message" deletes a specific message from a Looker Conversational Analytics conversation.
---

## About

The `looker-delete-conversation-message` tool allows deleting a specific message from a conversation.

```json
{
  "name": "looker-delete-conversation-message",
  "parameters": {
    "conversation_id": "conv-123",
    "message_id": "msg-456"
  }
}
```

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: delete_conversation_message
type: looker-delete-conversation-message
source: my-looker-instance
description: |
  Delete a message from a conversation.
  - `conversation_id` (string): The ID of the conversation.
  - `message_id` (string): The ID of the message to delete.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "looker-delete-conversation-message".      |
| source      |  string  |     true     | Name of the Looker source.                         |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
