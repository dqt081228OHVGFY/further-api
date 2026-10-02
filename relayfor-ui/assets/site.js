(function () {
  'use strict';
  const api = window.ProxyLan;

  async function loadAuthConfig() {
    try {
      return await api.request('/api/auth/config');
    } catch (_) {
      return { unavailable: true, local_registration_enabled: false, linuxdo_login_enabled: false, linuxdo_registration_enabled: false, registration_code_required: true };
    }
  }

  function applyContext(config) {
    document.documentElement.dataset.site = 'gateway';
    api.setText('[data-site-subtitle]', 'AI API 网关');
    if (config && config.system_name) {
      api.setText('.rail-title, .public-brand strong, .auth-brand strong, [data-site-name]', config.system_name);
      if (document.body.dataset.page === 'public' || document.body.dataset.page === 'auth') document.title = config.system_name;
    }
    if (config && Object.prototype.hasOwnProperty.call(config, 'logo')) {
      const logoUrl = config.logo || '/relayfor-logo.svg';
      if (window.RelayForGateway && window.RelayForGateway.applyLogo) {
        window.RelayForGateway.applyLogo(logoUrl);
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
          logo.onerror = function () { if (logo.getAttribute('src') !== '/relayfor-logo.svg') logo.src = '/relayfor-logo.svg'; };
          if (logo.getAttribute('src') !== logoUrl) logo.src = logoUrl;
        });
        document.querySelectorAll('link[rel="icon"]').forEach(function (icon) { icon.href = logoUrl; });
      }
    }
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

  window.ProxyLanSite = { loadAuthConfig: loadAuthConfig, applyContext: applyContext, loadAnnouncement: loadAnnouncement };
}());
