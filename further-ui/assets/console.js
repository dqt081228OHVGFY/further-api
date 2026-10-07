(function () {
  'use strict';
  const api = window.FurtherUI;
  const consoleModels = window.FurtherUIConsoleModels;
  const initialPreferences = api.preferences();
  const logState = { page: 1, pageSize: Number(initialPreferences.pageSize) || 25, keyword: '', statusCode: '', exclude200: Boolean(initialPreferences.hideSuccessfulLogs), startTime: 0, endTime: 0, sortBy: 'created_at', sortOrder: 'desc', totalPages: 0 };
  let userLogLoadGeneration = 0;
  let userLogLoadPending = null;
  const tokenState = { page: 1, pageSize: 25, total: 0, generation: 0 };
  let billingPriority = 'subscription_first';
  let subscriptionRows = [];
  let userSecurity = { totp_enabled: false, pending: false, login_required: false, key_required: false, abandon_required: false };

	let siteHelpDocsVisible = true;
	let userHelpDocsVisible = true;
  const resources = window.FurtherUIResources.create({
    overview: loadOverview, security: loadUserSecurity, tokens: loadTokens,
    logs: loadLogs, subscriptions: loadSubscriptions, plans: loadSubscriptionPlanCatalog,
    timedQuotas: loadTimedQuotas, benefits: loadUserBenefits, models: consoleModels.load
  });

  function resourcesForPage(name) {
    const pages = {
      overview: ['overview', 'benefits'], tokens: ['tokens', 'security'], models: ['models'], docs: [],
      usage: ['logs'], redeem: ['overview'],
      subscriptions: ['subscriptions', 'plans', 'timedQuotas', 'security'],
      benefits: ['benefits', 'overview'], wallet: [],
      profile: ['overview']
    };
    return pages[name] || [];
  }


	function applyHelpDocsVisibility() {
	  const visible = siteHelpDocsVisible && userHelpDocsVisible;
	  document.querySelectorAll('[data-nav="docs"], [data-panel="docs"]').forEach(function (node) { node.classList.toggle('hidden', !visible); });
	  const form = document.querySelector('[data-preferences-form]');
	  if (form && form.elements.help_docs_visible) {
		form.elements.help_docs_visible.checked = userHelpDocsVisible;
		form.elements.help_docs_visible.disabled = !siteHelpDocsVisible;
	  }
	  api.setText('[data-help-docs-preference-hint]', siteHelpDocsVisible ? '' : '管理员已隐藏使用文档。');
	  if (!visible && (location.hash === '#docs' || document.querySelector('[data-panel="docs"]')?.classList.contains('active'))) {
		location.hash = 'overview';
		document.querySelector('[data-nav="overview"]')?.click();
	  }
	}

	async function loadUserUIPreferences() {
	  let preferences = await api.request('/api/user/ui-preferences');
	  const local = api.preferences();
	  if (!preferences.configured) {
		const localDefaultPage = local.defaultPage === 'plans' ? 'subscriptions' : local.defaultPage;
		preferences = Object.assign({}, preferences, {
		  help_docs_visible: preferences.help_docs_visible !== false,
		  theme: local.theme, timezone: local.timezone, default_page: localDefaultPage, page_size: local.pageSize,
		  reduced_motion: false, hide_locked_models: local.hideLockedModels, sidebar_collapsed: local.sidebarCollapsed,
		  hour_cycle: local.hourCycle, show_seconds: local.showSeconds, hide_successful_logs: local.hideSuccessfulLogs
		});
	  }
	  const saved = api.savePreferences({
		theme: preferences.theme, timezone: preferences.timezone, defaultPage: preferences.default_page,
		pageSize: preferences.page_size, reducedMotion: false,
		hideLockedModels: preferences.hide_locked_models, sidebarCollapsed: preferences.sidebar_collapsed,
		hourCycle: preferences.hour_cycle, showSeconds: preferences.show_seconds,
		hideSuccessfulLogs: preferences.hide_successful_logs
	  }, { source: 'server' });
	  logState.pageSize = Number(saved.pageSize) || 25;
	  logState.exclude200 = Boolean(saved.hideSuccessfulLogs);
	  userHelpDocsVisible = preferences.help_docs_visible !== false;
	  applyHelpDocsVisibility();
	}

  function userAPIErrorType(error) {
    return error && error.payload && error.payload.error && error.payload.error.type || '';
  }

  async function protectedUserRequest(path, options, required, dialogOptions) {
    const requestOptions = Object.assign({}, options || {});
    requestOptions.body = Object.assign({}, requestOptions.body || {});
    let code = '';
    if (required) {
      code = await api.requestTOTP(dialogOptions);
      if (!code) return null;
    }
    while (true) {
      try {
        requestOptions.body.totp_code = code;
        return await api.request(path, requestOptions);
      } catch (error) {
        const type = userAPIErrorType(error);
        if (type !== 'totp_required' && type !== 'totp_invalid') throw error;
        if (type === 'totp_invalid') api.toast('动态验证码无效，请重试', 'danger');
        code = await api.requestTOTP(dialogOptions);
        if (!code) return null;
      }
    }
  }

  function requireLogin() {
    if (!api.token()) {
      location.href = '/auth/?mode=login';
      return false;
    }
    return true;
  }

  async function loadOverview() {
    const data = await api.request('/api/user/overview');
    const user = data.user;
	billingPriority = user.billing_priority === 'timed_first' ? 'timed_first' : 'subscription_first';
	const preferencesForm = document.querySelector('[data-preferences-form]');
	if (preferencesForm && preferencesForm.elements.prefer_timed_quota) preferencesForm.elements.prefer_timed_quota.checked = billingPriority === 'timed_first';
    const identity = data.identity || { provider: 'local', password_change_available: true };
    const profile = document.querySelector('[data-profile-form]');
    if (profile) profile.elements.username.value = user.username || '';
    api.setText('[data-account-id]', '#' + user.id);
    api.setText('[data-account-created]', api.date(user.created_at));
	const providerLabel = String(identity.provider || '').indexOf('linuxdo') === 0 ? 'LinuxDO' : (identity.provider === 'local' ? '本地账号' : identity.provider);
    api.setText('[data-account-provider]', providerLabel + (identity.provider_username ? ' · ' + identity.provider_username : ''));
    api.setText('[data-user-quota]', api.moneyFromQuota(user.quota, 2));
    api.setText('[data-card-quota]', api.moneyFromQuota(user.quota, 2));
    const timedQuota = Math.max(0, Number(data.timed_quota) || 0);
    api.setText('[data-card-timed-quota]', api.moneyFromQuota(timedQuota, 2));
    const timedQuotaPart = document.querySelector('[data-card-timed-quota-wrap]');
    if (timedQuotaPart) timedQuotaPart.classList.toggle('hidden', timedQuota <= 0);
	const groupQuotaCard = document.querySelector('[data-group-quota-card]');
	const groupQuotaRows = document.querySelector('[data-group-quota-rows]');
	if (groupQuotaCard) groupQuotaCard.classList.add('hidden');
	if (groupQuotaRows) groupQuotaRows.replaceChildren();
    api.setText('[data-card-used]', quotaUSD(user.used_quota));
    api.setText('[data-card-requests]', api.number(data.requests));
    api.setText('[data-card-tokens]', api.number(data.token_count));
    api.setText('[data-today-requests]', api.number(data.today.requests));
    api.setText('[data-today-tokens]', api.number(data.today.input_tokens + data.today.output_tokens));
    api.setText('[data-today-quota]', quotaUSD(data.today.quota));
    const subscriptions = Array.isArray(data.active_subscriptions) ? data.active_subscriptions : [];
    const card = document.querySelector('[data-active-subscription-card]');
    if (card) {
      card.classList.toggle('hidden', subscriptions.length === 0);
      if (subscriptions.length) {
        const item = subscriptions[0];
        const quota = item.unlimited_quota ? '不限额' : '剩余 ' + api.moneyFromQuota(item.quota_remaining, 2);
		const savings = Number(item.discount_savings_limit) > 0 ? ' · 已节省 ' + api.moneyFromQuota(item.discount_savings_used, 2) + ' / ' + api.moneyFromQuota(item.discount_savings_limit, 2) : '';
		const reset = item.next_reset_at ? ' · ' + api.date(item.next_reset_at) + ' 重置额度' : '';
        api.setText('[data-active-subscription-name]', item.plan_name || item.plan_code || '订阅计划');
		api.setText('[data-active-subscription-detail]', quota + ' · 订阅倍率 ×' + Number(item.discount_multiplier || 1).toFixed(2) + savings + reset + ' · ' + api.date(item.expires_at) + ' 到期' + (subscriptions.length > 1 ? ' · 另有 ' + (subscriptions.length - 1) + ' 个生效计划' : ''));
        const models = document.querySelector('[data-active-subscription-models]');
        const names = Object.keys(item.model_rules || {});
        if (models) models.innerHTML = names.length ? names.map(function (name) { return '<span class="tag">解锁 ' + api.escapeHTML(name) + '</span>'; }).join('') : '<span class="tag muted">公共模型折扣权益</span>';
      }
    }
  }

  async function loadUserSecurity() {
    userSecurity = await api.request('/api/user/security/totp');
    return userSecurity;
  }

  function bindAccountSecurityFeature() {
    const host = document.querySelector('[data-account-security-host]');
    if (!host) return;
    let pending = false;
    async function openWhenVisible() {
      const panel = document.querySelector('[data-panel="profile"]');
      if (!panel || !panel.classList.contains('active') || pending || host.dataset.mounted) return;
      pending = true;
      try {
        const loading = document.createElement('p'); loading.setAttribute('role', 'status'); loading.className = 'field-hint'; loading.textContent = '正在载入账户安全中心…'; host.replaceChildren(loading);
        if (!window.FurtherFeatures || typeof window.FurtherFeatures.mount !== 'function') throw new Error('账户安全模块尚未加载，请刷新当前管理页面后重试。');
        await window.FurtherFeatures.mount(host, { path: '/security' });
        loading.remove();
        host.dataset.mounted = 'true';
      } catch (error) {
        const notice = document.createElement('div'); notice.className = 'status-err'; notice.setAttribute('role', 'alert');
        notice.textContent = error.message || '账户安全暂时无法加载。';
        const retry = document.createElement('button'); retry.type = 'button'; retry.className = 'md-btn md-btn-outlined'; retry.textContent = '重试';
        retry.addEventListener('click', openWhenVisible); notice.append(retry);
        (host.shadowRoot || host).replaceChildren(notice);
      } finally { pending = false; }
    }
    window.addEventListener('further-ui:page', function (event) { if (event.detail && event.detail.name === 'profile') openWhenVisible(); });
    openWhenVisible();
  }

  function subscriptionStatus(item) {
    if (Number(item.status) === 1) return '<span class="tag success">生效中</span>';
    if (Number(item.status) === 2) return '<span class="tag error">已过期</span>';
    return '<span class="tag muted">已放弃</span>';
  }

  function subscriptionIsActive(item) {
    const now = Date.now();
    return Number(item.status) === 1 && new Date(item.starts_at).getTime() <= now && new Date(item.expires_at).getTime() > now;
  }

  function subscriptionRow(item) {
    const quota = item.unlimited_quota ? '不限额' : (Number(item.quota_remaining) <= 0 ? '额度耗尽' : api.moneyFromQuota(item.quota_remaining, 2));
    const unlocked = Object.keys(item.model_rules || {}).length;
    const active = subscriptionIsActive(item);
    const abandon = active ? '<button type="button" class="md-btn md-btn-text md-btn-compact" data-abandon-subscription="' + item.id + '">放弃计划</button>' : '';
    const deletable = Number(item.status) !== 1 || new Date(item.expires_at).getTime() <= Date.now();
    const remove = deletable ? '<button type="button" class="md-icon-button danger" title="删除订阅记录" data-delete-subscription="' + item.id + '"><span class="material-symbol" aria-hidden="true">delete</span></button>' : '';
    const upgrade = Number(item.upgrade_from_plan_id) > 0 ? ' · 由 ' + api.escapeHTML(item.upgrade_from_plan_name || '基础计划') + ' 升级' : '';
    const rpm = Number(item.requests_per_minute) > 0 ? Number(item.requests_per_minute) : 15;
    const concurrent = Number(item.concurrent_requests) > 0 ? Number(item.concurrent_requests) : 2;
	const savings = Number(item.discount_savings_limit) > 0 ? api.moneyFromQuota(item.discount_savings_used, 2) + ' / ' + api.moneyFromQuota(item.discount_savings_limit, 2) : '不限';
	const description = item.description ? '<div class="list-item-sub">' + api.escapeHTML(item.description) + '</div>' : '';
	const reset = item.next_reset_at ? api.date(item.next_reset_at) : (Number(item.reset_interval_days) > 0 ? '等待周期' : '不重置');
    return '<div class="list-item subscription-list-item"><span class="material-symbol" aria-hidden="true">today</span><div class="list-item-body"><div class="list-item-title">' + api.escapeHTML(item.plan_name || item.plan_code) + '</div><div class="list-item-sub"><code>' + api.escapeHTML(item.plan_code || '') + '</code> · ' + api.date(item.starts_at) + ' 至 ' + api.date(item.expires_at) + upgrade + '</div>' + description + '</div><div class="list-metrics"><div><b>' + quota + '</b>剩余</div><div><b>' + reset + '</b>下次重置</div><div><b>×' + Number(item.discount_multiplier || 1).toFixed(2) + '</b>计划倍率</div><div><b>' + savings + '</b>已省 / 上限</div><div><b>' + rpm + ' / ' + concurrent + '</b>RPM / 并发</div><div><b>' + unlocked + '</b>解锁模型</div></div><div class="list-actions">' + subscriptionStatus(item) + abandon + remove + '</div></div>';
  }

  function affectedSubscriptionsForAbandon(subscriptionID) {
    const active = subscriptionRows.filter(subscriptionIsActive);
    const target = active.find(function (item) { return Number(item.id) === Number(subscriptionID); });
    if (!target) return [];
    const selected = {};
    const removedPlanIDs = {};
    selected[target.id] = true;
    removedPlanIDs[target.plan_id] = true;
    let changed = true;
    while (changed) {
      changed = false;
      const remainingByPlan = {};
      active.forEach(function (item) { if (!selected[item.id]) remainingByPlan[item.plan_id] = Number(remainingByPlan[item.plan_id] || 0) + 1; });
      active.forEach(function (item) {
        const requiredPlanID = Number(item.upgrade_from_plan_id || 0);
        if (selected[item.id] || requiredPlanID <= 0 || !removedPlanIDs[requiredPlanID] || Number(remainingByPlan[requiredPlanID] || 0) > 0) return;
        selected[item.id] = true;
        removedPlanIDs[item.plan_id] = true;
        changed = true;
      });
    }
    return active.filter(function (item) { return selected[item.id]; });
  }

  function subscriptionPlanCard(plan) {
    const eligible = plan.upgrade_eligible !== false;
    const quota = plan.unlimited_quota ? '不限额' : api.moneyFromQuota(plan.quota, 2);
    const rpm = Number(plan.requests_per_minute) > 0 ? Number(plan.requests_per_minute) : 15;
    const concurrent = Number(plan.concurrent_requests) > 0 ? Number(plan.concurrent_requests) : 2;
    const rules = Array.isArray(plan.model_rules) ? plan.model_rules : [];
    const models = rules.length ? rules.map(function (rule) {
      return '<span class="tag">' + api.escapeHTML(rule.display_name || rule.model) + (Number(rule.multiplier) > 0 ? ' ×' + Number(rule.multiplier).toFixed(2) : '') + '</span>';
    }).join('') : '<span class="tag muted">公共模型优惠</span>';
    const upgrade = Number(plan.upgrade_from_plan_id) > 0 ? '<div class="plan-card-upgrade">' + (eligible ? '可从 ' : '需要先订阅 ') + '<b>' + api.escapeHTML(plan.upgrade_from_plan_name || plan.upgrade_from_plan_code) + '</b>' + (eligible ? ' 升级，兑换后两个计划并发生效。' : '，才能兑换此升级计划。') + '</div>' : '';
	const savingsLimit = Number(plan.discount_savings_limit) > 0 ? api.moneyFromQuota(plan.discount_savings_limit, 2) : '不限';
	const description = plan.description ? '<div class="plan-card-description">' + api.escapeHTML(plan.description) + '</div>' : '<div class="plan-card-description">包含公共模型折扣、访问上限与已配置的专属模型权益。</div>';
	const reset = Number(plan.reset_interval_days) > 0 ? ('每 ' + Number(plan.reset_interval_days) + ' 天') : '不重置';
    const redeemAction = eligible ? '<button type="button" class="md-btn md-btn-outlined md-btn-compact" data-plan-redeem="' + plan.id + '">使用兑换码</button>' : '<button type="button" class="md-btn md-btn-outlined md-btn-compact" disabled>暂不可升级</button>';
	return '<article class="md-card plan-card' + (eligible ? '' : ' locked') + '"><div class="plan-card-head"><div><div class="plan-card-name">' + api.escapeHTML(plan.name) + '</div><div class="plan-card-code"><code>' + api.escapeHTML(plan.code) + '</code></div></div>' + (plan.active ? '<span class="tag success">已订阅</span>' : '<span class="tag muted">可兑换</span>') + '</div>' + description + '<div class="plan-card-metrics"><div class="plan-card-metric">有效期<b>' + Number(plan.duration_days) + ' 天</b></div><div class="plan-card-metric">额度<b>' + quota + '</b></div><div class="plan-card-metric">额度重置<b>' + reset + '</b></div><div class="plan-card-metric">折扣倍率<b>×' + Number(plan.discount_multiplier || 1).toFixed(2) + '</b></div><div class="plan-card-metric">最高可节省<b>' + savingsLimit + '</b></div><div class="plan-card-metric">访问上限<b>' + rpm + ' RPM / ' + concurrent + ' 并发</b></div></div>' + upgrade + '<div class="plan-card-models">' + models + '</div><div class="plan-card-actions"><span class="card-hint">' + rules.length + ' 个专属模型</span><div class="plan-card-action-buttons">' + redeemAction + '</div></div></article>';
  }

  async function loadSubscriptionPlanCatalog() {
    const plans = await api.request('/api/user/subscription-plans');

    const body = document.querySelector('[data-subscription-plan-catalog]');
    const collapsedBody = document.querySelector('[data-collapsed-subscription-plan-catalog]');
    if (!body) return;
    const rows = Array.isArray(plans) ? plans : [];
	function byDisplayOrder(left, right) { return Number(left.display_order || 0) - Number(right.display_order || 0) || Number(left.id) - Number(right.id); }
	const primaryPlans = rows.filter(function (plan) { return !plan.collapsed; }).sort(byDisplayOrder);
	const collapsedPlans = rows.filter(function (plan) { return Boolean(plan.collapsed); }).sort(byDisplayOrder);
    const banner = collapsedPlans.length ? '<button type="button" class="more-plans-banner" data-more-subscription-plans><span class="material-symbol" aria-hidden="true">confirmation_number</span><span class="more-plans-banner-copy"><b>更多套餐</b><small>' + collapsedPlans.length + ' 个</small></span><span class="material-symbol" aria-hidden="true">arrow_forward</span></button>' : '';
    body.innerHTML = primaryPlans.length || collapsedPlans.length ? primaryPlans.map(subscriptionPlanCard).join('') + banner : '<div class="md-card empty">当前没有可兑换的订阅计划。</div>';
    if (collapsedBody) collapsedBody.innerHTML = collapsedPlans.length ? collapsedPlans.map(subscriptionPlanCard).join('') : '<div class="empty">当前没有更多套餐。</div>';
    function bindRedeemActions(container) {
      if (!container) return;
      container.querySelectorAll('[data-plan-redeem]').forEach(function (button) {
        button.addEventListener('click', function () {
		  const plan = rows.find(function (item) { return String(item.id) === String(button.dataset.planRedeem); });
		  const form = document.querySelector('[data-plan-redeem-form]');
		  if (!plan || !form) return;
          api.closeDialog('more-subscription-plans-dialog');
		  form.reset();
		  form.dataset.planId = String(plan.id);
		  api.setText('[data-plan-redeem-title]', '兑换 ' + (plan.name || plan.code));
		  api.setText('[data-plan-redeem-hint]', '输入订阅兑换码。');
		  const resultBody = document.querySelector('[data-plan-redeem-result]');
		  if (resultBody) resultBody.classList.add('hidden');
		  api.openDialog('plan-redeem-dialog');
		  setTimeout(function () { form.elements.code.focus(); }, 80);
        });
      });
    }
    bindRedeemActions(body);
    bindRedeemActions(collapsedBody);
    const moreButton = body.querySelector('[data-more-subscription-plans]');
    if (moreButton) moreButton.addEventListener('click', function () { api.openDialog('more-subscription-plans-dialog'); });
  }

  async function loadSubscriptions() {
    const result = await api.request('/api/user/subscriptions');
    const rows = Array.isArray(result) ? result : [];
    subscriptionRows = rows;
    const body = document.querySelector('[data-user-subscriptions]');
    if (!body) return;
    body.innerHTML = rows.length ? rows.map(subscriptionRow).join('') : '<div class="empty">还没有订阅，兑换订阅后会显示在这里。</div>';
    body.querySelectorAll('[data-abandon-subscription]').forEach(function (button) {
      button.addEventListener('click', function () {
        const form = document.querySelector('[data-abandon-subscription-form]');
        const affected = affectedSubscriptionsForAbandon(button.dataset.abandonSubscription);
        const selected = affected.find(function (item) { return Number(item.id) === Number(button.dataset.abandonSubscription); });
        if (!selected) return api.toast('该订阅已失效，请刷新后重试', 'danger');
        const dependents = affected.filter(function (item) { return Number(item.id) !== Number(selected.id); });
        const unlimited = Boolean(selected.unlimited_quota);
        const convertibleQuota = affected.reduce(function (total, item) { return total + (item.unlimited_quota ? 0 : Math.max(0, Number(item.quota_remaining || 0))); }, 0);
        form.dataset.subscriptionId = button.dataset.abandonSubscription;
        api.setText('[data-abandon-subscription-summary]', (selected.plan_name || selected.plan_code) + ' 的折扣和模型权限将立即停止。');
        const warning = document.querySelector('[data-abandon-cascade-warning]');
        if (warning) {
          warning.classList.toggle('hidden', dependents.length === 0);
          warning.textContent = dependents.length ? '该计划是升级链的唯一前置计划，不能单独放弃。' + dependents.map(function (item) { return item.plan_name || item.plan_code; }).join('、') + ' 也会按你的选择一并处理。' : '';
        }
        const convertDetail = unlimited ? '不限额计划没有可转换的固定额度。' : (dependents.length ? '受影响的有限额度计划共 ' + api.moneyFromQuota(convertibleQuota, 2) + '，将分别转换为通用限时额度并沿用各自到期日。' : api.moneyFromQuota(selected.quota_remaining, 2) + ' 将转换为通用限时额度，有效期至 ' + api.date(selected.expires_at) + '。');
        api.setText('[data-abandon-convert-detail]', convertDetail);
        const convert = form.elements.mode[0];
        const end = form.elements.mode[1];
        convert.disabled = unlimited;
        document.querySelector('[data-abandon-convert-option]').classList.toggle('disabled', unlimited);
        if (unlimited) end.checked = true;
        else convert.checked = true;
        api.openDialog('abandon-subscription-dialog');
      });
    });
    body.querySelectorAll('[data-delete-subscription]').forEach(function (button) {
      button.addEventListener('click', async function () {
        if (!(await api.confirm('删除后这条订阅历史将无法恢复，已转换的通用限时额度不会受影响。', { title: '删除订阅记录', confirmText: '删除' }))) return;
        try {
          await api.request('/api/user/subscriptions/' + button.dataset.deleteSubscription, { method: 'DELETE' });
          api.toast('订阅记录已删除');
          await loadSubscriptions();
        } catch (error) { api.toast(error.message, 'danger'); }
      });
    });
  }

  function timedQuotaStatus(item) {
    if (Number(item.status) === 1) return '<span class="tag success">可用</span>';
    if (Number(item.status) === 2) return '<span class="tag muted">已用完</span>';
    return '<span class="tag error">已过期</span>';
  }

  function timedQuotaRow(item) {
    return '<div class="list-item"><span class="material-symbol" aria-hidden="true">data_usage</span><div class="list-item-body"><div class="list-item-title">' + api.escapeHTML(item.name || '限时额度') + '</div><div class="list-item-sub">有效期至 ' + api.date(item.expires_at) + ' · 不包含订阅折扣与模型解锁</div></div><div class="list-metrics"><div><b>' + api.moneyFromQuota(item.quota_remaining, 2) + '</b>剩余</div><div><b>' + api.moneyFromQuota(item.quota_total, 2) + '</b>原始额度</div></div>' + timedQuotaStatus(item) + '</div>';
  }

  async function loadTimedQuotas() {
    const body = document.querySelector('[data-user-timed-quotas]');
    if (!body) return;
    let result;
    try {
      result = await api.request('/api/user/timed-quotas');
    } catch (error) {
      body.innerHTML = '<div class="status-line"><span class="material-symbol" aria-hidden="true">info</span><span>限时额度暂时无法读取，不影响永久余额、订阅与 API 调用。稍后进入本页时会自动重试。</span></div>';
      throw error;
    }
    const rows = Array.isArray(result) ? result : [];
    body.innerHTML = rows.length ? rows.map(timedQuotaRow).join('') : '<div class="empty">当前没有通用限时额度。</div>';
  }

  function userBenefitStatus(item) {
    if (Number(item.status) === 1) return '<span class="tag success">待领取</span>';
    if (Number(item.status) === 2) return '<span class="tag muted">已领取</span>';
	if (Number(item.status) === 4) return '<span class="tag error">已撤回</span>';
    return '<span class="tag error">已失效</span>';
  }

  function userBenefitRow(item) {
    const benefits = [];
    if (Number(item.permanent_quota) > 0) benefits.push('<div><b>' + api.moneyFromQuota(item.permanent_quota, 2) + '</b>永久余额</div>');
    if (Number(item.timed_quota) > 0) benefits.push('<div><b>' + api.moneyFromQuota(item.timed_quota, 2) + '</b>限时余额</div>');
    if (item.restore_quota_percent) benefits.push('<div><b>补回总额 ' + api.escapeHTML(String(item.restore_quota_percent)) + '%</b>套餐额度（不超过总额）</div>');
    if (item.timed_quota_expires_at) benefits.push('<div><b>' + api.date(item.timed_quota_expires_at) + '</b>限时额度到期</div>');
    if (item.reset_subscription_quota) benefits.push('<div><b>恢复至总额</b>套餐额度</div>');
    const description = item.description ? '<div class="list-item-sub">' + api.escapeHTML(item.description) + '</div>' : '';
    const claim = item.can_claim ? '<button type="button" class="md-btn md-btn-filled md-btn-compact" data-claim-benefit="' + item.id + '">领取</button>' : '';
	const dismiss = Number(item.status) === 2 ? '<button type="button" class="md-btn md-btn-text md-btn-compact" data-dismiss-benefit="' + item.id + '">忽略</button>' : '';
    return '<div class="md-card benefit-card"><div class="list-item"><span class="material-symbol" aria-hidden="true">redeem</span><div class="list-item-body"><div class="list-item-title">' + api.escapeHTML(item.name) + '</div><div class="list-item-sub">' + api.escapeHTML(item.plan_name) + ' · ' + api.date(item.claim_expires_at) + ' 前可领取</div>' + description + '</div><div class="list-metrics">' + benefits.join('') + '</div><div class="list-actions">' + userBenefitStatus(item) + claim + dismiss + '</div></div></div>';
  }

  function renderTokenLoan(status) {
    status = status || { visible:false };
    const card = document.querySelector('[data-token-loan-card]');
    if (!card) return;
    card.classList.toggle('hidden', !status.visible);
    if (!status.visible) return;
    const loan = status.loan || null;
    const active = Boolean(loan && Number(loan.status) === 1);
    api.setText('[data-token-loan-max]', api.moneyFromQuota(status.max_borrow_quota || 0, 2));
    api.setText('[data-token-loan-remaining]', active ? api.moneyFromQuota(loan.remaining_quota || 0, 2) : '已还清');
    api.setText('[data-token-loan-progress]', active ? Number(loan.checkin_count || 0) + ' / 4 天' : (status.borrow_paused ? '亏损池暂停' : '可再次借款'));
    api.setText('[data-token-loan-bonus]', Number(status.temporary_bonus_quota) > 0 ? '+' + api.moneyFromQuota(status.temporary_bonus_quota, 2) : '暂无');
	const pool = status.loss_pool || {};
	const poolBox = document.querySelector('[data-token-loan-loss-pool]');
	if (poolBox) poolBox.classList.toggle('hidden', pool.visible === false || Number(pool.total_quota || 0) <= 0);
	api.setText('[data-token-loan-loss-pool-title]', pool.title || '帮助站长砍一刀');
	const progress = Math.max(0, Math.min(100, Number(pool.progress_percent || 0)));
	api.setText('[data-token-loan-loss-pool-percent]', progress.toFixed(2) + '%');
	api.setText('[data-token-loan-loss-pool-remaining]', api.moneyFromQuota(pool.remaining_quota || 0, 2));
	api.setText('[data-token-loan-loss-pool-total]', api.moneyFromQuota(pool.total_quota || 0, 2));
	const progressBar = document.querySelector('[data-token-loan-loss-pool-progress]');
	if (progressBar) { progressBar.setAttribute('aria-valuenow', progress.toFixed(2)); const fill = progressBar.querySelector('span'); if (fill) fill.style.width = progress + '%'; }
	const cutsToday = Number(pool.user_cuts_today || 0);
	const dailyCutLimit = Number(pool.daily_cut_limit || 2);
	const poolStatus = pool.active ? (cutsToday >= dailyCutLimit ? '今天已完成 ' + dailyCutLimit + ' 次，共砍 ' + api.moneyFromQuota(pool.user_cut_quota || 0, 2) + '，明天 00:00（UTC+8）后重置。' : (cutsToday > 0 ? '已砍 ' + cutsToday + ' 次，共 ' + api.moneyFromQuota(pool.user_cut_quota || 0, 2) + '；今天还可继续。' : (pool.description || '每刀随机 $1–$3，每位用户每天可连续砍两刀；清零前暂停公益贷新借款。'))) : '亏损池已清零，公益贷已经恢复发放。';
	api.setText('[data-token-loan-loss-pool-status]', poolStatus);
	const cutButton = document.querySelector('[data-token-loan-loss-pool-cut]');
	if (cutButton) { cutButton.disabled = !pool.can_cut; cutButton.textContent = pool.active ? (cutsToday >= dailyCutLimit ? '今日已砍两刀' : (cutsToday > 0 ? '再砍一刀（1 / 2）' : '帮站长砍一刀')) : '已经砍完'; }
    const borrowForm = document.querySelector('[data-token-loan-borrow-form]');
    if (borrowForm) {
      borrowForm.classList.toggle('hidden', !status.can_borrow);
      const maxQuota = Math.max(0, Number(status.max_borrow_quota) || 0);
      const maxCents = Math.floor(maxQuota / 5000);
      const input = borrowForm.elements.borrow_usd;
      borrowForm.dataset.maxBorrowQuota = String(maxQuota);
      input.max = (maxCents / 100).toFixed(2);
      if (Number(input.value) > maxCents / 100) input.value = input.max;
    }
    const activeBox = document.querySelector('[data-token-loan-active]');
    if (activeBox) activeBox.classList.toggle('hidden', !active);
    const checkin = document.querySelector('[data-token-loan-checkin]');
    if (checkin) {
      checkin.disabled = !status.can_checkin;
      checkin.textContent = status.checked_in_today ? '今日已签到' : '今日签到还款';
    }
    api.setText('[data-token-loan-status]', status.checked_in_today ? '今日已签到' : (active ? '今日可签到' : (status.enabled ? '暂无借款' : '词元贷已关闭')));
    const history = document.querySelector('[data-token-loan-history]');
    const items = Array.isArray(status.checkins) ? status.checkins : [];
    if (history) history.innerHTML = items.length ? '<div class="token-loan-history-title">本笔签到记录</div>' + items.map(function (item) { const overflow = Number(item.overflow_quota) > 0 ? ' · 溢出 +' + api.moneyFromQuota(item.overflow_quota, 2) : ''; return '<div class="token-loan-history-row"><span>' + api.escapeHTML(item.checkin_date) + '</span><b>签到 ' + api.moneyFromQuota(item.awarded_quota || item.repayment_quota, 2) + '</b><small>还款 ' + api.moneyFromQuota(item.repayment_quota, 2) + overflow + ' · 剩余 ' + api.moneyFromQuota(item.remaining_after, 2) + '</small></div>'; }).join('') : '';
  }

  async function loadTokenLoan() {
    try {
      const status = await api.request('/api/user/public-benefits/token-loan');
      renderTokenLoan(status);
      return status;
    } catch (error) {
      renderTokenLoan({ visible:false });
      return { visible:false };
    }
  }











  async function loadUserBenefits() {
    const pair = await Promise.all([api.request('/api/user/benefits'), loadTokenLoan()]);
    const result = pair[0];
    const tokenLoan = pair[1] || { visible:false };
    const rows = Array.isArray(result && result.items) ? result.items : [];
    const pending = Number(result && result.pending_count) || 0;
    const banner = document.querySelector('[data-user-benefits-banner]');
    if (banner) banner.classList.toggle('hidden', pending <= 0 && !tokenLoan.visible);
    api.setText('[data-user-benefits-banner-title]', tokenLoan.visible && pending <= 0 ? '公益福利 · 词元贷' : '我的福利');
    let bannerDetail = pending > 0 ? '你有 ' + pending + ' 份套餐福利待领取。' : '';
	if (tokenLoan.visible) {
	  const lossPool = tokenLoan.loss_pool || {};
	  const loanDetail = tokenLoan.loan && Number(tokenLoan.loan.status) === 1 ? (tokenLoan.checked_in_today ? '词元贷今日已签到。' : '词元贷今日可签到还款。') : (lossPool.active ? '长上下文亏损池未清零，帮站长砍一刀后等待恢复公益贷。' : '公益词元贷当前可借。');
	  bannerDetail += (bannerDetail ? ' ' : '') + loanDetail;
	}
    api.setText('[data-user-benefits-banner-detail]', bannerDetail || '当前没有待领取福利。');
    const body = document.querySelector('[data-user-benefit-rows]');
    if (!body) return;
    body.innerHTML = rows.length ? rows.map(userBenefitRow).join('') : '<div class="md-card empty">当前没有套餐福利。</div>';
    body.querySelectorAll('[data-claim-benefit]').forEach(function (button) {
      button.addEventListener('click', async function () {
        api.showBusy(button, true);
        try {
          await api.request('/api/user/benefits/' + button.dataset.claimBenefit + '/claim', { method: 'POST' });
          api.toast('套餐福利领取成功');
          await Promise.all([loadUserBenefits(), loadOverview(), loadSubscriptions(), loadTimedQuotas()]);
        } catch (error) { api.toast(error.message, 'danger'); }
        finally { api.showBusy(button, false); }
      });
    });
	body.querySelectorAll('[data-dismiss-benefit]').forEach(function (button) {
	  button.addEventListener('click', async function () {
		api.showBusy(button, true);
		try {
		  await api.request('/api/user/benefits/' + button.dataset.dismissBenefit, { method: 'DELETE' });
		  api.toast('已忽略该条已领取福利');
		  await loadUserBenefits();
		} catch (error) { api.toast(error.message, 'danger'); }
		finally { api.showBusy(button, false); }
	  });
	});
  }

  function bindTokenLoan() {
    const form = document.querySelector('[data-token-loan-borrow-form]');
    if (form) form.addEventListener('submit', async function (event) {
      event.preventDefault();
      const button = form.querySelector('button[type="submit"]');
      api.showBusy(button, true);
      try {
        const input = form.elements.borrow_usd;
        const dollars = Number(input.value);
        const cents = Math.round(dollars * 100);
        const roundedQuota = cents * 5000;
        const maxQuota = Number(form.dataset.maxBorrowQuota);
        if (!Number.isFinite(dollars) || cents < 1 || !Number.isSafeInteger(roundedQuota) || Math.abs(dollars * 100 - cents) > 1e-7 || !Number.isFinite(maxQuota) || roundedQuota > maxQuota) {
          throw new Error('请输入不超过最高可借金额的美元数，精确到美分');
        }
        input.value = (cents / 100).toFixed(2);
        const result = await api.request('/api/user/public-benefits/token-loan/borrow', { method:'POST', body:{ quota:roundedQuota } });
        api.toast(result.message || '词元贷额度已到账');
        await Promise.all([loadUserBenefits(), loadOverview()]);
      } catch (error) { api.toast(error.message, 'danger'); }
      finally { api.showBusy(button, false); }
    });
    const checkin = document.querySelector('[data-token-loan-checkin]');
    if (checkin) checkin.addEventListener('click', async function () {
      api.showBusy(checkin, true);
      try {
        const result = await api.request('/api/user/public-benefits/token-loan/check-in', { method:'POST' });
		api.showBusy(checkin, false);
		if (result.status) renderTokenLoan(result.status);
        api.toast(result.message || '签到还款完成');
        await loadUserBenefits();
      } catch (error) { api.toast(error.message, 'danger'); }
	  finally { if (checkin.dataset.busy === 'true') api.showBusy(checkin, false); }
    });
	const cutButton = document.querySelector('[data-token-loan-loss-pool-cut]');
	if (cutButton) cutButton.addEventListener('click', async function () {
	  api.showBusy(cutButton, true);
	  try {
		const result = await api.request('/api/user/public-benefits/token-loan/loss-pool/cut', { method:'POST' });
		api.showBusy(cutButton, false);
		if (result.status) renderTokenLoan(result.status);
		api.toast((result.message || '成功帮站长砍了一刀') + '：' + api.moneyFromQuota(result.cut_quota || 0, 2));
		await loadUserBenefits();
	  } catch (error) { api.toast(error.message, 'danger'); }
	  finally { if (cutButton.dataset.busy === 'true') api.showBusy(cutButton, false); }
	});
  }

  function bindAbandonSubscription() {
    const form = document.querySelector('[data-abandon-subscription-form]');
    if (!form) return;
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      const mode = form.elements.mode.value;
      const button = form.querySelector('button[type="submit"]');
      api.showBusy(button, true);
      try {
        const payload = await protectedUserRequest('/api/user/subscriptions/' + form.dataset.subscriptionId + '/abandon', { method: 'POST', body: { mode: mode } }, userSecurity.abandon_required, { title: '验证后放弃计划', message: '此操作会结束订阅权益，请输入身份验证器中的 6 位验证码。', confirmText: '验证并继续' });
        if (!payload) return;
        api.closeDialog('abandon-subscription-dialog');
        const result = payload.result || {};
        const affectedCount = Array.isArray(result.affected_subscriptions) ? result.affected_subscriptions.length : 1;
        const prefix = affectedCount > 1 ? affectedCount + ' 个关联计划已结束，' : '计划已结束，';
        api.toast(mode === 'convert' ? prefix + api.moneyFromQuota(result.converted_quota, 2) + ' 已转为通用限时额度' : prefix.replace(/，$/, ''));
        await Promise.all([loadOverview(), loadSubscriptions(), loadTimedQuotas(), consoleModels.load(), loadSubscriptionPlanCatalog()]);
      } catch (error) { api.toast(error.message, 'danger'); }
      finally { api.showBusy(button, false); }
    });
  }

  function tokenRow(token) {
	const modelLimits = Array.isArray(token.model_limits) && token.model_limits.length ? token.model_limits.join(', ') : '全部模型';
	let expiry = '';
	if (token.expired_at_unavailable) expiry = ' · 到期时间无法显示';
	else if (token.expired_at) expiry = ' · 到期 ' + api.date(token.expired_at);
	const restrictions = modelLimits + (token.ip_limits ? ' · IP ' + token.ip_limits : '') + expiry;
	const encoded = encodeURIComponent(JSON.stringify(token));
	const enabled = Number(token.status) === 1;
	const editAttributes = token.expired_at_unavailable ? 'disabled title="到期时间无法显示，原始设置已保留，暂无法在页面编辑"' : 'title="编辑 API Key"';
    return '<div class="list-item compact-row">' +
      '<span class="material-symbol" aria-hidden="true">key</span>' +
	  '<div class="list-item-body"><div class="list-item-title">' + api.escapeHTML(token.name) + '</div><div class="list-item-sub"><code>' + api.escapeHTML(token.key) + '</code> · ' + api.escapeHTML(restrictions) + '</div></div>' +
      '<div class="list-metrics"><div><b>' + (token.unlimited_quota ? '不限额' : Number(token.remain_quota) < 0 ? '跟随账户' : api.moneyFromQuota(token.remain_quota, 2)) + '</b>剩余</div><div><b>' + quotaUSD(token.used_quota) + '</b>已用</div></div>' +
	  '<div class="list-actions"><span>' + api.statusLabel(token.status, 'token') + '</span><button type="button" class="md-icon-button" ' + editAttributes + ' data-edit-token="' + encoded + '"><span class="material-symbol" aria-hidden="true">edit</span></button><button type="button" class="md-icon-button" title="轮换 API Key" aria-label="轮换 API Key" data-rotate-token="' + token.id + '"><span class="material-symbol" aria-hidden="true">refresh</span></button><button type="button" class="md-icon-button ' + (enabled ? 'danger' : '') + '" title="' + (enabled ? '禁用' : '启用') + ' API Key" data-toggle-token="' + token.id + '" data-next-status="' + (enabled ? 2 : 1) + '"><span class="material-symbol" aria-hidden="true">' + (enabled ? 'block' : 'check_circle') + '</span></button><button type="button" class="md-icon-button danger" title="删除 API Key" data-delete-token="' + token.id + '"><span class="material-symbol" aria-hidden="true">delete</span></button></div>' +
      '</div>';
  }

	function tokenDateTimeLocal(value) {
	  if (!value) return '';
	  const date = new Date(value);
	  if (Number.isNaN(date.getTime())) return '';
	  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60000);
	  return local.toISOString().slice(0, 16);
	}

	// The model whitelist keeps the original form field contract (model_limits)
	// while presenting a Further-styled, animated multi-select.  A hidden input
	// is submitted to the existing token endpoints so older servers continue to
	// accept the same comma-separated value.
	function modelPickerValues(value) {
	  const values = Array.isArray(value) ? value : String(value || '').split(/[\s,;]+/);
	  const seen = Object.create(null);
	  return values.map(function (item) { return String(item || '').trim(); }).filter(function (item) {
		if (!item || seen[item]) return false;
		seen[item] = true;
		return true;
	  });
	}

	function modelPickerEscape(value) {
	  return api.escapeHTML(String(value || ''));
	}

	function modelPickerState(form) {
	  if (!form) return null;
	  if (form._modelPicker) return form._modelPicker;
	  const field = form.querySelector('[data-model-picker]');
	  if (!field) return null;
	  const trigger = field.querySelector('[data-model-picker-trigger]');
	  const menu = field.querySelector('[data-model-picker-menu]');
	  const search = field.querySelector('[data-model-picker-search]');
	  const options = field.querySelector('[data-model-picker-options]');
	  const valueInput = field.querySelector('[data-model-picker-value]');
	  if (!trigger || !menu || !search || !options || !valueInput) return null;
	  const state = {
		form: form, field: field, trigger: trigger, menu: menu, search: search,
		options: options, valueInput: valueInput, selected: modelPickerValues(valueInput.value),
		models: [], query: '', open: false, loaded: false, loading: false, error: '', focusFrame: 0
	  };
	  form._modelPicker = state;

	  function selectedSet() {
		const result = Object.create(null);
		state.selected.forEach(function (item) { result[item] = true; });
		return result;
	  }

	  function visibleModels() {
		const query = state.query.trim().toLowerCase();
		return state.models.filter(function (item) { return !query || item.toLowerCase().indexOf(query) >= 0; });
	  }

	  function render() {
		const selected = selectedSet();
		const visible = visibleModels();
		const label = field.querySelector('[data-model-picker-label]');
		const summary = field.querySelector('[data-model-picker-summary]');
		const count = field.querySelector('[data-model-picker-count]');
		const empty = field.querySelector('[data-model-picker-empty]');
		const chips = field.querySelector('[data-model-picker-chips]');
		valueInput.value = state.selected.join(', ');
		if (label) label.textContent = state.selected.length ? '已选择 ' + state.selected.length + ' 个模型' : '留空：不限模型';
		if (summary) summary.textContent = state.selected.length ? state.selected.join(' · ') : '点击展开选择模型';
		if (count) count.textContent = state.selected.length ? '已选择 ' + state.selected.length + ' 个模型' : '未选择模型';
		if (chips) chips.innerHTML = state.selected.map(function (item) {
		  return '<span class="model-picker-chip"><span>' + modelPickerEscape(item) + '</span><button type="button" data-model-picker-remove="' + encodeURIComponent(item) + '" aria-label="移除 ' + modelPickerEscape(item) + '"><span class="material-symbol" aria-hidden="true">close</span></button></span>';
		}).join('');
		if (state.loading) {
		  options.innerHTML = '<div class="model-picker-empty">加载模型列表…</div>';
		} else if (state.error) {
		  options.innerHTML = '<div class="model-picker-empty model-picker-error">' + modelPickerEscape(state.error) + '<small>可以在搜索框输入模型名称并回车添加。</small></div>';
		} else if (!visible.length) {
		  options.innerHTML = '<div class="model-picker-empty">' + (state.models.length ? '没有匹配的模型' : '暂无可用模型') + '<small>可以在搜索框输入模型名称并回车添加。</small></div>';
		} else {
		  options.innerHTML = visible.map(function (item, index) {
			const active = Boolean(selected[item]);
			return '<button type="button" class="model-picker-option' + (active ? ' selected' : '') + '" role="option" aria-selected="' + String(active) + '" data-model-picker-option="' + encodeURIComponent(item) + '" tabindex="-1"><span class="model-picker-option-check material-symbol" aria-hidden="true">' + (active ? 'check_circle' : 'radio_button_unchecked') + '</span><span class="model-picker-option-copy"><b>' + modelPickerEscape(item) + '</b><small>可用于此 API Key</small></span></button>';
		  }).join('');
		}
	  }

	  function setSelected(values) {
		state.selected = modelPickerValues(values);
		render();
	  }

	  function toggleSelected(model) {
		const index = state.selected.indexOf(model);
		if (index >= 0) state.selected.splice(index, 1);
		else state.selected.push(model);
		render();
	  }

	  function addModel(model) {
		const normalized = String(model || '').trim();
		if (!normalized) return false;
		if (state.models.indexOf(normalized) < 0) state.models.push(normalized);
		if (state.selected.indexOf(normalized) < 0) state.selected.push(normalized);
		render();
		return true;
	  }

	  function applyMenuVisibility(shown) {
        menu.hidden = !shown;
        menu.classList.toggle('is-open', shown);
      }
      function animateMenu(visible, immediate) {
        if (api.transitionDropdown) api.transitionDropdown(menu, visible, applyMenuVisibility, { immediate: immediate === true });
        else applyMenuVisibility(visible);
      }
	  function close(immediate) {
        if (!state.open && !immediate) return;
        state.open = false;
        if (state.focusFrame) window.cancelAnimationFrame(state.focusFrame);
        state.focusFrame = 0;
        trigger.setAttribute('aria-expanded', 'false');
        if (menu.contains(document.activeElement)) trigger.focus({ preventScroll: true });
        animateMenu(false, immediate);
      }
	  function open(focusSearch) {
        state.open = true;
        trigger.setAttribute('aria-expanded', 'true');
        animateMenu(true);
		if (!state.loaded && !state.loading) {
		  state.loading = true;
		  render();
		  api.request('/api/user/models').then(function (payload) {
			const rows = Array.isArray(payload) ? payload : (payload && Array.isArray(payload.data) ? payload.data : (payload && Array.isArray(payload.items) ? payload.items : []));
			state.models = modelPickerValues(rows.map(function (item) { return typeof item === 'string' ? item : item && (item.id || item.model || item.name); }));
			state.loaded = true;
			state.error = '';
		  }).catch(function (error) {
			state.loaded = true;
			state.error = error && error.message ? error.message : '模型列表加载失败';
		  }).finally(render);
		}
		if (state.focusFrame) window.cancelAnimationFrame(state.focusFrame);
        state.focusFrame = focusSearch ? window.requestAnimationFrame(function () { state.focusFrame = 0; if (state.open && menu.isConnected) search.focus({ preventScroll: true }); }) : 0;
	  }

      menu.addEventListener('keydown', function (event) {
        if (event.key === 'Escape') { event.preventDefault(); close(); trigger.focus({ preventScroll: true }); }
      });
      document.addEventListener('focusin', function (event) { if (state.open && !field.contains(event.target)) close(); });
	  trigger.addEventListener('click', function () { if (state.open) close(); else open(false); });
	  trigger.addEventListener('keydown', function (event) {
		if (event.key === 'ArrowDown' && !state.open) { event.preventDefault(); open(true); }
	  });
	  const closeButton = field.querySelector('[data-model-picker-close]');
	  if (closeButton) closeButton.addEventListener('click', close);
	  search.addEventListener('input', function () { state.query = search.value; render(); });
	  search.addEventListener('keydown', function (event) {
		if (event.key === 'Escape') { event.preventDefault(); close(); trigger.focus(); return; }
		if (event.key === 'Enter') {
		  event.preventDefault();
		  const query = search.value.trim();
		  if (!query) return;
		  const exact = state.models.find(function (item) { return item.toLowerCase() === query.toLowerCase(); });
		  if (exact) toggleSelected(exact);
		  else addModel(query);
		  search.select();
		}
	  });
	  options.addEventListener('click', function (event) {
		const option = event.target.closest('[data-model-picker-option]');
		if (option) {
		  const model = decodeURIComponent(option.dataset.modelPickerOption || '');
		  if (model) toggleSelected(model);
		  return;
		}
	  });
	  const chips = field.querySelector('[data-model-picker-chips]');
	  if (chips) chips.addEventListener('click', function (event) {
		const remove = event.target.closest('[data-model-picker-remove]');
		if (!remove) return;
		const model = decodeURIComponent(remove.dataset.modelPickerRemove || '');
		if (model) setSelected(state.selected.filter(function (item) { return item !== model; }));
	  });
	  options.addEventListener('keydown', function (event) {
		const choices = Array.from(options.querySelectorAll('[data-model-picker-option]'));
		const current = document.activeElement;
		const index = choices.indexOf(current);
		if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
		  event.preventDefault();
		  if (!choices.length) return;
		  const next = event.key === 'ArrowDown' ? (index + 1) % choices.length : (index <= 0 ? choices.length - 1 : index - 1);
		  choices[next].focus();
		} else if ((event.key === 'Enter' || event.key === ' ') && current && current.dataset.modelPickerOption) {
		  event.preventDefault();
		  current.click();
		}
	  });
	  const clear = field.querySelector('[data-model-picker-clear]');
	  if (clear) clear.addEventListener('click', function () { setSelected([]); });
	  const selectAll = field.querySelector('[data-model-picker-select-all]');
	  if (selectAll) selectAll.addEventListener('click', function () { visibleModels().forEach(function (item) { if (state.selected.indexOf(item) < 0) state.selected.push(item); }); render(); });
	  document.addEventListener('click', function (event) { if (state.open && !field.contains(event.target)) close(); });
	  state.render = render;
	  state.setSelected = setSelected;
	  state.close = close;
	  render();
	  return state;
	}

	function syncModelPicker(form, values) {
	  const state = modelPickerState(form);
	  if (!state) return;
	  state.setSelected(modelPickerValues(values));
	  state.query = '';
	  state.search.value = '';
	  state.error = '';
      state.close(true);
	  state.render();
	}

	function resetTokenForm() {
	  const form = document.querySelector('[data-create-token]');
	  if (!form) return;
	  const picker = modelPickerState(form);
	  form.reset();
	  form.elements.id.value = '';
	  if (picker) syncModelPicker(form, []);
	  api.setText('[data-token-dialog-title]', '新建 API Key');
	  api.setText('[data-token-submit]', '创建');
	}

	function editTokenForm(token) {
	  const form = document.querySelector('[data-create-token]');
	  if (!form) return;
	  form.elements.id.value = token.id;
	  form.elements.name.value = token.name || '';
	  modelPickerState(form);
	  syncModelPicker(form, Array.isArray(token.model_limits) ? token.model_limits : []);
	  form.elements.ip_limits.value = token.ip_limits || '';
	  form.elements.expires_at.value = tokenDateTimeLocal(token.expired_at);
	  api.setText('[data-token-dialog-title]', '编辑 API Key');
	  api.setText('[data-token-submit]', '保存');
	  api.openDialog('token-dialog');
	}

  function bindTokenPagination() {
    [['[data-token-page-prev]', -1], ['[data-token-page-next]', 1]].forEach(function (entry) {
      const button = document.querySelector(entry[0]);
      if (!button || button.dataset.pageBound === 'true') return;
      button.dataset.pageBound = 'true';
      button.addEventListener('click', async function () {
        if (button.disabled) return;
        const previous = tokenState.page;
        tokenState.page = Math.max(1, previous + entry[1]);
        try { await loadTokens(); }
        catch (error) { tokenState.page = previous; syncTokenPagination(); api.toast(error.message, 'danger'); }
      });
    });
  }

  function syncTokenPagination() {
    const previous = document.querySelector('[data-token-page-prev]');
    const next = document.querySelector('[data-token-page-next]');
    if (previous) previous.disabled = tokenState.page <= 1;
    if (next) next.disabled = tokenState.page * tokenState.pageSize >= tokenState.total;
  }

  async function loadTokens() {
    const generation = ++tokenState.generation;
    document.querySelectorAll('[data-token-page-prev], [data-token-page-next]').forEach(function (button) { button.disabled = true; });
    try {
    const result = await api.request('/api/user/tokens?page=' + tokenState.page + '&page_size=' + tokenState.pageSize);
    if (generation !== tokenState.generation) return;
    if (!result || !Array.isArray(result.items) || !Number.isSafeInteger(result.total) || result.total < 0) throw new Error('服务器未返回有效 API Key 列表，请重试。');
    const rows = result.items;
    tokenState.total = result.total;
    const lastPage = Math.max(1, Math.ceil(tokenState.total / tokenState.pageSize));
    if (tokenState.page > lastPage) { tokenState.page = lastPage; return await loadTokens(); }
    api.setText('[data-token-page-summary]', '第 ' + tokenState.page + ' / ' + lastPage + ' 页 · 共 ' + api.number(tokenState.total) + ' 个 Key');
    const body = document.querySelector('[data-token-rows]');
    if (!body) return;
    body.innerHTML = rows.length ? rows.map(tokenRow).join('') : '<div class="empty">还没有 API Key，创建一个开始使用。</div>';
	body.querySelectorAll('[data-edit-token]').forEach(function (button) {
	  button.addEventListener('click', function () {
		try { editTokenForm(JSON.parse(decodeURIComponent(button.dataset.editToken))); } catch (_) { api.toast('读取 Key 配置失败', 'danger'); }
	  });
	});
	body.querySelectorAll('[data-toggle-token]').forEach(function (button) {
	  button.addEventListener('click', async function () {
		try {
		  await api.request('/api/user/tokens/' + button.dataset.toggleToken, { method: 'PUT', body: { status: Number(button.dataset.nextStatus) } });
		  api.toast(Number(button.dataset.nextStatus) === 1 ? 'API Key 已启用' : 'API Key 已禁用');
		  await loadTokens();
		} catch (error) { api.toast(error.message, 'danger'); }
	  });
	});
	body.querySelectorAll('[data-rotate-token]').forEach(function (button) {
	  button.addEventListener('click', async function () {
		if (!(await api.confirm('轮换后旧 Key 将立即失效。请确认所有客户端都能及时更新。', { title: '轮换 API Key', confirmText: '确认轮换' }))) return;
		try {
		  const result = await protectedUserRequest('/api/user/tokens/' + button.dataset.rotateToken + '/rotate', { method: 'POST' }, userSecurity.key_required, { title: '验证后轮换 Key', message: '轮换会生成新的完整 Key，请输入身份验证器中的 6 位验证码。', confirmText: '验证并轮换' });
		  if (!result) return;
		  const reveal = document.querySelector('[data-created-token]');
		  if (reveal) reveal.textContent = result.key;
		  document.querySelector('#created-token-dialog .md-dialog-title').textContent = 'API Key 已轮换';
		  api.openDialog('created-token-dialog');
		  await loadTokens();
		} catch (error) { api.toast(error.message, 'danger'); }
	  });
	});
    body.querySelectorAll('[data-delete-token]').forEach(function (button) {
      button.addEventListener('click', async function () {
        if (!(await api.confirm('删除后这个 Key 将立即失效且无法恢复，历史调用记录会保留。', { title: '删除 API Key', confirmText: '删除' }))) return;
        try {
          await api.request('/api/user/tokens/' + button.dataset.deleteToken, { method: 'DELETE' });
          api.toast('API Key 已删除');
          await loadTokens();
          await loadOverview();
        } catch (error) { api.toast(error.message, 'danger'); }
      });
    });
    } finally {
      if (generation === tokenState.generation) {
        syncTokenPagination();
      }
    }
  }


  function logRow(item, index) {
    const failed = item.failed === true || Number(item.status_code) >= 400 && Number(item.status_code) <= 599;
    const statusLabel = Number(item.status_code) > 0 ? Number(item.status_code) : failed ? '失败' : '未记录';
    const totalTokens = Number(item.prompt_tokens || 0) + Number(item.completion_tokens || 0);
    const tokenBreakdown = '输入 ' + api.number(item.prompt_tokens) + ' · 缓存读 ' + api.number(item.cached_tokens) + ' · 缓存写 ' + api.number(item.cache_write_tokens) + ' · 输出 ' + api.number(item.completion_tokens);
    const compactTokenBreakdown = '入 ' + api.number(item.prompt_tokens) + ' · 读 ' + api.number(item.cached_tokens) + ' · 写 ' + api.number(item.cache_write_tokens) + ' · 出 ' + api.number(item.completion_tokens);
    const tokenName = item.token_name || (item.token_id ? 'Key #' + item.token_id : '—');
    return '<tr style="--log-row-delay:' + Math.min(Number(index) || 0, 10) * 20 + 'ms">' +
      '<td><span class="usage-cell-primary">' + api.date(item.created_at) + '</span></td>' +
      '<td><span class="usage-cell-primary" title="' + api.escapeHTML(tokenName) + '">' + api.escapeHTML(tokenName) + '</span></td>' +
      '<td><span class="usage-cell-primary" title="' + api.escapeHTML(item.model || '—') + '">' + api.escapeHTML(item.model || '—') + '</span></td>' +
      '<td title="' + api.escapeHTML(tokenBreakdown) + '"><span class="usage-cell-primary">' + api.number(totalTokens) + '</span><span class="usage-cell-sub">' + api.escapeHTML(compactTokenBreakdown) + '</span></td>' +
      '<td><button type="button" class="usage-cost-button" data-usage-cost-id="' + Number(item.id || 0) + '" title="查看费用明细">' + quotaUSD(item.quota) + '</button></td>' +
      '<td><span class="usage-cell-primary">' + api.number(item.duration_ms || 0) + ' ms</span></td>' +
      '<td><span class="usage-status ' + (failed ? 'error' : 'success') + '">' + statusLabel + '</span>' + (item.error_code ? '<span class="usage-cell-sub" title="' + api.escapeHTML(item.error_code) + '">' + api.escapeHTML(item.error_code) + '</span>' : '') + '<button type="button" class="user-logs-detail-button" data-user-log-detail-id="' + Number(item.id || 0) + '" aria-label="查看 ' + api.escapeHTML(item.model || '未知模型') + ' 的请求详情">查看详情 →</button></td>' +
      '</tr>';
  }

  function preciseUSD(value) {
    const numeric = Number(value);
    const amount = Number.isFinite(numeric) ? numeric : 0;
    const fixed = amount.toFixed(8).replace(/0+$/, '');
    const parts = fixed.split('.');
    return '$' + parts[0] + '.' + (parts[1] || '').padEnd(2, '0');
  }

  function quotaUSD(value) {
    const quota = Number(value);
    return preciseUSD((Number.isFinite(quota) ? quota : 0) / api.quotaPerUSD());
  }

  function usageQuotaPerUSD(detail) {
    const unit = Number(detail && detail.quota_per_usd);
    return Number.isFinite(unit) && unit > 0 ? unit : api.quotaPerUSD();
  }

  function pricePerMillion(value) {
    return '$' + (Number(value) || 0).toFixed(6).replace(/0+$/, '').replace(/\.$/, '.00') + ' / 1M';
  }

  function costCard(icon, label, tokens, price) {
    const count = Math.max(0, Number(tokens) || 0);
    const unitPrice = Math.max(0, Number(price) || 0);
	return '<div><span class="material-symbol" aria-hidden="true">' + api.escapeHTML(icon) + '</span><small>' + api.escapeHTML(label) + '</small><b>' + api.number(count) + '</b><span class="usage-cost-token-rate">' + pricePerMillion(unitPrice) + '</span><strong>' + preciseUSD(count / 1000000 * unitPrice) + '</strong></div>';
  }

  function quotaCostLine(label, value, quotaPerUSD) {
    const quota = Math.max(0, Number(value) || 0);
    const unit = Number(quotaPerUSD);
    const divisor = Number.isFinite(unit) && unit > 0 ? unit : api.quotaPerUSD();
	return '<div class="usage-cost-factor"><span>' + api.escapeHTML(label) + '</span><b>' + preciseUSD(quota / divisor) + '</b><small>美元金额</small></div>';
  }

  function openUsageCostDialog(item) {
    const detail = item && item.billing_detail;
    const quota = Math.max(0, Number(item && item.quota) || 0);
    api.setText('[data-usage-cost-context]', (item.model || '未知模型') + ' · ' + api.date(item.created_at));
    api.setText('[data-usage-cost-total]', preciseUSD(quota / usageQuotaPerUSD(detail)));
    const body = document.querySelector('[data-usage-cost-breakdown]');
    if (!body) return;
    const requestContext = '<section class="user-logs-record-context" aria-label="请求信息"><div><small>API Key</small><b>' + api.escapeHTML(item.token_name || (item.token_id ? 'Key #' + item.token_id : '未记录')) + '</b></div><div><small>状态与耗时</small><b>HTTP ' + (Number(item.status_code) > 0 ? Number(item.status_code) : '未记录') + ' · ' + api.number(item.duration_ms || 0) + ' ms</b></div><div><small>请求编号</small><b>' + api.escapeHTML(item.request_id || '未记录请求编号') + '</b></div></section>';
    if (!detail || Number(detail.version) < 1 || Number(detail.version) > 2) {
      body.innerHTML = requestContext + '<section class="usage-cost-section"><div class="usage-cost-section-head"><span class="material-symbol" aria-hidden="true">data_usage</span><div><b>Token 用量</b><small>本次请求的用量构成</small></div></div><div class="usage-cost-token-grid"><div><span class="material-symbol" aria-hidden="true">edit</span><small>输入</small><b>' + api.number(item.prompt_tokens) + '</b></div><div><span class="material-symbol" aria-hidden="true">download</span><small>缓存读取</small><b>' + api.number(item.cached_tokens) + '</b></div><div><span class="material-symbol" aria-hidden="true">save</span><small>缓存写入</small><b>' + api.number(item.cache_write_tokens) + '</b></div><div><span class="material-symbol" aria-hidden="true">arrow_forward</span><small>输出</small><b>' + api.number(item.completion_tokens) + '</b></div></div></section><div class="usage-cost-notice"><span class="material-symbol" aria-hidden="true">info</span><div><b>仅展示实际扣费</b><p>这条历史记录创建时尚未保存价格和倍率快照，因此无法准确还原逐项金额。上方实际扣费是当时保存的最终美元金额。</p></div></div>';
      api.openDialog('usage-cost-dialog');
      return;
    }
    const prices = detail.base_prices || {};
	const fixedRequest = detail.pricing_mode === 'catalog_request';
	const remoteCompaction = detail.pricing_mode === 'remote_compaction';
	const fixedPrice = remoteCompaction ? Number(detail.compaction_cost_usd || detail.base_compaction_price || 0) : Number(detail.request_cost_usd || detail.base_request_price || 0);
	const basisLabel = remoteCompaction ? '远程压缩基础价' : (fixedRequest ? '模型按次基础价' : '四项基础合计');
	const basisUSD = fixedRequest || remoteCompaction ? fixedPrice : Number(detail.token_cost_usd || 0);
	const longContext = detail.long_context_applied ? '<div class="usage-cost-notice accent"><span class="material-symbol" aria-hidden="true">speed</span><div><b>已应用长上下文价格</b><p>本次输入超过 ' + api.number(detail.long_context_threshold) + ' Token，整次请求已切换到长上下文价格：输入/缓存倍率 ×' + Number(detail.long_context_input_multiplier || 1).toFixed(2) + '，输出倍率 ×' + Number(detail.long_context_output_multiplier || 1).toFixed(2) + '。</p></div></div>' : '';
	const quotaPerUSD = usageQuotaPerUSD(detail);
    const subscriptionMultiplier = Number(detail.subscription_multiplier) > 0 ? Number(detail.subscription_multiplier) : 1;
    const factors = '<div class="usage-cost-factors">' +
      '<div class="usage-cost-factor"><span>' + basisLabel + '</span><b>' + preciseUSD(basisUSD) + '</b><small>倍率前</small></div>' +
      '<div class="usage-cost-factor"><span>模型倍率</span><b>×' + Number(detail.model_multiplier || 1).toFixed(4) + '</b><small>模型目录</small></div>' +
      '<div class="usage-cost-factor"><span>账户费率</span><b>×' + Number(detail.group_multiplier || 1).toFixed(4) + '</b><small>计费系数</small></div>' +
      quotaCostLine('倍率后基础费用', detail.base_quota, quotaPerUSD) +
      '</div>';
    let subscription = '';
    if (subscriptionMultiplier !== 1 || Number(detail.discount_savings_quota) > 0 || detail.subscription_plan) {
	  subscription = '<section class="usage-cost-subscription-card"><div class="usage-cost-section-head"><span class="material-symbol" aria-hidden="true">redeem</span><div><b>订阅优惠</b><small>' + api.escapeHTML(detail.subscription_plan || '当前订阅计划') + '</small></div></div><div class="usage-cost-subscription"><div><span>订阅倍率</span><b>×' + subscriptionMultiplier.toFixed(4) + '</b></div><div><span>倍率结果</span><b>' + preciseUSD(Number(detail.discounted_quota || 0) / quotaPerUSD) + '</b></div><div><span>本次节省</span><b class="usage-cost-saving">-' + preciseUSD(Number(detail.discount_savings_quota || 0) / quotaPerUSD) + '</b></div></div></section>';
    }
	const appliedPromotions = Array.isArray(detail.promotions) ? detail.promotions : [];
	let promotion = '';
	if (appliedPromotions.length || Number(detail.promotion_savings_quota) > 0) {
	  const names = appliedPromotions.map(function (item) { return api.escapeHTML(item.name) + '（' + api.escapeHTML(userPromotionDiscountLabel(item.target)) + ' ×' + Number(item.multiplier).toFixed(2) + '）'; }).join('、');
	  promotion = '<section class="usage-cost-subscription-card"><div class="usage-cost-section-head"><span class="material-symbol" aria-hidden="true">redeem</span><div><b>计费活动优惠</b><small>' + (names || '已命中活动') + '</small></div></div><div class="usage-cost-subscription"><div><span>活动前费用</span><b>' + preciseUSD(Number(detail.pre_promotion_quota || 0) / quotaPerUSD) + '</b></div><div><span>活动节省</span><b class="usage-cost-saving">-' + preciseUSD(Number(detail.promotion_savings_quota || 0) / quotaPerUSD) + '</b></div><div><span>最终扣费</span><b>' + preciseUSD(Number(detail.final_quota || 0) / quotaPerUSD) + '</b></div></div></section>';
	}
	const cashbackRewards = Array.isArray(detail.cashback_rewards) ? detail.cashback_rewards : [];
	let cashback = '';
	if (cashbackRewards.length) {
	  cashback = '<div class="usage-cost-notice accent"><span class="material-symbol" aria-hidden="true">redeem</span><div><b>本次已触发活动返现</b><p>' + cashbackRewards.map(function (reward) { const timed = reward.timed_expires_at ? '，有效至 ' + api.date(reward.timed_expires_at) : '，永久有效'; return api.escapeHTML(reward.promotion_name) + '：+' + api.moneyFromQuota(reward.quota, 2) + ' 额度' + timed; }).join('；') + '</p></div></div>';
	}
	const usageBreakdown = fixedRequest || remoteCompaction ? '<div class="usage-cost-notice accent"><span class="material-symbol" aria-hidden="true">payments</span><div><b>' + (remoteCompaction ? '远程压缩按次计费' : '模型按次计费') + '</b><p>本次调用基础价格为 ' + preciseUSD(fixedPrice) + '；输入、输出和缓存 Token 不参与本次费用计算。</p></div></div>' : '<section class="usage-cost-section"><div class="usage-cost-section-head"><span class="material-symbol" aria-hidden="true">data_usage</span><div><b>Token 用量</b><small>用量、单价与分项费用</small></div></div><div class="usage-cost-token-grid">' +
	  costCard('edit', '普通输入', detail.ordinary_input_tokens, prices.input) +
	  costCard('download', '缓存读取', detail.cached_tokens, prices.cache_read) +
	  costCard('save', '缓存写入', detail.cache_write_tokens, prices.cache_write) +
	  costCard('arrow_forward', '输出', detail.completion_tokens, prices.output) +
	  '</div></section>';
	body.innerHTML = requestContext + usageBreakdown + longContext + '<section class="usage-cost-section"><div class="usage-cost-section-head"><span class="material-symbol" aria-hidden="true">tune</span><div><b>倍率与换算</b><small>从基础美元费用计算最终扣费</small></div></div>' + factors + '</section>' + subscription + promotion + cashback +
	  '<div class="usage-cost-formula"><span class="material-symbol" aria-hidden="true">info</span><div><b>计费规则</b><p>' + (fixedRequest || remoteCompaction ? '固定基础价乘模型倍率和账户费率，再按系统计费精度计算；随后应用订阅优惠及计费活动分项优惠，得到实际扣费。' : '四项基础费用相加后乘模型倍率和账户费率，再按系统计费精度计算；随后应用订阅优惠及计费活动分项优惠，得到实际扣费。') + '</p></div></div>';
    api.openDialog('usage-cost-dialog');
  }

  function syncLogSortHeaders() {
    document.querySelectorAll('[data-user-log-sort]').forEach(function (button) {
      const active = button.dataset.userLogSort === logState.sortBy;
      button.classList.toggle('active', active);
      button.dataset.direction = active ? logState.sortOrder : '';
      const header = button.closest('th');
      if (header) header.setAttribute('aria-sort', active ? (logState.sortOrder === 'asc' ? 'ascending' : 'descending') : 'none');
    });
  }

  function userLogQueryParams(state) {
    const params = new URLSearchParams({ page: String(state.page), page_size: String(state.pageSize) });
    if (state.keyword) params.set('keyword', state.keyword);
    if (state.statusCode && !state.exclude200) params.set('status_code', state.statusCode);
    if (state.exclude200) params.set('only_errors', 'true');
    if (state.startTime) params.set('start_timestamp', String(state.startTime));
    if (state.endTime) params.set('end_timestamp', String(state.endTime));
    params.set('sort_by', state.sortBy);
    params.set('sort_order', state.sortOrder);
    return params;
  }

  function userLogAppliedFilter(state) {
    const parts = [];
    if (state.keyword) parts.push('关键词：' + state.keyword);
    if (state.exclude200) parts.push('只看错误');
    else if (state.statusCode) parts.push('HTTP ' + state.statusCode);
    const format = function (seconds) { return localDateTimeInputValue(new Date(seconds * 1000)).replace('T', ' '); };
    if (state.startTime && state.endTime) parts.push(format(state.startTime) + ' — ' + format(state.endTime) + '（UTC+8）');
    else if (state.startTime) parts.push('从 ' + format(state.startTime) + ' 起（UTC+8）');
    else if (state.endTime) parts.push('截至 ' + format(state.endTime) + '（UTC+8）');
    else parts.push('全部历史记录');
    return parts.join(' · ');
  }

  function setUserLogLoading(loading) {
    const results = document.querySelector('.user-logs-results');
    if (results) { results.dataset.loading = String(loading); results.setAttribute('aria-busy', String(loading)); }
    const status = document.querySelector('[data-user-log-load-status]');
    if (status) { status.hidden = !loading; status.textContent = loading ? '正在更新当前条件下的调用记录…' : ''; }
    const submit = document.querySelector('[data-user-log-filter] [type="submit"]');
    if (submit) api.showBusy(submit, loading);
    const refresh = document.querySelector('[data-user-log-refresh]');
    if (refresh) refresh.disabled = loading;
    const prev = document.querySelector('[data-user-log-prev]');
    const next = document.querySelector('[data-user-log-next]');
    if (prev) prev.disabled = loading || logState.page <= 1;
    if (next) next.disabled = loading || logState.totalPages === 0 || logState.page >= logState.totalPages;
  }

  function loadLogs() {
    const snapshot = Object.assign({}, logState);
    const signature = userLogQueryParams(snapshot).toString();
    if (userLogLoadPending && userLogLoadPending.signature === signature) return userLogLoadPending.promise;
    const generation = ++userLogLoadGeneration;
    setUserLogLoading(true);
    const promise = Promise.resolve().then(function () { return api.request('/api/user/logs?' + signature); }).then(function (payload) {
      if (generation !== userLogLoadGeneration) return;
      const rows = Array.isArray(payload && payload.items) ? payload.items : [];
      const body = document.querySelector('[data-log-rows]');
      if (!body) return;
      body.innerHTML = rows.length ? rows.map(logRow).join('') : '<tr><td colspan="7"><div class="user-logs-empty"><span class="material-symbol" aria-hidden="true">manage_search</span><b>没有符合条件的调用</b><p>试试调整时间范围，或重置筛选。</p></div></td></tr>';
      const results = document.querySelector('.user-logs-results');
      if (results) results.dataset.empty = String(rows.length === 0);
      body.querySelectorAll('[data-usage-cost-id],[data-user-log-detail-id]').forEach(function (button) {
        button.addEventListener('click', function () {
          const id = Number(button.dataset.usageCostId || button.dataset.userLogDetailId);
          const item = rows.find(function (row) { return Number(row.id) === id; });
          if (item) openUsageCostDialog(item);
        });
      });
      const total = Math.max(0, Number(payload.total) || 0);
      logState.totalPages = Number(payload.total_pages) || Math.ceil(total / snapshot.pageSize);
      api.setText('[data-user-log-page]', '第 ' + snapshot.page + ' 页 / 共 ' + Math.max(1, logState.totalPages) + ' 页 · ' + api.number(total) + ' 条');
      api.setText('[data-user-log-count]', api.number(total) + ' 条记录');
      api.setText('[data-user-log-applied-filter]', userLogAppliedFilter(snapshot));
      const usage = payload && payload.summary || {};
      api.setText('[data-user-log-cost]', quotaUSD(usage.quota));
      api.setText('[data-user-log-requests]', api.number(Number(usage.requests) || 0));
      api.setText('[data-user-log-input]', api.number(Number(usage.prompt_tokens) || 0));
      api.setText('[data-user-log-cache-read]', api.number(Number(usage.cached_tokens) || 0));
      api.setText('[data-user-log-cache-write]', api.number(Number(usage.cache_write_tokens) || 0));
      api.setText('[data-user-log-output]', api.number(Number(usage.completion_tokens) || 0));
    }).catch(function (error) {
      if (generation === userLogLoadGeneration) throw error;
    }).finally(function () {
      if (generation !== userLogLoadGeneration) return;
      userLogLoadPending = null;
      setUserLogLoading(false);
    });
    userLogLoadPending = { signature: signature, promise: promise };
    return promise;
  }

  function localDateTimeTimestamp(value) {
    if (!value) return 0;
    const milliseconds = new Date(value + '+08:00').getTime();
    return Number.isFinite(milliseconds) ? Math.floor(milliseconds / 1000) : 0;
  }

  function localDateTimeInputValue(value) {
    return new Date(value.getTime() + 8 * 3600000).toISOString().slice(0, 19);
  }

  function userLogPeriodRange(period, now) {
    const start = new Date(now.getTime() + 8 * 3600000);
    start.setUTCHours(0, 0, 0, 0);
    const end = new Date(start.getTime());
    if (period === 'week') {
      const daysSinceMonday = (start.getUTCDay() + 6) % 7;
      start.setUTCDate(start.getUTCDate() - daysSinceMonday);
      end.setTime(start.getTime());
      end.setUTCDate(end.getUTCDate() + 7);
    } else if (period === 'month') {
      start.setUTCDate(1);
      end.setTime(start.getTime());
      end.setUTCMonth(end.getUTCMonth() + 1);
    } else {
      end.setUTCDate(end.getUTCDate() + 1);
    }
    return { start: new Date(start.getTime() - 8 * 3600000), end: new Date(end.getTime() - 8 * 3600000 - 1000) };
  }

  function bindLogFilters() {
    const form = document.querySelector('[data-user-log-filter]');
    if (!form) return;
    const feedback = form.querySelector('[data-user-log-filter-feedback]');
    const clearFeedback = function () {
      if (feedback) { feedback.hidden = true; feedback.textContent = ''; }
      form.querySelectorAll('[aria-invalid]').forEach(function (input) { input.removeAttribute('aria-invalid'); });
    };
    const filterError = function (input, message) {
      if (feedback) { feedback.hidden = false; feedback.textContent = message; }
      if (input) { input.setAttribute('aria-invalid', 'true'); input.focus(); }
      api.toast(message, 'danger');
    };
    const periodButtons = Array.from(document.querySelectorAll('[data-user-log-periods] [data-log-period]'));
    const setActivePeriod = function (period) {
      periodButtons.forEach(function (button) {
        const active = button.dataset.logPeriod === period;
        button.classList.toggle('active', active);
        button.setAttribute('aria-pressed', String(active));
      });
    };
    form.elements.page_size.value = String(logState.pageSize);
    const statusInput = form.elements.status_code;
    const exclude200Input = form.elements.exclude_200;
	exclude200Input.checked = logState.exclude200;
    const sortButtons = Array.from(document.querySelectorAll('[data-user-log-sort]'));
    const syncStatusFilter = function () {
      statusInput.disabled = exclude200Input.checked;
      if (exclude200Input.checked) statusInput.value = '';
    };
    exclude200Input.addEventListener('change', syncStatusFilter);
    syncStatusFilter();
    syncLogSortHeaders();
    sortButtons.forEach(function (button) {
      button.addEventListener('click', async function () {
        const sortBy = button.dataset.userLogSort;
        logState.sortOrder = logState.sortBy === sortBy && logState.sortOrder === 'desc' ? 'asc' : 'desc';
        logState.sortBy = sortBy;
        logState.page = 1;
        syncLogSortHeaders();
        try { await loadLogs(); } catch (error) { api.toast(error.message, 'danger'); }
      });
    });
    periodButtons.forEach(function (button) {
      button.addEventListener('click', async function () {
        const range = userLogPeriodRange(button.dataset.logPeriod, new Date());
        clearFeedback();
        form.elements.start_time.value = localDateTimeInputValue(range.start);
        form.elements.end_time.value = localDateTimeInputValue(range.end);
        logState.startTime = Math.floor(range.start.getTime() / 1000);
        logState.endTime = Math.floor(range.end.getTime() / 1000);
        logState.page = 1;
        setActivePeriod(button.dataset.logPeriod);
        try { await loadLogs(); } catch (error) { api.toast(error.message, 'danger'); }
      });
    });
    form.elements.start_time.addEventListener('input', function () { setActivePeriod(''); });
    form.elements.end_time.addEventListener('input', function () { setActivePeriod(''); });
    form.addEventListener('input', clearFeedback);
    form.addEventListener('change', clearFeedback);
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      clearFeedback();
      const keyword = form.elements.keyword.value.trim();
      const exclude200 = exclude200Input.checked;
      const statusCode = exclude200 ? '' : statusInput.value.trim();
      const startTime = localDateTimeTimestamp(form.elements.start_time.value);
      const endTime = localDateTimeTimestamp(form.elements.end_time.value);
      const pageSize = Number(form.elements.page_size.value) || 25;
      if (keyword.length > 200) { filterError(form.elements.keyword, '关键词最多输入 200 个字符。'); return; }
      if (statusCode && (!Number.isInteger(Number(statusCode)) || Number(statusCode) < 100 || Number(statusCode) > 599)) { filterError(statusInput, '状态码应为 100 到 599 之间的整数。'); return; }
      if (startTime < 0 || endTime < 0) { filterError(form.elements.start_time, '请使用 1970 年以后的时间。'); return; }
      if (startTime && endTime && startTime > endTime) { filterError(form.elements.end_time, '开始时间不能晚于结束时间。'); return; }
      if (![10, 25, 50, 100].includes(pageSize)) { filterError(form.elements.page_size, '每页数量请选择 10、25、50 或 100 条。'); return; }
      Object.assign(logState, { keyword: keyword, exclude200: exclude200, statusCode: statusCode, startTime: startTime, endTime: endTime, pageSize: pageSize, page: 1 });
      try { await loadLogs(); } catch (error) { api.toast(error.message, 'danger'); }
    });
    form.addEventListener('reset', function () {
      window.setTimeout(async function () {
		clearFeedback();
		const preferences = api.preferences();
		logState.page = 1; logState.pageSize = Number(preferences.pageSize) || 25; logState.keyword = ''; logState.statusCode = ''; logState.exclude200 = Boolean(preferences.hideSuccessfulLogs); logState.startTime = 0; logState.endTime = 0;
        logState.sortBy = 'created_at'; logState.sortOrder = 'desc';
        form.elements.page_size.value = String(logState.pageSize);
		form.elements.exclude_200.checked = logState.exclude200;
        setActivePeriod('');
        syncStatusFilter();
        syncLogSortHeaders();
        try { await loadLogs(); } catch (error) { api.toast(error.message, 'danger'); }
      });
    });
    document.querySelector('[data-user-log-prev]').addEventListener('click', async function () {
      if (logState.page <= 1) return;
      logState.page -= 1;
      try { await loadLogs(); } catch (error) { api.toast(error.message, 'danger'); }
    });
    document.querySelector('[data-user-log-next]').addEventListener('click', async function () {
      if (logState.totalPages === 0 || logState.page >= logState.totalPages) return;
      logState.page += 1;
      try { await loadLogs(); } catch (error) { api.toast(error.message, 'danger'); }
    });
    document.querySelector('[data-user-log-refresh]').addEventListener('click', async function () {
      try { await loadLogs(); } catch (error) { api.toast(error.message, 'danger'); }
    });
  }

  function bindCreateToken() {
    const form = document.querySelector('[data-create-token]');
    if (!form || form.dataset.createBound === 'true') return;
    form.dataset.createBound = 'true';
    const presetHint = form.querySelector('[data-token-preset-status]');
    form.querySelectorAll('[data-token-preset]').forEach(function (button) {
      button.addEventListener('click', function () {
        const preset = {
          local: { name: '本机测试', days: 7, ip: '127.0.0.1,::1' },
          application: { name: '应用接入', days: 30, ip: '' },
          temporary: { name: '临时调试', days: 1, ip: '' }
        }[button.dataset.tokenPreset];
        if (!preset || form.getAttribute('aria-busy') === 'true') return;
        const fields = form.elements;
        const changed = [];
        if (!fields.name.value.trim()) { fields.name.value = preset.name; changed.push('名称'); }
        if (!fields.expires_at.value) {
          fields.expires_at.value = tokenDateTimeLocal(new Date(Date.now() + preset.days * 86400000).toISOString());
          changed.push('有效期');
        }
        if (preset.ip && !fields.ip_limits.value.trim()) { fields.ip_limits.value = preset.ip; changed.push('IP 白名单'); }
        if (presetHint) presetHint.textContent = changed.length ? '已补齐' + changed.join('、') + '。请检查后' + (fields.id.value ? '保存' : '创建') + '。' : '这些配置已有值，可直接检查访问范围后保存。';
      });
    });
    form.addEventListener('reset', function () { if (presetHint) presetHint.textContent = '自动补齐空白名称、有效期与对应的 IP 范围，创建前可继续调整。'; });
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      const button = form.querySelector('button[type="submit"]');
	  if (form.getAttribute('aria-busy') === 'true') return;
	  const id = form.elements.id.value;
	  const name = form.elements.name.value.trim();
      if (!name) return api.toast('请输入 Key 名称', 'danger');
	  const expiryValue = form.elements.expires_at.value;
	  const body = {
		name: name,
		// Further keys follow the account quota. Keep this legacy sentinel in
		// the adapter payload while the independent quota controls stay hidden.
		remain_quota: -1,
		// The gateway applies the account quota to each key.
		unlimited_quota: true,
		model_limits: form.elements.model_limits.value.split(',').map(function (value) { return value.trim(); }).filter(Boolean),
		ip_limits: form.elements.ip_limits.value.trim(),
		expires_at: expiryValue ? new Date(expiryValue).toISOString() : ''
	  };
      form.setAttribute('aria-busy', 'true');
      api.showBusy(button, true);
      try {
		const token = id ? await api.request('/api/user/tokens/' + id, { method: 'PUT', body: body }) : await protectedUserRequest('/api/user/tokens', { method: 'POST', body: body }, userSecurity.key_required, { title: '验证后创建 Key', message: '创建后将显示新的完整 Key，请输入身份验证器中的 6 位验证码。', confirmText: '验证并创建' });
		if (!token) return;
        api.closeDialog('token-dialog');
        form.reset();
		syncModelPicker(form, []);
		if (!id) {
		  const reveal = document.querySelector('[data-created-token]');
		  if (reveal) reveal.textContent = token.key;
		  document.querySelector('#created-token-dialog .md-dialog-title').textContent = 'API Key 已创建';
		  api.openDialog('created-token-dialog');
		}
		api.toast(id ? 'API Key 已更新' : 'API Key 已创建');
        await loadTokens();
        await loadOverview();
      } catch (error) { api.toast(error.message, 'danger'); }
      finally { api.showBusy(button, false); form.removeAttribute('aria-busy'); }
    });
  }

  function bindCopy() {
    const button = document.querySelector('[data-copy-token]');
    if (!button) return;
    button.addEventListener('click', async function () {
      const value = document.querySelector('[data-created-token]').textContent;
      try { await navigator.clipboard.writeText(value); api.toast('已复制'); } catch (_) { api.toast('复制失败，请手动保存', 'danger'); }
    });
  }

  function bindRedeem() {
    const form = document.querySelector('[data-redeem]');
    if (!form) return;
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      const button = form.querySelector('button[type="submit"]');
	  const code = form.elements.code.value.trim();
	  if (!code) return api.toast('请输入兑换码', 'danger');
	  const resultBody = document.querySelector('[data-redeem-result]');
	  if (resultBody) { resultBody.textContent = ''; resultBody.classList.add('hidden'); }
      api.showBusy(button, true);
      try {
		await redeemCode(code, resultBody);
        form.reset();
      } catch (error) { api.toast(error.message, 'danger'); }
      finally { api.showBusy(button, false); }
    });
  }

  function bindBatchRedeem() {
    const form = document.querySelector('[data-redeem-batch]');
    if (!form) return;
    document.querySelectorAll('[data-open-dialog="batch-redeem-dialog"]').forEach(function (button) {
      button.addEventListener('click', function () {
        form.reset();
        const summary = document.querySelector('[data-redeem-batch-result]');
        if (summary) { summary.textContent = ''; summary.classList.add('hidden'); }
        const results = document.querySelector('[data-redeem-batch-results]');
        if (results) results.innerHTML = '';
      });
    });
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      const button = form.querySelector('button[type="submit"]');
	  const codes = form.elements.codes.value.split(/[\s,，;；]+/).map(function (code) { return code.trim(); }).filter(Boolean);
	  if (!codes.length) return api.toast('请至少输入一个兑换码', 'danger');
	  if (codes.length > 100) return api.toast('单次最多兑换 100 个', 'danger');
      api.showBusy(button, true);
      try {
		const result = await api.request('/api/user/redeem/batch', { method: 'POST', body: { codes: codes } });
		renderBatchRedeemResult(result);
		if (Number(result.succeeded) > 0) {
		  await Promise.all([loadOverview(), loadSubscriptions(), loadSubscriptionPlanCatalog(), loadTimedQuotas()]);
		}
		api.toast(result.message, Number(result.failed) > 0 ? 'danger' : undefined);
        form.elements.codes.value = '';
      } catch (error) { api.toast(error.message, 'danger'); }
      finally { api.showBusy(button, false); }
    });
  }

	function renderBatchRedeemResult(result) {
	  const summary = document.querySelector('[data-redeem-batch-result]');
	  if (summary) {
		summary.textContent = result.message || '批量兑换已完成';
		summary.classList.remove('hidden');
	  }
	  const body = document.querySelector('[data-redeem-batch-results]');
	  if (!body) return;
	  const items = Array.isArray(result.items) ? result.items : [];
	  body.innerHTML = items.map(function (item) {
		const code = item.code.length > 18 ? item.code.slice(0, 8) + '…' + item.code.slice(-6) : item.code;
		return '<div class="list-item"><span class="material-symbol" aria-hidden="true">' + (item.success ? 'check_circle' : 'error') + '</span><div class="list-item-body"><div class="list-item-title"><code>' + api.escapeHTML(code) + '</code></div><div class="list-item-sub">' + api.escapeHTML(item.message || item.error || '') + '</div></div><span class="tag ' + (item.success ? 'success' : 'error') + '">' + (item.success ? '成功' : '失败') + '</span></div>';
	  }).join('');
	}

	async function redeemCode(code, resultBody) {
	  const result = await api.request('/api/user/redeem', { method: 'POST', body: { code: code } });
	  let message;
	  if (result.redeem_type === 'subscription' && result.subscription) {
		const subscription = result.subscription;
		const subscriptions = Array.isArray(result.subscriptions) && result.subscriptions.length ? result.subscriptions : [subscription];
		const planNames = subscriptions.map(function (item) { return item.plan_name || item.plan_code; });
		const quota = subscription.unlimited_quota ? '不限额' : api.moneyFromQuota(subscription.quota_total, 2);
		message = subscriptions.length > 1 ? '直升兑换成功：' + planNames.join(' → ') + '，共 ' + subscriptions.length + ' 个计划已并发生效。' : '订阅兑换成功：' + planNames[0] + '，有效期至 ' + api.date(subscription.expires_at) + '，额度 ' + quota;
		api.toast(subscriptions.length > 1 ? '订阅直升成功' : '订阅兑换成功');
		await loadSubscriptions();
		await loadSubscriptionPlanCatalog();
	  } else if (result.redeem_type === 'prepaid') {
		message = '额度卡充值成功，增加 ' + api.moneyFromQuota(result.added_quota, 2) + '，本卡额度有效期至 ' + api.date(result.expires_at);
		api.toast('额度卡充值成功');
		await loadSubscriptions();
	  } else {
		message = '充值成功，增加 ' + api.moneyFromQuota(result.added_quota, 2) + ' 额度';
		api.toast(message);
	  }
	  if (resultBody) { resultBody.textContent = message; resultBody.classList.remove('hidden'); }
	  await loadOverview();
	  return result;
	}

	function bindPlanRedeem() {
	  const form = document.querySelector('[data-plan-redeem-form]');
	  if (!form) return;
	  form.addEventListener('submit', async function (event) {
		event.preventDefault();
		const button = form.querySelector('button[type="submit"]');
		const code = form.elements.code.value.trim();
		if (!code) return api.toast('请输入兑换码', 'danger');
		api.showBusy(button, true);
		try {
		  await redeemCode(code, document.querySelector('[data-plan-redeem-result]'));
		  form.reset();
		  api.closeDialog('plan-redeem-dialog');
		} catch (error) { api.toast(error.message, 'danger'); }
		finally { api.showBusy(button, false); }
	  });
	}

  function bindProfile() {
    const form = document.querySelector('[data-profile-form]');
    if (!form) return;
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      const button = form.querySelector('button[type="submit"]');
      api.showBusy(button, true);
      try {
        const result = await api.request('/api/user/profile', { method: 'PATCH', body: { username: form.elements.username.value.trim() } });
        api.setToken(result.token);
        form.elements.username.value = result.user.username;
        api.toast('用户名已更新');
      } catch (error) { api.toast(error.message, 'danger'); }
      finally { api.showBusy(button, false); }
    });
  }

  function populatePreferencesForm() {
    const form = document.querySelector('[data-preferences-form]');
    if (!form) return;
    const prefs = api.preferences();
    form.elements.theme.value = prefs.theme;
    form.elements.timezone.value = prefs.timezone;
    form.elements.default_page.value = prefs.defaultPage === 'plans' ? 'subscriptions' : prefs.defaultPage;
    form.elements.page_size.value = String(prefs.pageSize);
    form.elements.hide_locked_models.checked = Boolean(prefs.hideLockedModels);
	form.elements.sidebar_collapsed.checked = Boolean(prefs.sidebarCollapsed);
	form.elements.hour_cycle.value = prefs.hourCycle === '12' ? '12' : '24';
	form.elements.hide_successful_logs.checked = Boolean(prefs.hideSuccessfulLogs);
	form.elements.help_docs_visible.checked = userHelpDocsVisible;
	form.elements.help_docs_visible.disabled = !siteHelpDocsVisible;
	form.elements.prefer_timed_quota.checked = billingPriority === 'timed_first';
  }

  function bindPreferences() {
    const form = document.querySelector('[data-preferences-form]');
    if (!form) return;
    populatePreferencesForm();
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      const button = form.querySelector('button[type="submit"]');
      api.showBusy(button, true);
      try {
		await Promise.all([
		  api.request('/api/user/billing-preference', { method: 'PATCH', body: { prefer_timed_quota: form.elements.prefer_timed_quota.checked } }),
		  api.request('/api/user/ui-preferences', { method: 'PATCH', body: {
			help_docs_visible: form.elements.help_docs_visible.checked, theme: form.elements.theme.value,
			timezone: form.elements.timezone.value, default_page: form.elements.default_page.value,
			page_size: Number(form.elements.page_size.value) || 25, reduced_motion: false,
			hide_locked_models: form.elements.hide_locked_models.checked, sidebar_collapsed: form.elements.sidebar_collapsed.checked,
			hour_cycle: form.elements.hour_cycle.value, show_seconds: Boolean(api.preferences().showSeconds),
			hide_successful_logs: form.elements.hide_successful_logs.checked
		  } })
		]);
		billingPriority = form.elements.prefer_timed_quota.checked ? 'timed_first' : 'subscription_first';
		userHelpDocsVisible = form.elements.help_docs_visible.checked;
		applyHelpDocsVisibility();
		const next = api.savePreferences({
		  theme: form.elements.theme.value,
		  timezone: form.elements.timezone.value,
		  defaultPage: form.elements.default_page.value,
		  pageSize: Number(form.elements.page_size.value) || 25,
		  reducedMotion: false,
		  hideLockedModels: form.elements.hide_locked_models.checked,
		  sidebarCollapsed: form.elements.sidebar_collapsed.checked,
		  hourCycle: form.elements.hour_cycle.value,
		  showSeconds: Boolean(api.preferences().showSeconds),
		  hideSuccessfulLogs: form.elements.hide_successful_logs.checked
		});
		logState.page = 1;
		logState.pageSize = next.pageSize;
		const logFilter = document.querySelector('[data-user-log-filter]');
		if (logFilter) logFilter.elements.page_size.value = String(next.pageSize);
        await Promise.all([loadOverview(), loadLogs(), loadSubscriptions(), consoleModels.load()]);
        api.toast('使用偏好已保存');
      } catch (error) { api.toast(error.message, 'danger'); }
      finally { api.showBusy(button, false); }
    });
  }

  function bindAccountActions() {
    const logout = document.querySelector('[data-settings-logout]');
    if (logout) logout.addEventListener('click', async function () { await api.clearToken(); location.href = '/'; });

    document.querySelectorAll('[data-export-logs],[data-user-log-export]').forEach(function (exportButton) { exportButton.addEventListener('click', async function () {
      api.showBusy(exportButton, true);
      try {
        const response = await window.FurtherGateway.exportLogs();
        if (!response.ok) {
          const payload = await response.json().catch(function () { return null; });
          throw new Error(payload && (payload.error && payload.error.message || payload.message) || '导出调用记录失败');
        }
        const blob = await response.blob();
        const url = URL.createObjectURL(blob);
        const anchor = document.createElement('a');
        anchor.href = url;
        anchor.download = 'further-api-usage-' + new Date().toISOString().slice(0, 10).replace(/-/g, '') + '.csv';
        document.body.appendChild(anchor);
        anchor.click();
        anchor.remove();
        URL.revokeObjectURL(url);
        api.toast('调用记录已导出');
      } catch (error) { api.toast(error.message, 'danger'); }
      finally { api.showBusy(exportButton, false); }
    }); });

  }

  function bindDocs() {
    const baseURL = location.origin.replace(/\/$/, '') + '/v1';
    const snippets = {
      responses: [
        'curl ' + baseURL + '/responses \\',
        '  -H "Authorization: Bearer YOUR_API_KEY" \\',
        '  -H "Content-Type: application/json" \\',
        "  -d '{",
        '    "model": "YOUR_MODEL_NAME",',
        '    "input": "你好，请用一句话介绍自己。"',
        "  }'"
      ].join('\n'),
      python: [
        'import os', 'from openai import OpenAI', '',
        'client = OpenAI(',
        '    api_key=os.environ["FURTHER_API_KEY"],',
        '    base_url=' + JSON.stringify(baseURL), ')', '',
        'response = client.chat.completions.create(',
        '    model="YOUR_MODEL_NAME",',
        '    messages=[',
        '        {"role": "user", "content": "你好，请用一句话介绍自己。"}',
        '    ]', ')', '', 'print(response.choices[0].message.content)'
      ].join('\n'),
      node: [
        'import OpenAI from "openai";', '',
        'const client = new OpenAI({',
        '  apiKey: process.env.FURTHER_API_KEY,',
        '  baseURL: ' + JSON.stringify(baseURL), '});', '',
        'const response = await client.chat.completions.create({',
        '  model: "YOUR_MODEL_NAME",',
        '  messages: [',
        '    { role: "user", content: "你好，请用一句话介绍自己。" }',
        '  ]', '});', '', 'console.log(response.choices[0].message.content);'
      ].join('\n'),
      anthropic: [
        'curl ' + baseURL + '/messages \\',
        '  -H "x-api-key: YOUR_API_KEY" \\',
        '  -H "anthropic-version: 2023-06-01" \\',
        '  -H "Content-Type: application/json" \\',
        "  -d '{",
        '    "model": "YOUR_MODEL_NAME",',
        '    "max_tokens": 256,',
        '    "messages": [',
        '      {"role": "user", "content": "你好，请用一句话介绍自己。"}',
        '    ]', "  }'"
      ].join('\n')
    };
    document.querySelectorAll('[data-doc-base-url]').forEach(function (node) { node.textContent = baseURL; });
    document.querySelectorAll('[data-doc-snippet]').forEach(function (node) { node.textContent = snippets[node.dataset.docSnippet] || ''; });
    document.querySelectorAll('[data-doc-copy]').forEach(function (button) {
      button.addEventListener('click', async function () {
        const key = button.dataset.docCopy;
        const value = key === 'base' ? baseURL : snippets[key];
        if (!value) return;
        try {
          await navigator.clipboard.writeText(value);
          api.toast(key === 'base' ? '接口地址已复制' : '代码已复制，请替换示例中的模型与密钥');
          const label = button.querySelector('[data-doc-copy-label]');
          if (label) {
            label.textContent = '已复制';
            clearTimeout(button.copyResetTimer);
            button.copyResetTimer = setTimeout(function () { label.textContent = '复制代码'; }, 1800);
          }
        }
        catch (_) { api.toast('复制失败，请手动选择文本', 'danger'); }
      });
    });
    const tabs = Array.from(document.querySelectorAll('[data-doc-tab]'));
    function selectTab(button) {
      tabs.forEach(function (item) {
        const selected = item === button;
        item.classList.toggle('active', selected);
        item.setAttribute('aria-selected', String(selected));
        item.tabIndex = selected ? 0 : -1;
      });
      document.querySelectorAll('[data-doc-tab-panel]').forEach(function (panel) {
        const hidden = panel.dataset.docTabPanel !== button.dataset.docTab;
        panel.hidden = hidden;
        panel.classList.toggle('hidden', hidden);
      });
    }
    tabs.forEach(function (button, index) {
      button.addEventListener('click', function () { selectTab(button); });
      button.addEventListener('keydown', function (event) {
        let target;
        if (event.key === 'ArrowRight') target = tabs[(index + 1) % tabs.length];
        if (event.key === 'ArrowLeft') target = tabs[(index + tabs.length - 1) % tabs.length];
        if (event.key === 'Home') target = tabs[0];
        if (event.key === 'End') target = tabs[tabs.length - 1];
        if (target) { event.preventDefault(); selectTab(target); target.focus(); }
      });
    });
    document.querySelectorAll('[data-doc-anchor]').forEach(function (button) {
      button.addEventListener('click', function () {
        const section = document.getElementById(button.dataset.docAnchor);
        if (section) {
          section.focus({ preventScroll: true });
          section.scrollIntoView({ behavior: 'smooth', block: 'start' });
          document.querySelectorAll('.rf-docs-toc [data-doc-anchor]').forEach(function (item) {
            if (item.dataset.docAnchor === button.dataset.docAnchor) item.setAttribute('aria-current', 'location');
            else item.removeAttribute('aria-current');
          });
        }
      });
    });
  }















  document.addEventListener('further:ready', async function () {
    if (!requireLogin()) return;
    await Promise.allSettled([
      api.loadSiteAnnouncement().then(function (announcement) {
        siteHelpDocsVisible = !announcement || announcement.user_docs_visible !== false;
        applyHelpDocsVisibility();
      }),
      loadUserUIPreferences().catch(function (error) {
        applyHelpDocsVisibility(); api.toast('界面偏好加载失败：' + error.message, 'danger');
      })
    ]);
	api.mountShell();
	api.bindQuotaUSDFields(document);
	document.querySelectorAll('[data-create-token]').forEach(function (form) { modelPickerState(form); });
	document.querySelectorAll('[data-open-dialog="token-dialog"]').forEach(function (button) { button.addEventListener('click', resetTokenForm); });
    bindCreateToken();
    bindTokenPagination();
    bindCopy();
    bindRedeem();
	bindBatchRedeem();
	bindPlanRedeem();
    bindAbandonSubscription();
    bindProfile();
    bindAccountSecurityFeature();
    bindPreferences();
    bindAccountActions();
    bindDocs();

    bindTokenLoan();
    bindLogFilters();
    window.FurtherUIResources.mount(resources, resourcesForPage);
    // Shared account and payment navigation data load alongside the active page.
    resources.load(['overview']).catch(function (error) { api.toast(error.message, 'danger'); });
  });
}());
