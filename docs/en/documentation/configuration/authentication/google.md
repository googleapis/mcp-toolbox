---
title: "Google Sign-In"
type: docs
weight: 1
description: >
  Use Google Sign-In for OAuth 2.0 flow and token lifecycle.
---

## Getting Started

Google Sign-In manages the OAuth 2.0 flow and token lifecycle. To integrate the Google Sign-In workflow to your web app, [follow this guide][gsi-setup].

After setting up Google Sign-In, configure your auth service in the toolbox. The Google auth provider supports two distinct validation modes:

1. **Web App Claims (OIDC)**: Used to authenticate user requests in web applications.
2. **MCP Authorization**: Used to secure the entire MCP server transport (SSE or Stdio).

[gsi-setup]: https://developers.google.com/identity/sign-in/web/sign-in

## Configuration Modes

### 1. Web App OIDC Authentication
If you are developing a web application using the Toolbox and need to retrieve user claims from Google ID tokens sent in custom request headers, configure the `clientId` field.

- **Header**: Expects the token in the `<name>_token` header (e.g. `my-google-auth_token`).
- **Token Type**: Google OIDC ID tokens (JWT).

#### Example
```yaml
kind: authService
name: my-google-auth
type: google
clientId: ${YOUR_GOOGLE_CLIENT_ID}
```

---

### 2. MCP Authorization
To secure all endpoints on your MCP server using Google OAuth tokens, enable `mcpEnabled` and specify the `audience` field.

- **Header**: Expects the token in the standard `Authorization: Bearer <token>` header.
- **Token Type**: Supports both Google **ID tokens (JWT)** and Google **opaque access tokens**.

#### Example
```yaml
kind: authService
name: my-google-auth
type: google
audience: ${YOUR_GOOGLE_CLIENT_ID}
mcpEnabled: true
scopesRequired:
  - https://www.googleapis.com/auth/userinfo.email
```

> [!IMPORTANT]
> - For **ID tokens (JWT)**: Local cryptographic signature verification is performed, which requires `audience` to be configured. If `audience` is not set, the provider will fall back to using `clientId`. If neither is configured, validation will fail.
> - For **Opaque tokens**: The provider automatically queries Google's secure tokeninfo endpoint (`https://oauth2.googleapis.com/tokeninfo`) and validates the resulting audience against the configured `audience` field (falling back to `clientId` if `audience` is not set).

---

## Behavior

### Authorized Invocations
When using [Authorized Invocations][auth-invoke], a tool will be considered authorized if it has a valid OAuth 2.0 token that matches the Client ID or Audience.

[auth-invoke]: ../tools/_index.md#authorized-invocations

### Authenticated Parameters
When using [Authenticated Parameters][auth-params], any [claim provided by the id-token][provided-claims] can be used for the parameter.

For **opaque access tokens** (MCP Auth mode), the claims are built from Google's tokeninfo response and are limited to `aud`, `scope`, `sub` and, when `email_verified` is true, `email` and `email_verified`. The `sub` and `email` claims are only present if the token was granted the `openid` / `email` scopes.

[auth-params]: ../tools/_index.md#authenticated-parameters
[provided-claims]: https://developers.google.com/identity/openid-connect/openid-connect#obtaininguserprofileinformation

---

## Reference

| **field**      | **type** | **required** | **description**                                                                                                                              |
|----------------|:--------:|:------------:|----------------------------------------------------------------------------------------------------------------------------------------------|
| type           |  string  |     true     | Must be "google".                                                                                                                            |
| clientId       |  string  |    false     | Client ID of your application. Required for validating ID tokens in non-MCP web apps (`GetClaimsFromHeader`), and acts as a fallback for `audience` in MCP auth mode if `audience` is not configured. |
| audience       |  string  |    false     | Expected audience. Required for validating ID tokens in MCP Auth mode (unless `clientId` is configured as a fallback). If specified, also validates opaque token audiences. Disallowed if `mcpEnabled` is false. |
| mcpEnabled     |   bool   |    false     | Enforces global MCP transport authentication using the `Authorization: Bearer` header. Defaults to false.                                    |
| scopesRequired | []string |    false     | A list of required scopes that must be present in the token's claims/metadata to be considered valid. Disallowed if `mcpEnabled` is false. |
