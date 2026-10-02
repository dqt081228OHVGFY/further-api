(function () {
  'use strict';
  const api = window.ProxyLan;

  // 紧急停机的倒计时状态：renderEmergencyStop 和定时器都要读它，所以留在模块作用域
  let emergencyStopStatus = { enabled: false, indefinite: false, remaining_seconds: 0 };
  let emergencyStopTimer = 0;
	const settingsPages = ['security', 'announcements', 'routing', 'registration', 'payments'];
	const settingsPageTitles = { security:'安全控制', announcements:'站点公告', routing:'路由运行', registration:'注册与福利', payments:'商业支付' };

	function requestedSettingsPage() {
	  const hashParts = location.hash.slice(1).split('/');
	  return hashParts[0] === 'settings' && settingsPages.includes(hashParts[1]) ? hashParts[1] : '';
	}

	function showSettingsPage(name, updateLocation) {
	  const pageName = settingsPages.includes(name) ? name : '';
	  const directory = document.querySelector('[data-settings-directory]');
	  if (directory) directory.hidden = Boolean(pageName);
	  document.querySelectorAll('[data-settings-subpage]').forEach(function (page) {
		const active = page.dataset.settingsSubpage === pageName;
		page.hidden = !active;
		page.classList.toggle('active', active);
	  });
	  if (document.querySelector('[data-panel="settings"].active')) api.setText('[data-page-title]', pageName ? settingsPageTitles[pageName] : '安全与设置');
	  if (updateLocation) {
		const target = '#settings' + (pageName ? '/' + pageName : '');
		if (location.hash !== target) history.pushState(null, '', target);
	  }
	}

	function bindSettingsSubpages() {
	  const directory = document.querySelector('[data-settings-directory]');
	  if (!directory) return;
	  directory.querySelectorAll('[data-settings-page]').forEach(function (button) {
		button.addEventListener('click', function () { showSettingsPage(button.dataset.settingsPage, true); });
	  });
	  document.querySelectorAll('[data-settings-back]').forEach(function (button) {
		button.addEventListener('click', function () { showSettingsPage('', true); });
	  });
	  const settingsNav = document.querySelector('[data-nav="settings"]');
	  if (settingsNav) settingsNav.addEventListener('click', function (event) { showSettingsPage(event.isTrusted ? '' : requestedSettingsPage(), event.isTrusted); });
	  window.addEventListener('popstate', function () {
		if (location.hash.slice(1).split('/')[0] === 'settings') showSettingsPage(requestedSettingsPage(), false);
	  });
	  showSettingsPage(requestedSettingsPage(), false);
	}

  async function loadRegistrationSettings() {
    const data = await api.request('/api/admin/settings/registration');
    const form = document.querySelector('[data-registration-settings]');
    if (!form) return;
    const settings = data.settings || {};
    ['local_registration_enabled', 'linuxdo_login_enabled', 'linuxdo_registration_enabled', 'registration_code_required', 'pay_registration_code_required', 'pay_local_registration_enabled'].forEach(function (key) { if (form.elements[key]) form.elements[key].checked = Boolean(settings[key]); });
    ['public_initial_quota', 'pay_initial_quota', 'linuxdo_extra_quota'].forEach(function (key) {
      if (form.elements[key]) form.elements[key].value = Number(settings[key] || 0);
    });
    // 三个额度字段填完后一次性回填对应的 USD 显示
    api.syncQuotaUSD(form);
    const status = document.querySelector('[data-registration-code-status]');
    if (status) status.textContent = settings.registration_code_configured ? '已配置' : '未配置';
    const payStatus = document.querySelector('[data-pay-registration-code-status]');
    if (payStatus) payStatus.textContent = settings.pay_registration_code_configured ? '已配置' : '未配置';
    if (data.linuxdo) { api.setText('[data-linuxdo-client-id]', data.linuxdo.client_id || '未配置'); api.setText('[data-linuxdo-redirect]', data.linuxdo.redirect_url || '未配置'); }
  }

  async function loadSiteAnnouncements() {
    const form = document.querySelector('[data-site-announcements]');
    if (!form) return;
    const data = await api.request('/api/admin/settings/announcements');
    const announcements = data.announcements || {};
    form.elements.public.value = announcements.public || '';
    form.elements.pay.value = announcements.pay || '';
  }

  function bindSiteAnnouncements() {
    const form = document.querySelector('[data-site-announcements]');
    if (!form) return;
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      const button = form.querySelector('button[type="submit"]');
      api.showBusy(button, true);
      try {
        await api.request('/api/admin/settings/announcements', { method: 'PUT', body: { public: form.elements.public.value, pay: form.elements.pay.value } });
        api.toast('站点公告已更新');
        await loadSiteAnnouncements();
      } catch (error) { api.toast(error.message, 'danger'); }
      finally { api.showBusy(button, false); }
    });
  }

  async function loadRoutingSettings() {
    const form = document.querySelector('[data-routing-settings]');
    if (!form) return;
    const data = await api.request('/api/admin/settings/routing');
    const settings = data.settings || {};
    form.elements.channel_affinity_minutes.value = String(settings.channel_affinity_minutes || 360);
    form.elements.channel_key_affinity_minutes.value = String(settings.channel_key_affinity_minutes || 360);
    form.elements.channel_failure_threshold.value = String(settings.channel_failure_threshold || 3);
    form.elements.channel_key_recovery_minutes.value = String(settings.channel_key_recovery_minutes || 5);
    form.elements.retry_enabled.checked = Boolean(settings.retry_enabled);
    form.elements.retry_max_attempts.value = String(settings.retry_max_attempts || 3);
    form.elements.retry_status_codes.value = Array.isArray(settings.retry_status_codes) ? settings.retry_status_codes.join(',') : '';
    form.elements.session_lease_minutes.value = String(settings.session_lease_minutes || 60);
    form.elements.session_requests_per_minute.value = String(settings.session_requests_per_minute || 15);
  }

  function bindRoutingSettings() {
    const form = document.querySelector('[data-routing-settings]');
    if (!form) return;
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      const statusCodes = form.elements.retry_status_codes.value.split(',').map(function (value) { return value.trim(); }).filter(Boolean).map(Number);
      if (statusCodes.some(function (code) { return !Number.isInteger(code); })) return api.toast('可重试状态码必须是整数，并使用英文逗号分隔', 'danger');
      if (statusCodes.some(function (code) { return code < 400 || code > 599; })) return api.toast('可重试状态码必须在 400 到 599 之间', 'danger');
      const button = form.querySelector('button[type="submit"]');
      api.showBusy(button, true);
      try {
        await api.request('/api/admin/settings/routing', { method:'PUT', body:{
          channel_affinity_minutes:Number(form.elements.channel_affinity_minutes.value),
          channel_key_affinity_minutes:Number(form.elements.channel_key_affinity_minutes.value),
          channel_failure_threshold:Number(form.elements.channel_failure_threshold.value),
          channel_key_recovery_minutes:Number(form.elements.channel_key_recovery_minutes.value),
          retry_enabled:form.elements.retry_enabled.checked,
          retry_max_attempts:Number(form.elements.retry_max_attempts.value),
          retry_status_codes:statusCodes,
          session_models:[],
          session_only_models:[],
          session_lease_minutes:Number(form.elements.session_lease_minutes.value),
          session_requests_per_minute:Number(form.elements.session_requests_per_minute.value),
          session_concurrent_requests:2
        } });
        api.toast('路由与容错设置已更新');
        await loadRoutingSettings();
      } catch (error) { api.toast(error.message, 'danger'); }
      finally { api.showBusy(button, false); }
    });
  }

  async function loadOperationalSettings() {
    const form = document.querySelector('[data-operational-settings]');
    if (!form) return;
    const data = await api.request('/api/admin/settings/operational');
    const settings = data.settings || {};
    const defaults = { global_ip_requests_per_minute:120, default_user_requests_per_minute:15, default_user_concurrent_requests:2, stream_heartbeat_seconds:8, realtime_handshake_timeout_seconds:15, realtime_write_timeout_seconds:10, realtime_max_message_mb:16, default_max_output_tokens:4096, unknown_image_token_estimate:2048 };
    Object.keys(defaults).forEach(function (key) { form.elements[key].value = String(settings[key] || defaults[key]); });
    form.elements.user_docs_visible.checked = settings.user_docs_visible !== false;
    const proofForm = document.querySelector('[data-admin-proof-settings]');
    if (proofForm) proofForm.elements.admin_proof_lifetime_minutes.value = String(settings.admin_proof_lifetime_minutes || 5);
  }

  function bindOperationalSettings() {
    const form = document.querySelector('[data-operational-settings]');
    if (!form) return;
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      const body = {};
      ['global_ip_requests_per_minute', 'default_user_requests_per_minute', 'default_user_concurrent_requests', 'stream_heartbeat_seconds', 'realtime_handshake_timeout_seconds', 'realtime_write_timeout_seconds', 'realtime_max_message_mb', 'default_max_output_tokens', 'unknown_image_token_estimate'].forEach(function (key) { body[key] = Number(form.elements[key].value); });
      const proofForm = document.querySelector('[data-admin-proof-settings]');
      body.admin_proof_lifetime_minutes = Number(proofForm ? proofForm.elements.admin_proof_lifetime_minutes.value : 5);
      body.user_docs_visible = form.elements.user_docs_visible.checked;
      const button = form.querySelector('button[type="submit"]');
      api.showBusy(button, true);
      try {
        await api.request('/api/admin/settings/operational', { method:'PUT', body:body });
        api.toast('高级运行参数已更新');
        await loadOperationalSettings();
      } catch (error) { api.toast(error.message, 'danger'); }
      finally { api.showBusy(button, false); }
    });
  }

  function bindAdminProofSettings() {
    const form = document.querySelector('[data-admin-proof-settings]');
    if (!form) return;
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      const minutes = Number(form.elements.admin_proof_lifetime_minutes.value);
      if (!Number.isInteger(minutes) || minutes < 1 || minutes > 60) return api.toast('管理员二次验证有效期必须在 1 到 60 分钟之间', 'danger');
      const button = form.querySelector('button[type="submit"]');
      api.showBusy(button, true);
      try {
        await api.request('/api/admin/security/proof-lifetime', { method:'PUT', body:{ admin_proof_lifetime_minutes:minutes } });
        api.toast('管理员二次验证有效期已更新');
        await loadOperationalSettings();
      } catch (error) { api.toast(error.message, 'danger'); }
      finally { api.showBusy(button, false); }
    });
  }

  async function loadTokenLoanSettings() {
    const data = await api.request('/api/admin/settings/token-loan');
    const form = document.querySelector('[data-token-loan-settings]');
    if (!form) return;
    const settings = data.settings || {};
    const pool = settings.loss_pool || {};
    form.elements.enabled.checked = Boolean(settings.enabled);
    form.elements.max_borrow_quota.value = String(settings.max_borrow_quota || 500000);
	form.elements.min_checkin_quota.value = String(settings.min_checkin_quota || 1440000);
    form.elements.daily_cut_limit.value = String(settings.daily_cut_limit || 2);
	form.elements.loss_pool_visible.checked = pool.visible !== false;
	form.elements.loss_pool_title.value = pool.title || '帮助站长砍一刀';
	form.elements.loss_pool_description.value = pool.description || '';
	form.elements.loss_pool_total_quota.value = String(Number(pool.total_quota) || 0);
	form.elements.loss_pool_remaining_quota.value = String(Number(pool.remaining_quota) || 0);
	form.elements.loss_pool_cut_min_quota.value = String(Number(pool.cut_min_quota) || 500000);
	form.elements.loss_pool_cut_max_quota.value = String(Number(pool.cut_max_quota) || 1500000);
    api.syncQuotaUSD(form);
  }

  function bindTokenLoanSettings() {
    const form = document.querySelector('[data-token-loan-settings]');
    if (!form) return;
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      const button = form.querySelector('button[type="submit"]');
      api.showBusy(button, true);
      try {
        await api.request('/api/admin/settings/token-loan', { method:'PUT', body:{
		  enabled:form.elements.enabled.checked,
		  max_borrow_quota:Number(form.elements.max_borrow_quota.value),
		  min_checkin_quota:Number(form.elements.min_checkin_quota.value),
		  daily_cut_limit:Number(form.elements.daily_cut_limit.value),
		  loss_pool_visible:form.elements.loss_pool_visible.checked,
		  loss_pool_title:form.elements.loss_pool_title.value.trim(),
		  loss_pool_description:form.elements.loss_pool_description.value.trim(),
		  loss_pool_total_quota:Number(form.elements.loss_pool_total_quota.value),
		  loss_pool_remaining_quota:Number(form.elements.loss_pool_remaining_quota.value),
		  loss_pool_cut_min_quota:Number(form.elements.loss_pool_cut_min_quota.value),
		  loss_pool_cut_max_quota:Number(form.elements.loss_pool_cut_max_quota.value)
		} });
        api.toast('词元贷设置已更新');
        await loadTokenLoanSettings();
      } catch (error) { api.toast(error.message, 'danger'); }
      finally { api.showBusy(button, false); }
    });
  }

  function adminPaymentStatus(item) {
    if (item.status === 'paid') return '<span class="tag success">已到账</span>';
    if (item.status === 'pending') return '<span class="tag">待支付</span>';
    return '<span class="tag muted">' + api.escapeHTML(item.status || '已关闭') + '</span>';
  }

  async function loadPaymentSettings() {
    const data = await api.request('/api/admin/settings/payment');
    const settings = data.settings || {};
    const form = document.querySelector('[data-payment-settings]');
    if (!form) return;
    form.elements.enabled.checked = Boolean(settings.enabled);
    form.elements.pro_site_host.value = settings.pro_site_host || 'pro.relayfor.xyz';
    form.elements.gateway_url.value = settings.gateway_url || 'https://pay.outme.cn';
    form.elements.merchant_id.value = settings.merchant_id || '';
    form.elements.new_merchant_key.value = '';
    form.elements.cny_per_usd.value = Number(settings.cny_per_usd || 7.3).toFixed(4);
    form.elements.min_cny.value = Number(settings.min_cny || 1).toFixed(2);
    form.elements.max_cny.value = Number(settings.max_cny || 5000).toFixed(2);
    form.elements.subscription_markup.value = Number(settings.subscription_markup || 1.04).toFixed(4);
    api.setText('[data-epay-key-status]', settings.merchant_key_configured ? '已配置' : '未配置');
    api.setText('[data-epay-notify-url]', 'https://' + form.elements.pro_site_host.value + '/api/payments/epay/notify');
  }

  async function loadAdminPaymentOrders() {
    const items = await api.request('/api/admin/payment-orders?limit=100');
    const body = document.querySelector('[data-admin-payment-orders]');
    if (!body) return;
    body.innerHTML = Array.isArray(items) && items.length ? items.map(function (item) {
      const subscription = item.product_type === 'subscription';
      const product = subscription ? '套餐 ' + api.escapeHTML(item.product_name || ('#' + item.subscription_plan_id)) : '永久余额';
      return '<div class="list-item"><span class="material-symbol" aria-hidden="true">receipt_long</span><div class="list-item-body"><div class="list-item-title"><code>' + api.escapeHTML(item.order_no) + '</code> · ' + api.escapeHTML(item.username || ('用户 #' + item.user_id)) + '</div><div class="list-item-sub">' + product + ' · ' + api.date(item.created_at) + ' · ' + api.escapeHTML(item.payment_type || '易支付') + (item.provider_trade_no ? ' · ' + api.escapeHTML(item.provider_trade_no) : '') + '</div></div><div class="list-metrics"><div><b>¥' + (Number(item.amount_cny_cents || 0) / 100).toFixed(2) + '</b>支付</div><div><b>' + (subscription ? api.escapeHTML(item.product_name || '订阅套餐') : api.moneyFromQuota(item.quota, 2)) + '</b>' + (subscription ? '套餐权益' : '到账') + '</div></div>' + adminPaymentStatus(item) + '</div>';
    }).join('') : '<div class="empty">暂无在线支付订单。</div>';
  }

  function bindPaymentSettings() {
    const form = document.querySelector('[data-payment-settings]');
    if (!form) return;
    form.elements.pro_site_host.addEventListener('input', function () { api.setText('[data-epay-notify-url]', 'https://' + form.elements.pro_site_host.value.trim() + '/api/payments/epay/notify'); });
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      const button = form.querySelector('button[type="submit"]');
      api.showBusy(button, true);
      try {
        await api.request('/api/admin/settings/payment', { method:'PUT', body:{ enabled:form.elements.enabled.checked, pro_site_host:form.elements.pro_site_host.value.trim(), gateway_url:form.elements.gateway_url.value.trim(), merchant_id:form.elements.merchant_id.value.trim(), new_merchant_key:form.elements.new_merchant_key.value.trim(), cny_per_usd:Number(form.elements.cny_per_usd.value), min_cny:Number(form.elements.min_cny.value), max_cny:Number(form.elements.max_cny.value), subscription_markup:Number(form.elements.subscription_markup.value) } });
        api.toast('易支付设置已更新');
        await loadPaymentSettings();
      } catch (error) { api.toast(error.message, 'danger'); }
      finally { api.showBusy(button, false); }
    });
    const refresh = document.querySelector('[data-admin-payment-refresh]');
    if (refresh) refresh.addEventListener('click', function () { loadAdminPaymentOrders().catch(function (error) { api.toast(error.message, 'danger'); }); });
  }

  function emergencyRemainingText(status) {
    if (!status || !status.enabled) return '当前未拉闸，新的 API 调用正常进入。';
    if (status.indefinite || !status.until) return '拉闸已开启，将持续到管理员手动恢复。';
    const seconds = Math.max(0, Math.ceil((new Date(status.until).getTime() - Date.now()) / 1000));
    if (seconds <= 0) return '拉闸时间已结束，正在恢复 API 调用…';
    const hours = Math.floor(seconds / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    const rest = seconds % 60;
    return '拉闸已开启，剩余 ' + (hours ? hours + ' 小时 ' : '') + (minutes ? minutes + ' 分 ' : '') + rest + ' 秒；到期后自动恢复。';
  }

  function renderEmergencyStop(status, updateForm) {
    emergencyStopStatus = status || { enabled:false, indefinite:false, remaining_seconds:0 };
    const enabled = Boolean(emergencyStopStatus.enabled);
    const form = document.querySelector('[data-emergency-stop-form]');
    const card = document.querySelector('[data-emergency-stop-card]');
    if (card) card.classList.toggle('active', enabled);
    if (form && updateForm !== false) {
      form.elements.enabled.checked = enabled;
      if (enabled && emergencyStopStatus.indefinite) form.elements.duration_minutes.value = '0';
      else if (enabled && emergencyStopStatus.remaining_seconds) form.elements.duration_minutes.value = String(Math.max(1, Math.ceil(Number(emergencyStopStatus.remaining_seconds) / 60)));
      else if (!form.elements.duration_minutes.value) form.elements.duration_minutes.value = '30';
    }
    api.setText('[data-emergency-switch-title]', enabled ? 'API 新调用已停止' : 'API 调用正常开放');
    api.setText('[data-emergency-switch-hint]', enabled ? '关闭开关并应用即可立即恢复。' : '打开开关后，新的 /v1/* 请求将立即返回 HTTP 503。');
    api.setText('[data-emergency-stop-status]', emergencyRemainingText(emergencyStopStatus));
  }

  async function loadEmergencyStop() {
    const status = await api.request('/api/admin/settings/emergency-stop');
    renderEmergencyStop(status, true);
    window.clearInterval(emergencyStopTimer);
    emergencyStopTimer = window.setInterval(function () {
      renderEmergencyStop(emergencyStopStatus, false);
      if (emergencyStopStatus.enabled && emergencyStopStatus.until && new Date(emergencyStopStatus.until).getTime() <= Date.now()) {
        window.clearInterval(emergencyStopTimer);
        loadEmergencyStop().catch(function () {});
      }
    }, 1000);
  }

  function bindEmergencyStop() {
    const form = document.querySelector('[data-emergency-stop-form]');
    if (!form) return;
    form.elements.enabled.addEventListener('change', function () {
      document.querySelector('[data-emergency-stop-card]').classList.toggle('active', form.elements.enabled.checked);
      api.setText('[data-emergency-switch-title]', form.elements.enabled.checked ? '准备停止 API 新调用' : '准备恢复 API 调用');
    });
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      const enabled = form.elements.enabled.checked;
      const minutes = Number(form.elements.duration_minutes.value);
      if (!Number.isInteger(minutes) || minutes < 0 || minutes > 43200) return api.toast('持续时间必须是 0 到 43200 的整数分钟', 'danger');
      if (enabled) {
        const duration = minutes === 0 ? '直到手动恢复' : minutes + ' 分钟';
        if (!(await api.confirm('开启后，所有新的 API 调用会立即返回 HTTP 503，持续' + duration + '。现有请求会继续完成。', { title:'确认全局拉闸', confirmText:'立即拉闸' }))) { renderEmergencyStop(emergencyStopStatus, true); return; }
      }
      const button = form.querySelector('[data-emergency-stop-submit]');
      api.showBusy(button, true);
      try {
        const result = await api.request('/api/admin/settings/emergency-stop', { method:'PUT', body:{ enabled:enabled, duration_minutes:minutes } });
        renderEmergencyStop(result.status, true);
        api.toast(result.message);
      } catch (error) { api.toast(error.message, 'danger'); renderEmergencyStop(emergencyStopStatus, true); }
      finally { api.showBusy(button, false); }
    });
  }

  function bindRegistrationSettings() {
    const form = document.querySelector('[data-registration-settings]');
    if (!form) return;
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      const button = form.querySelector('button[type="submit"]');
      api.showBusy(button, true);
      try {
        await api.request('/api/admin/settings/registration', { method: 'PATCH', body: {
          local_registration_enabled: form.elements.local_registration_enabled.checked,
          linuxdo_login_enabled: form.elements.linuxdo_login_enabled.checked,
          linuxdo_registration_enabled: form.elements.linuxdo_registration_enabled.checked,
          registration_code_required: form.elements.registration_code_required.checked,
          new_registration_code: form.elements.new_registration_code.value.trim(),
          pay_registration_code_required: form.elements.pay_registration_code_required.checked,
          pay_local_registration_enabled: form.elements.pay_local_registration_enabled.checked,
          new_pay_registration_code: form.elements.new_pay_registration_code.value.trim(),
          public_initial_quota: Number(form.elements.public_initial_quota.value || 0),
          pay_initial_quota: Number(form.elements.pay_initial_quota.value || 0),
          linuxdo_extra_quota: Number(form.elements.linuxdo_extra_quota.value || 0)
        } });
        form.elements.new_registration_code.value = '';
        form.elements.new_pay_registration_code.value = '';
        api.toast('注册设置已更新');
        await loadRegistrationSettings();
      } catch (error) { api.toast(error.message, 'danger'); }
      finally { api.showBusy(button, false); }
    });
  }


  // load 供 admin.js 引导时统一加载；loadRegistrationSettings 另外暴露，
  // 因为注册码增删后需要单独刷新注册设置里的余量显示。
  async function load() {
    const results = await Promise.allSettled([
      loadRegistrationSettings(), loadSiteAnnouncements(), loadRoutingSettings(),
      loadOperationalSettings(), loadTokenLoanSettings(), loadPaymentSettings(),
      loadAdminPaymentOrders(), loadEmergencyStop()
    ]);
    const failure = results.find(function (result) { return result.status === 'rejected'; });
    if (failure) throw failure.reason;
  }

  function bind() {
	bindSettingsSubpages();
    bindRegistrationSettings();
    bindSiteAnnouncements();
    bindRoutingSettings();
    bindOperationalSettings();
    bindAdminProofSettings();
    bindTokenLoanSettings();
    bindPaymentSettings();
    bindEmergencyStop();
  }

  window.ProxyLanAdminSettings = { load: load, bind: bind, loadRegistrationSettings: loadRegistrationSettings };
}());
