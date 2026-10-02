(function () {
  'use strict';

  const TOKEN_KEY = 'topublic_proxy_token';
	const ADMIN_PROOF_KEY = 'relayfor_admin_proof';
  const PREFERENCES_KEY = 'relayfor_user_preferences';
	const ADMIN_LIST_PREFERENCES_KEY = 'relayfor_admin_list_preferences';
  const DEFAULT_PREFERENCES = { theme: 'system', reducedMotion: false, defaultPage: 'overview', pageSize: 25, timezone: 'local', hideLockedModels: false, sidebarCollapsed: false, hourCycle: '24', showSeconds: false, hideSuccessfulLogs: false };
  // 单例 promise：同一时刻多个 sensitive 请求同时撞上 step_up_required 时，只弹一次窗
  let verificationPromise = null;
  let verificationResolve = null;
  function userToken() { return localStorage.getItem(TOKEN_KEY) || ''; }
  function setToken(token) { if (token) localStorage.setItem(TOKEN_KEY, token); }
  function clearToken() { localStorage.removeItem(TOKEN_KEY); }
  function preferences() {
    try { return Object.assign({}, DEFAULT_PREFERENCES, JSON.parse(localStorage.getItem(PREFERENCES_KEY) || '{}')); }
    catch (_) { return Object.assign({}, DEFAULT_PREFERENCES); }
  }
  function savePreferences(value) {
    const next = Object.assign({}, DEFAULT_PREFERENCES, value || {});
    localStorage.setItem(PREFERENCES_KEY, JSON.stringify(next));
    applyPreferences(next);
	window.dispatchEvent(new CustomEvent('proxylan:preferences', { detail: next }));
    return next;
  }
	function adminListPreferences() {
	  try {
		const stored = JSON.parse(localStorage.getItem(ADMIN_LIST_PREFERENCES_KEY) || '{}');
		return stored && typeof stored === 'object' && !Array.isArray(stored) ? stored : {};
	  } catch (_) { return {}; }
	}
	function adminListPreference(name) {
	  return adminListPreferences()[name] === true;
	}
	function saveAdminListPreference(name, value) {
	  const stored = adminListPreferences();
	  stored[name] = Boolean(value);
	  try { localStorage.setItem(ADMIN_LIST_PREFERENCES_KEY, JSON.stringify(stored)); } catch (_) {}
	  return stored[name];
	}
  function applyPreferences(value) {
    const next = Object.assign({}, DEFAULT_PREFERENCES, value || {});
    if (next.theme === 'light' || next.theme === 'dark') document.documentElement.dataset.theme = next.theme;
    else document.documentElement.removeAttribute('data-theme');
    document.documentElement.dataset.reducedMotion = next.reducedMotion ? 'true' : 'false';
    document.documentElement.dataset.railCollapsed = next.sidebarCollapsed ? 'true' : 'false';
  }
  applyPreferences(preferences());
  const MATERIAL_RIPPLE_SELECTOR = [
    '.md-btn',
    '.md-icon-button',
    '.nav-item',
    '.settings-directory-item',
    '.list-item-button',
    '.mobile-nav-option',
    '.docs-tab',
    '.trend-period-switch button',
    '.code-batch-toggle',
    '.list-visibility-toggle'
  ].join(',');
  function materialRippleHost(target) {
    if (!(target instanceof Element)) return null;
    const host = target.closest(MATERIAL_RIPPLE_SELECTOR);
    if (!host || host.matches(':disabled, [aria-disabled="true"]')) return null;
    return host;
  }
  function materialRippleReduced() {
    return document.documentElement.dataset.reducedMotion === 'true' || (window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);
  }
  function createMaterialRipple(host, clientX, clientY) {
    if (!host || materialRippleReduced()) return;
    const bounds = host.getBoundingClientRect();
    if (bounds.width <= 0 || bounds.height <= 0) return;
    const x = Math.min(Math.max(clientX - bounds.left, 0), bounds.width);
    const y = Math.min(Math.max(clientY - bounds.top, 0), bounds.height);
    const radius = Math.hypot(Math.max(x, bounds.width - x), Math.max(y, bounds.height - y));
    const size = Math.max(24, radius * 2);
    const ripple = document.createElement('span');
    ripple.className = 'md-ripple';
    ripple.setAttribute('aria-hidden', 'true');
    ripple.style.width = size + 'px';
    ripple.style.height = size + 'px';
    ripple.style.left = (x - size / 2) + 'px';
    ripple.style.top = (y - size / 2) + 'px';
    host.classList.add('md-ripple-host');
    host.appendChild(ripple);
    let removed = false;
    function removeRipple() {
      if (removed) return;
      removed = true;
      ripple.remove();
    }
    ripple.addEventListener('animationend', removeRipple, { once: true });
    window.setTimeout(removeRipple, 700);
  }
  function mountMaterialRipples() {
    document.addEventListener('pointerdown', function (event) {
      if (event.button !== 0 || event.isPrimary === false) return;
      createMaterialRipple(materialRippleHost(event.target), event.clientX, event.clientY);
    });
    document.addEventListener('click', function (event) {
      if (event.detail !== 0) return;
      const host = materialRippleHost(event.target);
      if (!host) return;
      const bounds = host.getBoundingClientRect();
      createMaterialRipple(host, bounds.left + bounds.width / 2, bounds.top + bounds.height / 2);
    }, true);
  }
  function setText(selector, value) { document.querySelectorAll(selector).forEach(function (element) { element.textContent = value; }); }
  function escapeHTML(value) {
    return String(value == null ? '' : value).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#039;');
  }
  function number(value) { return Number(value || 0).toLocaleString('zh-CN'); }
  const QUOTA_PER_USD = 500000;
  function moneyFromQuota(value, digits) { return '$' + (Number(value || 0) / QUOTA_PER_USD).toFixed(digits == null ? 2 : digits); }
  function quotaFromUSD(value) {
    const amount = Number(value);
    if (!Number.isFinite(amount)) return '';
    if (amount < 0) return -1;
    return Math.round(amount * QUOTA_PER_USD);
  }
  function usdFromQuota(value) {
    const quota = Number(value);
    if (!Number.isFinite(quota)) return '';
    if (quota < 0) return '-1';
    return (quota / QUOTA_PER_USD).toFixed(6).replace(/\.?0+$/, '') || '0';
  }
  function roundQuotaToUSDCent(value) {
    const quota = Number(value);
    if (!Number.isFinite(quota) || quota <= 0) return 0;
    const quotaPerCent = QUOTA_PER_USD / 100;
    return Math.round(quota / quotaPerCent) * quotaPerCent;
  }
  function quotaInputFor(usdInput) {
    const form = usdInput.closest('form');
    const name = usdInput.dataset.quotaUsdFor;
    return form && form.elements ? form.elements[name] : null;
  }
  function syncQuotaUSD(root) {
    (root || document).querySelectorAll('[data-quota-usd-for]').forEach(function (usdInput) {
      const quotaInput = quotaInputFor(usdInput);
      if (quotaInput) usdInput.value = usdFromQuota(quotaInput.value);
    });
  }
  function bindQuotaUSDFields(root) {
    (root || document).querySelectorAll('[data-quota-usd-for]').forEach(function (usdInput) {
      const quotaInput = quotaInputFor(usdInput);
      if (!quotaInput || usdInput.dataset.quotaUsdBound === 'true') return;
      usdInput.dataset.quotaUsdBound = 'true';
      quotaInput.addEventListener('input', function () { usdInput.value = usdFromQuota(quotaInput.value); });
      usdInput.addEventListener('input', function () { quotaInput.value = quotaFromUSD(usdInput.value); });
      usdInput.value = usdFromQuota(quotaInput.value);
    });
  }
  function date(value) {
    if (!value) return '暂无';
    const parsed = new Date(value);
    if (Number.isNaN(parsed.getTime())) return String(value);
	const currentPreferences = preferences();
	const options = { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: currentPreferences.hourCycle === '12' };
	if (currentPreferences.showSeconds) options.second = '2-digit';
	const timezone = currentPreferences.timezone;
    if (timezone && timezone !== 'local') options.timeZone = timezone;
    try { return parsed.toLocaleString('zh-CN', options); }
    catch (_) { delete options.timeZone; return parsed.toLocaleString('zh-CN', options); }
  }
  function statusLabel(status, kind) {
    const map = kind === 'channel'
      ? { 1: ['启用', 'success'], 2: ['停用', 'muted'], 3: ['自动下线', 'error'] }
      : { 1: ['正常', 'success'], 2: ['停用', 'muted'], 3: ['过期', 'error'], 4: ['额度耗尽', 'error'] };
    const item = map[Number(status)] || ['未知', 'muted'];
    return '<span class="tag ' + item[1] + '">' + item[0] + '</span>';
  }

  async function request(path, options) {
	const originalOptions = Object.assign({}, options || {});
    const init = Object.assign({}, options || {});
	const stepUpAttempt = Boolean(init._stepUpAttempt);
	const suppressAuthRedirect = Boolean(init._suppressAuthRedirect);
	delete init._stepUpAttempt;
	delete init._suppressAuthRedirect;
    const headers = new Headers(init.headers || {});
    if (init.body && typeof init.body !== 'string') {
      headers.set('Content-Type', 'application/json');
      init.body = JSON.stringify(init.body);
    }
    const token = userToken();
    if (token) headers.set('Authorization', 'Bearer ' + token);
	if (path.indexOf('/api/admin') === 0) {
	  try {
		const stored = JSON.parse(sessionStorage.getItem(ADMIN_PROOF_KEY) || 'null');
		if (stored && stored.proof && new Date(stored.expires_at).getTime() > Date.now()) headers.set('X-Admin-Verification', stored.proof);
	  } catch (_) { sessionStorage.removeItem(ADMIN_PROOF_KEY); }
	}
    headers.set('Accept', 'application/json');
    init.headers = headers;
    const response = await fetch(path, init);
    const text = await response.text();
    let data = null;
    if (text) {
      try { data = JSON.parse(text); } catch (_) { data = { raw: text }; }
    }
    if (response.status === 401 && location.pathname !== '/auth/' && location.pathname !== '/auth') {
      clearToken();
	  if (!suppressAuthRedirect) location.href = '/auth/?mode=login';
    }
	if (!response.ok) {
	  const error = data && data.error;
	  const errorType = typeof error === 'object' && error && error.type;
	  if (errorType === 'commercial_account_required' && !suppressAuthRedirect && location.pathname !== '/auth/' && location.pathname !== '/auth') {
		clearToken();
		location.href = '/auth/?mode=register&site_error=' + encodeURIComponent(error.message || '公益站账户无法使用商业站');
	  }
	  if (errorType === 'step_up_required' && !stepUpAttempt) {
		const verified = await ensureAdminVerification();
		if (verified) return request(path, Object.assign({}, originalOptions, { _stepUpAttempt: true }));
	  }
      const message = typeof error === 'string' ? error : (error && error.message) || '请求失败';
      const err = new Error(message);
      err.status = response.status;
      err.payload = data;
      throw err;
    }
    return data;
  }

	function setAdminProof(proof, expiresAt) {
	  if (!proof) return;
	  sessionStorage.setItem(ADMIN_PROOF_KEY, JSON.stringify({ proof: proof, expires_at: expiresAt }));
	}

	function clearAdminProof() { sessionStorage.removeItem(ADMIN_PROOF_KEY); }

  function showBusy(button, busy) {
    if (!button) return;
    if (busy && button.dataset.busy !== 'true') {
      button.dataset.previousHTML = button.innerHTML;
      button.dataset.busyIconOnly = String(button.classList.contains('md-icon-button'));
      button.style.minWidth = Math.ceil(button.getBoundingClientRect().width) + 'px';
      button.replaceChildren();
      if (button.dataset.busyIconOnly !== 'true') button.textContent = '处理中…';
    }
    button.disabled = busy;
    button.dataset.busy = String(busy);
    button.setAttribute('aria-busy', String(busy));
    if (!busy) {
      if (button.dataset.previousHTML != null) button.innerHTML = button.dataset.previousHTML;
      button.style.removeProperty('min-width');
      delete button.dataset.previousHTML;
      delete button.dataset.busyIconOnly;
      delete button.dataset.busy;
      button.removeAttribute('aria-busy');
    }
  }
  function toast(message, tone) {
    let snackbar = document.querySelector('.snackbar');
    if (!snackbar) {
      snackbar = document.createElement('div');
      snackbar.className = 'snackbar';
      document.body.appendChild(snackbar);
    }
    snackbar.setAttribute('role', tone === 'danger' ? 'alert' : 'status');
    snackbar.setAttribute('aria-live', tone === 'danger' ? 'assertive' : 'polite');
    snackbar.textContent = message;
    snackbar.classList.toggle('error', tone === 'danger');
    snackbar.classList.remove('hide');
    window.requestAnimationFrame(function () { snackbar.classList.add('show'); });
    window.clearTimeout(snackbar._timer);
    snackbar._timer = window.setTimeout(function () {
      snackbar.classList.remove('show');
      snackbar.classList.add('hide');
    }, 3000);
  }
  function focusableElements(container) {
    return Array.from(container.querySelectorAll('button:not(:disabled), [href], input:not(:disabled), select:not(:disabled), textarea:not(:disabled), [tabindex]:not([tabindex="-1"])')).filter(function (element) {
      return !element.closest('.hidden') && element.getClientRects().length > 0;
    });
  }
  function openDialog(id) {
    const dialog = document.getElementById(id);
    if (!dialog) return;
    window.clearTimeout(dialog._closeTimer);
    dialog._returnFocus = document.activeElement;
    dialog.removeAttribute('inert');
    dialog.classList.remove('closing');
    dialog.classList.add('show');
    dialog.setAttribute('aria-hidden', 'false');
    window.requestAnimationFrame(function () {
      const focusTarget = focusableElements(dialog)[0] || dialog.querySelector('.md-dialog');
      if (focusTarget) focusTarget.focus({ preventScroll: true });
    });
  }
  function closeDialog(id) {
    const dialog = document.getElementById(id);
    if (!dialog || !dialog.classList.contains('show') || dialog.classList.contains('closing')) return;
    const onClose = dialog._onClose;
    delete dialog._onClose;
    delete dialog._onAccept;
    if (typeof onClose === 'function') onClose();
    dialog.classList.add('closing');
    dialog.setAttribute('inert', '');
    if (!document.querySelector('.dialog-backdrop.show:not(.closing)') && dialog._returnFocus && document.contains(dialog._returnFocus)) {
      dialog._returnFocus.focus({ preventScroll: true });
    }
    const finish = function () {
      dialog.classList.remove('show', 'closing');
      dialog.removeAttribute('inert');
      dialog.setAttribute('aria-hidden', 'true');
    };
    const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    dialog._closeTimer = window.setTimeout(finish, reducedMotion ? 0 : 200);
  }
  function ensureConfirmDialog() {
    let dialog = document.getElementById('confirm-dialog');
    if (dialog) return dialog;
    dialog = document.createElement('div');
    dialog.id = 'confirm-dialog';
    dialog.className = 'dialog-backdrop';
    dialog.innerHTML = '<div class="md-dialog confirm-dialog" aria-describedby="confirm-dialog-message"><div class="md-dialog-title" id="confirm-dialog-title">确认操作</div><p class="confirm-dialog-message" id="confirm-dialog-message"></p><div class="form-actions"><button type="button" class="md-btn md-btn-text" data-confirm-cancel>取消</button><button type="button" class="md-btn md-btn-danger" data-confirm-accept>确认</button></div></div>';
    document.body.appendChild(dialog);
    dialog.addEventListener('click', function (event) { if (event.target === dialog) closeDialog(dialog.id); });
    dialog.addEventListener('keydown', function (event) {
      if (event.key === 'Escape') { event.preventDefault(); closeDialog(dialog.id); return; }
      if (event.key !== 'Tab') return;
      const items = focusableElements(dialog);
      if (!items.length) { event.preventDefault(); return; }
      const first = items[0];
      const last = items[items.length - 1];
      if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus(); }
      else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); }
    });
    dialog.querySelector('[data-confirm-cancel]').addEventListener('click', function () { closeDialog(dialog.id); });
    dialog.querySelector('[data-confirm-accept]').addEventListener('click', function () {
      const onAccept = dialog._onAccept;
      delete dialog._onAccept;
      delete dialog._onClose;
      if (typeof onAccept === 'function') onAccept();
      closeDialog(dialog.id);
    });
    return dialog;
  }
  function confirmAction(message, options) {
    const dialog = ensureConfirmDialog();
    const settings = options || {};
    dialog.querySelector('.md-dialog-title').textContent = settings.title || '确认操作';
    dialog.querySelector('.confirm-dialog-message').textContent = message;
    dialog.querySelector('[data-confirm-cancel]').textContent = settings.cancelText || '取消';
    dialog.querySelector('[data-confirm-accept]').textContent = settings.confirmText || '确认';
    return new Promise(function (resolve) {
      let settled = false;
      const settle = function (value) {
        if (settled) return;
        settled = true;
        resolve(value);
      };
      dialog._onClose = function () { settle(false); };
      dialog._onAccept = function () { settle(true); };
      openDialog(dialog.id);
    });
  }

  function ensureTOTPDialog() {
    let dialog = document.getElementById('totp-verification-dialog');
    if (dialog) return dialog;
    dialog = document.createElement('div');
    dialog.id = 'totp-verification-dialog';
    dialog.className = 'dialog-backdrop';
    dialog.innerHTML = '<div class="md-dialog" role="dialog" aria-modal="true" aria-labelledby="totp-verification-title" tabindex="-1"><div class="md-dialog-title" id="totp-verification-title">动态验证</div><p class="confirm-dialog-message" data-totp-message>请输入身份验证器中的 6 位动态验证码。</p><form data-totp-verification-form><div class="md-textfield"><input class="md-input" name="code" inputmode="numeric" pattern="[0-9]{6}" minlength="6" maxlength="6" autocomplete="one-time-code" placeholder=" " required><label class="md-input-label">6 位动态验证码</label></div><div class="form-actions"><button type="button" class="md-btn md-btn-text" data-totp-cancel>取消</button><button type="submit" class="md-btn md-btn-filled" data-totp-accept>验证</button></div></form></div>';
    document.body.appendChild(dialog);
    dialog.addEventListener('click', function (event) { if (event.target === dialog) closeDialog(dialog.id); });
    dialog.addEventListener('keydown', function (event) {
      if (event.key === 'Escape') { event.preventDefault(); closeDialog(dialog.id); return; }
      if (event.key !== 'Tab') return;
      const items = focusableElements(dialog);
      if (!items.length) { event.preventDefault(); return; }
      const first = items[0];
      const last = items[items.length - 1];
      if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus(); }
      else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); }
    });
    dialog.querySelector('[data-totp-cancel]').addEventListener('click', function () { closeDialog(dialog.id); });
    dialog.querySelector('[data-totp-verification-form]').addEventListener('submit', function (event) {
      event.preventDefault();
      const code = event.currentTarget.elements.code.value.trim();
      if (!/^\d{6}$/.test(code)) return toast('请输入 6 位动态验证码', 'danger');
      const onAccept = dialog._onAccept;
      delete dialog._onAccept;
      delete dialog._onClose;
      if (typeof onAccept === 'function') onAccept(code);
      closeDialog(dialog.id);
    });
    return dialog;
  }

  function requestTOTP(options) {
    const settings = options || {};
    const dialog = ensureTOTPDialog();
    dialog.querySelector('.md-dialog-title').textContent = settings.title || '动态验证';
    dialog.querySelector('[data-totp-message]').textContent = settings.message || '请输入身份验证器中的 6 位动态验证码。';
    dialog.querySelector('[data-totp-accept]').textContent = settings.confirmText || '验证';
    const form = dialog.querySelector('[data-totp-verification-form]');
    form.reset();
    return new Promise(function (resolve) {
      let settled = false;
      const settle = function (value) {
        if (settled) return;
        settled = true;
        resolve(value);
      };
      dialog._onClose = function () { settle(''); };
      dialog._onAccept = function (code) { settle(code); };
      openDialog(dialog.id);
    });
  }
  // 二次验证弹窗放在 app.js 而不是 admin.js：触发它的 step_up_required 重试逻辑就在
  // request() 里，两者必须同生共死。之前拆在 admin.js 时，任何不加载 admin.js 的独立
  // 管理页（如渠道 Key 管理）都会静默拿不到弹窗，而且编译期毫无察觉。
  function ensureAdminVerificationDialog() {
    let dialog = document.getElementById('admin-verification-dialog');
    if (dialog) return dialog;
    dialog = document.createElement('div');
    dialog.id = 'admin-verification-dialog';
    dialog.className = 'dialog-backdrop';
    dialog.innerHTML = '<div class="md-dialog" role="dialog" aria-modal="true" aria-labelledby="admin-verification-title" tabindex="-1"><div class="md-dialog-title" id="admin-verification-title">管理员二次验证</div><p class="dialog-copy" data-admin-verify-message>验证通过后，5 分钟内可执行敏感操作。</p><form data-admin-verification-form><div class="md-textfield" data-admin-verify-totp><input class="md-input" name="code" inputmode="numeric" autocomplete="one-time-code" pattern="[0-9]{6}" maxlength="6" placeholder=" "><label class="md-input-label">6 位动态验证码</label></div><div class="md-textfield hidden" data-admin-verify-password><input class="md-input" name="password" type="password" autocomplete="current-password" placeholder=" "><label class="md-input-label">当前登录密码</label></div><div class="form-actions"><button type="button" class="md-btn md-btn-text" data-admin-verify-cancel>取消</button><button class="md-btn md-btn-filled" type="submit">验证</button></div></form></div>';
    document.body.appendChild(dialog);
    // 动态创建晚于 initDialogDismiss/initDialogControls，关闭与焦点陷阱只能自己接上
    dialog.addEventListener('click', function (event) { if (event.target === dialog) closeDialog(dialog.id); });
    dialog.addEventListener('keydown', function (event) {
      if (event.key === 'Escape') { event.preventDefault(); closeDialog(dialog.id); return; }
      if (event.key !== 'Tab') return;
      const items = focusableElements(dialog);
      if (!items.length) { event.preventDefault(); return; }
      const first = items[0];
      const last = items[items.length - 1];
      if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus(); }
      else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); }
    });
    dialog.querySelector('[data-admin-verify-cancel]').addEventListener('click', function () { closeDialog(dialog.id); });
    const form = dialog.querySelector('[data-admin-verification-form]');
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      const button = form.querySelector('[type="submit"]');
      showBusy(button, true);
      try {
        // 标记 _stepUpAttempt 是防御性的：verify 端点本身不是 sensitive 级，但万一它
        // 哪天返回 step_up_required，request() 会等一个已 pending 的 verificationPromise，死锁
        const result = await request('/api/admin/security/verify', { method: 'POST', _stepUpAttempt: true, body: { code: form.elements.code.value.trim(), password: form.elements.password.value } });
        setAdminProof(result.proof, result.expires_at);
        dialog._onClose = null;
        closeDialog(dialog.id);
        finishAdminVerification(true);
      } catch (error) { toast(error.message, 'danger'); }
      finally { showBusy(button, false); }
    });
    return dialog;
  }

  function finishAdminVerification(result) {
    const resolve = verificationResolve;
    verificationResolve = null;
    verificationPromise = null;
    if (resolve) resolve(Boolean(result));
  }

  function ensureAdminVerification() {
    if (verificationPromise) return verificationPromise;
    verificationPromise = new Promise(function (resolve) { verificationResolve = resolve; });
    request('/api/admin/security').then(function (status) {
      const usesTOTP = Boolean(status && status.totp_enabled);
      const dialog = ensureAdminVerificationDialog();
      dialog.querySelector('[data-admin-verify-totp]').classList.toggle('hidden', !usesTOTP);
      dialog.querySelector('[data-admin-verify-password]').classList.toggle('hidden', usesTOTP);
      const form = dialog.querySelector('[data-admin-verification-form]');
      form.reset();
      form.elements.code.required = usesTOTP;
      form.elements.password.required = !usesTOTP;
      dialog.querySelector('[data-admin-verify-message]').textContent = usesTOTP ? '输入身份验证器生成的 6 位验证码。验证通过后 5 分钟内可执行敏感操作。' : '输入当前登录密码。建议在“安全与设置”中启用动态验证。';
      dialog._onClose = function () { finishAdminVerification(false); };
      openDialog(dialog.id);
    }).catch(function (error) {
      toast(error.message, 'danger');
      finishAdminVerification(false);
    });
    return verificationPromise;
  }

  function initDialogDismiss() {
    document.querySelectorAll('.dialog-backdrop').forEach(function (dialog) {
      dialog.setAttribute('aria-hidden', 'true');
      const surface = dialog.querySelector('.md-dialog');
      if (surface) {
        surface.setAttribute('role', 'dialog');
        surface.setAttribute('aria-modal', 'true');
        surface.setAttribute('tabindex', '-1');
        const title = surface.querySelector('.md-dialog-title');
        if (title) {
          if (!title.id) title.id = dialog.id + '-title';
          surface.setAttribute('aria-labelledby', title.id);
        }
      }
      dialog.addEventListener('click', function (event) { if (event.target === dialog) closeDialog(dialog.id); });
    });
    document.addEventListener('keydown', function (event) {
      const openDialogs = Array.from(document.querySelectorAll('.dialog-backdrop.show:not(.closing)'));
      const dialog = openDialogs[openDialogs.length - 1];
      if (!dialog) return;
      if (event.key === 'Escape') {
        event.preventDefault();
        closeDialog(dialog.id);
        return;
      }
      if (event.key === 'Tab') {
        const items = focusableElements(dialog);
        if (!items.length) { event.preventDefault(); dialog.querySelector('.md-dialog')?.focus(); return; }
        const first = items[0];
        const last = items[items.length - 1];
        if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus(); }
        else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); }
      }
    });
  }
  function initDialogControls() {
    document.querySelectorAll('[data-open-dialog]').forEach(function (button) {
      button.addEventListener('click', function () { openDialog(button.dataset.openDialog); });
    });
    document.querySelectorAll('[data-dialog-close]').forEach(function (button) {
      button.addEventListener('click', function () {
        const dialog = button.closest('.dialog-backdrop');
        if (dialog) closeDialog(dialog.id);
      });
    });
  }
  function goToRole(user) { location.href = Number(user && user.role) >= 25 ? '/admin/' : '/console/'; }

  async function loadAuthConfig() {
    return window.ProxyLanSite.loadAuthConfig();
  }

  function setVisible(element, visible) {
    if (element) element.classList.toggle('hidden', !visible);
  }

  function applySiteContext(config) {
    return window.ProxyLanSite.applyContext(config);
  }

  async function loadSiteAnnouncement() {
    return window.ProxyLanSite.loadAnnouncement();
  }

  function apiErrorType(error) {
    return error && error.payload && error.payload.error && error.payload.error.type || '';
  }

  async function requestLogin(path, body) {
    let totpCode = '';
    while (true) {
      try {
        return await request(path, { method: 'POST', body: Object.assign({}, body, { totp_code: totpCode }) });
      } catch (error) {
        const type = apiErrorType(error);
        if (type !== 'totp_required' && type !== 'totp_invalid') throw error;
        if (type === 'totp_invalid') toast('动态验证码无效，请重试', 'danger');
        totpCode = await requestTOTP({ title: '登录动态验证', message: '此账号已开启登录保护，请输入身份验证器中的 6 位验证码。', confirmText: '继续登录' });
        if (!totpCode) throw new Error('已取消动态验证');
      }
    }
  }

  async function completeLinuxDOTicket(ticket) {
    const data = await requestLogin('/api/auth/linuxdo/complete', { ticket: ticket });
    setToken(data.token);
    goToRole(data.user);
  }

  async function initAuth() {
    const form = document.getElementById('auth-form');
    if (!form) return;
    const mode = new URLSearchParams(location.search).get('mode') === 'register' ? 'register' : 'login';
    const register = mode === 'register';
    const query = new URLSearchParams(location.search);
	const [config] = await Promise.all([loadAuthConfig(), loadSiteAnnouncement()]);
	const isPro = applySiteContext(config);
    const title = document.getElementById('auth-title');
    const description = document.getElementById('auth-description');
    const submit = document.getElementById('auth-submit');
    const switcher = document.getElementById('auth-switch');
    const linuxdoButton = document.getElementById('linuxdo-login');
    const localFields = document.getElementById('local-auth-fields');
    const localClosed = document.getElementById('auth-local-closed');
    const codeWrap = document.getElementById('auth-registration-code-wrap');
    const codeHint = document.getElementById('auth-registration-hint');
    const oauthMessage = document.getElementById('auth-oauth-message');
    const localEnabled = !register || config.local_registration_enabled;
    const linuxdoEnabled = Boolean(config.linuxdo_login_enabled);
    const codeRequired = register && Boolean(config.registration_code_required);
	if (title) title.textContent = register ? (isPro ? '注册商业站账户' : '创建账户') : (isPro ? '登录 RelayFor 商业站' : '登录');
	if (description) description.textContent = register && isPro ? '商业站账户' : '';
    if (submit) submit.textContent = register ? '注册' : '登录';
    if (switcher) { switcher.textContent = register ? '已有账户？登录' : '创建账户'; switcher.href = '/auth/?mode=' + (register ? 'login' : 'register'); }
    setVisible(localFields, localEnabled);
    setVisible(localClosed, register && !config.local_registration_enabled);
    const codeVisible = codeRequired;
    setVisible(codeWrap, codeVisible);
    setVisible(codeHint, codeVisible);
    if (codeWrap) codeWrap.querySelector('input').required = codeRequired;
    setVisible(submit, !register || Boolean(config.local_registration_enabled));
    if (localClosed && register && !config.local_registration_enabled) {
      localClosed.textContent = '本地注册已关闭。';
    }
    if (codeHint) codeHint.textContent = '需要注册码。';
    setVisible(linuxdoButton, linuxdoEnabled && (register ? Boolean(config.linuxdo_registration_enabled) : true));
    if (register && !config.local_registration_enabled && !config.linuxdo_registration_enabled) {
      if (description) { description.textContent = '注册已关闭。'; description.classList.remove('hidden'); }
      setVisible(submit, false);
      setVisible(switcher, false);
    }
	if (query.get('oauth_error')) {
      if (oauthMessage) { oauthMessage.textContent = query.get('oauth_error'); oauthMessage.classList.remove('hidden'); }
	}
	if (query.get('site_error') && oauthMessage) {
	  oauthMessage.textContent = query.get('site_error');
	  oauthMessage.classList.remove('hidden');
	}
    if (query.get('account') === 'deactivated' && oauthMessage) {
      oauthMessage.textContent = '账户已停用。如需恢复，请联系管理员。';
      oauthMessage.classList.remove('hidden');
    }
    if (query.get('ticket')) {
      const ticket = query.get('ticket');
      history.replaceState(null, '', '/auth/?mode=login');
      try { await completeLinuxDOTicket(ticket); return; } catch (error) { if (oauthMessage) { oauthMessage.textContent = error.message; oauthMessage.classList.remove('hidden'); } }
    }
    if (linuxdoButton) {
      linuxdoButton.addEventListener('click', async function () {
        showBusy(linuxdoButton, true);
        try {
          const code = document.getElementById('auth-registration-code')?.value.trim() || '';
          const data = await request('/api/auth/linuxdo/start', { method: 'POST', body: { registration_code: code } });
          location.href = data.authorization_url;
		} catch (error) { if (oauthMessage) { oauthMessage.textContent = error.message; oauthMessage.classList.remove('hidden'); } toast(error.message, 'danger'); showBusy(linuxdoButton, false); }
      });
    }
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      const username = document.getElementById('auth-username').value.trim();
      const password = document.getElementById('auth-password').value;
      const registrationCode = document.getElementById('auth-registration-code')?.value.trim() || '';
      if (!username || !password) return toast('请输入用户名和密码', 'danger');
      showBusy(submit, true);
      try {
        const body = { username: username, password: password, registration_code: registrationCode };
        const data = register ? await request('/api/auth/register', { method: 'POST', body: body }) : await requestLogin('/api/auth/login', body);
        setToken(data.token);
        goToRole(data.user);
	  } catch (error) { if (oauthMessage) { oauthMessage.textContent = error.message; oauthMessage.classList.remove('hidden'); } toast(error.message, 'danger'); }
      finally { showBusy(submit, false); }
    });
  }

  function renderPublicSession(user) {
    const loggedIn = Boolean(user);
    document.querySelectorAll('[data-public-guest]').forEach(function (element) { element.classList.toggle('hidden', loggedIn); });
    document.querySelectorAll('[data-public-user]').forEach(function (element) { element.classList.toggle('hidden', !loggedIn); });
    if (!loggedIn) return;
    const destination = Number(user.role) >= 25 ? '/admin/' : '/console/';
    const destinationLabel = Number(user.role) >= 25 ? '进入管理后台' : '进入控制台';
    setText('[data-public-username]', user.username || '已登录');
    setText('[data-public-destination-label]', destinationLabel);
    document.querySelectorAll('[data-public-destination]').forEach(function (element) { element.href = destination; });
  }

  async function initPublic() {
	const [config] = await Promise.all([loadAuthConfig(), loadSiteAnnouncement()]);
	applySiteContext(config);
	const token = userToken();
    if (token) {
      // Avoid briefly showing login/register actions while the saved session
      // is being validated with the server.
      document.querySelectorAll('[data-public-guest]').forEach(function (element) { element.classList.add('hidden'); });
    }
    let user = null;
    if (token) {
      try {
        user = await request('/api/user/self', { _suppressAuthRedirect: true });
      } catch (error) {
        if (error.status === 401 || error.status === 403) clearToken();
        else user = { username: '已登录', role: 1 };
      }
    }
    renderPublicSession(user);
    document.querySelectorAll('[data-public-logout]').forEach(function (button) {
      button.addEventListener('click', function () { clearToken(); renderPublicSession(null); });
    });
	document.querySelectorAll('[data-public-register]').forEach(function (element) {
      element.classList.toggle('hidden', !config.local_registration_enabled && !config.linuxdo_registration_enabled);
    });
  }

  function mountMobileNavigation(rail) {
    const topBar = document.querySelector('.top-app-bar');
    if (!rail || !topBar || document.querySelector('[data-mobile-menu-trigger]')) return null;

    const trigger = document.createElement('button');
    trigger.type = 'button';
    trigger.className = 'md-icon-button mobile-menu-trigger';
    trigger.dataset.mobileMenuTrigger = '';
    trigger.setAttribute('aria-label', '打开导航菜单');
    trigger.setAttribute('aria-haspopup', 'menu');
    trigger.setAttribute('aria-expanded', 'false');
    trigger.innerHTML = '<span class="material-symbol" aria-hidden="true">menu</span>';

    const menu = document.createElement('div');
    menu.className = 'mobile-nav-menu';
    menu.id = 'mobile-navigation-menu';
    menu.setAttribute('role', 'menu');
    menu.setAttribute('aria-label', '页面导航');
    menu.hidden = true;
    menu.innerHTML = '<div class="mobile-nav-menu-title">导航</div>';
    trigger.setAttribute('aria-controls', menu.id);

    rail.querySelectorAll('.nav-item').forEach(function (source) {
      const option = document.createElement('button');
      option.type = 'button';
      option.className = 'mobile-nav-option';
      option.setAttribute('role', 'menuitem');
      if (source.dataset.nav) option.dataset.mobileNav = source.dataset.nav;
      const icon = source.querySelector('.material-symbol');
      if (icon) option.appendChild(icon.cloneNode(true));
      const label = document.createElement('span');
      const labelSource = source.cloneNode(true);
      labelSource.querySelectorAll('.material-symbol, .nav-port').forEach(function (node) { node.remove(); });
      label.textContent = labelSource.textContent.trim();
      option.appendChild(label);
      option.addEventListener('click', function () {
        source.click();
        setOpen(false);
      });
      menu.appendChild(option);
    });

    let open = false;
    let closeTimer = 0;
    function setOpen(next) {
      open = Boolean(next);
      window.clearTimeout(closeTimer);
      trigger.setAttribute('aria-expanded', String(open));
      trigger.setAttribute('aria-label', open ? '关闭导航菜单' : '打开导航菜单');
      if (open) {
        menu.hidden = false;
        window.requestAnimationFrame(function () { menu.classList.add('open'); });
        const first = menu.querySelector('.mobile-nav-option');
        if (first) first.focus({ preventScroll: true });
      } else {
        menu.classList.remove('open');
        closeTimer = window.setTimeout(function () { if (!open) menu.hidden = true; }, 180);
      }
    }

    trigger.addEventListener('click', function () { setOpen(!open); });
    document.addEventListener('click', function (event) {
      if (open && !menu.contains(event.target) && !trigger.contains(event.target)) setOpen(false);
    });
    document.addEventListener('keydown', function (event) {
      if (open && event.key === 'Escape') {
        setOpen(false);
        trigger.focus();
      }
    });
    window.addEventListener('resize', function () {
      if (open && !window.matchMedia('(max-width: 820px)').matches) setOpen(false);
    });

    topBar.insertBefore(trigger, topBar.firstChild);
    document.body.appendChild(menu);
    return menu;
  }

  function mountRailCollapse(rail) {
    const bottom = rail && rail.querySelector('.rail-bottom');
    if (!bottom || bottom.querySelector('[data-rail-collapse]')) return null;

    rail.querySelectorAll('.nav-item').forEach(function (item) {
      if (!item.title) item.title = item.textContent.trim();
    });

    const toggle = document.createElement('button');
    toggle.type = 'button';
    toggle.className = 'nav-item rail-collapse-toggle';
    toggle.dataset.railCollapse = '';
    bottom.insertBefore(toggle, bottom.firstChild);

    function render(collapsed) {
      const label = collapsed ? '展开侧边栏' : '收起侧边栏';
      toggle.title = label;
      toggle.setAttribute('aria-label', label);
      toggle.setAttribute('aria-expanded', String(!collapsed));
      toggle.innerHTML = '<span class="material-symbol" aria-hidden="true">' + (collapsed ? 'left_panel_open' : 'left_panel_close') + '</span><span class="rail-collapse-label">' + label + '</span>';
    }

    render(Boolean(preferences().sidebarCollapsed));
    window.requestAnimationFrame(function () { document.documentElement.dataset.railReady = 'true'; });
    toggle.addEventListener('click', function () {
      const next = preferences();
      next.sidebarCollapsed = !next.sidebarCollapsed;
      savePreferences(next);
      render(next.sidebarCollapsed);
    });
	window.addEventListener('proxylan:preferences', function (event) {
	  render(Boolean(event.detail && event.detail.sidebarCollapsed));
	});
    return toggle;
  }

  function mountShell() {
    const shell = document.querySelector('[data-shell]');
    if (!shell) return null;
    const rail = document.querySelector('.nav-rail');
    const mobileMenu = mountMobileNavigation(rail);
    mountRailCollapse(rail);
    document.querySelectorAll('[data-nav]').forEach(function (link) {
      link.addEventListener('click', function () {
        const name = link.dataset.nav;
        document.querySelectorAll('.nav-item[data-nav]').forEach(function (item) { item.classList.toggle('active', item.dataset.nav === name); });
        if (mobileMenu) mobileMenu.querySelectorAll('[data-mobile-nav]').forEach(function (item) { item.classList.toggle('active', item.dataset.mobileNav === name); });
        document.querySelectorAll('[data-panel]').forEach(function (panel) {
          const active = panel.dataset.panel === name;
          panel.classList.toggle('active', active);
          if (active) {
            const title = document.querySelector('[data-page-title]');
            if (title) {
              title.textContent = panel.dataset.title || title.textContent;
              title.classList.remove('updating');
              void title.offsetWidth;
              title.classList.add('updating');
              title.addEventListener('animationend', function () { title.classList.remove('updating'); }, { once: true });
            }
            const subtitle = document.querySelector('[data-page-subtitle]');
            if (subtitle && panel.dataset.subtitle) subtitle.textContent = panel.dataset.subtitle;
            panel.classList.remove('entering');
            void panel.offsetWidth;
            panel.classList.add('entering');
            panel.addEventListener('animationend', function (event) {
              if (event.target === panel) panel.classList.remove('entering');
            }, { once: true });
          }
        });
        if (rail && window.matchMedia('(max-width: 820px)').matches) rail.scrollLeft = Math.max(0, link.offsetLeft - 12);
        const currentHash = location.hash.slice(1).split('/');
        const preservedSubpage = currentHash[0] === name && currentHash.length > 1 ? '/' + currentHash.slice(1).join('/') : '';
        history.replaceState(null, '', '#' + link.dataset.nav + preservedSubpage);
        window.dispatchEvent(new CustomEvent('proxylan:page', { detail: { name: name } }));
      });
    });
    const defaultPage = document.body.dataset.page === 'console' ? preferences().defaultPage : '';
    const requestedPage = location.hash.slice(1).split('/')[0] || defaultPage || document.querySelector('[data-nav].active')?.dataset.nav;
    const initial = document.body.dataset.page === 'console' && requestedPage === 'plans' ? 'subscriptions' : requestedPage;
    if (initial) {
      const target = document.querySelector('[data-nav="' + CSS.escape(initial) + '"]');
      if (target) target.click();
    }
    function syncPageFromHistory() {
      const name = location.hash.slice(1).split('/')[0];
      const target = document.querySelector('[data-nav="' + CSS.escape(name) + '"]');
      if (target) target.click();
    }
    window.addEventListener('hashchange', syncPageFromHistory);
    window.addEventListener('popstate', syncPageFromHistory);
    const logout = document.querySelector('[data-logout]');
    if (logout) logout.addEventListener('click', function () { clearToken(); location.href = '/'; });
    initDialogDismiss();
    return shell;
  }

  function mountStandaloneAdminShell(activeNav) {
    const shell = document.querySelector('[data-shell]');
    if (!shell) return null;
    const rail = document.querySelector('.nav-rail');
    const mobileMenu = mountMobileNavigation(rail);
    mountRailCollapse(rail);
    document.querySelectorAll('.nav-item[data-nav]').forEach(function (item) {
      item.classList.toggle('active', item.dataset.nav === activeNav);
    });
    if (mobileMenu) mobileMenu.querySelectorAll('[data-mobile-nav]').forEach(function (item) {
      item.classList.toggle('active', item.dataset.mobileNav === activeNav);
    });
    const logout = document.querySelector('[data-logout]');
    if (logout) logout.addEventListener('click', function () { clearToken(); location.href = '/'; });
    initDialogDismiss();
    return shell;
  }

  window.ProxyLan = { token: userToken, request: request, requestTOTP: requestTOTP, loadSiteAnnouncement: loadSiteAnnouncement, toast: toast, confirm: confirmAction, showBusy: showBusy, openDialog: openDialog, closeDialog: closeDialog, number: number, moneyFromQuota: moneyFromQuota, quotaFromUSD: quotaFromUSD, usdFromQuota: usdFromQuota, roundQuotaToUSDCent: roundQuotaToUSDCent, bindQuotaUSDFields: bindQuotaUSDFields, syncQuotaUSD: syncQuotaUSD, date: date, escapeHTML: escapeHTML, statusLabel: statusLabel, setToken: setToken, setText: setText, clearToken: clearToken, setAdminProof: setAdminProof, clearAdminProof: clearAdminProof, ensureAdminVerification: ensureAdminVerification, goToRole: goToRole, mountShell: mountShell, mountStandaloneAdminShell: mountStandaloneAdminShell, preferences: preferences, savePreferences: savePreferences, applyPreferences: applyPreferences, adminListPreference: adminListPreference, saveAdminListPreference: saveAdminListPreference };

  document.addEventListener('DOMContentLoaded', function () {
    mountMaterialRipples();
    ensureConfirmDialog();
    initDialogControls();
    if (document.body.dataset.page === 'auth') initAuth();
    if (document.body.dataset.page === 'public') initPublic();
  });
}());
