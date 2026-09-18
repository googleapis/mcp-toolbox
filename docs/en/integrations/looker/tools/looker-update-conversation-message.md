---
title: "looker-update-conversation-message"
type: docs
weight: 1
description: >
  "looker-update-conversation-message" updates a specific message in a Looker Conversational Analytics conversation.
---

## About

The `looker-update-conversation-message` tool allows updating a specific message in a conversation.

```json
{
  "name": "looker-update-conversation-message",
  "parameters": {
    "conversation_id": "conv-123",
    "message_id": "msg-456",
    "type": "user",
    "message": {
      "text": "Updated text"
    }
  }
}
```

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: update_conversation_message
type: looker-update-conversation-message
source: my-looker-instance
description: |
  Update a message in a conversation.
  - `conversation_id` (string): The ID of the conversation.
  - `message_id` (string): The ID of the message to update.
  - `type` (string): Optional. The new type of the message.
  - `message` (map): Optional. The new message content (map).
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "looker-update-conversation-message".      |
| source      |  string  |     true     | Name of the Looker source.                         |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
