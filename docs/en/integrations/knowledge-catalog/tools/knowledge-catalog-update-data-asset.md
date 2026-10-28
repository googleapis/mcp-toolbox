---
title: "dataplex-update-data-asset"
type: docs
weight: 1
description: >
  A "dataplex-update-data-asset" tool updates an existing Data Asset under a Data Product in Knowledge Catalog.
---

## About

A `dataplex-update-data-asset` tool updates an existing Data Asset under a Data Product in Knowledge Catalog (formerly known as Dataplex). This is a long-running operation, and the tool returns immediately with the operation's location ID and operation ID.

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

The `dataplex-update-data-asset` tool accepts the following parameters:

| **field**          | **type** | **required** | **description**                                                                                                                                                                                                                                                                                                                                                             |
| ------------------ | -------- | ------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| locationId         | string   | true         | The location ID (e.g. `us`, `us-central1`) where the parent Data Product is located.                                                                                                                                                                                                                                                                                        |
| dataProductId      | string   | true         | The unique ID of the parent Data Product.                                                                                                                                                                                                                                                                                                                                   |
| dataAssetId        | string   | true         | The unique ID of the Data Asset to update.                                                                                                                                                                                                                                                                                                                                  |
| labels             | map      | false        | The labels associated with the Data Asset. Keys and values must be strings.                                                                                                                                                                                                                                                                                                 |
| accessGroupConfigs | map      | false        | Map of access group configurations to associate with the Data Asset. Keys represent the access group ID, and the value is a list of string IAM role names (e.g. `{"test-group": ["roles/bigquery.dataViewer"]}`). To find the list of supported roles that can be granted on the resource, refer to the [roles:queryGrantableRoles][query-grantable-roles-docs] API method. |
| updateMask         | array    | false        | The list of fields to update. If not specified, all non-empty fields will be updated.                                                                                                                                                                                                                                                                                       |

[query-grantable-roles-docs]: https://cloud.google.com/iam/docs/reference/rest/v1/roles/queryGrantableRoles

## Example

```yaml
kind: tool
name: update_data_asset
type: dataplex-update-data-asset
source: my-dataplex-source
description: Use this tool to update a Data Asset.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
| ----------- | -------- | ------------ | -------------------------------------------------- |
| type        | string   | true         | Must be "dataplex-update-data-asset".              |
| source      | string   | true         | Name of the source the tool should execute on.     |
| description | string   | true         | Description of the tool that is passed to the LLM. |
