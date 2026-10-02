(function () {
  'use strict';

  // Existing business components mount directly in the RelayFor content area.
  const inlinePages = {
    admin: [
      { panel: 'overview', slug: 'analysis', nativeLabel: '总览', label: '模型分析', path: '/dashboard/models' },
      { panel: 'models', slug: 'metadata', nativeLabel: '模型配置', label: '模型资料与部署', path: '/models/metadata' },
      { panel: 'subscriptions', slug: 'billing', nativeLabel: '订阅与福利', label: '网关订阅计划', path: '/subscriptions' },
      { panel: 'logs', slug: 'tasks', nativeLabel: '调用与追踪', label: '网关与任务日志', path: '/usage-logs' }
    ],
    console: [
      { panel: 'tokens', slug: 'keys', nativeLabel: 'API Key', label: 'Key 扩展设置', path: '/keys' }
    ]
  };
  const settingsPages = [
    { slug: 'system-info', title: '运行监控', icon: 'dns', path: '/system-info' },
    { slug: 'request-policies', title: '请求检测与健康', icon: 'rule', path: '/system-settings/request-policies' },
    { slug: 'passkey', title: '通行密钥', icon: 'passkey', path: '/system-settings/auth/passkey' },
    { slug: 'model-pricing', title: '完整模型计费', icon: 'price_check', path: '/system-settings/billing/model-pricing' },
    { slug: 'group-pricing', title: '分组与权益倍率', icon: 'groups', path: '/system-settings/billing/group-pricing' },
    { slug: 'chat', title: '聊天预设', icon: 'forum', path: '/system-settings/content/chat' },
    { slug: 'alerts', title: '监控与告警', icon: 'notifications_active', path: '/system-settings/operations/alerts' }
  ];



  function migratePaymentHash(page, activate) {
    if (page !== 'admin') return false;
    const match = location.hash.match(/^#(?:native|gateway)\/(.+)$/);
    if (!match) return false;
    let path;
    try { path = decodeURIComponent(match[1]).split('?')[0].replace(/\/+$/, ''); } catch (_) { return false; }
    if (path !== '/system-settings/billing/payment') return false;
    history.replaceState(null, '', '#settings/payment');
    if (activate) {
      const nav = document.querySelector('[data-nav="settings"]');
      if (nav) nav.click();
      const button = document.querySelector('[data-settings-page="payment"]');
      if (button) button.click();
    }
    return true;
  }

  function migrateUsersHash(page, activate) {
    if (page !== 'admin') return false;
    const match = location.hash.match(/^#(?:native|gateway)\/(.+)$/);
    if (!match) return false;
    let path;
    try { path = decodeURIComponent(match[1]).split('?')[0].replace(/\/+$/, ''); } catch (_) { return false; }
    if (path !== '/users') return false;
    history.replaceState(null, '', '#users');
    if (activate) {
      const nav = document.querySelector('[data-nav="users"]');
      if (nav) nav.click();
    }
    return true;
  }

  function migrateConsoleDirectory(activate) {
    if (document.body.dataset.page !== 'console') return;
    if (migrateActivityHash('console', activate)) return;
    const retiredUsageAnalysis = /^#usage\/analysis(?:[/?]|$)/.test(location.hash);
    if (!retiredUsageAnalysis && !/^#(?:native|gateway)(?:\/|$)/.test(location.hash)) return;
    let path = '';
    try { path = decodeURIComponent(location.hash.replace(/^#(?:native|gateway)\/?/, '')).split('?')[0].replace(/\/+$/, ''); } catch (_) {}
    const targets = { '/keys': 'tokens', '/profile': 'profile', '/security': 'profile', '/wallet': 'vip/payment', '/dashboard/models': 'usage', '/dashboard/flow': 'usage', '/usage-logs': 'usage', '/usage-logs/common': 'usage' };
    const target = retiredUsageAnalysis ? 'usage' : targets[path] || 'overview';
    history.replaceState(null, '', '#' + target);
    if (activate) {
      const nav = document.querySelector('[data-nav="' + target.split('/')[0] + '"]');
      if (nav) nav.click();
    }
  }

  function installInlineStyles() {
    if (document.querySelector('[data-relayfor-inline-styles]')) return;
    const link = document.createElement('link');
    link.rel = 'stylesheet';
    link.dataset.relayforInlineStyles = '';
    const appStyle = document.querySelector('link[href*="/relayfor-assets/app.css"]');
    const version = appStyle ? new URL(appStyle.href, location.origin).searchParams.get('v') : '';
    link.href = '/relayfor-assets/extensions-inline.css' + (version ? '?v=' + encodeURIComponent(version) : '');
    document.head.append(link);
  }

  function createFeatureHost(title, path) {
    const host = document.createElement('div');
    host.className = 'relayfor-feature-region';
    host.dataset.nativeFeaturePath = path;
    host.setAttribute('aria-label', title);
    const status = document.createElement('div');
    status.className = 'empty'; status.setAttribute('role', 'status');
    status.textContent = '正在加载' + title + '…'; host.append(status);
    async function mount() {
      status.textContent = '正在加载' + title + '…';
      try {
        if (!window.RelayForFeatures) throw new Error('功能资源未就绪，请刷新后重试。');
        await window.RelayForFeatures.mount(host, { path: path });
        host.dataset.nativeFeatureMounted = 'true';
      } catch (error) {
        status.textContent = error.message || '加载失败，请重试。';
        status.setAttribute('role', 'alert');
        const retry = document.createElement('button'); retry.type = 'button';
        retry.className = 'md-btn md-btn-outlined'; retry.textContent = '重新加载';
        retry.addEventListener('click', mount); status.append(retry);
        // A failed mount may already have created its isolated style root.
        if (host.shadowRoot) { host.shadowRoot.replaceChildren(status); }
      }
    }
    void mount();
    return host;
  }

  function migrateActivityHash(page, activate) {
    const prefix = location.hash.startsWith('#native/') ? '#native/' : location.hash.startsWith('#gateway/') ? '#gateway/' : '';
    if (!prefix) return false;
    let pathname = '';
    try { pathname = decodeURIComponent(location.hash.slice(prefix.length)).split('?')[0].replace(/\/+$/, ''); }
    catch (_) { return false; }
    const management = pathname === '/system-settings/billing/checkin';
    const festival = pathname.match(/^\/activities\/(duanwu|qixi|mid_autumn|chongyang|new_year_eve|national_day)$/);
    if (!management && !festival && !['/activities', '/activity', '/benefits', '/checkin', '/profile/checkin'].includes(pathname)) return false;
    const shell = management ? 'admin' : 'console';
    const hash = '#activities' + (festival ? '/' + festival[1] : '');
    if (page !== shell) { location.replace('/' + shell + '/' + hash); return true; }
    history.replaceState(null, '', hash);
    // Initial migration runs before activities.js adds its nav; mountShell will
    // select the canonical hash once every relayfor:ready listener has mounted.
    if (activate) {
      const nav = document.querySelector('[data-nav="activities"]');
      if (nav) nav.click();
    }
    return true;
  }

  function migrateOldGatewayHash(page) {
    if (page === 'admin' && location.hash === '#users/accounts') history.replaceState(null, '', '#users');
    if (migrateUsersHash(page, false)) return;
    if (migratePaymentHash(page, false)) return;
    if (migrateActivityHash(page, false)) return;
    if (page === 'admin' && (location.hash === '#redemptions/codes' || location.hash === '#gateway/%2Fredemption-codes')) {
      history.replaceState(null, '', '#redemptions');
      return;
    }
    if (!location.hash.startsWith('#gateway/')) return;
    let pathname = '';
    try { pathname = new URL(decodeURIComponent(location.hash.slice(9)), location.origin).pathname; }
    catch (_) { /* An invalid saved link returns to the original page. */ }
    const inline = inlinePages[page].find(function (entry) { return entry.path === pathname; });
    const settings = page === 'admin' && settingsPages.find(function (entry) { return entry.path === pathname; });
    const target = inline ? '#' + inline.panel + '/' + inline.slug : settings ? '#settings/' + settings.slug : page === 'admin' && pathname === '/channels' ? '#channels' : page === 'console' && pathname === '/security' ? '#profile' : '#native/' + encodeURIComponent(pathname || '/dashboard');
    history.replaceState(null, '', target);
  }

  function mountInlinePage(area, entry) {
    const panel = area.querySelector('[data-panel="' + entry.panel + '"]');
    if (!panel || panel.querySelector('[data-relayfor-inline-tabs]')) return null;
    const tabs = document.createElement('nav');
    tabs.className = 'model-subpage-tabs relayfor-inline-tabs';
    tabs.dataset.relayforInlineTabs = '';
    tabs.setAttribute('aria-label', entry.nativeLabel + '页面视图');
    const nativeButton = document.createElement('button');
    nativeButton.type = 'button';
    nativeButton.className = 'model-subpage-tab active';
    nativeButton.textContent = entry.nativeLabel;
    nativeButton.setAttribute('aria-pressed', 'true');
    const inlineButton = document.createElement('button');
    inlineButton.type = 'button';
    inlineButton.className = 'model-subpage-tab';
    inlineButton.textContent = entry.label;
    inlineButton.setAttribute('aria-pressed', 'false');
    tabs.append(nativeButton, inlineButton);
    const host = document.createElement('div');
    host.className = 'relayfor-inline-host';
    host.hidden = true;
    panel.prepend(tabs);
    panel.append(host);
    let loaded = false;
    function activate(inline, updateLocation) {
      panel.classList.toggle('relayfor-inline-active', inline);
      host.hidden = !inline;
      nativeButton.classList.toggle('active', !inline);
      inlineButton.classList.toggle('active', inline);
      nativeButton.setAttribute('aria-pressed', inline ? 'false' : 'true');
      inlineButton.setAttribute('aria-pressed', inline ? 'true' : 'false');
      if (inline && !loaded) { host.append(createFeatureHost(entry.label, entry.path)); loaded = true; }
      if (updateLocation) {
        const target = '#' + entry.panel + (inline ? '/' + entry.slug : '');
        if (location.hash !== target) history.pushState(null, '', target);
      }
    }
    nativeButton.addEventListener('click', function () { activate(false, true); });
    inlineButton.addEventListener('click', function () { activate(true, true); });
    return function () {
      const parts = location.hash.slice(1).split('/');
      if (parts[0] === entry.panel) activate(parts[1] === entry.slug, false);
    };
  }

  function mountSettingsPages(area) {
    const panel = area.querySelector('[data-panel="settings"]');
    const directory = panel && panel.querySelector('[data-settings-directory]');
    if (!directory) return function () {};
    const hosts = Object.create(null);
    settingsPages.forEach(function (entry) {
      const button = document.createElement('button');
      button.type = 'button';
      button.className = 'md-card settings-directory-item';
      button.dataset.settingsPage = entry.slug;
      const icon = document.createElement('span');
      icon.className = 'settings-directory-icon';
      const symbol = document.createElement('span');
      symbol.className = 'material-symbol';
      symbol.setAttribute('aria-hidden', 'true');
      symbol.textContent = entry.icon;
      icon.append(symbol);
      const label = document.createElement('span');
      const bold = document.createElement('b');
      bold.textContent = entry.title;
      label.append(bold);
      const arrow = document.createElement('span');
      arrow.className = 'material-symbol settings-directory-arrow';
      arrow.setAttribute('aria-hidden', 'true');
      arrow.textContent = 'arrow_forward';
      button.append(icon, label, arrow);
      directory.append(button);

      const subpage = document.createElement('div');
      subpage.className = 'settings-subpage relayfor-inline-settings';
      subpage.dataset.settingsSubpage = entry.slug;
      subpage.hidden = true;
      const heading = document.createElement('div');
      heading.className = 'card-head settings-page-head';
      const actions = document.createElement('div');
      actions.className = 'list-actions';
      const back = document.createElement('button');
      back.type = 'button';
      back.className = 'md-btn md-btn-text';
      back.dataset.settingsBack = '';
      const backIcon = document.createElement('span');
      backIcon.className = 'material-symbol';
      backIcon.setAttribute('aria-hidden', 'true');
      backIcon.textContent = 'arrow_back';
      back.append(backIcon, document.createTextNode('返回设置'));
      const title = document.createElement('div');
      title.className = 'md-card-title';
      title.textContent = entry.title;
      actions.append(back, title);
      heading.append(actions);
      const host = document.createElement('div');
      host.className = 'relayfor-inline-host';
      subpage.append(heading, host);
      panel.append(subpage);
      hosts[entry.slug] = { entry: entry, host: host, loaded: false };
    });
    return function (slug) {
      const current = hosts[slug];
      if (!current || current.loaded) return;
      current.host.append(createFeatureHost(current.entry.title, current.entry.path));
      current.loaded = true;
    };
  }

  document.addEventListener('relayfor:ready', function () {
    const rail = document.querySelector('.nav-rail');
    const area = document.querySelector('.content-area');
    const page = document.body.dataset.page;
    if (!rail || !area || (page !== 'admin' && page !== 'console')) return;
    if (document.body.dataset.relayforInlineMounted === 'true') return;
    document.body.dataset.relayforInlineMounted = 'true';
    if (page === 'console') migrateConsoleDirectory(false);
    migrateOldGatewayHash(page);
    installInlineStyles();
    const syncNative = page === 'admin' ? window.ProxyLanAdminFeatureDirectory.mount(rail, area, { createFeatureHost: createFeatureHost, migrate: function () { return migrateUsersHash(page, true) || migratePaymentHash(page, true) || migrateActivityHash(page, true); } }) : function () { migrateConsoleDirectory(true); };
    if (page === 'console') {
      window.addEventListener('popstate', syncNative);
      window.addEventListener('hashchange', syncNative);
    }

    if (page === 'admin') {
      const bottom = rail.querySelector('.rail-bottom');
      const group = document.createElement('div');
      group.className = 'nav-group-title';
      group.textContent = '网关功能';
      rail.insertBefore(group, bottom);
      const button = document.createElement('button');
      button.type = 'button';
      button.className = 'nav-item';
      button.dataset.nav = 'risk';
      const icon = document.createElement('span');
      icon.className = 'material-symbol';
      icon.setAttribute('aria-hidden', 'true');
      icon.textContent = 'lock';
      button.append(icon, document.createTextNode('风险预警与反测活'));
      rail.insertBefore(button, bottom);
      const panel = document.createElement('section');
      panel.className = 'view gateway-extension-panel';
      panel.dataset.panel = 'risk';
      panel.dataset.title = '风险预警与反测活';
      panel.dataset.subtitle = '统一网关管理';
      area.append(panel);
      if (window.ProxyLanRisk) window.ProxyLanRisk.mount(panel);
      else window.addEventListener('proxylan:page', function (event) {
        if (event.detail && event.detail.name === 'risk' && !panel.querySelector('[data-native-feature-path]')) panel.append(createFeatureHost('风险预警与反测活', '/risk-control'));
      });
    }

    const syncPages = inlinePages[page].map(function (entry) { return mountInlinePage(area, entry); }).filter(Boolean);
    const showSettingsFrame = page === 'admin' ? mountSettingsPages(area) : function () {};
    window.addEventListener('proxylan:page', function () { syncPages.forEach(function (sync) { sync(); }); });
    window.addEventListener('relayfor:settings-subpage', function (event) { showSettingsFrame(event.detail && event.detail.name); });
    syncPages.forEach(function (sync) { sync(); });
    syncNative();
    if (page === 'admin') {
      const parts = location.hash.slice(1).split('/');
      if (parts[0] === 'settings' && parts[1]) showSettingsFrame(parts[1]);
    }
  });
}());
