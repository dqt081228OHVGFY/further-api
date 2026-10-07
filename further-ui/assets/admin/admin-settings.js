(function () {
  'use strict';
  const api = window.FurtherUI;

  function updatePresetButtons(form) {
    const busy = form.dataset.settingsLoaded !== 'true' || form.dataset.siteMutation === 'true' || form.dataset.presetBusy === 'true';
    form.querySelectorAll('[data-settings-preset]').forEach(function (button) { button.disabled = busy; });
  }

  function mountFormPresets(form, entries) {
    if (!form || form.querySelector('[data-form-presets]')) return;
    const card = document.createElement('section'); card.className = 'md-card'; card.dataset.formPresets = '';
    const title = document.createElement('h3'); title.className = 'md-card-title'; title.textContent = '一键填写';
    const hint = document.createElement('p'); hint.className = 'field-hint'; hint.textContent = '按场景填写，检查表单后保存。';
    const choices = document.createElement('div'); choices.className = 'form-grid two';
    const status = document.createElement('p'); status.className = 'field-hint'; status.setAttribute('role', 'status');
    const undo = document.createElement('button'); undo.type = 'button'; undo.className = 'md-btn md-btn-text'; undo.textContent = '撤销本次填写'; undo.hidden = true; undo.dataset.settingsPreset = 'undo';
    let previous = null;
    const setValues = function (values) {
      Object.keys(values).forEach(function (key) {
        const field = form.elements.namedItem(key);
        if (!field || field.type === 'password') return;
        if (field.type === 'checkbox') field.checked = Boolean(values[key]); else field.value = String(values[key]);
        field.dispatchEvent(new Event('input', { bubbles:true }));
        field.dispatchEvent(new Event('change', { bubbles:true }));
      });
    };
    entries.forEach(function (entry) {
      const choice = document.createElement('div');
      const button = document.createElement('button'); button.type = 'button'; button.className = 'md-btn md-btn-outlined'; button.textContent = entry.label; button.dataset.settingsPreset = entry.id;
      const description = document.createElement('p'); description.className = 'field-hint'; description.textContent = entry.description;
      button.addEventListener('click', function () {
        if (button.disabled) return;
        try {
          const values = typeof entry.values === 'function' ? entry.values() : entry.values;
          previous = {};
          Object.keys(values).forEach(function (key) { const field = form.elements.namedItem(key); if (field && field.type !== 'password') previous[key] = field.type === 'checkbox' ? field.checked : field.value; });
          setValues(values); status.textContent = '已填写“' + entry.label + '”，检查后保存即可生效。'; undo.hidden = false;
        } catch (error) { status.textContent = error.message; api.toast(error.message, 'danger'); }
      });
      choice.append(button, description); choices.append(choice);
    });
    undo.addEventListener('click', function () { if (undo.disabled || !previous) return; setValues(previous); previous = null; undo.hidden = true; status.textContent = '已撤销本次填写。'; });
    card.append(title, hint, choices, status, undo); form.prepend(card); updatePresetButtons(form);
  }

	const settingsPages = [ 'site', 'announcements', 'routing', 'registration', 'system-info', 'passkey', 'payment', 'alerts'];
	const settingsPageTitles = { site:'品牌与登录', announcements:'站点公告', routing:'会话与重试', registration:'注册与准入', 'system-info':'运行监控', passkey:'通行密钥', payment:'支付网关', alerts:'监控与告警' };

  function knownSettingsPage(name) {
    return typeof name === 'string' && /^[a-z0-9-]+$/.test(name) && (settingsPages.includes(name) || Boolean(document.querySelector('[data-panel="settings"] [data-settings-subpage="' + name + '"]')));
  }

	function requestedSettingsPage() {
	  const hashParts = location.hash.slice(1).split('/');
	  if (hashParts[0] === 'settings' && ['group-pricing', 'model-pricing'].includes(hashParts[1])) return '';
	  return hashParts[0] === 'settings' && knownSettingsPage(hashParts[1]) ? hashParts[1] : '';
	}

	function showSettingsPage(name, updateLocation) {
	  if (name === 'group-pricing' || name === 'model-pricing') {
        history.replaceState(history.state, '', '#models');
        document.querySelector('.nav-rail [data-nav="models"]')?.click();
        return;
      }
	  const pageName = knownSettingsPage(name) ? name : '';
	  const directory = document.querySelector('[data-settings-directory]');
      const panel = directory?.closest('[data-panel]');
      const options = { skipMotion: document.documentElement.dataset.shellReady !== 'true' || panel?.classList.contains('rf-entering') };
      const reveal = function (element, visible, apply) { if (api.transitionVisibility) api.transitionVisibility(element, visible, apply, options); else apply(visible); };
	  if (directory) reveal(directory, !pageName, function (visible) { directory.hidden = !visible; });
      const overviewHeading = document.querySelector('[data-settings-overview-heading]');
      if (overviewHeading) overviewHeading.hidden = Boolean(pageName);
	  document.querySelectorAll('[data-settings-subpage]').forEach(function (page) {
		const active = page.dataset.settingsSubpage === pageName;
        reveal(page, active, function (visible) { page.hidden = !visible; page.classList.toggle('active', visible); });
	  });
	  if (document.querySelector('[data-panel="settings"].active')) {
        const title = pageName && (settingsPageTitles[pageName] || document.querySelector('[data-settings-subpage="' + pageName + '"]')?.dataset.settingsTitle);
        api.setText('[data-page-title]', title || '安全与设置');
        document.querySelectorAll('.nav-item[data-nav="settings"], .mobile-nav-option[data-mobile-nav="settings"]').forEach(function (nav) {
          const active = nav.dataset.openSettings ? nav.dataset.openSettings === pageName : true;
          nav.classList.toggle('active', active);
          if (active) nav.setAttribute('aria-current', 'page'); else nav.removeAttribute('aria-current');
        });
      }
	  if (updateLocation) {
		const target = '#settings' + (pageName ? '/' + pageName : '');
		if (location.hash !== target) history.pushState(null, '', target);
	  }
	  window.dispatchEvent(new CustomEvent('further:settings-subpage', { detail: { name: pageName } }));
	}

	function bindSettingsSubpages() {
	  const directory = document.querySelector('[data-settings-directory]');
	  if (!directory) return;
	  const hashParts = location.hash.slice(1).split('/');
	  if (hashParts[0] === 'settings' && ['group-pricing', 'model-pricing'].includes(hashParts[1])) {
		history.replaceState(null, '', '#models');
		hashParts[0] = 'models';
	  }
	  if (hashParts[0] === 'settings' && hashParts[1] && !knownSettingsPage(hashParts[1])) {
		history.replaceState(null, '', '#settings');
	  }
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
      window.addEventListener('hashchange', function () {
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
    const form = document.querySelector('[data-payment-settings]');
    form.dataset.settingsLoaded = String(Boolean(paymentConfig)); form.dataset.presetBusy = String(busy); updatePresetButtons(form);
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
    mountFormPresets(form, [{ id:'payment-address', label:'使用站点回调地址', description:'使用已保存的 HTTPS 站点地址生成付款回调；商户密钥、兑换比例和最低金额保留。', values:function () {
      const site = document.querySelector('[data-site-settings]');
      const address = new URL(site?.dataset.savedServerAddress || location.origin);
      if (address.protocol !== 'https:') throw new Error('请先在品牌与登录中保存 HTTPS 站点地址。');
      return { callback_base_url:address.origin };
    } }]);
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
    form.dataset.settingsLoaded = 'true'; updatePresetButtons(form);
    ['local_registration_enabled', 'linuxdo_registration_enabled', 'registration_code_required'].forEach(function (key) { if (form.elements[key]) form.elements[key].checked = Boolean(settings[key]); });
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
    form.dataset.settingsLoaded = 'true'; updatePresetButtons(form);
  }

  function bindSiteAnnouncements() {
    const form = document.querySelector('[data-site-announcements]');
    if (!form) return;
    mountFormPresets(form, [{ id:'notice-maintenance', label:'填写维护公告模板', description:'替换公告草稿为维护通知；保存前补充时间和影响范围。', values:{ public:'服务维护通知\n维护时间：【开始时间】至【结束时间】\n影响范围：【受影响的服务】\n维护完成后将恢复正常服务，感谢理解。' } }]);
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      if (/【(?:开始时间|结束时间|受影响的服务)】/.test(form.elements.public.value)) return api.toast('请先补全公告模板中的维护时间和影响范围', 'danger');
      const button = form.querySelector('button[type="submit"]');
      form.dataset.presetBusy = 'true'; updatePresetButtons(form);
      api.showBusy(button, true);
      try {
        await api.request('/api/admin/settings/announcements', { method: 'PUT', body: { public: form.elements.public.value } });
        api.toast('站点公告已更新');
        await loadSiteAnnouncements();
      } catch (error) { api.toast(error.message, 'danger'); }
      finally { api.showBusy(button, false); form.dataset.presetBusy = 'false'; updatePresetButtons(form); }
    });
  }

  function updateSiteMutationControls(form) {
    const busy = form.dataset.siteMutation === 'true';
    const loaded = form.dataset.settingsLoaded === 'true';
    const configured = form.dataset.secretConfigured === 'true';
    const enabled = form.dataset.linuxdoEnabled === 'true';
    form.querySelector('button[type="submit"]').disabled = busy || !loaded;
    updatePresetButtons(form);
    const clear = form.querySelector('[data-linuxdo-clear-secret]');
    if (clear) clear.disabled = busy || !loaded || !configured || enabled;
    const hint = form.querySelector('[data-linuxdo-clear-secret-hint]');
    if (hint) {
      if (!loaded) hint.textContent = '读取配置后可管理已保存密钥。';
      else if (!configured) hint.textContent = '当前没有已保存的密钥。';
      else if (enabled) hint.textContent = '清除前请先停用 LinuxDO 登录并保存。';
      else hint.textContent = '清除后需要重新填写有效密钥才能启用登录。';
    }
  }

  async function loadSiteSettings() {
    const form = document.querySelector('[data-site-settings]');
    if (!form) return;
    const data = await api.request('/api/admin/settings/site');
    const settings = data.settings || {};
    form.elements.system_name.value = 'further-api'; form.elements.system_name.readOnly = true;
    form.elements.logo.value = '/further-api-logo.svg?v=handshake-20261004'; form.elements.logo.readOnly = true;
    form.elements.server_address.value = settings.server_address || location.origin;
    form.dataset.savedServerAddress = settings.server_address || location.origin;
    form.elements.linuxdo_enabled.checked = Boolean(settings.linuxdo_enabled);
    form.elements.linuxdo_client_id.value = settings.linuxdo_client_id || '';
    form.elements.linuxdo_minimum_trust_level.value = String(settings.linuxdo_minimum_trust_level || 0);
    form.dataset.secretConfigured = String(Boolean(settings.linuxdo_client_secret_configured));
    form.dataset.linuxdoEnabled = String(Boolean(settings.linuxdo_enabled));
    form.dataset.settingsLoaded = 'true';
    updateSiteMutationControls(form);
    const secret = document.querySelector('[data-linuxdo-secret-status]');
    if (secret) secret.textContent = settings.linuxdo_client_secret_configured ? '密钥已配置，留空可保留。' : '尚未配置密钥。';
    api.setText('[data-linuxdo-callback-url]', settings.linuxdo_redirect_url || '');
    updateSitePreview(form);
    api.setText('[data-site-setup-state]', settings.linuxdo_enabled ? 'Linux DO 登录已启用' : 'Linux DO 登录未启用');
    api.setText('[data-site-setup-hint]', settings.linuxdo_enabled ? '登录入口已经开放；新用户注册权限在「注册与准入」中设置。' : '填写 Connect 应用凭据、登记回调地址并启用开关，即可开放登录入口。');
  }

  function updateSitePreview(form) {
    api.setText('[data-site-preview-name]', 'further-api');
    const image = form.querySelector('[data-site-preview-logo]');
    if (image) {
      image.src = '/further-api-logo.svg?v=handshake-20261004';
    }
  }

  function prepareSiteSettings(form) {
    if (form.querySelector('[data-site-preview-name]')) return;
    const preview = document.createElement('div');
    preview.className = 'md-card site-settings-preview';
    preview.innerHTML = '<div class="card-head"><div class="site-preview-brand"><span class="site-preview-logo"><img data-site-preview-logo src="/further-api-logo.svg?v=handshake-20261004" width="44" height="44" alt=""></span><div><div class="md-card-title" data-site-preview-name>further-api</div><p class="card-hint">further-api 使用固定品牌；服务地址和登录配置仍可编辑。</p></div></div><span class="material-symbol" aria-hidden="true">visibility</span></div><div class="site-setup-status"><span class="material-symbol" aria-hidden="true">passkey</span><div><b data-site-setup-state>读取站点配置…</b><p class="card-hint" data-site-setup-hint>站点配置保存在服务器。</p></div></div>';
    form.prepend(preview);
    preview.querySelector('img').addEventListener('error', function (event) {
      if (event.target.getAttribute('src') !== '/further-api-logo.svg?v=handshake-20261004') event.target.src = '/further-api-logo.svg?v=handshake-20261004';
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
    mountFormPresets(form, [
      { id:'site-origin', label:'使用当前访问地址', description:'将当前浏览器访问的站点来源填入站点地址，登录凭据保留。', values:function () { return { server_address:location.origin }; } },
      { id:'linuxdo-verified', label:'LinuxDO 已验证账户', description:'启用 LinuxDO 登录，最低信任等级设为 1；需先填写或保存应用凭据。', values:function () {
        if (!form.elements.linuxdo_client_id.value.trim() || (!form.elements.linuxdo_client_secret.value.trim() && form.dataset.secretConfigured !== 'true')) throw new Error('请先填写 LinuxDO Client ID 和 Client Secret。');
        return { linuxdo_enabled:true, linuxdo_minimum_trust_level:'1' };
      } }
    ]);
    const clear = form.querySelector('[data-linuxdo-clear-secret]');
    if (clear) clear.addEventListener('click', async function () {
      if (clear.disabled || form.dataset.siteMutation === 'true') return;
      form.dataset.siteMutation = 'true';
      updateSiteMutationControls(form);
      api.showBusy(clear, true);
      try {
        if (!await api.confirm('清除后需要重新配置密钥才能再次开放 LinuxDO 登录。是否继续？', { title: '清除 LinuxDO 应用密钥', confirmText: '清除密钥' })) return;
        await api.request('/api/option/', { method: 'PUT', body: { key: 'LinuxDOClientSecret', value: '' } });
        form.elements.linuxdo_client_secret.value = '';
        await loadSiteSettings();
        if (form.dataset.secretConfigured === 'true') throw new Error('服务尚未确认密钥已清除，请重新读取后核对。');
        api.toast('LinuxDO 应用密钥已清除');
      } catch (error) { api.toast(error.message, 'danger'); }
      finally { delete form.dataset.siteMutation; api.showBusy(clear, false); updateSiteMutationControls(form); }
    });
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      if (form.dataset.siteMutation === 'true') return;
      if (form.dataset.settingsLoaded !== 'true') return api.toast('正在读取站点配置，请稍后再保存', 'danger');
      if (form.elements.linuxdo_enabled.checked && (!form.elements.linuxdo_client_id.value.trim() || (!form.elements.linuxdo_client_secret.value.trim() && form.dataset.secretConfigured !== 'true'))) {
        api.toast('启用 Linux DO 登录前，请填写 Client ID 和 Client Secret', 'danger');
        form.elements[!form.elements.linuxdo_client_id.value.trim() ? 'linuxdo_client_id' : 'linuxdo_client_secret'].focus();
        return;
      }
      const button = form.querySelector('button[type="submit"]');
      form.dataset.siteMutation = 'true';
      updateSiteMutationControls(form);
      api.showBusy(button, true);
      try {
        await api.request('/api/admin/settings/site', { method: 'PUT', body: {
          system_name: 'further-api',
          logo: '/further-api-logo.svg?v=handshake-20261004',
          server_address: form.elements.server_address.value.trim(),
          linuxdo_enabled: form.elements.linuxdo_enabled.checked,
          linuxdo_client_id: form.elements.linuxdo_client_id.value.trim(),
          linuxdo_client_secret: form.elements.linuxdo_client_secret.value.trim(),
          linuxdo_minimum_trust_level: Number(form.elements.linuxdo_minimum_trust_level.value || 0)
        } });
        form.elements.linuxdo_client_secret.value = '';
        api.toast('站点与 LinuxDO 设置已更新');
        await Promise.all([loadSiteSettings(), loadRegistrationSettings()]);
        const status = { system_name: 'further-api', logo: '/further-api-logo.svg?v=handshake-20261004' };
        if (window.FurtherUISite) window.FurtherUISite.applyContext(status);
        else api.setText('.rail-title, .public-brand strong, .auth-brand strong, [data-site-name]', status.system_name);
        document.title = '管理页面 · further-api';
      } catch (error) { api.toast(error.message, 'danger'); }
      finally { delete form.dataset.siteMutation; api.showBusy(button, false); updateSiteMutationControls(form); }
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









  function bindRegistrationSettings() {
    const form = document.querySelector('[data-registration-settings]');
    if (!form) return;
    mountFormPresets(form, [
      { id:'registration-open', label:'开放本地注册', description:'开放本地账户注册并取消邀请码要求；新用户余额和 LinuxDO 注册开关保留。', values:{ local_registration_enabled:true, registration_code_required:false } },
      { id:'registration-invite', label:'邀请码准入', description:'开放本地账户注册，新用户需要有效邀请码；请先在注册码页面生成邀请码。余额保留。', values:{ local_registration_enabled:true, registration_code_required:true } },
      { id:'registration-closed', label:'暂停新用户注册', description:'关闭本地和 LinuxDO 新用户注册，已有账户仍可登录。', values:{ local_registration_enabled:false, linuxdo_registration_enabled:false } }
    ]);
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      const button = form.querySelector('button[type="submit"]');
      form.dataset.presetBusy = 'true'; updatePresetButtons(form);
      api.showBusy(button, true);
      try {
        await api.request('/api/admin/settings/registration', { method: 'PATCH', body: {
          local_registration_enabled: form.elements.local_registration_enabled.checked,
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
      finally { api.showBusy(button, false); form.dataset.presetBusy = 'false'; updatePresetButtons(form); }
    });
  }


  // load 供 admin.js 引导时统一加载；loadRegistrationSettings 另外暴露，
  // 因为注册码增删后需要单独刷新注册设置里的余量显示。
  async function load() {
    const results = await Promise.allSettled([
      loadRegistrationSettings(), loadSiteSettings(), loadSiteAnnouncements(),
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
    bindTokenLoanSettings();
  }

  window.FurtherUIAdminSettings = { load: load, bind: bind, loadRegistrationSettings: loadRegistrationSettings };
}());
