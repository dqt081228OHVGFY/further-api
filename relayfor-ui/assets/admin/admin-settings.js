(function () {
  'use strict';
  const api = window.ProxyLan;

  // 紧急停机的倒计时状态：renderEmergencyStop 和定时器都要读它，所以留在模块作用域
  let emergencyStopStatus = { enabled: false, indefinite: false, remaining_seconds: 0 };
  let emergencyStopTimer = 0;
	const settingsPages = ['security', 'site', 'announcements', 'routing', 'registration', 'system-info', 'request-policies', 'passkey', 'payment', 'model-pricing', 'group-pricing', 'chat', 'alerts'];
	const settingsPageTitles = { security:'安全控制', site:'站点与登录', announcements:'站点公告', routing:'请求重试', registration:'注册与福利', 'system-info':'运行监控', 'request-policies':'请求检测与健康', passkey:'通行密钥', payment:'支付网关', 'model-pricing':'完整模型计费', 'group-pricing':'分组与权益倍率', chat:'聊天预设', alerts:'监控与告警' };

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
	  if (document.querySelector('[data-panel="settings"].active')) {
        api.setText('[data-page-title]', pageName ? settingsPageTitles[pageName] : '安全与设置');
        document.querySelectorAll('.nav-item[data-nav="settings"], .mobile-nav-option[data-mobile-nav="settings"]').forEach(function (nav) {
          const active = nav.dataset.openSettings ? nav.dataset.openSettings === pageName : pageName !== 'site';
          nav.classList.toggle('active', active);
          if (active) nav.setAttribute('aria-current', 'page'); else nav.removeAttribute('aria-current');
        });
      }
	  if (updateLocation) {
		const target = '#settings' + (pageName ? '/' + pageName : '');
		if (location.hash !== target) history.pushState(null, '', target);
	  }
	  window.dispatchEvent(new CustomEvent('relayfor:settings-subpage', { detail: { name: pageName } }));
	}

	function bindSettingsSubpages() {
	  const directory = document.querySelector('[data-settings-directory]');
	  if (!directory) return;
	  directory.addEventListener('click', function (event) {
		const button = event.target.closest('[data-settings-page]');
		if (button && directory.contains(button)) showSettingsPage(button.dataset.settingsPage, true);
	  });
	  document.querySelector('[data-panel="settings"]').addEventListener('click', function (event) {
		if (event.target.closest('[data-settings-back]')) showSettingsPage('', true);
	  });
	  document.querySelectorAll('[data-nav="settings"]').forEach(function (settingsNav) {
	    settingsNav.addEventListener('click', function (event) {
	      const target = settingsNav.dataset.openSettings;
	      showSettingsPage(target || (event.isTrusted ? '' : requestedSettingsPage()), Boolean(target) || event.isTrusted);
	    });
	  });
	  window.addEventListener('popstate', function () {
		if (location.hash.slice(1).split('/')[0] === 'settings') showSettingsPage(requestedSettingsPage(), false);
	  });
	  showSettingsPage(requestedSettingsPage(), false);
	}

  let paymentBusy = false;
  let paymentConfig = null;

  function renderPaymentSettings(config) {
    const form = document.querySelector('[data-payment-settings]');
    paymentConfig = config;
    ['client_id', 'credits_per_unit', 'min_topup', 'callback_base_url', 'public_key'].forEach(function (key) { form.elements[key].value = config[key] || ''; });
    form.elements.enabled.checked = Boolean(config.enabled);
    form.elements.client_secret.value = '';
    form.elements.private_key.value = '';
    api.setText('[data-payment-secret-status]', config.secret_configured ? '应用密钥已配置，留空保留现有密钥。' : '尚未配置应用密钥。');
    api.setText('[data-payment-notify]', config.notify_url || '保存有效站点地址后生成');
    api.setText('[data-payment-return]', config.return_url || '保存有效站点地址后生成');
    form.querySelector('[data-payment-copy]').disabled = !config.public_key;
  }

  function setPaymentBusy(busy) {
    paymentBusy = busy;
    document.querySelector('[data-payment-fields]').disabled = busy || !paymentConfig;
    document.querySelector('[data-payment-reload]').disabled = busy;
  }

  async function loadPaymentSettings() {
    if (!document.querySelector('[data-payment-settings]') || paymentBusy) return;
    setPaymentBusy(true);
    api.setText('[data-payment-status]', '正在读取支付配置…');
    try {
      const config = await api.request('/api/admin/ldc/config');
      renderPaymentSettings(config);
      api.setText('[data-payment-status]', config.enabled && config.configured ? 'LDC 支付已开放' : 'LDC 支付尚未开放，可在下方完成配置。');
    } catch (error) {
      api.setText('[data-payment-status]', '读取失败：' + error.message + '。请点击重新读取。');
    } finally { setPaymentBusy(false); }
  }

  function bindPaymentSettings() {
    const form = document.querySelector('[data-payment-settings]');
    if (!form) return;
    document.querySelector('[data-payment-reload]').addEventListener('click', loadPaymentSettings);
    form.querySelector('[data-payment-copy]').addEventListener('click', async function () {
      try {
        if (!navigator.clipboard) throw new Error('请手动选择商户公钥复制');
        await navigator.clipboard.writeText(form.elements.public_key.value);
        api.toast('商户公钥已复制');
      } catch (error) { api.toast(error.message || '复制失败，请手动复制', 'danger'); }
    });
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      if (paymentBusy || !paymentConfig) return;
      const body = { enabled: form.elements.enabled.checked };
      ['client_id', 'credits_per_unit', 'min_topup', 'callback_base_url'].forEach(function (key) { body[key] = form.elements[key].value.trim(); });
      for (const key of ['credits_per_unit', 'min_topup']) {
        if (!Number.isFinite(Number(body[key])) || Number(body[key]) <= 0) {
          api.toast('兑换比例与最低充值金额必须为大于 0 的数字', 'danger'); form.elements[key].focus(); return;
        }
      }
      if (body.callback_base_url) {
        try {
          const url = new URL(body.callback_base_url);
          if (url.protocol !== 'https:' || url.username || url.password || url.search || url.hash || url.pathname !== '/') throw new Error();
          body.callback_base_url = url.origin;
        } catch (_) { api.toast('请填写完整 HTTPS 站点域名，例如 https://relay.example.com', 'danger'); form.elements.callback_base_url.focus(); return; }
      }
      if (body.enabled && (!body.client_id || (!paymentConfig.secret_configured && !form.elements.client_secret.value.trim()))) {
        api.toast('开放支付前，请填写 LDC 应用 ID 和应用密钥', 'danger'); return;
      }
      ['client_secret', 'private_key'].forEach(function (key) { if (form.elements[key].value.trim()) body[key] = form.elements[key].value.trim(); });
      setPaymentBusy(true);
      api.setText('[data-payment-status]', '正在保存支付配置…');
      try {
        renderPaymentSettings(await api.request('/api/admin/ldc/config', { method: 'PUT', body: body }));
        api.setText('[data-payment-status]', '支付配置已保存' + (paymentConfig.enabled && paymentConfig.configured ? '，LDC 支付已开放。' : '。'));
        api.toast('支付配置已保存');
      } catch (error) {
        api.setText('[data-payment-status]', '保存失败：' + error.message);
        api.toast(error.message, 'danger');
      } finally { setPaymentBusy(false); }
    });
  }

  async function loadRegistrationSettings() {
    const data = await api.request('/api/admin/settings/registration');
    const form = document.querySelector('[data-registration-settings]');
    if (!form) return;
    const settings = data.settings || {};
    ['local_registration_enabled', 'linuxdo_login_enabled', 'linuxdo_registration_enabled', 'registration_code_required'].forEach(function (key) { if (form.elements[key]) form.elements[key].checked = Boolean(settings[key]); });
    ['public_initial_quota', 'linuxdo_extra_quota'].forEach(function (key) {
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
  }

  function bindSiteAnnouncements() {
    const form = document.querySelector('[data-site-announcements]');
    if (!form) return;
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      const button = form.querySelector('button[type="submit"]');
      api.showBusy(button, true);
      try {
        await api.request('/api/admin/settings/announcements', { method: 'PUT', body: { public: form.elements.public.value } });
        api.toast('站点公告已更新');
        await loadSiteAnnouncements();
      } catch (error) { api.toast(error.message, 'danger'); }
      finally { api.showBusy(button, false); }
    });
  }

  async function loadSiteSettings() {
    const form = document.querySelector('[data-site-settings]');
    if (!form) return;
    const data = await api.request('/api/admin/settings/site');
    const settings = data.settings || {};
    form.elements.system_name.value = settings.system_name || '';
    form.elements.logo.value = settings.logo || '';
    form.elements.server_address.value = settings.server_address || location.origin;
    form.elements.linuxdo_enabled.checked = Boolean(settings.linuxdo_enabled);
    form.elements.linuxdo_client_id.value = settings.linuxdo_client_id || '';
    form.elements.linuxdo_minimum_trust_level.value = String(settings.linuxdo_minimum_trust_level || 0);
    form.dataset.secretConfigured = String(Boolean(settings.linuxdo_client_secret_configured));
    form.dataset.settingsLoaded = 'true';
    form.querySelector('button[type="submit"]').disabled = false;
    const secret = document.querySelector('[data-linuxdo-secret-status]');
    if (secret) secret.textContent = settings.linuxdo_client_secret_configured ? '密钥已配置，留空可保留。' : '尚未配置密钥。';
    api.setText('[data-linuxdo-callback-url]', settings.linuxdo_redirect_url || '');
    updateSitePreview(form);
    api.setText('[data-site-setup-state]', settings.linuxdo_enabled ? 'Linux DO 登录已启用' : 'Linux DO 登录未启用');
    api.setText('[data-site-setup-hint]', settings.linuxdo_enabled ? '登录入口已经开放；新用户注册权限在「注册与福利」中设置。' : '填写 Connect 应用凭据、登记回调地址并启用开关，即可开放登录入口。');
  }

  function updateSitePreview(form) {
    const name = form.elements.system_name.value.trim() || '你的网站';
    api.setText('[data-site-preview-name]', name);
    const image = form.querySelector('[data-site-preview-logo]');
    if (image) {
      const value = form.elements.logo.value.trim();
      try {
        const url = new URL(value || '/relayfor-logo.svg', location.origin);
        image.src = ['http:', 'https:'].includes(url.protocol) ? url.href : '/relayfor-logo.svg';
      } catch (_) { image.src = '/relayfor-logo.svg'; }
    }
  }

  function prepareSiteSettings(form) {
    if (form.querySelector('[data-site-preview-name]')) return;
    const preview = document.createElement('div');
    preview.className = 'md-card site-settings-preview';
    preview.innerHTML = '<div class="card-head"><div class="site-preview-brand"><span class="site-preview-logo"><img data-site-preview-logo src="/relayfor-logo.svg" width="44" height="44" alt=""></span><div><div class="md-card-title" data-site-preview-name>你的网站</div><p class="card-hint">网站名称与 Logo 将同步到首页、登录页和控制台。</p></div></div><span class="material-symbol" aria-hidden="true">visibility</span></div><div class="site-setup-status"><span class="material-symbol" aria-hidden="true">passkey</span><div><b data-site-setup-state>读取站点配置…</b><p class="card-hint" data-site-setup-hint>站点配置保存在服务器。</p></div></div>';
    form.prepend(preview);
    preview.querySelector('img').addEventListener('error', function (event) {
      if (event.target.getAttribute('src') !== '/relayfor-logo.svg') event.target.src = '/relayfor-logo.svg';
    });
    ['system_name', 'logo'].forEach(function (name) { form.elements[name].addEventListener('input', function () { updateSitePreview(form); }); });
    form.querySelectorAll('.md-textfield').forEach(function (field) {
      const control = field.querySelector('input, select');
      const label = field.querySelector('label');
      if (control && label) { control.id = control.id || 'site-' + control.name; label.htmlFor = control.id; }
    });
    const callback = form.querySelector('[data-linuxdo-callback-url]');
    if (callback) {
      const copy = document.createElement('button');
      copy.type = 'button';
      copy.className = 'md-btn md-btn-text md-btn-compact';
      copy.dataset.linuxdoCopyCallback = '';
      copy.innerHTML = '<span class="material-symbol" aria-hidden="true">content_copy</span>复制回调地址';
      callback.closest('.status-line').append(copy);
      copy.addEventListener('click', async function () {
        try {
          const value = callback.textContent.trim();
          if (!value) throw new Error('回调地址尚未读取，请稍后重试');
          if (!navigator.clipboard) throw new Error('当前浏览器无法自动复制，请手动选择回调地址');
          await navigator.clipboard.writeText(value);
          api.toast('回调地址已复制');
        } catch (error) { api.toast(error.message || '复制失败，请手动选择回调地址', 'danger'); }
      });
    }
    const save = form.querySelector('button[type="submit"]');
    save.disabled = true;
    save.innerHTML = '<span class="material-symbol" aria-hidden="true">save</span>保存站点与登录配置';
  }

  function bindSiteSettings() {
    const form = document.querySelector('[data-site-settings]');
    if (!form) return;
    prepareSiteSettings(form);
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      if (form.dataset.settingsLoaded !== 'true') return api.toast('正在读取站点配置，请稍后再保存', 'danger');
      if (form.elements.linuxdo_enabled.checked && (!form.elements.linuxdo_client_id.value.trim() || (!form.elements.linuxdo_client_secret.value.trim() && form.dataset.secretConfigured !== 'true'))) {
        api.toast('启用 Linux DO 登录前，请填写 Client ID 和 Client Secret', 'danger');
        form.elements[!form.elements.linuxdo_client_id.value.trim() ? 'linuxdo_client_id' : 'linuxdo_client_secret'].focus();
        return;
      }
      const logoValue = form.elements.logo.value.trim();
      if (logoValue) {
        try {
          const logoURL = new URL(logoValue, location.origin);
          if (!['http:', 'https:'].includes(logoURL.protocol)) throw new Error('invalid');
        } catch (_) { api.toast('Logo 地址须为 HTTP、HTTPS 地址或站内路径', 'danger'); form.elements.logo.focus(); return; }
      }
      const button = form.querySelector('button[type="submit"]');
      api.showBusy(button, true);
      try {
        await api.request('/api/admin/settings/site', { method: 'PUT', body: {
          system_name: form.elements.system_name.value.trim(),
          logo: form.elements.logo.value.trim(),
          server_address: form.elements.server_address.value.trim(),
          linuxdo_enabled: form.elements.linuxdo_enabled.checked,
          linuxdo_client_id: form.elements.linuxdo_client_id.value.trim(),
          linuxdo_client_secret: form.elements.linuxdo_client_secret.value.trim(),
          linuxdo_minimum_trust_level: Number(form.elements.linuxdo_minimum_trust_level.value || 0)
        } });
        form.elements.linuxdo_client_secret.value = '';
        api.toast('站点与 LinuxDO 设置已更新');
        await loadSiteSettings();
        await loadRegistrationSettings();
        const status = await api.request('/api/status');
        if (window.ProxyLanSite) window.ProxyLanSite.applyContext(status);
        else api.setText('.rail-title, .public-brand strong, .auth-brand strong, [data-site-name]', status.system_name);
        document.title = '管理后台 · ' + status.system_name;
      } catch (error) { api.toast(error.message, 'danger'); }
      finally { api.showBusy(button, false); }
    });
  }

  async function loadRoutingSettings() {
    const form = document.querySelector('[data-routing-settings]');
    if (!form) return;
    const data = await api.request('/api/admin/settings/routing');
    const settings = data.settings || {};
    form.elements.retry_enabled.checked = Boolean(settings.retry_enabled);
    form.elements.retry_max_attempts.value = String(settings.retry_max_attempts || 1);
    form.elements.retry_status_codes.value = String(settings.retry_status_codes || '');
  }

  function bindRoutingSettings() {
    const form = document.querySelector('[data-routing-settings]');
    if (!form) return;
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      const statusCodes = form.elements.retry_status_codes.value.trim();
      if (statusCodes && !statusCodes.split(',').every(function (part) {
        const value = part.trim();
        const match = value.match(/^(\d{3})(?:\s*-\s*(\d{3}))?$/);
        return match && Number(match[1]) >= 100 && Number(match[1]) <= 599 && (!match[2] || (Number(match[2]) >= Number(match[1]) && Number(match[2]) <= 599));
      })) return api.toast('可重试状态码须在 100 到 599 之间，可用英文逗号和 500-503 范围', 'danger');
      const button = form.querySelector('button[type="submit"]');
      api.showBusy(button, true);
      try {
        await api.request('/api/admin/settings/routing', { method:'PUT', body:{
          retry_enabled:form.elements.retry_enabled.checked,
          retry_max_attempts:Number(form.elements.retry_max_attempts.value),
          retry_status_codes:statusCodes
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
    form.elements.max_borrow_quota.value = String(settings.max_borrow_quota ?? api.quotaFromUSD(1));
	form.elements.min_checkin_quota.value = String(settings.min_checkin_quota ?? api.quotaFromUSD(2.88));
    form.elements.daily_cut_limit.value = String(settings.daily_cut_limit || 2);
	form.elements.loss_pool_visible.checked = pool.visible !== false;
	form.elements.loss_pool_title.value = pool.title || '帮助站长砍一刀';
	form.elements.loss_pool_description.value = pool.description || '';
	form.elements.loss_pool_total_quota.value = String(Number(pool.total_quota) || 0);
	form.elements.loss_pool_remaining_quota.value = String(Number(pool.remaining_quota) || 0);
	form.elements.loss_pool_cut_min_quota.value = String(pool.cut_min_quota == null ? api.quotaFromUSD(1) : Number(pool.cut_min_quota));
	form.elements.loss_pool_cut_max_quota.value = String(pool.cut_max_quota == null ? api.quotaFromUSD(3) : Number(pool.cut_max_quota));
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
          public_initial_quota: Number(form.elements.public_initial_quota.value || 0),
          linuxdo_extra_quota: Number(form.elements.linuxdo_extra_quota.value || 0)
        } });
        form.elements.new_registration_code.value = '';
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
      loadRegistrationSettings(), loadSiteSettings(), loadSiteAnnouncements(), loadRoutingSettings(),
      loadTokenLoanSettings(), loadPaymentSettings()
    ]);
    const failure = results.find(function (result) { return result.status === 'rejected'; });
    if (failure) throw failure.reason;
  }

  function bind() {
	bindSettingsSubpages();
    bindSiteSettings();
    bindPaymentSettings();
    bindRegistrationSettings();
    bindSiteAnnouncements();
    bindRoutingSettings();
    bindTokenLoanSettings();
  }

  window.ProxyLanAdminSettings = { load: load, bind: bind, loadRegistrationSettings: loadRegistrationSettings };
}());
