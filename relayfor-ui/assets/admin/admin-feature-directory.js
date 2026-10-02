(function () {
  'use strict';
  const hiddenAdminGroups = new Set(['系统设置 · 内容', '系统设置 · 运维', '系统设置 · 计费', '系统设置 · 模型', '开发', '日志', '管理', '系统设置 · 站点', '账户']);

  function mountAdminFeatureDirectory(rail, area, helpers) {
    const createFeatureHost = helpers.createFeatureHost;
    const nav = document.createElement('button');
    nav.type = 'button'; nav.className = 'nav-item'; nav.dataset.nav = 'native';
    nav.innerHTML = '<span class="material-symbol" aria-hidden="true">apps</span>更多功能';
    rail.insertBefore(nav, rail.querySelector('.rail-bottom'));
    const panel = document.createElement('section');
    panel.className = 'view'; panel.dataset.panel = 'native'; panel.dataset.title = '更多功能';
    panel.dataset.subtitle = '账户、模型、运行与系统配置';
    const directory = document.createElement('div'); directory.className = 'relayfor-native-directory';
    const role = Number(document.body.dataset.gatewayRole || 0);
    const entries = (window.RelayForFeatures && window.RelayForFeatures.entries || []).filter(function (entry) {
      return role >= (entry.role || 1) && !hiddenAdminGroups.has(entry.group);
    });
    const groups = new Map();
    entries.forEach(function (entry) {
      if (!groups.has(entry.group)) {
        const section = document.createElement('section'); section.className = 'relayfor-feature-group';
        const heading = document.createElement('h2'); heading.textContent = entry.group; section.append(heading);
        const cards = document.createElement('div'); cards.className = 'relayfor-feature-grid'; section.append(cards);
        directory.append(section); groups.set(entry.group, cards);
      }
      const button = document.createElement('button'); button.type = 'button'; button.className = 'md-card settings-directory-item';
      button.textContent = entry.title;
      button.addEventListener('click', function () { history.replaceState(null, '', '#native/' + encodeURIComponent(entry.path)); nav.click(); });
      groups.get(entry.group).append(button);
    });
    const detail = document.createElement('div'); detail.hidden = true;
    const back = document.createElement('button'); back.type = 'button'; back.className = 'md-btn md-btn-text'; back.textContent = '返回功能目录';
    back.addEventListener('click', function () { history.replaceState(null, '', '#native'); nav.click(); });
    const host = document.createElement('div'); host.className = 'relayfor-inline-host'; detail.append(back, host);
    panel.append(directory, detail); area.append(panel);
    function sync() {
      if (helpers.migrate()) return;
      if (!location.hash.startsWith('#native')) return;
      let path = '';
      try { path = decodeURIComponent(location.hash.slice('#native/'.length)); } catch (_) {}
      if (location.hash === '#native') path = '';
      directory.hidden = Boolean(path); detail.hidden = !path;
      if (!path) {
        panel.dataset.title = '更多功能';
        const heading = document.querySelector('[data-page-title]'); if (heading) heading.textContent = '更多功能';
        return;
      }
      const entry = entries.find(function (item) { return item.path === path.split('?')[0]; });
      const title = entry ? entry.title : '功能设置';
      panel.dataset.title = title;
      const appbar = document.querySelector('[data-page-title]'); if (appbar) appbar.textContent = title;
      const existing = host.querySelector('[data-native-feature-path]');
      if (existing) {
        window.RelayForFeatures.mount(existing, { path: path }).catch(function (error) { window.ProxyLan.toast(error.message, 'danger'); });
      } else { host.replaceChildren(createFeatureHost(title, path)); }
    }
    window.addEventListener('proxylan:page', function (event) { if (event.detail && event.detail.name === 'native') sync(); });
    return sync;
  }

  window.ProxyLanAdminFeatureDirectory = { mount: mountAdminFeatureDirectory };
}());
