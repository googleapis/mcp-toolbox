---
title: "conversational-analytics-list-accessible-data-agents"
type: docs
weight: 1
description: >
  A "conversational-analytics-list-accessible-data-agents" tool allows listing accessible Conversational Analytics data agents.
aliases:
- /resources/tools/conversational-analytics-list-accessible-data-agents
---

## About

A `conversational-analytics-list-accessible-data-agents` tool allows you to list
data agents that are accessible.

The Conversational Analytics API returns accessible data agents one page at a
time. By default this tool hides that: it follows every page itself and returns
all accessible data agents in a single response. Set `page_size` or `page_token`
to take over pagination and receive one page at a time instead.

It's compatible with the following sources:

- cloud-gemini-data-analytics

`conversational-analytics-list-accessible-data-agents` accepts the following parameters:

- **`page_size`** (optional): The maximum number of data agents to return in this call. Must be a positive integer. Only set this to page through the results manually; when both `page_size` and `page_token` are omitted, every page is fetched automatically.
- **`page_token`** (optional): A `nextPageToken` returned by a previous call, used to fetch the next page. Only set this to page through the results manually.

## Example

```yaml
kind: tool
name: list_agents
type: conversational-analytics-list-accessible-data-agents
source: my-conversational-analytics-source
location: global
description: |
  Use this tool to list available data agents.
```

## Output Format

```json
{
  "dataAgents": [
    {
      "name": "projects/my-project/locations/global/dataAgents/my-agent",
      "displayName": "My Agent",
      "createTime": "2026-01-01T00:00:00.000000Z",
      "updateTime": "2026-01-01T00:00:00.000000Z"
    }
  ]
}
```

`nextPageToken` is only included when more data agents remain:

- **Without pagination parameters**, the response has no `nextPageToken`,
  because every page was already fetched. If the automatic fetch reaches its
  safety limit (100 pages or around 1000 data agents), it returns everything
  collected so far along with the `nextPageToken` it stopped at, so you can pass
  that token back as `page_token` to step through the rest. If a page fails or
  the automatic fetch exceeds its 5 minute budget, the call returns an error
  rather than a partial list.
- **With `page_size` or `page_token`**, the response is the API's own page and
  carries a `nextPageToken` whenever more data agents are available. Pass it
  back as `page_token` to fetch the next page.

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "conversational-analytics-list-accessible-data-agents". |
| source      |  string  |     true     | Name of the source.                                |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
| location    |  string  |    false     | The Google Cloud location (default: "global").     |