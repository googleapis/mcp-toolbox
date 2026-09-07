---
title: "FAQ"
type: docs
weight: 2
description: Frequently asked questions about Toolbox.
---

## How can I deploy or run Toolbox?

MCP Toolbox for Databases is open-source and can be run or deployed to a
multitude of environments. For convenience, we release [compiled binaries and
docker images][release-notes] (but you can always compile yourself as well!).

For detailed instructions, check out these resources:

- [Quickstart: How to Run Locally](../documentation/getting-started/local_quickstart.md)
- [Deploy to Cloud Run](../documentation/deploy-to/cloud-run/_index.md)
- To run Toolbox more securely or harden security, check out our [hardening guidelines](./cli.md#hardening-toolbox)

[release-notes]: https://github.com/googleapis/mcp-toolbox/releases/

## Do I need a Google Cloud account/project to get started with Toolbox?

Nope! While some of the sources Toolbox connects to may require GCP credentials,
Toolbox doesn't require them and can connect to a bunch of different resources
that don't.

## How do I configure Google Cloud telemetry with ADC when running locally?

If you run Toolbox with `--telemetry-gcp`, make sure Application Default Credentials are available via `GOOGLE_APPLICATION_CREDENTIALS` (or other ADC-compatible setup) and specify a project ID either by:

- setting `--telemetry-gcp-project`, or
- setting `GOOGLE_CLOUD_PROJECT` environment variable.

This avoids "no project found" errors when exporting telemetry to Google Cloud.

## How do I reduce the token cost of large tool results?

Set `--response-encoding=gcf` to return tool results as [Graph Compact Format](https://gcformat.com)
instead of JSON. For the uniform record arrays these tools return, GCF factors the repeated
field names into a single header, cutting token usage on large results (~30% on a typical row
set) when they cross the LLM boundary. It is opt-in and lossless: GCF is used only when it is
smaller than the JSON and round-trips back to the same values, otherwise JSON is returned. See
the [`--response-encoding` flag](./cli.md) for details.

## Does Toolbox take contributions from external users?

Absolutely! Please check out our [DEVELOPER.md][] for instructions on how to get
started developing _on_ Toolbox instead of with it, and the [CONTRIBUTING.md][]
for instructions on completing the CLA and getting a PR accepted.

[DEVELOPER.md]: https://github.com/googleapis/mcp-toolbox/blob/main/DEVELOPER.md
[CONTRIBUTING.MD]: https://github.com/googleapis/mcp-toolbox/blob/main/CONTRIBUTING.md

## Can Toolbox support a feature to let me do _$FOO_?

Maybe? The best place to start is by [opening an issue][github-issue] for
discussion (or seeing if there is already one open), so we can better understand
your use case and the best way to solve it. Generally we aim to prioritize the
most popular issues, so make sure to +1 ones you are the most interested in.

[github-issue]: https://github.com/googleapis/mcp-toolbox/issues

## Can Toolbox be used for non-database tools?

**Yes!** While Toolbox is primarily focused on databases, it also supports generic
**HTTP tools** (`type: http`). These allow you to connect your agents to REST APIs
and other web services, enabling workflows that extend beyond database interactions.

For configuration details, see the [HTTP Tools documentation](../integrations/http/_index.md).

## Can I use _$BAR_ orchestration framework to use tools from Toolbox?

Currently, Toolbox only supports a limited number of client SDKs at our initial
launch. We are investigating support for more frameworks as well as more general
approaches for users without a framework -- look forward to seeing an update
soon.

## Why does Toolbox use a server-client architecture pattern?

Toolbox's server-client architecture allows us to more easily support a wide
variety of languages and frameworks with a centralized implementation. It also
allows us to tackle problems like connection pooling, auth, or caching more
completely than entirely client-side solutions.

## Why was Toolbox written in Go?

While a large part of the Gen AI Ecosystem is predominately Python, we opted to
use Go. We chose Go because it's still easy and simple to use, but also easier
to write fast, efficient, and concurrent servers. Additionally, given the
server-client architecture, we can still meet many developers where they are
with clients in their preferred language. As Gen AI matures, we want developers
to be able to use Toolbox on the serving path of mission critical applications.
It's easier to build the needed robustness, performance and scalability in Go
than in Python.

## Is Toolbox compatible with Model Context Protocol (MCP)?

Yes! Toolbox is compatible with [Anthropic's Model Context Protocol
(MCP)](https://modelcontextprotocol.io/). Please checkout [Connect via
MCP](../documentation/connect-to/mcp-client/_index.md) on how to connect to Toolbox with an MCP
client.

## What is the difference between MCP Toolbox and Google Cloud MCP Servers?

MCP Toolbox and [Google Cloud MCP Servers](https://cloud.google.com/blog/products/databases/managed-mcp-servers-for-google-cloud-databases) serve complementary purposes.

- **Google Cloud MCP Servers** provides a managed, enterprise-grade experience with necessary stability and governance, offering a "Zero Ops" experience ideal for **[Build-Time Developers ("AI-Assisted")](../documentation/getting-started/_index.md#build-time-vs-runtime-implementation)** who need immediate, secure integration with Google Cloud services without any setup.
- **MCP Toolbox** is open-source and self-hosted, allowing for rapid prototyping. It is designed for **[Run-Time Developers](../documentation/getting-started/_index.md#build-time-vs-runtime-implementation)** who need to build custom agents/applications, deploy custom tools, and connect to non-Google or on-premises data sources. MCP Toolbox is focused on delivering the most up to date and cutting-edge features quickly before they might be available in managed environments.
