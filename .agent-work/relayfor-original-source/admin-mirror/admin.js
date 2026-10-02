(function () {
  'use strict';
  const api = window.ProxyLan;
  const adminOverview = window.ProxyLanAdminOverview;
  const adminSettings = window.ProxyLanAdminSettings;
  const adminModels = window.ProxyLanAdminModels;
  const userState = { page: 1, pageSize: 50, keyword: '', group: '', role: '', status: '', sleeping: false, totalPages: 0 };
  const logState = { page: 1, pageSize: 50, keyword: '', statusCode: '', exclude200: false, sortBy: 'created_at', sortOrder: 'desc', totalPages: 0 };
  const auditState = { page: 1, pageSize: 50, keyword: '', action: '', totalPages: 0 };
  const traceState = { page: 1, pageSize: 25, keyword: '', status: '', totalPages: 0 };
  const redemptionState = { page: 1, pageSize: 25, keyword: '', status: '', userGroup: '', totalPages: 0 };
  const registrationCodeState = { page: 1, pageSize: 25, keyword: '', status: '', userGroup: '', totalPages: 0 };
  const prepaidCardState = { page: 1, pageSize: 25, keyword: '', status: '', totalPages: 0 };
  let channelRows = [];
	let hideDisabledChannels = api.adminListPreference('hideDisabledChannels');
  let subscriptionPlans = [];
  let billingPromotions = [];
  let activeChannelID = 0;
  let activeChannelStatus = 0;
  let currentAdminRole = 0;
  let channelKeyManagementReady = false;
  let disabledChannelManagementReady = false;
  let adminSecurityStatus = { totp_enabled: false, pending: false };
	let activeTraceRequestID = '';
	let activeLogView = 'calls';
  const resources = window.ProxyLanResources.create({
    overview: loadOverview, channels: loadChannels, models: adminModels.load,
    subscriptions: loadSubscriptionPlans, benefits: loadSubscriptionBenefitCampaigns, promotions: loadBillingPromotions,
    users: loadUsers, redemptions: loadRedemptions, registrationCodes: loadRegistrationCodes,
    prepaidCards: loadPrepaidCards, logs: loadLogs, traces: loadTraces,
    settings: adminSettings.load, security: loadAdminSecurity, audit: loadAdminAudit
  });

  function resourcesForPage(name) {
    const pages = {
      overview: ['overview'], channels: ['channels'], models: ['models', 'channels'],
      subscriptions: ['subscriptions', 'benefits', 'promotions', 'models'], users: ['users'],
      redemptions: ['redemptions', 'subscriptions'], 'registration-codes': ['registrationCodes'],
      'prepaid-cards': ['prepaidCards'], logs: [activeLogView === 'traces' ? 'traces' : 'logs'],
      settings: currentAdminRole >= 100 ? ['settings', 'security'] : [],
      audit: currentAdminRole >= 100 ? ['audit'] : []
    };
    return pages[name] || [];
  }


  function adminRoleLabel(role) {
    return ({ 1: '用户', 25: '审计员', 50: '运维员', 100: '超级管理员' })[Number(role)] || '未知';
  }

  function applyAdminCapabilities() {
    const root = currentAdminRole >= 100;
    const operator = currentAdminRole >= 50;
    document.body.dataset.adminRole = root ? 'root' : (operator ? 'operator' : 'auditor');
    document.querySelectorAll('[data-root-only]').forEach(function (element) { element.classList.toggle('hidden', !root); });
    const rootSelectors = '[data-open-dialog], [data-edit-channel], [data-delete-channel], [data-edit-model], [data-delete-model], [data-edit-subscription-plan], [data-edit-billing-promotion], [data-delete-billing-promotion], [data-edit-user], [data-toggle-user], [data-delete-redemption], [data-delete-redemption-batch], [data-delete-registration-code], [data-delete-registration-code-batch], [data-delete-prepaid-card], [data-delete-prepaid-batch], [data-channel-key-add], [data-edit-channel-key], [data-delete-channel-key]';
    document.querySelectorAll(rootSelectors).forEach(function (element) { element.classList.toggle('hidden', !root); });
	const operatorSelectors = '[data-toggle-channel], [data-toggle-model], [data-toggle-subscription-plan], [data-toggle-billing-promotion], [data-manage-channel-keys], [data-channel-key-management-open], [data-probe-channel], [data-channel-operation], [data-process-subscription-resets]';
    document.querySelectorAll(operatorSelectors).forEach(function (element) { element.classList.toggle('hidden', !operator); });
    if (!root && (document.querySelector('[data-panel="settings"].active') || document.querySelector('[data-panel="audit"].active'))) {
      const overview = document.querySelector('[data-nav="overview"]');
      if (overview) overview.click();
    }
  }

  async function loadOverview() { return adminOverview.load(); }

  function channelSubpageFromHash() {
    const parts = location.hash.slice(1).split('/');
    if (parts[0] !== 'channels') return 'index';
    return parts[1] === 'keys' || parts[1] === 'disabled' ? parts[1] : 'index';
  }

  function showChannelSubpage(name, historyMode) {
    const page = name === 'keys' || name === 'disabled' ? name : 'index';
    document.querySelectorAll('[data-channel-page]').forEach(function (element) {
      element.classList.toggle('hidden', element.dataset.channelPage !== page);
    });
    const panel = document.querySelector('[data-panel="channels"]');
    if (panel && panel.classList.contains('active')) {
      const title = document.querySelector('[data-page-title]');
      const subtitle = document.querySelector('[data-page-subtitle]');
      if (title) title.textContent = page === 'keys' ? '渠道 Key 管理' : page === 'disabled' ? '禁用渠道' : '渠道';
      if (subtitle) subtitle.textContent = page === 'keys' ? '管理上游凭据与可用状态' : page === 'disabled' ? '查看并恢复不可用渠道' : panel.dataset.subtitle;
    }
    const target = page === 'index' ? '#channels' : '#channels/' + page;
    if (historyMode === 'push' && location.hash !== target) history.pushState(null, '', target);
    if (historyMode === 'replace' && location.hash !== target) history.replaceState(null, '', target);
  }

  async function ensureChannelKeyManagement() {
    if (channelKeyManagementReady || currentAdminRole < 50 || !window.ProxyLanChannelKeys) return;
    channelKeyManagementReady = true;
    try {
      await window.ProxyLanChannelKeys.init({ role: currentAdminRole });
    } catch (error) {
      channelKeyManagementReady = false;
      api.toast(error.message || '加载渠道 Key 失败', 'danger');
    }
  }

  async function ensureDisabledChannelManagement() {
    if (disabledChannelManagementReady || currentAdminRole < 25 || !window.ProxyLanDisabledChannels) return;
    disabledChannelManagementReady = true;
    try {
      await window.ProxyLanDisabledChannels.init({ role: currentAdminRole });
    } catch (error) {
      disabledChannelManagementReady = false;
      api.toast(error.message || '加载禁用渠道失败', 'danger');
    }
  }

  function ensureActiveChannelSubpage() {
    const page = channelSubpageFromHash();
    if (page === 'keys') ensureChannelKeyManagement();
    if (page === 'disabled') ensureDisabledChannelManagement();
  }

  function bindChannelSubpages() {
    const open = document.querySelector('[data-channel-key-management-open]');
    const back = document.querySelector('[data-channel-key-management-back]');
    const disabledOpen = document.querySelector('[data-disabled-channel-management-open]');
    const disabledBack = document.querySelector('[data-disabled-channel-management-back]');
    if (open) open.addEventListener('click', function () {
      showChannelSubpage('keys', 'push');
      ensureChannelKeyManagement();
    });
    if (back) back.addEventListener('click', function () { showChannelSubpage('index', 'push'); });
    if (disabledOpen) disabledOpen.addEventListener('click', function () {
      showChannelSubpage('disabled', 'push');
      ensureDisabledChannelManagement();
    });
    if (disabledBack) disabledBack.addEventListener('click', function () { showChannelSubpage('index', 'push'); });
    const channelNav = document.querySelector('.nav-item[data-nav="channels"]');
    if (channelNav) channelNav.addEventListener('click', function (event) {
      if (event.isTrusted) showChannelSubpage('index', 'replace');
    });
    const mobileChannelNav = document.querySelector('[data-mobile-nav="channels"]');
    if (mobileChannelNav) mobileChannelNav.addEventListener('click', function () { showChannelSubpage('index', 'replace'); });
    window.addEventListener('popstate', function () {
      showChannelSubpage(channelSubpageFromHash());
      ensureActiveChannelSubpage();
    });
    window.addEventListener('hashchange', function () {
      showChannelSubpage(channelSubpageFromHash());
      ensureActiveChannelSubpage();
    });
    showChannelSubpage(channelSubpageFromHash());
  }

	function formatTTFT(value) {
	  const milliseconds = Number(value || 0);
	  if (milliseconds <= 0) return '—';
	  return milliseconds < 1000 ? Math.round(milliseconds) + ' ms' : (milliseconds / 1000).toFixed(2) + ' s';
	}

	function channelHealthMarkup(channel) {
	  const health = channel.health || {};
	  const requests = Number(health.requests || 0);
	  const successRate = Number(health.success_rate || 0);
	  const successClass = requests === 0 ? 'empty' : (successRate >= 99 ? 'good' : (successRate >= 95 ? 'warn' : 'bad'));
	  const rateLimited = Number(health.rate_limited || 0);
	  const serverErrors = Number(health.server_errors || 0);
	  const status = Number(health.last_error_status || 0);
	  const errorCode = String(health.last_error_code || '').trim();
	  const errorParts = [];
	  if (status > 0) errorParts.push('HTTP ' + status);
	  if (errorCode) errorParts.push(errorCode);
	  const lastError = errorParts.length ? '<b>' + api.escapeHTML(errorParts.join(' · ')) + '</b><small>' + api.date(health.last_error_at) + '</small>' : '<b>暂无错误记录</b><small>该渠道尚未记录失败请求</small>';
	  return '<div class="channel-health-grid"><div class="channel-health-metric health-' + successClass + '"><span>成功率</span><b>' + (requests ? successRate.toFixed(1) + '%' : '—') + '</b><small>' + api.number(requests) + ' 个请求</small></div><div class="channel-health-metric' + (rateLimited ? ' health-warn' : '') + '"><span>429</span><b>' + api.number(rateLimited) + '</b><small>限流响应</small></div><div class="channel-health-metric' + (serverErrors ? ' health-bad' : '') + '"><span>5xx</span><b>' + api.number(serverErrors) + '</b><small>上游错误</small></div><div class="channel-health-metric"><span>平均首字</span><b>' + formatTTFT(health.average_ttft_ms) + '</b><small>TTFT</small></div><div class="channel-health-metric"><span>平均速度</span><b>' + (Number(health.average_tokens_per_second || 0) > 0 ? Number(health.average_tokens_per_second).toFixed(1) + ' tok/s' : '—') + '</b><small>生成吞吐</small></div><div class="channel-health-metric channel-last-error' + (errorParts.length ? ' health-bad' : '') + '"><span>最近错误</span>' + lastError + '</div></div>';
	}

  function channelRow(channel) {
    const protocol = Number(channel.type) === 2 ? 'Anthropic' : Number(channel.type) === 1 ? 'OpenAI' : 'Custom';
    const upstreamFormats = Array.isArray(channel.upstream_formats) && channel.upstream_formats.length ? channel.upstream_formats : [channel.upstream_format || (Number(channel.type) === 2 ? 'anthropic' : 'openai_chat')];
    const formatLabels = { openai_chat: 'OpenAI Chat', openai_responses: 'OpenAI Responses', anthropic: 'Anthropic Messages' };
    const upstreamFormat = upstreamFormats.map(function (format) { return formatLabels[format] || format; }).join(' + ');
	const capacity = (Number(channel.requests_per_minute) > 0 ? Number(channel.requests_per_minute) + ' RPM' : 'RPM 不限') + ' · ' + (Number(channel.concurrent_requests) > 0 ? Number(channel.concurrent_requests) + ' 并发' : '并发不限');
    const quota = Number(channel.quota_remaining) < 0 ? '不限额' : api.moneyFromQuota(channel.quota_remaining, 2);
	const probe = channel.last_probe;
	const probeTag = probe ? '<span class="tag ' + (probe.success ? 'success' : 'error') + '">探测 ' + (probe.success ? (Number(probe.latency_ms) + ' ms') : ('HTTP ' + (probe.status_code || '失败'))) + '</span>' : '<span class="tag muted">尚未探测</span>';
	const maintenance = channel.maintenance_note ? '<div class="list-item-sub">维护：' + api.escapeHTML(channel.maintenance_note) + ' · ' + api.date(channel.maintenance_at) + '</div>' : '';
	const operation = Number(channel.status) === 1 ? 'drain' : 'restore';
	const dataTrainTag = channel.data_train ? '<span class="tag error">data-train</span>' : '';
	const compactTag = channel.remote_compaction ? '<span class="tag success">远程压缩 v2</span>' : '';
    return '<div class="channel-row">' +
      '<span class="channel-radio"><span class="material-symbol" aria-hidden="true">hub</span></span>' +
	  '<div class="channel-body"><div class="channel-name">' + api.escapeHTML(channel.name) + '</div><div class="channel-target">' + api.escapeHTML(channel.base_url) + '</div><div class="tags"><span class="tag">' + protocol + '</span><span class="tag">' + upstreamFormat + '</span><span class="tag">' + capacity + '</span><span class="tag">优先级 ' + channel.priority + '</span><span class="tag">权重 ' + channel.weight + '</span><span class="tag">额度 ' + quota + '</span><span class="tag">Key ' + api.number(channel.key_count) + '</span><span class="tag muted">' + api.escapeHTML(channel.groups || 'default') + '</span>' + dataTrainTag + compactTag + probeTag + '</div>' + maintenance + channelHealthMarkup(channel) + '</div>' +
      '<div class="channel-actions"><span>' + api.statusLabel(channel.status, 'channel') + '</span><button type="button" class="md-icon-button" title="立即探测" data-probe-channel="' + channel.id + '"><span class="material-symbol" aria-hidden="true">monitoring</span></button><button type="button" class="md-icon-button" title="管理渠道 Key" data-manage-channel-keys="' + channel.id + '"><span class="material-symbol" aria-hidden="true">key</span></button><button type="button" class="md-icon-button" title="编辑" data-edit-channel="' + channel.id + '"><span class="material-symbol" aria-hidden="true">edit</span></button><button type="button" class="md-icon-button" title="' + (operation === 'drain' ? '摘除渠道' : '恢复渠道') + '" data-channel-operation="' + operation + '" data-channel-id="' + channel.id + '"><span class="material-symbol" aria-hidden="true">' + (operation === 'drain' ? 'close' : 'check') + '</span></button><button type="button" class="md-icon-button" title="清零故障计数" data-channel-operation="reset_failures" data-channel-id="' + channel.id + '"><span class="material-symbol" aria-hidden="true">refresh</span></button><button type="button" class="md-icon-button danger" title="删除" data-delete-channel="' + channel.id + '"><span class="material-symbol" aria-hidden="true">delete</span></button></div>' +
      '</div>';
  }

  async function loadChannels(refresh) {
	if (refresh !== false) {
	  const result = await api.request('/api/admin/channels');
	  channelRows = Array.isArray(result) ? result : [];
	  // 模型编辑器要用渠道列表填绑定下拉框，推给模型模块避免它重复请求一次
	  adminModels.setChannels(channelRows);
	}
	const rows = hideDisabledChannels ? channelRows.filter(function (channel) { return Number(channel.status) === 1; }) : channelRows;
    const body = document.querySelector('[data-admin-channel-rows]');
    if (!body) return;
	body.innerHTML = rows.length ? rows.map(channelRow).join('') : '<div class="empty">' + (channelRows.length ? '停用和自动下线的渠道已隐藏，可关闭筛选后查看。' : '还没有配置渠道。') + '</div>';
    body.querySelectorAll('[data-edit-channel]').forEach(function (button) {
      button.addEventListener('click', function () {
        const channel = rows.find(function (item) { return String(item.id) === button.dataset.editChannel; });
        const form = document.querySelector('[data-channel-form]');
        form.elements.id.value = channel.id;
        form.elements.name.value = channel.name || '';
        form.elements.type.value = channel.type;
        form.elements.upstream_format.value = channel.upstream_format || (Number(channel.type) === 2 ? 'anthropic' : 'openai_chat');
        const supportedFormats = Array.isArray(channel.upstream_formats) && channel.upstream_formats.length ? channel.upstream_formats : [form.elements.upstream_format.value];
        form.querySelectorAll('input[name="upstream_formats"]').forEach(function (input) { input.checked = supportedFormats.includes(input.value); });
        form.elements.priority.value = channel.priority;
        form.elements.weight.value = channel.weight;
        form.elements.groups.value = channel.groups || 'default';
		form.elements.data_train.checked = Boolean(channel.data_train);
		form.elements.remote_compaction.checked = Boolean(channel.remote_compaction);
        form.elements.base_url.value = channel.base_url || '';
        form.elements.key.value = channel.key || '';
        form.elements.models.value = channel.models || '';
        form.elements.model_mapping.value = channel.model_mapping || '';
        form.elements.quota_total.value = channel.quota_total == null ? -1 : channel.quota_total;
        form.elements.quota_remaining.value = channel.quota_remaining == null ? -1 : channel.quota_remaining;
		form.elements.requests_per_minute.value = channel.requests_per_minute || 0;
		form.elements.concurrent_requests.value = channel.concurrent_requests || 0;
        api.syncQuotaUSD(form);
        document.querySelector('#channel-dialog .md-dialog-title').textContent = '编辑渠道';
        api.openDialog('channel-dialog');
      });
    });
    body.querySelectorAll('[data-manage-channel-keys]').forEach(function (button) {
      button.addEventListener('click', function () {
        const channel = rows.find(function (item) { return String(item.id) === button.dataset.manageChannelKeys; });
        if (channel) openChannelKeyDialog(channel);
      });
    });
	body.querySelectorAll('[data-probe-channel]').forEach(function (button) {
	  button.addEventListener('click', async function () {
		api.showBusy(button, true);
		try {
		  const result = await api.request('/api/admin/channels/' + button.dataset.probeChannel + '/probe', { method: 'POST' });
		  api.toast(result.success ? '渠道探测成功 · ' + result.latency_ms + ' ms' : '渠道探测失败 · HTTP ' + (result.status_code || '网络错误'), result.success ? '' : 'danger');
		  await loadChannels();
		} catch (error) { api.toast(error.message, 'danger'); }
		finally { api.showBusy(button, false); }
	  });
	});
    body.querySelectorAll('[data-channel-operation]').forEach(function (button) {
      button.addEventListener('click', async function () {
		const action = button.dataset.channelOperation;
		if (action === 'drain' && !(await api.confirm('摘除后新请求不会再选择该渠道，当前请求不受影响。', { title: '摘除渠道', confirmText: '确认摘除' }))) return;
        try {
		  await api.request('/api/admin/channels/' + button.dataset.channelId + '/operation', { method: 'POST', body: { action: action } });
		  api.toast(action === 'drain' ? '渠道已摘除' : action === 'restore' ? '渠道已恢复' : '故障计数已清零');
          await loadChannels();
          await loadOverview();
        } catch (error) { api.toast(error.message, 'danger'); }
      });
    });
    body.querySelectorAll('[data-delete-channel]').forEach(function (button) {
      button.addEventListener('click', async function () {
        if (!(await api.confirm('删除渠道后无法恢复，关联配置也将失效。', { title: '删除渠道', confirmText: '删除' }))) return;
        try {
          await api.request('/api/admin/channels/' + button.dataset.deleteChannel, { method: 'DELETE' });
          api.toast('渠道已删除');
          await loadChannels();
          await loadOverview();
        } catch (error) { api.toast(error.message, 'danger'); }
      });
    });
  }


  function subscriptionPlanStatus(status) {
    return Number(status) === 1 ? '<span class="tag success">启用</span>' : '<span class="tag muted">停用</span>';
  }

  function subscriptionPlanRow(plan) {
    const quota = (plan.unlimited_quota ? '不限额' : api.moneyFromQuota(plan.quota, 2)) + (plan.quota_group ? ' · ' + api.escapeHTML(plan.quota_group) : ' · 通用');
    const modelCount = Array.isArray(plan.model_rules) ? plan.model_rules.length : 0;
    const upgrade = Number(plan.upgrade_from_plan_id) > 0 ? ' · ' + api.escapeHTML(plan.upgrade_from_plan_name) + ' 的升级计划' : '';
	const placement = plan.collapsed ? '<span class="tag">更多套餐 · ' + Number(plan.display_order || 0) + '</span>' : '<span class="tag muted">直接展示 · ' + Number(plan.display_order || 0) + '</span>';
    const rpm = Number(plan.requests_per_minute) > 0 ? Number(plan.requests_per_minute) : 15;
    const concurrent = Number(plan.concurrent_requests) > 0 ? Number(plan.concurrent_requests) : 2;
    const savingsLimit = Number(plan.discount_savings_limit) > 0 ? api.moneyFromQuota(plan.discount_savings_limit, 2) : '不限';
    const description = plan.description ? '<div class="list-item-sub">' + api.escapeHTML(plan.description) + '</div>' : '';
	const reset = Number(plan.reset_interval_days) > 0 ? '每 ' + Number(plan.reset_interval_days) + ' 天' : '不重置';
	return '<div class="list-item"><span class="material-symbol" aria-hidden="true">redeem</span><div class="list-item-body"><div class="list-item-title">' + api.escapeHTML(plan.name) + '</div><div class="list-item-sub"><code>' + api.escapeHTML(plan.code) + '</code> · 有效期 ' + Number(plan.duration_days) + ' 天 · 解锁 ' + modelCount + ' 个模型' + upgrade + '</div>' + description + '</div><div class="list-metrics"><div><b>' + quota + '</b>额度</div><div><b>' + reset + '</b>周期重置</div><div><b>×' + Number(plan.discount_multiplier || 1).toFixed(2) + '</b>折扣倍率</div><div><b>' + savingsLimit + '</b>最高节省</div><div><b>' + rpm + ' / ' + concurrent + '</b>RPM / 并发</div></div><div class="list-actions">' + placement + subscriptionPlanStatus(plan.status) + '<button type="button" class="md-icon-button" title="编辑" data-edit-subscription-plan="' + plan.id + '"><span class="material-symbol" aria-hidden="true">edit</span></button><button type="button" class="md-icon-button" title="切换状态" data-toggle-subscription-plan="' + plan.id + '"><span class="material-symbol" aria-hidden="true">refresh</span></button></div></div>';
  }

  function updateUpgradePlanOptions(currentPlanID, selectedPlanID) {
    const select = document.querySelector('[data-plan-upgrade-from]');
    if (!select) return;
    select.innerHTML = '<option value="0">独立计划</option>' + subscriptionPlans.filter(function (plan) { return String(plan.id) !== String(currentPlanID || ''); }).map(function (plan) {
      return '<option value="' + plan.id + '">' + api.escapeHTML(plan.name) + ' · ' + api.escapeHTML(plan.code) + '</option>';
    }).join('');
    select.value = String(selectedPlanID || 0);
  }

  function renderSubscriptionModelRules(rules) {
    const body = document.querySelector('[data-subscription-model-rules]');
    if (!body) return;
    const byID = {};
    (Array.isArray(rules) ? rules : []).forEach(function (rule) { byID[String(rule.model_id)] = rule; });
    const models = adminModels.rows();
    body.innerHTML = models.length ? models.map(function (item) {
      const rule = byID[String(item.id)];
      const checked = Boolean(rule);
      const multiplier = checked && Number(rule.multiplier) > 0 ? Number(rule.multiplier) : '';
      return '<div class="subscription-model-rule"><label><input type="checkbox" data-plan-model-access value="' + item.id + '"' + (checked ? ' checked' : '') + '><span><b>' + api.escapeHTML(item.display_name || item.model) + '</b><small><code>' + api.escapeHTML(item.model) + '</code>' + (item.subscription_only ? ' · 订阅专属' : ' · 公开模型') + '</small></span></label><div class="md-textfield"><input class="md-input" data-plan-model-multiplier type="number" min="0" max="1" step="0.01" value="' + multiplier + '" placeholder=" "' + (checked ? '' : ' disabled') + '><label class="md-input-label">专属倍率</label></div></div>';
    }).join('') : '<div class="empty">请先创建模型。</div>';
    body.querySelectorAll('[data-plan-model-access]').forEach(function (checkbox) {
      checkbox.addEventListener('change', function () {
        const input = checkbox.closest('.subscription-model-rule').querySelector('[data-plan-model-multiplier]');
        input.disabled = !checkbox.checked;
        if (!checkbox.checked) input.value = '';
      });
    });
  }

  function updateRedeemPlanOptions() {
    const select = document.querySelector('[data-redeem-plan]');
    if (!select) return;
    const current = select.value;
    select.innerHTML = '<option value="">请选择订阅计划</option>' + subscriptionPlans.filter(function (plan) { return Number(plan.status) === 1; }).map(function (plan) {
      return '<option value="' + plan.id + '">' + api.escapeHTML(plan.name) + ' · ' + api.escapeHTML(plan.code) + '</option>';
    }).join('');
    if (current) select.value = current;
  }

  function updateBenefitPlanOptions() {
    const select = document.querySelector('[data-benefit-plan]');
    if (!select) return;
    const current = select.value;
    select.innerHTML = '<option value="">请选择套餐</option>' + subscriptionPlans.map(function (plan) {
      return '<option value="' + plan.id + '">' + api.escapeHTML(plan.name) + ' · ' + api.escapeHTML(plan.code) + (plan.unlimited_quota ? ' · 不限额' : '') + '</option>';
    }).join('');
    if (current) select.value = current;
  }

  function resetSubscriptionBenefitForm() {
    const form = document.querySelector('[data-subscription-benefit-form]');
    if (!form) return;
    form.reset();
    form.elements.permanent_quota.value = '0';
	form.elements.timed_quota.value = '0';
	form.elements.quota_group.value = '';
	form.elements.target_user_group.value = '';
	form.elements.extend_subscription_days.value = '0';
    api.syncQuotaUSD(form);
    updateBenefitPlanOptions();
  }

  function subscriptionBenefitSummary(item) {
	const parts = [];
	parts.push(item.target_user_group ? '用户组 ' + item.target_user_group : '全部用户组');
	if (item.quota_group) parts.push('额度组 ' + item.quota_group);
    if (Number(item.permanent_quota) > 0) parts.push('永久 ' + api.moneyFromQuota(item.permanent_quota, 2));
    if (Number(item.timed_quota) > 0) parts.push('限时 ' + api.moneyFromQuota(item.timed_quota, 2));
    if (item.restore_quota_percent) parts.push('恢复套餐总额 ' + item.restore_quota_percent + '%');
	if (item.extend_subscription_days) parts.push('套餐展期 ' + item.extend_subscription_days + ' 天');
    if (item.timed_quota_expires_at) parts.push('到期 ' + api.date(item.timed_quota_expires_at));
    if (item.reset_subscription_quota) parts.push('套餐额度重置');
    return parts.join(' + ');
  }

  function subscriptionBenefitCampaignRow(item) {
    const description = item.description ? '<div class="list-item-sub">' + api.escapeHTML(item.description) + '</div>' : '';
	const revoke = Number(item.pending_count) > 0 ? '<button type="button" class="md-btn md-btn-danger md-btn-compact" data-root-only data-revoke-benefit-pending="' + item.id + '" data-benefit-name="' + api.escapeHTML(item.name) + '"><span class="material-symbol" aria-hidden="true">undo</span>撤回待领取</button>' : '';
	const deletable = Number(item.pending_count) === 0 && (Number(item.revoked_count) > 0 || Number(item.claimed_count) === Number(item.recipient_count));
	const remove = deletable ? '<button type="button" class="md-icon-button danger" title="删除发放记录" data-root-only data-delete-benefit-campaign="' + item.id + '" data-benefit-name="' + api.escapeHTML(item.name) + '"><span class="material-symbol" aria-hidden="true">delete</span></button>' : '';
    return '<div class="list-item"><span class="material-symbol" aria-hidden="true">redeem</span><div class="list-item-body"><div class="list-item-title">' + api.escapeHTML(item.name) + '</div><div class="list-item-sub">' + api.escapeHTML(item.plan_name) + ' · ' + api.escapeHTML(subscriptionBenefitSummary(item)) + ' · ' + api.date(item.created_at) + '</div>' + description + '</div><div class="list-metrics"><div><b>' + api.number(item.recipient_count) + '</b>发放</div><div><b>' + api.number(item.pending_count) + '</b>待领取</div><div><b>' + api.number(item.claimed_count) + '</b>已领取</div><div><b>' + api.number(item.expired_count) + '</b>已失效</div><div><b>' + api.number(item.revoked_count) + '</b>已撤回</div></div><div class="list-actions">' + revoke + remove + '</div></div>';
  }

  async function loadSubscriptionBenefitCampaigns() {
    const result = await api.request('/api/admin/subscription-benefits');
    const rows = Array.isArray(result) ? result : [];
    const body = document.querySelector('[data-subscription-benefit-campaign-rows]');
    if (!body) return;
    body.innerHTML = rows.length ? rows.map(subscriptionBenefitCampaignRow).join('') : '<div class="empty">还没有发放套餐福利。</div>';
	body.querySelectorAll('[data-revoke-benefit-pending]').forEach(function (button) {
	  button.addEventListener('click', async function () {
		const name = button.dataset.benefitName || '该批次';
		if (!(await api.confirm('将撤回“' + name + '”中所有仍可领取的福利。已领取福利和已到账额度不会受到影响。', { title: '撤回待领取福利', confirmText: '确认撤回' }))) return;
		api.showBusy(button, true);
		try {
		  const result = await api.request('/api/admin/subscription-benefits/' + button.dataset.revokeBenefitPending + '/pending', { method: 'DELETE' });
		  api.toast('已撤回 ' + api.number(result.revoked_count) + ' 份待领取福利');
		  await loadSubscriptionBenefitCampaigns();
		} catch (error) { api.toast(error.message, 'danger'); }
		finally { api.showBusy(button, false); }
	  });
	});
	body.querySelectorAll('[data-delete-benefit-campaign]').forEach(function (button) {
	  button.addEventListener('click', async function () {
		const name = button.dataset.benefitName || '该批次';
		if (!(await api.confirm('删除“' + name + '”的发放记录？已到账福利不会被追回，但记录删除后无法恢复。', { title: '删除福利发放记录', confirmText: '删除' }))) return;
		api.showBusy(button, true);
		try {
		  await api.request('/api/admin/subscription-benefits/' + button.dataset.deleteBenefitCampaign, { method: 'DELETE' });
		  api.toast('福利发放记录已删除');
		  await loadSubscriptionBenefitCampaigns();
		} catch (error) { api.toast(error.message, 'danger'); }
		finally { api.showBusy(button, false); }
	  });
	});
  }

  function bindSubscriptionBenefitForm() {
    const form = document.querySelector('[data-subscription-benefit-form]');
    if (!form) return;
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      const button = form.querySelector('button[type="submit"]');
      const data = {
        plan_id: Number(form.elements.plan_id.value),
        name: form.elements.name.value.trim(),
        description: form.elements.description.value.trim(),
        target_user_group: form.elements.target_user_group.value.trim(),
        permanent_quota: Number(form.elements.permanent_quota.value) || 0,
        timed_quota: Number(form.elements.timed_quota.value) || 0,
		quota_group: form.elements.quota_group.value.trim(),
        restore_quota_percent: Number(form.elements.restore_quota_percent.value) || 0,
		extend_subscription_days: Number(form.elements.extend_subscription_days.value) || 0,
        timed_quota_expires_at: form.elements.timed_quota_expires_at.value ? new Date(form.elements.timed_quota_expires_at.value).toISOString() : null,
        reset_subscription_quota: form.elements.reset_subscription_quota.checked
      };
      if (data.permanent_quota <= 0 && data.timed_quota <= 0 && !data.reset_subscription_quota && !data.restore_quota_percent && !data.extend_subscription_days) {
        api.toast('请至少配置一种福利', 'danger');
        return;
      }
      if (data.reset_subscription_quota && data.restore_quota_percent > 0) {
        api.toast('百分比恢复与全额重置不能同时选择', 'danger');
        return;
      }
      if (data.timed_quota_expires_at && (data.timed_quota <= 0 || new Date(data.timed_quota_expires_at).getTime() <= Date.now())) {
        api.toast('请配置限时额度，并选择未来的到期时间', 'danger');
        return;
      }
      api.showBusy(button, true);
      try {
        const result = await api.request('/api/admin/subscription-benefits', { method: 'POST', body: data });
        api.closeDialog('subscription-benefit-dialog');
        api.toast('福利已发放给 ' + api.number(result.campaign && result.campaign.recipient_count) + ' 位用户');
        await loadSubscriptionBenefitCampaigns();
      } catch (error) { api.toast(error.message, 'danger'); }
      finally { api.showBusy(button, false); }
    });
  }

	function bindChannelVisibilityFilter() {
	  const input = document.querySelector('[data-hide-disabled-channels]');
	  if (!input) return;
	  input.checked = hideDisabledChannels;
	  input.addEventListener('change', function () {
		hideDisabledChannels = api.saveAdminListPreference('hideDisabledChannels', input.checked);
		loadChannels(false).catch(function (error) { api.toast(error.message, 'danger'); });
	  });
	}

  const promotionDiscountLabels = {
    none: '不打折', all: '整单', input: '全部输入类', output: '全部输出类',
    ordinary_input: '普通输入', cache_read: '缓存读取', cache_write: '缓存写入',
    request: '按次请求费', compaction: '远程压缩费'
  };

  function promotionDateTimeLocal(value) {
    if (!value) return '';
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return '';
    const local = new Date(date.getTime() - date.getTimezoneOffset() * 60000);
    return local.toISOString().slice(0, 16);
  }

  function updatePromotionScopeOptions() {
    const form = document.querySelector('[data-billing-promotion-form]');
    const list = document.querySelector('[data-promotion-scope-values]');
    const label = document.querySelector('[data-promotion-scope-label]');
    const valueField = document.querySelector('[data-promotion-scope-value-field]');
    if (!form || !list) return;
    const allModelsMode = form.elements.scope_type.value === 'all_models';
    const quotaGroupMode = form.elements.scope_type.value === 'quota_group';
    form.elements.scope_value.disabled = allModelsMode;
    form.elements.scope_value.required = !allModelsMode;
    if (valueField) valueField.hidden = allModelsMode;
    if (allModelsMode) form.elements.scope_value.value = '';
    const models = adminModels.rows();
    const values = quotaGroupMode
      ? Array.from(new Set(models.map(function (item) { return String(item.quota_group || '').trim(); }).filter(Boolean))).sort()
      : models.map(function (item) { return item.model; });
    list.innerHTML = values.map(function (value) { return '<option value="' + api.escapeHTML(value) + '">'; }).join('');
    if (label) label.textContent = quotaGroupMode ? '额度组名称' : '模型名称';
  }

  function resetBillingPromotionForm() {
    const form = document.querySelector('[data-billing-promotion-form]');
    if (!form) return;
    form.reset();
    form.elements.id.value = '';
    form.elements.scope_type.value = 'model';
    form.elements.discount_target.value = 'none';
    form.elements.discount_multiplier.value = '1';
    form.elements.cashback_threshold.value = '0';
    form.elements.cashback_amount.value = '0';
    form.elements.cashback_validity_days.value = '0';
    form.elements.cashback_repeatable.checked = true;
    form.elements.status.value = '1';
    form.elements.starts_at.value = '';
    form.elements.ends_at.value = '';
    api.syncQuotaUSD(form);
    updatePromotionScopeOptions();
    document.querySelector('[data-billing-promotion-dialog-title]').textContent = '新建计费活动';
  }

  function editBillingPromotion(item) {
    const form = document.querySelector('[data-billing-promotion-form]');
    if (!form) return;
    form.elements.id.value = item.id;
    form.elements.name.value = item.name || '';
    form.elements.description.value = item.description || '';
    form.elements.target_user_group.value = item.target_user_group || '';
    form.elements.scope_type.value = item.scope_type || 'model';
    updatePromotionScopeOptions();
    form.elements.scope_value.value = item.scope_value || '';
    form.elements.discount_target.value = item.discount_target || 'none';
    form.elements.discount_multiplier.value = item.discount_multiplier == null ? 1 : Number(item.discount_multiplier);
    form.elements.cashback_threshold.value = Number(item.cashback_threshold || 0);
    form.elements.cashback_amount.value = Number(item.cashback_amount || 0);
    form.elements.cashback_quota_group.value = item.cashback_quota_group || '';
    form.elements.cashback_validity_days.value = Number(item.cashback_validity_days || 0);
    form.elements.cashback_repeatable.checked = Boolean(item.cashback_repeatable);
    form.elements.starts_at.value = promotionDateTimeLocal(item.starts_at);
    form.elements.ends_at.value = promotionDateTimeLocal(item.ends_at);
    form.elements.status.value = String(item.status || 1);
    api.syncQuotaUSD(form);
    document.querySelector('[data-billing-promotion-dialog-title]').textContent = '编辑计费活动';
    api.openDialog('billing-promotion-dialog');
  }

  function billingPromotionSummary(item) {
    const parts = [];
    parts.push(item.target_user_group ? '用户组 ' + item.target_user_group : '全部用户组');
    parts.push(item.scope_type === 'all_models' ? '全站模型' : (item.scope_type === 'quota_group' ? '额度组 ' : '模型 ') + item.scope_value);
    if (item.discount_target && item.discount_target !== 'none') {
      parts.push((promotionDiscountLabels[item.discount_target] || item.discount_target) + ' ×' + Number(item.discount_multiplier).toFixed(2));
    }
    if (Number(item.cashback_threshold) > 0) {
      const rewardTarget = item.cashback_quota_group ? ' → ' + item.cashback_quota_group : ' → 通用余额';
      const validity = Number(item.cashback_validity_days) > 0 ? ' · ' + Number(item.cashback_validity_days) + ' 天限时' : ' · 永久';
      parts.push('消费 ' + api.moneyFromQuota(item.cashback_threshold, 2) + ' 返 ' + api.moneyFromQuota(item.cashback_amount, 2) + rewardTarget + validity + (item.cashback_repeatable ? ' · 循环' : ' · 单次'));
    }
    return parts.join(' · ');
  }

  function billingPromotionRow(item) {
    const enabled = Number(item.status) === 1;
    const schedule = api.date(item.starts_at) + ' — ' + (item.ends_at ? api.date(item.ends_at) : '长期');
    const description = item.description ? '<div class="list-item-sub">' + api.escapeHTML(item.description) + '</div>' : '';
    return '<div class="list-item"><span class="material-symbol" aria-hidden="true">redeem</span><div class="list-item-body"><div class="list-item-title">' + api.escapeHTML(item.name) + '</div><div class="list-item-sub">' + api.escapeHTML(billingPromotionSummary(item)) + '</div><div class="list-item-sub">' + api.escapeHTML(schedule) + '</div>' + description + '</div><div class="list-metrics"><div><b>' + api.number(item.participant_count) + '</b>参与用户</div><div><b>' + api.number(item.reward_count) + '</b>返现次数</div><div><b>' + api.moneyFromQuota(item.reward_quota, 2) + '</b>累计返现</div></div><div class="list-actions"><span>' + api.statusLabel(item.status, 'token') + '</span><button type="button" class="md-icon-button" title="编辑活动" data-edit-billing-promotion="' + item.id + '"><span class="material-symbol" aria-hidden="true">edit</span></button><button type="button" class="md-icon-button" title="' + (enabled ? '停用活动' : '启用活动') + '" data-toggle-billing-promotion="' + item.id + '"><span class="material-symbol" aria-hidden="true">' + (enabled ? 'pause' : 'play_arrow') + '</span></button><button type="button" class="md-icon-button danger" title="删除活动" data-delete-billing-promotion="' + item.id + '"><span class="material-symbol" aria-hidden="true">delete</span></button></div></div>';
  }

  async function loadBillingPromotions() {
    const result = await api.request('/api/admin/billing-promotions');
    billingPromotions = Array.isArray(result) ? result : [];
    updatePromotionScopeOptions();
    const body = document.querySelector('[data-billing-promotion-rows]');
    if (!body) return;
    body.innerHTML = billingPromotions.length ? billingPromotions.map(billingPromotionRow).join('') : '<div class="empty">还没有配置计费活动。</div>';
    body.querySelectorAll('[data-edit-billing-promotion]').forEach(function (button) {
      button.addEventListener('click', function () {
        const item = billingPromotions.find(function (promotion) { return String(promotion.id) === button.dataset.editBillingPromotion; });
        if (item) editBillingPromotion(item);
      });
    });
    body.querySelectorAll('[data-toggle-billing-promotion]').forEach(function (button) {
      button.addEventListener('click', async function () {
        const item = billingPromotions.find(function (promotion) { return String(promotion.id) === button.dataset.toggleBillingPromotion; });
        if (!item) return;
        try {
          await api.request('/api/admin/billing-promotions/' + item.id + '/status', { method: 'PATCH', body: { status: Number(item.status) === 1 ? 2 : 1 } });
          api.toast(Number(item.status) === 1 ? '计费活动已停用' : '计费活动已启用');
          await loadBillingPromotions();
        } catch (error) { api.toast(error.message, 'danger'); }
      });
    });
    body.querySelectorAll('[data-delete-billing-promotion]').forEach(function (button) {
      button.addEventListener('click', async function () {
        const item = billingPromotions.find(function (promotion) { return String(promotion.id) === button.dataset.deleteBillingPromotion; });
        if (!item || !(await api.confirm('删除“' + item.name + '”？已有返现记录的活动只能停用。', { title: '删除计费活动', confirmText: '删除' }))) return;
        try {
          await api.request('/api/admin/billing-promotions/' + item.id, { method: 'DELETE' });
          api.toast('计费活动已删除');
          await loadBillingPromotions();
        } catch (error) { api.toast(error.message, 'danger'); }
      });
    });
  }

  function bindBillingPromotionForm() {
    const form = document.querySelector('[data-billing-promotion-form]');
    if (!form) return;
    form.elements.scope_type.addEventListener('change', updatePromotionScopeOptions);
    form.elements.discount_target.addEventListener('change', function () {
      if (form.elements.discount_target.value === 'none') form.elements.discount_multiplier.value = '1';
    });
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      const threshold = Number(form.elements.cashback_threshold.value) || 0;
      const amount = Number(form.elements.cashback_amount.value) || 0;
      if ((threshold === 0) !== (amount === 0)) {
        api.toast('返现门槛与返现金额必须同时填写', 'danger');
        return;
      }
      if (form.elements.discount_target.value === 'none' && threshold === 0) {
        api.toast('请至少配置折扣或返现', 'danger');
        return;
      }
      const starts = form.elements.starts_at.value;
      const ends = form.elements.ends_at.value;
      if (starts && ends && new Date(ends).getTime() <= new Date(starts).getTime()) {
        api.toast('活动结束时间必须晚于开始时间', 'danger');
        return;
      }
      const id = form.elements.id.value;
      const data = {
        name: form.elements.name.value.trim(), description: form.elements.description.value.trim(),
        target_user_group: form.elements.target_user_group.value.trim(),
        scope_type: form.elements.scope_type.value, scope_value: form.elements.scope_value.value.trim(),
        discount_target: form.elements.discount_target.value,
        discount_multiplier: Number(form.elements.discount_multiplier.value),
        cashback_threshold: threshold, cashback_amount: amount,
        cashback_quota_group: form.elements.cashback_quota_group.value.trim(),
        cashback_validity_days: Number(form.elements.cashback_validity_days.value) || 0,
        cashback_repeatable: form.elements.cashback_repeatable.checked,
        starts_at: starts ? new Date(starts).toISOString() : '',
        ends_at: ends ? new Date(ends).toISOString() : null,
        status: Number(form.elements.status.value)
      };
      const button = form.querySelector('button[type="submit"]');
      api.showBusy(button, true);
      try {
        await api.request(id ? '/api/admin/billing-promotions/' + id : '/api/admin/billing-promotions', { method: id ? 'PUT' : 'POST', body: data });
        api.closeDialog('billing-promotion-dialog');
        api.toast(id ? '计费活动已更新' : '计费活动已创建');
        await loadBillingPromotions();
      } catch (error) { api.toast(error.message, 'danger'); }
      finally { api.showBusy(button, false); }
    });
  }

  function resetSubscriptionPlanForm() {
    const form = document.querySelector('[data-subscription-plan-form]');
    if (!form) return;
    form.reset();
    form.elements.id.value = '';
    form.elements.duration_days.value = '30';
	form.elements.reset_interval_days.value = '0';
    form.elements.quota.value = '0';
	form.elements.quota_group.value = '';
    form.elements.discount_multiplier.value = '1';
	form.elements.discount_savings_limit.value = '0';
	form.elements.display_order.value = '0';
    form.elements.requests_per_minute.value = '0';
    form.elements.concurrent_requests.value = '0';
    form.elements.collapsed.checked = false;
    form.elements.status.value = '1';
    api.syncQuotaUSD(form);
    updateUpgradePlanOptions(0, 0);
    renderSubscriptionModelRules([]);
    document.querySelector('[data-subscription-plan-dialog-title]').textContent = '添加订阅计划';
  }

  function editSubscriptionPlan(plan) {
    const form = document.querySelector('[data-subscription-plan-form]');
    if (!form) return;
    form.elements.id.value = plan.id;
    form.elements.refresh_existing_benefits.checked = false;
    form.elements.code.value = plan.code || '';
    form.elements.name.value = plan.name || '';
	form.elements.description.value = plan.description || '';
    form.elements.duration_days.value = plan.duration_days;
	form.elements.reset_interval_days.value = plan.reset_interval_days || 0;
    form.elements.quota.value = plan.quota;
	form.elements.quota_group.value = plan.quota_group || '';
    form.elements.unlimited_quota.checked = Boolean(plan.unlimited_quota);
    form.elements.discount_multiplier.value = plan.discount_multiplier || 1;
	form.elements.discount_savings_limit.value = plan.discount_savings_limit || 0;
	form.elements.display_order.value = plan.display_order || 0;
    form.elements.requests_per_minute.value = plan.requests_per_minute || 0;
    form.elements.concurrent_requests.value = plan.concurrent_requests || 0;
    form.elements.collapsed.checked = Boolean(plan.collapsed);
    form.elements.status.value = plan.status;
    api.syncQuotaUSD(form);
    updateUpgradePlanOptions(plan.id, plan.upgrade_from_plan_id);
    renderSubscriptionModelRules(plan.model_rules);
    document.querySelector('[data-subscription-plan-dialog-title]').textContent = '编辑订阅计划';
    api.openDialog('subscription-plan-dialog');
  }

  async function loadSubscriptionPlans() {
    const result = await api.request('/api/admin/subscription-plans');
    subscriptionPlans = Array.isArray(result) ? result : [];
    updateRedeemPlanOptions();
    updateBenefitPlanOptions();
    const body = document.querySelector('[data-subscription-plan-rows]');
    if (!body) return;
	const directPlans = subscriptionPlans.filter(function (plan) { return !plan.collapsed; });
	const collapsedPlans = subscriptionPlans.filter(function (plan) { return Boolean(plan.collapsed); });
	function planGroup(title, hint, plans) {
	  return '<section class="admin-plan-group"><div class="admin-plan-group-head"><div><b>' + title + '</b><small>' + hint + '</small></div><span class="tag muted">' + plans.length + ' 个</span></div>' + (plans.length ? plans.map(subscriptionPlanRow).join('') : '<div class="empty">此分组暂无计划。</div>') + '</section>';
	}
	body.innerHTML = subscriptionPlans.length ? planGroup('直接显示计划', '在用户套餐主列表中直接展示', directPlans) + planGroup('折叠计划', '收纳在“更多套餐”对话框中', collapsedPlans) : '<div class="empty">还没有订阅计划。</div>';
    body.querySelectorAll('[data-edit-subscription-plan]').forEach(function (button) {
      button.addEventListener('click', function () {
        const plan = subscriptionPlans.find(function (item) { return String(item.id) === button.dataset.editSubscriptionPlan; });
        if (plan) editSubscriptionPlan(plan);
      });
    });
    body.querySelectorAll('[data-toggle-subscription-plan]').forEach(function (button) {
      button.addEventListener('click', async function () {
        const plan = subscriptionPlans.find(function (item) { return String(item.id) === button.dataset.toggleSubscriptionPlan; });
        if (!plan) return;
        try {
          await api.request('/api/admin/subscription-plans/' + plan.id + '/status', { method: 'PATCH', body: { status: Number(plan.status) === 1 ? 2 : 1 } });
          api.toast('订阅计划状态已更新');
          await loadSubscriptionPlans();
        } catch (error) { api.toast(error.message, 'danger'); }
      });
    });
  }

  function bindSubscriptionPlanForm() {
    const form = document.querySelector('[data-subscription-plan-form]');
    if (!form) return;
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      const values = Object.fromEntries(new FormData(form).entries());
      const modelRules = Array.from(form.querySelectorAll('[data-plan-model-access]:checked')).map(function (checkbox) {
        const multiplier = checkbox.closest('.subscription-model-rule').querySelector('[data-plan-model-multiplier]').value;
        return { model_id: Number(checkbox.value), multiplier: multiplier === '' ? 0 : Number(multiplier) };
      });
      const data = {
        code: values.code.trim(),
        name: values.name.trim(),
		description: values.description.trim(),
        duration_days: Number(values.duration_days),
		reset_interval_days: Number(values.reset_interval_days) || 0,
        quota: Number(values.quota) || 0,
		quota_group: values.quota_group.trim(),
        unlimited_quota: form.elements.unlimited_quota.checked,
        discount_multiplier: Number(values.discount_multiplier) || 1,
		discount_savings_limit: Number(values.discount_savings_limit) || 0,
		display_order: Number(values.display_order) || 0,
        upgrade_from_plan_id: Number(values.upgrade_from_plan_id) || 0,
        requests_per_minute: Number(values.requests_per_minute) || 0,
        concurrent_requests: Number(values.concurrent_requests) || 0,
        collapsed: form.elements.collapsed.checked,
        model_rules: modelRules,
        refresh_existing_benefits: Boolean(values.id) && form.elements.refresh_existing_benefits.checked,
        status: Number(values.status)
      };
      const editing = Boolean(values.id);
      try {
        await api.request(editing ? '/api/admin/subscription-plans/' + values.id : '/api/admin/subscription-plans', { method: editing ? 'PUT' : 'POST', body: data });
        api.closeDialog('subscription-plan-dialog');
        api.toast(editing ? '订阅计划已更新' : '订阅计划已创建');
        await loadSubscriptionPlans();
      } catch (error) { api.toast(error.message, 'danger'); }
    });
  }

  function userRow(user) {
    const enabled = Number(user.status) === 1;
    return '<div class="list-item"><span class="material-symbol" aria-hidden="true">person</span><div class="list-item-body"><div class="list-item-title">' + api.escapeHTML(user.username) + '</div><div class="list-item-sub">#' + user.id + ' · ' + api.escapeHTML(user.group || 'default') + ' · ' + api.date(user.created_at) + '</div></div><div class="list-metrics"><div><b>' + api.moneyFromQuota(user.quota, 2) + '</b>余额</div><div><b>' + adminRoleLabel(user.role) + '</b>角色</div></div><div class="list-actions"><span>' + api.statusLabel(user.status, 'token') + '</span><button type="button" class="md-icon-button" title="编辑用户" data-edit-user="' + user.id + '" data-user="' + encodeURIComponent(JSON.stringify(user)) + '"><span class="material-symbol" aria-hidden="true">edit</span></button><button type="button" class="md-icon-button ' + (enabled ? 'danger' : '') + '" title="' + (enabled ? '禁用用户' : '启用用户') + '" data-toggle-user="' + user.id + '" data-next-status="' + (enabled ? 2 : 1) + '"><span class="material-symbol" aria-hidden="true">' + (enabled ? 'close' : 'check') + '</span></button></div></div>';
  }

  async function loadUsers() {
    const params = new URLSearchParams({ page: String(userState.page), page_size: String(userState.pageSize) });
    ['keyword', 'group', 'role', 'status'].forEach(function (key) { if (userState[key]) params.set(key, userState[key]); });
    if (userState.sleeping) params.set('sleeping', '1');
    const payload = await api.request('/api/admin/users?' + params.toString());
    const rows = Array.isArray(payload && payload.items) ? payload.items : [];
    const body = document.querySelector('[data-admin-user-rows]');
    if (!body) return;
    body.innerHTML = rows.length ? rows.map(userRow).join('') : '<div class="empty">还没有用户。</div>';
    userState.totalPages = Number(payload.total_pages) || 0;
    updatePagination('admin-user', userState, payload.total);
    body.querySelectorAll('[data-edit-user]').forEach(function (button) {
      button.addEventListener('click', function () {
        const user = JSON.parse(decodeURIComponent(button.dataset.user));
        const form = document.querySelector('[data-user-form]');
        form.elements.id.value = user.id;
        form.elements.username.value = user.username || '';
        form.elements.quota.value = user.quota;
        api.syncQuotaUSD(form);
        form.elements.group.value = user.group || 'default';
        form.elements.role.value = user.role;
        form.elements.status.value = user.status;
        api.openDialog('user-dialog');
      });
    });
    body.querySelectorAll('[data-toggle-user]').forEach(function (button) {
      button.addEventListener('click', async function () {
        const disabling = Number(button.dataset.nextStatus) === 2;
        if (disabling && !(await api.confirm('禁用后该用户将无法登录或调用 API。', { title: '禁用用户', confirmText: '禁用' }))) return;
        try {
          await api.request('/api/admin/users/' + button.dataset.toggleUser, { method: 'PATCH', body: { status: Number(button.dataset.nextStatus) } });
          api.toast(disabling ? '用户已禁用' : '用户已启用');
          await loadUsers();
          await loadOverview();
        } catch (error) { api.toast(error.message, 'danger'); }
      });
    });
  }

  function codeBatchStatus(batch) {
    const unused = Number(batch.unused_count || 0);
    const used = Number(batch.used_count || 0);
    return (unused ? '<span class="tag success">未使用 ' + api.number(unused) + '</span>' : '') +
      (used ? '<span class="tag muted">已使用 ' + api.number(used) + '</span>' : '');
  }

  function redemptionBatchRow(batch, batchIndex) {
    const items = Array.isArray(batch.items) ? batch.items : [];
    const first = items[0] || {};
    const batchID = batch.batch_id || ('legacy-' + (first.id || 0));
    const publicCode = Boolean(first.is_public);
    const group = String(batch.user_group || first.user_group || '').trim();
    const siteLabel = group === 'pay' ? '商业站 · pay' : (group === 'default' ? '公益站 · default' : '历史通用 · 未标记');
	const extension = first.redeem_type === 'subscription_extension';
    const typeLabel = (publicCode ? '公共 · ' : '') + (first.redeem_type === 'subscription' ? (first.includes_base_plans ? '订阅直升码' : '订阅兑换') : (extension ? '套餐展期' : '额度充值'));
    const amountLabel = first.redeem_type === 'subscription' ? (first.includes_base_plans ? '完整升级链 · ' : '') + '订阅计划 #' + Number(first.subscription_plan_id || 0) : (extension ? '延长 ' + Number(first.subscription_extension_days || 0) + ' 天 · 套餐 #' + Number(first.subscription_plan_id || 0) : api.moneyFromQuota(first.quota, 2) + ' / $' + Number(first.amount_usd || 0).toFixed(2));
    const detailRows = items.map(function (item) {
      const used = Number(item.status) === 2;
      const claimed = Number(item.redeemed_count || 0);
      const limit = Number(item.redeem_limit || 1);
      const latest = claimed > 0 ? ' · 最近由 ' + api.escapeHTML(item.used_by_account || ('用户 #' + Number(item.used_by_user_id || 0))) + ' 兑换 · ' + api.date(item.used_at) : '';
      const usage = publicCode ? '已兑换 ' + api.number(claimed) + ' / ' + api.number(limit) + latest : (used ? '由 ' + api.escapeHTML(item.used_by_account || ('用户 #' + Number(item.used_by_user_id || 0))) + ' 使用 · ' + api.date(item.used_at) : '未使用');
      const statusLabel = publicCode ? (used ? '已达上限' : '可兑换') : (used ? '已使用' : '未使用');
      const canDelete = publicCode || (!used && claimed === 0);
      const publicDeleteAttribute = publicCode ? ' data-public-redemption="true"' : '';
      return '<div class="list-item code-batch-item"><span class="material-symbol" aria-hidden="true">confirmation_number</span><div class="list-item-body"><div class="list-item-title"><code>' + api.escapeHTML(item.code) + '</code></div><div class="list-item-sub">' + usage + '</div></div><span class="tag ' + (used ? 'muted' : 'success') + '">' + statusLabel + '</span>' + (canDelete ? '<button type="button" class="md-icon-button danger" title="' + (publicCode ? '删除公共兑换码' : '删除兑换码') + '" data-delete-redemption="' + Number(item.id) + '"' + publicDeleteAttribute + '><span class="material-symbol" aria-hidden="true">delete</span></button>' : '') + '</div>';
    }).join('');
    const quantityMetric = publicCode ? '<div><b>' + api.number(first.redeemed_count || 0) + ' / ' + api.number(first.redeem_limit || 1) + '</b>兑换进度</div>' : '<div><b>' + api.number(batch.count || items.length) + '</b>码数量</div>';
    const status = publicCode ? '<span class="tag ' + (Number(first.status) === 2 ? 'muted' : 'success') + '">' + (Number(first.status) === 2 ? '已达上限' : '公共可用') + '</span>' : codeBatchStatus(batch);
    const deleteAction = publicCode
      ? '<button type="button" class="md-icon-button danger" title="删除公共兑换码" data-delete-redemption="' + Number(first.id) + '" data-public-redemption="true"><span class="material-symbol" aria-hidden="true">delete</span></button>'
      : (Number(batch.deletable_count || 0) > 0 ? '<button type="button" class="md-icon-button danger" title="删除批次内未使用兑换码" data-delete-redemption-batch="' + api.escapeHTML(batchID) + '"><span class="material-symbol" aria-hidden="true">delete_sweep</span></button>' : '');
    return '<section class="code-batch-card"><div class="code-batch-head"><button type="button" class="md-icon-button code-batch-toggle" title="展开批次" aria-expanded="false" data-code-batch-toggle><span class="material-symbol code-batch-chevron" aria-hidden="true">expand_more</span></button><span class="code-batch-icon material-symbol" aria-hidden="true">confirmation_number</span><div class="list-item-body"><div class="list-item-title">' + api.escapeHTML(batch.name || first.name || '未命名批次') + '</div><div class="list-item-sub"><code>' + api.escapeHTML(batchID) + '</code> · 归属：' + siteLabel + ' · ' + typeLabel + ' · ' + api.date(batch.created_at || first.created_at) + '</div></div><div class="list-metrics"><div><b>' + amountLabel + '</b>面额</div>' + quantityMetric + '</div>' + status + '<button type="button" class="md-icon-button" title="批量复制该批次" data-copy-redemption-batch="' + Number(batchIndex) + '"><span class="material-symbol" aria-hidden="true">content_copy</span></button>' + deleteAction + '</div><div class="code-batch-items hidden">' + detailRows + '</div></section>';
  }

  function registrationCodeBatchRow(batch, batchIndex) {
    const items = Array.isArray(batch.items) ? batch.items : [];
    const first = items[0] || {};
    const batchID = batch.batch_id || ('legacy-' + (first.id || 0));
    const group = batch.user_group || first.user_group || 'default';
    const source = group === 'pay' ? '付费用户' : '普通用户';
    const detailRows = items.map(function (item) {
      const used = Number(item.status) === 2;
      const usage = used ? '由 ' + api.escapeHTML(item.used_by_account || ('用户 #' + Number(item.used_by_user_id || 0))) + ' 使用 · ' + api.date(item.used_at) : '未使用';
      return '<div class="list-item code-batch-item"><span class="material-symbol" aria-hidden="true">key</span><div class="list-item-body"><div class="list-item-title"><code>' + api.escapeHTML(item.code) + '</code></div><div class="list-item-sub">' + usage + '</div></div><span class="tag ' + (used ? 'muted' : 'success') + '">' + (used ? '已使用' : '未使用') + '</span>' + (used ? '' : '<button type="button" class="md-icon-button danger" title="删除注册码" data-delete-registration-code="' + Number(item.id) + '"><span class="material-symbol" aria-hidden="true">delete</span></button>') + '</div>';
    }).join('');
    return '<section class="code-batch-card"><div class="code-batch-head"><button type="button" class="md-icon-button code-batch-toggle" title="展开批次" aria-expanded="false" data-code-batch-toggle><span class="material-symbol code-batch-chevron" aria-hidden="true">expand_more</span></button><span class="code-batch-icon material-symbol" aria-hidden="true">key</span><div class="list-item-body"><div class="list-item-title">' + api.escapeHTML(batch.name || first.name || '未命名批次') + '</div><div class="list-item-sub"><code>' + api.escapeHTML(batchID) + '</code> · ' + source + ' · ' + api.date(batch.created_at || first.created_at) + '</div></div><div class="list-metrics"><div><b>' + api.number(batch.count || items.length) + '</b>码数量</div></div><span class="tag ' + (group === 'pay' ? '' : 'muted') + '">' + api.escapeHTML(group) + '</span>' + codeBatchStatus(batch) + '<button type="button" class="md-icon-button" title="批量复制该批次" data-copy-registration-batch="' + Number(batchIndex) + '"><span class="material-symbol" aria-hidden="true">content_copy</span></button>' + (Number(batch.unused_count || 0) > 0 ? '<button type="button" class="md-icon-button danger" title="删除批次内未使用注册码" data-delete-registration-code-batch="' + api.escapeHTML(batchID) + '"><span class="material-symbol" aria-hidden="true">delete_sweep</span></button>' : '') + '</div><div class="code-batch-items hidden">' + detailRows + '</div></section>';
  }

  function prepaidCardBatchRow(batch, batchIndex) {
    const items = Array.isArray(batch.items) ? batch.items : [];
    const first = items[0] || {};
    const batchID = batch.batch_id || ('legacy-' + (first.id || 0));
    const group = batch.user_group || first.user_group || 'pay';
    const unused = Number(batch.unused_count || 0);
    const redeemed = Number(batch.redeemed_count || 0);
    const expired = Number(batch.expired_count || 0);
    const claimExpiry = batch.claim_expires_at || first.claim_expires_at;
    const detailRows = items.map(function (item) {
      const status = Number(item.status);
      const statusMeta = status === 1 ? ['未使用', 'success'] : (status === 2 ? ['已兑换', 'muted'] : ['已过期', 'error']);
      const identity = item.redeemed_by_identity ? '身份 ' + api.escapeHTML(item.redeemed_by_identity) : '';
      const expiry = status === 2 && item.usage_expires_at ? '额度到期 ' + api.date(item.usage_expires_at) : (item.claim_expires_at ? '兑换截止 ' + api.date(item.claim_expires_at) : '');
      const detail = [identity, expiry].filter(Boolean).join(' · ') || '未绑定身份';
      return '<div class="list-item code-batch-item"><span class="material-symbol" aria-hidden="true">payments</span><div class="list-item-body"><div class="list-item-title"><code>' + api.escapeHTML(item.code) + '</code></div><div class="list-item-sub">' + detail + '</div></div><span class="tag ' + statusMeta[1] + '">' + statusMeta[0] + '</span>' + (status === 1 ? '<button type="button" class="md-icon-button danger" title="删除预付卡" data-delete-prepaid-card="' + Number(item.id) + '"><span class="material-symbol" aria-hidden="true">delete</span></button>' : '') + '</div>';
    }).join('');
    const statusTags = (unused ? '<span class="tag success">未使用 ' + api.number(unused) + '</span>' : '') + (redeemed ? '<span class="tag muted">已兑换 ' + api.number(redeemed) + '</span>' : '') + (expired ? '<span class="tag error">已过期 ' + api.number(expired) + '</span>' : '');
    return '<section class="code-batch-card"><div class="code-batch-head"><button type="button" class="md-icon-button code-batch-toggle" title="展开批次" aria-expanded="false" data-code-batch-toggle><span class="material-symbol code-batch-chevron" aria-hidden="true">expand_more</span></button><span class="code-batch-icon material-symbol" aria-hidden="true">payments</span><div class="list-item-body"><div class="list-item-title">' + api.escapeHTML(batch.name || first.name || '未命名批次') + '</div><div class="list-item-sub"><code>' + api.escapeHTML(batchID) + '</code> · ' + Number(batch.validity_days || first.validity_days || 0) + ' 天' + (claimExpiry ? ' · 兑换截止 ' + api.date(claimExpiry) : '') + ' · ' + api.date(batch.created_at || first.created_at) + '</div></div><div class="list-metrics"><div><b>' + api.moneyFromQuota(batch.quota || first.quota, 2) + '</b>面额</div><div><b>' + api.number(batch.count || items.length) + '</b>卡数量</div></div><span class="tag ' + (group === 'pay' ? '' : 'muted') + '">' + (group === 'pay' ? '商业 pay' : '公益 default') + '</span>' + statusTags + '<button type="button" class="md-icon-button" title="批量复制该批次" data-copy-prepaid-batch="' + Number(batchIndex) + '"><span class="material-symbol" aria-hidden="true">content_copy</span></button>' + (unused > 0 ? '<button type="button" class="md-icon-button danger" title="删除批次内未使用预付卡" data-delete-prepaid-batch="' + api.escapeHTML(batchID) + '"><span class="material-symbol" aria-hidden="true">delete_sweep</span></button>' : '') + '</div><div class="code-batch-items hidden">' + detailRows + '</div></section>';
  }

  function bindCodeBatchToggles(body) {
    body.querySelectorAll('[data-code-batch-toggle]').forEach(function (button) {
      button.addEventListener('click', function () {
        const detail = button.closest('.code-batch-card').querySelector('.code-batch-items');
        const expanded = button.getAttribute('aria-expanded') !== 'true';
        button.setAttribute('aria-expanded', String(expanded));
        button.title = expanded ? '收起批次' : '展开批次';
        detail.classList.toggle('hidden', !expanded);
      });
    });
  }

  async function loadRedemptions() {
    const params = new URLSearchParams({ page: String(redemptionState.page), page_size: String(redemptionState.pageSize) });
    params.set('group_by', 'batch');
    if (redemptionState.keyword) params.set('keyword', redemptionState.keyword);
    if (redemptionState.status) params.set('status', redemptionState.status);
    if (redemptionState.userGroup) params.set('user_group', redemptionState.userGroup);
    const result = await api.request('/api/admin/redemptions?' + params.toString());
    const rows = Array.isArray(result && result.items) ? result.items : [];
    const body = document.querySelector('[data-redemption-rows]');
    if (!body) return;
    body.innerHTML = rows.length ? rows.map(redemptionBatchRow).join('') : '<div class="empty">还没有兑换码。</div>';
    redemptionState.totalPages = Number(result.total_pages) || 0;
    updatePagination('redemption', redemptionState, result.total, '个批次');
    bindCodeBatchToggles(body);
    body.querySelectorAll('[data-copy-redemption-batch]').forEach(function (button) {
      button.addEventListener('click', function () {
        const batch = rows[Number(button.dataset.copyRedemptionBatch)];
        if (!batch) return;
        const codes = (Array.isArray(batch.items) ? batch.items : []).map(function (item) { return item.code; }).filter(Boolean);
        openRedemptionCodeDialog(codes, (batch.name || '兑换码批次') + ' · ' + (batch.batch_id || ''));
      });
    });
    body.querySelectorAll('[data-delete-redemption]').forEach(function (button) {
      button.addEventListener('click', async function () {
        const publicCode = button.dataset.publicRedemption === 'true';
        const message = publicCode ? '删除后该公共兑换码将立即失效，已有兑换记录也会一并删除，且无法恢复。' : '删除后该兑换码将无法使用且无法恢复。';
        if (!(await api.confirm(message, { title: publicCode ? '删除公共兑换码' : '删除兑换码', confirmText: '删除' }))) return;
        try { await api.request('/api/admin/redemptions/' + button.dataset.deleteRedemption, { method: 'DELETE' }); api.toast(publicCode ? '公共兑换码已删除' : '兑换码已删除'); await loadRedemptions(); } catch (error) { api.toast(error.message, 'danger'); }
      });
    });
    body.querySelectorAll('[data-delete-redemption-batch]').forEach(function (button) {
      button.addEventListener('click', async function () {
        if (!(await api.confirm('将删除该批次中的所有未使用兑换码，已使用码会保留。', { title: '删除未使用批次', confirmText: '删除' }))) return;
        try { await api.request('/api/admin/redemptions?batch_id=' + encodeURIComponent(button.dataset.deleteRedemptionBatch), { method: 'DELETE' }); api.toast('批次中的未使用兑换码已删除'); await loadRedemptions(); } catch (error) { api.toast(error.message, 'danger'); }
      });
    });
  }

  async function loadRegistrationCodes() {
    const params = new URLSearchParams({ page: String(registrationCodeState.page), page_size: String(registrationCodeState.pageSize) });
    params.set('group_by', 'batch');
    if (registrationCodeState.keyword) params.set('keyword', registrationCodeState.keyword);
    if (registrationCodeState.status) params.set('status', registrationCodeState.status);
    if (registrationCodeState.userGroup) params.set('user_group', registrationCodeState.userGroup);
    const result = await api.request('/api/admin/registration-codes?' + params.toString());
    const rows = Array.isArray(result && result.items) ? result.items : [];
    const body = document.querySelector('[data-registration-code-rows]');
    if (!body) return;
    body.innerHTML = rows.length ? rows.map(registrationCodeBatchRow).join('') : '<div class="empty">还没有注册码。</div>';
    registrationCodeState.totalPages = Number(result.total_pages) || 0;
    updatePagination('registration-code', registrationCodeState, result.total, '个批次');
    bindCodeBatchToggles(body);
    body.querySelectorAll('[data-copy-registration-batch]').forEach(function (button) {
      button.addEventListener('click', function () {
        const batch = rows[Number(button.dataset.copyRegistrationBatch)];
        if (!batch) return;
        const codes = (Array.isArray(batch.items) ? batch.items : []).map(function (item) { return item.code; }).filter(Boolean);
        openRegistrationCodeDialog(codes, (batch.name || '注册码批次') + ' · ' + (batch.batch_id || ''));
      });
    });
    body.querySelectorAll('[data-delete-registration-code]').forEach(function (button) {
      button.addEventListener('click', async function () {
        if (!(await api.confirm('删除后该注册码将无法使用且无法恢复。', { title: '删除注册码', confirmText: '删除' }))) return;
        try { await api.request('/api/admin/registration-codes/' + button.dataset.deleteRegistrationCode, { method: 'DELETE' }); api.toast('注册码已删除'); await loadRegistrationCodes(); await adminSettings.loadRegistrationSettings(); } catch (error) { api.toast(error.message, 'danger'); }
      });
    });
    body.querySelectorAll('[data-delete-registration-code-batch]').forEach(function (button) {
      button.addEventListener('click', async function () {
        if (!(await api.confirm('将删除该批次中的所有未使用注册码，已使用码会保留。', { title: '删除未使用批次', confirmText: '删除' }))) return;
        try { await api.request('/api/admin/registration-codes?batch_id=' + encodeURIComponent(button.dataset.deleteRegistrationCodeBatch), { method: 'DELETE' }); api.toast('批次中的未使用注册码已删除'); await loadRegistrationCodes(); await adminSettings.loadRegistrationSettings(); } catch (error) { api.toast(error.message, 'danger'); }
      });
    });
  }

  async function loadPrepaidCards() {
    const params = new URLSearchParams({ page: String(prepaidCardState.page), page_size: String(prepaidCardState.pageSize) });
    params.set('group_by', 'batch');
    if (prepaidCardState.keyword) params.set('keyword', prepaidCardState.keyword);
    if (prepaidCardState.status) params.set('status', prepaidCardState.status);
    const result = await api.request('/api/admin/prepaid-cards?' + params.toString());
    const rows = Array.isArray(result && result.items) ? result.items : [];
    const body = document.querySelector('[data-prepaid-card-rows]');
    if (!body) return;
    body.innerHTML = rows.length ? rows.map(prepaidCardBatchRow).join('') : '<div class="empty">还没有预付卡。</div>';
    prepaidCardState.totalPages = Number(result.total_pages) || 0;
    updatePagination('prepaid-card', prepaidCardState, result.total, '个批次');
    bindCodeBatchToggles(body);
    applyAdminCapabilities();
    body.querySelectorAll('[data-copy-prepaid-batch]').forEach(function (button) {
      button.addEventListener('click', function () {
        const batch = rows[Number(button.dataset.copyPrepaidBatch)];
        if (!batch) return;
        const codes = (Array.isArray(batch.items) ? batch.items : []).map(function (item) { return item.code; }).filter(Boolean);
        openPrepaidCodeDialog(codes, (batch.name || '预付卡批次') + ' · ' + (batch.batch_id || ''));
      });
    });
    body.querySelectorAll('[data-delete-prepaid-card]').forEach(function (button) {
      button.addEventListener('click', async function () {
        if (!(await api.confirm('删除后该预付卡将无法兑换且无法恢复。', { title: '删除预付卡', confirmText: '删除' }))) return;
        try { await api.request('/api/admin/prepaid-cards/' + button.dataset.deletePrepaidCard, { method:'DELETE' }); api.toast('预付卡已删除'); await loadPrepaidCards(); } catch (error) { api.toast(error.message, 'danger'); }
      });
    });
    body.querySelectorAll('[data-delete-prepaid-batch]').forEach(function (button) {
      button.addEventListener('click', async function () {
        if (!(await api.confirm('将删除该批次中全部未使用预付卡，已兑换记录不受影响。', { title:'删除未使用批次', confirmText:'删除' }))) return;
        try { await api.request('/api/admin/prepaid-cards?batch_id=' + encodeURIComponent(button.dataset.deletePrepaidBatch), { method:'DELETE' }); api.toast('批次中未使用预付卡已删除'); await loadPrepaidCards(); } catch (error) { api.toast(error.message, 'danger'); }
      });
    });
  }

  function openRedemptionCodeDialog(codes, title) {
    const list = Array.isArray(codes) ? codes : [];
    document.querySelector('[data-generated-codes]').value = list.join('\n');
    api.setText('[data-generated-redemption-title]', title || '兑换码批量复制');
    api.openDialog('generated-dialog');
  }

  function openRegistrationCodeDialog(codes, title) {
    const list = Array.isArray(codes) ? codes : [];
    document.querySelector('[data-generated-registration-codes]').value = list.join('\n');
    api.setText('[data-generated-registration-title]', title || '注册码批量复制');
    api.openDialog('generated-registration-code-dialog');
  }

  function openPrepaidCodeDialog(codes, title) {
    const list = Array.isArray(codes) ? codes : [];
    document.querySelector('[data-generated-prepaid-codes]').value = list.join('\n');
    api.setText('[data-generated-prepaid-title]', title || '预付卡批量复制');
    api.openDialog('generated-prepaid-dialog');
  }

  function bindPrepaidCards() {
    const form = document.querySelector('[data-prepaid-card-form]');
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      const button = form.querySelector('button[type="submit"]');
      api.showBusy(button, true);
      try {
		const payload = { name:form.elements.name.value.trim(), user_group:form.elements.user_group.value, count:Number(form.elements.count.value), quota:Number(form.elements.quota.value), validity_days:Number(form.elements.validity_days.value), claim_expires_at:form.elements.claim_expires_at.value };
        const result = await api.request('/api/admin/prepaid-cards/generate', { method:'POST', body:payload });
        api.closeDialog('prepaid-card-dialog'); openPrepaidCodeDialog(result.codes || [], '预付卡已生成'); api.toast('已生成 ' + Number(result.count || 0) + ' 张预付卡');
        prepaidCardState.page = 1; await loadPrepaidCards();
      } catch (error) { api.toast(error.message, 'danger'); } finally { api.showBusy(button, false); }
    });
    document.querySelector('[data-copy-prepaid-codes]').addEventListener('click', async function () {
      try { await navigator.clipboard.writeText(document.querySelector('[data-generated-prepaid-codes]').value); api.toast('预付卡已复制'); } catch (_) { api.toast('复制失败，请手动复制', 'danger'); }
    });
    document.querySelector('[data-delete-used-prepaid]').addEventListener('click', async function () {
      if (!(await api.confirm('将删除全部已兑换和已过期的预付卡记录。现有额度与 Key 不受影响，审计记录不可恢复。', { title:'清理预付卡记录', confirmText:'全部删除' }))) return;
      try { const result = await api.request('/api/admin/prepaid-cards/used', { method:'DELETE' }); api.toast('已删除 ' + Number(result.deleted_count || 0) + ' 条记录'); prepaidCardState.page=1; await loadPrepaidCards(); } catch (error) { api.toast(error.message, 'danger'); }
    });
    bindCodeFilters('prepaid-card', prepaidCardState, loadPrepaidCards);
  }

  function bindUsedCodeCleanup() {
    const redemptionButton = document.querySelector('[data-delete-used-redemptions]');
    if (redemptionButton) redemptionButton.addEventListener('click', async function () {
      if (!(await api.confirm('将永久删除全部已使用兑换码记录。用户已到账额度不会受影响，操作无法恢复。', { title: '删除已使用兑换码', confirmText: '全部删除' }))) return;
      try {
        const result = await api.request('/api/admin/redemptions/used', { method: 'DELETE' });
        redemptionState.page = 1;
        api.toast('已删除 ' + Number(result.deleted_count || 0) + ' 个已使用兑换码');
        await loadRedemptions();
      } catch (error) { api.toast(error.message, 'danger'); }
    });

    const registrationButton = document.querySelector('[data-delete-used-registration-codes]');
    if (registrationButton) registrationButton.addEventListener('click', async function () {
      if (!(await api.confirm('将永久删除全部已使用注册码记录。已注册账户不会受影响，操作无法恢复。', { title: '删除已使用注册码', confirmText: '全部删除' }))) return;
      try {
        const result = await api.request('/api/admin/registration-codes/used', { method: 'DELETE' });
        registrationCodeState.page = 1;
        api.toast('已删除 ' + Number(result.deleted_count || 0) + ' 个已使用注册码');
        await loadRegistrationCodes();
        await adminSettings.loadRegistrationSettings();
      } catch (error) { api.toast(error.message, 'danger'); }
    });
  }

  async function loadLogs() {
    const params = new URLSearchParams({ page: String(logState.page), page_size: String(logState.pageSize) });
    if (logState.keyword) params.set('keyword', logState.keyword);
    if (logState.statusCode) params.set('status_code', logState.statusCode);
    if (logState.exclude200) params.set('exclude_status_code', '200');
    params.set('sort_by', logState.sortBy);
    params.set('sort_order', logState.sortOrder);
    const result = await api.request('/api/admin/logs?' + params.toString());
    const rows = Array.isArray(result && result.items) ? result.items : [];
    const body = document.querySelector('[data-admin-log-rows]');
    if (!body) return;
    body.innerHTML = rows.length ? rows.map(function (item) {
      const failed = Number(item.status_code) >= 400;
      const requestID = String(item.request_id || '').trim();
      const traceAttributes = requestID ? ' data-log-request-id="' + api.escapeHTML(requestID) + '" title="右键或长按查看请求追踪"' : '';
      const totalTokens = Number(item.prompt_tokens || 0) + Number(item.completion_tokens || 0);
      const tokenBreakdown = '输入 ' + api.number(item.prompt_tokens) + ' · 缓存读 ' + api.number(item.cached_tokens) + ' · 缓存写 ' + api.number(item.cache_write_tokens) + ' · 输出 ' + api.number(item.completion_tokens);
      const compactTokenBreakdown = '入 ' + api.number(item.prompt_tokens) + ' · 读 ' + api.number(item.cached_tokens) + ' · 写 ' + api.number(item.cache_write_tokens) + ' · 出 ' + api.number(item.completion_tokens);
      const tokenName = item.token_name || (item.token_id ? 'Key #' + item.token_id : '—');
      return '<tr class="admin-log-row' + (requestID ? ' has-trace' : '') + '"' + traceAttributes + '>' +
        '<td><span class="usage-cell-primary">' + api.date(item.created_at) + '</span></td>' +
        '<td><span class="usage-cell-primary" title="' + api.escapeHTML(tokenName) + '">' + api.escapeHTML(tokenName) + '</span><span class="usage-cell-sub">' + api.escapeHTML(item.username || '未知用户') + '</span></td>' +
        '<td><span class="usage-cell-primary" title="' + api.escapeHTML(item.model || '—') + '">' + api.escapeHTML(item.model || '—') + '</span><span class="usage-cell-sub" title="' + api.escapeHTML(item.channel_name || '') + '">' + api.escapeHTML(item.channel_name || '—') + '</span></td>' +
        '<td title="' + api.escapeHTML(tokenBreakdown) + '"><span class="usage-cell-primary">' + api.number(totalTokens) + '</span><span class="usage-cell-sub">' + api.escapeHTML(compactTokenBreakdown) + '</span></td>' +
        '<td><span class="usage-cell-primary">' + api.moneyFromQuota(item.quota, 4) + '</span></td>' +
        '<td><span class="usage-cell-primary">' + api.number(item.duration_ms || 0) + ' ms</span></td>' +
        '<td><span class="usage-status ' + (failed ? 'error' : 'success') + '">' + (item.status_code || 200) + '</span>' + (item.error_code ? '<span class="usage-cell-sub" title="' + api.escapeHTML(item.error_code) + '">' + api.escapeHTML(item.error_code) + '</span>' : '') + '</td>' +
        '</tr>';
    }).join('') : '<tr><td class="empty" colspan="7">还没有审计日志。</td></tr>';
    logState.totalPages = Number(result.total_pages) || 0;
    updatePagination('admin-log', logState, result.total);
    bindLogTraceGestures(body);
  }

  function syncLogSortHeaders() {
    document.querySelectorAll('[data-admin-log-sort]').forEach(function (button) {
      const active = button.dataset.adminLogSort === logState.sortBy;
      button.classList.toggle('active', active);
      button.dataset.direction = active ? logState.sortOrder : '';
      const header = button.closest('th');
      if (header) header.setAttribute('aria-sort', active ? (logState.sortOrder === 'asc' ? 'ascending' : 'descending') : 'none');
    });
  }

  function bindLogTraceGestures(body) {
    body.querySelectorAll('[data-log-request-id]').forEach(function (row) {
      let timer = 0;
      let startX = 0;
      let startY = 0;
      function cancelLongPress() { window.clearTimeout(timer); timer = 0; }
      row.addEventListener('contextmenu', function (event) {
        event.preventDefault();
        cancelLongPress();
        openTrace(row.dataset.logRequestId);
      });
      row.addEventListener('pointerdown', function (event) {
        if (event.pointerType === 'mouse') return;
        startX = event.clientX;
        startY = event.clientY;
        cancelLongPress();
        timer = window.setTimeout(function () {
          timer = 0;
          if (navigator.vibrate) navigator.vibrate(20);
          openTrace(row.dataset.logRequestId);
        }, 600);
      });
      row.addEventListener('pointermove', function (event) {
        if (Math.abs(event.clientX - startX) > 10 || Math.abs(event.clientY - startY) > 10) cancelLongPress();
      });
      ['pointerup', 'pointercancel', 'pointerleave'].forEach(function (name) { row.addEventListener(name, cancelLongPress); });
    });
  }

  function setLogView(name) {
    activeLogView = name === 'traces' ? 'traces' : 'calls';
    document.querySelectorAll('[data-log-view]').forEach(function (button) {
      const active = button.dataset.logView === activeLogView;
      button.classList.toggle('active', active);
      button.setAttribute('aria-pressed', String(active));
    });
    document.querySelectorAll('[data-log-subpanel]').forEach(function (panel) { panel.classList.toggle('hidden', panel.dataset.logSubpanel !== activeLogView); });
    if (currentAdminRole >= 25) resources.ensure(activeLogView === 'traces' ? 'traces' : 'logs').catch(function (error) { api.toast(error.message, 'danger'); });
  }

  function bindLogViewSwitch() {
    document.querySelectorAll('[data-log-view]').forEach(function (button) { button.addEventListener('click', function () { setLogView(button.dataset.logView); }); });
  }

  function traceStatusLabel(status) {
	const labels = { running: '执行中', succeeded: '成功', failed: '失败', rejected: '已拒绝', aborted: '已中止' };
	const tone = status === 'succeeded' ? 'success' : status === 'running' ? '' : 'error';
	return '<span class="tag ' + tone + '">' + (labels[status] || api.escapeHTML(status || '未知')) + '</span>';
  }

  async function openTrace(requestID) {
	activeTraceRequestID = requestID;
	api.setText('[data-trace-dialog-title]', '请求追踪 · ' + requestID);
	api.setText('[data-trace-dialog-hint]', '正在读取完整时间线…');
	document.querySelector('[data-trace-summary]').innerHTML = '';
	document.querySelector('[data-trace-timeline]').innerHTML = '<div class="empty">加载中…</div>';
	api.openDialog('trace-dialog');
	try {
	  const result = await api.request('/api/admin/traces/' + encodeURIComponent(requestID));
	  if (activeTraceRequestID !== requestID) return;
	  const trace = result.trace || {};
	  const events = Array.isArray(result.events) ? result.events : [];
	  api.setText('[data-trace-dialog-hint]', api.date(trace.started_at) + ' · ' + Number(trace.duration_ms || 0) + ' ms · 重试 ' + Number(trace.retry_count || 0) + ' 次');
	  const upstreamRoute = Number(trace.channel_id) > 0
		? '<span class="tag">上游 #' + Number(trace.channel_id) + ' 渠道' + (Number(trace.channel_key_id) > 0 ? ' · #' + Number(trace.channel_key_id) + ' Key' : '') + '</span>'
		: '<span class="tag muted">未选择上游</span>';
	  document.querySelector('[data-trace-summary]').innerHTML = '<span class="tag">' + api.escapeHTML(trace.method || '') + ' ' + api.escapeHTML(trace.path || '') + '</span>' + traceStatusLabel(trace.status) + '<span class="tag muted">' + api.escapeHTML(trace.model || '未识别模型') + '</span>' + upstreamRoute + (trace.upstream_request_id ? '<span class="tag"><code>' + api.escapeHTML(trace.upstream_request_id) + '</code></span>' : '');
	  document.querySelector('[data-trace-timeline]').innerHTML = events.length ? events.map(function (event) {
		const metadata = event.metadata && Object.keys(event.metadata).length ? '<code>' + api.escapeHTML(JSON.stringify(event.metadata)) + '</code>' : '';
		const eventRoute = Number(event.channel_id) > 0
		  ? '<span class="tag muted">上游 #' + Number(event.channel_id) + ' 渠道' + (Number(event.channel_key_id) > 0 ? ' · #' + Number(event.channel_key_id) + ' Key' : '') + '</span>'
		  : (Number(event.channel_key_id) > 0 ? '<span class="tag muted">上游 #' + Number(event.channel_key_id) + ' Key</span>' : '');
		return '<div class="trace-event"><span class="trace-dot"></span><div><div class="trace-event-head"><b>' + api.escapeHTML(event.stage) + '</b><span>' + api.date(event.created_at) + '</span></div><p>' + api.escapeHTML(event.message || '状态已更新') + '</p><div class="tags">' + (event.status_code ? '<span class="tag">HTTP ' + event.status_code + '</span>' : '') + (event.duration_ms ? '<span class="tag muted">' + event.duration_ms + ' ms</span>' : '') + eventRoute + metadata + '</div></div></div>';
	  }).join('') : '<div class="empty">该请求还没有时间线事件。</div>';
	} catch (error) {
	  if (activeTraceRequestID !== requestID) return;
	  document.querySelector('[data-trace-timeline]').innerHTML = '<div class="empty">' + api.escapeHTML(error.message) + '</div>';
	}
  }

  async function loadTraces() {
	const body = document.querySelector('[data-trace-rows]');
	if (!body) return;
	const params = new URLSearchParams({ page: String(traceState.page), page_size: String(traceState.pageSize) });
	if (traceState.keyword) params.set('keyword', traceState.keyword);
	if (traceState.status) params.set('status', traceState.status);
	let result;
	try {
	  result = await api.request('/api/admin/traces?' + params.toString());
	} catch (error) {
	  body.innerHTML = '<div class="empty">请求追踪加载失败，请稍后重试。</div>';
	  throw error;
	}
	const rows = Array.isArray(result && result.items) ? result.items : [];
	body.innerHTML = rows.length ? rows.map(function (item) {
	  const route = Number(item.channel_id) > 0 ? ' · #' + Number(item.channel_id) + ' 渠道' + (Number(item.channel_key_id) > 0 ? ' · #' + Number(item.channel_key_id) + ' Key' : '') : '';
	  return '<button type="button" class="list-item list-item-button" data-open-trace="' + api.escapeHTML(item.request_id) + '"><span class="material-symbol" aria-hidden="true">swap_horiz</span><div class="list-item-body"><div class="list-item-title"><code>' + api.escapeHTML(item.request_id) + '</code></div><div class="list-item-sub">' + api.escapeHTML((item.method || '—') + ' ' + (item.path || '—')) + ' · ' + api.escapeHTML(item.username || '未鉴权') + ' · ' + api.escapeHTML(item.model || '未识别模型') + route + '</div></div><div class="list-metrics"><div><b>' + Number(item.duration_ms || 0) + ' ms</b>耗时</div><div><b>' + Number(item.retry_count || 0) + '</b>重试</div></div>' + traceStatusLabel(item.status) + '</button>';
	}).join('') : '<div class="empty">还没有请求追踪记录。</div>';
	traceState.totalPages = Number(result.total_pages) || 0;
	updatePagination('trace', traceState, result.total);
	body.querySelectorAll('[data-open-trace]').forEach(function (button) { button.addEventListener('click', function () { openTrace(button.dataset.openTrace); }); });
  }

  function updatePagination(prefix, state, total, unit) {
    const summary = document.querySelector('[data-' + prefix + '-page]');
    const prev = document.querySelector('[data-' + prefix + '-prev]');
    const next = document.querySelector('[data-' + prefix + '-next]');
    if (summary) summary.textContent = '第 ' + state.page + ' 页 / 共 ' + state.totalPages + ' 页 · ' + api.number(total || 0) + ' ' + (unit || '条');
    if (prev) prev.disabled = state.page <= 1;
    if (next) next.disabled = state.totalPages === 0 || state.page >= state.totalPages;
  }

  function bindUserFilters() {
    const form = document.querySelector('[data-admin-user-filter]');
    if (!form) return;
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      userState.keyword = form.elements.keyword.value.trim();
      userState.group = form.elements.group.value.trim();
      userState.role = form.elements.role.value;
      userState.status = form.elements.status.value;
      userState.sleeping = form.elements.sleeping.checked;
      userState.pageSize = Number(form.elements.page_size.value) || 50;
      userState.page = 1;
      try { await loadUsers(); } catch (error) { api.toast(error.message, 'danger'); }
    });
    form.addEventListener('reset', function () {
      window.setTimeout(async function () {
        Object.assign(userState, { page: 1, pageSize: 50, keyword: '', group: '', role: '', status: '', sleeping: false });
        try { await loadUsers(); } catch (error) { api.toast(error.message, 'danger'); }
      });
    });
    bindPager('admin-user', userState, loadUsers);
  }

  function bindLogFilters() {
    const form = document.querySelector('[data-admin-log-filter]');
    if (!form) return;
    const statusInput = form.elements.status_code;
    const exclude200Input = form.elements.exclude_200;
    const sortButtons = Array.from(document.querySelectorAll('[data-admin-log-sort]'));
    const syncStatusFilter = function () {
      statusInput.disabled = exclude200Input.checked;
      if (exclude200Input.checked) statusInput.value = '';
    };
    exclude200Input.addEventListener('change', syncStatusFilter);
    syncStatusFilter();
    syncLogSortHeaders();
    sortButtons.forEach(function (button) {
      button.addEventListener('click', async function () {
        const sortBy = button.dataset.adminLogSort;
        logState.sortOrder = logState.sortBy === sortBy && logState.sortOrder === 'desc' ? 'asc' : 'desc';
        logState.sortBy = sortBy;
        logState.page = 1;
        syncLogSortHeaders();
        try { await loadLogs(); } catch (error) { api.toast(error.message, 'danger'); }
      });
    });
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      logState.keyword = form.elements.keyword.value.trim();
      logState.exclude200 = exclude200Input.checked;
      logState.statusCode = logState.exclude200 ? '' : statusInput.value.trim();
      logState.pageSize = Number(form.elements.page_size.value) || 50;
      logState.page = 1;
      try { await loadLogs(); } catch (error) { api.toast(error.message, 'danger'); }
    });
    form.addEventListener('reset', function () {
      window.setTimeout(async function () {
        Object.assign(logState, { page: 1, pageSize: 50, keyword: '', statusCode: '', exclude200: false, sortBy: 'created_at', sortOrder: 'desc' });
        syncStatusFilter();
        syncLogSortHeaders();
        try { await loadLogs(); } catch (error) { api.toast(error.message, 'danger'); }
      });
    });
    bindPager('admin-log', logState, loadLogs);
    document.querySelector('[data-admin-log-refresh]').addEventListener('click', async function () {
      try { await loadLogs(); } catch (error) { api.toast(error.message, 'danger'); }
    });
  }

  function bindTraceFilters() {
	const form = document.querySelector('[data-trace-filter]');
	if (!form) return;
	form.addEventListener('submit', async function (event) {
	  event.preventDefault();
	  traceState.keyword = form.elements.keyword.value.trim();
	  traceState.status = form.elements.status.value;
	  traceState.pageSize = Number(form.elements.page_size.value) || 25;
	  traceState.page = 1;
	  try { await loadTraces(); } catch (error) { api.toast(error.message, 'danger'); }
	});
	form.addEventListener('reset', function () { window.setTimeout(async function () { Object.assign(traceState, { page: 1, pageSize: 25, keyword: '', status: '' }); try { await loadTraces(); } catch (error) { api.toast(error.message, 'danger'); } }); });
	bindPager('trace', traceState, loadTraces);
	document.querySelector('[data-trace-refresh]').addEventListener('click', function () { loadTraces().catch(function (error) { api.toast(error.message, 'danger'); }); });
  }

  function bindSubscriptionCycleReset() {
	const button = document.querySelector('[data-process-subscription-resets]');
	if (!button) return;
	button.addEventListener('click', async function () {
	  api.showBusy(button, true);
	  try {
		const result = await api.request('/api/admin/subscriptions/process-resets', { method: 'POST' });
		const summary = result.result || {};
		api.toast('周期检查完成：重置 ' + Number(summary.reset || 0) + ' 个，延后 ' + Number(summary.skipped || 0) + ' 个');
	  } catch (error) { api.toast(error.message, 'danger'); }
	  finally { api.showBusy(button, false); }
	});
  }

  function bindCodeFilters(prefix, state, loader) {
    const form = document.querySelector('[data-' + prefix + '-filter]');
    if (!form) return;
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      state.keyword = form.elements.keyword.value.trim();
      state.status = form.elements.status.value;
      if (form.elements.user_group) state.userGroup = form.elements.user_group.value;
      state.pageSize = Number(form.elements.page_size.value) || 25;
      state.page = 1;
      try { await loader(); } catch (error) { api.toast(error.message, 'danger'); }
    });
    form.addEventListener('reset', function () {
      window.setTimeout(async function () {
        Object.assign(state, { page: 1, pageSize: 25, keyword: '', status: '', userGroup: '' });
        try { await loader(); } catch (error) { api.toast(error.message, 'danger'); }
      });
    });
    bindPager(prefix, state, loader);
  }

  function bindPager(prefix, state, loader) {
    document.querySelector('[data-' + prefix + '-prev]').addEventListener('click', async function () {
      if (state.page <= 1) return;
      state.page -= 1;
      try { await loader(); } catch (error) { api.toast(error.message, 'danger'); }
    });
    document.querySelector('[data-' + prefix + '-next]').addEventListener('click', async function () {
      if (state.totalPages === 0 || state.page >= state.totalPages) return;
      state.page += 1;
      try { await loader(); } catch (error) { api.toast(error.message, 'danger'); }
    });
  }

  function resetChannelForm() {
    const form = document.querySelector('[data-channel-form]');
    if (!form) return;
    form.reset();
    form.elements.id.value = '';
    form.elements.type.value = '1';
    form.elements.upstream_format.value = 'openai_chat';
    form.querySelectorAll('input[name="upstream_formats"]').forEach(function (input) { input.checked = input.value === 'openai_chat'; });
    form.elements.priority.value = '0';
    form.elements.weight.value = '1';
    form.elements.groups.value = 'default';
	form.elements.requests_per_minute.value = '0';
	form.elements.concurrent_requests.value = '0';
	form.elements.data_train.checked = false;
	form.elements.remote_compaction.checked = false;
    api.syncQuotaUSD(form);
    document.querySelector('#channel-dialog .md-dialog-title').textContent = '添加渠道';
  }

  function bindChannelForm() {
    const form = document.querySelector('[data-channel-form]');
    if (!form) return;
    form.elements.upstream_format.addEventListener('change', function () {
      const primary = form.querySelector('input[name="upstream_formats"][value="' + form.elements.upstream_format.value + '"]');
      if (primary) primary.checked = true;
    });
    form.querySelectorAll('input[name="upstream_formats"]').forEach(function (input) {
      input.addEventListener('change', function () {
        const selected = Array.from(form.querySelectorAll('input[name="upstream_formats"]:checked'));
        if (!selected.length) {
          input.checked = true;
          return;
        }
        if (!selected.some(function (item) { return item.value === form.elements.upstream_format.value; })) {
          form.elements.upstream_format.value = selected[0].value;
        }
      });
    });
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      const formData = new FormData(form);
      const data = Object.fromEntries(formData.entries());
      data.upstream_formats = formData.getAll('upstream_formats');
	  data.data_train = form.elements.data_train.checked;
	  data.remote_compaction = form.elements.remote_compaction.checked;
      data.type = Number(data.type); data.priority = Number(data.priority); data.weight = Number(data.weight);
      data.quota_total = Number(data.quota_total);
      data.quota_remaining = Number(data.quota_remaining);
	  data.requests_per_minute = Number(data.requests_per_minute) || 0;
	  data.concurrent_requests = Number(data.concurrent_requests) || 0;
      const editing = Boolean(data.id);
      try {
        await api.request(editing ? '/api/admin/channels/' + data.id : '/api/admin/channels', { method: editing ? 'PUT' : 'POST', body: data });
        api.closeDialog('channel-dialog');
        api.toast(editing ? '渠道已更新' : '渠道已创建');
        await loadChannels();
        await loadOverview();
      } catch (error) { api.toast(error.message, 'danger'); }
    });
  }

  function channelKeyStatus(status) {
    return Number(status) === 1 ? '<span class="tag success">启用</span>' : Number(status) === 3 ? '<span class="tag error">自动下线</span>' : '<span class="tag muted">停用</span>';
  }

  function channelKeyRow(item) {
    const quota = Number(item.quota_remaining) < 0 ? '不限额' : api.moneyFromQuota(item.quota_remaining, 2);
    const balanceAction = activeChannelStatus === 1 && Number(item.status) === 1 && Number(item.quota_remaining) === 0 ? '<button type="button" class="md-icon-button" title="立即复查余额" data-refresh-dialog-key-balance="' + Number(item.id) + '"><span class="material-symbol" aria-hidden="true">account_balance_wallet</span></button>' : '';
    return '<div class="list-item"><span class="material-symbol" aria-hidden="true">key</span><div class="list-item-body"><div class="list-item-title">#' + Number(item.id) + ' Key · ' + api.escapeHTML(item.label || '未命名 Key') + '</div><div class="list-item-sub"><code>' + api.escapeHTML(item.masked_key || '') + '</code> · 权重 ' + Number(item.weight || 1) + ' · ' + (Number(item.requests_per_minute) > 0 ? Number(item.requests_per_minute) + ' RPM' : 'RPM 跟随渠道') + ' · ' + (Number(item.concurrent_requests) > 0 ? Number(item.concurrent_requests) + ' 并发' : '并发跟随渠道') + ' · 失败 ' + Number(item.failure_count || 0) + ' 次</div></div><div class="list-metrics"><div><b>' + quota + '</b>剩余</div></div><div class="list-actions">' + channelKeyStatus(item.status) + balanceAction + '<button type="button" class="md-icon-button" title="编辑 #' + Number(item.id) + ' Key" data-edit-channel-key="' + item.id + '"><span class="material-symbol" aria-hidden="true">edit</span></button><button type="button" class="md-icon-button danger" title="删除 #' + Number(item.id) + ' Key" data-delete-channel-key="' + item.id + '"><span class="material-symbol" aria-hidden="true">delete</span></button></div></div>';
  }

  async function loadChannelKeys() {
    const body = document.querySelector('[data-channel-key-rows]');
    if (!body || !activeChannelID) return;
    const result = await api.request('/api/admin/channels/' + activeChannelID + '/keys');
    const rows = Array.isArray(result) ? result : [];
    body.innerHTML = rows.length ? rows.map(channelKeyRow).join('') : '<div class="empty">该渠道还没有独立 Key。</div>';
    body.querySelectorAll('[data-edit-channel-key]').forEach(function (button) {
      button.addEventListener('click', function () {
        const item = rows.find(function (key) { return String(key.id) === button.dataset.editChannelKey; });
        if (item) editChannelKey(item);
      });
    });
    body.querySelectorAll('[data-refresh-dialog-key-balance]').forEach(function (button) {
      button.addEventListener('click', async function () {
        api.showBusy(button, true);
        try {
          const summary = await api.request('/api/admin/channels/' + activeChannelID + '/keys/' + button.dataset.refreshDialogKeyBalance + '/refresh-balance', { method: 'POST' });
          api.toast('余额复查完成' + (Number(summary.recovered) ? '，Key 已恢复' : ''));
          await loadChannelKeys();
          await loadChannels();
        } catch (error) { api.toast(error.message || '余额复查失败', 'danger'); api.showBusy(button, false); }
      });
    });
    body.querySelectorAll('[data-delete-channel-key]').forEach(function (button) {
      button.addEventListener('click', async function () {
        if (!(await api.confirm('删除后该渠道 Key 将立即失效且无法恢复。', { title: '删除渠道 Key', confirmText: '删除' }))) return;
        try {
          await api.request('/api/admin/channels/' + activeChannelID + '/keys/' + button.dataset.deleteChannelKey, { method: 'DELETE' });
          api.toast('渠道 Key 已删除');
          await loadChannelKeys();
          await loadChannels();
        } catch (error) { api.toast(error.message, 'danger'); }
      });
    });
  }

  function resetChannelKeyForm() {
    const form = document.querySelector('[data-channel-key-form]');
    if (!form) return;
    form.reset();
    form.elements.channel_id.value = activeChannelID;
    form.elements.id.value = '';
    form.elements.weight.value = '1';
    form.elements.status.value = '1';
    form.elements.quota_total.value = '-1';
    form.elements.quota_remaining.value = '-1';
	form.elements.requests_per_minute.value = '0';
    form.elements.concurrent_requests.value = '0';
    api.syncQuotaUSD(form);
    api.setText('[data-channel-key-dialog-title]', '#' + activeChannelID + ' 渠道 · 新增 Key');
    api.setText('[data-channel-key-dialog-hint]', '');
    form.classList.remove('hidden');
  }

  function editChannelKey(item) {
    const form = document.querySelector('[data-channel-key-form]');
    if (!form) return;
    form.elements.channel_id.value = activeChannelID;
    form.elements.id.value = item.id;
    form.elements.label.value = item.label || '';
    form.elements.key.value = '';
    form.elements.weight.value = item.weight || 1;
    form.elements.status.value = item.status || 1;
    form.elements.quota_total.value = item.quota_total == null ? -1 : item.quota_total;
    form.elements.quota_remaining.value = item.quota_remaining == null ? -1 : item.quota_remaining;
	form.elements.requests_per_minute.value = item.requests_per_minute || 0;
    form.elements.concurrent_requests.value = item.concurrent_requests || 0;
    api.syncQuotaUSD(form);
    api.setText('[data-channel-key-dialog-title]', '#' + activeChannelID + ' 渠道 · #' + Number(item.id) + ' Key');
    api.setText('[data-channel-key-dialog-hint]', item.label || '未命名 Key');
    form.classList.remove('hidden');
  }

  function openChannelKeyDialog(channel) {
    activeChannelID = Number(channel.id);
    activeChannelStatus = Number(channel.status);
    api.setText('[data-channel-key-dialog-title]', '#' + Number(channel.id) + ' 渠道 · Key');
    api.setText('[data-channel-key-dialog-hint]', channel.name || '未命名渠道');
    document.querySelector('[data-channel-key-form]').classList.add('hidden');
    api.openDialog('channel-key-dialog');
    loadChannelKeys().catch(function (error) { api.toast(error.message, 'danger'); });
  }

  function bindChannelKeyForm() {
    const form = document.querySelector('[data-channel-key-form]');
    if (!form) return;
    document.querySelector('[data-channel-key-add]').addEventListener('click', resetChannelKeyForm);
    document.querySelector('[data-channel-key-cancel]').addEventListener('click', function () { form.classList.add('hidden'); });
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      const values = Object.fromEntries(new FormData(form).entries());
      const data = {
        label: values.label.trim(),
        key: values.key.trim(),
        weight: Number(values.weight) || 1,
        quota_total: Number(values.quota_total),
        quota_remaining: Number(values.quota_remaining),
		requests_per_minute: Number(values.requests_per_minute) || 0,
		concurrent_requests: Number(values.concurrent_requests) || 0,
        status: Number(values.status)
      };
      const editing = Boolean(values.id);
      try {
        await api.request(editing ? '/api/admin/channels/' + activeChannelID + '/keys/' + values.id : '/api/admin/channels/' + activeChannelID + '/keys', { method: editing ? 'PUT' : 'POST', body: data });
        form.classList.add('hidden');
        api.toast(editing ? '渠道 Key 已更新' : '渠道 Key 已添加');
        await loadChannelKeys();
        await loadChannels();
      } catch (error) { api.toast(error.message, 'danger'); }
    });
  }

  function bindUserForm() {
    const form = document.querySelector('[data-user-form]');
    if (!form) return;
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      const data = Object.fromEntries(new FormData(form).entries());
      data.quota = Number(data.quota); data.status = Number(data.status); data.role = Number(data.role);
      try {
        await api.request('/api/admin/users/' + data.id, { method: 'PATCH', body: data });
        api.closeDialog('user-dialog');
        api.toast('用户已更新');
        await loadUsers();
        await loadOverview();
      } catch (error) { api.toast(error.message, 'danger'); }
    });
  }

  function bindRedeemForm() {
    const form = document.querySelector('[data-redeem-form]');
    if (!form) return;
    function selectedPlanChain() {
      const chain = [];
      const visited = {};
      let plan = subscriptionPlans.find(function (item) { return String(item.id) === String(form.elements.subscription_plan_id.value); });
      while (plan && !visited[plan.id]) {
        visited[plan.id] = true;
        chain.unshift(plan);
        if (Number(plan.upgrade_from_plan_id) <= 0) break;
        plan = subscriptionPlans.find(function (item) { return Number(item.id) === Number(plan.upgrade_from_plan_id); });
      }
      return chain;
    }
    function updateFields() {
      const subscription = form.elements.redeem_type.value === 'subscription';
	  const extension = form.elements.redeem_type.value === 'subscription_extension';
	  const needsPlan = subscription || extension;
      const usd = form.elements.denomination.value === 'usd';
      const publicCode = form.elements.is_public.checked;
      const planChain = selectedPlanChain();
      const canIncludeBasePlans = subscription && planChain.length > 1;
	  document.querySelector('[data-redeem-plan-field]').classList.toggle('hidden', !needsPlan);
	  document.querySelector('[data-redeem-extension-days-field]').classList.toggle('hidden', !extension);
	  document.querySelector('[data-redeem-extension-hint]').classList.toggle('hidden', !extension);
	  document.querySelector('[data-redeem-base-plans-field]').classList.toggle('hidden', !canIncludeBasePlans);
	  document.querySelector('[data-redeem-denomination-field]').classList.toggle('hidden', needsPlan);
	  document.querySelector('[data-redeem-quota-group-field]').classList.toggle('hidden', needsPlan);
	  document.querySelector('[data-redeem-quota-field]').classList.toggle('hidden', needsPlan || usd);
	  document.querySelector('[data-redeem-usd-field]').classList.toggle('hidden', needsPlan || !usd);
      document.querySelector('[data-redeem-count-field]').classList.toggle('hidden', publicCode);
      document.querySelector('[data-redeem-limit-field]').classList.toggle('hidden', !publicCode);
      form.elements.count.required = !publicCode;
      form.elements.redeem_limit.required = publicCode;
	  form.elements.subscription_plan_id.required = needsPlan;
	  form.elements.subscription_extension_days.required = extension;
	  form.elements.denomination.required = !needsPlan;
	  form.elements.quota.required = !needsPlan && !usd;
	  form.elements.amount_usd.required = !needsPlan && usd;
	  if (!canIncludeBasePlans) form.elements.include_base_plans.checked = false;
	  const chainDescription = document.querySelector('[data-redeem-base-plans-desc]');
	  if (chainDescription && canIncludeBasePlans) chainDescription.textContent = '兑换时依次激活 ' + planChain.map(function (plan) { return plan.name; }).join(' → ') + '，无需预先拥有基础计划。';
    }
    form.elements.redeem_type.addEventListener('change', updateFields);
    form.elements.denomination.addEventListener('change', updateFields);
    form.elements.subscription_plan_id.addEventListener('change', updateFields);
    form.elements.is_public.addEventListener('change', updateFields);
    updateFields();
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      const values = Object.fromEntries(new FormData(form).entries());
      const publicCode = form.elements.is_public.checked;
      const data = { name: values.name, count: publicCode ? 1 : Number(values.count), user_group: values.user_group, redeem_type: values.redeem_type, quota_group: values.quota_group.trim(), is_public: publicCode, redeem_limit: publicCode ? Number(values.redeem_limit) : 1 };
      if (values.redeem_type === 'subscription') {
        data.subscription_plan_id = Number(values.subscription_plan_id);
		data.include_base_plans = form.elements.include_base_plans.checked;
	  } else if (values.redeem_type === 'subscription_extension') {
		data.subscription_plan_id = Number(values.subscription_plan_id);
		data.subscription_extension_days = Number(values.subscription_extension_days);
      } else if (values.denomination === 'usd') {
        data.amount_usd = Number(values.amount_usd);
      } else {
        data.quota = Number(values.quota);
      }
      try {
        const result = await api.request('/api/admin/redemptions/generate', { method: 'POST', body: data });
        api.closeDialog('redeem-dialog');
		const siteLabel = result.user_group === 'pay' ? '商业站（pay）' : '公益站（default）';
		openRedemptionCodeDialog(result.codes || [], '兑换码已生成 · ' + siteLabel);
		api.toast(publicCode ? '公共兑换码已生成' : (data.include_base_plans ? '直升兑换码已生成' : (data.redeem_type === 'subscription_extension' ? '展期兑换码已生成' : '兑换码已生成')));
        await loadRedemptions();
      } catch (error) { api.toast(error.message, 'danger'); }
    });
  }

  function bindRegistrationCodeForm() {
    const form = document.querySelector('[data-registration-code-form]');
    if (!form) return;
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      const data = Object.fromEntries(new FormData(form).entries());
      data.count = Number(data.count);
      try {
        const result = await api.request('/api/admin/registration-codes/generate', { method: 'POST', body: data });
        api.closeDialog('registration-code-dialog');
        openRegistrationCodeDialog(result.codes || [], '注册码已生成');
        api.toast('注册码已生成');
        await loadRegistrationCodes();
        await adminSettings.loadRegistrationSettings();
      } catch (error) { api.toast(error.message, 'danger'); }
    });
  }

  function bindCopyCodes() {
    const button = document.querySelector('[data-copy-codes]');
    if (!button) return;
    button.addEventListener('click', async function () {
      try { await navigator.clipboard.writeText(document.querySelector('[data-generated-codes]').value); api.toast('已复制'); } catch (_) { api.toast('复制失败，请手动保存', 'danger'); }
    });
  }

  function bindCopyRegistrationCodes() {
    const button = document.querySelector('[data-copy-registration-codes]');
    if (!button) return;
    button.addEventListener('click', async function () {
      try { await navigator.clipboard.writeText(document.querySelector('[data-generated-registration-codes]').value); api.toast('已复制'); } catch (_) { api.toast('复制失败，请手动保存', 'danger'); }
    });
  }

  function renderAdminSecurityStatus(status) {
    adminSecurityStatus = status || { totp_enabled: false, pending: false };
    const enabled = Boolean(adminSecurityStatus.totp_enabled);
    api.setText('[data-admin-totp-status]', enabled ? '动态验证已启用' : '动态验证未启用');
    api.setText('[data-admin-totp-hint]', enabled ? '敏感操作将使用 6 位动态验证码完成二次验证。' : (adminSecurityStatus.pending ? '已有待确认的绑定，可重新开始并生成新密钥。' : '当前使用登录密码进行二次验证。'));
    const setup = document.querySelector('[data-admin-totp-setup]');
    const disable = document.querySelector('[data-admin-totp-disable-form]');
    if (setup) setup.classList.toggle('hidden', enabled);
    if (disable) disable.classList.toggle('hidden', !enabled);
  }

  async function loadAdminSecurity() {
    const status = await api.request('/api/admin/security');
    renderAdminSecurityStatus(status);
  }

  async function loadAdminAudit() {
    const params = new URLSearchParams({ page: String(auditState.page), page_size: String(auditState.pageSize) });
    if (auditState.keyword) params.set('keyword', auditState.keyword);
    if (auditState.action) params.set('action', auditState.action);
    const payload = await api.request('/api/admin/security/audit?' + params.toString());
    const items = Array.isArray(payload && payload.items) ? payload.items : [];
    const body = document.querySelector('[data-admin-audit-rows]');
    if (!body) return;
    body.innerHTML = items.length ? items.map(function (item) {
      const actor = item.username || ('用户 #' + item.user_id);
      const target = item.target ? '目标 #' + api.escapeHTML(item.target) : '无指定目标';
      return '<div class="list-item"><span class="material-symbol" aria-hidden="true">lock</span><div class="list-item-body"><div class="list-item-title">' + api.escapeHTML(item.action) + '</div><div class="list-item-sub">' + api.escapeHTML(actor) + ' · ' + api.escapeHTML(item.client_ip || '未知 IP') + ' · ' + api.date(item.created_at) + '</div></div><div class="list-metrics"><div><b>' + target + '</b>操作目标</div><div><b><code>' + api.escapeHTML(item.request_id || '—') + '</code></b>请求 ID</div></div></div>';
    }).join('') : '<div class="empty">还没有敏感操作审计记录。</div>';
    auditState.totalPages = Number(payload.total_pages) || 0;
    updatePagination('admin-audit', auditState, payload.total);
  }

  function bindAdminAudit() {
    const form = document.querySelector('[data-admin-audit-filter]');
    if (!form) return;
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      auditState.keyword = form.elements.keyword.value.trim();
      auditState.action = form.elements.action.value.trim();
      auditState.pageSize = Number(form.elements.page_size.value) || 50;
      auditState.page = 1;
      try { await loadAdminAudit(); } catch (error) { api.toast(error.message, 'danger'); }
    });
    form.addEventListener('reset', function () {
      window.setTimeout(async function () {
        Object.assign(auditState, { page: 1, pageSize: 50, keyword: '', action: '' });
        try { await loadAdminAudit(); } catch (error) { api.toast(error.message, 'danger'); }
      });
    });
    bindPager('admin-audit', auditState, loadAdminAudit);
    document.querySelector('[data-admin-audit-refresh]').addEventListener('click', function () { loadAdminAudit().catch(function (error) { api.toast(error.message, 'danger'); }); });

  }

  function bindAdminSecurity() {
    const setup = document.querySelector('[data-admin-totp-setup]');
    if (setup) setup.addEventListener('click', async function () {
      api.showBusy(setup, true);
      try {
        const result = await api.request('/api/admin/security/totp/setup', { method: 'POST', body: {} });
        api.setText('[data-admin-totp-secret]', result.secret || '—');
        api.setText('[data-admin-totp-uri]', result.otpauth_uri || '—');
		const qrCode = document.querySelector('[data-admin-totp-qr]');
		if (qrCode) {
		  const source = result.qr_code_data_uri || '';
		  if (source) qrCode.src = source; else qrCode.removeAttribute('src');
		  qrCode.classList.toggle('hidden', !source);
		}
        document.querySelector('[data-admin-totp-enable-form]').reset();
        api.openDialog('admin-totp-setup-dialog');
      } catch (error) { api.toast(error.message, 'danger'); }
      finally { api.showBusy(setup, false); }
    });

    const enableForm = document.querySelector('[data-admin-totp-enable-form]');
    if (enableForm) enableForm.addEventListener('submit', async function (event) {
      event.preventDefault();
      try {
        await api.request('/api/admin/security/totp/enable', { method: 'POST', body: { code: enableForm.elements.code.value.trim() } });
        api.clearAdminProof();
        api.closeDialog('admin-totp-setup-dialog');
        api.toast('管理员动态验证已启用');
        await loadAdminSecurity();
      } catch (error) { api.toast(error.message, 'danger'); }
    });

    const disableForm = document.querySelector('[data-admin-totp-disable-form]');
    if (disableForm) disableForm.addEventListener('submit', async function (event) {
      event.preventDefault();
      if (!(await api.confirm('关闭后，敏感操作将退回使用登录密码验证。', { title: '关闭动态验证', confirmText: '关闭' }))) return;
      try {
        await api.request('/api/admin/security/totp/disable', { method: 'POST', body: { code: disableForm.elements.code.value.trim() } });
        api.clearAdminProof();
        disableForm.reset();
        api.toast('管理员动态验证已关闭');
        await loadAdminSecurity();
      } catch (error) { api.toast(error.message, 'danger'); }
    });

  }

  document.addEventListener('DOMContentLoaded', async function () {
    if (!api.token()) { location.href = '/auth/?mode=login'; return; }
    const legacyTraceHash = location.hash === '#traces';
    if (legacyTraceHash) history.replaceState(null, '', '#logs');
    api.mountShell();
    bindChannelSubpages(); bindChannelVisibilityFilter();
    api.bindQuotaUSDFields(document);
    document.querySelectorAll('[data-open-dialog="channel-dialog"]').forEach(function (button) { button.addEventListener('click', resetChannelForm); });
    document.querySelectorAll('[data-open-dialog="model-dialog"]').forEach(function (button) { button.addEventListener('click', adminModels.resetForm); });
    document.querySelectorAll('[data-open-dialog="model-routing-rule-dialog"]').forEach(function (button) { button.addEventListener('click', function () { if (window.ProxyLanAdminRouting) window.ProxyLanAdminRouting.resetRuleForm(); }); });
    document.querySelectorAll('[data-open-dialog="model-alias-dialog"]').forEach(function (button) { button.addEventListener('click', function () { if (window.ProxyLanAdminRouting) window.ProxyLanAdminRouting.resetAliasForm(); }); });
    document.querySelectorAll('[data-open-dialog="subscription-plan-dialog"]').forEach(function (button) { button.addEventListener('click', resetSubscriptionPlanForm); });
    document.querySelectorAll('[data-open-dialog="subscription-benefit-dialog"]').forEach(function (button) { button.addEventListener('click', resetSubscriptionBenefitForm); });
	document.querySelectorAll('[data-open-dialog="billing-promotion-dialog"]').forEach(function (button) { button.addEventListener('click', resetBillingPromotionForm); });
	adminOverview.bind();
    bindChannelForm(); adminModels.bind(); if (window.ProxyLanAdminRouting) window.ProxyLanAdminRouting.bind(); bindSubscriptionPlanForm(); bindSubscriptionBenefitForm(); bindBillingPromotionForm(); bindSubscriptionCycleReset(); bindChannelKeyForm(); bindUserForm(); bindRedeemForm(); bindCopyCodes(); bindRegistrationCodeForm(); bindCopyRegistrationCodes(); bindUsedCodeCleanup(); bindPrepaidCards(); bindUserFilters(); bindLogFilters(); bindTraceFilters(); bindLogViewSwitch(); bindAdminAudit(); bindCodeFilters('redemption', redemptionState, loadRedemptions); bindCodeFilters('registration-code', registrationCodeState, loadRegistrationCodes); adminSettings.bind(); bindAdminSecurity();
    if (legacyTraceHash) setLogView('traces');
    try {
      const self = await api.request('/api/user/self');
      currentAdminRole = Number(self.role) || 0;
      if (currentAdminRole < 25) { location.href = '/console/'; return; }
      applyAdminCapabilities();
      if (currentAdminRole < 50 && channelSubpageFromHash() === 'keys') showChannelSubpage('index', 'replace');
      if (channelSubpageFromHash() === 'keys') ensureChannelKeyManagement();
      if (channelSubpageFromHash() === 'disabled') ensureDisabledChannelManagement();
      const capabilityObserver = new MutationObserver(function () { applyAdminCapabilities(); });
      capabilityObserver.observe(document.querySelector('.content-area'), { childList: true, subtree: true });
      window.ProxyLanResources.mount(resources, resourcesForPage);
      applyAdminCapabilities();
    } catch (error) { api.toast(error.message, 'danger'); }
  });
}());
