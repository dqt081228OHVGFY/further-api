(function () {
  'use strict';

	const ADMIN_PROOF_KEY = 'further_admin_proof';
  const PREFERENCES_KEY = 'further_user_preferences';
	const ADMIN_LIST_PREFERENCES_KEY = 'further_admin_list_preferences';
  const DEFAULT_PREFERENCES = { theme: 'light', reducedMotion: false, defaultPage: 'overview', pageSize: 25, timezone: 'local', hideLockedModels: false, sidebarCollapsed: false, hourCycle: '24', showSeconds: false, hideSuccessfulLogs: false };
  // 单例 promise：同一时刻多个 sensitive 请求同时撞上 step_up_required 时，只弹一次窗
  let verificationPromise = null;
  let verificationResolve = null;
  function userToken() { return window.FurtherGateway && window.FurtherGateway.authenticated() ? 'session' : ''; }
  function setToken() { /* The gateway owns the in-memory access token and refresh cookie. */ }
  function clearToken() { return window.FurtherGateway.logout(); }
  function preferences() {
    try {
      const stored = JSON.parse(localStorage.getItem(PREFERENCES_KEY) || '{}') || {};
      const next = Object.assign({}, DEFAULT_PREFERENCES, stored, { reducedMotion: false });
      const sharedTheme = localStorage.getItem('further-api:theme:v1:mode');
      if (['light', 'dark', 'system'].includes(sharedTheme)) next.theme = sharedTheme;
      try {
        if (stored.reducedMotion !== false) localStorage.setItem(PREFERENCES_KEY, JSON.stringify(next));
        if (localStorage.getItem('further-motion') !== 'on') localStorage.setItem('further-motion', 'on');
      } catch (_) { /* Stored preferences still apply when writes are unavailable. */ }
      return next;
    }
    catch (_) { return Object.assign({}, DEFAULT_PREFERENCES); }
  }
  function savePreferences(value, options) {
    const next = Object.assign({}, DEFAULT_PREFERENCES, value || {}, { reducedMotion: false });
    if (options && options.source === 'server') {
      try {
        const selected = localStorage.getItem('further-api:theme:v1:mode');
        if (['light', 'dark', 'system'].includes(selected)) next.theme = selected;
      } catch (_) {}
    }
    try {
      localStorage.setItem(PREFERENCES_KEY, JSON.stringify(next));
      localStorage.setItem('further-api:theme:v1:mode', next.theme);
      localStorage.setItem('further-motion', 'on');
    } catch (_) { /* Preferences still apply when local storage is unavailable. */ }
    applyPreferences(next);
	window.dispatchEvent(new CustomEvent('further-ui:preferences', { detail: next }));
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
    const theme = next.theme === 'dark' || (next.theme === 'system' && window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches) ? 'dark' : 'light';
    document.documentElement.dataset.theme = theme;
    document.documentElement.classList.toggle('dark', theme === 'dark');
    document.documentElement.classList.toggle('light', theme === 'light');
    document.documentElement.dataset.furtherMotion = 'on';
    document.documentElement.dataset.reducedMotion = 'false';
    document.documentElement.dataset.railCollapsed = next.sidebarCollapsed ? 'true' : 'false';
  }
  applyPreferences(preferences());
  const MATERIAL_RIPPLE_SELECTOR = [
    'button:not([data-no-feedback])',
    '.md-btn', '[data-slot="button"]', '[data-slot="tabs-trigger"]',
    '.md-icon-button',
    '.nav-item',
    '.settings-directory-item',
    '.list-item-button',
    '.mobile-nav-option',
    '.docs-tab',
    '.trend-period-switch button',
    '.code-batch-toggle',
    '.model-subpage-tab', '.workspace-search', '.console-text-link',
    '[data-doc-anchor]', '.home-protocol-tabs button',
    '.list-visibility-toggle'
  ].join(',');
  function materialRippleHost(target) {
    if (!(target instanceof Element)) return null;
    const host = target.closest(MATERIAL_RIPPLE_SELECTOR);
    if (!host || host.matches(':disabled, [aria-disabled="true"]')) return null;
    return host;
  }
  const visibilityTransitions = new WeakMap();
  const transitionParents = new WeakMap();
  function transitionVisibility(element, visible, applyVisibility, options) {
    if (!element) return;
    visible = Boolean(visible);
    const previous = visibilityTransitions.get(element);
    if (previous && previous.visible === visible) {
      applyVisibility(visible);
      return;
    }
    const wasVisible = previous ? previous.visible : !element.hidden && !element.classList.contains('hidden') && window.getComputedStyle(element).display !== 'none';
    const interrupted = previous && element.isConnected ? window.getComputedStyle(element) : null;
    const startFrame = interrupted ? { opacity: interrupted.opacity || '1', transform: interrupted.transform || 'none' } : null;
    if (previous) previous.finish();
    const skipExit = options && options.skipExit === true && !visible;
    let animate = (wasVisible !== visible || (visible && options?.forceEnter)) && !skipExit && !(options && options.skipMotion) && element.isConnected;
    const parent = element.parentElement;
    for (let ancestor = parent; animate && ancestor; ancestor = ancestor.parentElement) {
      if (ancestor.hidden || ancestor.classList.contains('hidden') || window.getComputedStyle(ancestor).display === 'none') animate = false;
    }
    const bounds = animate && !visible && parent ? element.getBoundingClientRect() : null;
    const parentBounds = bounds && parent ? parent.getBoundingClientRect() : null;
    const display = bounds ? window.getComputedStyle(element).display : '';
    applyVisibility(visible);
    if (!animate) return;

    const savedStyles = new Map();
    function setStyle(name, value) {
      savedStyles.set(name, [element.style.getPropertyValue(name), element.style.getPropertyPriority(name)]);
      element.style.setProperty(name, value, 'important');
    }
    const originalInert = element.hasAttribute('inert');
    const originalAriaHidden = element.getAttribute('aria-hidden');
    let releaseParent = null;
    if (!visible && parent && bounds) {
      let positioning = transitionParents.get(parent);
      if (!positioning && window.getComputedStyle(parent).position === 'static') {
        positioning = { count: 0, value: parent.style.getPropertyValue('position'), priority: parent.style.getPropertyPriority('position') };
        transitionParents.set(parent, positioning);
        parent.style.setProperty('position', 'relative');
      }
      if (positioning) {
        positioning.count += 1;
        releaseParent = function () {
          positioning.count -= 1;
          if (!positioning.count) {
            if (positioning.value) parent.style.setProperty('position', positioning.value, positioning.priority);
            else parent.style.removeProperty('position');
            transitionParents.delete(parent);
          }
        };
      }
      setStyle('position', 'absolute');
      setStyle('left', (bounds.left - parentBounds.left + parent.scrollLeft - parent.clientLeft) + 'px');
      setStyle('top', (bounds.top - parentBounds.top + parent.scrollTop - parent.clientTop - Number(options?.exitOffsetY || 0)) + 'px');
      setStyle('width', bounds.width + 'px');
      setStyle('height', bounds.height + 'px');
      ['top', 'right', 'bottom', 'left'].forEach(function (side) { setStyle('margin-' + side, '0'); });
      setStyle('box-sizing', 'border-box');
      setStyle('display', display);
      element.setAttribute('inert', '');
      element.setAttribute('aria-hidden', 'true');
    }
    const originalEngine = element.getAttribute('data-surface-motion-engine');
    const transition = { visible: visible, timer: 0, finish: null, animation: null };
    const finish = function () {
      if (visibilityTransitions.get(element) !== transition) return;
      visibilityTransitions.delete(element);
      window.clearTimeout(transition.timer);
      transition.animation?.cancel();
      element.removeEventListener('animationend', onEnd);
      element.classList.remove('rf-entering', 'rf-leaving', 'entering');
      if (originalEngine === null) element.removeAttribute('data-surface-motion-engine');
      else element.setAttribute('data-surface-motion-engine', originalEngine);
      savedStyles.forEach(function (saved, name) {
        element.style.removeProperty(name);
        if (saved[0]) element.style.setProperty(name, saved[0], saved[1]);
      });
      if (!visible) {
        if (!originalInert) element.removeAttribute('inert');
        if (originalAriaHidden === null) element.removeAttribute('aria-hidden');
        else element.setAttribute('aria-hidden', originalAriaHidden);
      }
      if (releaseParent) releaseParent();
    };
    const onEnd = function (event) {
      if (event.target === element && (!event.animationName || event.animationName === (visible ? 'rf-surface-enter' : 'rf-surface-exit'))) finish();
    };
    transition.finish = finish;
    visibilityTransitions.set(element, transition);
    element.classList.add(visible ? 'rf-entering' : 'rf-leaving');
    if (visible && element.classList.contains('view')) element.classList.add('entering');
    if (typeof element.animate === 'function') {
      element.setAttribute('data-surface-motion-engine', 'web');
      transition.animation = element.animate([
        startFrame || { opacity: visible ? 0 : 1, transform: 'none' },
        { opacity: visible ? 1 : 0, transform: 'none' }
      ], { duration: visible ? 220 : 140, easing: visible ? 'cubic-bezier(.2,.8,.2,1)' : 'cubic-bezier(.4,0,1,1)', fill: 'both' });
      transition.animation.finished?.then(finish, function () {});
    }
    element.addEventListener('animationend', onEnd);
    // WAAPI starts on the compositor's next frame. Its finished promise owns
    // completion; a wall timer can cut the exit short while the browser is busy.
    if (!transition.animation?.finished) transition.timer = window.setTimeout(finish, visible ? 220 : 140);
  }
  const dropdownTransitions = new WeakMap();
  function transitionDropdown(element, visible, applyVisibility, options) {
    if (!element) return;
    visible = Boolean(visible);
    const previous = dropdownTransitions.get(element);
    if (previous && previous.visible === visible && !options?.immediate) return;
    const wasVisible = previous ? previous.visible : !element.hidden && !element.classList.contains('hidden') && window.getComputedStyle(element).display !== 'none';
    const current = previous && element.isConnected ? window.getComputedStyle(element) : null;
    const start = current ? { opacity: current.opacity || '1', transform: current.transform || 'none' } : { opacity: wasVisible ? 1 : 0, transform: wasVisible ? 'none' : 'translateY(-4px)' };
    const startHeight = wasVisible || previous ? element.getBoundingClientRect().height : 0;
    previous?.cancel();
    element.toggleAttribute('inert', !visible);
    element.setAttribute('aria-hidden', String(!visible));
    if ((!previous && wasVisible === visible) || options?.immediate || !element.isConnected) {
      applyVisibility(visible);
      return;
    }
    // Keep closing menus painted until the compositor completes. Their logical
    // state and focusability change immediately, even when an exit is interrupted.
    applyVisibility(true);
    const collapse = options?.collapse === true && element.classList.contains('code-batch-items');
    const end = { opacity: visible ? 1 : 0, transform: visible ? 'none' : 'translateY(-4px)' };
    const overflow = element.style.overflow;
    const endHeight = visible ? element.getBoundingClientRect().height : 0;
    if (collapse) {
      start.height = startHeight + 'px';
      end.height = endHeight + 'px';
      element.style.overflow = 'clip';
      element.dataset.dropdownCollapse = 'true';
      element.style.setProperty('--further-dropdown-start-height', start.height);
      element.style.setProperty('--further-dropdown-end-height', end.height);
    }
    const transition = { visible: visible, animation: null, timer: 0, cancel: null, finish: null };
    const cleanup = function () {
      window.clearTimeout(transition.timer);
      element.removeEventListener('animationend', onEnd);
      transition.animation?.cancel();
      if (collapse) {
        element.style.overflow = overflow;
        element.style.removeProperty('--further-dropdown-start-height');
        element.style.removeProperty('--further-dropdown-end-height');
        delete element.dataset.dropdownCollapse;
      }
      delete element.dataset.dropdownMotion;
      delete element.dataset.dropdownMotionEngine;
    };
    const finish = function () {
      if (dropdownTransitions.get(element) !== transition) return;
      dropdownTransitions.delete(element);
      cleanup();
      applyVisibility(visible);
    };
    const onEnd = function (event) {
      if (event.target === element && event.animationName === (collapse ? 'rf-dropdown-collapse' : visible ? 'rf-dropdown-enter' : 'rf-dropdown-exit')) finish();
    };
    transition.cancel = function () { dropdownTransitions.delete(element); cleanup(); };
    transition.finish = finish;
    dropdownTransitions.set(element, transition);
    element.dataset.dropdownMotion = visible ? 'opening' : 'closing';
    element.dataset.dropdownMotionEngine = typeof element.animate === 'function' ? 'web' : 'css';
    if (typeof element.animate === 'function') {
      transition.animation = element.animate([start, end], { duration: visible ? 220 : 140, easing: visible ? 'cubic-bezier(.2,.8,.2,1)' : 'cubic-bezier(.4,0,1,1)', fill: 'both' });
      transition.animation.finished?.then(finish, function () {});
    }
    element.addEventListener('animationend', onEnd);
    if (!transition.animation?.finished) transition.timer = window.setTimeout(finish, visible ? 220 : 140);
  }
  function createMaterialRipple(host, clientX, clientY) {
    if (!host) return;
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
  let materialRipplesMounted = false;
  function mountMaterialRipples() {
    if (materialRipplesMounted) return;
    materialRipplesMounted = true;
    document.addEventListener('pointerdown', function (event) {
      if (event.button !== 0 || event.isPrimary === false) return;
      createMaterialRipple(materialRippleHost(event.target), event.clientX, event.clientY);
    });
    document.addEventListener('click', function (event) {
      // Keyboard activation is trusted; shell initialization and route sync use .click().
      if (!event.isTrusted || event.detail !== 0) return;
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
  function quotaPerUSD() {
    const unit = Number(window.FurtherQuotaPerUnit);
    if (!Number.isFinite(unit) || unit <= 0) throw new Error('站点额度换算尚未加载，请刷新后重试。');
    return unit;
  }
  function moneyFromQuota(value, digits) { return '$' + (Number(value || 0) / quotaPerUSD()).toFixed(digits == null ? 2 : digits); }
  function quotaFromUSD(value) {
    const amount = Number(value);
    if (!Number.isFinite(amount)) return '';
    if (amount < 0) return -1;
    const quota = Math.round(amount * quotaPerUSD());
    if (!Number.isSafeInteger(quota)) throw new Error('金额超出允许范围');
    return quota;
  }
  function usdFromQuota(value) {
    const quota = Number(value);
    if (!Number.isFinite(quota)) return '';
    if (quota < 0) return '-1';
    return (quota / quotaPerUSD()).toFixed(6).replace(/\.?0+$/, '') || '0';
  }
  function roundQuotaToUSDCent(value) {
    const quota = Number(value);
    if (!Number.isFinite(quota) || quota <= 0) return 0;
    const quotaPerCent = quotaPerUSD() / 100;
    return Math.round(Math.round(quota / quotaPerCent) * quotaPerCent);
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
    const numeric = typeof value === 'number' || /^\d+(?:\.\d+)?$/.test(String(value)) ? Number(value) : null;
    const parsed = new Date(numeric === null ? value : Math.abs(numeric) < 1e12 ? numeric * 1000 : numeric);
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
    return window.FurtherGateway.request(path, options || {});
  }

	function clearAdminProof() { try { sessionStorage.removeItem(ADMIN_PROOF_KEY); } catch (_) {} }
  // Historical UI proofs are unused by the gateway's in-memory verification
  // flow. Remove them rather than retaining sensitive data across navigation.
  clearAdminProof();

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
  function dialogMotionSnapshot(dialog) {
    const surface = dialog.querySelector('.md-dialog');
    return { surface: surface, opacity: window.getComputedStyle(dialog).opacity,
      surfaceOpacity: surface ? window.getComputedStyle(surface).opacity : '1',
      transform: surface ? window.getComputedStyle(surface).transform : 'none' };
  }
  function cancelDialogMotion(dialog) {
    (dialog._motionAnimations || []).forEach(function (animation) { animation.cancel(); });
    dialog._motionAnimations = [];
    window.cancelAnimationFrame(dialog._focusFrame);
    dialog._focusFrame = 0;
  }
  function animateDialog(dialog, snapshot, opening) {
    if (typeof dialog.animate !== 'function') return [];
    dialog.dataset.dialogMotionEngine = 'web';
    const animations = [dialog.animate([
      { opacity: snapshot.opacity }, { opacity: opening ? 1 : 0 }
    ], { duration: opening ? 180 : 140, easing: 'linear', fill: 'both' })];
    if (snapshot.surface) animations.push(snapshot.surface.animate([
      { opacity: snapshot.surfaceOpacity, transform: snapshot.transform },
      { opacity: opening ? 1 : 0, transform: 'none' }
    ], { duration: opening ? 220 : 140, easing: opening ? 'cubic-bezier(.2,.8,.2,1)' : 'cubic-bezier(.4,0,1,1)', fill: 'both' }));
    dialog._motionAnimations = animations;
    return animations;
  }
  function openDialog(id, options) {
    const dialog = document.getElementById(id);
    if (!dialog) return;
    if (dialog.classList.contains('show') && !dialog.classList.contains('closing')) return;
    const wasShowing = dialog.classList.contains('show');
    const snapshot = wasShowing ? dialogMotionSnapshot(dialog) : { surface: dialog.querySelector('.md-dialog'), opacity: 0, surfaceOpacity: 0, transform: 'none' };
    if (dialog._cancelClose) dialog._cancelClose();
    cancelDialogMotion(dialog);
    if (!wasShowing) dialog._returnFocus = document.activeElement;
    dialog.removeAttribute('inert');
    dialog.classList.remove('closing');
    dialog.classList.add('opening');
    dialog.classList.add('show');
    dialog.setAttribute('aria-hidden', 'false');
    if (snapshot.surface) {
      snapshot.surface.setAttribute('role', 'dialog');
      snapshot.surface.setAttribute('aria-modal', 'true');
      snapshot.surface.setAttribute('tabindex', '-1');
    }
    const animations = animateDialog(dialog, snapshot, true);
    if (animations.length) Promise.allSettled(animations.map(function (animation) { return animation.finished; })).then(function () {
      if (dialog._motionAnimations === animations && !dialog.classList.contains('closing')) { cancelDialogMotion(dialog); dialog.classList.remove('opening'); }
    });
    if (!animations.length) window.setTimeout(function () { dialog.classList.remove('opening'); }, 220);
    dialog._focusFrame = window.requestAnimationFrame(function () {
      dialog._focusFrame = 0;
      if (!dialog.classList.contains('show') || dialog.classList.contains('closing')) return;
      const focusTarget = options && options.focusTarget || focusableElements(dialog)[0] || dialog.querySelector('.md-dialog');
      if (focusTarget) focusTarget.focus({ preventScroll: true });
    });
  }
  function closeDialog(id) {
    const dialog = document.getElementById(id);
    if (!dialog || dialog.dataset.dialogBusy === 'true' || !dialog.classList.contains('show') || dialog.classList.contains('closing')) return;
    const onClose = dialog._onClose;
    delete dialog._onClose;
    delete dialog._onAccept;
    const snapshot = dialogMotionSnapshot(dialog);
    cancelDialogMotion(dialog);
    dialog.classList.remove('opening');
    dialog.classList.add('closing');
    dialog.setAttribute('inert', '');
    if (!document.querySelector('.dialog-backdrop.show:not(.closing)') && dialog._returnFocus && document.contains(dialog._returnFocus)) {
      dialog._returnFocus.focus({ preventScroll: true });
    }
    let cancelled = false;
    const cancel = function () {
      cancelled = true;
      window.clearTimeout(dialog._closeTimer);
      dialog.removeEventListener('animationend', onEnd);
      if (dialog._cancelClose === cancel) { delete dialog._cancelClose; delete dialog._finishClose; }
    };
    const finish = function () {
      if (cancelled) return;
      cancel();
      cancelDialogMotion(dialog);
      dialog.classList.remove('show', 'closing', 'opening');
      delete dialog.dataset.dialogMotionEngine;
      dialog.removeAttribute('inert');
      dialog.setAttribute('aria-hidden', 'true');
      dialog.querySelectorAll('input[type="password"], input[autocomplete="one-time-code"]').forEach(function (input) { input.value = ''; });
      dialog.querySelectorAll('[data-created-token]').forEach(function (element) { element.textContent = ''; });
    };
    const onEnd = function (event) { if (event.target === dialog && (!event.animationName || event.animationName === 'backdrop-exit')) finish(); };
    dialog._cancelClose = cancel;
    dialog._finishClose = finish;
    dialog.addEventListener('animationend', onEnd);
    const animations = animateDialog(dialog, snapshot, false);
    if (animations.length) Promise.allSettled(animations.map(function (animation) { return animation.finished; })).then(finish);
    if (!animations.length || animations.some(function (animation) { return !animation.finished; })) dialog._closeTimer = window.setTimeout(finish, 230);
    if (typeof onClose === 'function') onClose();
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
        await request('/api/admin/security/verify', { method: 'POST', _stepUpAttempt: true, body: { code: form.elements.code.value.trim(), password: form.elements.password.value } });
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

  const mountedDialogs = new WeakSet();
  let dialogKeyboardMounted = false;
  function initDialogDismiss() {
    document.querySelectorAll('.dialog-backdrop').forEach(function (dialog) {
      if (mountedDialogs.has(dialog)) return;
      mountedDialogs.add(dialog);
      dialog.setAttribute('aria-hidden', String(!dialog.classList.contains('show')));
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
    if (dialogKeyboardMounted) return;
    dialogKeyboardMounted = true;
    document.addEventListener('keydown', function (event) {
      if (event.defaultPrevented) return;
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
  function authRedirectTarget() {
    return window.FurtherGateway.sanitizeAuthRedirect(new URLSearchParams(location.search).get('redirect'), location.origin) || '';
  }
  function goToRole(user) {
    location.replace(window.FurtherGateway.postLoginDestination(user, authRedirectTarget() || undefined));
  }

  async function loadAuthConfig() {
    return window.FurtherUISite.loadAuthConfig();
  }

  function setVisible(element, visible) {
    if (element) element.classList.toggle('hidden', !visible);
  }

  function applySiteContext(config) {
    return window.FurtherUISite.applyContext(config);
  }

  async function loadSiteAnnouncement() {
    return window.FurtherUISite.loadAnnouncement();
  }

  function apiErrorType(error) {
    return error && error.payload && error.payload.error && error.payload.error.type || '';
  }

  async function requestLogin(path, body, passwordEncryptionEnabled) {
    const result = await request(path, { method: 'POST', body: { username: body.username, password: body.password }, passwordEncryptionEnabled: passwordEncryptionEnabled === true });
    if (!result || (result.require_verification !== true && result.require_2fa !== true)) return result;
    if (!result.flow_token || (Number(result.expires_at) > 0 && Number(result.expires_at) * 1000 <= Date.now())) throw new Error('登录验证流程已过期，请重新登录');
    if (Array.isArray(result.methods) && !result.methods.some(function (method) { return method.method === '2fa' && method.available === true; })) throw new Error('此账号需要其他验证方式，请使用标准登录页面。');
    while (true) {
      const code = await requestTOTP({ title: '登录动态验证', message: '此账号已开启登录保护，请输入身份验证器中的 6 位验证码。', confirmText: '继续登录' });
      if (!code) throw new Error('已取消动态验证');
      try {
        return await request('/api/user/login/2fa', { method: 'POST', body: { flow_token: result.flow_token, code: code } });
      } catch (error) {
        const type = apiErrorType(error);
        const serverCode = error && (error.code || error.cause && error.cause.code || error.response && error.response.data && error.response.data.code);
        if (type !== 'totp_invalid' && serverCode !== 'TWOFA_CODE_INVALID') throw error;
        toast('动态验证码无效，请重试', 'danger');
      }
    }
  }

  async function initAuth() {
    const form = document.getElementById('auth-form');
    if (!form) return;
    // Configuration decides whether passwords must be encrypted. Keep both
    // native form submission and API login blocked until that read succeeds.
    form.addEventListener('submit', function (event) { event.preventDefault(); });
    const submit = document.getElementById('auth-submit');
    const linuxdoButton = document.getElementById('linuxdo-login');
    const oauthMessage = document.getElementById('auth-oauth-message');
    if (submit) submit.disabled = true;
    if (linuxdoButton) {
      linuxdoButton.disabled = true;
      linuxdoButton.setAttribute('aria-disabled', 'true');
    }
    const mode = (new URLSearchParams(location.search).get('mode') === 'register' || document.body.dataset.authMode === 'register') ? 'register' : 'login';
    const register = mode === 'register';
    const query = new URLSearchParams(location.search);
	const [config] = await Promise.all([loadAuthConfig(), loadSiteAnnouncement()]);

    if (config.unavailable) {
      const message = oauthMessage || document.createElement('div');
      message.className = 'status-err';
      message.setAttribute('role', 'alert');
      message.textContent = '登录配置加载失败，请重新加载后重试。';
      const reload = document.createElement('button');
      reload.type = 'button';
      reload.className = 'md-btn md-btn-outlined';
      reload.dataset.authConfigRetry = 'true';
      reload.textContent = '重新加载';
      reload.addEventListener('click', function () { location.reload(); });
      message.append(reload);
      if (!oauthMessage) form.before(message);
      return;
    }
	applySiteContext(config);
    const title = document.getElementById('auth-title');
    const description = document.getElementById('auth-description');
    const switcher = document.getElementById('auth-switch');
    const localFields = document.getElementById('local-auth-fields');
    const localClosed = document.getElementById('auth-local-closed');
    const codeWrap = document.getElementById('auth-registration-code-wrap');
    const codeHint = document.getElementById('auth-registration-hint');
    const localEnabled = !register || config.local_registration_enabled;
    const linuxdoEnabled = Boolean(config.linuxdo_login_enabled) && (!register || Boolean(config.linuxdo_registration_enabled));
    const codeRequired = register && Boolean(config.registration_code_required);
	if (title) title.textContent = register ? '注册账户' : '登录';
	if (description) { description.textContent = ''; description.classList.add('hidden'); }
    document.title = (register ? '注册' : '登录') + ' · further-api';
    setText('[data-auth-switch-hint]', register ? '已经有账户？' : '还没有账户？');
    const password = document.getElementById('auth-password');
    if (password) password.autocomplete = register ? 'new-password' : 'current-password';
    const passwordToggle = document.getElementById('auth-password-toggle');
    if (passwordToggle && password) passwordToggle.addEventListener('click', function () {
      const visible = password.type === 'password';
      password.type = visible ? 'text' : 'password';
      passwordToggle.setAttribute('aria-pressed', String(visible));
      passwordToggle.setAttribute('aria-label', visible ? '隐藏密码' : '显示密码');
      passwordToggle.querySelector('.material-symbol').textContent = visible ? 'visibility_off' : 'visibility';
    });
    if (submit) {
      submit.textContent = register ? '注册' : '登录';
      submit.disabled = false;
    }
    if (switcher) {
      switcher.textContent = register ? '已有账户？登录' : '创建账户';
      const switchQuery = new URLSearchParams({ mode: register ? 'login' : 'register' });
      const originalTarget = authRedirectTarget();
      if (originalTarget) switchQuery.set('redirect', originalTarget);
      switcher.href = '/auth/?' + switchQuery.toString();
    }
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
    setVisible(linuxdoButton, true);
    if (linuxdoButton) {
      linuxdoButton.disabled = !linuxdoEnabled;
      linuxdoButton.setAttribute('aria-disabled', String(!linuxdoEnabled));
    }
    const linuxdoHint = document.getElementById('linuxdo-login-hint');
    if (linuxdoHint) {
      linuxdoHint.textContent = config.unavailable ? '暂时无法读取登录配置，请刷新后重试。' : (register && config.linuxdo_login_enabled ? '本站暂未开放 Linux DO 新用户注册。' : 'Linux DO 登录尚未启用，可由管理员在「站点与登录」中配置。');
      setVisible(linuxdoHint, !linuxdoEnabled);
    }
    setVisible(document.querySelector('.auth-divider'), localEnabled);
    if (register && !config.local_registration_enabled && !config.linuxdo_registration_enabled) {
      if (description) { description.textContent = '注册已关闭。'; description.classList.remove('hidden'); }
      setVisible(submit, false);
      setVisible(switcher, false);
      setVisible(document.querySelector('[data-auth-switch-hint]'), false);
    }
    if (!register && !config.local_registration_enabled && !config.linuxdo_registration_enabled) {
      setVisible(switcher, false);
      setVisible(document.querySelector('[data-auth-switch-hint]'), false);
    }
	if (query.get('oauth_error')) {
      if (oauthMessage) { oauthMessage.textContent = query.get('oauth_error'); oauthMessage.classList.remove('hidden'); }
	}
	if (query.get('site_error') && oauthMessage) {
	  oauthMessage.textContent = query.get('site_error');
	  oauthMessage.classList.remove('hidden');
	}
    if (query.get('registered') === '1' && oauthMessage) {
      oauthMessage.textContent = '注册成功，请使用新账户登录。';
      oauthMessage.classList.remove('hidden');
    }
    if (query.get('account') === 'deactivated' && oauthMessage) {
      oauthMessage.textContent = '账户已停用。如需恢复，请联系管理员。';
      oauthMessage.classList.remove('hidden');
    }
    if (linuxdoButton) {
      linuxdoButton.addEventListener('click', async function () {
        showBusy(linuxdoButton, true);
        try {
          const code = document.getElementById('auth-registration-code')?.value.trim() || '';
          const data = await request('/api/oauth/state', { method: 'POST', body: { provider: 'linuxdo', intent: 'login', registration_code: code } });
          if (!data || !data.authorization_url) throw new Error('LinuxDO 登录尚未完成配置');
          const authorization = new URL(data.authorization_url);
          if (authorization.protocol !== 'https:' || authorization.username || authorization.password) throw new Error('身份服务返回的授权地址无效，请重新登录。');
          location.assign(authorization.href);
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
        const data = register ? await request('/api/user/register', { method: 'POST', body: body }) : await requestLogin('/api/user/login', body, config.password_login_encryption_enabled === true);
        if (register) {
          // Registration creates the account but deliberately does not issue a
          // session. Return to the same original page in login mode so the
          // user can sign in without entering the React auth shell.
          const loginQuery = new URLSearchParams({ mode: 'login', registered: '1' });
          const originalTarget = authRedirectTarget();
          if (originalTarget) loginQuery.set('redirect', originalTarget);
          location.href = '/auth/?' + loginQuery.toString();
          return;
        }
        if (!data || !data.access_token || !data.session || !data.user || !window.FurtherGateway.authenticated()) throw new Error('登录尚未完成，请完成身份验证后重试。');
        goToRole(data.user);
	  } catch (error) { if (oauthMessage) { oauthMessage.textContent = error.message; oauthMessage.classList.remove('hidden'); } toast(error.message, 'danger'); }
      finally { showBusy(submit, false); }
    });
  }

  function renderPublicSession(user) {
    document.querySelectorAll('[data-public-session-status]').forEach(function (element) { element.remove(); });
    const loggedIn = Boolean(user);
    document.querySelectorAll('[data-public-guest]').forEach(function (element) { element.classList.toggle('hidden', loggedIn); });
    document.querySelectorAll('[data-public-user]').forEach(function (element) { element.classList.toggle('hidden', !loggedIn); });
    const destination = loggedIn ? window.FurtherGateway.managementDestination(Number(user.role)) : '/auth/?mode=login';
    document.querySelectorAll('[data-management-entry], [data-public-destination]').forEach(function (element) { element.href = destination; });
    setText('[data-public-destination-label]', window.FurtherGateway.managementLabel);
    if (!loggedIn) return;
    setText('[data-public-username]', user.username || '已登录');
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
    async function refreshPublicSession() {
      try {
        const user = await request('/api/user/self', { _suppressAuthRedirect: true });
        if (!user || typeof user.username !== 'string' || !user.username) throw new Error('服务器未返回有效账户信息');
        renderPublicSession(user);
      } catch (error) {
        const status = Number(error.status || error.response && error.response.status);
        if (status === 401 || status === 403) {
          try { await clearToken(); } catch (_) { /* The server already rejected this session. */ }
          renderPublicSession(null);
          return;
        }
        document.querySelectorAll('[data-public-session-status]').forEach(function (element) { element.remove(); });
        document.querySelectorAll('[data-public-user], [data-public-guest]').forEach(function (element) { element.classList.add('hidden'); });
        document.querySelectorAll('[data-public-guest]').forEach(function (guest) {
          const notice = document.createElement(guest.tagName === 'SPAN' ? 'span' : 'div');
          notice.className = 'status-line'; notice.dataset.publicSessionStatus = ''; notice.setAttribute('role', 'status');
          const message = document.createElement('span'); message.textContent = '暂时无法确认登录状态'; notice.append(message);
          const retry = document.createElement('button'); retry.type = 'button'; retry.className = 'md-btn md-btn-text'; retry.textContent = '重试';
          retry.addEventListener('click', async function () { showBusy(retry, true); try { await refreshPublicSession(); } finally { showBusy(retry, false); } });
          notice.append(retry); guest.before(notice);
        });
      }
    }
    if (token) await refreshPublicSession();
    else renderPublicSession(null);
    document.querySelectorAll('[data-public-logout]').forEach(function (button) {
      button.addEventListener('click', async function () { await clearToken(); renderPublicSession(null); });
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
        setOpen(false);
        source.click();
      });
      menu.appendChild(option);
    });

    let open = false;
    function applyMenuVisibility(shown) {
      menu.hidden = !shown;
      menu.classList.toggle('open', shown);
    }
    function setOpen(next) {
      if (open === Boolean(next)) return;
      open = Boolean(next);
      trigger.setAttribute('aria-expanded', String(open));
      trigger.setAttribute('aria-label', open ? '关闭导航菜单' : '打开导航菜单');
      if (open) {
        menu.querySelectorAll('.mobile-nav-option').forEach(function (option, index) {
          const source = rail.querySelectorAll('.nav-item')[index];
          option.hidden = Boolean(source && (source.hidden || source.classList.contains('hidden')));
        });
      } else if (menu.contains(document.activeElement)) trigger.focus({ preventScroll: true });
      transitionDropdown(menu, open, applyMenuVisibility);
      if (open) menu.querySelector('.mobile-nav-option:not([hidden])')?.focus({ preventScroll: true });
    }
    menu._finishMotion = function () { transitionDropdown(menu, open, applyMenuVisibility, { immediate: true }); };

    trigger.addEventListener('click', function () { setOpen(!open); });
    document.addEventListener('click', function (event) {
      if (open && !menu.contains(event.target) && !trigger.contains(event.target)) setOpen(false);
    });
    document.addEventListener('keydown', function (event) {
      if (!open || event.defaultPrevented) return;
      if (open && event.key === 'Escape') {
        event.preventDefault();
        setOpen(false);
        trigger.focus();
      } else if (['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) {
        event.preventDefault();
        const items = Array.from(menu.querySelectorAll('.mobile-nav-option:not([hidden])'));
        if (!items.length) return;
        const index = items.indexOf(document.activeElement);
        const next = event.key === 'Home' ? 0 : event.key === 'End' ? items.length - 1 : (index + (event.key === 'ArrowUp' ? -1 : 1) + items.length) % items.length;
        items[next].focus({ preventScroll: true });
      } else if (event.key === 'Tab') {
        setOpen(false);
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
	window.addEventListener('further-ui:preferences', function (event) {
	  render(Boolean(event.detail && event.detail.sidebarCollapsed));
	});
    return toggle;
  }

  const mountedShells = new WeakMap();
  function pageFromHash() {
    const name = location.hash.slice(1).split('/')[0];
    return document.body.dataset.page === 'console' && name === 'plans' ? 'subscriptions' : name;
  }
  function navigatePage(name, options) {
    const shell = document.querySelector('[data-shell]');
    const mounted = shell && mountedShells.get(shell);
    return Boolean(mounted && mounted.navigate(name, options));
  }
  function mountShell() {
    const shell = document.querySelector('[data-shell]');
    if (!shell) return null;
    const rail = document.querySelector('.nav-rail');
    let mounted = mountedShells.get(shell);
    const firstMount = !mounted;
    if (!mounted) {
      mounted = { mobileMenu: mountMobileNavigation(rail), navigation: new WeakSet(), initialized: false };
      mountedShells.set(shell, mounted);
      mountRailCollapse(rail);
    }
    const mobileMenu = mounted.mobileMenu;
    function activatePage(name, options) {
      if (!shell.isConnected) return false;
      const settings = options || {};
      if (document.body.dataset.page === 'console' && name === 'plans') name = 'subscriptions';
      const panel = document.querySelector('[data-panel="' + CSS.escape(name || '') + '"]');
      if (!panel || panel.hidden || panel.classList.contains('hidden')) return false;
      const activePanel = document.querySelector('[data-panel].active');
      const changed = !activePanel || activePanel !== panel;
      const initial = !mounted.initialized;
      document.querySelectorAll('.nav-item[data-nav]').forEach(function (item) {
        item.classList.toggle('active', item.dataset.nav === name);
        if (item.dataset.nav === name) item.setAttribute('aria-current', 'page');
        else item.removeAttribute('aria-current');
      });
      if (mobileMenu) mobileMenu.querySelectorAll('[data-mobile-nav]').forEach(function (item) { item.classList.toggle('active', item.dataset.mobileNav === name); });
      if (changed || initial) {
        const main = document.querySelector('.main-content');
        const shouldResetScroll = Boolean(main && main.scrollTop > 0);
        const exitOffsetY = shouldResetScroll ? main.scrollTop : 0;
        // Only the outgoing and incoming surfaces need layout/animation work.
        document.querySelectorAll('[data-panel].active').forEach(function (outgoing) {
          if (outgoing === panel) return;
          transitionVisibility(outgoing, false, function (visible) { outgoing.classList.toggle('active', visible); }, { skipExit: initial, skipMotion: initial, exitOffsetY: exitOffsetY });
        });
        if (shouldResetScroll) {
          if (typeof main.scrollTo === 'function') main.scrollTo({ top: 0, behavior: 'instant' });
          else main.scrollTop = 0;
        }
        panel.dataset.shellMotionOwner = 'true';
        transitionVisibility(panel, true, function (visible) { panel.classList.toggle('active', visible); }, { forceEnter: initial });
        const title = document.querySelector('[data-page-title]');
        if (title) {
          title.textContent = panel.dataset.title || title.textContent;
          title.classList.remove('updating');
          if (!initial) {
            if (typeof title.getAnimations === 'function') title.getAnimations().forEach(function (animation) { animation.cancel(); });
            title.classList.add('updating');
          }
        }
        const subtitle = document.querySelector('[data-page-subtitle]');
        if (subtitle) subtitle.textContent = panel.dataset.subtitle || '';
        const link = rail && rail.querySelector('[data-nav="' + CSS.escape(name) + '"]');
        if (link && window.matchMedia('(max-width: 820px)').matches) rail.scrollLeft = Math.max(0, link.offsetLeft - 12);
      }
      const parts = location.hash.slice(1).split('/');
      const subpage = pageFromHash() === name && parts.length > 1 ? '/' + parts.slice(1).join('/') : '';
      const hash = '#' + name + subpage;
      if (settings.history !== 'none' && location.hash !== hash) {
        if (initial || settings.history === 'replace') history.replaceState(history.state, '', hash);
        else history.pushState(history.state, '', hash);
      }
      mounted.initialized = true;
      if (changed || initial) window.dispatchEvent(new CustomEvent('further-ui:page', { detail: { name: name, waitUntil: function (request) { window.FurtherUIResources?.track?.(name, request); } } }));
      return true;
    }
    mounted.navigate = activatePage;
    document.querySelectorAll('[data-nav]').forEach(function (link) {
      if (mounted.navigation.has(link)) return;
      mounted.navigation.add(link);
      link.addEventListener('click', function (event) {
        if (!shell.isConnected || event.defaultPrevented) return;
        const origin = event.target instanceof Element ? event.target : event.target && event.target.parentElement;
        if (origin && origin.closest('[data-nav]') !== link) return;
        if (event instanceof MouseEvent && (event.button !== 0 || event.ctrlKey || event.metaKey || event.shiftKey || event.altKey) && link.tagName === 'A') return;
        // Links without a local panel remain ordinary standalone navigation.
        if (!document.querySelector('[data-panel="' + CSS.escape(link.dataset.nav || '') + '"]')) return;
        event.preventDefault();
        activatePage(link.dataset.nav);
      });
    });
    // Repeated calls may bind newly inserted controls, but must not activate
    // the current page again or install another set of history listeners.
    if (!firstMount) return shell;
    const defaultPage = document.body.dataset.page === 'console' ? preferences().defaultPage : '';
    const requestedPage = pageFromHash() || defaultPage || document.querySelector('[data-nav].active')?.dataset.nav;
    if (!activatePage(requestedPage, { history: 'replace' })) {
      const fallback = document.querySelector('[data-panel].active:not(.hidden):not([hidden])') || document.querySelector('[data-panel]:not(.hidden):not([hidden])');
      if (fallback) activatePage(fallback.dataset.panel, { history: location.hash ? 'none' : 'replace' });
    }
    function syncPageFromHistory() {
      if (!shell.isConnected) return;
      activatePage(pageFromHash() || defaultPage || 'overview', { history: 'none' });
    }
    window.addEventListener('hashchange', syncPageFromHistory);
    window.addEventListener('popstate', syncPageFromHistory);
    const title = document.querySelector('[data-page-title]');
    if (title) title.addEventListener('animationend', function (event) { if (event.target === title) title.classList.remove('updating'); });
    const logout = document.querySelector('[data-logout]');
    if (logout) logout.addEventListener('click', async function () { await clearToken(); location.href = '/'; });
    initDialogDismiss();
    document.documentElement.dataset.shellReady = 'true';
    window.dispatchEvent(new CustomEvent('further:shell-ready', { detail: { name: document.querySelector('[data-panel].active')?.dataset.panel } }));
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
    if (logout) logout.addEventListener('click', async function () { await clearToken(); location.href = '/'; });
    initDialogDismiss();
    document.documentElement.dataset.shellReady = 'true';
    window.dispatchEvent(new CustomEvent('further:shell-ready', { detail: { name: activeNav } }));
    return shell;
  }

  window.FurtherUI = { transitionDropdown: transitionDropdown, transitionVisibility: transitionVisibility, initializeVisuals: mountMaterialRipples, token: userToken, request: request, requestTOTP: requestTOTP, loadSiteAnnouncement: loadSiteAnnouncement, toast: toast, confirm: confirmAction, showBusy: showBusy, openDialog: openDialog, closeDialog: closeDialog, number: number, quotaPerUSD: quotaPerUSD, moneyFromQuota: moneyFromQuota, quotaFromUSD: quotaFromUSD, usdFromQuota: usdFromQuota, roundQuotaToUSDCent: roundQuotaToUSDCent, bindQuotaUSDFields: bindQuotaUSDFields, syncQuotaUSD: syncQuotaUSD, date: date, escapeHTML: escapeHTML, statusLabel: statusLabel, setToken: setToken, setText: setText, clearToken: clearToken, clearAdminProof: clearAdminProof, ensureAdminVerification: ensureAdminVerification, goToRole: goToRole, mountShell: mountShell, mountStandaloneAdminShell: mountStandaloneAdminShell, preferences: preferences, savePreferences: savePreferences, applyPreferences: applyPreferences, adminListPreference: adminListPreference, saveAdminListPreference: saveAdminListPreference };

  document.addEventListener('further:ready', function () {
    document.querySelectorAll('[data-workspace-refresh]').forEach(function (button) {
      if (button.dataset.refreshBound === 'true') return;
      button.dataset.refreshBound = 'true';
      button.addEventListener('click', async function () {
        if (button.disabled) return;
        const panel = document.querySelector('[data-panel].active');
        if (!panel) return;
        const work = [];
        showBusy(button, true);
        window.dispatchEvent(new CustomEvent('further-ui:refresh', { detail: {
          name: panel.dataset.panel,
          waitUntil: function (pending) { work.push(Promise.resolve(pending)); }
        } }));
        try {
          const results = await Promise.allSettled(work);
          const failure = results.find(function (result) { return result.status === 'rejected'; });
          if (failure) toast(failure.reason.message || '本页数据暂时无法更新，请重试。', 'danger');
        } finally { showBusy(button, false); }
      });
    });
    mountMaterialRipples();
    ensureConfirmDialog();
    initDialogControls();
    initDialogDismiss();
    // The console mounts after remote UI preferences. The bootstrap only waits
    // for synchronous ready listeners, so keep its default panel hidden until
    // mountShell has selected and bound the actual destination.
    if (document.querySelector('[data-shell]') && ['admin', 'console'].includes(document.body.dataset.page) && document.documentElement.dataset.shellReady !== 'true') {
      document.documentElement.dataset.shellReady = 'pending';
      window.queueMicrotask(function () {
        if (document.documentElement.dataset.shellReady === 'true') return;
        const name = pageFromHash() || (document.body.dataset.page === 'console' ? preferences().defaultPage : 'overview');
        const panel = document.querySelector('[data-panel="' + CSS.escape(name || '') + '"]');
        if (panel && !panel.hidden && !panel.classList.contains('hidden')) document.querySelectorAll('[data-panel]').forEach(function (item) { item.classList.toggle('active', item === panel); });
      });
    }
    if (document.body.dataset.page === 'auth') initAuth();
    if (document.body.dataset.page === 'public') initPublic();
  });
  window.FurtherUI.navigatePage = navigatePage;
  window.addEventListener('further-ui:preferences', function (event) {
    applyPreferences(event.detail || preferences());
  });
}());
