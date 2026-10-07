(function () {
  'use strict';

  // Existing business components mount directly in the Further content area.
  const inlinePages = {
    admin: [
      { panel: 'overview', slug: 'analysis', nativeLabel: '概览', label: '模型分析', path: '/dashboard/models' }
    ],
    console: []
  };
  const settingsPages = [
    { slug: 'system-info', title: '运行监控', description: '网关资源与服务状态', group: 'operations', icon: 'dns', path: '/system-info' },
    { slug: 'passkey', title: '通行密钥', description: '账户免密登录和设备凭证', group: 'access', icon: 'passkey', path: '/system-settings/auth/passkey' },
    { slug: 'alerts', title: '监控与告警', description: '运行异常的通知和阈值', group: 'operations', icon: 'notifications_active', path: '/system-settings/operations/alerts' }
  ];
  const baseSettingsPages = settingsPages.slice();
  const settingsGroups = {
    access: { title: '身份与安全', description: '管理账户入口、登录身份与访问保护。', icon: 'shield' },
    routing: { title: '请求与流量', description: '处理请求检测、重试和渠道健康。', icon: 'alt_route' },
    billing: { title: '计费与权益', description: '维护支付和账户额度策略。', icon: 'payments' },
    site: { title: '站点呈现', description: '设置站点地址、登录和公告。', icon: 'language' },
    protocols: { title: '模型与协议', description: '调整模型兼容、思考预算和部署参数。', icon: 'neurology' },
    content: { title: '扩展服务', description: '管理数据导出与绘图服务。', icon: 'description' },
    operations: { title: '运行维护', description: '配置邮件、告警、性能和系统记录。', icon: 'dns' }
  };
  function prepareSettingsEntries(area) {
    const directory = area.querySelector('[data-settings-directory]');
    if (!directory) return;
    settingsPages.splice.apply(settingsPages, [0, settingsPages.length].concat(baseSettingsPages));
    const paths = new Set(settingsPages.map(function (entry) { return entry.path; }));
    // The local payment form is the single editor for this endpoint.
    if (directory.querySelector('[data-settings-page="payment"]')) paths.add('/system-settings/billing/payment');
    const slugs = new Set(settingsPages.map(function (entry) { return entry.slug; }));
    directory.querySelectorAll('[data-settings-page]').forEach(function (button) { slugs.add(button.dataset.settingsPage); });
    const role = Number(document.body.dataset.gatewayRole || 0);
    (window.FurtherFeatures && window.FurtherFeatures.entries || []).forEach(function (entry) {
      if (retiredSettingsPaths[entry.path]) return;
      if (!entry.path.startsWith('/system-settings/') || paths.has(entry.path) || role < (entry.role || 1)) return;
      const parts = entry.path.split('/').filter(Boolean), category = parts[1], leaf = parts[2];
      if (!leaf) return;
      const group = ({ auth: 'access', security: 'access', 'request-policies': 'routing', models: 'protocols', billing: 'billing', site: 'site', content: 'content', operations: 'operations' })[category] || 'operations';
      let slug = slugs.has(leaf) ? category + '-' + leaf : leaf;
      if (slugs.has(slug)) return;
      settingsPages.push({ slug: slug, title: entry.title, description: entry.description || settingsGroups[group].description, group: group, icon: settingsGroups[group].icon, path: entry.path });
      paths.add(entry.path); slugs.add(slug);
    });
  }

  const retiredSettingsPaths = {
    '/system-settings/site': '#settings',
    '/system-settings/auth': '#settings',
    '/system-settings/security': '#settings',
    '/system-settings/models': '#settings',
    '/system-settings/billing': '#settings',
    '/system-settings/content': '#settings',
    '/system-settings/operations': '#settings',
    '/system-settings/billing/group-pricing': '#models',
    '/system-settings/billing/model-pricing': '#models',
    '/system-settings/site/notice': '#settings/announcements',
    '/system-settings/site/header-navigation': '#settings',
    '/system-settings/site/sidebar-modules': '#settings',
    '/system-settings/content/announcements': '#settings/announcements',
    '/system-settings/content/api-info': '#settings/site-system-info',
    '/system-settings/content/faq': '#settings/site-system-info',
    '/system-settings/content/uptime-kuma': '#settings/system-info',
    '/system-settings/operations/update-checker': '#settings/system-info',
    '/system-settings/request-policies': '#settings/routing'
  };
  const retiredSettingsHashes = {
    '#models/pricing': '#models',
    '#settings/group-pricing': '#models',
    '#settings/model-pricing': '#models',
    '#settings/header-navigation': '#settings',
    '#settings/sidebar-modules': '#settings',
    '#settings/content-announcements': '#settings/announcements',
    '#settings/api-info': '#settings/site-system-info',
    '#settings/faq': '#settings/site-system-info',
    '#settings/uptime-kuma': '#settings/system-info',
    '#settings/update-checker': '#settings/system-info',
    '#settings/request-policies': '#settings/routing'
  };
  function migrateAccountSecurityHash(page) {
    if (page !== 'admin' || !/^#settings\/(?:security|account-security)(?:[/?]|$)/.test(location.hash)) return false;
    location.replace('/console/#profile');
    return true;
  }

  function migrateSettingsHash(page, activate) {
    if (page !== 'admin') return false;
    let target = retiredSettingsHashes[location.hash];
    if (!target) {
      const match = location.hash.match(/^#(?:workspace|native|gateway)\/(.+)$/);
      if (!match) return false;
      let path;
      try { path = decodeURIComponent(match[1]).split('?')[0].replace(/\/+$/, ''); } catch (_) { return false; }
      const entry = settingsPages.find(function (item) { return item.path === path; });
      target = retiredSettingsPaths[path] || (entry && '#settings/' + entry.slug);
    }
    if (!target) return false;
    history.replaceState(null, '', target);
    if (activate) {
      const name = target.slice(1).split('/')[0];
      const nav = document.querySelector('.nav-rail [data-nav="' + name + '"]');
      if (nav) nav.click();
      else window.FurtherUI?.navigatePage?.(name, { history: 'none' });
    }
    return true;
  }

  function mountSettingsFilter(directory) {
    const controls = document.createElement('div'); controls.className = 'settings-directory-controls';
    const search = document.createElement('label'); search.className = 'settings-directory-search';
    const symbol = document.createElement('span'); symbol.className = 'material-symbol'; symbol.textContent = 'search'; symbol.setAttribute('aria-hidden', 'true');
    const input = document.createElement('input'); input.type = 'search'; input.placeholder = '查找安全与设置'; input.setAttribute('aria-label', '查找安全与设置'); search.append(symbol, input);
    const categories = document.createElement('nav'); categories.className = 'settings-directory-categories'; categories.setAttribute('aria-label', '设置分类');
    const results = document.createElement('p'); results.className = 'settings-directory-results'; results.setAttribute('role', 'status'); results.setAttribute('aria-live', 'polite');
    const empty = document.createElement('div'); empty.className = 'empty settings-directory-empty'; empty.textContent = '没有找到相关配置，试试“登录”“计费”或“邮件”。'; empty.hidden = true;
    const groups = Array.from(directory.querySelectorAll('[data-settings-group]'));
    let category = '';
    const choices = [['', '全部配置']].concat(groups.map(function (group) { return [group.dataset.settingsGroup, settingsGroups[group.dataset.settingsGroup].title]; }));
    const update = function () {
      const query = input.value.trim().toLowerCase(); let count = 0;
      groups.forEach(function (group) {
        let matches = 0;
        group.querySelectorAll('[data-settings-page]').forEach(function (button) { const visible = (!category || category === group.dataset.settingsGroup) && (!query || button.textContent.toLowerCase().includes(query)); button.hidden = !visible; if (visible) { matches++; count++; } });
        const section = group.closest('.settings-directory-group'); if (section) section.hidden = matches === 0;
      });
      categories.querySelectorAll('button').forEach(function (button) { button.setAttribute('aria-pressed', String(button.dataset.settingsCategory === category)); });
      results.textContent = count + ' 个配置页面'; empty.hidden = count > 0;
    };
    choices.forEach(function (choice) { const button = document.createElement('button'); button.type = 'button'; button.dataset.settingsCategory = choice[0]; button.textContent = choice[1]; button.addEventListener('click', function () { category = choice[0]; update(); }); categories.append(button); });
    input.addEventListener('input', update); controls.append(search, categories, results); directory.prepend(controls); directory.append(empty); update();
  }



  function migratePaymentHash(page, activate) {
    if (page !== 'admin') return false;
    const match = location.hash.match(/^#(?:workspace|native|gateway)\/(.+)$/);
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
    const match = location.hash.match(/^#(?:workspace|native|gateway)\/(.+)$/);
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

  function migrateSubscriptionsHash(page, activate) {
    if (page !== 'admin') return false;
    const match = location.hash.match(/^#(?:workspace|native|gateway)\/(.+)$/);
    if (!match) return false;
    let path;
    try { path = decodeURIComponent(match[1]).split('?')[0].replace(/\/+$/, ''); } catch (_) { return false; }
    if (path !== '/subscriptions') return false;
    history.replaceState(null, '', '#subscriptions/paid');
    if (activate) {
      const nav = document.querySelector('[data-nav="subscriptions"]');
      if (nav) nav.click();
    }
    return true;
  }

  function migrateConsoleDirectory(activate) {
    if (document.body.dataset.page !== 'console') return;
    if (migrateActivityHash('console', activate)) return;
    if (/^#tokens\/keys(?:[/?]|$)/.test(location.hash)) {
      history.replaceState(null, '', '#tokens');
      if (activate) document.querySelector('.nav-rail [data-nav="tokens"]')?.click();
      return;
    }
    const retiredUsageAnalysis = /^#usage\/analysis(?:[/?]|$)/.test(location.hash);
    if (!retiredUsageAnalysis && !/^#(?:workspace|native|gateway)(?:\/|$)/.test(location.hash)) return;
    let path = '';
    try { path = decodeURIComponent(location.hash.replace(/^#(?:workspace|native|gateway)\/?/, '')).split('?')[0].replace(/\/+$/, ''); } catch (_) {}
    const targets = { '/keys': 'tokens', '/profile': 'profile', '/security': 'profile', '/wallet': 'wallet', '/dashboard/models': 'usage', '/dashboard/flow': 'usage', '/usage-logs': 'usage', '/usage-logs/common': 'usage' };
    const target = retiredUsageAnalysis ? 'usage' : targets[path] || 'overview';
    history.replaceState(null, '', '#' + target);
    if (activate) {
      const nav = document.querySelector('[data-nav="' + target.split('/')[0] + '"]');
      if (nav) nav.click();
    }
  }

  function migrateUnifiedFeatureHash(page, activate) {
    if (page !== 'admin' || typeof window.FurtherGateway?.workspaceShellTarget !== 'function') return false;
    const match = location.hash.match(/^#(?:workspace|native|gateway)\/(.+)$/);
    if (!match) return false;
    let path;
    try { path = decodeURIComponent(match[1]); } catch (_) { return false; }
    const target = new URL(window.FurtherGateway.workspaceShellTarget(path, Number(document.body.dataset.gatewayRole || 0)), location.origin);
    if (target.pathname.replace(/\/+$/, '') !== location.pathname.replace(/\/+$/, '')) {
      location.replace(target.href);
      return true;
    }
    if (target.hash === '#workspace/' + encodeURIComponent(path)) return false;
    if (location.hash !== target.hash) history.replaceState(history.state, '', target.hash);
    if (activate) {
      const name = target.hash.slice(1).split('/')[0];
      const nav = document.querySelector('.nav-rail [data-nav="' + CSS.escape(name) + '"]');
      if (nav) nav.click();
      else window.FurtherUI.navigatePage?.(name, { history: 'none' });
    }
    return true;
  }

  function installInlineStyles() {
    if (document.querySelector('[data-further-inline-styles]')) return;
    const link = document.createElement('link');
    link.rel = 'stylesheet';
    link.dataset.furtherInlineStyles = '';
    const appStyle = document.querySelector('link[href*="/further-assets/app.css"]');
    const version = appStyle ? new URL(appStyle.href, location.origin).searchParams.get('v') : '';
    link.href = '/further-assets/extensions-inline.css' + (version ? '?v=' + encodeURIComponent(version) : '');
    document.head.append(link);
  }

  function createFeatureHost(title, path) {
    const host = document.createElement('div');
    host.className = 'further-feature-region';
    host.dataset.nativeFeaturePath = path;
    host.setAttribute('aria-label', title);
    const status = document.createElement('div');
    status.className = 'empty'; status.setAttribute('role', 'status');
    status.textContent = '正在加载' + title + '…'; host.append(status);
    async function mount() {
      status.textContent = '正在加载' + title + '…';
      try {
        if (!window.FurtherFeatures) throw new Error('功能资源未就绪，请刷新后重试。');
        await window.FurtherFeatures.mount(host, { path: path });
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
    const prefix = ['#workspace/', '#native/', '#gateway/'].find(function (value) { return location.hash.startsWith(value); }) || '';
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
    // select the canonical hash once every further:ready listener has mounted.
    if (activate) {
      const nav = document.querySelector('[data-nav="activities"]');
      if (nav) nav.click();
    }
    return true;
  }

  function migrateOldGatewayHash(page) {
    if (page === 'admin' && location.hash === '#logs/tasks') history.replaceState(history.state, '', '#logs');
    if (page === 'admin' && location.hash === '#audit') history.replaceState(history.state, '', '#workspace/' + encodeURIComponent('/usage-logs/audit'));
    if (page === 'admin' && /^#models\/(?:metadata|deployments)$/.test(location.hash)) history.replaceState(null, '', '#models');
    if (page === 'admin' && location.hash === '#users/accounts') history.replaceState(null, '', '#users');
    if (migrateUsersHash(page, false)) return;
    if (migratePaymentHash(page, false)) return;
    if (migrateActivityHash(page, false)) return;
    if (migrateSettingsHash(page, false)) return;
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
    const target = inline ? '#' + inline.panel + '/' + inline.slug : settings ? '#settings/' + settings.slug : page === 'admin' && pathname === '/channels' ? '#channels' : page === 'console' && pathname === '/security' ? '#profile' : '#workspace/' + encodeURIComponent(pathname || '/dashboard');
    history.replaceState(null, '', target);
  }

  function mountInlinePage(area, entry) {
    const panel = area.querySelector('[data-panel="' + entry.panel + '"]');
    if (!panel || panel.querySelector('[data-further-inline-tabs]')) return null;
    const tabs = document.createElement('nav');
    tabs.className = 'model-subpage-tabs further-inline-tabs';
    tabs.dataset.furtherInlineTabs = '';
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
    host.className = 'further-inline-host';
    host.hidden = true;
    const nativeHost = document.createElement('div');
    nativeHost.className = 'further-inline-native';
    while (panel.firstChild) nativeHost.append(panel.firstChild);
    panel.append(tabs, nativeHost, host);
    let loaded = false;
    function activate(inline, updateLocation) {
      panel.classList.toggle('further-inline-active', inline);
      const options = { skipMotion: document.documentElement.dataset.shellReady !== 'true' || panel.classList.contains('rf-entering') };
      const show = function (element, visible) {
        const apply = function (shown) { element.hidden = !shown; };
        if (window.FurtherUI?.transitionVisibility) window.FurtherUI.transitionVisibility(element, visible, apply, options);
        else apply(visible);
      };
      show(nativeHost, !inline);
      show(host, inline);
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
    function settingGroup(name) {
      let group = directory.querySelector('[data-settings-group="' + name + '"]');
      if (group) return group;
      const definition = settingsGroups[name];
      const section = document.createElement('section'); section.className = 'settings-directory-group';
      const heading = document.createElement('div'); heading.className = 'settings-directory-heading';
      const title = document.createElement('h2'); title.id = 'settings-group-' + name; title.textContent = definition.title; section.setAttribute('aria-labelledby', title.id);
      const description = document.createElement('p'); description.textContent = definition.description; heading.append(title, description);
      group = document.createElement('div'); group.className = 'settings-directory-list'; group.dataset.settingsGroup = name; section.append(heading, group); directory.append(section); return group;
    }
    settingsPages.forEach(function (entry) {
      const button = document.createElement('button');
      button.type = 'button';
      button.className = 'settings-directory-item';
      button.dataset.settingsPage = entry.slug;
      button.dataset.settingsFeaturePath = entry.path;
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
      const hint = document.createElement('small');
      hint.textContent = entry.description;
      label.append(hint);
      const arrow = document.createElement('span');
      arrow.className = 'material-symbol settings-directory-arrow';
      arrow.setAttribute('aria-hidden', 'true');
      arrow.textContent = 'arrow_forward';
      button.append(icon, label, arrow);
      settingGroup(entry.group).append(button);

      const subpage = document.createElement('div');
      subpage.className = 'settings-subpage further-inline-settings';
      subpage.dataset.settingsSubpage = entry.slug;
      subpage.dataset.settingsTitle = entry.title;
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
      actions.append(back);
      heading.append(actions);
      const host = document.createElement('div');
      host.className = 'further-inline-host';
      subpage.append(heading, host);
      panel.append(subpage);
      hosts[entry.slug] = { entry: entry, host: host, loaded: false };
    });
    mountSettingsFilter(directory);
    return function (slug) {
      const current = hosts[slug];
      if (!panel.isConnected || !current || current.loaded) return;
      const region = createFeatureHost(current.entry.title, current.entry.path);
      region.dataset.adminSettingsHost = '';
      current.host.append(region);
      current.loaded = true;
    };
  }

  function mountDirectWorkspace(area) {
    const panel = document.createElement('section');
    panel.className = 'view'; panel.dataset.panel = 'workspace';
    const context = document.createElement('div'); context.className = 'workspace-context';
    const back = document.createElement('a'); back.className = 'md-btn md-btn-text';
    const caption = document.createElement('span'); context.append(back, caption);
    const host = document.createElement('div'); host.className = 'further-inline-host';
    panel.append(context, host); area.append(panel);
    let currentPath = '';
    function sync() {
      if (migrateAccountSecurityHash('admin')) return;
      if (location.hash === '#logs/tasks') history.replaceState(history.state, '', '#logs');
      if (location.hash === '#audit') history.replaceState(history.state, '', '#workspace/' + encodeURIComponent('/usage-logs/audit'));
      if (!panel.isConnected) return;
      if (/^#models\/(?:metadata|deployments)$/.test(location.hash)) {
        history.replaceState(null, '', '#models');
        window.FurtherUI.navigatePage?.('models', { history:'none' });
        return;
      }
      const retired = location.hash.match(/^#(?:workspace|native|gateway)\/(.+)$/);
      if (retired) {
        let path = ''; try { path = decodeURIComponent(retired[1]).split('?')[0].replace(/\/+$/, ''); } catch (_) {}
        if (path === '/models/metadata' || path === '/models/deployments') {
          history.replaceState(null, '', '#models');
          window.FurtherUI.navigatePage?.('models', { history:'none' });
          return;
        }
      }
      if (migrateUnifiedFeatureHash('admin', true) || migrateUsersHash('admin', true) || migrateSubscriptionsHash('admin', true) || migratePaymentHash('admin', true) || migrateActivityHash('admin', true) || migrateSettingsHash('admin', true)) return;
      if (/^#(?:native|gateway|workspace)\/?$/.test(location.hash)) {
        history.replaceState(null, '', '#overview');
        window.FurtherUI.navigatePage?.('overview', { history: 'none' });
        return;
      }
      const match = location.hash.match(/^#(?:workspace|native|gateway)\/(.+)$/);
      if (!match) return;
      let path;
      try { path = decodeURIComponent(match[1]); } catch (_) { return; }
      const canonical = '#workspace/' + encodeURIComponent(path);
      if (location.hash !== canonical) history.replaceState(null, '', canonical);
      const pathname = path.split('?')[0].replace(/\/+$/, '');
      const entry = (window.FurtherFeatures?.entries || []).find(function (item) { return item.path === pathname; });
      const title = entry?.title || '工作区';
      const parent = pathname.startsWith('/models') ? ['models', '模型管理'] : pathname.startsWith('/usage-logs') ? ['logs', '调用记录'] : pathname.startsWith('/dashboard') || pathname === '/playground' ? ['overview', '概览'] : ['settings', '安全与设置'];
      back.href = '#' + parent[0]; back.textContent = '返回' + parent[1]; caption.textContent = title;
      panel.dataset.title = title; panel.dataset.subtitle = parent[1];
      if (currentPath !== path) {
        currentPath = path;
        const existing = host.querySelector('[data-native-feature-path]');
        if (existing) {
          window.FurtherFeatures.mount(existing, { path: path }).catch(function (error) {
            if (currentPath === path) currentPath = '';
            window.FurtherUI.toast(error.message, 'danger');
          });
        } else host.append(createFeatureHost(title, path));
      }
      window.FurtherUI.navigatePage?.('workspace', { history: 'none' });
      const appbar = document.querySelector('[data-page-title]'); if (appbar) appbar.textContent = title;
      document.querySelectorAll('.nav-rail [data-nav]').forEach(function (nav) {
        const selected = nav.dataset.nav === parent[0]; nav.classList.toggle('active', selected);
        if (selected) nav.setAttribute('aria-current', 'page'); else nav.removeAttribute('aria-current');
      });
    }
    window.addEventListener('hashchange', sync);
    window.addEventListener('popstate', sync);
    window.addEventListener('further:shell-ready', sync);
    window.addEventListener('further-ui:page', function (event) {
      if (event.detail?.name === 'workspace' || event.detail?.name === 'native') sync();
    });
    return sync;
  }

  document.addEventListener('further:ready', function () {
    const rail = document.querySelector('.nav-rail');
    const area = document.querySelector('.content-area');
    const page = document.body.dataset.page;
    if (!rail || !area || (page !== 'admin' && page !== 'console')) return;
    if (document.body.dataset.furtherInlineMounted === 'true') return;
    document.body.dataset.furtherInlineMounted = 'true';
    if (page === 'admin') prepareSettingsEntries(area);
    if (page === 'console') migrateConsoleDirectory(false);
    if (migrateAccountSecurityHash(page)) return;
    migrateOldGatewayHash(page);
    migrateUnifiedFeatureHash(page, false);
    installInlineStyles();
    const syncNative = page === 'admin' ? mountDirectWorkspace(area) : function () { migrateConsoleDirectory(true); };
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
      function showRiskWorkspace() { if (!panel.querySelector('[data-native-feature-path]')) panel.append(createFeatureHost('风险预警与反测活', '/risk-control')); }
      window.addEventListener('further-ui:page', function (event) { if (event.detail && event.detail.name === 'risk') showRiskWorkspace(); });
      if (location.hash === '#risk') showRiskWorkspace();
    }

    const syncPages = inlinePages[page].map(function (entry) { return mountInlinePage(area, entry); }).filter(Boolean);
    const showSettingsFrame = page === 'admin' ? mountSettingsPages(area) : function () {};
    window.addEventListener('further-ui:page', function () { syncPages.forEach(function (sync) { sync(); }); });
    window.addEventListener('further:settings-subpage', function (event) { showSettingsFrame(event.detail && event.detail.name); });
    syncPages.forEach(function (sync) { sync(); });
    syncNative();
    if (page === 'admin') {
      const parts = location.hash.slice(1).split('/');
      if (parts[0] === 'settings' && parts[1]) showSettingsFrame(parts[1]);
    }
  });
}());
