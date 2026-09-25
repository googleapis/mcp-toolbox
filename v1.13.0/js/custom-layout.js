/**
 * Custom Layout Interactivity
 * Handles dynamic offsets, DOM repositioning, and UI enhancements.
 */

// ==========================================================================
// BANNER CONFIGURATION
// ==========================================================================
const BANNER_CONFIG = {
  title: "MCP Toolbox Now Supports the New Stateless MCP Spec!",
  message: "Try out the new 2026-07-28 stateless MCP spec for better scaling.",
  linkText: "Read the launch blog!",
  linkUrl: "https://medium.com/google-cloud/mcp-toolbox-adds-support-for-the-new-july-28-mcp-spec-a0fbe401cbba"
};


document.addEventListener('DOMContentLoaded', function() {

  // ==========================================================================
  // DYNAMIC STYLES INJECTION
  // ==========================================================================
  var styleTag = document.createElement('style');
  styleTag.innerHTML = `
    .td-navbar .dropdown-menu {
      z-index: 9999 !important;
    }

    .theme-banner-wrapper {
      z-index: 20;
      padding-top: 15px;
      padding-bottom: 5px;
      margin-bottom: 2rem;
      background-color: var(--bs-body-bg, #ffffff);
    }

    .theme-banner {
      background-color: #ebf3fc;
      border: 1px solid #80a7e9;
      color: #1c3a6b;
      border-radius: 4px;
      padding: 12px 15px;
      width: 100%;
      box-shadow: 0 4px 6px rgba(0,0,0,0.05);
      display: flex;
      justify-content: space-between;
      align-items: center;
      gap: 15px;
    }

    .theme-banner-content {
      flex-grow: 1;
      text-align: center;
    }

    .theme-banner a {
      color: #4484f4;
      text-decoration: underline;
      font-weight: bold;
    }

    /* Cancel Button Styling */
    .theme-banner button.close-btn {
      background: none;
      border: none;
      color: inherit;
      font-size: 22px;
      cursor: pointer;
      line-height: 1;
      padding: 0 5px;
      opacity: 0.6;
      transition: opacity 0.2s;
    }

    .theme-banner button.close-btn:hover {
      opacity: 1;
    }

    /* DARK MODE STYLING */
    html[data-bs-theme="dark"] .theme-banner-wrapper,
    body.dark .theme-banner-wrapper,
    html.dark-mode .theme-banner-wrapper {
      background-color: var(--bs-body-bg, #20252b);
    }

    html[data-bs-theme="dark"] .theme-banner,
    body.dark .theme-banner,
    html.dark-mode .theme-banner {
      background-color: #1a273b;
      color: #e6efff;
      box-shadow: 0 4px 6px rgba(0,0,0,0.3);
    }

    html[data-bs-theme="dark"] .theme-banner a,
    body.dark .theme-banner a,
    html.dark-mode .theme-banner a {
      color: #80a7e9;
    }

    @media (prefers-color-scheme: dark) {
      html:not([data-bs-theme="light"]):not(.light) .theme-banner-wrapper {
        background-color: var(--bs-body-bg, #20252b);
      }
      html:not([data-bs-theme="light"]):not(.light) .theme-banner {
        background-color: #1a273b;
        color: #e6efff;
        box-shadow: 0 4px 6px rgba(0,0,0,0.3);
      }
      html:not([data-bs-theme="light"]):not(.light) .theme-banner a {
        color: #80a7e9;
      }
    }

    /* Disable Sticky Banner on Mobile */
    @media (max-width: 991.98px) {
      .theme-banner-wrapper {
        position: relative !important;
        top: auto !important;
        z-index: 1;
      }
    }
  `;
  document.head.appendChild(styleTag);

  // ==========================================================================
  // HEADER OFFSET CALCULATOR
  // ==========================================================================

  function updateHeaderOffset() {
    var mainNav = document.querySelector('.td-navbar');
    var secondaryNav = document.getElementById('secondary-nav');

    var h1 = mainNav ? mainNav.offsetHeight : 0;
    var h2 = secondaryNav ? secondaryNav.offsetHeight : 0;
    var totalHeight = h1 + h2;

    document.documentElement.style.setProperty('--header-offset', totalHeight + 'px');
  }

  // ==========================================================================
  // INJECT BANNER
  // ==========================================================================

  var storageKey = "hideGeneralBanner";

  if (sessionStorage.getItem(storageKey) !== "true") {

    var wrapper = document.createElement('div');
    wrapper.id = 'banner-wrapper';
    wrapper.className = 'theme-banner-wrapper';

    var banner = document.createElement('div');
    banner.className = 'theme-banner';

    // If the URL starts with http/https, add target="_blank" and security attributes
    var isExternal = BANNER_CONFIG.linkUrl.startsWith('http');
    var linkAttributes = isExternal ? ' target="_blank" rel="noopener noreferrer"' : '';

    banner.innerHTML = `
      <div class="theme-banner-content">
        <strong>${BANNER_CONFIG.title}</strong> ${BANNER_CONFIG.message}
        <a href="${BANNER_CONFIG.linkUrl}"${linkAttributes}>${BANNER_CONFIG.linkText}</a>.
      </div>
      <button id="close-general-banner" class="close-btn" aria-label="Close">&times;</button>
    `;
    wrapper.appendChild(banner);

    var contentArea = document.querySelector('.td-content') || document.querySelector('main');
    if (contentArea) {
      contentArea.prepend(wrapper);
    }

    var closeBtn = document.getElementById('close-general-banner');
    if (closeBtn) {
      closeBtn.addEventListener('click', function() {
        wrapper.style.display = 'none';
        sessionStorage.setItem(storageKey, "true");
      });
    }
  }

  // Initialize the dynamic offset
  updateHeaderOffset();

  window.addEventListener('resize', updateHeaderOffset);

  if (window.ResizeObserver) {
    const ro = new ResizeObserver(updateHeaderOffset);
    const navToWatch = document.querySelector('.td-navbar');
    const secNavToWatch = document.getElementById('secondary-nav');

    if (navToWatch) ro.observe(navToWatch);
    if (secNavToWatch) ro.observe(secNavToWatch);
  }

  // ==========================================================================
  // BREADCRUMBS REPOSITIONING
  // ==========================================================================
  var breadcrumbs = document.querySelector('.td-breadcrumbs') || document.querySelector('nav[aria-label="breadcrumb"]');
  var pageTitle = document.querySelector('.td-content h1');

  if (breadcrumbs && pageTitle) {
    pageTitle.parentNode.insertBefore(breadcrumbs, pageTitle);
    breadcrumbs.style.marginTop = "1rem";
    breadcrumbs.style.marginBottom = "2rem";
  }

});
