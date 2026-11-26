---
title: "Cloud Run"
type: docs
weight: 1
description: >
  How to set up and configure Toolbox to run on Cloud Run using prebuilt configs or custom configuration files.
---


## Before you begin

1. [Install](https://cloud.google.com/sdk/docs/install) the Google Cloud CLI.

1. Set the PROJECT_ID environment variable:

    ```bash
    export PROJECT_ID="my-project-id"
    ```

1. Initialize gcloud CLI:

    ```bash
    gcloud init
    gcloud config set project $PROJECT_ID
    ```

1. Make sure you've set up and initialized your database.

1. You must have the following APIs enabled:

    ```bash
    gcloud services enable run.googleapis.com \
                           cloudbuild.googleapis.com \
                           artifactregistry.googleapis.com \
                           iam.googleapis.com \
                           secretmanager.googleapis.com

    ```

1. To create an IAM account, you must have the following IAM permissions (or
   roles):
    - Create Service Account role (roles/iam.serviceAccountCreator)

1. To create a secret, you must have the following roles:
    - Secret Manager Admin role (roles/secretmanager.admin)

1. To deploy to Cloud Run, you must have the following set of roles:
    - Cloud Run Developer (roles/run.developer)
    - Service Account User role (roles/iam.serviceAccountUser)

{{< notice note >}}
If you are using sources that require VPC-access (such as
AlloyDB or Cloud SQL over private IP), make sure your Cloud Run service and the
database are in the same VPC network.
{{< /notice >}}

## Create a service account

1. Create a backend service account if you don't already have one:

    ```bash
    gcloud iam service-accounts create toolbox-identity
    ```

1. Grant permissions to use secret manager:

    ```bash
    gcloud projects add-iam-policy-binding $PROJECT_ID \
        --member serviceAccount:toolbox-identity@$PROJECT_ID.iam.gserviceaccount.com \
        --role roles/secretmanager.secretAccessor
    ```

1. Grant additional permissions to the service account that are specific to the
   source, e.g.:
    - [AlloyDB for PostgreSQL](../../../integrations/alloydb/source.md#iam-authentication)
    - [Cloud SQL for PostgreSQL](../../../integrations/cloud-sql-pg/source.md#iam-permissions)
    - [BigQuery](../../../integrations/bigquery/source.md)

## Configure Toolbox

Toolbox can be deployed using either **Prebuilt Configurations** (`--prebuilt`) or a **Custom Configuration File** (`tools.yaml`).

### Popular Prebuilt Configurations

Prebuilt configurations allow you to deploy Toolbox without creating a `tools.yaml` file. You pass required connection parameters through environment variables stored securely in Secret Manager.

Below are examples of environment variables required for popular prebuilt toolsets:

- **AlloyDB for PostgreSQL (`--prebuilt=alloydb-postgres`)**
  - `ALLOYDB_POSTGRES_PROJECT`: The GCP project ID.
  - `ALLOYDB_POSTGRES_REGION`: The region of your AlloyDB cluster.
  - `ALLOYDB_POSTGRES_CLUSTER`: The ID of your AlloyDB cluster.
  - `ALLOYDB_POSTGRES_INSTANCE`: The ID of your AlloyDB instance.
  - `ALLOYDB_POSTGRES_DATABASE`: The database name.

- **Cloud SQL for PostgreSQL (`--prebuilt=cloud-sql-postgres`)**
  - `CLOUD_SQL_POSTGRES_PROJECT`: The GCP project ID.
  - `CLOUD_SQL_POSTGRES_REGION`: The region of your Cloud SQL instance.
  - `CLOUD_SQL_POSTGRES_INSTANCE`: The Cloud SQL instance ID.
  - `CLOUD_SQL_POSTGRES_DATABASE`: The database name.

- **BigQuery (`--prebuilt=bigquery`)**
  - `BIGQUERY_PROJECT`: The GCP project ID.
  - `BIGQUERY_LOCATION`: (Optional) Dataset location (e.g. `US`).

For a full list of supported prebuilt toolsets and their required environment variables, see the [Prebuilt Configs Catalog](../../configuration/prebuilt-configs/_index.md).

### Custom Configuration (`tools.yaml`)

For custom tool definitions or multi-source configurations, create a `tools.yaml` file containing your configuration. For details, see the [Configuration](../../configuration/_index.md) section.

## Deploy to Cloud Run

Choose one of the deployment modes below:

{{< tabpane persist=header >}}
{{< tab header="Prebuilt Config (--prebuilt)" lang="bash" >}}
# 1. Create secrets in Secret Manager for required environment variables (Example: AlloyDB)
echo -n "$ALLOYDB_POSTGRES_PROJECT" | gcloud secrets create ALLOYDB_POSTGRES_PROJECT \
    --data-file=- \
    --replication-policy="automatic"

echo -n "$ALLOYDB_POSTGRES_REGION" | gcloud secrets create ALLOYDB_POSTGRES_REGION \
    --data-file=- \
    --replication-policy="automatic"

echo -n "$ALLOYDB_POSTGRES_CLUSTER" | gcloud secrets create ALLOYDB_POSTGRES_CLUSTER \
    --data-file=- \
    --replication-policy="automatic"

echo -n "$ALLOYDB_POSTGRES_INSTANCE" | gcloud secrets create ALLOYDB_POSTGRES_INSTANCE \
    --data-file=- \
    --replication-policy="automatic"

echo -n "$ALLOYDB_POSTGRES_DATABASE" | gcloud secrets create ALLOYDB_POSTGRES_DATABASE \
    --data-file=- \
    --replication-policy="automatic"

# 2. Set the Toolbox container image
export IMAGE=us-central1-docker.pkg.dev/database-toolbox/toolbox/toolbox:latest

# 3. Deploy to Cloud Run with prebuilt toolset flag
gcloud run deploy toolbox \
    --image=$IMAGE \
    --region=us-central1 \
    --service-account toolbox-identity \
    --args="--prebuilt=alloydb-postgres","--address=0.0.0.0","--port=8080" \
    --set-secrets="\
ALLOYDB_POSTGRES_PROJECT=ALLOYDB_POSTGRES_PROJECT:latest,\
ALLOYDB_POSTGRES_REGION=ALLOYDB_POSTGRES_REGION:latest,\
ALLOYDB_POSTGRES_CLUSTER=ALLOYDB_POSTGRES_CLUSTER:latest,\
ALLOYDB_POSTGRES_INSTANCE=ALLOYDB_POSTGRES_INSTANCE:latest,\
ALLOYDB_POSTGRES_DATABASE=ALLOYDB_POSTGRES_DATABASE:latest" \
    --allow-unauthenticated

# If using a VPC network for private IP database access:
# gcloud run deploy toolbox \
#     --image=$IMAGE \
#     --region=us-central1 \
#     --service-account toolbox-identity \
#     --args="--prebuilt=alloydb-postgres","--address=0.0.0.0","--port=8080" \
#     --set-secrets="\
# ALLOYDB_POSTGRES_PROJECT=ALLOYDB_POSTGRES_PROJECT:latest,\
# ALLOYDB_POSTGRES_REGION=ALLOYDB_POSTGRES_REGION:latest,\
# ALLOYDB_POSTGRES_CLUSTER=ALLOYDB_POSTGRES_CLUSTER:latest,\
# ALLOYDB_POSTGRES_INSTANCE=ALLOYDB_POSTGRES_INSTANCE:latest,\
# ALLOYDB_POSTGRES_DATABASE=ALLOYDB_POSTGRES_DATABASE:latest" \
#     --network default \
#     --subnet default \
#     --allow-unauthenticated
{{< /tab >}}
{{< tab header="Custom Config (tools.yaml)" lang="bash" >}}
# 1. Upload tools.yaml as a secret
gcloud secrets create tools --data-file=tools.yaml

# If updating an existing secret:
# gcloud secrets versions add tools --data-file=tools.yaml

# 2. Set the Toolbox container image
export IMAGE=us-central1-docker.pkg.dev/database-toolbox/toolbox/toolbox:latest

# 3. Deploy to Cloud Run using config file
gcloud run deploy toolbox \
    --image=$IMAGE \
    --region=us-central1 \
    --service-account toolbox-identity \
    --set-secrets="/app/tools.yaml=tools:latest" \
    --args="--config=/app/tools.yaml","--address=0.0.0.0","--port=8080" \
    --allow-unauthenticated

# If using a VPC network:
# gcloud run deploy toolbox \
#     --image=$IMAGE \
#     --region=us-central1 \
#     --service-account toolbox-identity \
#     --set-secrets="/app/tools.yaml=tools:latest" \
#     --args="--config=/app/tools.yaml","--address=0.0.0.0","--port=8080" \
#     --network default \
#     --subnet default \
#     --allow-unauthenticated
{{< /tab >}}
{{< /tabpane >}}

{{< notice note >}}
**The `$PORT` Environment Variable**  
Google Cloud Run dictates the port your application must listen on by setting
the `$PORT` environment variable inside your container. This value defaults to
**8080**. Your application's `--port` argument **must** be set to listen on this
port. If there is a mismatch, the container will fail to start and the
deployment will time out.  
{{< /notice >}}

{{< notice note >}}
If you are using a VPC network, use the command below. The
`--network default` and `--subnet default` values are examples for the
default VPC network and the default subnet in the Cloud Run region. If your
database uses a different VPC network or subnet, replace these values. To
find the values for your project, run:
```bash
gcloud compute networks list
gcloud compute networks subnets list --regions=us-central1
```
For more information, see [Direct VPC egress with a VPC
network](https://cloud.google.com/run/docs/configuring/vpc-direct-vpc).
{{< /notice >}}

{{< notice note >}}
This guide uses `--allow-unauthenticated` flag to allow public access to the Cloud Run service.
To enforce the Cloud Run Invoker IAM check, see [Re-enable the Cloud Run Invoker IAM check](https://docs.cloud.google.com/run/docs/authenticating/public#re-enable_invoker)
{{< /notice >}}

### Update deployed server to be secure

{{< production-security-warning >}}

To prevent DNS rebinding attacks, use the `--allowed-hosts` flag to specify a
list of hosts. In order to do that, re-deploy the Cloud Run service with the new flags.

To implement CORS checks, use the `--allowed-origins` flag to specify a list of
origins permitted to access the server.

1. Set environment variables for the Cloud Run URL and host:

    ```bash
    export URL=<cloud run url>
    export HOST=<cloud run host>
    ```

2. Redeploy Toolbox with security flags:

    For **Prebuilt Config**:

    ```bash
    gcloud run deploy toolbox \
        --image=$IMAGE \
        --region=us-central1 \
        --service-account toolbox-identity \
        --args="--prebuilt=alloydb-postgres","--address=0.0.0.0","--port=8080","--allowed-origins=$URL","--allowed-hosts=$HOST" \
        --set-secrets="\
ALLOYDB_POSTGRES_PROJECT=ALLOYDB_POSTGRES_PROJECT:latest,\
ALLOYDB_POSTGRES_REGION=ALLOYDB_POSTGRES_REGION:latest,\
ALLOYDB_POSTGRES_CLUSTER=ALLOYDB_POSTGRES_CLUSTER:latest,\
ALLOYDB_POSTGRES_INSTANCE=ALLOYDB_POSTGRES_INSTANCE:latest,\
ALLOYDB_POSTGRES_DATABASE=ALLOYDB_POSTGRES_DATABASE:latest" 
    ```

    For **Custom Config (`tools.yaml`)**:

    ```bash
    gcloud run deploy toolbox \
        --image=$IMAGE \
        --region=us-central1 \
        --service-account toolbox-identity \
        --set-secrets="/app/tools.yaml=tools:latest" \
        --args="--config=/app/tools.yaml","--address=0.0.0.0","--port=8080","--allowed-origins=$URL","--allowed-hosts=$HOST" 
    ```

## Connecting with Toolbox Client SDK

You can connect to Toolbox Cloud Run instances directly through the SDK.

1. [Set up `Cloud Run Invoker` role
   access](https://cloud.google.com/run/docs/securing/managing-access#service-add-principals)
   to your Cloud Run service.

1. (Only for local runs) Set up [Application Default
   Credentials](https://cloud.google.com/docs/authentication/set-up-adc-local-dev-environment)
   for the principal you set up the `Cloud Run Invoker` role access to.

1. Run the following to retrieve a non-deterministic URL for the Cloud Run service:

    ```bash
    gcloud run services describe toolbox --format 'value(status.url)'
    ```

1. Import and initialize the toolbox client with the URL retrieved above:

    {{< tabpane persist=header >}}
{{< tab header="Python" lang="python" >}}
import asyncio
from toolbox_core import ToolboxClient, auth_methods

# Replace with the Cloud Run service URL generated in the previous step
URL = "https://cloud-run-url.app"

auth_token_provider = auth_methods.aget_google_id_token(URL) # can also use sync method

async def main():
  async with ToolboxClient(
      URL,
      client_headers={"Authorization": auth_token_provider},
  ) as toolbox:
    toolset = await toolbox.load_toolset()
    # ...

asyncio.run(main())
{{< /tab >}}
{{< tab header="Javascript" lang="javascript" >}}
import { ToolboxClient } from '@toolbox-sdk/core';
import {getGoogleIdToken} from '@toolbox-sdk/core/auth'

// Replace with the Cloud Run service URL generated in the previous step.
const URL = 'http://127.0.0.1:5000';
const authTokenProvider = () => getGoogleIdToken(URL);

const client = new ToolboxClient(URL, null, {"Authorization": authTokenProvider});
{{< /tab >}}
{{< tab header="Go" lang="go" >}}
import "github.com/googleapis/mcp-toolbox-sdk-go/core"

func main() {
    // Replace with the Cloud Run service URL generated in the previous step.
    URL := "http://127.0.0.1:5000"
    auth_token_provider, err := core.GetGoogleIDToken(ctx, URL)
    if err != nil {
        log.Fatalf("Failed to fetch token %v", err)
    }
    toolboxClient, err := core.NewToolboxClient(
        URL,
        core.WithClientHeaderString("Authorization", auth_token_provider))
    if err != nil {
        log.Fatalf("Failed to create Toolbox client: %v", err)
    }
}
{{< /tab >}}
{{< /tabpane >}}

Now, you can use this client to connect to the deployed Cloud Run instance!

## Troubleshooting

{{< notice note >}}
For any deployment or runtime error, the best first step is to check the logs
for your service in the Google Cloud Console's Cloud Run section. They often
contain the specific error message needed to diagnose the problem.
{{< /notice >}}

- **Deployment Fails with "Container failed to start":** This is almost always
    caused by a port mismatch. Ensure your container's `--port` argument is set to
    `8080` to match the `$PORT` environment variable provided by Cloud Run.

- **Missing Environment Variables for Prebuilt Configs:** When using `--prebuilt`, ensure all required environment variables for the selected toolset are passed via `--set-secrets` or `--set-env-vars`. Check the [Prebuilt Configs Catalog](../../configuration/prebuilt-configs/_index.md) for required variable names.

- **Client Receives Permission Denied Error (401 or 403):** If your client
  application (e.g., your local SDK) gets a `401 Unauthorized` or `403
  Forbidden` error when trying to call your Cloud Run service, it means the
  client is not properly authenticated as an invoker.
  - Ensure the user or service account calling the service has the **Cloud Run
      Invoker** (`roles/run.invoker`) IAM role.
  - If running locally, make sure your Application Default Credentials are set
      up correctly by running `gcloud auth application-default login`.

- **Service Fails to Access Secrets (in logs):** If your application starts but
  the logs show errors like "permission denied" when trying to access Secret
  Manager, it means the Toolbox service account is missing permissions.
  - Ensure the `toolbox-identity` service account has the **Secret Manager
      Secret Accessor** (`roles/secretmanager.secretAccessor`) IAM role.

- **Cloud Run Connections via IAP:** Currently we do not support Cloud Run connections via [IAP](https://docs.cloud.google.com/iap/docs/concepts-overview). Please disable IAP if you are using it.

