(function () {
  'use strict';
  const api = window.ProxyLan;
  const rows = document.querySelector('[data-disabled-channel-rows]');
  const refreshButton = document.querySelector('[data-refresh-disabled-channels]');
  let currentRole = 0;
  let initialized = false;

  function errorSummary(item) {
    if (!item.last_error_at) return '最近错误：暂无记录';
    const status = Number(item.last_error_status) > 0 ? 'HTTP ' + Number(item.last_error_status) : '无状态码';
    const code = item.last_error_code ? ' · ' + api.escapeHTML(item.last_error_code) : '';
    return '最近错误：' + status + code + ' · ' + api.date(item.last_error_at);
  }

  function probeSummary(item) {
    if (!item.last_probe_at) return '最近探测：暂无记录';
    const result = item.last_probe_success === true ? '成功' : '失败';
    const status = Number(item.last_probe_status) > 0 ? ' · HTTP ' + Number(item.last_probe_status) : '';
    const message = item.last_probe_error ? ' · ' + api.escapeHTML(item.last_probe_error) : '';
    return '最近探测：' + result + status + message + ' · ' + api.date(item.last_probe_at);
  }

  function render(items) {
    const list = Array.isArray(items) ? items : [];
    const automatic = list.filter(function (item) { return Number(item.status) === 3; }).length;
    document.querySelector('[data-disabled-total]').textContent = api.number(list.length);
    document.querySelector('[data-disabled-auto]').textContent = api.number(automatic);
    document.querySelector('[data-disabled-manual]').textContent = api.number(list.length - automatic);
    if (!list.length) {
      rows.innerHTML = '<div class="empty"><span class="material-symbol" aria-hidden="true">check_circle</span><div>目前没有被禁用的渠道</div></div>';
      return;
    }
    rows.innerHTML = list.map(function (item) {
      const automaticOff = Number(item.status) === 3;
      const status = automaticOff ? '<span class="tag error">自动下线</span>' : '<span class="tag muted">手动禁用</span>';
      const restore = currentRole >= 50 ? '<button type="button" class="md-btn md-btn-filled" data-restore-channel="' + Number(item.id) + '"><span class="material-symbol" aria-hidden="true">check</span>恢复渠道</button>' : '';
      return '<article class="disabled-channel-row">' +
        '<div class="disabled-channel-icon ' + (automaticOff ? '' : 'manual') + '"><span class="material-symbol" aria-hidden="true">block</span></div>' +
        '<div class="disabled-channel-main"><div class="disabled-channel-title"><b>#' + Number(item.id) + ' 渠道</b>' + status + '</div>' +
        '<div class="disabled-channel-url">' + api.escapeHTML(item.base_url || '未配置上游地址') + '</div>' +
        '<div class="disabled-channel-reason">' + api.escapeHTML(item.reason || '未记录禁用原因') + '</div>' +
        '<div class="disabled-channel-meta"><span>错误计数 ' + api.number(item.failure_count) + '</span><span>不可用 Key ' + api.number(item.disabled_key_count) + '</span><span>' + errorSummary(item) + '</span><span>' + probeSummary(item) + '</span></div></div>' +
        '<div class="disabled-channel-side"><span class="disabled-channel-time">禁用于 ' + api.date(item.disabled_at) + '</span>' + restore + '</div></article>';
    }).join('');
  }

  async function load() {
    api.showBusy(refreshButton, true);
    try {
      render(await api.request('/api/admin/channels/disabled'));
    } catch (error) {
      rows.innerHTML = '<div class="empty">' + api.escapeHTML(error.message || '加载失败') + '</div>';
      api.toast(error.message || '加载已禁用渠道失败', 'danger');
    } finally {
      api.showBusy(refreshButton, false);
    }
  }

  rows.addEventListener('click', async function (event) {
    const button = event.target.closest('[data-restore-channel]');
    if (!button) return;
    if (!(await api.confirm('恢复后，新请求将可以再次选择该渠道，并清零渠道与自动下线 Key 的错误计数。', { title: '恢复渠道', confirmText: '确认恢复' }))) return;
    api.showBusy(button, true);
    try {
      await api.request('/api/admin/channels/' + button.dataset.restoreChannel + '/operation', { method: 'POST', body: { action: 'restore' } });
      api.toast('渠道已恢复');
      await load();
    } catch (error) {
      api.toast(error.message || '恢复渠道失败', 'danger');
      api.showBusy(button, false);
    }
  });

  refreshButton.addEventListener('click', load);

  async function init(options) {
    currentRole = Number(options && options.role) || 0;
    if (currentRole < 25) return;
    document.querySelectorAll('[data-root-only], [data-mobile-nav="settings"]').forEach(function (element) { element.classList.toggle('hidden', currentRole < 100); });
    initialized = true;
    await load();
  }

  window.ProxyLanDisabledChannels = { init: init, load: load, initialized: function () { return initialized; } };

  document.addEventListener('relayfor:ready', async function () {
    if (!document.body.classList.contains('disabled-channels-page')) return;
    api.mountStandaloneAdminShell('channels');
    if (!api.token()) { location.href = '/auth/?mode=login'; return; }
    try {
      const self = await api.request('/api/user/self');
      const role = Number(self.role) || 0;
      if (role < 25) { location.href = '/console/'; return; }
      await init({ role: role });
    } catch (_) {}
  });
}());
