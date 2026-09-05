---
title: "Pre- and Post- Processing"
type: docs
weight: 12
description: >
  Intercept and modify interactions between the agent and its tools either before or after a tool is executed.
---

Pre- and post- processing allow developers to intercept and modify interactions between the agent and its tools or the user.

{{< notice note >}}

These capabilities are typically features of **orchestration frameworks** (like LangChain, LangGraph, or Agent Builder) rather than the Toolbox SDK itself. However, Toolbox tools are designed to fully leverage these framework capabilities to support robust, secure, and compliant agent architectures.

{{< /notice >}}

## Types of Processing

### Pre-processing

Pre-processing occurs before a tool is executed or an agent processes a message. Key types include:

- **Input Sanitization & Redaction**: Detecting and masking sensitive information (like PII) in user queries or tool arguments to prevent it from being logged or sent to unauthorized systems.
- **Business Logic Validation**: Verifying that the proposed action complies with business rules (e.g., ensuring a requested hotel stay does not exceed 14 days, or checking if a user has sufficient permission).
- **Security Guardrails**: Analyzing inputs for potential prompt injection attacks or malicious payloads.

### Post-processing

Post-processing occurs after a tool has executed or the model has generated a response. Key types include:

- **Response Enrichment**: Injecting additional data into the tool output that wasn't part of the raw API response (e.g., calculating loyalty points earned based on the booking value).
- **Output Formatting**: Transforming raw data (like JSON or XML) into a more human-readable or model-friendly format to improve the agent's understanding.
- **Compliance Auditing**: Logging the final outcome of transactions, including the original request and the result, to a secure audit trail.

## Processing Scopes

While processing logic can be applied at various levels (Agent, Model, Tool), this guide primarily focuses on **Tool Level** processing, which is most relevant for granular control over tool execution.

### Tool Level (Primary Focus)

Wraps individual tool executions. This is best for logic specific to a single tool or a set of tools.

- **Scope**: Intercepts the raw inputs (arguments) to a tool and its outputs.
- **Use Cases**: Argument validation, output formatting, specific privacy rules for sensitive tools.

### Other Levels

It is helpful to understand how tool-level processing differs from other scopes:

- **Model Level**: Intercepts individual calls to the LLM (prompts and responses). Unlike tool-level, this applies globally to all text sent/received, making it better for global PII redaction or token tracking.
- **Agent Level**: Wraps the high-level execution loop (e.g., a "turn" in the conversation). Unlike tool-level, this envelopes the entire turn (user input to final response), making it suitable for session management or end-to-end auditing.

## Best Practices

### Security & Guardrails

- **Principle of Least Privilege**: Ensure that tools run with the minimum necessary permissions. Middleware is an excellent place to enforce "read-only" modes or verify user identity before executing sensitive actions.
- **Input Sanitization**: Actively strip potential PII (like credit card numbers or raw emails) from tool arguments before logging them.
- **Prompt Injection Defense**: Use pre-processing hooks to scan user inputs for known jailbreak patterns or malicious directives before they reach the model or tools.

### Observability & Debugging

- **Structured Logging**: Instead of simple print statements, use structured JSON logging with correlation IDs. This allows you to trace a single user request through multiple agent turns and tool calls.
- **Logging for Testability**: LLM responses are non-deterministic and may summarize away key details.
  - **Pattern**: Add explicit logging markers in your post-processing middleware (e.g., `logger.info("ACTION_SUCCESS: <id>")`).
  - **Benefit**: Your integration tests can grep logs for these stable markers to verify tool success, rather than painfully parsing variable natural language responses.

### Performance & Cost Optimization

- **Token Economy**: Tools often return verbose JSON. Use post-processing to strip unnecessary fields or summarize large datasets *before* returning the result to the LLM's context window. This saves tokens and reduces latency.
- **Caching**: For read-heavy tools (like "search_knowledge_base"), implement caching middleware to return previous results for identical queries, saving both time and API costs.

### Error Handling

- **Graceful Degradation**: If a tool fails (e.g., API timeout), catch the exception in middleware and return a structured error message to the LLM (e.g., `Error: Database timeout, please try again`).
- **Self-Correction**: Well-formatted error messages often allow the LLM to understand *why* a call failed and retry it with corrected parameters automatically.


## Samples
