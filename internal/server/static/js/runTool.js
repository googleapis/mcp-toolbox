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

import { isParamIncluded } from "./toolDisplay.js";
import { createMcpHeaders, createMcpRequestBody } from "./mcpClient.js";

/**
 * Runs a specific tool using the /api/tools/toolName/invoke endpoint
 * @param {string} toolId The unique identifier for the tool.
 * @param {!HTMLFormElement} form The form element containing parameter inputs.
 * @param {!HTMLTextAreaElement} responseArea The textarea to display results or errors.
 * @param {!Array<!Object>} parameters An array of parameter definition objects
 * @param {!HTMLInputElement} prettifyCheckbox The checkbox to control JSON formatting.
 * @param {function(?Object): void} updateLastResults Callback to store the last results.
 */
export async function handleRunTool(toolId, form, responseArea, parameters, prettifyCheckbox, updateLastResults, headers, uiMeta) {
    const formData = new FormData(form);
    const typedParams = {};
    responseArea.value = 'Running tool...';
    updateLastResults(null);

    const isAppTool = Boolean(uiMeta && uiMeta.resourceUri);
    const iframe = isAppTool ? document.getElementById(`mcp-app-iframe-${toolId}`) : null;
    const statusElement = isAppTool ? document.querySelector(`#mcp-app-container-${toolId} .mcp-app-status`) : null;

    if (statusElement) {
        statusElement.textContent = 'Running tool...';
    }

    for (const param of parameters) {
        const NAME = param.name;
        const VALUE_TYPE = param.valueType;
        const RAW_VALUE = formData.get(NAME);
        const INCLUDE_CHECKED = isParamIncluded(toolId, NAME)

        try {
            if (!INCLUDE_CHECKED) {
                console.debug(`Param ${NAME} was intentionally skipped.`)
                // if param was purposely unchecked, don't include it in body
                continue;
            }

            if (VALUE_TYPE === 'boolean') {
                typedParams[NAME] = RAW_VALUE !== null;
                console.debug(`Parameter ${NAME} (boolean) set to: ${typedParams[NAME]}`);
                continue; 
            }

            // process remaining types
            if (VALUE_TYPE && (VALUE_TYPE.startsWith('array<') || VALUE_TYPE === 'array')) {
                typedParams[NAME] = parseArrayParameter(RAW_VALUE, VALUE_TYPE, NAME);
            } else if (VALUE_TYPE === 'object') {
                if (!RAW_VALUE || RAW_VALUE.trim() === '') {
                    typedParams[NAME] = {};
                } else {
                    try {
                        typedParams[NAME] = JSON.parse(RAW_VALUE.trim());
                    } catch (e) {
                        throw new Error(`Invalid JSON format for object parameter ${NAME}: ${e.message}`);
                    }
                }
            } else {
                switch (VALUE_TYPE) {
                    case 'number':
                    case 'integer':
                        if (RAW_VALUE === "") {
                            console.debug(`Param ${NAME} was empty, setting to empty string.`)
                            typedParams[NAME] = "";
                        } else {
                            const num = Number(RAW_VALUE);
                            if (isNaN(num)) {
                                throw new Error(`Invalid number input for ${NAME}: ${RAW_VALUE}`);
                            }
                            typedParams[NAME] = num;
                        }
                        break;
                    case 'string':
                    default:
                        typedParams[NAME] = RAW_VALUE;
                        break;
                }
            }
        } catch (error) {
            console.error('Error processing parameter:', NAME, error);
            responseArea.value = `Error for ${NAME}: ${error.message}`;
            if (statusElement) statusElement.textContent = 'Error';
            return; 
        }
    }

    console.debug('Running tool:', toolId, 'with typed params:', typedParams);
    try {
        const body = createMcpRequestBody("tools/call", {
            name: toolId,
            arguments: typedParams
        }, "2");

        const mcpHeaders = createMcpHeaders("tools/call", toolId, headers);

        const response = await fetch(`/mcp`, {
            method: 'POST',
            headers: mcpHeaders,
            body: JSON.stringify(body)
        });

        if (!response.ok) {
            const errorBody = await response.text();
            throw new Error(`HTTP error ${response.status}: ${errorBody}`);
        }
        
        const results = await response.json();
        updateLastResults(results);
        displayResults(results, responseArea, prettifyCheckbox.checked);

        // A tool can fail either at the JSON-RPC transport level (results.error)
        // or at the tool level (result.isError with the reason in content).
        const isError = !!results.error || results.result?.isError === true;

        if (statusElement) {
            statusElement.textContent = isError ? 'Tool Error' : 'App Active';
        }

        // Send MCP App postMessage to iframe if this is an App tool
        if (iframe && iframe.contentWindow) {
            let parsedData = null;
            try {
                if (results.result && Array.isArray(results.result.content)) {
                    const textItems = results.result.content.filter(c => c.type === 'text');
                    if (textItems.length === 1) {
                        parsedData = JSON.parse(textItems[0].text);
                    } else if (textItems.length > 1) {
                        try {
                            parsedData = textItems.map(c => JSON.parse(c.text));
                        } catch {
                            parsedData = textItems.map(c => c.text);
                        }
                    }
                } else if (results.result) {
                    parsedData = results.result;
                }
            } catch (e) {
                parsedData = results.result;
            }

            let structuredContent = results.result?.structuredContent;
            if (!structuredContent) {
                if (parsedData !== null && typeof parsedData === 'object') {
                    if (Array.isArray(parsedData)) {
                        structuredContent = { data: parsedData };
                    } else {
                        structuredContent = { ...parsedData };
                    }
                } else if (parsedData !== null) {
                    structuredContent = { result: parsedData };
                } else {
                    structuredContent = {};
                }
            }

            // If tabular or visualization data is present, attach queryData and visualizationData for visualization components
            const rawData = structuredContent.data || (Array.isArray(parsedData) ? parsedData : []);
            if (!structuredContent.visualizationData && (rawData.length > 0 || structuredContent.vis_config || structuredContent.fields)) {
                let visConfig = { type: 'table' };
                if (structuredContent.vis_config) {
                    try {
                        visConfig = typeof structuredContent.vis_config === 'string' ? JSON.parse(structuredContent.vis_config) : structuredContent.vis_config;
                    } catch (e) {
                        visConfig = { type: structuredContent.vis_config };
                    }
                }

                structuredContent.visualizationData = {
                    queryResult: {
                        data: rawData,
                        fields: structuredContent.fields,
                        pivots: structuredContent.pivots || [],
                        totals_data: structuredContent.totals_data
                    },
                    visConfig: visConfig,
                    title: visConfig?.title || structuredContent.title || (structuredContent.explore ? `${structuredContent.explore} query` : 'Visualization'),
                    query: {
                        id: structuredContent.query_id || structuredContent.qid || typedParams?.query_id,
                        model: structuredContent.model || typedParams?.model,
                        view: structuredContent.explore || structuredContent.view || typedParams?.explore,
                        fields: structuredContent.fields || typedParams?.fields,
                        filters: structuredContent.filters || typedParams?.filters,
                        pivots: structuredContent.pivots || typedParams?.pivots,
                        sorts: structuredContent.sorts || typedParams?.sorts,
                        limit: structuredContent.limit || typedParams?.limit
                    }
                };
            }
            if (rawData.length > 0 && !structuredContent.queryData) {
                structuredContent.queryData = rawData;
            }

            const toolResultNotification = {
                jsonrpc: "2.0",
                method: "ui/notifications/tool-result",
                params: {
                    content: results.result?.content || [
                        { type: "text", text: results.error ? (results.error.message || JSON.stringify(results.error)) : (typeof results.result === 'string' ? results.result : JSON.stringify(results.result || {})) }
                    ],
                    isError: isError,
                    // Don't hand back error text as if it were renderable data.
                    structuredContent: isError ? undefined : structuredContent
                }
            };

            console.debug("Sending MCP App notification to iframe:", toolResultNotification);
            iframe.contentWindow.postMessage(toolResultNotification, '*');
        }
    } catch (error) {
        console.error('Error running tool:', error);
        responseArea.value = `Error: ${error.message}`;
        if (statusElement) statusElement.textContent = 'Error';
        updateLastResults(null);
    }
}

/**
 * Parses and validates a single array parameter from a raw string value.
 * @param {string} rawValue The raw string value from FormData.
 * @param {string} valueType The full array type string (e.g., "array<number>").
 * @param {string} paramName The name of the parameter for error messaging.
 * @return {!Array<*>} The parsed array.
 * @throws {Error} If parsing or type validation fails.
 */
function parseArrayParameter(rawValue, valueType, paramName) {
    if (!rawValue || typeof rawValue !== 'string' || rawValue.trim() === '') {
        return [];
    }

    const trimmed = rawValue.trim();
    const ELEMENT_TYPE = valueType.startsWith('array<')
        ? valueType.substring(6, valueType.length - 1)
        : 'string';

    let parsedArray;
    if (trimmed.startsWith('[')) {
        try {
            parsedArray = JSON.parse(trimmed);
        } catch (e) {
            throw new Error(`Invalid JSON format for ${paramName}. Expected a JSON array (e.g. ["a", "b"]): ${e.message}`);
        }
    } else {
        // Support comma-separated strings for convenience in the Playground UI
        parsedArray = trimmed.split(',').map(s => s.trim().replace(/^["']|["']$/g, '')).filter(s => s.length > 0);
    }

    if (!Array.isArray(parsedArray)) {
        throw new Error(`Input for ${paramName} must be an array (e.g., ["a", "b"] or a, b).`);
    }

    return parsedArray.map((item, index) => {
        switch (ELEMENT_TYPE) {
            case 'number': {
                const NUM = Number(item);
                if (item === '' || item === null || isNaN(NUM)) {
                    throw new Error(`Invalid number "${item}" found in array for ${paramName} at index ${index}.`);
                }
                return NUM;
            }
            case 'integer': {
                const INT = Number(item);
                if (item === '' || item === null || isNaN(INT)) {
                    throw new Error(`Invalid integer "${item}" found in array for ${paramName} at index ${index}.`);
                }
                if (!Number.isInteger(INT)) {
                    throw new Error(`Value "${item}" in array for ${paramName} at index ${index} must be an integer.`);
                }
                return INT;
            }
            case 'boolean':
                return item === true || String(item).toLowerCase() === 'true';
            case 'string':
                return String(item);
            default:
                return item;
        }
    });
}

/**
 * Displays the results from the tool run in the response area.
 */
export function displayResults(results, responseArea, prettify) {
    if (results === null || results === undefined) {
        return;
    }

    if (results.error) {
        responseArea.value = `MCP Error ${results.error.code}: ${results.error.message}\n${JSON.stringify(results.error.data, null, 2) || ''}`;
        return;
    }

    try {
        let textContent = "";
        if (results.result && Array.isArray(results.result.content)) {
            textContent = results.result.content
                .filter(c => c.type === 'text' && typeof c.text === 'string')
                .map(c => c.text)
                .join('\n');
        } else if (results.result && typeof results.result.content === 'string') {
            textContent = results.result.content;
        } else {
            textContent = JSON.stringify(results.result, null, 2);
        }

        try {
            const resultJson = JSON.parse(textContent);
            responseArea.value = prettify ? JSON.stringify(resultJson, null, 2) : JSON.stringify(resultJson);
        } catch (e) {
            // Not pure JSON string, output as-is
            responseArea.value = textContent;
        }
    } catch (error) {
        console.error("Error parsing or stringifying results:", error);
        responseArea.value = typeof results === 'object' ? JSON.stringify(results, null, 2) : String(results);
    }
}
