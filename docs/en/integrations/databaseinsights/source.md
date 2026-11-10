---
title: "Database Insights Source"
linkTitle: "Source"
type: docs
weight: 1
description: "The \"databaseinsights\" source provides a client for the Google Cloud Database Insights API.\n"
no_list: true
---

## About

The `databaseinsights` source provides a client to interact with the Google Cloud Database Insights API (`databaseinsights.googleapis.com`). This source supports the Advanced Query Insights tools that are available via the remote [Database Insights MCP Server](https://docs.cloud.google.com/alloydb/docs/reference/mcp/databaseinsights/mcp), allowing tools to retrieve query performance statistics, wait event metrics, time-series trends, and index recommendations for Google Cloud databases like AlloyDB.

Authentication is handled via Application Default Credentials (ADC) using OAuth 2.0 access tokens.

## Available Tools

{{< list-tools >}}

## Requirements

### API Enablement

Enable the required Google Cloud APIs on your target project:
*   **AlloyDB API** (`alloydb.googleapis.com`)
*   **Database Insights API** (`databaseinsights.googleapis.com`)

**Roles required to enable APIs:**
To enable APIs, you need the `serviceusage.services.enable` permission. If you created the project, you likely already have this permission through the **Owner** role (`roles/owner`). Otherwise, you can obtain this permission through the **Service Usage Admin** role (`roles/serviceusage.serviceUsageAdmin`).

### IAM Roles & Permissions

To query performance metrics, wait events, and index recommendations, grant the appropriate IAM roles on the target project or database location:
*   **View Cloud Monitoring data:** Monitoring Viewer (`roles/monitoring.viewer`)
*   **View Database Insights data:** Database Insights Viewer (`roles/databaseinsights.viewer`)

**Fine-grained Database Insights permissions:**
*   **Get advanced aggregated query statistics:** `databaseinsights.queryStats.fetch`
*   **Get advanced time series query statistics:** `databaseinsights.queryTimeSeries.fetch`
*   **Get advanced aggregated wait event statistics:** `databaseinsights.waitEventStats.fetch`
*   **Get advanced time series wait event statistics:** `databaseinsights.waitEventTimeSeries.fetch`
*   **Get index recommendations:** `databaseinsights.indexRecommendations.batchQuery`, `databaseinsights.recommendations.query`, `databaseinsights.resourceRecommendations.query`

## Example

```yaml
kind: source
name: database-insights-source
type: databaseinsights
---
kind: source
name: custom-project-database-insights
type: databaseinsights
project: my-quota-project
```

## Reference

| **field** | **type** | **required** | **description** |
| -------------- | :------: | :----------: | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| type | string | true | Must be "databaseinsights". |
| project | string | false | The Google Cloud project ID to use as a fallback billing/quota project for API calls if not extracted from the resource path. |
| endpoint | string | false | Override API endpoint URL (e.g., for staging or testing environments). Defaults to `https://databaseinsights.googleapis.com`. |

## Additional Resources

*   [Use Database Insights with Model Context Protocol (MCP)](https://cloud.google.com/alloydb/docs/ai/use-database-insights-mcp)
*   [AlloyDB Database Insights MCP Reference](https://cloud.google.com/alloydb/docs/reference/mcp/databaseinsights/mcp)

