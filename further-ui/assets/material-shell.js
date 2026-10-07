(function () {
  'use strict';
  function mountMaterialShell() {
    const api = window.FurtherUI;
    const bar = document.querySelector('.top-app-bar');
    if (!bar || !api || bar.querySelector('.workspace-search')) return;
    const role = Number(document.body.dataset.gatewayRole || 0);
    if (role < 100) document.querySelectorAll('.workspace-welcome [data-open-settings]').forEach(function (button) { button.classList.add('hidden'); });
    const search = document.createElement('button');
    search.type = 'button'; search.className = 'workspace-search'; search.setAttribute('aria-label', '搜索页面');
    search.setAttribute('aria-expanded', 'false'); search.setAttribute('aria-controls', 'workspace-jump');
    search.innerHTML = '<span class="material-symbol" aria-hidden="true">search</span><span>搜索页面</span><kbd>Ctrl K</kbd>';
    const spacer = bar.querySelector('.appbar-spacer'); if (spacer) spacer.after(search); else bar.append(search);
    const context = document.createElement('div'); context.className = 'appbar-context';
    const dot = document.createElement('span'); dot.className = 'dot';
    const date = document.createElement('span'); date.textContent = new Intl.DateTimeFormat('zh-CN', { month:'long', day:'numeric', timeZone:'Asia/Shanghai' }).format(new Date());
    context.append(dot, date); search.before(context);
    const dialog = document.createElement('div'); dialog.className = 'dialog-backdrop'; dialog.id = 'workspace-jump';
    dialog.innerHTML = '<div class="md-dialog workspace-jump-dialog" role="dialog" aria-modal="true" aria-labelledby="workspace-jump-title"><div class="card-head"><div><h2 class="md-dialog-title" id="workspace-jump-title">前往页面</h2><p class="card-hint">输入页面或配置名称，快速切换工作区。</p></div><button class="md-icon-button" type="button" data-close-jump aria-label="关闭"><span class="material-symbol" aria-hidden="true">close</span></button></div><input class="workspace-jump-input" type="search" placeholder="搜索页面和配置…" aria-label="搜索页面" autocomplete="off"><div class="workspace-jump-results" aria-label="匹配页面"></div></div>';
    document.body.append(dialog);
    const input = dialog.querySelector('input'); const results = dialog.querySelector('.workspace-jump-results');
    function featureTarget(path) {
      const target = window.FurtherGateway.workspaceShellTarget(path, role);
      if (document.body.dataset.page === 'admin') {
        const settings = [...document.querySelectorAll('[data-settings-feature-path]')].find(function (node) { return node.dataset.settingsFeaturePath === path; });
        if (settings && role >= 100) return '/admin/#settings/' + settings.dataset.settingsPage;
      }
      return target;
    }
    function go(target) {
      const destination = new URL(target, location.origin);
      if (destination.pathname.replace(/\/+$/, '') === location.pathname.replace(/\/+$/, '')) location.hash = destination.hash;
      else location.assign(destination.href);
    }
    function choose(action) {
      api.closeDialog(dialog.id);
      action();
    }
    function render() {
      results.replaceChildren(); const term = input.value.trim().toLowerCase();
      const choices = new Map();
      const shell = '/' + document.body.dataset.page + '/';
      [...document.querySelectorAll('.nav-rail [data-nav]')].filter(function (nav) { return !nav.hidden && !nav.classList.contains('hidden'); }).forEach(function (nav) {
        const icon = nav.querySelector('.material-symbol'); const title = nav.textContent.replace(icon?.textContent || '', '').trim();
        if (!title) return;
        const target = shell + '#' + nav.dataset.nav;
        if (!choices.has(target)) choices.set(target, { title:title, icon:icon, matches:title.toLowerCase().includes(term), action:function () { if (!api.navigatePage?.(nav.dataset.nav)) nav.click(); } });
      });
      if (term) (window.FurtherFeatures?.entries || []).forEach(function (entry) {
        if (role < (entry.role || 1) || ![entry.title].concat(entry.aliases || []).some(function (label) { return label.toLowerCase().includes(term); })) return;
        const target = featureTarget(entry.path);
        // Subtabs are distinct only when they add a different capability.
        // A root navigation item owns every alias of its main feature.
        const rootTarget = target.replace(/#(?:subscriptions)\/paid$/, '#subscriptions');
        const existing = choices.get(rootTarget) || choices.get(target);
        if (existing) existing.matches = true;
        else choices.set(target, { title:entry.title, matches:true, action:function () { go(target); } });
      });
      const matches = [...choices.values()].filter(function (entry) { return entry.matches; });
      matches.slice(0, 16).forEach(function (entry, index) {
        const button = document.createElement('button'); button.type = 'button'; button.style.setProperty('--jump-delay', Math.min(index, 5) * 22 + 'ms');
        if (entry.icon) button.append(entry.icon.cloneNode(true));
        const label = document.createElement('span'); label.textContent = entry.title; button.append(label);
        button.addEventListener('click', function () { choose(entry.action); }); results.append(button);
      });
      if (!matches.length) { const empty = document.createElement('p'); empty.className = 'empty'; empty.textContent = '没有匹配的页面'; results.append(empty); }
    }
    function open() {
      input.value = ''; render(); search.setAttribute('aria-expanded', 'true');
      dialog._onClose = function () { search.setAttribute('aria-expanded', 'false'); };
      api.openDialog(dialog.id, { focusTarget:input });
    }
    search.addEventListener('click', open); input.addEventListener('input', render);
    input.addEventListener('keydown', function (event) {
      const first = results.querySelector('button');
      if ((event.key === 'ArrowDown' || event.key === 'Enter') && first) { event.preventDefault(); if (event.key === 'Enter') first.click(); else first.focus(); }
    });
    results.addEventListener('keydown', function (event) {
      if (!['ArrowDown', 'ArrowUp'].includes(event.key)) return;
      const buttons = [...results.querySelectorAll('button')]; const index = buttons.indexOf(document.activeElement);
      if (index < 0) return; event.preventDefault(); buttons[(index + (event.key === 'ArrowDown' ? 1 : buttons.length - 1)) % buttons.length].focus();
    });
    dialog.querySelector('[data-close-jump]').addEventListener('click', function () { api.closeDialog(dialog.id); });
    dialog.addEventListener('click', function (event) { if (event.target === dialog) api.closeDialog(dialog.id); });
    document.addEventListener('keydown', function (event) { if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'k') { event.preventDefault(); open(); } });
  }
  document.addEventListener('further:ready', mountMaterialShell);
  if (document.body.dataset.gatewayReady === 'true') mountMaterialShell();
}());
