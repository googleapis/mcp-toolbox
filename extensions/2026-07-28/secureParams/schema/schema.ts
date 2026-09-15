// Copyright 2024 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

/**
 * Extension Identifier: com.google.cloud/toolbox.v1
 * Protocol Version: 2026-07-28
 *
 * This schema defines the TypeScript interfaces for the Secure Parameters feature
 * under the com.google.cloud/toolbox.v1 MCP extension.
 */

import type { CallToolRequestParams, Tool } from "./spec.types.js";

/**
 * Extension identifier constant.
 */
export const TOOLBOX_EXTENSION_ID = "com.google.cloud/toolbox.v1";

/**
 * Tool definition in `tools/list` results augmented with `secureInputSchema`.
 *
 * @category `secure_params`
 */
export interface ToolWithSecureParams extends Tool {
  /**
   * JSON Schema object defining sensitive runtime parameters hidden from the LLM agent
   * and passed out-of-band by the calling application.
   */
  secureInputSchema?: { $schema?: string; type: "object"; [key: string]: unknown };
}

/**
 * Augmented request parameters for `tools/call`.
 *
 * @category `secure_params`
 */
export interface CallToolRequestParamsWithSecureParams extends CallToolRequestParams {
  /**
   * Standard parameters passed by the LLM agent or caller.
   * Secure parameters MUST NOT be included in this object.
   */
  arguments?: { [key: string]: unknown };

  /**
   * Secure parameters passed out-of-band by the client application.
   * Non-secure parameters MUST NOT be included in this object.
   */
  secureArguments?: { [key: string]: unknown };
}
