# Experimental MCP Toolbox Extensions

This directory contains specifications and schemas for experimental Model Context Protocol (MCP) extensions supported by MCP Toolbox. Toolbox includes  features that fall outside the official MCP specification. While some capabilities may remain proprietary, others are experimental features being incubated for potential inclusion in the core specification.

## Extension Identifier

The identifier we are using for this extension is `com.google.cloud/toolbox.<version_string>`. It packages all the custom experimental Toolbox features (such as groups, secure parameters, and authenticated parameters) under a single extension namespace.

## Versioning Strategy

- **Strict Backward Compatibility Within Versions:** Once a version is established, all updates to it must be non-breaking. New capabilities can be added and schemas updated, provided they fail gracefully if unsupported and do not break existing implementations.
- **Breaking Changes Between Versions:** Breaking changes are allowed when introducing a new protocol version release.
- **Graduation to Official Spec:** When an experimental feature is officially adopted into the core MCP specification, it will be removed from this experimental extension package. Clients must transition to the official implementation to utilize the feature.
