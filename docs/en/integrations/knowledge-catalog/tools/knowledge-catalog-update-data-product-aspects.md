---
title: "dataplex-update-data-product-aspects"
type: docs
weight: 1
description: >
  A "dataplex-update-data-product-aspects" tool updates aspects for an existing Data Product Entry in Knowledge Catalog.
---

## About

A `dataplex-update-data-product-aspects` tool updates aspects on an existing Data Product Entry in Knowledge Catalog (formerly known as Dataplex). This tool operates on the catalog entry associated with the Data Product, allowing you to add or modify metadata aspects in a single request. 

View the [Data Products guide][guide] for more information.

[guide]: https://docs.cloud.google.com/dataplex/docs/data-products-overview

## Compatible Sources

{{< compatible-sources >}}

## Requirements

### IAM Permissions

Knowledge Catalog uses [Identity and Access Management (IAM)][iam-overview] to control
user and group access to Knowledge Catalog resources. Toolbox will use your
[Application Default Credentials (ADC)][adc] to authorize and authenticate when
interacting with [Knowledge Catalog][dataplex-docs].

In addition to [setting the ADC for your server][set-adc], you need to ensure
the IAM identity has been given the correct IAM permissions for the tasks you
intend to perform. See [Knowledge Catalog IAM permissions][iam-permissions]
and [Knowledge Catalog IAM roles][iam-roles] for more information on
applying IAM permissions and roles to an identity.

[iam-overview]: https://cloud.google.com/dataplex/docs/iam-and-access-control
[adc]: https://cloud.google.com/docs/authentication#adc
[set-adc]: https://cloud.google.com/docs/authentication/provide-credentials-adc
[iam-permissions]: https://cloud.google.com/dataplex/docs/iam-permissions
[iam-roles]: https://cloud.google.com/dataplex/docs/iam-roles
[dataplex-docs]: https://cloud.google.com/dataplex

## Parameters

The `dataplex-update-data-product-aspects` tool accepts the following parameters:

| **field**     | **type**         | **required** | **description**                                                                                                                                                                                                                                            |
| ------------- | ---------------- | ------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| locationId    | string           | true         | The location ID (e.g. `us`, `us-central1`) of the Data Product.                                                                                                                                                                                         |
| dataProductId | string           | true         | The unique ID of the Data Product.                                                                                                                                                                                                                      |
| aspects       | array of objects | true         | The list of aspects to add or update on the Data Product Entry. Each object contains `projectId` (string, required), `locationId` (string, required), `aspectTypeId` (string, required), and `data` (object, optional, JSON map of the aspect details). |

### Aspect Object Fields

Each object in the `aspects` array must have the following fields:

*   **`projectId`**: The project ID of the aspect type. The value is `"dataplex-types"` for system aspects (like `overview` and `refresh-cadence`).
*   **`locationId`**: The location of the aspect type. The value is `"global"` for system aspects (like `overview` and `refresh-cadence`).
*   **`aspectTypeId`**: The unique name of the aspect type (e.g. `"overview"` or `"refresh-cadence"`).
*   **`data`**: The JSON payload conforming to the aspect type's schema:
    *   **System `overview` (Documentation) Schema**:
        *   `content` (string, required): The detailed documentation content (markdown or plain text).
        *   `contentType` (string, optional): The content type format. Values: `MARKDOWN`, `HTML`.
        *   `links` (array of objects, optional): List of relevant URL links. Each link object contains `url` (string, required) and `title` (string, optional).
    *   **System `refresh-cadence` (Contract) Schema**:
        *   `frequency` (string, required): How often the data is updated. Values: `Daily`, `Weekly`, `Monthly`, `Quarterly`, `Half-Yearly`, `Yearly`.
        *   `refreshTime` (string, optional): Time of day when the data is updated (e.g. `"09:00 PST"`).
        *   `thresholdInMinutes` (int, optional): Delinquency threshold in minutes (e.g. `15`).
        *   `cronSchedule` (string, optional): Optional cron schedule expression (e.g. `"0 * * * *"`).

## Example

```yaml
kind: tool
name: update_data_product_aspects
type: dataplex-update-data-product-aspects
source: my-dataplex-source
description: Use this tool to update aspects (like overview or contacts) on a Data Product Entry.
```

## Output Format

The tool returns the updated catalog entry details and aspects:

```json
{
  "locationId": "us",
  "dataProductId": "my-data-product",
  "entrySource": {
    "resource": "projects/...",
    "displayName": "My Data Product Resource",
    "description": "Resource details"
  },
  "entryType": "projects/dataplex-types/locations/global/entryTypes/data-product",
  "aspects": [
    {
      "projectId": "dataplex-types",
      "locationId": "global",
      "aspectTypeId": "overview",
      "data": {
        "content": "Updated description content"
      }
    }
  ]
}
```

## Reference

| **field**   | **type** | **required** | **description**                                      |
| ----------- | -------- | ------------ | ---------------------------------------------------- |
| type        | string   | true         | Must be "dataplex-update-data-product-aspects".      |
| source      | string   | true         | Name of the source the tool should execute on.       |
| description | string   | true         | Description of the tool that is passed to the LLM.   |
