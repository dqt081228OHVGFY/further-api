(function () {
  'use strict';
  const api = window.FurtherUI;
  const adminOverview = window.FurtherUIAdminOverview;
  const adminSettings = window.FurtherUIAdminSettings;
  const adminModels = window.FurtherUIAdminModels;
  const userState = { page: 1, pageSize: 50, keyword: '', group: '', role: '', status: '', sleeping: false, totalPages: 0, loading: false };
  let userLoadSequence = 0;
  const logState = { page: 1, pageSize: 50, keyword: '', statusCode: '', exclude200: false, sortBy: 'created_at', sortOrder: 'desc', totalPages: 0 };
  const traceState = { page: 1, pageSize: 25, keyword: '', status: '', totalPages: 0 };
  const redemptionState = { page: 1, pageSize: 25, keyword: '', status: '', userGroup: '', totalPages: 0 };
  const redemptionExpanded = new Set();
  const redemptionCollapsed = new Set();
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
	let activeTraceRequestID = '';
	let activeLogView = 'calls';
  const resources = window.FurtherUIResources.create({
    overview: loadOverview, channels: loadChannels, models: adminModels.load,
    subscriptions: loadSubscriptionPlans, benefits: loadSubscriptionBenefitCampaigns,
    users: loadUsers, tokenUsage: function () { return window.FurtherUIAdminTokenStats ? window.FurtherUIAdminTokenStats.load() : Promise.resolve(); }, redemptions: loadRedemptions, registrationCodes: loadRegistrationCodes,
    prepaidCards: loadPrepaidCards, logs: loadLogs, traces: loadTraces,
    settings: adminSettings.load
  });

  function resourcesForPage(name) {
    const pages = {
      overview: ['overview'], channels: ['channels'], models: ['models', 'channels'],
      subscriptions: ['subscriptions', 'benefits', 'models'], users: ['users', 'tokenUsage'],
      redemptions: ['redemptions'], 'registration-codes': ['registrationCodes'],
      'prepaid-cards': ['prepaidCards'], logs: [activeLogView === 'traces' ? 'traces' : 'logs'],
      settings: currentAdminRole >= 100 ? ['settings'] : [],

    };
    return pages[name] || [];
  }


  function adminRoleLabel(role) {
    return ({ 1: '用户', 25: '审计员', 50: '管理员', 100: '超级管理员' })[Number(role)] || '未知';
  }

  function applyAdminCapabilities() {
    const root = currentAdminRole >= 100;
    const operator = currentAdminRole >= 50;
    document.body.dataset.adminRole = root ? 'root' : (operator ? 'operator' : 'auditor');
    document.querySelectorAll('[data-root-only]').forEach(function (element) { element.classList.toggle('hidden', !root); });
    const rootSelectors = '[data-open-dialog], [data-edit-channel], [data-delete-channel], [data-edit-model], [data-delete-model], [data-edit-subscription-plan], [data-edit-billing-promotion], [data-delete-billing-promotion], [data-delete-redemption], [data-delete-redemption-batch], [data-delete-registration-code], [data-delete-registration-code-batch], [data-delete-prepaid-card], [data-delete-prepaid-batch], [data-channel-key-add], [data-edit-channel-key], [data-delete-channel-key]';
    document.querySelectorAll(rootSelectors).forEach(function (element) { element.classList.toggle('hidden', !root); });
	const operatorSelectors = '[data-toggle-channel], [data-toggle-model], [data-toggle-subscription-plan], [data-toggle-billing-promotion], [data-manage-channel-keys], [data-channel-key-management-open], [data-channel-pull-models], [data-probe-channel], [data-channel-operation], [data-process-subscription-resets]';
    document.querySelectorAll(operatorSelectors).forEach(function (element) { element.classList.toggle('hidden', !operator); });
    document.querySelectorAll('[data-user-target-role]').forEach(function (element) {
      const targetRole = Number(element.dataset.userTargetRole);
      const allowed = canManageUser(targetRole) && !(element.hasAttribute('data-toggle-user') && targetRole >= 100 && Number(element.dataset.nextStatus) === 2);
      element.classList.toggle('hidden', !allowed);
    });
    if (!root && (document.querySelector('[data-panel="settings"].active'))) {
      const overview = document.querySelector('[data-nav="overview"]');
      if (overview) overview.click();
    }
  }

  function canManageUser(role) {
    return currentAdminRole >= 100 || (currentAdminRole >= 50 && currentAdminRole > Number(role));
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
    if (channelKeyManagementReady || currentAdminRole < 50 || !window.FurtherUIChannelKeys) return;
    channelKeyManagementReady = true;
    try {
      await window.FurtherUIChannelKeys.init({ role: currentAdminRole });
    } catch (error) {
      channelKeyManagementReady = false;
      api.toast(error.message || '加载渠道 Key 失败', 'danger');
    }
  }

  async function ensureDisabledChannelManagement() {
    if (disabledChannelManagementReady || currentAdminRole < 25 || !window.FurtherUIDisabledChannels) return;
    disabledChannelManagementReady = true;
    try {
      await window.FurtherUIDisabledChannels.init({ role: currentAdminRole });
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
	  if (channel.health_unavailable) return '';
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
	const provider = channel.provider_name || (Number(channel.type) === 2 ? 'Anthropic' : Number(channel.type) === 1 ? 'OpenAI' : 'Custom');
	const channelTag = channel.tag ? '<span class="tag">' + api.escapeHTML(channel.tag) + '</span>' : '';
	const probe = channel.last_probe;
	const probeTag = channel.health_unavailable ? '' : probe ? '<span class="tag ' + (probe.success ? 'success' : 'error') + '">探测 ' + (probe.success ? (Number(probe.latency_ms) + ' ms') : ('HTTP ' + (probe.status_code || '失败'))) + '</span>' : '<span class="tag muted">尚未探测</span>';
	const maintenance = channel.maintenance_note ? '<div class="list-item-sub">维护：' + api.escapeHTML(channel.maintenance_note) + ' · ' + api.date(channel.maintenance_at) + '</div>' : '';
	const operation = Number(channel.status) === 1 ? 'drain' : 'restore';
    return '<div class="channel-row">' +
      '<span class="channel-radio"><span class="material-symbol" aria-hidden="true">hub</span></span>' +
	  '<div class="channel-body"><div class="channel-name">' + api.escapeHTML(channel.name) + '</div><div class="channel-target">' + api.escapeHTML(channel.base_url || '使用供应商默认地址') + '</div><div class="tags"><span class="tag">' + api.escapeHTML(provider) + '</span><span class="tag">优先级 ' + Number(channel.priority || 0) + '</span><span class="tag">权重 ' + Number(channel.weight || 0) + '</span><span class="tag">Key ' + api.number(channel.key_count) + '</span><span class="tag">模型 ' + String(channel.models || '').split(',').filter(function (id) { return id.trim(); }).length + '</span>' + channelTag + probeTag + '</div>' + maintenance + channelHealthMarkup(channel) + '<button type="button" class="md-btn md-btn-text md-btn-compact" data-channel-pull-models="' + channel.id + '"><span class="material-symbol" aria-hidden="true">cloud_download</span>从上游拉取模型</button></div>' +
      '<div class="channel-actions"><span>' + api.statusLabel(channel.status, 'channel') + '</span><button type="button" class="md-icon-button" title="立即探测" data-probe-channel="' + channel.id + '"><span class="material-symbol" aria-hidden="true">monitoring</span></button><button type="button" class="md-icon-button" title="管理渠道 Key" data-manage-channel-keys="' + channel.id + '"><span class="material-symbol" aria-hidden="true">key</span></button><button type="button" class="md-icon-button" title="编辑" data-edit-channel="' + channel.id + '"><span class="material-symbol" aria-hidden="true">edit</span></button><button type="button" class="md-icon-button" title="' + (operation === 'drain' ? '摘除渠道' : '恢复渠道') + '" data-channel-operation="' + operation + '" data-channel-id="' + channel.id + '"><span class="material-symbol" aria-hidden="true">' + (operation === 'drain' ? 'close' : 'check') + '</span></button><button type="button" class="md-icon-button danger" title="删除" data-delete-channel="' + channel.id + '"><span class="material-symbol" aria-hidden="true">delete</span></button></div>' +
      '</div>';
  }

  async function loadChannels(refresh) {
	if (refresh !== false) {
	  const result = await api.request('/api/admin/channels');
	  channelRows = Array.isArray(result) ? result : [];
	  // 模型编辑器要用渠道列表填绑定下拉框，推给模型模块避免它重复请求一次
	  adminModels.setChannels(channelRows);
	  if (window.FurtherUIAdminChannelModels) window.FurtherUIAdminChannelModels.setChannels(channelRows);
	}
	const rows = hideDisabledChannels ? channelRows.filter(function (channel) { return Number(channel.status) === 1; }) : channelRows;
    const body = document.querySelector('[data-admin-channel-rows]');
    if (!body) return;
	body.innerHTML = rows.length ? rows.map(channelRow).join('') : '<div class="empty">' + (channelRows.length ? '停用和自动下线的渠道已隐藏，可关闭筛选后查看。' : '还没有配置渠道。') + '</div>';
    body.querySelectorAll('[data-channel-pull-models]').forEach(function (button) {
      button.addEventListener('click', function () {
        if (window.FurtherUIAdminChannelModels) window.FurtherUIAdminChannelModels.open({ channelID: Number(button.dataset.channelPullModels) });
      });
    });
    body.querySelectorAll('[data-edit-channel]').forEach(function (button) {
      button.addEventListener('click', function () {
        const channel = rows.find(function (item) { return String(item.id) === button.dataset.editChannel; });
        const form = document.querySelector('[data-channel-form]');
        form.elements.id.value = channel.id;
        form.elements.name.value = channel.name || '';
		const providerType = Number(channel.provider_type || channel.type || 1);
		const providerOptions = Array.from(form.elements.provider_type.options);
		if (!providerOptions.some(function (option) { return Number(option.value) === providerType; })) {
		  const option = document.createElement('option');
		  option.value = String(providerType);
		  option.textContent = channel.provider_name || '类型 ' + providerType;
		  form.elements.provider_type.add(option);
		}
		form.elements.provider_type.value = String(providerType);
		form.elements.type.value = providerType === 14 ? '2' : providerType === 1 ? '1' : '3';
        form.elements.priority.value = channel.priority;
        form.elements.weight.value = channel.weight;
        form.elements.groups.value = channel.groups || 'default';
        form.elements.base_url.value = channel.base_url || '';
		form.elements.key.value = '';
		form.elements.key.classList.add('channel-key-masked');
		form.querySelector('[data-channel-key-visibility]').setAttribute('aria-pressed', 'false');
		form.querySelector('[data-channel-key-visibility]').textContent = '显示 Key';
        form.elements.models.value = channel.models || '';
        form.elements.model_mapping.value = channel.model_mapping || '';
		['tag', 'remark', 'test_model', 'openai_organization', 'other', 'status_code_mapping', 'param_override', 'header_override', 'setting', 'settings'].forEach(function (name) {
		  const value = channel[name];
		  form.elements[name].value = value && typeof value === 'object' ? JSON.stringify(value, null, 2) : value || '';
		});
		form.elements.auto_ban.value = String(channel.auto_ban == null ? 1 : channel.auto_ban);
		const multiKey = Boolean(channel.channel_info && channel.channel_info.is_multi_key);
		form.dataset.multiKey = multiKey ? 'true' : 'false';
		form.elements.edit_multi_key_type.value = channel.channel_info && channel.channel_info.multi_key_mode === 'polling' ? 'polling' : 'random';
		form.elements.key_mode.value = 'append';
		form.querySelector('[data-channel-advanced]').open = ['tag', 'remark', 'test_model', 'openai_organization', 'other', 'status_code_mapping', 'param_override', 'header_override'].some(function (name) { return Boolean(channel[name]); }) || ['setting', 'settings'].some(function (name) { return Boolean(channel[name]) && channel[name] !== '{}'; }) || Number(channel.auto_ban) === 0;
		syncChannelFormMode(form);
		if (window.FurtherUIAdminChannelModels) window.FurtherUIAdminChannelModels.resetForm(form);
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
    const quota = plan.unlimited_quota ? '不限额' : api.moneyFromQuota(plan.quota, 2);
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
	parts.push('生效中的订阅');
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
    const count = document.querySelector('[data-subscription-count="benefits"]');
    if (count) count.textContent = api.number(rows.length);
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
        target_user_group: '',
        permanent_quota: Number(form.elements.permanent_quota.value) || 0,
        timed_quota: Number(form.elements.timed_quota.value) || 0,
		quota_group: '',
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















  function bindSubscriptionTabs() {
    const workspace = document.querySelector('[data-panel="subscriptions"]');
    if (!workspace) return;
    const tabs = ['paid', 'site', 'benefits'];
    function sync() {
      if (location.hash === '#subscriptions/billing') history.replaceState(null, '', '#subscriptions/paid');
      const selected = location.hash.match(/^#subscriptions\/(paid|site|benefits)$/)?.[1] || 'paid';
      workspace.querySelectorAll('[data-subscription-tab]').forEach(function (button) {
        const active = button.dataset.subscriptionTab === selected;
        button.classList.toggle('active', active);
        button.setAttribute('aria-pressed', String(active));
        button.setAttribute('aria-selected', String(active));
        button.tabIndex = active ? 0 : -1;
      });
      workspace.querySelectorAll('[data-subscription-page]').forEach(function (page) {
        page.hidden = page.dataset.subscriptionPage !== selected;
      });
      if (selected === 'paid' && /^#subscriptions(?:\/|$)/.test(location.hash) && window.FurtherUIVIP) window.FurtherUIVIP.load({ reuse: true });
    }
    workspace.querySelectorAll('[data-subscription-tab]').forEach(function (button) {
      button.addEventListener('keydown', function (event) {
        if (!['ArrowRight', 'ArrowLeft', 'Home', 'End'].includes(event.key)) return;
        event.preventDefault();
        const index = tabs.indexOf(button.dataset.subscriptionTab);
        const next = event.key === 'Home' ? 0 : event.key === 'End' ? tabs.length - 1 : (index + (event.key === 'ArrowRight' ? 1 : -1) + tabs.length) % tabs.length;
        const target = workspace.querySelector('[data-subscription-tab="' + tabs[next] + '"]');
        target.click(); target.focus();
      });
      button.addEventListener('click', function () {
        if (!tabs.includes(button.dataset.subscriptionTab)) return;
        const target = '#subscriptions/' + button.dataset.subscriptionTab;
        if (location.hash !== target) history.pushState(null, '', target);
        sync();
      });
    });
    window.addEventListener('popstate', sync);
    window.addEventListener('hashchange', sync);
    window.addEventListener('further-ui:page', function (event) { if (event.detail && event.detail.name === 'subscriptions') sync(); });
    sync();
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
    const count = document.querySelector('[data-subscription-count="site"]');
    if (count) count.textContent = api.number(subscriptionPlans.length);
    updateRedeemPlanOptions();
    updateBenefitPlanOptions();
    const body = document.querySelector('[data-subscription-plan-rows]');
    if (!body) return;
	const directPlans = subscriptionPlans.filter(function (plan) { return !plan.collapsed; });
	const collapsedPlans = subscriptionPlans.filter(function (plan) { return Boolean(plan.collapsed); });
	function planGroup(title, hint, plans) {
	  if (!plans.length) return '';
	  return '<section class="admin-plan-group"><div class="admin-plan-group-head"><div><b>' + title + '</b><small>' + hint + '</small></div><span class="tag muted">' + plans.length + ' 个</span></div><div class="admin-plan-cards">' + plans.map(subscriptionPlanRow).join('') + '</div></section>';
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
		quota_group: '',
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
    const role = Number(user.role) || 1;
    const data = ' data-user-target-role="' + role + '" data-user="' + api.escapeHTML(encodeURIComponent(JSON.stringify(user))) + '"';
    const hidden = canManageUser(role) ? '' : ' hidden';
    const toggleHidden = canManageUser(role) && !(role >= 100 && enabled) ? '' : ' hidden';
    return '<div class="list-item"><span class="material-symbol" aria-hidden="true">person</span><div class="list-item-body"><div class="list-item-title">' + api.escapeHTML(user.username) + '</div><div class="list-item-sub">#' + user.id + ' · ' + api.date(user.created_at) + '</div></div><div class="list-metrics"><div><b>' + api.moneyFromQuota(user.quota, 2) + '</b>余额</div><div><b>' + adminRoleLabel(user.role) + '</b>角色</div></div><div class="list-actions"><span>' + api.statusLabel(user.status, 'token') + '</span><button type="button" class="md-btn md-btn-text md-btn-compact' + hidden + '" data-edit-user="' + user.id + '"' + data + '>编辑</button><button type="button" class="md-btn md-btn-text md-btn-compact' + toggleHidden + (enabled ? ' danger' : '') + '" data-toggle-user="' + user.id + '" data-next-status="' + (enabled ? 2 : 1) + '"' + data + '>' + (enabled ? '禁用' : '启用') + '</button><button type="button" class="md-btn md-btn-text md-btn-compact' + hidden + '" data-user-actions="' + user.id + '"' + data + '>更多操作</button></div></div>';
  }

  async function loadUsers() {
    const sequence = ++userLoadSequence;
    const params = new URLSearchParams({ page: String(userState.page), page_size: String(userState.pageSize) });
    ['keyword', 'role', 'status'].forEach(function (key) { if (userState[key]) params.set(key, userState[key]); });
    if (userState.sleeping) params.set('sleeping', '1');
    const payload = await api.request('/api/admin/users?' + params.toString());
    if (sequence !== userLoadSequence) return;
    const rows = Array.isArray(payload && payload.items) ? payload.items : [];
    const body = document.querySelector('[data-admin-user-rows]');
    if (!body) return;
    body.innerHTML = rows.length ? rows.map(userRow).join('') : '<div class="empty">还没有用户。</div>';
    userState.totalPages = Number(payload.total_pages) || 0;
    updatePagination('admin-user', userState, payload.total);
    body.querySelectorAll('[data-edit-user]').forEach(function (button) {
      button.addEventListener('click', function () {
        const user = JSON.parse(decodeURIComponent(button.dataset.user));
        if (!canManageUser(user.role)) return;
        const form = document.querySelector('[data-user-form]');
        if (!form) return;
        form.elements.id.value = user.id;
        form.elements.username.value = user.username || '';
        ['display_name', 'remark'].forEach(function (key) {
          if (form.elements[key]) form.elements[key].value = user[key] || '';
        });
        form.elements.quota.value = user.quota;
        api.syncQuotaUSD(form);
        form.elements.group.value = user.group || 'default';
        form.elements.role.value = user.role;
        form.dataset.targetRole = String(user.role);
        form.elements.role.disabled = currentAdminRole < 100 || Number(user.role) >= 100;
        form.elements.status.value = user.status;
        form.elements.status.disabled = Number(user.role) >= 100;
        api.openDialog('user-dialog');
      });
    });
    body.querySelectorAll('[data-toggle-user]').forEach(function (button) {
      button.addEventListener('click', async function () {
        if (!canManageUser(button.dataset.userTargetRole) || button.disabled) return;
        const disabling = Number(button.dataset.nextStatus) === 2;
        if (disabling && Number(button.dataset.userTargetRole) >= 100) return;
        button.disabled = true;
        try {
          if (disabling && !(await api.confirm('禁用后该用户将无法登录或调用 API。', { title: '禁用用户', confirmText: '禁用' }))) return;
          await api.request('/api/admin/users/' + button.dataset.toggleUser, { method: 'PATCH', body: { status: Number(button.dataset.nextStatus) } });
          api.toast(disabling ? '用户已禁用' : '用户已启用');
          await loadUsers();
        } catch (error) { api.toast(error.message, 'danger'); }
        finally { button.disabled = false; }
      });
    });
  }

  function codeBatchStatus(batch) {
    const unused = Number(batch.unused_count || 0);
    const used = Number(batch.used_count || 0);
    return (unused ? '<span class="tag success">未使用 ' + api.number(unused) + '</span>' : '') +
      (used ? '<span class="tag muted">已使用 ' + api.number(used) + '</span>' : '') +
      (batch.disabled_count ? '<span class="tag muted">停用 ' + api.number(batch.disabled_count) + '</span>' : '') +
      (batch.expired_count ? '<span class="tag error">过期 ' + api.number(batch.expired_count) + '</span>' : '');
  }

  function redemptionBatchRow(batch, batchIndex) {
    const items = Array.isArray(batch.items) ? batch.items : [];
    const first = items[0] || {};
    const batchID = batch.batch_id || ('legacy-' + (first.id || 0));
    const expanded = redemptionExpanded.has(batchID) || (!redemptionCollapsed.has(batchID) && (Boolean(redemptionState.keyword) || batchIndex === 0));
    const usable = items.filter(function (item) { return Number(item.status) === 1; });
    const amountSet = new Set(items.map(function (item) { return Number(item.quota || 0); }));
    const amountLabel = amountSet.size > 1 ? '多种面额' : api.moneyFromQuota(first.quota, 2);
    const detailRows = items.map(function (item) {
      const used = Number(item.status) === 2;
      const usage = used ? api.escapeHTML(item.used_by_account || ('用户 #' + Number(item.used_by_user_id || 0))) + '<span class="usage-cell-sub">' + api.date(item.used_at) + '</span>' : '—';
      const statusLabel = ({ 1: '未使用', 2: '已使用', 3: '停用', 4: '过期' })[Number(item.status)] || '未知';
      const actions = used ? '' : '<button type="button" class="md-icon-button" title="编辑兑换码" data-root-only data-edit-redemption="' + Number(item.id) + '"><span class="material-symbol" aria-hidden="true">edit</span></button><button type="button" class="md-icon-button" title="' + (Number(item.status) === 3 ? '启用' : '停用') + '兑换码" data-root-only data-toggle-redemption="' + Number(item.id) + '" data-enable="' + (Number(item.status) === 3) + '"><span class="material-symbol" aria-hidden="true">' + (Number(item.status) === 3 ? 'play_arrow' : 'pause') + '</span></button><button type="button" class="md-icon-button danger" title="删除兑换码" data-root-only data-delete-redemption="' + Number(item.id) + '"><span class="material-symbol" aria-hidden="true">delete</span></button>';
      return '<tr><td><code>' + api.escapeHTML(item.code) + '</code><span class="usage-cell-sub">' + api.escapeHTML(item.name || '') + ' · #' + Number(item.id) + '</span></td><td>' + api.moneyFromQuota(item.quota, 2) + '</td><td><span class="tag ' + (Number(item.status) === 1 ? 'success' : Number(item.status) === 4 ? 'error' : 'muted') + '">' + statusLabel + '</span></td><td>' + (item.expires_at ? api.date(item.expires_at) : '永久有效') + '</td><td>' + usage + '</td><td><div class="list-actions"><button type="button" class="md-icon-button" title="复制兑换码" data-copy-redemption="' + Number(item.id) + '"><span class="material-symbol" aria-hidden="true">content_copy</span></button>' + actions + '</div></td></tr>';
    }).join('');
    const deleteAction = Number(batch.deletable_count || 0) > 0 ? '<button type="button" class="md-icon-button danger" title="删除批次内未使用兑换码" data-root-only data-delete-redemption-batch="' + api.escapeHTML(batchID) + '"><span class="material-symbol" aria-hidden="true">delete_sweep</span></button>' : '';
    return '<section class="code-batch-card redemption-batch" data-redemption-batch="' + api.escapeHTML(batchID) + '"><div class="code-batch-head"><button type="button" class="md-icon-button code-batch-toggle" title="' + (expanded ? '收起明细' : '展开明细') + '" aria-expanded="' + expanded + '" aria-controls="redemption-details-' + batchIndex + '" data-code-batch-toggle><span class="material-symbol code-batch-chevron" aria-hidden="true">expand_more</span></button><span class="code-batch-icon material-symbol" aria-hidden="true">confirmation_number</span><div class="list-item-body"><div class="list-item-title">' + api.escapeHTML(batch.name || first.name || '未命名批次') + '</div><div class="list-item-sub">批次 ' + api.escapeHTML(batchID) + ' · ' + api.date(batch.created_at || first.created_at) + '</div></div><div class="list-metrics"><div><b>' + amountLabel + '</b>面额</div><div><b>' + api.number(items.length) + '</b>兑换码</div></div><div class="tags">' + codeBatchStatus(batch) + '</div><button type="button" class="md-btn md-btn-text md-btn-compact" title="仅复制未使用且未过期的兑换码" data-copy-redemption-batch="' + Number(batchIndex) + '"' + (usable.length ? '' : ' disabled') + '><span class="material-symbol" aria-hidden="true">content_copy</span>复制可用码</button>' + deleteAction + '</div><div id="redemption-details-' + batchIndex + '" class="code-batch-items redemption-details' + (expanded ? '' : ' hidden') + '"><div class="usage-table-scroll"><table class="usage-table redemption-table"><thead><tr><th>兑换码 / 名称</th><th>面额</th><th>状态</th><th>有效期</th><th>兑换记录</th><th>操作</th></tr></thead><tbody>' + detailRows + '</tbody></table></div></div></section>';
  }

  function registrationCodeBatchRow(batch, batchIndex) {
    const items = Array.isArray(batch.items) ? batch.items : [];
    const first = items[0] || {};
    const batchID = batch.batch_id || ('legacy-' + (first.id || 0));
    const detailRows = items.map(function (item) {
      const used = Number(item.status) === 2;
      const usage = used ? '由 ' + api.escapeHTML(item.used_by_account || ('用户 #' + Number(item.used_by_user_id || 0))) + ' 使用 · ' + api.date(item.used_at) : '未使用';
      return '<div class="list-item code-batch-item"><span class="material-symbol" aria-hidden="true">key</span><div class="list-item-body"><div class="list-item-title"><code>' + api.escapeHTML(item.code) + '</code></div><div class="list-item-sub">' + usage + '</div></div><span class="tag ' + (used ? 'muted' : 'success') + '">' + (used ? '已使用' : '未使用') + '</span>' + (used ? '' : '<button type="button" class="md-icon-button danger" title="删除注册码" data-delete-registration-code="' + Number(item.id) + '"><span class="material-symbol" aria-hidden="true">delete</span></button>') + '</div>';
    }).join('');
    return '<section class="code-batch-card"><div class="code-batch-head"><button type="button" class="md-icon-button code-batch-toggle" title="展开批次" aria-expanded="false" data-code-batch-toggle><span class="material-symbol code-batch-chevron" aria-hidden="true">expand_more</span></button><span class="code-batch-icon material-symbol" aria-hidden="true">key</span><div class="list-item-body"><div class="list-item-title">' + api.escapeHTML(batch.name || first.name || '未命名批次') + '</div><div class="list-item-sub"><code>' + api.escapeHTML(batchID) + '</code> · ' + api.date(batch.created_at || first.created_at) + '</div></div><div class="list-metrics"><div><b>' + api.number(batch.count || items.length) + '</b>码数量</div></div>' + codeBatchStatus(batch) + '<button type="button" class="md-icon-button" title="批量复制该批次" data-copy-registration-batch="' + Number(batchIndex) + '"><span class="material-symbol" aria-hidden="true">content_copy</span></button>' + (Number(batch.unused_count || 0) > 0 ? '<button type="button" class="md-icon-button danger" title="删除批次内未使用注册码" data-delete-registration-code-batch="' + api.escapeHTML(batchID) + '"><span class="material-symbol" aria-hidden="true">delete_sweep</span></button>' : '') + '</div><div class="code-batch-items hidden">' + detailRows + '</div></section>';
  }

  function prepaidCardBatchRow(batch, batchIndex) {
    const items = Array.isArray(batch.items) ? batch.items : [];
    const first = items[0] || {};
    const batchID = batch.batch_id || ('legacy-' + (first.id || 0));
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
      return '<div class="list-item code-batch-item"><span class="material-symbol" aria-hidden="true">payments</span><div class="list-item-body"><div class="list-item-title"><code>' + api.escapeHTML(item.code) + '</code></div><div class="list-item-sub">' + detail + '</div></div><span class="tag ' + statusMeta[1] + '">' + statusMeta[0] + '</span>' + (status === 1 ? '<button type="button" class="md-icon-button danger" title="删除额度卡" data-delete-prepaid-card="' + Number(item.id) + '"><span class="material-symbol" aria-hidden="true">delete</span></button>' : '') + '</div>';
    }).join('');
    const statusTags = (unused ? '<span class="tag success">未使用 ' + api.number(unused) + '</span>' : '') + (redeemed ? '<span class="tag muted">已兑换 ' + api.number(redeemed) + '</span>' : '') + (expired ? '<span class="tag error">已过期 ' + api.number(expired) + '</span>' : '');
    return '<section class="code-batch-card"><div class="code-batch-head"><button type="button" class="md-icon-button code-batch-toggle" title="展开批次" aria-expanded="false" data-code-batch-toggle><span class="material-symbol code-batch-chevron" aria-hidden="true">expand_more</span></button><span class="code-batch-icon material-symbol" aria-hidden="true">payments</span><div class="list-item-body"><div class="list-item-title">' + api.escapeHTML(batch.name || first.name || '未命名批次') + '</div><div class="list-item-sub"><code>' + api.escapeHTML(batchID) + '</code> · ' + Number(batch.validity_days || first.validity_days || 0) + ' 天' + (claimExpiry ? ' · 兑换截止 ' + api.date(claimExpiry) : '') + ' · ' + api.date(batch.created_at || first.created_at) + '</div></div><div class="list-metrics"><div><b>' + api.moneyFromQuota(batch.quota || first.quota, 2) + '</b>面额</div><div><b>' + api.number(batch.count || items.length) + '</b>卡数量</div></div>' + statusTags + '<button type="button" class="md-icon-button" title="批量复制该批次" data-copy-prepaid-batch="' + Number(batchIndex) + '"><span class="material-symbol" aria-hidden="true">content_copy</span></button>' + (unused > 0 ? '<button type="button" class="md-icon-button danger" title="删除批次内未使用额度卡" data-delete-prepaid-batch="' + api.escapeHTML(batchID) + '"><span class="material-symbol" aria-hidden="true">delete_sweep</span></button>' : '') + '</div><div class="code-batch-items hidden">' + detailRows + '</div></section>';
  }

  function bindCodeBatchToggles(body) {
    body.querySelectorAll('[data-code-batch-toggle]').forEach(function (button) {
      button.addEventListener('click', function () {
        const detail = button.closest('.code-batch-card').querySelector('.code-batch-items');
        const expanded = button.getAttribute('aria-expanded') !== 'true';
        button.setAttribute('aria-expanded', String(expanded));
        button.title = expanded ? '收起批次' : '展开批次';
        if (detail.contains(document.activeElement)) button.focus({ preventScroll: true });
        const apply = function (shown) { detail.classList.toggle('hidden', !shown); };
        if (api.transitionDropdown) api.transitionDropdown(detail, expanded, apply, { collapse: true });
        else apply(expanded);
      });
    });
  }

  async function loadRedemptions() {
    const params = new URLSearchParams({ page: String(redemptionState.page), page_size: String(redemptionState.pageSize) });
    params.set('group_by', 'batch');
    if (redemptionState.keyword) params.set('keyword', redemptionState.keyword);
    if (redemptionState.status) params.set('status', redemptionState.status);

    const result = await api.request('/api/admin/redemptions?' + params.toString());
    const lastPage = Math.max(1, Number(result && result.total_pages) || 0);
    if (redemptionState.page > lastPage) { redemptionState.page = lastPage; return loadRedemptions(); }
    const rows = Array.isArray(result && result.items) ? result.items : [];
    const body = document.querySelector('[data-redemption-rows]');
    if (!body) return;
    const allItems = rows.flatMap(function (batch) { return batch.items || []; });
    const usableItems = allItems.filter(function (item) { return Number(item.status) === 1; });
    const toolbar = '<div class="redemption-toolbar card-head"><div><b>兑换码</b><div class="card-hint">本页 ' + rows.length + ' 个批次 · ' + allItems.length + ' 个码 · ' + usableItems.length + ' 个可用码</div></div><div class="list-actions"><button class="md-btn md-btn-text md-btn-compact" type="button" data-redemption-expand-all>展开全部明细</button><button class="md-btn md-btn-outlined md-btn-compact" type="button" data-copy-redemption-page' + (usableItems.length ? '' : ' disabled') + '><span class="material-symbol" aria-hidden="true">content_copy</span>复制本页可用码</button></div></div>';
    body.innerHTML = rows.length ? toolbar + rows.map(redemptionBatchRow).join('') : '<div class="empty">当前筛选下没有兑换码，可调整筛选或新建批次。</div>';
    redemptionState.totalPages = Number(result.total_pages) || 0;
    updatePagination('redemption', redemptionState, result.total, '个批次');
    bindCodeBatchToggles(body);
    body.querySelectorAll('[data-code-batch-toggle]').forEach(function (button) { button.addEventListener('click', function () { const batchID = button.closest('[data-redemption-batch]').dataset.redemptionBatch; if (button.getAttribute('aria-expanded') === 'true') { redemptionExpanded.add(batchID); redemptionCollapsed.delete(batchID); } else { redemptionExpanded.delete(batchID); redemptionCollapsed.add(batchID); } }); });
    const expandAll = body.querySelector('[data-redemption-expand-all]');
    if (expandAll) expandAll.addEventListener('click', function () {
      const expand = expandAll.dataset.expanded !== 'true';
      expandAll.dataset.expanded = String(expand);
      expandAll.textContent = expand ? '收起全部明细' : '展开全部明细';
      body.querySelectorAll('[data-code-batch-toggle]').forEach(function (button) { if ((button.getAttribute('aria-expanded') === 'true') !== expand) button.click(); });
    });
    const copyPage = body.querySelector('[data-copy-redemption-page]');
    if (copyPage) copyPage.addEventListener('click', function () { openRedemptionCodeDialog(usableItems.map(function (item) { return item.code; }).filter(Boolean), '本页可用兑换码 · ' + usableItems.length + ' 个'); });
    body.querySelectorAll('[data-copy-redemption]').forEach(function (button) { button.addEventListener('click', async function () {
      const item = allItems.find(function (code) { return Number(code.id) === Number(button.dataset.copyRedemption); });
      if (!item || !item.code) return;
      try { await navigator.clipboard.writeText(item.code); api.toast('兑换码已复制'); } catch (_) { openRedemptionCodeDialog([item.code], '复制兑换码'); }
    }); });
    body.querySelectorAll('[data-edit-redemption]').forEach(function (button) {
      button.addEventListener('click', function () {
        const item = rows.flatMap(function (batch) { return batch.items || []; }).find(function (code) { return Number(code.id) === Number(button.dataset.editRedemption); });
        if (!item) return;
        resetRedeemForm();
        const form = document.querySelector('[data-redeem-form]');
        form.elements.id.value = item.id;
        form.elements.name.value = item.name || '';
        form.elements.amount_usd.value = api.usdFromQuota(item.quota);
        form.elements.count.disabled = true;
        document.querySelector('[data-redeem-count-field]').classList.add('hidden');
        if (item.expires_at) {
          const expiry = new Date(item.expires_at);
          form.elements.expires_at.value = new Date(expiry.getTime() - expiry.getTimezoneOffset() * 60000).toISOString().slice(0, 16);
        }
        api.setText('[data-redeem-dialog-title]', '编辑兑换码');
        form.querySelector('[type="submit"]').textContent = '保存修改';
        api.openDialog('redeem-dialog');
      });
    });
    body.querySelectorAll('[data-toggle-redemption]').forEach(function (button) {
      button.addEventListener('click', async function () {
        try {
          await api.request('/api/admin/redemptions/' + button.dataset.toggleRedemption + '/status', { method: 'PUT', body: { enabled: button.dataset.enable === 'true' } });
          await loadRedemptions();
        } catch (error) { api.toast(error.message, 'danger'); }
      });
    });
    body.querySelectorAll('[data-copy-redemption-batch]').forEach(function (button) {
      button.addEventListener('click', function () {
        const batch = rows[Number(button.dataset.copyRedemptionBatch)];
        if (!batch) return;
        const codes = (Array.isArray(batch.items) ? batch.items : []).filter(function (item) { return Number(item.status) === 1; }).map(function (item) { return item.code; }).filter(Boolean);
        openRedemptionCodeDialog(codes, (batch.name || '兑换码批次') + ' · 可用码 ' + codes.length + ' 个');
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
    body.innerHTML = rows.length ? rows.map(prepaidCardBatchRow).join('') : '<div class="empty">还没有额度卡。</div>';
    prepaidCardState.totalPages = Number(result.total_pages) || 0;
    updatePagination('prepaid-card', prepaidCardState, result.total, '个批次');
    bindCodeBatchToggles(body);
    applyAdminCapabilities();
    body.querySelectorAll('[data-copy-prepaid-batch]').forEach(function (button) {
      button.addEventListener('click', function () {
        const batch = rows[Number(button.dataset.copyPrepaidBatch)];
        if (!batch) return;
        const codes = (Array.isArray(batch.items) ? batch.items : []).map(function (item) { return item.code; }).filter(Boolean);
        openPrepaidCodeDialog(codes, (batch.name || '额度卡批次') + ' · ' + (batch.batch_id || ''));
      });
    });
    body.querySelectorAll('[data-delete-prepaid-card]').forEach(function (button) {
      button.addEventListener('click', async function () {
        if (!(await api.confirm('删除后该额度卡将无法兑换且无法恢复。', { title: '删除额度卡', confirmText: '删除' }))) return;
        try { await api.request('/api/admin/prepaid-cards/' + button.dataset.deletePrepaidCard, { method:'DELETE' }); api.toast('额度卡已删除'); await loadPrepaidCards(); } catch (error) { api.toast(error.message, 'danger'); }
      });
    });
    body.querySelectorAll('[data-delete-prepaid-batch]').forEach(function (button) {
      button.addEventListener('click', async function () {
        if (!(await api.confirm('将删除该批次中全部未使用额度卡，已兑换记录不受影响。', { title:'删除未使用批次', confirmText:'删除' }))) return;
        try { await api.request('/api/admin/prepaid-cards?batch_id=' + encodeURIComponent(button.dataset.deletePrepaidBatch), { method:'DELETE' }); api.toast('批次中未使用额度卡已删除'); await loadPrepaidCards(); } catch (error) { api.toast(error.message, 'danger'); }
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
    api.setText('[data-generated-prepaid-title]', title || '额度卡批量复制');
    api.openDialog('generated-prepaid-dialog');
  }

  function bindPrepaidCards() {
    const form = document.querySelector('[data-prepaid-card-form]');
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      const button = form.querySelector('button[type="submit"]');
      api.showBusy(button, true);
      try {
		const payload = { name:form.elements.name.value.trim(), user_group:'default', count:Number(form.elements.count.value), quota:Number(form.elements.quota.value), validity_days:Number(form.elements.validity_days.value), claim_expires_at:form.elements.claim_expires_at.value };
        const result = await api.request('/api/admin/prepaid-cards/generate', { method:'POST', body:payload });
        api.closeDialog('prepaid-card-dialog'); openPrepaidCodeDialog(result.codes || [], '额度卡已生成'); api.toast('已生成 ' + Number(result.count || 0) + ' 张额度卡');
        prepaidCardState.page = 1; await loadPrepaidCards();
      } catch (error) { api.toast(error.message, 'danger'); } finally { api.showBusy(button, false); }
    });
    document.querySelector('[data-copy-prepaid-codes]').addEventListener('click', async function () {
      try { await navigator.clipboard.writeText(document.querySelector('[data-generated-prepaid-codes]').value); api.toast('额度卡已复制'); } catch (_) { api.toast('复制失败，请手动复制', 'danger'); }
    });
    document.querySelector('[data-delete-used-prepaid]').addEventListener('click', async function () {
      if (!(await api.confirm('将删除全部已兑换和已过期的额度卡记录。现有额度与 Key 不受影响，审计记录不可恢复。', { title:'清理额度卡记录', confirmText:'全部删除' }))) return;
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
    if (logState.exclude200) params.set('only_errors', 'true');
    params.set('sort_by', logState.sortBy);
    params.set('sort_order', logState.sortOrder);
    const result = await api.request('/api/admin/logs?' + params.toString());
    const rows = Array.isArray(result && result.items) ? result.items : [];
    const body = document.querySelector('[data-admin-log-rows]');
    if (!body) return;
    body.innerHTML = rows.length ? rows.map(function (item) {
      const failed = item.failed === true || Number(item.status_code) >= 400 && Number(item.status_code) <= 599;
      const statusLabel = Number(item.status_code) > 0 ? Number(item.status_code) : failed ? '失败' : '未记录';
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
        '<td><span class="usage-status ' + (failed ? 'error' : 'success') + '">' + statusLabel + '</span>' + (item.error_code ? '<span class="usage-cell-sub" title="' + api.escapeHTML(item.error_code) + '">' + api.escapeHTML(item.error_code) + '</span>' : '') + '</td>' +
        '</tr>';
    }).join('') : '<tr><td class="empty" colspan="7">还没有调用日志。</td></tr>';
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
    const refresh = document.querySelector('[data-admin-user-refresh]');
    if (refresh) refresh.addEventListener('click', async function () {
      if (refresh.disabled) return;
      refresh.disabled = true;
      try { await loadUsers(); } catch (error) { api.toast(error.message, 'danger'); }
      finally { refresh.disabled = false; }
    });
    document.addEventListener('further:users-updated', function () {
      loadUsers().catch(function (error) { api.toast(error.message, 'danger'); });
    });
    const form = document.querySelector('[data-admin-user-filter]');
    if (!form) return;
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      userState.keyword = form.elements.keyword.value.trim();
      userState.group = '';
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
      state.userGroup = '';
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
    ['prev', 'next'].forEach(function (direction) {
      const button = document.querySelector('[data-' + prefix + '-' + direction + ']');
      if (!button) return;
      button.addEventListener('click', async function () {
        if (state.loading || (direction === 'prev' ? state.page <= 1 : state.totalPages === 0 || state.page >= state.totalPages)) return;
        const previousPage = state.page;
        state.page += direction === 'prev' ? -1 : 1;
        state.loading = true;
        try { await loader(); } catch (error) {
          state.page = previousPage;
          api.toast(error.message, 'danger');
        } finally { state.loading = false; }
      });
    });
  }

  function syncChannelFormMode(form) {
	const editing = Boolean(form.elements.id.value);
	const mode = form.elements.multi_key_mode.value;
	const multiKey = form.dataset.multiKey === 'true';
	const providerType = Number(form.elements.provider_type.value);
	form.elements.type.value = providerType === 14 ? '2' : providerType === 1 ? '1' : '3';
	form.querySelector('[data-channel-create-only]').classList.toggle('hidden', editing);
	form.querySelector('[data-channel-multi-key-type]').classList.toggle('hidden', editing || mode !== 'multi_to_single');
	form.querySelector('[data-channel-batch-prefix]').classList.toggle('hidden', editing || mode !== 'batch');
	form.querySelector('[data-channel-edit-key-options]').classList.toggle('hidden', !editing || !multiKey);
	form.elements.key.required = !editing;
	form.elements.base_url.required = [3, 8, 36, 45, 60, 61, 62, 63].includes(providerType);
	const hint = form.querySelector('[data-channel-key-hint]');
	hint.textContent = editing ? (multiKey ? '留空保持现有 Key；填写后可追加或替换。' : '留空保持现有 Key。') : mode === 'batch' ? '每行一个 Key，将分别创建渠道。' : mode === 'multi_to_single' ? '每行一个 Key，合并到同一渠道。' : '填写一个 Key。';
  }

  function resetChannelForm() {
    const form = document.querySelector('[data-channel-form]');
    if (!form) return;
    form.reset();
    const presetStatus = form.querySelector('[data-channel-preset-status]');
    if (presetStatus) presetStatus.textContent = '';
    form.elements.id.value = '';
    form.elements.type.value = '1';
	form.elements.provider_type.value = '1';
    form.elements.priority.value = '0';
    form.elements.weight.value = '1';
    form.elements.groups.value = 'default';
	form.elements.multi_key_mode.value = 'single';
	form.elements.multi_key_type.value = 'random';
	form.elements.edit_multi_key_type.value = 'random';
	form.elements.auto_ban.value = '1';
	form.dataset.multiKey = 'false';
	form.elements.key.classList.add('channel-key-masked');
	form.querySelector('[data-channel-key-visibility]').setAttribute('aria-pressed', 'false');
	form.querySelector('[data-channel-key-visibility]').textContent = '显示 Key';
	form.querySelector('[data-channel-advanced]').open = false;
	syncChannelFormMode(form);
	if (window.FurtherUIAdminChannelModels) window.FurtherUIAdminChannelModels.resetForm(form);
    document.querySelector('#channel-dialog .md-dialog-title').textContent = '添加渠道';
  }

  function bindChannelPresets(form) {
    if (form.querySelector('[data-channel-presets]')) return;
    const toolbar = document.createElement('section'); toolbar.className = 'form-presets'; toolbar.dataset.channelPresets = ''; toolbar.dataset.rootOnly = '';
    toolbar.innerHTML = '<div class="field-title">一键接入配置</div><p class="field-hint">按所选供应商补齐空白名称、默认地址、分组、优先级、权重及测试模型；Key 和已填内容保留。填写结果确认后点击保存。</p><div class="list-actions"><button type="button" class="md-btn md-btn-tonal" data-channel-defaults>补齐接入默认值</button><button type="button" class="md-btn md-btn-outlined" data-channel-normalize-models>整理模型列表</button><button type="button" class="md-btn md-btn-outlined" data-channel-key-preset>多 Key 轮询预设</button></div><p class="field-hint" data-channel-preset-status role="status" aria-live="polite"></p>';
    form.querySelector('.form-grid').after(toolbar);
    const status = toolbar.querySelector('[data-channel-preset-status]');
    toolbar.querySelector('[data-channel-defaults]').addEventListener('click', async function (event) {
      const button = event.currentTarget;
      const provider = form.elements.provider_type.value, channelID = form.elements.id.value;
      api.showBusy(button, true);
      try {
        const defaults = await api.request('/api/channel/default_base_urls');
        if (provider !== form.elements.provider_type.value || channelID !== form.elements.id.value) { status.textContent = '渠道或供应商已切换，请重新应用预设。'; return; }
        if (!defaults || typeof defaults !== 'object' || Array.isArray(defaults)) throw new Error('供应商默认地址格式无效');
        const providerName = form.elements.provider_type.selectedOptions[0]?.textContent || '新';
        const firstModel = String(form.elements.models.value).split(/[,，\r\n]+/).map(function (value) { return value.trim(); }).find(Boolean);
        const fields = { name: providerName + ' 渠道', base_url: defaults[provider], groups: 'default', priority: '0', weight: '1', auto_ban: '1', test_model: firstModel };
        const labels = { name: '名称', base_url: '上游地址', groups: '分组', priority: '优先级', weight: '权重', auto_ban: '故障处理', test_model: '测试模型' };
        const filled = [];
        Object.entries(fields).forEach(function (entry) {
          const field = form.elements[entry[0]];
          if (entry[1] == null || !String(entry[1]).trim() || !field || field.value.trim()) return;
          field.value = String(entry[1]); field.dispatchEvent(new Event('input', { bubbles: true })); filled.push(labels[entry[0]]);
        });
        syncChannelFormMode(form);
        status.textContent = (filled.length ? '已补齐：' + filled.join('、') + '。保存后生效。' : '现有字段已填写，无需补齐。') + (!defaults[provider] && !form.elements.base_url.value.trim() ? '此供应商需要手动填写专属上游地址。' : '');
      } catch (error) { status.textContent = '默认配置读取失败：' + error.message + '。已有输入保留，可重试或手动填写。'; }
      finally { api.showBusy(button, false); }
    });
    toolbar.querySelector('[data-channel-normalize-models]').addEventListener('click', function () {
      const names = Array.from(new Set(form.elements.models.value.split(/[,，\r\n]+/).map(function (name) { return name.trim(); }).filter(Boolean)));
      if (!names.length) { status.textContent = '请填写模型列表，或从上游读取模型。'; return; }
      form.elements.models.value = names.join(',');
      form.elements.models.dispatchEvent(new Event('input', { bubbles: true }));
      if (!form.elements.test_model.value.trim()) form.elements.test_model.value = names[0];
      status.textContent = '已去重并统一分隔符，共 ' + names.length + ' 个模型；空白测试模型已补齐。保存后生效。';
    });
    toolbar.querySelector('[data-channel-key-preset]').addEventListener('click', function () {
      if (form.elements.id.value) {
        if (form.dataset.multiKey !== 'true') { status.textContent = '现有单 Key 渠道保持不变；请新建多 Key 渠道使用轮询。'; return; }
        form.elements.edit_multi_key_type.value = 'polling';
        status.textContent = '已将此渠道的多 Key 策略设为轮询草稿，Key 内容与追加/替换方式保持。保存后生效。';
      } else {
        if ([41, 57, 58, 61].includes(Number(form.elements.provider_type.value))) { status.textContent = '此供应商使用结构化凭据或专用认证，请手动配置 Key 策略。'; return; }
        form.elements.multi_key_mode.value = 'multi_to_single';
        form.elements.multi_key_type.value = 'polling';
        status.textContent = '已选择“多个 Key 合并为一个渠道 + 轮询”；请每行填写一个 Key，保存后生效。';
      }
      syncChannelFormMode(form);
    });
  }

  function bindChannelForm() {
    const form = document.querySelector('[data-channel-form]');
    if (!form) return;
    bindChannelPresets(form);
	if (window.FurtherUIAdminChannelModels) {
	  window.FurtherUIAdminChannelModels.mountForm(form);
	  window.FurtherUIAdminChannelModels.setRefreshHandler(loadChannels);
	}
	form.elements.provider_type.addEventListener('change', function () {
	  syncChannelFormMode(form);
	  if ([3, 18, 21, 39, 41, 49, 58, 61].includes(Number(form.elements.provider_type.value))) form.querySelector('[data-channel-advanced]').open = true;
	});
	form.elements.multi_key_mode.addEventListener('change', function () { syncChannelFormMode(form); });
	form.querySelector('[data-channel-key-visibility]').addEventListener('click', function (event) {
	  const visible = event.currentTarget.getAttribute('aria-pressed') !== 'true';
	  form.elements.key.classList.toggle('channel-key-masked', !visible);
	  event.currentTarget.setAttribute('aria-pressed', visible ? 'true' : 'false');
	  event.currentTarget.textContent = visible ? '隐藏 Key' : '显示 Key';
	});
	['model_mapping', 'status_code_mapping', 'param_override', 'header_override', 'setting', 'settings'].forEach(function (name) {
	  form.elements[name].addEventListener('input', function () { form.elements[name].setCustomValidity(''); });
	});
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
	  const advanced = form.querySelector('[data-channel-advanced]');
	  for (const name of ['model_mapping', 'status_code_mapping', 'param_override', 'header_override', 'setting', 'settings']) {
		const field = form.elements[name];
		field.setCustomValidity('');
		if (!field.value.trim()) continue;
		try {
		  const parsed = JSON.parse(field.value);
		  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) throw new Error('请输入 JSON 对象');
		} catch (_) {
		  advanced.open = true;
		  field.setCustomValidity('请输入有效的 JSON 对象');
		  field.reportValidity();
		  return;
		}
	  }
	  if ([3, 18, 21, 39, 41, 49].includes(Number(form.elements.provider_type.value)) && !form.elements.other.value.trim()) {
		advanced.open = true;
		form.elements.other.focus();
		api.toast('当前供应商需要填写附加信息', 'danger');
		return;
	  }
      const formData = new FormData(form);
      const data = Object.fromEntries(formData.entries());
	  data.type = Number(data.type); data.provider_type = Number(data.provider_type);
	  data.priority = Number(data.priority); data.weight = Number(data.weight);
	  data.auto_ban = Number(data.auto_ban);
      const editing = Boolean(data.id);
	  const multiKey = form.dataset.multiKey === 'true';
	  if (editing) {
		delete data.multi_key_mode;
		delete data.batch_add_set_key_prefix_2_name;
		data.multi_key_type = multiKey ? String(data.edit_multi_key_type) : '';
		if (!multiKey || !String(data.key).trim()) delete data.key_mode;
	  } else {
		data.batch_add_set_key_prefix_2_name = form.elements.batch_add_set_key_prefix_2_name.checked;
		delete data.key_mode;
		if (data.multi_key_mode !== 'multi_to_single') delete data.multi_key_type;
	  }
	  delete data.edit_multi_key_type;
	  if (editing) {
		for (const name of ['tag', 'remark', 'test_model', 'openai_organization', 'other', 'status_code_mapping', 'param_override', 'header_override', 'setting', 'settings']) {
		  if (!String(data[name] || '').trim()) delete data[name];
		}
	  }
      try {
        await api.request(editing ? '/api/admin/channels/' + data.id : '/api/admin/channels', { method: editing ? 'PUT' : 'POST', body: data });
      } catch (error) { api.toast('渠道保存失败：' + error.message, 'danger'); return; }
      api.closeDialog('channel-dialog');
      api.toast(editing ? '渠道已更新' : data.multi_key_mode === 'batch' ? '渠道已批量创建' : '渠道已创建');
      const refreshed = await Promise.allSettled([loadChannels(), adminModels.refresh ? adminModels.refresh() : adminModels.load(), loadOverview()]);
      if (refreshed.some(function (result) { return result.status === 'rejected'; })) {
        api.toast('渠道已保存，部分页面数据刷新失败，请点击刷新重试。', 'danger');
      }
      if (form.querySelector('[data-channel-setup-pricing]')?.checked && window.FurtherUIModelImports) {
        window.FurtherUIModelImports.afterChannelSave(data.models, refreshed[1].status !== 'fulfilled');
      }
    });
  }

  function channelKeyStatus(status) {
    return Number(status) === 1 ? '<span class="tag success">启用</span>' : Number(status) === 3 ? '<span class="tag error">自动下线</span>' : '<span class="tag muted">停用</span>';
  }

  function channelKeyRow(item) {
    const quota = Number(item.quota_remaining) < 0 ? '不限额' : api.moneyFromQuota(item.quota_remaining, 2);
    const balanceAction = activeChannelStatus === 1 && Number(item.status) === 1 ? '<button type="button" class="md-icon-button" title="查询上游渠道余额" data-refresh-dialog-key-balance="' + Number(item.id) + '"><span class="material-symbol" aria-hidden="true">account_balance_wallet</span></button>' : '';
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
          const report = summary.reports && summary.reports[0];
          api.toast(report ? report.message + (report.balance_usd == null ? '' : ' · $' + Number(report.balance_usd).toFixed(4)) : '未返回余额查询结果', report && report.status === 'failed' ? 'danger' : undefined);
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
      const usdLimits = form.querySelectorAll('[data-quota-usd-for="quota_total"], [data-quota-usd-for="quota_remaining"]');
      for (const input of usdLimits) {
        const value = Number(input.value);
        if (!Number.isFinite(value) || (value < 0 && value !== -1)) {
          input.focus();
          api.toast('额度请填非负美元金额，或填 -1 表示不限额', 'danger');
          return;
        }
      }
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
      if (!canManageUser(form.dataset.targetRole) || form.dataset.saving === 'true') return;
      const data = Object.fromEntries(new FormData(form).entries());
      data.quota = Number(data.quota);
      delete data.group;
      if (!form.elements.status.disabled) data.status = Number(data.status);
      if (!form.elements.role.disabled) data.role = Number(data.role);
      const submit = form.querySelector('[type="submit"]');
      form.dataset.saving = 'true';
      if (submit) submit.disabled = true;
      try {
        await api.request('/api/admin/users/' + data.id, { method: 'PATCH', body: data });
        api.closeDialog('user-dialog');
        api.toast('用户已更新');
        await loadUsers();
      } catch (error) { api.toast(error.message, 'danger'); }
      finally { form.dataset.saving = 'false'; if (submit) submit.disabled = false; }
    });
  }

  function resetRedeemForm() {
    const form = document.querySelector('[data-redeem-form]');
    if (form.dataset.saving === 'true') return;
    form.reset();
    form.elements.id.value = '';
    form.elements.count.disabled = false;
    form.elements.count.value = '1';
    form.dataset.saving = 'false';
    const submit = form.querySelector('[type="submit"]');
    if (submit) submit.textContent = '立即生成';
    document.querySelector('[data-redeem-count-field]').classList.remove('hidden');
    api.setText('[data-redeem-dialog-title]', '生成兑换码');
  }

  function bindRedeemForm() {
    const form = document.querySelector('[data-redeem-form]');
    document.querySelectorAll('[data-open-dialog="redeem-dialog"]').forEach(function (button) {
      button.addEventListener('click', resetRedeemForm);
    });
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      if (form.dataset.saving === 'true') return;
      const expiry = form.elements.expires_at.value;
      const expiredTime = expiry ? Math.floor(new Date(expiry).getTime() / 1000) : 0;
      if (!Number.isFinite(expiredTime)) return api.toast('截止时间无效', 'danger');
      const button = form.querySelector('[type="submit"]');
      form.dataset.saving = 'true';
      const dialog = form.closest('.dialog-backdrop');
      if (dialog) dialog.dataset.dialogBusy = 'true';
      api.showBusy(button, true);
      const id = form.elements.id.value;
      try {
        const result = await api.request(id ? '/api/admin/redemptions/' + id : '/api/admin/redemptions/generate', { method: id ? 'PUT' : 'POST', body: {
          name: form.elements.name.value.trim(),
          count: Number(form.elements.count.value),
          amount_usd: Number(form.elements.amount_usd.value),
          expired_time: expiredTime, redeem_type: 'quota'
        } });
        if (!id && (!Array.isArray(result.codes) || !result.codes.length || !result.codes.every(function (code) { return typeof code === 'string' && code.trim(); }))) throw new Error('服务器未返回有效兑换码，请刷新发放记录确认后重试');
        if (dialog) delete dialog.dataset.dialogBusy;
        api.closeDialog('redeem-dialog');
        if (!id) openRedemptionCodeDialog(result.codes, '兑换码已生成，可直接复制');
        api.toast(id ? '兑换码已更新' : '兑换码已生成');
        try { await loadRedemptions(); } catch (error) { api.toast('兑换码已保存，列表刷新失败：' + error.message, 'danger'); }
      } catch (error) { api.toast(error.message, 'danger'); }
      finally { form.dataset.saving = 'false'; if (dialog) delete dialog.dataset.dialogBusy; api.showBusy(button, false); }
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

  document.addEventListener('further:ready', async function () {
    if (!api.token()) { location.href = '/auth/?mode=login'; return; }
    const legacyTraceHash = location.hash === '#traces';
    if (legacyTraceHash) history.replaceState(null, '', '#logs');
    api.mountShell();
    if (window.FurtherUIAdminTokenStats) window.FurtherUIAdminTokenStats.bind();
    if (window.FurtherUIAdminUserActions) window.FurtherUIAdminUserActions.bind();
    bindChannelSubpages(); bindChannelVisibilityFilter(); bindSubscriptionTabs();
    api.bindQuotaUSDFields(document);
    document.querySelectorAll('[data-open-dialog="channel-dialog"]').forEach(function (button) { button.addEventListener('click', resetChannelForm); });
    document.querySelectorAll('[data-open-dialog="model-dialog"]').forEach(function (button) { button.addEventListener('click', adminModels.resetForm); });
    document.querySelectorAll('[data-open-dialog="subscription-plan-dialog"]').forEach(function (button) { button.addEventListener('click', resetSubscriptionPlanForm); });
    document.querySelectorAll('[data-open-dialog="subscription-benefit-dialog"]').forEach(function (button) { button.addEventListener('click', resetSubscriptionBenefitForm); });
	adminOverview.bind();
    bindChannelForm(); adminModels.bind(); if (window.FurtherUIAdminRouting) window.FurtherUIAdminRouting.bind(); bindSubscriptionPlanForm(); bindSubscriptionBenefitForm();  bindSubscriptionCycleReset(); bindChannelKeyForm(); bindUserForm(); bindRedeemForm(); bindCopyCodes(); bindRegistrationCodeForm(); bindCopyRegistrationCodes(); bindUsedCodeCleanup(); bindPrepaidCards(); bindUserFilters(); bindLogFilters(); bindTraceFilters(); bindLogViewSwitch(); bindCodeFilters('redemption', redemptionState, loadRedemptions); bindCodeFilters('registration-code', registrationCodeState, loadRegistrationCodes); adminSettings.bind();
    if (legacyTraceHash) setLogView('traces');
    try {
      const role = Number(document.body.dataset.gatewayRole) || 0;
      currentAdminRole = role >= 100 ? 100 : role >= 10 ? 50 : role;
      if (currentAdminRole < 25) { location.href = '/console/'; return; }
      applyAdminCapabilities();
      if (currentAdminRole < 50 && channelSubpageFromHash() === 'keys') showChannelSubpage('index', 'replace');
      if (channelSubpageFromHash() === 'keys') ensureChannelKeyManagement();
      if (channelSubpageFromHash() === 'disabled') ensureDisabledChannelManagement();
      const capabilityObserver = new MutationObserver(function () { applyAdminCapabilities(); });
      capabilityObserver.observe(document.querySelector('.content-area'), { childList: true, subtree: true });
      window.FurtherUIResources.mount(resources, resourcesForPage);
      applyAdminCapabilities();
    } catch (error) { api.toast(error.message, 'danger'); }
  });
}());
