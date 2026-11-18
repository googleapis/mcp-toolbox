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

const NAV_COLLAPSED_STORAGE_KEY = 'toolbox-left-nav-collapsed';

/**
 * Renders the navigation bar HTML content into the specified container element.
 * @param {string} containerId The ID of the DOM element to inject the navbar into.
 * @param {string | null} activePath The active tab from the navbar.
 */
function renderNavbar(containerId, activePath) {
    const navbarContainer = document.getElementById(containerId);
    if (!navbarContainer) {
        console.error(`Navbar container with ID "${containerId}" not found.`);
        return;
    }

    const nav = document.createElement('nav');
    nav.className = 'left-nav';

    const toggleBtn = document.createElement('button');
    toggleBtn.type = 'button';
    toggleBtn.className = 'nav-toggle-btn';

    const logoDiv = document.createElement('div');
    logoDiv.className = 'nav-logo';
    const logoImage = document.createElement('img');
    logoImage.src = '/ui/assets/mcptoolboxlogo.png';
    logoImage.alt = 'App Logo';
    logoImage.addEventListener('click', () => {
        window.location.href = '/ui/';
    });
    logoDiv.appendChild(logoImage);

    const navList = document.createElement('ul');
    const items = [
        { href: '/ui/tools', text: 'Tools' },
        { href: '/ui/toolsets', text: 'Toolsets' }
    ];

    items.forEach(item => {
        const li = document.createElement('li');
        const link = document.createElement('a');
        link.href = item.href;
        link.textContent = item.text;
        if (activePath && item.href === activePath) {
            link.classList.add('active');
        }
        li.appendChild(link);
        navList.appendChild(li);
    });

    const applyCollapsedState = (isCollapsed) => {
        navbarContainer.classList.toggle('collapsed', isCollapsed);
        toggleBtn.textContent = isCollapsed ? '»' : '«';
        toggleBtn.title = isCollapsed ? 'Expand navigation panel' : 'Collapse navigation panel';
        toggleBtn.setAttribute('aria-label', toggleBtn.title);
        toggleBtn.setAttribute('aria-expanded', String(!isCollapsed));
    };

    let initialCollapsed = false;
    try {
        initialCollapsed = localStorage.getItem(NAV_COLLAPSED_STORAGE_KEY) === 'true';
    } catch (e) {
        // Ignore storage access errors in restricted modes
    }
    applyCollapsedState(initialCollapsed);

    toggleBtn.addEventListener('click', () => {
        const nextCollapsed = !navbarContainer.classList.contains('collapsed');
        applyCollapsedState(nextCollapsed);
        try {
            localStorage.setItem(NAV_COLLAPSED_STORAGE_KEY, String(nextCollapsed));
        } catch (e) {
            // Ignore storage access errors in restricted modes
        }
    });

    nav.appendChild(toggleBtn);
    nav.appendChild(logoDiv);
    nav.appendChild(navList);
    navbarContainer.replaceChildren(nav);
}
