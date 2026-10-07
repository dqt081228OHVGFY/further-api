(function () {
  'use strict';
  const api = window.FurtherUI;
  const rows = document.querySelector('[data-channel-key-management-rows]');
  const refreshButton = document.querySelector('[data-refresh-channel-keys]');
  const balanceRefreshButton = document.querySelector('[data-refresh-key-balances]');
  const addButton = document.querySelector('[data-add-channel-key]');
  const form = document.querySelector('[data-channel-key-management-form]');
  const searchInput = document.querySelector('[data-key-search]');
  const channelFilter = document.querySelector('[data-key-channel-filter]');
  const statusFilter = document.querySelector('[data-key-status-filter]');
  let currentRole = 0;
  let items = [];
  let channels = [];
  let initialized = false;

  function usable(item) {
    return Number(item.status) === 1 && Number(item.channel_status) === 1 && Number(item.quota_remaining) !== 0;
  }

  function keyStatus(item) {
    if (Number(item.channel_status) !== 1) return '<span class="tag error">渠道不可用</span>';
    if (Number(item.status) === 3) return '<span class="tag error">自动下线</span>';
    if (Number(item.status) === 2) return '<span class="tag muted">手动停用</span>';
    if (Number(item.quota_remaining) === 0) return '<span class="tag error">余额耗尽</span>';
    return '<span class="tag success">可用</span>';
  }

  function quotaLabel(value) {
    return Number(value) < 0 ? '不限额' : api.moneyFromQuota(value, 2);
  }

  function balanceRefreshMessage(summary) {
    const checked = Number(summary && summary.checked) || 0;
    return '已尝试查询 ' + checked + ' 个渠道 · 更新 ' + (Number(summary.updated) || 0) + ' · 不支持 ' + (Number(summary.unavailable) || 0) + ' · 失败 ' + (Number(summary.failed) || 0);
  }

  function renderBalanceReports(summary) {
    let container = document.querySelector('[data-channel-balance-reports]');
    if (!container) { container = document.createElement('div'); container.dataset.channelBalanceReports = ''; container.className = 'md-card'; rows.before(container); }
    const reports = Array.isArray(summary && summary.reports) ? summary.reports : [];
    container.innerHTML = '<div class="card-head"><div class="md-card-title">渠道余额查询结果</div></div><p class="card-hint">余额来自上游渠道查询；不会自动修改独立 Key 配额或启停状态。</p>' + reports.map(function (report) {
      const balance = report.balance_usd == null ? '' : ' · $' + Number(report.balance_usd).toFixed(4);
      return '<div class="list-item"><div class="list-item-body"><b>' + api.escapeHTML(report.channel_name || ('渠道 #' + report.channel_id)) + balance + '</b><div class="list-item-sub">' + api.escapeHTML(report.message) + '</div></div></div>';
    }).join('');
  }

  function matchesStatus(item, filter) {
    if (!filter) return true;
    if (filter === 'usable') return usable(item);
    if (filter === 'exhausted') return Number(item.quota_remaining) === 0;
    if (filter === 'disabled') return Number(item.status) === 2;
    if (filter === 'auto_off') return Number(item.status) === 3;
    if (filter === 'channel_off') return Number(item.channel_status) !== 1;
    return true;
  }

  function visibleItems() {
    const query = searchInput.value.trim().toLowerCase();
    return items.filter(function (item) {
      if (channelFilter.value && String(item.channel_id) !== channelFilter.value) return false;
      if (!matchesStatus(item, statusFilter.value)) return false;
      if (!query) return true;
      return [item.channel_id, item.id, item.channel_name, item.label, item.masked_key].some(function (value) { return String(value || '').toLowerCase().includes(query); });
    });
  }

  function render() {
    document.querySelector('[data-key-total]').textContent = api.number(items.length);
    document.querySelector('[data-key-usable]').textContent = api.number(items.filter(usable).length);
    document.querySelector('[data-key-exhausted]').textContent = api.number(items.filter(function (item) { return Number(item.quota_remaining) === 0; }).length);
    document.querySelector('[data-key-disabled]').textContent = api.number(items.filter(function (item) { return Number(item.status) !== 1; }).length);
    const visible = visibleItems();
    if (!visible.length) {
      rows.innerHTML = '<div class="empty">没有符合条件的渠道 Key。</div>';
      return;
    }
    rows.innerHTML = visible.map(function (item) {
      const enabled = Number(item.status) === 1;
      const toggleTitle = enabled ? '停用 Key' : '启用 Key';
      const operatorAction = currentRole >= 100 ? '<button type="button" class="md-icon-button ' + (enabled ? 'danger' : '') + '" title="' + toggleTitle + '" data-set-key-status="' + Number(item.id) + '" data-channel-id="' + Number(item.channel_id) + '" data-next-status="' + (enabled ? 2 : 1) + '"><span class="material-symbol" aria-hidden="true">' + (enabled ? 'close' : 'check') + '</span></button>' : '';
      const balanceAction = enabled && Number(item.channel_status) === 1 ? '<button type="button" class="md-icon-button" title="查询上游渠道余额" data-refresh-key-balance="' + Number(item.id) + '" data-channel-id="' + Number(item.channel_id) + '"><span class="material-symbol" aria-hidden="true">account_balance_wallet</span></button>' : '';
      const rootActions = currentRole >= 100 ? '<button type="button" class="md-icon-button" title="编辑 Key" data-edit-key="' + Number(item.id) + '"><span class="material-symbol" aria-hidden="true">edit</span></button><button type="button" class="md-icon-button danger" title="删除 Key" data-delete-key="' + Number(item.id) + '" data-channel-id="' + Number(item.channel_id) + '"><span class="material-symbol" aria-hidden="true">delete</span></button>' : '';
      const iconClass = usable(item) ? 'usable' : 'unusable';
      const balanceCheck = item.balance_checked_at ? '<span>最近余额查询 ' + api.date(item.balance_checked_at) + '</span>' : '';
      const upstreamBalance = item.upstream_balance_usd == null ? '' : '<span>渠道余额 $' + Number(item.upstream_balance_usd).toFixed(4) + '</span>';
      const balanceError = item.balance_check_error ? '<span title="' + api.escapeHTML(item.balance_check_error) + '">复查异常：' + api.escapeHTML(item.balance_check_error) + '</span>' : '';
      const autoCheckTag = '';
      return '<article class="channel-key-management-row"><div class="channel-key-management-icon ' + iconClass + '"><span class="material-symbol" aria-hidden="true">key</span></div>' +
        '<div class="channel-key-management-main"><div class="channel-key-management-title"><b>' + api.escapeHTML(item.label || '未命名 Key') + '</b>' + keyStatus(item) + autoCheckTag + '</div>' +
        '<div class="disabled-channel-meta"><span class="tag">#' + Number(item.channel_id) + ' 渠道 · #' + Number(item.id) + ' Key</span><code>' + api.escapeHTML(item.masked_key || '') + '</code><span>' + api.escapeHTML(item.channel_name || '未命名渠道') + '</span><span>权重 ' + api.number(item.weight || 1) + '</span><span>失败 ' + api.number(item.failure_count) + ' 次</span><span>最后使用 ' + api.date(item.last_used_at) + '</span><span>最后错误 ' + api.date(item.last_error_at) + '</span>' + balanceCheck + upstreamBalance + balanceError + '</div></div>' +
        '<div class="channel-key-management-side"><div><b class="' + (Number(item.quota_remaining) === 0 ? 'quota-exhausted' : '') + '">' + quotaLabel(item.quota_remaining) + '</b><span>剩余额度</span></div><div class="channel-key-management-actions">' + balanceAction + operatorAction + rootActions + '</div></div></article>';
    }).join('');
  }

  function populateChannels() {
    const options = channels.map(function (channel) { return '<option value="' + Number(channel.id) + '">' + api.escapeHTML(channel.name || '未命名渠道') + '（渠道 ' + Number(channel.id) + '）</option>'; }).join('');
    channelFilter.innerHTML = '<option value="">全部渠道</option>' + options;
    form.elements.channel_id.innerHTML = options;
  }

  async function load() {
    api.showBusy(refreshButton, true);
    try {
      const result = await Promise.all([api.request('/api/admin/channel-keys'), api.request('/api/admin/channels')]);
      items = Array.isArray(result[0]) ? result[0] : [];
      channels = Array.isArray(result[1]) ? result[1] : [];
      populateChannels();
      render();
    } catch (error) {
      rows.innerHTML = '<div class="empty">' + api.escapeHTML(error.message || '加载失败') + '</div>';
      api.toast(error.message || '加载渠道 Key 失败', 'danger');
    } finally {
      api.showBusy(refreshButton, false);
    }
  }

  function openForm(item) {
    form.reset();
    const editing = Boolean(item);
    document.querySelector('[data-channel-key-form-title]').textContent = editing ? '#' + Number(item.channel_id) + ' 渠道 · #' + Number(item.id) + ' Key' : '添加渠道 Key';
    form.elements.id.value = editing ? item.id : '';
    form.elements.channel_id.disabled = editing;
    form.elements.channel_id.value = editing ? item.channel_id : (channels[0] ? channels[0].id : '');
    form.elements.label.value = editing ? (item.label || '') : '';
    form.elements.key.value = '';
    form.elements.key.required = !editing;
    form.elements.weight.value = editing ? (item.weight || 1) : 1;
    form.elements.quota_total.value = editing ? item.quota_total : -1;
    form.elements.quota_remaining.value = editing ? item.quota_remaining : -1;
    form.elements.status.value = editing ? item.status : 1;
    api.syncQuotaUSD(form);
    api.openDialog('channel-key-management-dialog');
  }

  rows.addEventListener('click', async function (event) {
    const balanceButton = event.target.closest('[data-refresh-key-balance]');
    if (balanceButton) {
      api.showBusy(balanceButton, true);
      try {
        const summary = await api.request('/api/admin/channels/' + balanceButton.dataset.channelId + '/keys/' + balanceButton.dataset.refreshKeyBalance + '/refresh-balance', { method: 'POST' });
        api.toast(balanceRefreshMessage(summary));
        renderBalanceReports(summary);
        await load();
      } catch (error) { api.toast(error.message || '余额复查失败', 'danger'); api.showBusy(balanceButton, false); }
      return;
    }
    const statusButton = event.target.closest('[data-set-key-status]');
    if (statusButton) {
      const next = Number(statusButton.dataset.nextStatus);
      if (next === 2 && !(await api.confirm('停用后，该凭据将立即退出上游选路。', { title: '停用渠道 Key', confirmText: '确认停用' }))) return;
      api.showBusy(statusButton, true);
      try {
        await api.request('/api/admin/channels/' + statusButton.dataset.channelId + '/keys/' + statusButton.dataset.setKeyStatus + '/status', { method: 'PATCH', body: { status: next } });
        api.toast(next === 1 ? '渠道 Key 已启用' : '渠道 Key 已停用');
        await load();
      } catch (error) { api.toast(error.message || '更新状态失败', 'danger'); api.showBusy(statusButton, false); }
      return;
    }
    const editButton = event.target.closest('[data-edit-key]');
    if (editButton) {
      const item = items.find(function (candidate) { return String(candidate.id) === editButton.dataset.editKey; });
      if (item) openForm(item);
      return;
    }
    const deleteButton = event.target.closest('[data-delete-key]');
    if (deleteButton) {
      if (!(await api.confirm('删除后该渠道 Key 将立即失效且无法恢复。', { title: '删除渠道 Key', confirmText: '删除' }))) return;
      api.showBusy(deleteButton, true);
      try {
        await api.request('/api/admin/channels/' + deleteButton.dataset.channelId + '/keys/' + deleteButton.dataset.deleteKey, { method: 'DELETE' });
        api.toast('渠道 Key 已删除');
        await load();
      } catch (error) { api.toast(error.message || '删除失败', 'danger'); api.showBusy(deleteButton, false); }
    }
  });

  form.addEventListener('submit', async function (event) {
    event.preventDefault();
    const usdLimits = form.querySelectorAll('[data-quota-usd-for="quota_total"], [data-quota-usd-for="quota_remaining"]');
    for (const input of usdLimits) {
      const value = Number(input.value);
      if (!Number.isFinite(value) || (value < 0 && value !== -1)) {
        input.focus();
        api.toast('额度请填非负美元金额，或填 -1 表示不限额', 'danger');
        return;
      }
    }
    const submit = form.querySelector('[type="submit"]');
    const editing = Boolean(form.elements.id.value);
    const channelID = Number(form.elements.channel_id.value);
    const payload = {
      label: form.elements.label.value.trim(), key: form.elements.key.value.trim(), weight: Number(form.elements.weight.value) || 1,
      quota_total: Number(form.elements.quota_total.value), quota_remaining: Number(form.elements.quota_remaining.value), status: Number(form.elements.status.value)
    };
    api.showBusy(submit, true);
    try {
      const path = editing ? '/api/admin/channels/' + channelID + '/keys/' + form.elements.id.value : '/api/admin/channels/' + channelID + '/keys';
      await api.request(path, { method: editing ? 'PUT' : 'POST', body: payload });
      api.closeDialog('channel-key-management-dialog');
      api.toast(editing ? '渠道 Key 已更新' : '渠道 Key 已添加');
      await load();
    } catch (error) { api.toast(error.message || '保存失败', 'danger'); }
    finally { api.showBusy(submit, false); }
  });

  refreshButton.addEventListener('click', load);
  balanceRefreshButton.addEventListener('click', async function () {
    api.showBusy(balanceRefreshButton, true);
    try {
      const summary = await api.request('/api/admin/channel-keys/refresh-balances', { method: 'POST' });
      api.toast(balanceRefreshMessage(summary));
      renderBalanceReports(summary);
      await load();
    } catch (error) { api.toast(error.message || '余额复查失败', 'danger'); }
    finally { api.showBusy(balanceRefreshButton, false); }
  });
  addButton.addEventListener('click', function () { openForm(null); });
  [searchInput, channelFilter, statusFilter].forEach(function (control) { control.addEventListener(control === searchInput ? 'input' : 'change', render); });

  async function init(options) {
    currentRole = Number(options && options.role) || 0;
    if (currentRole < 50) return;
    document.querySelectorAll('[data-root-only], [data-mobile-nav="settings"]').forEach(function (element) { element.classList.toggle('hidden', currentRole < 100); });
    addButton.classList.toggle('hidden', currentRole < 100);
    if (!initialized) {
      initialized = true;
      api.bindQuotaUSDFields(form);
    }
    await load();
  }

  window.FurtherUIChannelKeys = { init: init, load: load };

  document.addEventListener('further:ready', async function () {
    if (!document.body.classList.contains('channel-keys-page')) return;
    api.mountStandaloneAdminShell('channels');
    if (!api.token()) { location.href = '/auth/?mode=login'; return; }
    try {
      const self = await api.request('/api/user/self');
      const role = Number(self.role) || 0;
      if (role < 50) { location.href = '/admin/#channels'; return; }
      await init({ role: role });
    } catch (_) {}
  });
}());
