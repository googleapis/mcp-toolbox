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

import { renderToolInterface } from "./toolDisplay.js";
import { escapeHtml } from "./sanitize.js";
import { createMcpHeaders, createMcpRequestBody } from "./mcpClient.js";

let currentToolsList = [];

/**
 * Fetches a toolset from the /mcp endpoint and initiates creating the tool list.
 * @param {!HTMLElement} secondNavContent The HTML element where the tool list will be rendered.
 * @param {!HTMLElement} toolDisplayArea The HTML element where the details of a selected tool will be displayed.
 * @param {string} toolsetName The name of the toolset to load (empty string loads all tools).
 * @returns {!Promise<void>} A promise that resolves when the tools are loaded and rendered, or rejects on error.
 */
export async function loadTools(secondNavContent, toolDisplayArea, toolsetName) {
    secondNavContent.innerHTML = '<p>Fetching tools...</p>';
    try {
        const url = toolsetName ? `/mcp/${toolsetName}` : `/mcp`;
        const response = await fetch(url, {
            method: 'POST',
            headers: createMcpHeaders('tools/list'),
            body: JSON.stringify(createMcpRequestBody('tools/list', {}, '1'))
        });
        
        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }
        
        const apiResponse = await response.json();
        renderToolList(apiResponse, secondNavContent, toolDisplayArea);
    } catch (error) {
        console.error('Failed to load tools:', error);
        secondNavContent.innerHTML = `<p class="error">Failed to load tools: <pre><code>${escapeHtml(String(error))}</code></pre></p>`;
    }
}

/**
 * Renders the list of tools as buttons within the provided HTML element.
 * @param {Object} apiResponse The API response object containing the tools.
 * @param {!HTMLElement} secondNavContent The HTML element to render the tool list into.
 * @param {!HTMLElement} toolDisplayArea The HTML element for displaying tool details (passed to event handlers).
 */
function renderToolList(apiResponse, secondNavContent, toolDisplayArea) {
    secondNavContent.innerHTML = '';

    if (apiResponse && apiResponse.error) {
        console.error('MCP API Error:', apiResponse.error);
        secondNavContent.textContent = `Error: ${apiResponse.error.message || 'Unknown MCP error'}`;
        return;
    }

    if (!apiResponse || !apiResponse.result || !Array.isArray(apiResponse.result.tools)) {
        console.error('Error: Expected a valid MCP response with "result.tools" array, but received:', apiResponse);
        secondNavContent.textContent = 'Error: Invalid response format from toolset API.';
        return;
    }

    currentToolsList = apiResponse.result.tools;

    if (currentToolsList.length === 0) {
        secondNavContent.textContent = 'No tools found.';
        return;
    }

    const ul = document.createElement('ul');
    currentToolsList.forEach(toolObj => {
        const li = document.createElement('li');
        const button = document.createElement('button');
        button.textContent = toolObj.name;
        button.dataset.toolname = toolObj.name;
        button.classList.add('tool-button');
        button.addEventListener('click', (event) => handleToolClick(event, secondNavContent, toolDisplayArea));
        li.appendChild(button);
        ul.appendChild(li);
    });
    secondNavContent.appendChild(ul);
}

/**
 * Handles the click event on a tool button. 
 * @param {!Event} event The click event object.
 * @param {!HTMLElement} secondNavContent The parent element containing the tool buttons.
 * @param {!HTMLElement} toolDisplayArea The HTML element where tool details will be shown.
 */
function handleToolClick(event, secondNavContent, toolDisplayArea) {
    const toolName = event.target.dataset.toolname;
    if (toolName) {
        const currentActive = secondNavContent.querySelector('.tool-button.active');
        if (currentActive) {
            currentActive.classList.remove('active');
        }
        event.target.classList.add('active');
        renderToolDetails(toolName, toolDisplayArea);
    }
}

/**
 * Renders details for a specific tool from the cached MCP tools list.
 * @param {string} toolName The name of the tool to render details for.
 * @param {!HTMLElement} toolDisplayArea The HTML element to display the tool interface in.
 */
function renderToolDetails(toolName, toolDisplayArea) {
    const toolObject = currentToolsList.find(t => t.name === toolName);

    if (!toolObject) {
        toolDisplayArea.innerHTML = `<p class="error">Tool "${escapeHtml(toolName)}" data not found.</p>`;
        return;
    }

    console.debug("Rendering tool object: ", toolObject);

    let toolAuthRequired = [];
    let toolAuthParams = {};
    let toolUIMeta = null;
    if (toolObject._meta) {
        if (toolObject._meta["toolbox/authInvoke"]) {
            toolAuthRequired = toolObject._meta["toolbox/authInvoke"];
        }
        if (toolObject._meta["toolbox/authParam"]) {
            toolAuthParams = toolObject._meta["toolbox/authParam"];
        }
        if (toolObject._meta.ui && toolObject._meta.ui.resourceUri) {
            toolUIMeta = toolObject._meta.ui;
        } else if (toolObject._meta["ui/resourceUri"]) {
            toolUIMeta = { resourceUri: toolObject._meta["ui/resourceUri"] };
        }
    }

    // Default processing if inputSchema properties are not present
    let toolParameters = [];
    if (toolObject.inputSchema && toolObject.inputSchema.properties) {
        const props = toolObject.inputSchema.properties;
        const requiredFields = toolObject.inputSchema.required || [];

        toolParameters = Object.keys(props).map(paramName => {
            const param = props[paramName];
            let inputType = 'text'; 
            const apiType = param.type ? param.type.toLowerCase() : 'string';
            let valueType = 'string'; 
            let label = param.description || paramName;

            if (apiType === 'integer' || apiType === 'number') {
                inputType = 'number';
                valueType = 'number';
            } else if (apiType === 'boolean') {
                inputType = 'checkbox';
                valueType = 'boolean';
            } else if (apiType === 'array') {
                inputType = 'textarea'; 
                const itemType = param.items && param.items.type ? param.items.type.toLowerCase() : 'string';
                valueType = `array<${itemType}>`;
                label += ' (Array)';
            }

            return {
                name: paramName,
                type: inputType,    
                valueType: valueType, 
                label: label,
                required: requiredFields.includes(paramName),
                authServices: toolAuthParams[paramName] || []
            };
        });
    }

    const toolInterfaceData = {
        id: toolName,
        name: toolName,
        description: toolObject.description || "No description provided.",
        authRequired: toolAuthRequired,
        parameters: toolParameters,
        ui: toolUIMeta
    };

    console.debug("Transformed toolInterfaceData:", toolInterfaceData);
    renderToolInterface(toolInterfaceData, toolDisplayArea);
}