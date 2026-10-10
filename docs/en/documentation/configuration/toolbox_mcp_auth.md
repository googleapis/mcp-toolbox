---
title: "Toolbox with MCP Authorization"
type: docs
weight: 4
description: >
  How to set up and configure Toolbox with [MCP Authorization](https://modelcontextprotocol.io/specification/2025-11-25/basic/authorization).
---

## Overview

Toolbox supports integration with Model Context Protocol (MCP) clients by acting as a Resource Server that implements OAuth 2.1 authorization. This enables Toolbox to validate JWT-based Bearer tokens before processing requests for resources or tool executions.

This guide details the specific configuration steps required to deploy Toolbox with MCP Auth enabled.

## Step 1: Configure the `generic` Auth Service

Update your `tools.yaml` file to use a `generic` authorization service with `mcpEnabled` set to `true`. This instructs Toolbox to intercept requests on the `/mcp` routes and validate Bearer tokens using the JWKS (JSON Web Key Set) fetched from your OIDC provider endpoint (`authorizationServer`).

```yaml
kind: authService
name: my-mcp-auth
type: generic
mcpEnabled: true
authorizationServer: "https://accounts.google.com" # Your authorization server URL
audience: "your-mcp-audience" # Matches the `aud` claim in the JWT
scopesRequired:
    - "mcp:tools"
```

When `mcpEnabled` is true, Toolbox also provisions the `/.well-known/oauth-protected-resource` Protected Resource Metadata (PRM) endpoint automatically using the `authorizationServer`.

## Step 2: Deployment

Deploying Toolbox with MCP auth requires defining an absolute `TOOLBOX_URL` (including scheme and host, e.g., `https://my-toolbox.example.com`) that the deployed service will use, as this URL must be included as the `resource` field in the PRM returned to the client.

You can set this absolute URL either through the `TOOLBOX_URL` environment variable or the `--toolbox-url` command-line flag during deployment.

### Local Deployment

To run Toolbox locally with MCP auth enabled, simply export the `TOOLBOX_URL` referencing your local port before running the binary:

```bash
export TOOLBOX_URL="http://127.0.0.1:5000"
./toolbox --tools-file tools.yaml
```

If you prefer to use the `--toolbox-url` flag explicitly:

```bash
./toolbox --tools-file tools.yaml --toolbox-url "http://127.0.0.1:5000"
```

### Cloud Run Deployment

```bash
export IMAGE="us-central1-docker.pkg.dev/database-toolbox/toolbox/toolbox:latest"

# Pass your target Cloud Run URL to the `--toolbox-url` flag
gcloud run deploy toolbox \
    --image $IMAGE \
    --service-account toolbox-identity \
    --region us-central1 \
    --set-secrets "/app/tools.yaml=tools:latest" \
    --args="--tools-file=/app/tools.yaml","--address=0.0.0.0","--port=8080","--toolbox-url=${CLOUD_RUN_TOOLBOX_URL}"
```

### Alternative: Manual PRM File Override

If you strictly need to define your own Protected Resource Metadata instead of auto-generating it from the `AuthService` config, you can use the `--mcp-prm-file <path>` flag. 

1. Create a `prm.json` containing the RFC-9207 compliant metadata. Note that the `resource` field must match the `TOOLBOX_URL`:
   ```json
   {
     "resource": "https://toolbox-service-123456789-uc.a.run.app",
     "authorization_servers": ["https://your-auth-server.example.com"],
     "scopes_supported": ["mcp:tools"],
     "bearer_methods_supported": ["header"]
   }
   ```
2. Set the `--mcp-prm-file` flag to the path of the PRM file.

 - If you are using local deployment, you can just provide the path to the file directly:
   ```bash
   ./toolbox --tools-file tools.yaml --mcp-prm-file prm.json
   ```
 - If you are using Cloud Run, upload it to GCP Secret Manager and Attach the secret to the Cloud Run deployment and provide the flag.
    ```bash
    gcloud secrets create prm_file --data-file=prm.json

    gcloud run deploy toolbox \
      # ... previous args
      --set-secrets "/app/tools.yaml=tools:latest,/app/prm.json=prm_file:latest" \
      --args="--tools-file=/app/tools.yaml","--mcp-prm-file=/app/prm.json","--port=8080"
    ```

## Step 3: Connecting to the Secure MCP Endpoint

Once the Cloud Run instance is deployed, your MCP client must obtain a valid JWT token from your authorization server (the `authorizationServer` in `tools.yaml`).

The client should provide this JWT via the standard HTTP `Authorization` header when connecting to the Streamable HTTP or SSE endpoint (`/mcp`):

```bash
{
  "mcpServers": {
    "toolbox-secure": {
      "type": "http",
      "url": "https://toolbox-service-123456789-uc.a.run.app/mcp",
      "headers": {
        "Authorization": "Bearer <your-jwt-access-token>"
      }
    }
  }
}
```
Important: The token provided in the Authorization header must be a JWT token (issued by the auth server you configured previously), not a Google Cloud Run access token.

Toolbox will intercept incoming connections, fetch the latest JWKS from your authorizationServer, and validate that the aud (audience), signature, and scopes on the JWT match the requirements defined by your mcpEnabled auth service.

If your Cloud Run service also requires IAM authentication, you must pass the Cloud Run identity token using [Cloud Run's alternate auth header][cloud-run-alternate-auth-header] to avoid conflicting with Toolbox's internal authentication.

[cloud-run-alternate-auth-header]: https://docs.cloud.google.com/run/docs/authenticating/service-to-service#acquire-token
