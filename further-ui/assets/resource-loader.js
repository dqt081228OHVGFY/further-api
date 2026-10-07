(function () {
  'use strict';
  const loads = new Map();
  const failures = new Map();
  const mounts = new WeakMap();
  let indicator = null;
  let leaveTimer = 0;
  function renderIndicator() {
    const panel = document.querySelector('[data-panel].active');
    const name = panel?.dataset.panel;
    const count = loads.get(name) || 0;
    const failure = failures.get(name);
    if (!count && !failure) {
      if (indicator && !leaveTimer) {
        indicator.classList.add('is-leaving');
        leaveTimer = window.setTimeout(function () { indicator?.remove(); indicator = null; leaveTimer = 0; }, 180);
      }
      return;
    }
    window.clearTimeout(leaveTimer); leaveTimer = 0;
    const main = document.querySelector('.main-content');
    if (!main) return;
    if (!indicator?.isConnected) {
      indicator = document.createElement('div');
      indicator.className = 'workspace-page-load-status';
      indicator.dataset.pageLoadStatus = '';
      indicator.setAttribute('aria-live', 'polite');
      (document.querySelector('.top-app-bar') || main).append(indicator);
    }
    indicator.classList.remove('is-leaving');
    indicator.dataset.loading = String(count > 0);
    indicator.dataset.page = name;
    indicator.setAttribute('role', count ? 'status' : 'alert');
    indicator.replaceChildren();
    const label = document.createElement('span');
    label.textContent = count ? (panel.dataset.pageLoadState === 'initial' ? '正在载入此页…' : '正在更新此页…') : failure.message;
    indicator.append(label);
    if (!count && failure) {
      const retry = document.createElement('button'); retry.type = 'button'; retry.className = 'md-btn md-btn-text'; retry.textContent = '重试';
      retry.addEventListener('click', failure.retry); indicator.append(retry);
    }
  }
  function track(name, request) {
    const pending = Promise.resolve(request);
    loads.set(name, (loads.get(name) || 0) + 1);
    renderIndicator();
    pending.finally(function () {
      const remaining = (loads.get(name) || 1) - 1;
      if (remaining) loads.set(name, remaining); else loads.delete(name);
      renderIndicator();
    }).catch(function () {});
    return pending;
  }
  function create(loaders) {
    const requests = new Map();
    const inFlight = new Set();
    function ensure(name) {
      if (!requests.has(name)) {
        inFlight.add(name);
        const request = Promise.resolve().then(function () {
          if (typeof loaders[name] !== 'function') throw new Error('Unknown resource: ' + name);
          return loaders[name]();
        }).catch(function (error) { requests.delete(name); throw error; }).finally(function () { inFlight.delete(name); });
        requests.set(name, request);
      }
      return requests.get(name);
    }
    async function load(names) {
      const results = await Promise.allSettled([...new Set(names)].map(ensure));
      const failure = results.find(function (result) { return result.status === 'rejected'; });
      if (failure) throw failure.reason;
    }
    function refresh(names) {
      names.forEach(function (name) { if (!inFlight.has(name)) requests.delete(name); });
      return load(names);
    }
    return { ensure: ensure, load: load, refresh: refresh };
  }
  function nativeVisible(panel) {
    return Array.from(panel.querySelectorAll('[data-native-feature-path],.further-native-feature-host')).some(function (host) { return !host.closest('[hidden],.hidden'); });
  }
  function mount(resources, select) {
    if (mounts.has(resources)) return mounts.get(resources);
    const scope = document.querySelector('[data-shell]') || document.querySelector('.main-content') || document.body;
    const pages = new Map();
    let selected = '';
    function activate(name, force) {
      if (!scope.isConnected) return Promise.resolve();
      const panel = document.querySelector('[data-panel="' + CSS.escape(name || '') + '"]');
      if (!panel?.classList.contains('active')) return Promise.resolve();
      if (!force && selected === name) return pages.get(name)?.pending || Promise.resolve();
      selected = name;
      const state = pages.get(name) || { ready: false, pending: null };
      pages.set(name, state);
      renderIndicator();
      if (state.pending) return state.pending;
      const names = nativeVisible(panel) && name !== 'profile' || name === 'subscriptions' && location.hash.endsWith('/paid') ? [] : select(name);
      if (!names.length) { state.ready = true; return Promise.resolve(); }
      failures.delete(name);
      panel.dataset.pageLoadState = state.ready ? 'refreshing' : 'initial';
      panel.setAttribute('aria-busy', 'true');
      const pending = Promise.resolve().then(function () {
        if (!force && !panel.classList.contains('active')) return false;
        return (state.ready || force ? resources.refresh(names) : resources.load(names)).then(function () { return true; });
      }).then(function (loaded) { if (loaded) state.ready = true; }).catch(function (error) {
        failures.set(name, { message: (state.ready ? '更新失败，已保留当前内容：' : '此页暂时无法载入：') + (error.message || '请重试'), retry: function () { activate(name, true); } });
      }).finally(function () {
        if (state.pending === pending) state.pending = null;
        panel.removeAttribute('aria-busy'); delete panel.dataset.pageLoadState;
        renderIndicator();
      });
      state.pending = pending;
      track(name, pending);
      return pending;
    }
    function receivePage(event) { activate(event.detail.name); }
    function receiveRefresh(event) {
      const name = event.detail.name;
      const panel = document.querySelector('[data-panel="' + CSS.escape(name) + '"].active');
      if (!scope.isConnected || !panel) return;
      if (nativeVisible(panel) && name !== 'profile') return;
      const wait = function (request) { event.detail.waitUntil(track(name, request)); };
      let names = select(name);
      if (name === 'vip' || (name === 'subscriptions' && location.hash.endsWith('/paid'))) {
        if (window.FurtherUIVIP) wait(window.FurtherUIVIP.load());
        names = [];
      }
      if (name === 'activities' && window.FurtherUIActivities) {
        if (panel.querySelector('[data-activity-dirty="true"]')) window.FurtherUI.toast('请先保存或撤销活动修改，再刷新本页。');
        else wait(window.FurtherUIActivities.load(true));
      }
      if (name === 'multi-user-stats' && window.FurtherUIMultiUserStats) wait(window.FurtherUIMultiUserStats.query());
      if (name === 'channels' && location.hash.endsWith('/disabled') && window.FurtherUIDisabledChannels) { wait(window.FurtherUIDisabledChannels.load()); names = []; }
      if (name === 'channels' && location.hash.endsWith('/keys') && window.FurtherUIChannelKeys) { wait(window.FurtherUIChannelKeys.load()); names = []; }
      if (names.length) event.detail.waitUntil(activate(name, true));
    }
    window.addEventListener('further-ui:page', receivePage);
    window.addEventListener('further-ui:refresh', receiveRefresh);
    activate.dispose = function () {
      window.removeEventListener('further-ui:page', receivePage);
      window.removeEventListener('further-ui:refresh', receiveRefresh);
      mounts.delete(resources);
    };
    mounts.set(resources, activate);
    const active = document.querySelector('[data-panel].active');
    if (active) activate(active.dataset.panel);
    return activate;
  }
  window.addEventListener('further-ui:page', renderIndicator);
  window.FurtherUIResources = { create: create, mount: mount, track: track };
}());
