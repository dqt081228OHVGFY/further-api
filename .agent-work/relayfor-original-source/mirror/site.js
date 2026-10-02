(function () {
  'use strict';
  const api = window.ProxyLan;

  async function loadAuthConfig() {
    try {
      return await api.request('/api/auth/config');
    } catch (_) {
      return { local_registration_enabled: false, linuxdo_login_enabled: false, linuxdo_registration_enabled: false, registration_code_required: true };
    }
  }

  function applyContext(config) {
    const isPro = Boolean(config && config.is_pro_host);
    document.documentElement.dataset.site = isPro ? 'pro' : 'public';
    api.setText('[data-site-subtitle]', isPro ? '商业站' : '公益站');
    if (document.body.dataset.page === 'public') {
      document.title = isPro ? 'RelayFor 商业站' : 'RelayFor 公益站';
      api.setText('[data-public-title]', isPro ? 'RelayFor 商业站' : 'RelayFor 公益站');
      api.setText('[data-public-description]', isPro ? '商业 API 服务。' : '公益 API 服务。');
    }
    return isPro;
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
