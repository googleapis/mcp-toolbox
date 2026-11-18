---
title: "Versioning Policy"
type: docs
weight: 10
description: How MCP Toolbox manages versions and breaking changes.
---

MCP Toolbox for Databases follows [Semantic Versioning](https://semver.org/).

## Definition of the Public API

For the purposes of this policy, the "Public API" includes:

* **Server**   
  * **CLI:** The execution engine and lifecycle manager for tool hosting.  
  * **Configuration Manifests:** The structural specification of `tools.yaml`.  
  * **Pre-built Configs:** Curated sets of tools (and other MCP primitives) including the CLI flag, source configuration, toolsets names, and tools. 
  * **MCP versions**: Supporting MCP revisions and transport protocols.   
* **Client SDKs:** Both the foundational "Base SDKs" and the orchestration-specific "Integrated SDKs".

## What Constitutes a Breaking Change (Major Version Bump) 

A major version bump (e.g., v1.x.x to v2.0.0) is required for the following modifications:

* **Server:** Supporting MCP revisions and transport protocols.   
  * **CLI & Config:** Removing existing CLI flags or introducing backwards-incompatible changes to the core configuration format.  
  * **Pre-built Configs:** Renaming or removing the name of a pre-built toolset. Agents rely on the toolset names for discovery, so altering them breaks downstream integrations.  
* **Client SDKs:** Removing or renaming public methods, modifying expected input payload structures, or changing expected return types.  
* **MCP Protocol Support:** Removing support for an existing MCP protocol version. Until official MCP protocol guidelines dictate otherwise, dropping an MCP version counts as a major breaking change. A deprecation warning will be provided prior to removal, aligning with typical new specification cycle timelines.

## What is NOT a Breaking Change (Minor/Patch Version) 

The following changes will **not** trigger a major version bump:

* **Server**  
  * **Pre-built Config Modifications:** Adding, removing, or renaming individual tools within a pre-built toolset, as well as modifying server description, prompts, resources, tool descriptions or inputs, are treated as non-breaking changes.  
* **Experimental Features:** Features or wrapper packages explicitly documented as "Preview" or "Beta" may introduce breaking changes without a major version bump to the core project.

## Container Tagging Policy

Official container images are published to `us-central1-docker.pkg.dev/database-toolbox/toolbox/toolbox`:

* **Version tags (`X.Y.Z`, e.g., `1.14.0`):** Immutable tags pinned to a specific release digest (`linux/amd64` and `linux/arm64`). Once published, version tags are never overwritten. Pinning to a specific version tag or image digest (`@sha256:...`) is recommended for production workloads.
* **`latest` tag:** A floating tag that points to the most recent stable release digest and is updated automatically whenever a new versioned release is promoted.
