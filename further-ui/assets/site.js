(function () {
  'use strict';
  const api = window.FurtherUI;

  function enforceBrand() {
    api.setText('.rail-title, .public-brand strong, .auth-brand strong, [data-site-name]', 'further-api');
    document.querySelectorAll('link[rel="icon"]').forEach(function (icon) { icon.href = '/further-api-logo.svg?v=handshake-20261004'; });
    document.querySelectorAll('.rail-logo img').forEach(function (logo) {
      logo.onerror = function () { logo.removeAttribute('onerror'); };
      logo.src = '/further-api-logo.svg?v=handshake-20261004';
    });
    if (document.title) {
      const prefix = document.title.split('·')[0].trim();
      document.title = (prefix || 'further-api') + ' · further-api';
    }
  }

  async function loadAuthConfig() {
    try {
      return await api.request('/api/auth/config');
    } catch (_) {
      return { unavailable: true, local_registration_enabled: false, linuxdo_login_enabled: false, linuxdo_registration_enabled: false, registration_code_required: true };
    }
  }

  function applyContext(config) {
    if (config) { config.system_name = 'further-api'; config.logo = '/further-api-logo.svg?v=handshake-20261004'; }
    document.documentElement.dataset.site = 'gateway';
    api.setText('[data-site-subtitle]', 'AI API 网关');
    enforceBrand();
    if (document.body.dataset.page === 'public') document.title = 'further-api · AI API 网关';
    if (config && Object.prototype.hasOwnProperty.call(config, 'logo')) {
      const logoUrl = config.logo || '/further-api-logo.svg?v=handshake-20261004';
      if (window.FurtherGateway && window.FurtherGateway.applyLogo) {
        window.FurtherGateway.applyLogo(logoUrl);
      } else {
        document.querySelectorAll('.rail-logo').forEach(function (element) {
          let logo = element.querySelector('img');
          if (!logo) {
            logo = document.createElement('img');
            logo.alt = '';
            logo.width = 40;
            logo.height = 40;
            element.replaceChildren(logo);
          }
          logo.onerror = function () { if (logo.getAttribute('src') !== '/further-api-logo.svg?v=handshake-20261004') logo.src = '/further-api-logo.svg?v=handshake-20261004'; };
          if (logo.getAttribute('src') !== logoUrl) logo.src = logoUrl;
        });
        document.querySelectorAll('link[rel="icon"]').forEach(function (icon) { icon.href = logoUrl; });
      }
    }
    enforceBrand();
    return false;
  }

  async function loadAnnouncement() {
    const container = document.querySelector('[data-site-announcement]');
    try {
      const announcement = await api.request('/api/site/announcement', { _suppressAuthRedirect: true });
      const content = String(announcement && announcement.content || '').trim();
      if (container) {
        api.setText('[data-site-announcement-title]', announcement && announcement.title || '站点公告');
        api.setText('[data-site-announcement-content]', content);
        container.classList.toggle('hidden', !content);
      }
      return announcement;
    } catch (_) {
      if (container) container.classList.add('hidden');
      return null;
    }
  }

  window.FurtherUISite = { loadAuthConfig: loadAuthConfig, applyContext: applyContext, loadAnnouncement: loadAnnouncement, enforceBrand: enforceBrand };
  document.addEventListener('further:ready', enforceBrand);
}());
