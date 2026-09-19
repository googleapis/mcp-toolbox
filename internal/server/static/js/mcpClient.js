// Copyright 2025 Google LLC
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

export const MCP_PROTOCOL_VERSION = '2026-07-28';

export const MCP_CLIENT_INFO = {
    name: 'mcp-toolbox-playground',
    version: '1.0.0'
};

export const MCP_CLIENT_CAPABILITIES = {
    extensions: {
        'io.modelcontextprotocol/ui': {
            mimeTypes: ['text/html;profile=mcp-app']
        }
    }
};

export const MCP_META = {
    'io.modelcontextprotocol/protocolVersion': MCP_PROTOCOL_VERSION,
    'io.modelcontextprotocol/clientInfo': MCP_CLIENT_INFO,
    'io.modelcontextprotocol/clientCapabilities': MCP_CLIENT_CAPABILITIES
};

/**
 * Creates standardized headers for MCP HTTP requests.
 * @param {string} method The MCP method (e.g., 'tools/list', 'tools/call', 'resources/read')
 * @param {string} [name] The tool name (required for 'tools/call')
 * @param {Object} [customHeaders] Additional custom headers
 * @returns {Object} Headers object
 */
export function createMcpHeaders(method, name = '', customHeaders = {}) {
    const headers = {
        ...customHeaders,
        'Content-Type': 'application/json',
        'MCP-Protocol-Version': MCP_PROTOCOL_VERSION,
        'Mcp-Method': method
    };
    if (name) {
        headers['Mcp-Name'] = name;
    }
    return headers;
}

/**
 * Creates standardized JSON-RPC request body with _meta for MCP v20260728.
 * @param {string} method The MCP method
 * @param {Object} [params] Parameters for the method
 * @param {string|number} [id] Request ID
 * @returns {Object} Request body object
 */
export function createMcpRequestBody(method, params = {}, id = '1') {
    return {
        jsonrpc: '2.0',
        id: String(id),
        method: method,
        params: {
            ...params,
            _meta: MCP_META
        }
    };
}
