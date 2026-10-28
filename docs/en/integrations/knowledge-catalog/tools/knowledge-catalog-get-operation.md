---
title: "dataplex-get-operation"
type: docs
weight: 1
description: >
  Retrieves the status of an asynchronous Dataplex Long-Running Operation (LRO).
aliases:
  - /integrations/dataplex/tools/dataplex-get-operation/
---

## About

A `dataplex-get-operation` tool retrieves the status of a Dataplex long-running operation (LRO) (such as scan creation, data product creation/update, or data asset creation/update).

Poll this tool until the `done` field from the response is `true`. Once completed, the `response` field will contain the details of the created or updated resource (such as `DataScan`, `DataProduct`, or `DataAsset`). 
For scan template creation, you can extract the `scanId` (the last part of the `name` field, e.g. `nq-doc-1234`) to pass to `get_run_status` and retrieve execution results.

WARNING: For scans, this only tracks the creation of the scan template, NOT the actual background execution.

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

## Parameters

The `dataplex-get-operation` tool accepts the following parameters:

| **field**     | **type** | **required** | **description**                                                                                                    |
| ------------- | :------: | :----------: | ------------------------------------------------------------------------------------------------------------------ |
| operationName |  string  |     true     | The full operation resource name (format: `projects/{projectId}/locations/{locationId}/operations/{operationId}`). |

## Example

```yaml
kind: tool
name: get_operation
type: dataplex-get-operation
source: my-dataplex-source
description: Check the status of a long-running scan template creation.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
| ----------- | :------: | :----------: | -------------------------------------------------- |
| type        |  string  |     true     | Must be "dataplex-get-operation".                  |
| source      |  string  |     true     | Name of the source the tool should execute on.     |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
