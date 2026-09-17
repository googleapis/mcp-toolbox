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

import { handleRunTool, displayResults } from './runTool.js';
import { createGoogleAuthMethodItem } from './auth.js'
import { escapeHtml } from './sanitize.js'
import { createMcpHeaders, createMcpRequestBody, MCP_PROTOCOL_VERSION } from './mcpClient.js';

let activeTool = null;
const toolHeadersMap = new Map();
let activeHeaders = {
    "Content-Type": "application/json"
};

/**
 * Helper function to create form inputs for parameters.
 */
function createParamInput(param, toolId) {
    const paramItem = document.createElement('div');
    paramItem.className = 'param-item';

    const label = document.createElement('label');
    const INPUT_ID = `param-${toolId}-${param.name}`;
    const NAME_TEXT = document.createTextNode(param.name);
    label.setAttribute('for', INPUT_ID);
    label.appendChild(NAME_TEXT);

    const IS_AUTH_PARAM = param.authServices && param.authServices.length > 0;
    let additionalLabelText = '';
    if (IS_AUTH_PARAM) {
        additionalLabelText += ' (auth)';
    }
    if (!param.required) {
        additionalLabelText += ' (optional)';
    }

    if (additionalLabelText) {
        const additionalSpan = document.createElement('span');
        additionalSpan.textContent = additionalLabelText;
        additionalSpan.classList.add('param-label-extras');
        label.appendChild(additionalSpan);
    }
    paramItem.appendChild(label);

    const inputCheckboxWrapper = document.createElement('div');
    const inputContainer = document.createElement('div');
    inputCheckboxWrapper.className = 'input-checkbox-wrapper';
    inputContainer.className = 'param-input-element-container';

    // Build parameter's value input box.
    const PLACEHOLDER_LABEL = param.label;
    let inputElement;
    let boolValueLabel = null;

    if (param.type === 'textarea') {
        inputElement = document.createElement('textarea');
        inputElement.rows = 3;
        inputContainer.appendChild(inputElement);
    } else if(param.type === 'checkbox') {
        inputElement = document.createElement('input');
        inputElement.type = 'checkbox';
        inputElement.title = PLACEHOLDER_LABEL;
        inputElement.checked = false;

        // handle true/false label for boolean params
        boolValueLabel = document.createElement('span');
        boolValueLabel.className = 'checkbox-bool-label';
        boolValueLabel.textContent = inputElement.checked ? ' true' : ' false';

        inputContainer.appendChild(inputElement); 
        inputContainer.appendChild(boolValueLabel); 

        inputElement.addEventListener('change', () => {
            boolValueLabel.textContent = inputElement.checked ? ' true' : ' false';
        });
    } else {
        inputElement = document.createElement('input');
        inputElement.type = param.type;
        inputContainer.appendChild(inputElement);
    }

    inputElement.id = INPUT_ID;
    inputElement.name = param.name;
    inputElement.classList.add('param-input-element');

    if (IS_AUTH_PARAM) {
        inputElement.disabled = true;
        inputElement.classList.add('auth-param-input');
        if (param.type !== 'checkbox') {
            inputElement.placeholder = param.authServices;
        }
    } else if (param.type !== 'checkbox') {
        inputElement.placeholder = PLACEHOLDER_LABEL ? PLACEHOLDER_LABEL.trim() : '';
    }
    inputCheckboxWrapper.appendChild(inputContainer);

    // create the "Include Param" checkbox
    const INCLUDE_CHECKBOX_ID = `include-${INPUT_ID}`;
    const includeContainer = document.createElement('div');
    const includeCheckbox = document.createElement('input');

    includeContainer.className = 'include-param-container';
    includeCheckbox.type = 'checkbox';
    includeCheckbox.id = INCLUDE_CHECKBOX_ID;
    includeCheckbox.name = `include-${param.name}`;
    includeCheckbox.title = 'Include this parameter'; // Add a tooltip

    // default to checked, unless it's an optional parameter
    includeCheckbox.checked = param.required;

    includeContainer.appendChild(includeCheckbox);
    inputCheckboxWrapper.appendChild(includeContainer);

    paramItem.appendChild(inputCheckboxWrapper);

    // function to update UI based on checkbox state
    const updateParamIncludedState = () => {
        const isIncluded = includeCheckbox.checked;
        if (isIncluded) {
            paramItem.classList.remove('disabled-param');
            if (!IS_AUTH_PARAM) {
                 inputElement.disabled = false;
            }
            if (boolValueLabel) {
                boolValueLabel.classList.remove('disabled');
            }
        } else {
            paramItem.classList.add('disabled-param');
            inputElement.disabled = true;
            if (boolValueLabel) {
                boolValueLabel.classList.add('disabled');
            }
        }
    };

    // add event listener to the include checkbox
    includeCheckbox.addEventListener('change', updateParamIncludedState);
    updateParamIncludedState(); 

    return paramItem;
}

/**
 * Function to create the header editor popup modal.
 * @param {string} toolId The unique identifier for the tool.
 * @param {!Object<string, string>} currentHeaders The current headers.
 * @param {function(!Object<string, string>): void} saveCallback A function to be
 *     called when the "Save" button is clicked and the headers are successfully
 *     parsed. The function receives the updated headers object as its argument.
 * @return {!HTMLDivElement} The outermost div element of the created modal.
 */
function createHeaderEditorModal(toolId, currentHeaders, toolParameters, authRequired, saveCallback) {
    const MODAL_ID = `header-modal-${toolId}`;
    let modal = document.getElementById(MODAL_ID);

    if (modal) {
        modal.remove(); 
    }

    modal = document.createElement('div');
    modal.id = MODAL_ID;
    modal.className = 'header-modal';

    const modalContent = document.createElement('div');
    const modalHeader = document.createElement('h5');
    const headersTextarea = document.createElement('textarea');

    modalContent.className = 'header-modal-content';
    modalHeader.textContent = 'Edit Request Headers';
    headersTextarea.id = `headers-textarea-${toolId}`;
    headersTextarea.className = 'headers-textarea';
    headersTextarea.rows = 10;
    headersTextarea.value = JSON.stringify(currentHeaders, null, 2);

    // handle authenticated params
    const authProfileNames = new Set();
    toolParameters.forEach(param => {
        const isAuthParam = param.authServices && param.authServices.length > 0;
        if (isAuthParam && param.authServices) {
             param.authServices.forEach(name => authProfileNames.add(name));
        }
    });

    // handle authorized invocations
    if (authRequired && authRequired.length > 0) {
        authRequired.forEach(name => authProfileNames.add(name));
    }

    modalContent.appendChild(modalHeader);
    modalContent.appendChild(headersTextarea);

    if (authProfileNames.size > 0 || authRequired.length > 0) {
        const authHelperSection = document.createElement('div');
        authHelperSection.className = 'auth-helper-section';
        const authList = document.createElement('div');
        authList.className = 'auth-method-list';

        authProfileNames.forEach(profileName => {
            const authItem = createGoogleAuthMethodItem(toolId, profileName);
            authList.appendChild(authItem);
        });
        authHelperSection.appendChild(authList);
        modalContent.appendChild(authHelperSection);
    }

    const modalActions = document.createElement('div');
    const closeButton = document.createElement('button');
    const saveButton = document.createElement('button');
    const authTokenDropdown = createAuthTokenInfoDropdown();

    modalActions.className = 'header-modal-actions';
    closeButton.textContent = 'Close';
    closeButton.className = 'btn btn--closeHeaders';
    closeButton.addEventListener('click', () => closeHeaderEditor(toolId));
    saveButton.textContent = 'Save';
    saveButton.className = 'btn btn--saveHeaders';
    saveButton.addEventListener('click', () => {
        try {
            const updatedHeaders = JSON.parse(headersTextarea.value);
            saveCallback(updatedHeaders);
            closeHeaderEditor(toolId);
        } catch (e) {
            alert('Invalid JSON format for headers.');
            console.error("Header JSON parse error:", e);
        }
    });

    modalActions.appendChild(closeButton);
    modalActions.appendChild(saveButton);
    modalContent.appendChild(modalActions);
    modalContent.appendChild(authTokenDropdown);
    modal.appendChild(modalContent);

    return modal;
}

/**
 * Function to open the header popup.
 */
function openHeaderEditor(toolId) {
    const modal = document.getElementById(`header-modal-${toolId}`);
    if (modal) {
        modal.style.display = 'block';
    }
}

/**
 * Function to close the header popup.
 */
function closeHeaderEditor(toolId) {
    const modal = document.getElementById(`header-modal-${toolId}`);
    if (modal) {
        modal.style.display = 'none';
    }
}

/**
 * Creates a dropdown element showing information on how to extract Google auth tokens.
 * @return {HTMLDetailsElement} The details element representing the dropdown.
 */
function createAuthTokenInfoDropdown() {
    const details = document.createElement('details');
    const summary = document.createElement('summary');
    const content = document.createElement('div');

    details.className = 'auth-token-details';
    details.appendChild(summary);
    summary.textContent = 'How to extract Google OAuth ID Token manually';
    content.className = 'auth-token-content';

    // auth instruction dropdown
    const tabButtons = document.createElement('div');
    const leftTab = document.createElement('button');
    const rightTab = document.createElement('button');
    
    tabButtons.className = 'auth-tab-group';
    leftTab.className = 'auth-tab-picker active';
    leftTab.textContent = 'With Standard Account';
    leftTab.setAttribute('data-tab', 'standard');
    rightTab.className = 'auth-tab-picker';
    rightTab.textContent = 'With Service Account';
    rightTab.setAttribute('data-tab', 'service');

    tabButtons.appendChild(leftTab);
    tabButtons.appendChild(rightTab);
    content.appendChild(tabButtons);

    const tabContentContainer = document.createElement('div');
    const standardAccInstructions = document.createElement('div');
    const serviceAccInstructions = document.createElement('div');

    standardAccInstructions.id = 'auth-tab-standard';
    standardAccInstructions.className = 'auth-tab-content active'; 
    standardAccInstructions.innerHTML = AUTH_TOKEN_INSTRUCTIONS_STANDARD;
    serviceAccInstructions.id = 'auth-tab-service';
    serviceAccInstructions.className = 'auth-tab-content';
    serviceAccInstructions.innerHTML = AUTH_TOKEN_INSTRUCTIONS_SERVICE_ACCOUNT;

    tabContentContainer.appendChild(standardAccInstructions);
    tabContentContainer.appendChild(serviceAccInstructions);
    content.appendChild(tabContentContainer);

    // switching tabs logic
    const tabBtns = [leftTab, rightTab];
    const tabContents = [standardAccInstructions, serviceAccInstructions];

    tabBtns.forEach(btn => {
        btn.addEventListener('click', () => {
            // deactivate all buttons and contents
            tabBtns.forEach(b => b.classList.remove('active'));
            tabContents.forEach(c => c.classList.remove('active'));

            btn.classList.add('active');

            const tabId = btn.getAttribute('data-tab');
            const activeContent = content.querySelector(`#auth-tab-${tabId}`);
            if (activeContent) {
                activeContent.classList.add('active');
            }
        });
    });

    details.appendChild(content);
    return details;
}

/**
 * Renders the tool display area.
 */
export function renderToolInterface(tool, containerElement) {
    activeTool = tool;
    const TOOL_ID = tool.id;
    containerElement.innerHTML = '';

    let lastResults = null;
    let currentHeaders = {
        "Content-Type": "application/json"
    };
    activeHeaders = currentHeaders;
    toolHeadersMap.set(TOOL_ID, currentHeaders);

    // function to update lastResults so we can toggle json
    const updateLastResults = (newResults) => {
        lastResults = newResults;
    };
    const updateCurrentHeaders = (newHeaders) => {
        currentHeaders = newHeaders;
        activeHeaders = newHeaders;
        toolHeadersMap.set(TOOL_ID, newHeaders);
        const newModal = createHeaderEditorModal(TOOL_ID, currentHeaders, tool.parameters, tool.authRequired, updateCurrentHeaders);
        containerElement.appendChild(newModal);
    };

    const gridContainer = document.createElement('div');
    gridContainer.className = 'tool-details-grid';

    const toolInfoContainer = document.createElement('div');
    const nameBox = document.createElement('div');
    const descBox = document.createElement('div');

    const isAppTool = Boolean(tool.ui && tool.ui.resourceUri);
    nameBox.className = 'tool-box tool-name';
    nameBox.innerHTML = `<h5>Name:</h5><div class="tool-name-container"><p>${escapeHtml(tool.name)}</p>${isAppTool ? `<span class="mcp-app-badge" title="MCP App (${escapeHtml(tool.ui.resourceUri)})">MCP App (UI)</span>` : ''}</div>`;
    descBox.className = 'tool-box tool-description';
    descBox.innerHTML = `<h5>Description:</h5><p>${escapeHtml(tool.description)}</p>`;

    toolInfoContainer.className = 'tool-info';
    toolInfoContainer.appendChild(nameBox);
    toolInfoContainer.appendChild(descBox);
    gridContainer.appendChild(toolInfoContainer);

    const DISLCAIMER_INFO = "*Checked parameters are sent with the value from their text field. Empty fields will be sent as an empty string. To exclude a parameter, uncheck it."
    const paramsContainer = document.createElement('div');
    const form = document.createElement('form');
    const paramsHeader = document.createElement('div');
    const disclaimerText = document.createElement('div');

    paramsContainer.className = 'tool-params tool-box';
    paramsContainer.innerHTML = '<h5>Parameters:</h5>';
    paramsHeader.className = 'params-header';
    paramsContainer.appendChild(paramsHeader);
    disclaimerText.textContent = DISLCAIMER_INFO;
    disclaimerText.className = 'params-disclaimer'; 
    paramsContainer.appendChild(disclaimerText);

    form.id = `tool-params-form-${TOOL_ID}`;

    tool.parameters.forEach(param => {
        form.appendChild(createParamInput(param, TOOL_ID));
    });
    paramsContainer.appendChild(form);
    gridContainer.appendChild(paramsContainer);

    containerElement.appendChild(gridContainer);

    const RESPONSE_AREA_ID = `tool-response-area-${TOOL_ID}`;
    const runButtonContainer = document.createElement('div');
    const editHeadersButton = document.createElement('button');
    const runButton = document.createElement('button');

    editHeadersButton.className = 'btn btn--editHeaders';
    editHeadersButton.textContent = 'Edit Headers';
    editHeadersButton.addEventListener('click', () => openHeaderEditor(TOOL_ID));
    runButtonContainer.className = 'run-button-container';
    runButtonContainer.appendChild(editHeadersButton);

    runButton.className = 'btn btn--run';
    runButton.textContent = 'Run Tool';
    runButtonContainer.appendChild(runButton);
    containerElement.appendChild(runButtonContainer);

    // response Area (bottom)
    const responseContainer = document.createElement('div');
    const responseHeaderControls = document.createElement('div');
    const responseHeader = document.createElement('h5');
    const responseArea = document.createElement('textarea');

    responseContainer.className = 'tool-response tool-box';
    responseHeaderControls.className = 'response-header-controls';
    responseHeader.textContent = 'Response:';
    responseHeaderControls.appendChild(responseHeader);

    // Tab switcher for MCP Apps
    let appContainer = null;
    let iframeElement = null;
    let appStatusElement = null;

    if (isAppTool) {
        const viewTabs = document.createElement('div');
        viewTabs.className = 'response-view-tabs';

        const tabApp = document.createElement('button');
        tabApp.className = 'response-view-tab active';
        tabApp.textContent = 'Visual App';

        const tabJson = document.createElement('button');
        tabJson.className = 'response-view-tab';
        tabJson.textContent = 'Raw JSON';

        viewTabs.appendChild(tabApp);
        viewTabs.appendChild(tabJson);
        responseHeaderControls.appendChild(viewTabs);

        appContainer = document.createElement('div');
        appContainer.className = 'mcp-app-container';
        appContainer.id = `mcp-app-container-${TOOL_ID}`;

        const appTopBar = document.createElement('div');
        appTopBar.className = 'mcp-app-topbar';

        const uriInfo = document.createElement('div');
        uriInfo.className = 'mcp-app-uri';
        uriInfo.innerHTML = `<span>Resource:</span> <code>${escapeHtml(tool.ui.resourceUri)}</code>`;

        appStatusElement = document.createElement('div');
        appStatusElement.className = 'mcp-app-status';
        appStatusElement.textContent = 'App Ready';

        const reloadBtn = document.createElement('button');
        reloadBtn.className = 'mcp-app-reload-btn';
        reloadBtn.innerHTML = '↻ Reload';
        reloadBtn.style.marginLeft = 'auto';
        reloadBtn.style.padding = '4px 10px';
        reloadBtn.style.fontSize = '12px';
        reloadBtn.style.cursor = 'pointer';
        reloadBtn.style.background = '#f1f3f4';
        reloadBtn.style.color = '#5f6368';
        reloadBtn.style.border = '1px solid #dadce0';
        reloadBtn.style.borderRadius = '4px';
        reloadBtn.style.fontWeight = '500';
        reloadBtn.addEventListener('click', () => {
            loadAppResource(tool.ui.resourceUri, iframeElement, appStatusElement, currentHeaders);
        });

        const exitFullscreenBtn = document.createElement('button');
        exitFullscreenBtn.className = 'mcp-app-exit-fullscreen-btn';
        exitFullscreenBtn.innerHTML = '⤓ Exit Fullscreen';
        exitFullscreenBtn.style.display = 'none';
        exitFullscreenBtn.style.marginLeft = '10px';
        exitFullscreenBtn.style.padding = '4px 10px';
        exitFullscreenBtn.style.fontSize = '12px';
        exitFullscreenBtn.style.cursor = 'pointer';
        exitFullscreenBtn.style.background = '#e8f0fe';
        exitFullscreenBtn.style.color = '#1a73e8';
        exitFullscreenBtn.style.border = '1px solid #1a73e8';
        exitFullscreenBtn.style.borderRadius = '4px';
        exitFullscreenBtn.style.fontWeight = '500';
        exitFullscreenBtn.addEventListener('click', () => {
            setAppDisplayMode(appContainer, iframeElement, 'inline');
            if (iframeElement && iframeElement.contentWindow) {
                iframeElement.contentWindow.postMessage({
                    jsonrpc: '2.0',
                    method: 'ui/notifications/host-context-changed',
                    params: { displayMode: 'inline' }
                }, '*');
            }
        });

        appTopBar.appendChild(uriInfo);
        appTopBar.appendChild(appStatusElement);
        appTopBar.appendChild(reloadBtn);
        appTopBar.appendChild(exitFullscreenBtn);
        appContainer.appendChild(appTopBar);

        iframeElement = document.createElement('iframe');
        iframeElement.id = `mcp-app-iframe-${TOOL_ID}`;
        iframeElement.className = 'mcp-app-iframe';
        iframeElement.title = `MCP App - ${tool.name}`;
        iframeElement.setAttribute('sandbox', 'allow-scripts allow-forms allow-popups allow-popups-to-escape-sandbox allow-storage-access-by-user-activation');
        iframeElement.setAttribute('allow', 'fullscreen; clipboard-read; clipboard-write');
        appContainer.appendChild(iframeElement);

        tabApp.addEventListener('click', (e) => {
            e.preventDefault();
            tabApp.classList.add('active');
            tabJson.classList.remove('active');
            appContainer.style.display = 'flex';
            appContainer.style.flexDirection = 'column';
            responseArea.style.display = 'none';
        });

        tabJson.addEventListener('click', (e) => {
            e.preventDefault();
            tabJson.classList.add('active');
            tabApp.classList.remove('active');
            appContainer.style.display = 'none';
            responseArea.style.display = 'block';
        });

        // Pre-fetch the resource HTML
        loadAppResource(tool.ui.resourceUri, iframeElement, appStatusElement, currentHeaders);
    }

    // prettify box
    const PRETTIFY_ID = `prettify-${TOOL_ID}`;
    const prettifyDiv = document.createElement('div');
    const prettifyLabel = document.createElement('label');
    const prettifyCheckbox = document.createElement('input');

    prettifyDiv.className = 'prettify-container';
    prettifyLabel.setAttribute('for', PRETTIFY_ID);
    prettifyLabel.textContent = 'Prettify JSON';
    prettifyLabel.className = 'prettify-label';

    prettifyCheckbox.type = 'checkbox';
    prettifyCheckbox.id = PRETTIFY_ID;
    prettifyCheckbox.checked = true;
    prettifyCheckbox.className = 'prettify-checkbox';

    prettifyDiv.appendChild(prettifyLabel);
    prettifyDiv.appendChild(prettifyCheckbox);

    responseHeaderControls.appendChild(prettifyDiv);
    responseContainer.appendChild(responseHeaderControls);

    if (appContainer) {
        responseContainer.appendChild(appContainer);
    }

    responseArea.id = RESPONSE_AREA_ID;
    responseArea.readOnly = true;
    responseArea.placeholder = 'Results will appear here...';
    responseArea.className = 'tool-response-area';
    responseArea.rows = 10;
    if (isAppTool) {
        responseArea.style.display = 'none'; // Default to visual app view for App tools
    }
    responseContainer.appendChild(responseArea);

    containerElement.appendChild(responseContainer);

    // create and append the header editor modal
    const headerModal = createHeaderEditorModal(TOOL_ID, currentHeaders, tool.parameters, tool.authRequired, updateCurrentHeaders);
    containerElement.appendChild(headerModal);

    prettifyCheckbox.addEventListener('change', () => {
        if (lastResults) {
            displayResults(lastResults, responseArea, prettifyCheckbox.checked);
        }
    });

    runButton.addEventListener('click', (event) => {
        event.preventDefault();
        handleRunTool(TOOL_ID, form, responseArea, tool.parameters, prettifyCheckbox, updateLastResults, currentHeaders, tool.ui);
    });
}

/**
 * Helper function to transition MCP App container between inline and fullscreen.
 */
function setAppDisplayMode(mcpContainer, iframeElement, mode) {
    const container = mcpContainer || iframeElement?.closest('.mcp-app-container') || document.querySelector('.mcp-app-container');
    const iframe = iframeElement || container?.querySelector('.mcp-app-iframe') || document.querySelector('.mcp-app-iframe');
    const exitBtn = container?.querySelector('.mcp-app-exit-fullscreen-btn');

    if (mode === 'fullscreen') {
        if (container) {
            container.classList.add('mcp-app-fullscreen');
            container.style.cssText = 'position: fixed !important; top: 0 !important; left: 0 !important; right: 0 !important; bottom: 0 !important; width: 100vw !important; height: 100vh !important; max-width: 100vw !important; max-height: 100vh !important; z-index: 2147483647 !important; margin: 0 !important; padding: 0 !important; border: none !important; border-radius: 0 !important; display: flex !important; flex-direction: column !important; background-color: #ffffff !important; box-sizing: border-box !important;';
        }
        if (iframe) {
            iframe.style.cssText = 'width: 100% !important; height: 100% !important; flex: 1 1 100% !important; min-height: 0 !important; border: none !important; box-sizing: border-box !important;';
        }
        if (exitBtn) {
            exitBtn.style.display = 'inline-block';
        }
        document.body.classList.add('mcp-app-fullscreen-active');
    } else {
        if (container) {
            container.classList.remove('mcp-app-fullscreen');
            container.removeAttribute('style');
            container.style.display = 'flex';
            container.style.flexDirection = 'column';
        }
        if (iframe) {
            iframe.removeAttribute('style');
        }
        if (exitBtn) {
            exitBtn.style.display = 'none';
        }
        document.body.classList.remove('mcp-app-fullscreen-active');
    }

    if (iframe && iframe.contentWindow) {
        iframe.contentWindow.postMessage({
            jsonrpc: '2.0',
            method: 'ui/notifications/host-context-changed',
            params: { displayMode: mode }
        }, '*');
        iframe.contentWindow.postMessage({
            type: 'ui/host_context_changed',
            displayMode: mode
        }, '*');
    }
}

/**
 * Constructs a standard Content Security Policy string according to SEP-1865.
 */
function constructCsp(csp) {
    const connectList = new Set(csp?.connectDomains || []);
    const resourceList = new Set(csp?.resourceDomains || []);

    // In local development or testing environments, ensure localhost and host ports are allowed
    const isLocal = window.location.hostname === 'localhost' || 
                    window.location.hostname === '127.0.0.1' || 
                    window.location.hostname.endsWith('.googlers.com');
    if (isLocal) {
        ['http://localhost:9998', 'http://localhost:9999', 'http://localhost:19999', 'http://localhost:3035',
         'http://127.0.0.1:9998', 'http://127.0.0.1:9999', 'http://127.0.0.1:19999', 'http://127.0.0.1:3035'].forEach(d => {
            resourceList.add(d);
            connectList.add(d);
        });
        connectList.add('ws://localhost:*');
        connectList.add('ws://127.0.0.1:*');
        connectList.add('wss://localhost:*');
        connectList.add('wss://127.0.0.1:*');
    }

    const connect = Array.from(connectList).join(' ');
    const resource = Array.from(resourceList).join(' ');
    const frame = (csp?.frameDomains && csp.frameDomains.length > 0) ? csp.frameDomains.join(' ') : "'none'";
    const baseUri = (csp?.baseUriDomains && csp.baseUriDomains.length > 0) ? csp.baseUriDomains.join(' ') : "'self'";

    return `default-src 'none'; script-src 'self' 'unsafe-inline' 'unsafe-eval' ${resource}; style-src 'self' 'unsafe-inline' ${resource}; connect-src 'self' ${connect}; img-src 'self' data: ${resource}; font-src 'self' data: ${resource}; media-src 'self' data: ${resource}; frame-src ${frame}; object-src 'none'; base-uri ${baseUri};`.replace(/\s+/g, ' ').trim();
}

/**
 * Injects a Content-Security-Policy meta tag into the raw HTML.
 */
function injectCspIntoHtml(html, cspHeader) {
    // Only escape & and " so that single-quoted CSP keywords ('self', 'unsafe-inline') are preserved literally
    const safeCspHeader = cspHeader.replace(/&/g, '&amp;').replace(/"/g, '&quot;');
    const metaTag = `<meta http-equiv="Content-Security-Policy" content="${safeCspHeader}">`;
    
    const headMatch = html.match(/<head[^>]*>/i);
    if (headMatch) {
        const index = headMatch.index + headMatch[0].length;
        return html.slice(0, index) + '\n  ' + metaTag + html.slice(index);
    }
    const htmlMatch = html.match(/<html[^>]*>/i);
    if (htmlMatch) {
        const index = htmlMatch.index + htmlMatch[0].length;
        return html.slice(0, index) + '\n<head>' + metaTag + '</head>' + html.slice(index);
    }
    return '<head>' + metaTag + '</head>\n' + html;
}

/**
 * Loads the HTML content for an MCP App resource into an iframe.
 */
async function loadAppResource(uri, iframeElement, statusElement, headers) {
    if (!uri || !iframeElement) return;
    try {
        if (statusElement) statusElement.textContent = 'Loading resource...';
        const response = await fetch('/mcp', {
            method: 'POST',
            headers: createMcpHeaders('resources/read', uri, headers),
            body: JSON.stringify(createMcpRequestBody('resources/read', { uri: uri }, 'read-resource'))
        });
        if (!response.ok) {
            throw new Error(`HTTP error ${response.status}`);
        }
        const data = await response.json();
        if (data.error) {
            throw new Error(data.error.message || 'Unknown MCP error');
        }
        if (data.result && data.result.contents && data.result.contents.length > 0) {
            const resContent = data.result.contents[0];
            if (resContent.text) {
                const csp = resContent._meta?.ui?.csp || data.result?._meta?.ui?.csp;
                const cspHeader = constructCsp(csp);
                const finalHtml = injectCspIntoHtml(resContent.text, cspHeader);

                iframeElement.srcdoc = finalHtml;
                if (statusElement) {
                    const domains = (csp?.resourceDomains || []).concat(csp?.connectDomains || []);
                    if (domains.length > 0) {
                        statusElement.title = `Enforced CSP: ${domains.join(', ')}`;
                        statusElement.textContent = 'App Ready (CSP Enforced)';
                    } else {
                        statusElement.textContent = 'App Ready (Restricted CSP)';
                    }
                }
            } else {
                throw new Error('Resource content text is empty');
            }
        } else {
            throw new Error('No resource contents returned');
        }
    } catch (e) {
        console.error('Error fetching UI resource:', e);
        if (statusElement) statusElement.textContent = `Error loading resource: ${e.message}`;
    }
}

// MCP Apps Host Protocol Handshake handler
window.addEventListener('message', (event) => {
    const data = event.data;
    const sender = event.source;
    const senderOrigin = (event.origin && event.origin !== 'null') ? event.origin : '*';

    // Verify that the sender is one of our managed iframes to prevent unauthorized cross-document message attacks
    let matchingIframe = null;
    try {
        const iframes = Array.from(document.querySelectorAll('.mcp-app-iframe'));
        matchingIframe = iframes.find(f => f.contentWindow === sender);
    } catch (e) {
        // Ignore cross-origin access errors
    }

    if (!matchingIframe) {
        return;
    }

    if (data && typeof data === 'object') {
        const hasId = data.id !== undefined && data.id !== null;
        console.debug('[MCP Host Received Message]', data);
        if (data.method === 'ui/initialize' && hasId) {
            console.debug('Handling MCP Apps initialize request:', data);
            sender?.postMessage({
                jsonrpc: '2.0',
                id: data.id,
                result: {
                    protocolVersion: data.params?.protocolVersion || MCP_PROTOCOL_VERSION,
                    hostInfo: { name: 'MCP Toolbox Playground', version: '1.0.0' },
                    hostCapabilities: {
                        openLinks: {},
                        serverTools: {},
                        serverResources: {}
                    },
                    hostContext: {
                        theme: 'light',
                        displayMode: 'inline',
                        platform: 'web',
                        deviceCapabilities: {
                            touch: false,
                            hover: true
                        }
                    }
                }
            }, senderOrigin);
        } else if (data.method === 'ui/request-display-mode' || data.method === 'requestDisplayMode' || data.type === 'ui/requestDisplayMode' || data.type === 'ui/set_display_mode') {
            console.debug('Handling MCP Apps request-display-mode:', data);
            const mode = data.params?.mode || data.params?.displayMode || data.mode || data.displayMode || 'inline';
            // matchingIframe is already resolved at the top of the message handler
            const mcpContainer = matchingIframe?.closest('.mcp-app-container') || document.querySelector('.mcp-app-container');

            setAppDisplayMode(mcpContainer, matchingIframe, mode);

            if (hasId && sender) {
                sender.postMessage({
                    jsonrpc: '2.0',
                    id: data.id,
                    result: { mode: mode }
                }, senderOrigin);
            }
            sender?.postMessage({
                jsonrpc: '2.0',
                method: 'ui/notifications/host-context-changed',
                params: { displayMode: mode }
            }, senderOrigin);
        } else if (data.method === 'ui/open-link' || data.method === 'open-link' || data.type === 'ui/openUrl' || data.type === 'open_link' || data.action === 'open_link') {
            const targetUrl = data.params?.url || data.payload?.url || data.url;
            if (!targetUrl || typeof targetUrl !== 'string') {
                if (hasId && sender) {
                    sender.postMessage({
                        jsonrpc: '2.0',
                        id: data.id,
                        error: {
                            code: -32602,
                            message: 'URL is required and must be a string'
                        }
                    }, senderOrigin);
                }
            } else {
                try {
                    const parsed = new URL(targetUrl, window.location.href);
                    if (parsed.protocol === 'http:' || parsed.protocol === 'https:') {
                        console.debug('Opening external link requested by MCP App:', parsed.href);
                        const opened = window.open(parsed.href, '_blank', 'noopener,noreferrer');
                        if (!opened) {
                            console.warn('Browser popup blocker prevented opening link:', parsed.href);
                            if (hasId && sender) {
                                sender.postMessage({
                                    jsonrpc: '2.0',
                                    id: data.id,
                                    error: {
                                        code: -32000,
                                        message: 'Browser popup blocker blocked opening external link.'
                                    }
                                }, senderOrigin);
                            }
                        } else if (hasId && sender) {
                            sender.postMessage({
                                jsonrpc: '2.0',
                                id: data.id,
                                result: { success: true }
                            }, senderOrigin);
                        }
                    } else {
                        console.warn('Blocked opening non-http/https URL from MCP App:', targetUrl);
                        if (hasId && sender) {
                            sender.postMessage({
                                jsonrpc: '2.0',
                                id: data.id,
                                error: {
                                    code: -32602,
                                    message: 'Only http and https URLs are allowed'
                                }
                            }, senderOrigin);
                        }
                    }
                } catch (e) {
                    console.error('Invalid URL requested by MCP App:', targetUrl, e);
                    if (hasId && sender) {
                        sender.postMessage({
                            jsonrpc: '2.0',
                            id: data.id,
                            error: {
                                code: -32602,
                                message: `Invalid URL: ${e.message}`
                            }
                        }, senderOrigin);
                    }
                }
            }
        } else if (data.method === 'tools/call' || data.method === 'callTool') {
            console.debug('Handling MCP Apps tools/call request:', data);
            const toolCallParams = data.params || {};
            const toolName = toolCallParams.name;
            const toolArgs = toolCallParams.arguments || {};

            if (!toolName || typeof toolName !== 'string') {
                if (hasId && sender) {
                    sender.postMessage({
                        jsonrpc: '2.0',
                        id: data.id,
                        error: {
                            code: -32602,
                            message: 'Tool name is required and must be a string'
                        }
                    }, senderOrigin);
                }
                return;
            }

            const toolId = matchingIframe ? matchingIframe.id.replace('mcp-app-iframe-', '') : '';
            const toolHeaders = toolHeadersMap.get(toolId) || activeHeaders;

            fetch('/mcp', {
                method: 'POST',
                headers: createMcpHeaders('tools/call', toolName, toolHeaders),
                body: JSON.stringify(createMcpRequestBody('tools/call', {
                    name: toolName,
                    arguments: toolArgs
                }, hasId ? data.id : 'tool-call'))
            })
            .then(async res => {
                if (!res.ok) {
                    const errorText = await res.text();
                    throw new Error(`HTTP error ${res.status}: ${errorText || res.statusText}`);
                }
                return res.json();
            })
            .then(toolResult => {
                console.debug('Tool call response from server:', toolResult);
                let parsedData = null;
                try {
                    if (toolResult.result && Array.isArray(toolResult.result.content)) {
                        // SQL-backed tools return one text item per row, so collect them all
                        // rather than truncating the result to the first row.
                        const textItems = toolResult.result.content.filter(c => c.type === 'text');
                        if (textItems.length === 1) {
                            parsedData = JSON.parse(textItems[0].text);
                        } else if (textItems.length > 1) {
                            try {
                                parsedData = textItems.map(c => JSON.parse(c.text));
                            } catch {
                                parsedData = textItems.map(c => c.text);
                            }
                        }
                    } else if (toolResult.result) {
                        parsedData = toolResult.result;
                    }
                } catch (e) {
                    parsedData = toolResult.result;
                }

                // A tool can fail either at the JSON-RPC transport level (toolResult.error)
                // or at the tool level (result.isError with the reason in content).
                const isError = !!toolResult.error || toolResult.result?.isError === true;

                let structuredContent;
                if (isError) {
                    // Don't hand back error text as if it were renderable data.
                    structuredContent = undefined;
                } else if (Array.isArray(parsedData)) {
                    structuredContent = {
                        data: parsedData,
                        queryData: parsedData
                    };
                } else if (parsedData && typeof parsedData === 'object') {
                    const rawData = parsedData.visualizationData?.queryResult?.data || parsedData.data || [];
                    structuredContent = {
                        ...parsedData,
                        queryData: rawData,
                        visualizationData: parsedData.visualizationData
                    };
                } else {
                    structuredContent = parsedData;
                }

                const resultContent = toolResult.result?.content || [
                    { type: 'text', text: toolResult.error ? (toolResult.error.message || JSON.stringify(toolResult.error)) : (typeof toolResult.result === 'string' ? toolResult.result : JSON.stringify(toolResult.result || {})) }
                ];

                // 1. Reply to the app's callServerTool request
                if (hasId && sender) {
                    sender.postMessage({
                        jsonrpc: '2.0',
                        id: data.id,
                        result: {
                            content: resultContent,
                            structuredContent: structuredContent,
                            isError: isError
                        }
                    }, senderOrigin);
                }

                // 2. Only send tool-result notification and update response area if this was the root tool
                if (activeTool && activeTool.name === toolName) {
                    const toolResultNotification = {
                        jsonrpc: '2.0',
                        method: 'ui/notifications/tool-result',
                        params: {
                            content: resultContent,
                            isError: isError,
                            structuredContent: structuredContent
                        }
                    };
                    sender?.postMessage(toolResultNotification, senderOrigin);

                    // 3. Update the playground UI response area
                    const responseArea = document.querySelector('.tool-response-area');
                    const prettifyCheckbox = document.querySelector('.prettify-checkbox');
                    if (responseArea) {
                        responseArea.value = prettifyCheckbox && prettifyCheckbox.checked ? JSON.stringify(toolResult, null, 2) : JSON.stringify(toolResult);
                    }
                }
            })
            .catch(err => {
                console.error('Error executing tool call from app:', err);
                if (hasId && sender) {
                    sender.postMessage({
                        jsonrpc: '2.0',
                        id: data.id,
                        error: {
                            code: -32603,
                            message: err.message
                        }
                    }, senderOrigin);
                }
            });
        } else if (data.method === 'ui/notifications/size-changed' || data.method === 'ui/size-changed' || data.type === 'ui/size_changed' || data.type === 'size_changed') {
            const height = data.params?.height || data.height;
            // matchingIframe is already resolved at the top of the message handler
            if (matchingIframe && height && typeof height === 'number') {
                const container = matchingIframe.closest('.mcp-app-container');
                if (!container || (!container.classList.contains('mcp-app-fullscreen') && container.style.position !== 'fixed')) {
                    matchingIframe.style.height = `${Math.max(480, Math.ceil(height))}px`;
                }
            }
        } else if (data.method === 'ping' || data.method === 'ui/ping') {
            if (hasId && event.source) {
                event.source.postMessage({
                    jsonrpc: '2.0',
                    id: data.id,
                    result: {}
                }, '*');
            }
        } else if (hasId && event.source) {
            // General fallback response for any unanswered request to avoid timeout
            console.debug('Acknowledging unhandled MCP Apps request to prevent timeout:', data.method || data.type, data.id);
            event.source.postMessage({
                jsonrpc: '2.0',
                id: data.id,
                result: {}
            }, '*');
        }
    }
});

// Support exiting fullscreen with Escape key
document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') {
        const fullscreenContainer = document.querySelector('.mcp-app-container.mcp-app-fullscreen') || document.querySelector('.mcp-app-container');
        if (fullscreenContainer && (fullscreenContainer.classList.contains('mcp-app-fullscreen') || fullscreenContainer.style.position === 'fixed')) {
            const iframe = fullscreenContainer.querySelector('.mcp-app-iframe');
            setAppDisplayMode(fullscreenContainer, iframe, 'inline');
            if (iframe && iframe.contentWindow) {
                iframe.contentWindow.postMessage({
                    jsonrpc: '2.0',
                    method: 'ui/notifications/host-context-changed',
                    params: { displayMode: 'inline' }
                }, '*');
            }
        }
    }
});

/**
 * Checks if a specific parameter is marked as included for a given tool.
 * @param {string} toolId The ID of the tool.
 * @param {string} paramName The name of the parameter.
 * @return {boolean|null} True if the parameter's include checkbox is checked,
 *                         False if unchecked, Null if the checkbox element is not found.
 */
export function isParamIncluded(toolId, paramName) {
    const inputId = `param-${toolId}-${paramName}`;
    const includeCheckboxId = `include-${inputId}`;
    const includeCheckbox = document.getElementById(includeCheckboxId);

    if (includeCheckbox && includeCheckbox.type === 'checkbox') {
        return includeCheckbox.checked;
    }

    console.warn(`Include checkbox not found for ID: ${includeCheckboxId}`);
    return null;
}

// Templates for inserting token retrieval instructions into edit header modal
const AUTH_TOKEN_INSTRUCTIONS_SERVICE_ACCOUNT = `
        <p>To obtain a Google OAuth ID token using a service account:</p>
        <ol>
            <li>Make sure you are on the intended SERVICE account (typically contain iam.gserviceaccount.com). Verify by running the command below.
                <pre><code>gcloud auth list</code></pre>
            </li>
            <li>Print an id token with the audience set to your clientID defined in config:
                <pre><code>gcloud auth print-identity-token --audiences=YOUR_CLIENT_ID_HERE</code></pre>
            </li>
            <li>Copy the output token.</li>
            <li>Paste this token into the header in JSON editor. The key should be the name of your auth service followed by <code>_token</code>
                <pre><code>{
  "Content-Type": "application/json",
  "my-google-auth_token": "YOUR_ID_TOKEN_HERE"
}               </code></pre>
            </li>
        </ol>
        <p>This token is typically short-lived.</p>`;

const AUTH_TOKEN_INSTRUCTIONS_STANDARD = `
        <p>To obtain a Google OAuth ID token using a standard account:</p>
        <ol>
            <li>Make sure you are on your intended standard account. Verify by running the command below.
                <pre><code>gcloud auth list</code></pre>
            </li>
            <li>Within your Cloud Console, add the following link to the "Authorized Redirect URIs".</li>
            <pre><code>https://developers.google.com/oauthplayground</code></pre>
            <li>Go to the Google OAuth Playground site: <a href="https://developers.google.com/oauthplayground/" target="_blank">https://developers.google.com/oauthplayground/</a></li>
            <li>In the top right settings menu, select "Use your own OAuth Credentials".</li>
            <li>Input your clientID (from config), along with the client secret from Cloud Console.</li>
            <li>Inside the Google OAuth Playground, select "Google OAuth2 API v2.</li>
            <ul>
                <li>Select "Authorize APIs".</li>
                <li>Select "Exchange Authorization codes for tokens"</li>
                <li>Copy the id_token field provided in the response.</li>
            </ul>
            <li>Paste this token into the header in JSON editor. The key should be the name of your auth service followed by <code>_token</code>
                <pre><code>{
  "Content-Type": "application/json",
  "my-google-auth_token": "YOUR_ID_TOKEN_HERE"
}               </code></pre>
            </li>
        </ol>
        <p>This token is typically short-lived.</p>`;