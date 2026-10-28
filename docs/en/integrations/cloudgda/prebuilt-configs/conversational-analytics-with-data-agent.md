---
title: "Conversational Analytics with Data Agent"
type: docs
description: "Details of the Conversational Analytics with Data Agent prebuilt configuration."
---

## Conversational Analytics with Data Agent

*   `--prebuilt` value: `conversational-analytics-with-data-agent`
*   **Environment Variables:**
    *   `CLOUD_GDA_PROJECT`: The GCP project ID.
    *   `CLOUD_GDA_LOCATION`: (Optional) The location of the data agent (e.g., `us` or `eu`). Defaults to `global`.
    *   `CLOUD_GDA_USE_CLIENT_OAUTH`: (Optional) If `true`, forwards the client's
        OAuth access token for authentication. Defaults to `false`.
    *   `CLOUD_GDA_MAX_RESULTS`: (Optional) The maximum number of rows
        to return. Defaults to `50`.
    *   `CLOUD_GDA_READONLY`: (Optional) When set to `true`, suppresses the
        write-capable tools (`create_data_agent`, `update_data_agent`, and
        `delete_data_agent`). Set to `false` to enable them. Defaults to `true`.
*   **Permissions:**
    *   **Gemini Data Analytics Stateless Chat User (Beta)** (`roles/geminidataanalytics.dataAgentStatelessUser`) to interact with the data agent.
    *   **Gemini Data Analytics Data Agent Creator / Owner** (`roles/geminidataanalytics.dataAgentCreator`, `roles/geminidataanalytics.dataAgentOwner`) to create, update, or delete data agents.
    *   **BigQuery User** (`roles/bigquery.user`) and **BigQuery Data Viewer** (`roles/bigquery.dataViewer`) on the underlying datasets/tables to allow the data agent to execute queries.
*   **Tools:**
    *   `list_accessible_data_agents`: List data agents that are accessible.
    *   `get_data_agent_info`: Retrieve details about a specific data agent.
    *   `ask_data_agent`: Use this tool to perform natural language data analysis,
        get insights, or answer complex questions using pre-configured data
        sources via a specific Data Agent. For more information on
        required roles, API setup, and IAM configuration, see the setup and
        authentication section of the [Conversational Analytics API
        documentation](https://cloud.google.com/gemini/docs/conversational-analytics-api/overview).
    *   `create_data_agent`: Create a new Data Agent for conversational analytics.
    *   `update_data_agent`: Update an existing Data Agent for conversational analytics.
    *   `delete_data_agent`: Delete an existing Data Agent in conversational analytics.
