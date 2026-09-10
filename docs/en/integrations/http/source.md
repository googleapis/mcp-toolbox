---
title: "HTTP Source"
linkTitle: "Source"
type: docs
weight: 1
description: >
  The HTTP source enables the Toolbox to retrieve data from a remote server using HTTP requests.
no_list: true
---

## About

The HTTP Source allows Toolbox to retrieve data from arbitrary HTTP
endpoints. This enables Generative AI applications to access data from web APIs
and other HTTP-accessible resources.



## Available Tools

{{< list-tools >}}

## Example

```yaml
kind: source
name: my-http-source
type: http
baseUrl: https://api.example.com/data
timeout: 10s # default to 30s
headers:
  Authorization: Bearer ${API_KEY}
  Content-Type: application/json
queryParams:
  param1: value1
  param2: value2
# returnFullError: false
# disableSslVerification: false
```

{{< notice tip >}}
Use environment variable replacement with the format ${ENV_NAME}
instead of hardcoding your secrets into the configuration file.
{{< /notice >}}

## Reference

| **field**              |     **type**      | **required** | **description**                                                                                                                    |
|------------------------|:-----------------:|:------------:|------------------------------------------------------------------------------------------------------------------------------------|
| type                   |      string       |     true     | Must be "http".                                                                                                                    |
| baseUrl                |      string       |     true     | The base URL for the HTTP requests (e.g., `https://api.example.com`).                                                              |
| timeout                |      string       |    false     | The timeout for HTTP requests (e.g., "5s", "1m", refer to [ParseDuration][parse-duration-doc] for more examples). Defaults to 30s. |
| headers                | map[string]string |    false     | Default headers to include in the HTTP requests.                                                                                   |
| queryParams            | map[string]string |    false     | Default query parameters to include in the HTTP requests.                                                                          |
| returnFullError        |       bool        |    false     | Include raw upstream response bodies in error messages for non-2xx responses. Defaults to `false`.                                 |
| disableSslVerification |       bool        |    false     | Disable SSL certificate verification. This should only be used for local development. Defaults to `false`.                         |
| allowPrivateNetworks   |       bool        |    false     | Allow requests and redirects to loopback and private networks (RFC 1918 / link-local). Defaults to `false`.                         |
| allowedIpRanges        |     []string      |    false     | List of IP addresses or CIDR blocks to explicitly allow (whitelisted overrides).                                                   |
| customBlockedIpRanges  |     []string      |    false     | List of IP addresses or CIDR blocks to explicitly block.                                                                           |

## Advanced Usage

### SSRF Protection (SSRF Guard)
By default, the HTTP source implements strict protection against Server-Side Request Forgery (SSRF) and DNS Rebinding (TOCTOU) attacks. It automatically intercepts, resolves, and blocks connection requests to private IP ranges, loopback ranges (such as `127.0.0.1`), link-local ranges (e.g. AWS/GCP metadata service at `169.254.169.254`), the RFC 6598 shared address space (`100.64.0.0/10`, commonly used for Kubernetes node and Pod networking), and the RFC 6890 IETF protocol assignments range (`192.0.0.0/24`, used by special-purpose protocol machinery such as NAT64/DNS64 translation components).

To override the default protection or block custom ranges, configure `allowPrivateNetworks`, `allowedIpRanges`, and `customBlockedIpRanges`:

```yaml
kind: source
name: my-http-source
type: http
baseUrl: https://internal.corp/api
allowedIpRanges:
  - 10.0.0.0/24         # Explicitly trust internal subnet
customBlockedIpRanges:
  - 10.0.0.99           # Block a specific sensitive host inside the subnet
```

[parse-duration-doc]: https://pkg.go.dev/time#ParseDuration
