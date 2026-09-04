/**
 * Extension Identifier: com.google.cloud/toolbox.v1
 * Protocol Version: 2026-07-28
 *
 * This schema defines the TypeScript interfaces for the Secure Parameters feature
 * under the com.google.cloud/toolbox.v1 MCP extension.
 */

/**
 * Extension identifier constant.
 */
export const TOOLBOX_EXTENSION_ID = "com.google.cloud/toolbox.v1";

/**
 * Client capabilities metadata structure advertising support for com.google.cloud/toolbox.v1.
 */
export interface SecureParamsClientCapabilities {
  extensions?: {
    [TOOLBOX_EXTENSION_ID]?: Record<string, unknown>;
    [key: string]: unknown;
  };
}

/**
 * Augmented JSON Schema object representing parameters.
 */
export interface ParameterSchema {
  type: "object";
  properties?: Record<string, Record<string, unknown>>;
  required?: string[];
  [key: string]: unknown;
}

/**
 * Tool definition in `tools/list` results augmented with `secureInputSchema`.
 */
export interface ToolWithSecureParams {
  /**
   * The unique name of the tool.
   */
  name: string;

  /**
   * Human-readable description of the tool (prompt hint for the LLM).
   */
  description?: string;

  /**
   * JSON Schema object defining standard parameters exposed to the LLM agent.
   */
  inputSchema: ParameterSchema;

  /**
   * JSON Schema object defining sensitive runtime parameters hidden from the LLM agent
   * and passed out-of-band by the calling application.
   */
  secureInputSchema?: ParameterSchema;

  /**
   * Optional tool annotations.
   */
  annotations?: {
    destructiveHint?: boolean;
    idempotentHint?: boolean;
    openWorldHint?: boolean;
    readOnlyHint?: boolean;
    [key: string]: unknown;
  };

  /**
   * Optional metadata fields (e.g. authInvoke, authParam).
   */
  _meta?: Record<string, unknown>;
}

/**
 * Augmented request parameters for `tools/call`.
 */
export interface CallToolParamsWithSecureParams {
  /**
   * The name of the tool to execute.
   */
  name: string;

  /**
   * Standard parameters passed by the LLM agent or caller.
   * Secure parameters MUST NOT be included in this object.
   */
  arguments?: Record<string, unknown>;

  /**
   * Secure parameters passed out-of-band by the client application.
   * Non-secure parameters MUST NOT be included in this object.
   */
  secureArguments?: Record<string, unknown>;

  /**
   * Request metadata including protocol version and client capabilities.
   */
  _meta?: {
    "io.modelcontextprotocol/protocolVersion"?: string;
    "io.modelcontextprotocol/clientCapabilities"?: SecureParamsClientCapabilities;
    [key: string]: unknown;
  };
}

/**
 * JSON-RPC Error codes related to Secure Parameters.
 */
export enum SecureParamsErrorCode {
  /**
   * Returned when invoking a tool requiring secure parameters, but the client
   * did not declare support for `com.google.cloud/toolbox.v1` in client capabilities.
   */
  MissingRequiredClientCapability = -32021,

  /**
   * Returned when parameter routing constraints are violated:
   * - A secure parameter is passed in standard `arguments`
   * - A non-secure parameter is passed in `secureArguments`
   */
  InvalidParams = -32602,
}
