---
title: "looker-create-conversation-message"
type: docs
weight: 1
description: >
  "looker-create-conversation-message" creates one or more messages in a Looker Conversational Analytics conversation.
---

## About

The `looker-create-conversation-message` tool allows creating messages in a conversation.

```json
{
  "name": "looker-create-conversation-message",
  "parameters": {
    "conversation_id": "conv-123",
    "messages": [
      {
        "type": "user",
        "message": {
          "text": "Hello, how are you?"
        }
      }
    ]
  }
}
```

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: create_conversation_message
type: looker-create-conversation-message
source: my-looker-instance
description: |
  Create messages in a conversation.
  - `conversation_id` (string): The ID of the conversation.
  - `messages` (array): A list of messages to create. Each message must have `type` (string) and `message` (map).
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "looker-create-conversation-message".      |
| source      |  string  |     true     | Name of the Looker source.                         |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
