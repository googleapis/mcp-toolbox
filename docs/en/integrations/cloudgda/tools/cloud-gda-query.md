---
title: "cloud-gemini-data-analytics-query"
type: docs
weight: 1
description: >
  A tool to convert natural language queries into SQL statements using the Gemini Data Analytics QueryData API.
---

## About

The `cloud-gemini-data-analytics-query` tool allows you to send natural language questions to the Gemini Data Analytics API and receive structured responses containing SQL queries, natural language answers, and explanations. For details on defining data agent context for database data sources, see the official [documentation](https://docs.cloud.google.com/gemini/docs/conversational-analytics-api/data-agent-authored-context-databases).

> [!NOTE]
> Only `alloydb`, `spannerReference`, and `cloudSqlReference` are supported as [datasource references](https://cloud.google.com/gemini/docs/conversational-analytics-api/reference/rest/v1beta/projects.locations.dataAgents#DatasourceReferences).


## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: my-gda-query-tool
type: cloud-gemini-data-analytics-query
source: my-gda-source
description: "Use this tool to send natural language queries to the Gemini Data Analytics API and receive SQL, natural language answers, and explanations."
location: ${your_database_location}
context:
  datasourceReferences:
    cloudSqlReference:
      databaseReference:
        projectId: "${your_project_id}"
        region: "${your_database_instance_region}"
        instanceId: "${your_database_instance_id}"
        databaseId: "${your_database_name}"
        engine: "POSTGRESQL"
      agentContextReference:
        contextSetId: "${your_context_set_id}" # E.g. projects/${project_id}/locations/${context_set_location}/contextSets/${context_set_id}
generationOptions:
  generateQueryResult: true
  generateNaturalLanguageAnswer: true
  generateExplanation: true
  generateDisambiguationQuestion: true
```

### Usage Flow

When using this tool, a `query` parameter containing a natural language query is provided to the tool (typically by an agent). The tool then interacts with the Gemini Data Analytics API using the context defined in your configuration.

The structure of the response depends on the `generationOptions` configured in your tool definition (e.g., enabling `generateQueryResult` will include the SQL query results).

See [Data Analytics API REST documentation](https://cloud.google.com/gemini/docs/conversational-analytics-api/reference/rest/v1alpha/projects.locations/queryData) for details.

**Example Input Query:**

```text
How many accounts who have region in Prague are eligible for loans? A3 contains the data of region.
```

**Example API Response:**

```json
{
  "generatedQuery": "SELECT COUNT(T1.account_id) FROM account AS T1 INNER JOIN loan AS T2 ON T1.account_id = T2.account_id INNER JOIN district AS T3 ON T1.district_id = T3.district_id WHERE T3.A3 = 'Prague'",
  "intentExplanation": "I found a template that matches the user's question. The template asks about the number of accounts who have region in a given city and are eligible for loans. The question asks about the number of accounts who have region in Prague and are eligible for loans. The template's parameterized SQL is 'SELECT COUNT(T1.account_id) FROM account AS T1 INNER JOIN loan AS T2 ON T1.account_id = T2.account_id INNER JOIN district AS T3 ON T1.district_id = T3.district_id WHERE T3.A3 = ?'. I will replace the named parameter '?' with 'Prague'.",
  "naturalLanguageAnswer": "There are 84 accounts from the Prague region that are eligible for loans.",
  "queryResult": {
    "columns": [
      {
        "type": "INT64"
      }
    ],
    "rows": [
      {
        "values": [
          {
            "value": "84"
          }
        ]
      }
    ],
    "totalRowCount": "1"
  }
}
```

## Reference

| **field**         | **type** | **required** | **description**                                                                                                                                                                                                                                              |
| ----------------- | :------: | :----------: | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| type              |  string  |     true     | Must be "cloud-gemini-data-analytics-query".                                                                                                                                                                                                                 |
| source            |  string  |     true     | The name of the `cloud-gemini-data-analytics` source to use.                                                                                                                                                                                                 |
| description       |  string  |     true     | A description of the tool's purpose.                                                                                                                                                                                                                         |
| location          |  string  |     true     | The Google Cloud location of the target database resource (e.g., "us-central1"). This is used to construct the parent resource name in the API call.                                                                                                         |
| context           |  object  |     true     | The database to query, and optional query context. See [Context](#context) below.                                                                                                                                                                            |
| generationOptions |  object  |    false     | Options for generating the response. See [GenerationOptions](https://github.com/googleapis/googleapis/blob/b32495a713a68dd0dff90cf0b24021debfca048a/google/cloud/geminidataanalytics/v1beta/data_chat_service.proto#L135) for details.                       |

### Context

`context` is passed to the Gemini Data Analytics API unchanged, as a
[QueryDataContext](https://github.com/googleapis/googleapis/blob/b32495a713a68dd0dff90cf0b24021debfca048a/google/cloud/geminidataanalytics/v1beta/data_chat_service.proto#L156).
Field names are lowerCamelCase, and unrecognized fields are rejected when
Toolbox loads the configuration.

| **field**                                    | **type**            | **required** | **description**                                                                                                                       |
| -------------------------------------------- | :-----------------: | :----------: | ------------------------------------------------------------------------------------------------------------------------------------- |
| datasourceReferences                         |       object        |     true     | The database to query. Set exactly one datasource kind. See [Datasource references](#datasource-references) below.                    |
| parameterizedSecureViewParameters.parameters |  map[string]string  |    false     | Parameter names and values to inject into a parameterized secure view. See [Parameterized Secure Views](#parameterized-secure-views-psv). |

#### Datasource references

Set exactly one of `alloydb`, `spannerReference`, or `cloudSqlReference` under
`datasourceReferences`. The API also defines `bq`, `studio`, and `looker`, but
this tool does not support them.

All three supported kinds take the same two fields:

| **field**                          | **type** | **required** | **description**                                                                                                                                                            |
| ---------------------------------- | :------: | :----------: | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| databaseReference                  |  object  |     true     | Identifies the database. Fields vary by kind. See [databaseReference fields](#databasereference-fields) below.                                                             |
| agentContextReference.contextSetId |  string  |    false     | Resource name of an authored context set to apply to the query, in the form `projects/{project}/locations/{location}/contextSets/{context_set}`. Improves query accuracy. |

#### databaseReference fields

One table covers all three kinds; the **applies to** column shows where each
field is accepted. Supplying a field to a kind that does not accept it is a
configuration error.

| **field**  |    **type**    | **required** | **applies to**                       | **description**                                                                                            |
| ---------- | :------------: | :----------: | ------------------------------------ | ---------------------------------------------------------------------------------------------------------- |
| projectId  |     string     |     true     | all                                  | The project the instance belongs to.                                                                       |
| region     |     string     |     true     | all                                  | The region of the instance, for example `us-central1`.                                                     |
| clusterId  |     string     |     true     | `alloydb`                            | The AlloyDB cluster id.                                                                                    |
| instanceId |     string     |     true     | all                                  | The instance id.                                                                                           |
| databaseId |     string     |     true     | all                                  | The database id.                                                                                           |
| engine     |     string     |     true     | `cloudSqlReference`, `spannerReference` | `POSTGRESQL` or `MYSQL` for `cloudSqlReference`; `GOOGLE_SQL` or `POSTGRESQL` for `spannerReference`. AlloyDB has no engine field. |
| tableIds   | list of string |    false     | all                                  | Restricts the query to these tables. All tables in the database are used if unset.                         |

## Advanced Usage

### Parameterized Secure Views (PSV)

Parameterized Secure Views (PSV) provide a robust mechanism for Row-Level Access Control (RLAC). A PSV is a view defined on a base table that requires mandatory parameters at query time, users cannot read from the view without supplying the defined parameters, and direct access to the underlying base tables is revoked.

This is useful in agentic applications where each end-user should only see their own data, without the application having broad access to the base tables.

**How it works:**

1. The database administrator creates a parameterized secure view  and grants the API caller access **only** to that view, not the base table.
2. At query time, the caller supplies `parameterizedSecureViewParameters` in the tool `context`. These key/value pairs are injected into the view's filter, ensuring the query returns only the rows matching the provided parameters.
3. The base tables are invisible to the caller; any attempt to query them directly will fail with a permissions error.

**CloudSQL PostgreSQL example:**

```yaml
kind: tool
name: my-gda-psv-pg-tool
type: cloud-gemini-data-analytics-query
source: my-gda-source
description: "Query user-specific data via a parameterized secure view on CloudSQL Postgres."
location: ${your_database_location}
context:
  datasourceReferences:
    cloudSqlReference:
      databaseReference:
        projectId: "${your_project_id}"
        region: "${your_database_instance_region}"
        instanceId: "${your_database_instance_id}"
        databaseId: "${your_database_name}"
        engine: "POSTGRESQL"
      agentContextReference:
        contextSetId: "${your_context_set_id}" # E.g. projects/${project_id}/locations/${context_set_location}/contextSets/${context_set_id}
  parameterizedSecureViewParameters:
    parameters:
      # Key is the parameter name defined in your secure view; value is what to
      # filter rows by (e.g., the end-user's ID).
      app_end_userid: "303"
generationOptions:
  generateQueryResult: true
  generateNaturalLanguageAnswer: true
  generateExplanation: true
```
