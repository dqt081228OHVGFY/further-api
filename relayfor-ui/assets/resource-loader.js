(function () {
  'use strict';

  // Share in-flight and successful initial loads, but allow failed loads to retry.
  function create(loaders) {
    const requests = new Map();
    function ensure(name) {
      if (!requests.has(name)) {
        const request = Promise.resolve().then(function () {
          if (typeof loaders[name] !== 'function') throw new Error('Unknown resource: ' + name);
          return loaders[name]();
        }).catch(function (error) {
          requests.delete(name);
          throw error;
        });
        requests.set(name, request);
      }
      return requests.get(name);
    }
    async function load(names) {
      const results = await Promise.allSettled(names.map(ensure));
      const failure = results.find(function (result) { return result.status === 'rejected'; });
      if (failure) throw failure.reason;
    }
    return { ensure: ensure, load: load };
  }

  function mount(resources, select) {
    const pages = new Map();
    const blockedContent = new WeakMap();
    function activate(name) {
      const panel = document.querySelector('[data-panel="' + CSS.escape(name) + '"]');
      if (!panel || pages.has(name)) return;
      const previousStatus = panel.querySelector('[data-page-load-status]');
      if (previousStatus) previousStatus.remove();
      const status = document.createElement('div');
      status.dataset.pageLoadStatus = '';
      status.className = 'field-hint';
      status.setAttribute('role', 'status');
      status.textContent = '正在加载本页数据…';
      panel.prepend(status);
      panel.setAttribute('aria-busy', 'true');
      panel.inert = true;
      const pending = resources.load(select(name)).then(function () {
        Array.from(panel.children).forEach(function (child) {
          if (blockedContent.has(child)) {
            child.inert = blockedContent.get(child);
            blockedContent.delete(child);
          }
        });
        status.remove();
      }).catch(function (error) {
        pages.delete(name);
        // Keep incomplete forms unavailable, while leaving the retry action usable.
        Array.from(panel.children).forEach(function (child) {
          if (child === status) return;
          if (!blockedContent.has(child)) blockedContent.set(child, child.inert);
          child.inert = true;
        });
        status.textContent = '部分数据加载失败：' + (error.message || '请重试') + ' ';
        const retry = document.createElement('button');
        retry.type = 'button';
        retry.className = 'md-btn md-btn-text';
        retry.textContent = '重试';
        retry.addEventListener('click', function () { status.remove(); activate(name); });
        status.append(retry);
      }).finally(function () {
        panel.inert = false;
        panel.removeAttribute('aria-busy');
      });
      pages.set(name, pending);
    }
    window.addEventListener('proxylan:page', function (event) { activate(event.detail.name); });
    const active = document.querySelector('[data-panel].active');
    if (active) activate(active.dataset.panel);
    return activate;
  }

  window.ProxyLanResources = { create: create, mount: mount };
}());
