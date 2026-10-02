(function () {
  'use strict';
  const api = window.ProxyLan;
  const consoleModels = window.ProxyLanConsoleModels;
  const initialPreferences = api.preferences();
  const logState = { page: 1, pageSize: Number(initialPreferences.pageSize) || 25, keyword: '', statusCode: '', exclude200: Boolean(initialPreferences.hideSuccessfulLogs), startTime: 0, endTime: 0, sortBy: 'created_at', sortOrder: 'desc', totalPages: 0 };
  let accountContext = { username: '', passwordEnabled: true };
  let billingPriority = 'subscription_first';
  let subscriptionRows = [];
  let userSecurity = { totp_enabled: false, pending: false, login_required: false, key_required: false, abandon_required: false };
  let paymentConfig = null;
	let siteHelpDocsVisible = true;
	let userHelpDocsVisible = true;
  const resources = window.ProxyLanResources.create({
    overview: loadOverview, security: loadUserSecurity, tokens: loadTokens,
    logs: loadLogs, subscriptions: loadSubscriptions, plans: loadSubscriptionPlanCatalog,
    timedQuotas: loadTimedQuotas, benefits: loadUserBenefits, promotions: loadUserBillingPromotions, models: consoleModels.load,
    paymentConfig: loadPaymentConfig,
    payments: async function () {
      await resources.ensure('paymentConfig');
      if (paymentConfig && paymentConfig.enabled && paymentConfig.is_pro_host) await loadPaymentOrders();
    }
  });

  function resourcesForPage(name) {
    const pages = {
      overview: ['overview', 'benefits', 'promotions'], tokens: ['tokens', 'security'], models: ['models', 'promotions'], docs: [],
      usage: ['logs'], redeem: ['overview'],
      subscriptions: ['subscriptions', 'plans', 'timedQuotas', 'security'],
      benefits: ['benefits', 'promotions', 'overview'], wallet: ['payments'],
      profile: ['overview', 'security']
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
		preferences = await api.request('/api/user/ui-preferences', { method: 'PATCH', body: {
		  help_docs_visible: preferences.help_docs_visible !== false,
		  theme: local.theme, timezone: local.timezone, default_page: localDefaultPage, page_size: local.pageSize,
		  reduced_motion: local.reducedMotion, hide_locked_models: local.hideLockedModels, sidebar_collapsed: local.sidebarCollapsed,
		  hour_cycle: local.hourCycle, show_seconds: local.showSeconds, hide_successful_logs: local.hideSuccessfulLogs
		} });
	  }
	  const saved = api.savePreferences({
		theme: preferences.theme, timezone: preferences.timezone, defaultPage: preferences.default_page,
		pageSize: preferences.page_size, reducedMotion: preferences.reduced_motion,
		hideLockedModels: preferences.hide_locked_models, sidebarCollapsed: preferences.sidebar_collapsed,
		hourCycle: preferences.hour_cycle, showSeconds: preferences.show_seconds,
		hideSuccessfulLogs: preferences.hide_successful_logs
	  });
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
    accountContext = { username: user.username || '', passwordEnabled: identity.password_change_available !== false };
    const profile = document.querySelector('[data-profile-form]');
    if (profile) profile.elements.username.value = user.username || '';
    api.setText('[data-account-id]', '#' + user.id);
    api.setText('[data-account-created]', api.date(user.created_at));
	const providerLabel = String(identity.provider || '').indexOf('linuxdo') === 0 ? 'LinuxDO' : (identity.provider === 'local' ? '本地账号' : identity.provider);
    api.setText('[data-account-provider]', providerLabel + (identity.provider_username ? ' · ' + identity.provider_username : ''));
    const passwordForm = document.querySelector('[data-password-form]');
    const oauthSecurity = document.querySelector('[data-oauth-security]');
    if (passwordForm) passwordForm.classList.toggle('hidden', !accountContext.passwordEnabled);
    if (oauthSecurity) oauthSecurity.classList.toggle('hidden', accountContext.passwordEnabled);
    api.setText('[data-oauth-provider]', providerLabel);
    const deactivatePasswordField = document.querySelector('[data-deactivate-password-field]');
    if (deactivatePasswordField) {
      deactivatePasswordField.classList.toggle('hidden', !accountContext.passwordEnabled);
      deactivatePasswordField.querySelector('input').required = accountContext.passwordEnabled;
    }
    const dangerZone = document.querySelector('.danger-zone');
    if (dangerZone) dangerZone.classList.toggle('hidden', Number(user.role) >= 25);
    api.setText('[data-user-quota]', api.moneyFromQuota(user.quota, 2));
    api.setText('[data-card-quota]', api.moneyFromQuota(user.quota, 2));
    const timedQuota = Math.max(0, Number(data.timed_quota) || 0);
    api.setText('[data-card-timed-quota]', api.moneyFromQuota(timedQuota, 2));
    const timedQuotaPart = document.querySelector('[data-card-timed-quota-wrap]');
    if (timedQuotaPart) timedQuotaPart.classList.toggle('hidden', timedQuota <= 0);
	const groupQuotas = Array.isArray(data.group_quotas) ? data.group_quotas : [];
	const groupQuotaCard = document.querySelector('[data-group-quota-card]');
	const groupQuotaRows = document.querySelector('[data-group-quota-rows]');
	if (groupQuotaCard) groupQuotaCard.classList.toggle('hidden', groupQuotas.length === 0);
	if (groupQuotaRows) groupQuotaRows.innerHTML = groupQuotas.map(function (item) { return '<div class="list-item"><span class="material-symbol" aria-hidden="true">account_balance_wallet</span><div class="list-item-body"><div class="list-item-title">' + api.escapeHTML(item.quota_group) + '</div><div class="list-item-sub">模型分组专属余额</div></div><div class="list-metrics"><div><b>' + api.moneyFromQuota(item.quota, 2) + '</b>剩余</div><div><b>' + api.moneyFromQuota(item.used_quota, 2) + '</b>已用</div></div></div>'; }).join('');
    api.setText('[data-card-used]', api.number(user.used_quota));
    api.setText('[data-card-requests]', api.number(data.requests));
    api.setText('[data-card-tokens]', api.number(data.token_count));
    api.setText('[data-today-requests]', api.number(data.today.requests));
    api.setText('[data-today-tokens]', api.number(data.today.input_tokens + data.today.output_tokens));
    api.setText('[data-today-quota]', api.moneyFromQuota(data.today.quota, 3));
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

  function renderUserSecurity() {
    const enabled = Boolean(userSecurity.totp_enabled);
    api.setText('[data-user-totp-status]', enabled ? '动态验证已启用' : '动态验证未启用');
    api.setText('[data-user-totp-status-hint]', enabled ? '可以分别控制登录、Key 和订阅计划的保护策略。' : (userSecurity.pending ? '存在尚未确认的验证器配置。' : '先配置验证器，随后即可启用各项保护。'));
    const setup = document.querySelector('[data-user-totp-setup]');
    if (setup) setup.textContent = enabled ? '更换验证器' : (userSecurity.pending ? '继续配置' : '配置验证器');
    const policy = document.querySelector('[data-user-totp-policy-form]');
    const disable = document.querySelector('[data-user-totp-disable-form]');
    if (policy) {
      policy.classList.toggle('hidden', !enabled);
      policy.elements.login_required.checked = Boolean(userSecurity.login_required);
      policy.elements.key_required.checked = Boolean(userSecurity.key_required);
      policy.elements.abandon_required.checked = Boolean(userSecurity.abandon_required);
      policy.elements.code.value = '';
    }
    if (disable) {
      disable.classList.toggle('hidden', !enabled);
      disable.elements.code.value = '';
    }
  }

  async function loadUserSecurity() {
    userSecurity = await api.request('/api/user/security/totp');
    renderUserSecurity();
    return userSecurity;
  }

  function bindUserSecurity() {
    const setup = document.querySelector('[data-user-totp-setup]');
    if (setup) setup.addEventListener('click', async function () {
      const currentCode = userSecurity.totp_enabled ? await api.requestTOTP({ title: '验证后更换验证器', message: '更换验证器前，请输入当前验证器中的 6 位验证码。', confirmText: '验证并继续' }) : '';
      if (userSecurity.totp_enabled && !currentCode) return;
      api.showBusy(setup, true);
      try {
        const result = await api.request('/api/user/security/totp/setup', { method: 'POST', body: { code: currentCode } });
        document.querySelector('[data-user-totp-secret]').value = result.secret || '';
        document.querySelector('[data-user-totp-uri]').value = result.otpauth_uri || '';
		const qrCode = document.querySelector('[data-user-totp-qr]');
		if (qrCode) {
		  const source = result.qr_code_data_uri || '';
		  if (source) qrCode.src = source; else qrCode.removeAttribute('src');
		  qrCode.classList.toggle('hidden', !source);
		}
        const enableForm = document.querySelector('[data-user-totp-enable-form]');
        if (enableForm) enableForm.reset();
        userSecurity.pending = true;
        renderUserSecurity();
        api.openDialog('user-totp-setup-dialog');
      } catch (error) { api.toast(error.message, 'danger'); }
      finally { api.showBusy(setup, false); }
    });

    const enableForm = document.querySelector('[data-user-totp-enable-form]');
    if (enableForm) enableForm.addEventListener('submit', async function (event) {
      event.preventDefault();
      const button = enableForm.querySelector('button[type="submit"]');
      api.showBusy(button, true);
      try {
        await api.request('/api/user/security/totp/enable', { method: 'POST', body: { code: enableForm.elements.code.value.trim() } });
        api.closeDialog('user-totp-setup-dialog');
        await loadUserSecurity();
        api.toast('动态验证已启用');
      } catch (error) { api.toast(error.message, 'danger'); }
      finally { api.showBusy(button, false); }
    });

    const policyForm = document.querySelector('[data-user-totp-policy-form]');
    if (policyForm) policyForm.addEventListener('submit', async function (event) {
      event.preventDefault();
      const button = policyForm.querySelector('button[type="submit"]');
      api.showBusy(button, true);
      try {
        await api.request('/api/user/security/totp/policy', { method: 'PATCH', body: {
          login_required: policyForm.elements.login_required.checked,
          key_required: policyForm.elements.key_required.checked,
          abandon_required: policyForm.elements.abandon_required.checked,
          code: policyForm.elements.code.value.trim()
        } });
        await loadUserSecurity();
        api.toast('动态验证策略已更新');
      } catch (error) { api.toast(error.message, 'danger'); }
      finally { api.showBusy(button, false); }
    });

    const disableForm = document.querySelector('[data-user-totp-disable-form]');
    if (disableForm) disableForm.addEventListener('submit', async function (event) {
      event.preventDefault();
      if (!(await api.confirm('关闭后，登录、Key 和计划放弃保护会同时失效。', { title: '关闭动态验证', confirmText: '确认关闭' }))) return;
      const button = disableForm.querySelector('button[type="submit"]');
      api.showBusy(button, true);
      try {
        await api.request('/api/user/security/totp/disable', { method: 'POST', body: { code: disableForm.elements.code.value.trim() } });
        await loadUserSecurity();
        api.toast('动态验证已关闭');
      } catch (error) { api.toast(error.message, 'danger'); }
      finally { api.showBusy(button, false); }
    });
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

  function subscriptionPurchasePriceCNY(plan) {
    if (!paymentConfig || !paymentConfig.enabled || !paymentConfig.is_pro_host || plan.unlimited_quota || Number(plan.quota) <= 0) return 0;
    const usd = Number(plan.quota) / 500000;
    const amount = usd * Number(paymentConfig.cny_per_usd || 0) * Number(paymentConfig.subscription_markup || 1.04);
    return Math.round((amount + Number.EPSILON) * 100) / 100;
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
    const purchasePrice = eligible ? subscriptionPurchasePriceCNY(plan) : 0;
    const redeemAction = eligible ? '<button type="button" class="md-btn md-btn-outlined md-btn-compact" data-plan-redeem="' + plan.id + '">使用兑换码</button>' : '<button type="button" class="md-btn md-btn-outlined md-btn-compact" disabled>暂不可升级</button>';
    const purchaseAction = purchasePrice > 0 ? '<button type="button" class="md-btn md-btn-filled md-btn-compact" data-plan-purchase="' + plan.id + '">支付宝购买 ¥' + purchasePrice.toFixed(2) + '</button>' : '';
    const priceHint = purchasePrice > 0 ? '<small class="plan-purchase-hint">$' + (Number(plan.quota) / 500000).toFixed(2) + ' × ¥' + Number(paymentConfig.cny_per_usd).toFixed(4) + ' × ' + Number(paymentConfig.subscription_markup || 1.04).toFixed(2) + '</small>' : '<span class="card-hint">' + rules.length + ' 个专属模型</span>';
	return '<article class="md-card plan-card' + (eligible ? '' : ' locked') + '"><div class="plan-card-head"><div><div class="plan-card-name">' + api.escapeHTML(plan.name) + '</div><div class="plan-card-code"><code>' + api.escapeHTML(plan.code) + '</code></div></div>' + (plan.active ? '<span class="tag success">已订阅</span>' : '<span class="tag muted">可兑换</span>') + '</div>' + description + '<div class="plan-card-metrics"><div class="plan-card-metric">有效期<b>' + Number(plan.duration_days) + ' 天</b></div><div class="plan-card-metric">额度<b>' + quota + '</b></div><div class="plan-card-metric">额度重置<b>' + reset + '</b></div><div class="plan-card-metric">折扣倍率<b>×' + Number(plan.discount_multiplier || 1).toFixed(2) + '</b></div><div class="plan-card-metric">最高可节省<b>' + savingsLimit + '</b></div><div class="plan-card-metric">访问上限<b>' + rpm + ' RPM / ' + concurrent + ' 并发</b></div></div>' + upgrade + '<div class="plan-card-models">' + models + '</div><div class="plan-card-actions">' + priceHint + '<div class="plan-card-action-buttons">' + redeemAction + purchaseAction + '</div></div></article>';
  }

  async function loadSubscriptionPlanCatalog() {
    const plans = await api.request('/api/user/subscription-plans');
    await resources.ensure('paymentConfig');
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
      container.querySelectorAll('[data-plan-purchase]').forEach(function (button) {
        button.addEventListener('click', async function () {
          const plan = rows.find(function (item) { return String(item.id) === String(button.dataset.planPurchase); });
          if (!plan) return;
          api.showBusy(button, true);
          try {
            const result = await api.request('/api/user/payments/orders', { method:'POST', body:{ subscription_plan_id:Number(plan.id), payment_type:'alipay' } });
            location.href = result.payment_url;
          } catch (error) {
            api.toast(error.message, 'danger');
            api.showBusy(button, false);
          }
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
    return '<div class="list-item"><span class="material-symbol" aria-hidden="true">data_usage</span><div class="list-item-body"><div class="list-item-title">' + api.escapeHTML(item.name || '通用限时额度') + '</div><div class="list-item-sub">有效期至 ' + api.date(item.expires_at) + ' · ' + (item.quota_group ? '额度组 ' + api.escapeHTML(item.quota_group) : '通用额度') + ' · 不包含订阅折扣与模型解锁</div></div><div class="list-metrics"><div><b>' + api.moneyFromQuota(item.quota_remaining, 2) + '</b>剩余</div><div><b>' + api.moneyFromQuota(item.quota_total, 2) + '</b>原始额度</div></div>' + timedQuotaStatus(item) + '</div>';
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
	if (item.quota_group) benefits.push('<div><b>' + api.escapeHTML(item.quota_group) + '</b>额度分组</div>');
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
      borrowForm.elements.quota.max = String(status.max_borrow_quota || 1);
      if (Number(borrowForm.elements.quota.value) > Number(status.max_borrow_quota)) borrowForm.elements.quota.value = String(status.max_borrow_quota);
      api.syncQuotaUSD(borrowForm);
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

  function userPromotionDiscountLabel(target) {
    return ({ all: '整单', input: '全部输入类', output: '全部输出类', ordinary_input: '普通输入', cache_read: '缓存读取', cache_write: '缓存写入', request: '按次请求费', compaction: '远程压缩费' })[target] || target;
  }

  function userBillingPromotionRow(item) {
    const details = [];
    details.push(item.scope_type === 'all_models' ? '全站模型' : (item.scope_type === 'quota_group' ? '额度组 ' : '模型 ') + item.scope_value);
    if (item.discount_target && item.discount_target !== 'none') details.push(userPromotionDiscountLabel(item.discount_target) + ' ×' + Number(item.discount_multiplier).toFixed(2));
    if (Number(item.cashback_threshold) > 0) {
      const rewardTarget = item.cashback_quota_group ? item.cashback_quota_group + ' 额度' : '通用余额';
      const validity = Number(item.cashback_validity_days) > 0 ? Number(item.cashback_validity_days) + ' 天限时' : '永久';
      details.push('每消费 ' + api.moneyFromQuota(item.cashback_threshold, 2) + ' 返 ' + api.moneyFromQuota(item.cashback_amount, 2) + ' 至' + rewardTarget + '（' + validity + '）');
    }
    const metrics = [];
    if (Number(item.cashback_threshold) > 0) {
      const completed = !item.cashback_repeatable && Number(item.reward_count) > 0;
      metrics.push('<div><b>' + (completed ? '已完成' : api.moneyFromQuota(item.spend_remainder, 2) + ' / ' + api.moneyFromQuota(item.cashback_threshold, 2)) + '</b>' + (completed ? '单次返现' : '本轮进度') + '</div>');
      metrics.push('<div><b>' + api.moneyFromQuota(item.total_reward, 2) + '</b>累计返现</div>');
    }
    const description = item.description ? '<div class="list-item-sub">' + api.escapeHTML(item.description) + '</div>' : '';
    return '<div class="md-card benefit-card"><div class="list-item"><span class="material-symbol" aria-hidden="true">redeem</span><div class="list-item-body"><div class="list-item-title">' + api.escapeHTML(item.name) + '</div><div class="list-item-sub">' + api.escapeHTML(details.join(' · ')) + '</div>' + description + '<div class="list-item-sub">' + api.date(item.starts_at) + ' 起' + (item.ends_at ? ' · ' + api.date(item.ends_at) + ' 结束' : ' · 长期有效') + '</div></div>' + (metrics.length ? '<div class="list-metrics">' + metrics.join('') + '</div>' : '') + '</div></div>';
  }

  function modelPromotionDiscountText(item) {
    if (!item.discount_target || item.discount_target === 'none') return '';
    const target = userPromotionDiscountLabel(item.discount_target);
    const multiplier = Number(item.discount_multiplier);
    if (multiplier === 0) return target + '免费';
    const discount = (multiplier * 10).toFixed(2).replace(/\.00$/, '').replace(/(\.\d)0$/, '$1');
    return target + ' ' + discount + ' 折';
  }

  function modelBillingPromotionCard(item) {
    const scopeLabel = item.scope_type === 'all_models' ? '全站活动' : (item.scope_type === 'quota_group' ? '额度组活动' : '模型活动');
    const scopeText = item.scope_type === 'all_models' ? '适用全站模型' : (item.scope_type === 'quota_group' ? '适用额度组 ' : '适用模型 ') + item.scope_value;
    const discountText = modelPromotionDiscountText(item);
    const hasCashback = Number(item.cashback_threshold) > 0;
    const rewardTarget = item.cashback_quota_group ? item.cashback_quota_group + ' 额度组' : '通用余额';
    const rewardValidity = Number(item.cashback_validity_days) > 0 ? Number(item.cashback_validity_days) + ' 天有效' : '永久有效';
    const cashbackText = hasCashback ? '消费 ' + api.moneyFromQuota(item.cashback_threshold, 2) + ' 返 ' + api.moneyFromQuota(item.cashback_amount, 2) : '';
    const completed = hasCashback && !item.cashback_repeatable && Number(item.reward_count) > 0;
    const progress = hasCashback ? (completed ? 100 : Math.min(100, Math.max(0, Number(item.spend_remainder) / Number(item.cashback_threshold) * 100))) : 0;
    const progressMarkup = hasCashback ? '<div class="model-promotion-progress"><div class="model-promotion-progress-head"><span>' + (completed ? '返现已完成' : '本轮返现进度') + '</span><b>' + (completed ? '100%' : progress.toFixed(0) + '%') + '</b></div><div class="model-promotion-progress-track" role="progressbar" aria-label="' + api.escapeHTML(item.name) + '返现进度" aria-valuemin="0" aria-valuemax="100" aria-valuenow="' + progress.toFixed(0) + '"><span style="width:' + progress.toFixed(2) + '%"></span></div><small>' + (completed ? '该活动为单次返现，你已领取 ' + api.moneyFromQuota(item.total_reward, 2) : '已累计 ' + api.moneyFromQuota(item.spend_remainder, 2) + ' / ' + api.moneyFromQuota(item.cashback_threshold, 2)) + '</small></div>' : '';
    const discountMarkup = discountText ? '<div class="model-promotion-offer"><span class="material-symbol" aria-hidden="true">percent</span><div><small>价格优惠</small><b>' + api.escapeHTML(discountText) + '</b></div></div>' : '';
    const cashbackMarkup = hasCashback ? '<div class="model-promotion-offer"><span class="material-symbol" aria-hidden="true">savings</span><div><small>消费返现</small><b>' + api.escapeHTML(cashbackText) + '</b><em>发至' + api.escapeHTML(rewardTarget) + ' · ' + api.escapeHTML(rewardValidity) + '</em></div></div>' : '';
    const description = item.description ? '<p>' + api.escapeHTML(item.description) + '</p>' : '';
    const expiry = item.ends_at ? api.date(item.ends_at) + ' 结束' : '长期有效';
    return '<article class="model-promotion-card"><div class="model-promotion-card-top"><span class="model-promotion-scope">' + scopeLabel + '</span><span class="model-promotion-expiry"><span class="material-symbol" aria-hidden="true">schedule</span>' + expiry + '</span></div><h3>' + api.escapeHTML(item.name) + '</h3>' + description + '<div class="model-promotion-offers">' + discountMarkup + cashbackMarkup + '</div>' + progressMarkup + '<div class="model-promotion-card-foot"><span class="material-symbol" aria-hidden="true">target</span><span>' + api.escapeHTML(scopeText) + '</span></div></article>';
  }

  async function loadUserBillingPromotions() {
    const result = await api.request('/api/user/billing-promotions');
    const rows = Array.isArray(result) ? result : [];
    const body = document.querySelector('[data-user-billing-promotions]');
    if (body) body.innerHTML = rows.length ? rows.map(userBillingPromotionRow).join('') : '<div class="md-card empty">当前没有进行中的计费活动。</div>';
    const modelSection = document.querySelector('[data-model-billing-promotions-section]');
    const modelBody = document.querySelector('[data-model-billing-promotions]');
    if (modelSection) modelSection.classList.toggle('hidden', rows.length === 0);
    if (modelBody) modelBody.innerHTML = rows.map(modelBillingPromotionCard).join('');
    api.setText('[data-model-billing-promotion-count]', rows.length + ' 个活动');
    return rows;
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
        const roundedQuota = api.roundQuotaToUSDCent(form.elements.quota.value);
        form.elements.quota.value = String(roundedQuota);
        api.syncQuotaUSD(form);
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
	const restrictions = modelLimits + (token.ip_limits ? ' · IP ' + token.ip_limits : '') + (token.expired_at ? ' · 到期 ' + api.date(token.expired_at) : '');
	const encoded = encodeURIComponent(JSON.stringify(token));
	const enabled = Number(token.status) === 1;
    return '<div class="list-item compact-row">' +
      '<span class="material-symbol" aria-hidden="true">key</span>' +
	  '<div class="list-item-body"><div class="list-item-title">' + api.escapeHTML(token.name) + '</div><div class="list-item-sub"><code>' + api.escapeHTML(token.key) + '</code> · ' + api.escapeHTML(restrictions) + '</div></div>' +
      '<div class="list-metrics"><div><b>' + (token.unlimited_quota ? '不限额' : Number(token.remain_quota) < 0 ? '跟随账户' : api.moneyFromQuota(token.remain_quota, 2)) + '</b>剩余</div><div><b>' + api.number(token.used_quota) + '</b>已用</div></div>' +
	  '<div class="list-actions"><span>' + api.statusLabel(token.status, 'token') + '</span><button type="button" class="md-icon-button" title="编辑 API Key" data-edit-token="' + encoded + '"><span class="material-symbol" aria-hidden="true">edit</span></button><button type="button" class="md-icon-button" title="轮换 API Key" aria-label="轮换 API Key" data-rotate-token="' + token.id + '"><span class="material-symbol" aria-hidden="true">refresh</span></button><button type="button" class="md-icon-button ' + (enabled ? 'danger' : '') + '" title="' + (enabled ? '禁用' : '启用') + ' API Key" data-toggle-token="' + token.id + '" data-next-status="' + (enabled ? 2 : 1) + '"><span class="material-symbol" aria-hidden="true">' + (enabled ? 'block' : 'check_circle') + '</span></button><button type="button" class="md-icon-button danger" title="删除 API Key" data-delete-token="' + token.id + '"><span class="material-symbol" aria-hidden="true">delete</span></button></div>' +
      '</div>';
  }

	function tokenDateTimeLocal(value) {
	  if (!value) return '';
	  const date = new Date(value);
	  if (Number.isNaN(date.getTime())) return '';
	  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60000);
	  return local.toISOString().slice(0, 16);
	}

	function resetTokenForm() {
	  const form = document.querySelector('[data-create-token]');
	  if (!form) return;
	  form.reset();
	  form.elements.id.value = '';
	  form.elements.remain_quota.value = '-1';
	  api.syncQuotaUSD(form);
	  api.setText('[data-token-dialog-title]', '新建 API Key');
	  api.setText('[data-token-submit]', '创建');
	}

	function editTokenForm(token) {
	  const form = document.querySelector('[data-create-token]');
	  if (!form) return;
	  form.elements.id.value = token.id;
	  form.elements.name.value = token.name || '';
	  form.elements.remain_quota.value = Number(token.remain_quota);
	  api.syncQuotaUSD(form);
	  form.elements.unlimited_quota.checked = Boolean(token.unlimited_quota);
	  form.elements.model_limits.value = Array.isArray(token.model_limits) ? token.model_limits.join(', ') : '';
	  form.elements.ip_limits.value = token.ip_limits || '';
	  form.elements.expires_at.value = tokenDateTimeLocal(token.expired_at);
	  api.setText('[data-token-dialog-title]', '编辑 API Key');
	  api.setText('[data-token-submit]', '保存');
	  api.openDialog('token-dialog');
	}

  async function loadTokens() {
    const result = await api.request('/api/user/tokens');
    const rows = Array.isArray(result) ? result : [];
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
  }


  function logRow(item) {
    const failed = Number(item.status_code) >= 400;
    const totalTokens = Number(item.prompt_tokens || 0) + Number(item.completion_tokens || 0);
    const tokenBreakdown = '输入 ' + api.number(item.prompt_tokens) + ' · 缓存读 ' + api.number(item.cached_tokens) + ' · 缓存写 ' + api.number(item.cache_write_tokens) + ' · 输出 ' + api.number(item.completion_tokens);
    const compactTokenBreakdown = '入 ' + api.number(item.prompt_tokens) + ' · 读 ' + api.number(item.cached_tokens) + ' · 写 ' + api.number(item.cache_write_tokens) + ' · 出 ' + api.number(item.completion_tokens);
    const tokenName = item.token_name || (item.token_id ? 'Key #' + item.token_id : '—');
    return '<tr>' +
      '<td><span class="usage-cell-primary">' + api.date(item.created_at) + '</span></td>' +
      '<td><span class="usage-cell-primary" title="' + api.escapeHTML(tokenName) + '">' + api.escapeHTML(tokenName) + '</span></td>' +
      '<td><span class="usage-cell-primary" title="' + api.escapeHTML(item.model || '—') + '">' + api.escapeHTML(item.model || '—') + '</span></td>' +
      '<td title="' + api.escapeHTML(tokenBreakdown) + '"><span class="usage-cell-primary">' + api.number(totalTokens) + '</span><span class="usage-cell-sub">' + api.escapeHTML(compactTokenBreakdown) + '</span></td>' +
      '<td><button type="button" class="usage-cost-button" data-usage-cost-id="' + Number(item.id || 0) + '" title="查看费用明细">' + api.moneyFromQuota(item.quota, 4) + '</button></td>' +
      '<td><span class="usage-cell-primary">' + api.number(item.duration_ms || 0) + ' ms</span></td>' +
      '<td><span class="usage-status ' + (failed ? 'error' : 'success') + '">' + (item.status_code || 200) + '</span>' + (item.error_code ? '<span class="usage-cell-sub" title="' + api.escapeHTML(item.error_code) + '">' + api.escapeHTML(item.error_code) + '</span>' : '') + '</td>' +
      '</tr>';
  }

  function preciseUSD(value) {
    const amount = Number(value) || 0;
    return '$' + amount.toFixed(8).replace(/0+$/, '').replace(/\.$/, '.00');
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
    const divisor = Math.max(1, Number(quotaPerUSD) || 500000);
	return '<div class="usage-cost-factor"><span>' + api.escapeHTML(label) + '</span><b>' + api.number(quota) + ' Quota</b><small>' + preciseUSD(quota / divisor) + '</small></div>';
  }

  function openUsageCostDialog(item) {
    const detail = item && item.billing_detail;
    const quota = Math.max(0, Number(item && item.quota) || 0);
    api.setText('[data-usage-cost-context]', (item.model || '未知模型') + ' · ' + api.date(item.created_at));
    api.setText('[data-usage-cost-total]', api.moneyFromQuota(quota, 6));
    api.setText('[data-usage-cost-quota]', api.number(quota) + ' Quota');
    const body = document.querySelector('[data-usage-cost-breakdown]');
    if (!body) return;
    if (!detail || Number(detail.version) < 1 || Number(detail.version) > 2) {
	  body.innerHTML = '<section class="usage-cost-section"><div class="usage-cost-section-head"><span class="material-symbol" aria-hidden="true">data_usage</span><div><b>Token 用量</b><small>本次请求的用量构成</small></div></div><div class="usage-cost-token-grid"><div><span class="material-symbol" aria-hidden="true">edit</span><small>输入</small><b>' + api.number(item.prompt_tokens) + '</b></div><div><span class="material-symbol" aria-hidden="true">download</span><small>缓存读取</small><b>' + api.number(item.cached_tokens) + '</b></div><div><span class="material-symbol" aria-hidden="true">save</span><small>缓存写入</small><b>' + api.number(item.cache_write_tokens) + '</b></div><div><span class="material-symbol" aria-hidden="true">arrow_forward</span><small>输出</small><b>' + api.number(item.completion_tokens) + '</b></div></div></section><div class="usage-cost-notice"><span class="material-symbol" aria-hidden="true">info</span><div><b>仅展示实际扣费</b><p>这条历史记录创建时尚未保存价格和倍率快照，因此无法准确还原逐项金额。上方实际扣费来自当时已经落库的最终 Quota。</p></div></div>';
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
    const quotaPerUSD = Math.max(1, Number(detail.quota_per_usd) || 500000);
    const subscriptionMultiplier = Number(detail.subscription_multiplier) > 0 ? Number(detail.subscription_multiplier) : 1;
    const factors = '<div class="usage-cost-factors">' +
      '<div class="usage-cost-factor"><span>' + basisLabel + '</span><b>' + preciseUSD(basisUSD) + '</b><small>倍率前</small></div>' +
      '<div class="usage-cost-factor"><span>模型倍率</span><b>×' + Number(detail.model_multiplier || 1).toFixed(4) + '</b><small>模型目录</small></div>' +
      '<div class="usage-cost-factor"><span>用户组倍率</span><b>×' + Number(detail.group_multiplier || 1).toFixed(4) + '</b><small>账户分组</small></div>' +
      quotaCostLine('倍率后基础费用', detail.base_quota, quotaPerUSD) +
      '</div>';
    let subscription = '';
    if (subscriptionMultiplier !== 1 || Number(detail.discount_savings_quota) > 0 || detail.subscription_plan) {
	  subscription = '<section class="usage-cost-subscription-card"><div class="usage-cost-section-head"><span class="material-symbol" aria-hidden="true">redeem</span><div><b>订阅优惠</b><small>' + api.escapeHTML(detail.subscription_plan || '当前订阅计划') + '</small></div></div><div class="usage-cost-subscription"><div><span>订阅倍率</span><b>×' + subscriptionMultiplier.toFixed(4) + '</b></div><div><span>倍率结果</span><b>' + api.number(detail.discounted_quota) + ' Quota</b></div><div><span>本次节省</span><b class="usage-cost-saving">-' + api.number(detail.discount_savings_quota) + ' Quota</b></div></div></section>';
    }
	const appliedPromotions = Array.isArray(detail.promotions) ? detail.promotions : [];
	let promotion = '';
	if (appliedPromotions.length || Number(detail.promotion_savings_quota) > 0) {
	  const names = appliedPromotions.map(function (item) { return api.escapeHTML(item.name) + '（' + api.escapeHTML(userPromotionDiscountLabel(item.target)) + ' ×' + Number(item.multiplier).toFixed(2) + '）'; }).join('、');
	  promotion = '<section class="usage-cost-subscription-card"><div class="usage-cost-section-head"><span class="material-symbol" aria-hidden="true">redeem</span><div><b>计费活动优惠</b><small>' + (names || '已命中活动') + '</small></div></div><div class="usage-cost-subscription"><div><span>活动前费用</span><b>' + api.number(detail.pre_promotion_quota) + ' Quota</b></div><div><span>活动节省</span><b class="usage-cost-saving">-' + api.number(detail.promotion_savings_quota) + ' Quota</b></div><div><span>最终扣费</span><b>' + api.number(detail.final_quota) + ' Quota</b></div></div></section>';
	}
	const cashbackRewards = Array.isArray(detail.cashback_rewards) ? detail.cashback_rewards : [];
	let cashback = '';
	if (cashbackRewards.length) {
	  cashback = '<div class="usage-cost-notice accent"><span class="material-symbol" aria-hidden="true">redeem</span><div><b>本次已触发活动返现</b><p>' + cashbackRewards.map(function (reward) { const destination = reward.quota_group ? reward.quota_group + ' 额度组' : '通用余额'; const timed = reward.timed_expires_at ? '，有效至 ' + api.date(reward.timed_expires_at) : '，永久有效'; return api.escapeHTML(reward.promotion_name) + '：+' + api.moneyFromQuota(reward.quota, 2) + ' 到 ' + api.escapeHTML(destination) + timed; }).join('；') + '</p></div></div>';
	}
	const usageBreakdown = fixedRequest || remoteCompaction ? '<div class="usage-cost-notice accent"><span class="material-symbol" aria-hidden="true">payments</span><div><b>' + (remoteCompaction ? '远程压缩按次计费' : '模型按次计费') + '</b><p>本次调用基础价格为 ' + preciseUSD(fixedPrice) + '；输入、输出和缓存 Token 不参与本次费用计算。</p></div></div>' : '<section class="usage-cost-section"><div class="usage-cost-section-head"><span class="material-symbol" aria-hidden="true">data_usage</span><div><b>Token 用量</b><small>用量、单价与分项费用</small></div></div><div class="usage-cost-token-grid">' +
	  costCard('edit', '普通输入', detail.ordinary_input_tokens, prices.input) +
	  costCard('download', '缓存读取', detail.cached_tokens, prices.cache_read) +
	  costCard('save', '缓存写入', detail.cache_write_tokens, prices.cache_write) +
	  costCard('arrow_forward', '输出', detail.completion_tokens, prices.output) +
	  '</div></section>';
	body.innerHTML = usageBreakdown + longContext + '<section class="usage-cost-section"><div class="usage-cost-section-head"><span class="material-symbol" aria-hidden="true">tune</span><div><b>倍率与换算</b><small>从基础美元费用换算为 Quota</small></div></div>' + factors + '</section>' + subscription + promotion + cashback +
	  '<div class="usage-cost-formula"><span class="material-symbol" aria-hidden="true">info</span><div><b>计费规则</b><p>' + (fixedRequest || remoteCompaction ? '固定基础价乘模型倍率和用户组倍率，换算为 Quota 并向上取整；随后应用订阅优惠及计费活动分项优惠，得到实际扣费。' : '四项基础费用相加后乘模型倍率和用户组倍率，换算为 Quota 并向上取整；随后应用订阅优惠及计费活动分项优惠，得到实际扣费。') + '</p></div></div>';
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

  async function loadLogs() {
    const params = new URLSearchParams({ page: String(logState.page), page_size: String(logState.pageSize) });
    if (logState.keyword) params.set('keyword', logState.keyword);
    if (logState.statusCode) params.set('status_code', logState.statusCode);
    if (logState.exclude200) params.set('exclude_status_code', '200');
    if (logState.startTime) params.set('start_timestamp', String(logState.startTime));
    if (logState.endTime) params.set('end_timestamp', String(logState.endTime));
    params.set('sort_by', logState.sortBy);
    params.set('sort_order', logState.sortOrder);
    const payload = await api.request('/api/user/logs?' + params.toString());
    const rows = Array.isArray(payload && payload.items) ? payload.items : [];
    const body = document.querySelector('[data-log-rows]');
    if (!body) return;
    body.innerHTML = rows.length ? rows.map(logRow).join('') : '<tr><td class="empty" colspan="7">还没有调用记录。</td></tr>';
    body.querySelectorAll('[data-usage-cost-id]').forEach(function (button) {
      button.addEventListener('click', function () {
        const item = rows.find(function (row) { return Number(row.id) === Number(button.dataset.usageCostId); });
        if (item) openUsageCostDialog(item);
      });
    });
    logState.totalPages = Number(payload.total_pages) || 0;
    const summary = document.querySelector('[data-user-log-page]');
    if (summary) summary.textContent = '第 ' + logState.page + ' 页 / 共 ' + logState.totalPages + ' 页 · ' + api.number(payload.total || 0) + ' 条';
    const prev = document.querySelector('[data-user-log-prev]');
    const next = document.querySelector('[data-user-log-next]');
    if (prev) prev.disabled = logState.page <= 1;
    if (next) next.disabled = logState.totalPages === 0 || logState.page >= logState.totalPages;
    const usage = payload && payload.summary || {};
    api.setText('[data-user-log-cost]', api.moneyFromQuota(Number(usage.quota) || 0, 4));
    api.setText('[data-user-log-requests]', api.number(Number(usage.requests) || 0));
    api.setText('[data-user-log-input]', api.number(Number(usage.prompt_tokens) || 0));
    api.setText('[data-user-log-cache-read]', api.number(Number(usage.cached_tokens) || 0));
    api.setText('[data-user-log-cache-write]', api.number(Number(usage.cache_write_tokens) || 0));
    api.setText('[data-user-log-output]', api.number(Number(usage.completion_tokens) || 0));
  }

  function localDateTimeTimestamp(value) {
    if (!value) return 0;
    const milliseconds = new Date(value).getTime();
    return Number.isFinite(milliseconds) ? Math.floor(milliseconds / 1000) : 0;
  }

  function localDateTimeInputValue(value) {
    const pad = function (number) { return String(number).padStart(2, '0'); };
    return value.getFullYear() + '-' + pad(value.getMonth() + 1) + '-' + pad(value.getDate()) + 'T' +
      pad(value.getHours()) + ':' + pad(value.getMinutes()) + ':' + pad(value.getSeconds());
  }

  function userLogPeriodRange(period, now) {
    const start = new Date(now.getTime());
    start.setHours(0, 0, 0, 0);
    const end = new Date(start.getTime());
    if (period === 'week') {
      const daysSinceMonday = (start.getDay() + 6) % 7;
      start.setDate(start.getDate() - daysSinceMonday);
      end.setTime(start.getTime());
      end.setDate(end.getDate() + 7);
    } else if (period === 'month') {
      start.setDate(1);
      end.setTime(start.getTime());
      end.setMonth(end.getMonth() + 1);
    } else {
      end.setDate(end.getDate() + 1);
    }
    end.setSeconds(end.getSeconds() - 1);
    return { start: start, end: end };
  }

  function bindLogFilters() {
    const form = document.querySelector('[data-user-log-filter]');
    if (!form) return;
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
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      logState.keyword = form.elements.keyword.value.trim();
      logState.exclude200 = exclude200Input.checked;
      logState.statusCode = logState.exclude200 ? '' : statusInput.value.trim();
      logState.startTime = localDateTimeTimestamp(form.elements.start_time.value);
      logState.endTime = localDateTimeTimestamp(form.elements.end_time.value);
      if (logState.startTime && logState.endTime && logState.startTime > logState.endTime) {
        api.toast('开始时间不能晚于结束时间', 'danger');
        return;
      }
      logState.pageSize = Number(form.elements.page_size.value) || 25;
      logState.page = 1;
      try { await loadLogs(); } catch (error) { api.toast(error.message, 'danger'); }
    });
    form.addEventListener('reset', function () {
      window.setTimeout(async function () {
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
    if (!form) return;
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      const button = form.querySelector('button[type="submit"]');
	  const id = form.elements.id.value;
	  const name = form.elements.name.value.trim();
      if (!name) return api.toast('请输入 Key 名称', 'danger');
	  const expiryValue = form.elements.expires_at.value;
	  const body = {
		name: name,
		remain_quota: Number(form.elements.remain_quota.value || -1),
		unlimited_quota: form.elements.unlimited_quota.checked,
		model_limits: form.elements.model_limits.value.split(',').map(function (value) { return value.trim(); }).filter(Boolean),
		ip_limits: form.elements.ip_limits.value.trim(),
		expires_at: expiryValue ? new Date(expiryValue).toISOString() : ''
	  };
      api.showBusy(button, true);
      try {
		const token = id ? await api.request('/api/user/tokens/' + id, { method: 'PUT', body: body }) : await protectedUserRequest('/api/user/tokens', { method: 'POST', body: body }, userSecurity.key_required, { title: '验证后创建 Key', message: '创建后将显示新的完整 Key，请输入身份验证器中的 6 位验证码。', confirmText: '验证并创建' });
		if (!token) return;
        api.closeDialog('token-dialog');
        form.reset();
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
      finally { api.showBusy(button, false); }
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
		message = '预付卡充值成功，增加 ' + api.moneyFromQuota(result.added_quota, 2) + '，本卡额度有效期至 ' + api.date(result.expires_at);
		api.toast('预付卡充值成功');
		await loadSubscriptions();
	  } else {
		message = '充值成功，增加 ' + api.moneyFromQuota(result.added_quota, 2) + (result.quota_group ? '（额度组 ' + result.quota_group + '）' : '（通用额度）');
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
        accountContext.username = result.user.username;
        api.toast('用户名已更新');
      } catch (error) { api.toast(error.message, 'danger'); }
      finally { api.showBusy(button, false); }
    });
  }

  function bindPassword() {
    const form = document.querySelector('[data-password-form]');
    if (!form) return;
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      if (form.elements.new_password.value !== form.elements.confirm_password.value) return api.toast('两次输入的新密码不一致', 'danger');
      const button = form.querySelector('button[type="submit"]');
      api.showBusy(button, true);
      try {
        await api.request('/api/user/password', { method: 'PATCH', body: { current_password: form.elements.current_password.value, new_password: form.elements.new_password.value } });
        form.reset();
        api.toast('密码已更新');
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
    form.elements.reduced_motion.checked = Boolean(prefs.reducedMotion);
    form.elements.hide_locked_models.checked = Boolean(prefs.hideLockedModels);
	form.elements.sidebar_collapsed.checked = Boolean(prefs.sidebarCollapsed);
	form.elements.hour_cycle.value = prefs.hourCycle === '12' ? '12' : '24';
	form.elements.show_seconds.checked = Boolean(prefs.showSeconds);
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
			page_size: Number(form.elements.page_size.value) || 25, reduced_motion: form.elements.reduced_motion.checked,
			hide_locked_models: form.elements.hide_locked_models.checked, sidebar_collapsed: form.elements.sidebar_collapsed.checked,
			hour_cycle: form.elements.hour_cycle.value, show_seconds: form.elements.show_seconds.checked,
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
		  reducedMotion: form.elements.reduced_motion.checked,
		  hideLockedModels: form.elements.hide_locked_models.checked,
		  sidebarCollapsed: form.elements.sidebar_collapsed.checked,
		  hourCycle: form.elements.hour_cycle.value,
		  showSeconds: form.elements.show_seconds.checked,
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
    if (logout) logout.addEventListener('click', function () { api.clearToken(); location.href = '/'; });

    const exportButton = document.querySelector('[data-export-logs]');
    if (exportButton) exportButton.addEventListener('click', async function () {
      api.showBusy(exportButton, true);
      try {
        const response = await fetch('/api/user/logs/export', { headers: { Authorization: 'Bearer ' + api.token(), Accept: 'text/csv' } });
        if (!response.ok) {
          const payload = await response.json().catch(function () { return null; });
          throw new Error(payload && payload.error && payload.error.message || '导出调用记录失败');
        }
        const blob = await response.blob();
        const url = URL.createObjectURL(blob);
        const anchor = document.createElement('a');
        anchor.href = url;
        anchor.download = 'relayfor-usage-' + new Date().toISOString().slice(0, 10).replace(/-/g, '') + '.csv';
        document.body.appendChild(anchor);
        anchor.click();
        anchor.remove();
        URL.revokeObjectURL(url);
        api.toast('调用记录已导出');
      } catch (error) { api.toast(error.message, 'danger'); }
      finally { api.showBusy(exportButton, false); }
    });

    const deactivateForm = document.querySelector('[data-deactivate-form]');
    if (deactivateForm) deactivateForm.addEventListener('submit', async function (event) {
      event.preventDefault();
      if (deactivateForm.elements.confirmation.value !== accountContext.username) return api.toast('请输入当前用户名确认停用', 'danger');
      const button = deactivateForm.querySelector('button[type="submit"]');
      api.showBusy(button, true);
      try {
        await api.request('/api/user/deactivate', { method: 'POST', body: { confirmation: deactivateForm.elements.confirmation.value, current_password: deactivateForm.elements.current_password.value } });
        api.clearToken();
        location.href = '/auth/?mode=login&account=deactivated';
      } catch (error) { api.toast(error.message, 'danger'); api.showBusy(button, false); }
    });
  }

  function bindDocs() {
    const baseURL = location.origin.replace(/\/$/, '') + '/v1';
    const snippets = {
      responses: 'curl ' + baseURL + '/responses \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  -H "Content-Type: application/json" \\\n  -d \'{"model":"gpt-5.6-sol","input":"你好，请介绍一下自己"}\'',
      python: 'from openai import OpenAI\n\nclient = OpenAI(\n    api_key="YOUR_API_KEY",\n    base_url="' + baseURL + '"\n)\n\nresponse = client.responses.create(\n    model="gpt-5.6-sol",\n    input="你好，请介绍一下自己"\n)\nprint(response.output_text)',
      node: 'import OpenAI from "openai";\n\nconst client = new OpenAI({\n  apiKey: "YOUR_API_KEY",\n  baseURL: "' + baseURL + '"\n});\n\nconst response = await client.responses.create({\n  model: "gpt-5.6-sol",\n  input: "你好，请介绍一下自己"\n});\nconsole.log(response.output_text);',
      anthropic: 'curl ' + baseURL + '/messages \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  -H "anthropic-version: 2023-06-01" \\\n  -H "Content-Type: application/json" \\\n  -d \'{"model":"claude-opus-5","max_tokens":256,"messages":[{"role":"user","content":"你好"}]}\''
    };
    snippets.responses = [
      'curl ' + baseURL + '/responses \\',
      '  -H "Authorization: Bearer YOUR_API_KEY" \\',
      '  -H "Content-Type: application/json" \\',
      '  -d \'{"model":"gpt-5.6-sol","input":"你好，请介绍一下自己"}\''
    ].join('\n');
    snippets.anthropic = [
      'curl ' + baseURL + '/messages \\',
      '  -H "Authorization: Bearer YOUR_API_KEY" \\',
      '  -H "anthropic-version: 2023-06-01" \\',
      '  -H "Content-Type: application/json" \\',
      '  -d \'{"model":"claude-opus-5","max_tokens":256,"messages":[{"role":"user","content":"你好"}]}\''
    ].join('\n');
    document.querySelectorAll('[data-doc-base-url]').forEach(function (node) { node.textContent = baseURL; });
    document.querySelectorAll('[data-doc-snippet]').forEach(function (node) { node.textContent = snippets[node.dataset.docSnippet] || ''; });
    document.querySelectorAll('[data-doc-copy]').forEach(function (button) {
      button.addEventListener('click', async function () {
        const key = button.dataset.docCopy;
        const value = key === 'base' ? baseURL : snippets[key];
        if (!value) return;
        try { await navigator.clipboard.writeText(value); api.toast('已复制到剪贴板'); }
        catch (_) { api.toast('复制失败，请手动选择文本', 'danger'); }
      });
    });
    document.querySelectorAll('[data-doc-tab]').forEach(function (button) {
      button.addEventListener('click', function () {
        const name = button.dataset.docTab;
        document.querySelectorAll('[data-doc-tab]').forEach(function (item) { item.classList.toggle('active', item === button); });
        document.querySelectorAll('[data-doc-tab-panel]').forEach(function (panel) { panel.classList.toggle('hidden', panel.dataset.docTabPanel !== name); });
      });
    });
    document.querySelectorAll('[data-doc-anchor]').forEach(function (button) {
      button.addEventListener('click', function () {
        const section = document.getElementById(button.dataset.docAnchor);
        if (section) section.scrollIntoView({ behavior: api.preferences().reducedMotion ? 'auto' : 'smooth', block: 'start' });
      });
    });
  }

  function paymentStatus(item) {
    if (item.status === 'paid') return '<span class="tag success">已到账</span>';
    if (item.status === 'pending') return '<span class="tag">待支付</span>';
    return '<span class="tag muted">已关闭</span>';
  }

  function paymentTypeLabel(value) {
    return { alipay:'支付宝', wxpay:'微信支付', qqpay:'QQ 钱包' }[value] || value || '易支付';
  }

  function paymentOrderRow(item) {
    const subscription = item.product_type === 'subscription';
    const product = subscription ? '套餐 · ' + api.escapeHTML(item.product_name || ('#' + item.subscription_plan_id)) : '永久余额充值';
    const benefit = subscription ? api.escapeHTML(item.product_name || '订阅套餐') : api.moneyFromQuota(item.quota, 2);
    return '<div class="list-item"><span class="material-symbol" aria-hidden="true">receipt_long</span><div class="list-item-body"><div class="list-item-title"><code>' + api.escapeHTML(item.order_no) + '</code> · ' + product + '</div><div class="list-item-sub">' + api.date(item.created_at) + ' · ' + api.escapeHTML(paymentTypeLabel(item.payment_type)) + (item.provider_trade_no ? ' · 流水号 ' + api.escapeHTML(item.provider_trade_no) : '') + '</div></div><div class="list-metrics"><div><b>¥' + (Number(item.amount_cny_cents || 0) / 100).toFixed(2) + '</b>支付</div><div><b>' + benefit + '</b>' + (subscription ? '套餐权益' : '到账') + '</div></div>' + paymentStatus(item) + '</div>';
  }

  async function loadPaymentOrders() {
    const body = document.querySelector('[data-payment-orders]');
    if (!body || !paymentConfig || !paymentConfig.enabled || !paymentConfig.is_pro_host) return;
    const items = await api.request('/api/user/payments/orders');
    body.innerHTML = Array.isArray(items) && items.length ? items.map(paymentOrderRow).join('') : '<div class="empty">还没有在线充值订单。</div>';
  }

  function renderPaymentQuote() {
    const form = document.querySelector('[data-payment-form]');
    if (!form || !paymentConfig) return;
    const amount = Number(form.elements.amount_cny.value);
    const usd = paymentConfig.cny_per_usd > 0 ? amount / Number(paymentConfig.cny_per_usd) : 0;
    api.setText('[data-payment-quota]', Number.isFinite(usd) ? '$' + usd.toFixed(2) : '—');
    api.setText('[data-payment-rate]', '当前换算：¥' + Number(paymentConfig.cny_per_usd).toFixed(4) + ' = $1.00；单笔 ¥' + Number(paymentConfig.min_cny).toFixed(2) + '–¥' + Number(paymentConfig.max_cny).toFixed(2));
  }

  async function loadPaymentConfig() {
	paymentConfig = await api.request('/api/user/payments/config');
	const availableHere = Boolean(paymentConfig.enabled && paymentConfig.is_pro_host);
	document.querySelectorAll('[data-payment-nav]').forEach(function (node) { node.classList.toggle('hidden', !availableHere); });
	if (!paymentConfig.is_pro_host) {
	  const panel = document.querySelector('[data-payment-panel]');
	  if (panel) panel.remove();
	  return;
	}
    const form = document.querySelector('[data-payment-form]');
    if (form) {
      form.elements.amount_cny.min = String(paymentConfig.min_cny || 1);
      form.elements.amount_cny.max = String(paymentConfig.max_cny || 5000);
      if (Number(form.elements.amount_cny.value) < Number(paymentConfig.min_cny)) form.elements.amount_cny.value = Number(paymentConfig.min_cny).toFixed(2);
    }
    renderPaymentQuote();

  }

  function bindPayments() {
    const form = document.querySelector('[data-payment-form]');
    if (!form) return;
    form.elements.amount_cny.addEventListener('input', renderPaymentQuote);
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      const button = form.querySelector('[data-payment-submit]');
      api.showBusy(button, true);
      try {
        const amount = Number(form.elements.amount_cny.value);
        if (!Number.isFinite(amount) || amount < Number(paymentConfig.min_cny) || amount > Number(paymentConfig.max_cny)) throw new Error('充值金额不在允许范围内');
        const result = await api.request('/api/user/payments/orders', { method:'POST', body:{ amount_cny:amount.toFixed(2), payment_type:form.elements.payment_type.value } });
        location.href = result.payment_url;
      } catch (error) { api.toast(error.message, 'danger'); api.showBusy(button, false); }
    });
    const refresh = document.querySelector('[data-payment-refresh]');
    if (refresh) refresh.addEventListener('click', function () { loadPaymentOrders().catch(function (error) { api.toast(error.message, 'danger'); }); });
  }

  document.addEventListener('DOMContentLoaded', async function () {
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
	document.querySelectorAll('[data-open-dialog="token-dialog"]').forEach(function (button) { button.addEventListener('click', resetTokenForm); });
    bindCreateToken();
    bindCopy();
    bindRedeem();
	bindBatchRedeem();
	bindPlanRedeem();
    bindAbandonSubscription();
    bindProfile();
    bindPassword();
    bindUserSecurity();
    bindPreferences();
    bindAccountActions();
    bindDocs();
    bindPayments();
    bindTokenLoan();
    bindLogFilters();
    window.ProxyLanResources.mount(resources, resourcesForPage);
    // Shared account and payment navigation data load alongside the active page.
    resources.load(['overview', 'paymentConfig']).catch(function (error) { api.toast(error.message, 'danger'); });
  });
}());
