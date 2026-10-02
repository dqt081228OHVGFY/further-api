(function () {
  'use strict';
  document.addEventListener('relayfor:ready', function () {
    const api = window.ProxyLan;
    const bar = document.querySelector('.top-app-bar');
    if (!bar || !api) return;
    let previousPage = (location.hash.slice(1).split('/')[0] || 'overview');
    window.addEventListener('proxylan:page', function (event) {
      const name = event.detail && event.detail.name;
      if (name && name !== previousPage) {
        const main = document.querySelector('.main-content');
        if (main) main.scrollTop = 0;
        previousPage = name;
      }
    });
    if (Number(document.body.dataset.gatewayRole) < 100) document.querySelectorAll('.workspace-welcome [data-open-settings]').forEach(function (button) { button.classList.add('hidden'); });
    const search = document.createElement('button');
    search.type = 'button'; search.className = 'workspace-search'; search.setAttribute('aria-label', '搜索页面');
    search.innerHTML = '<span class="material-symbol" aria-hidden="true">search</span><span>搜索页面</span><kbd>Ctrl K</kbd>';
    const spacer = bar.querySelector('.appbar-spacer');
    if (spacer) spacer.after(search); else bar.append(search);
    const context = document.createElement('div'); context.className = 'appbar-context';
    const dot = document.createElement('span'); dot.className = 'dot'; context.append(dot);
    const date = document.createElement('span'); date.textContent = new Intl.DateTimeFormat('zh-CN', { month:'long', day:'numeric', timeZone:'Asia/Shanghai' }).format(new Date()); context.append(date); search.before(context);
    const dialog = document.createElement('div'); dialog.className = 'dialog-backdrop'; dialog.id = 'workspace-jump';
    dialog.innerHTML = '<div class="md-dialog workspace-jump-dialog" role="dialog" aria-modal="true" aria-labelledby="workspace-jump-title"><div class="card-head"><h2 class="md-dialog-title" id="workspace-jump-title">前往页面</h2><button class="md-icon-button" type="button" data-close-jump aria-label="关闭"><span class="material-symbol" aria-hidden="true">close</span></button></div><input class="workspace-jump-input" type="search" placeholder="输入页面名称…" aria-label="搜索页面"><div class="workspace-jump-results"></div></div>';
    document.body.append(dialog);
    const input = dialog.querySelector('input'); const results = dialog.querySelector('.workspace-jump-results');
    function render() {
      results.replaceChildren(); const term = input.value.trim().toLowerCase();
      const entries = [...document.querySelectorAll('.nav-rail [data-nav]')].filter(function (nav) { return !nav.hidden && !nav.classList.contains('hidden') && nav.textContent.toLowerCase().includes(term); });
      entries.forEach(function (nav) {
        const button = document.createElement('button'); button.type = 'button';
        const icon = nav.querySelector('.material-symbol'); if (icon) button.append(icon.cloneNode(true));
        const label = document.createElement('span'); label.textContent = nav.textContent.replace(icon ? icon.textContent : '', '').trim(); button.append(label);
        button.addEventListener('click', function () { api.closeDialog(dialog.id); nav.click(); }); results.append(button);
      });
      if (!entries.length) { const empty = document.createElement('p'); empty.className = 'empty'; empty.textContent = '没有匹配的页面'; results.append(empty); }
    }
    function open() { input.value = ''; render(); api.openDialog(dialog.id); setTimeout(function () { input.focus(); }, 80); }
    search.addEventListener('click', open); input.addEventListener('input', render);
    dialog.querySelector('[data-close-jump]').addEventListener('click', function () { api.closeDialog(dialog.id); });
    dialog.addEventListener('click', function (event) { if (event.target === dialog) api.closeDialog(dialog.id); });
    document.addEventListener('keydown', function (event) { if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'k') { event.preventDefault(); open(); } });
  });
}());
