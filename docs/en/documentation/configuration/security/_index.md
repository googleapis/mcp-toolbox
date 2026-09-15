---
title: "Security"
type: docs
weight: 1
description: >
  Harden your MCP Toolbox agents and tools against prompt injection, jailbreaks,
  and sensitive data leakage.
---

This section covers how to secure MCP Toolbox deployments against common AI
security risks like prompt injection, jailbreaks, unauthorized database modifications,
and sensitive data leakage across the traffic between your users, agents, and tools:

- **[Model Armor](./model-armor.md):** Defend AI applications against prompt injection,
  jailbreaks, and sensitive data leakage by screening traffic with Google Cloud Model Armor.
- **[Read-Only Tools](./read-only.md):** Enforce deterministic read-only database
  access across custom tools and prebuilt servers using engine-level protocol locks,
  tool suppression, and MCP annotations.
