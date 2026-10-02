(function () {
  'use strict';
  const api = window.ProxyLan;
  const gateway = window.RelayForGateway;
  const form = document.querySelector('[data-prepaid-form]');
  const errorBox = document.querySelector('[data-prepaid-error]');
  const pendingKey = 'relayfor:prepaid:pending';

  function showError(error) {
    errorBox.textContent = error && error.message || '请求失败，请稍后重试';
    errorBox.classList.remove('hidden');
    if (errorBox.textContent.includes('绑定 LinuxDO')) {
      const link = document.createElement('a');
      link.href = '/profile';
      link.className = 'md-btn md-btn-text';
      link.textContent = '前往账户安全设置绑定';
      errorBox.appendChild(link);
    }
  }

  function login(action, code) {
    // The bearer card is never put in a URL, OAuth state, localStorage or logs.
    sessionStorage.setItem(pendingKey, JSON.stringify({ action: action, code: code || '', createdAt: Date.now() }));
    location.assign('/sign-in?redirect=%2Fprepaid%2F');
  }

  function showView(selector) {
    document.querySelector('[data-prepaid-form-view]').classList.add('hidden');
    document.querySelector('[data-prepaid-footer]').classList.add('hidden');
    document.querySelector(selector).classList.remove('hidden');
  }

  async function redeem(code) {
    const button = form.querySelector('button[type="submit"]');
    errorBox.classList.add('hidden');
    api.showBusy(button, true);
    try {
      const result = await api.request('/api/prepaid/redeem', { method: 'POST', body: { code: code }, _suppressAuthRedirect: true });
      sessionStorage.removeItem(pendingKey);
      showView('[data-prepaid-result]');
      document.querySelector('[data-prepaid-result-title]').textContent = result.merged ? '额度卡已合并' : '限时额度与 API Key 已生成';
      document.querySelector('[data-prepaid-result-detail]').textContent = '额度已核销到当前账户，用户组：' + result.user_group + '。' + (result.merged ? '继续使用现有额度卡 Key；每张卡的额度独立到期。' : '请妥善保存下方 Key，它仅属于当前已认证账户。');
      document.querySelector('[data-prepaid-quota]').textContent = api.moneyFromQuota(result.added_quota, 2);
      document.querySelector('[data-prepaid-expiry]').textContent = api.date(result.expires_at);
      if (result.api_key) {
        document.querySelector('[data-prepaid-key]').textContent = result.api_key;
        document.querySelector('[data-prepaid-key-box]').classList.remove('hidden');
        document.querySelector('[data-prepaid-key-disclaimer]').classList.remove('hidden');
      }
    } catch (error) {
      showError(error);
    } finally {
      api.showBusy(button, false);
    }
  }

  async function balance() {
    const button = document.querySelector('[data-prepaid-balance-login]');
    errorBox.classList.add('hidden');
    api.showBusy(button, true);
    try {
      const result = await api.request('/api/prepaid/balance', { _suppressAuthRedirect: true });
      showView('[data-prepaid-balance-result]');
      document.querySelector('[data-prepaid-balance-total]').textContent = api.moneyFromQuota(result.total_remaining, 2);
      document.querySelector('[data-prepaid-balance-expiry]').textContent = api.date(result.expires_at);
      document.querySelector('[data-prepaid-balance-key]').textContent = result.key_hint || '尚未生成额度卡 Key';
      document.querySelector('[data-prepaid-balance-key-status]').textContent = Number(result.key_status) === 1 ? 'Key 状态正常' : 'Key 当前不可用，请在密钥页面检查状态或有效期';
      document.querySelector('[data-prepaid-billing-hint]').textContent = result.billing_preference === 'wallet_only'
        ? '当前仅使用永久余额；请在用户控制台的账户设置中开启「优先使用限时额度」。'
        : '当前按照账户设置选择资金来源；额度卡额度到期后不会转为永久余额。';
      const buckets = Array.isArray(result.buckets) ? result.buckets : [];
      document.querySelector('[data-prepaid-balance-buckets]').innerHTML = buckets.map(function (item) {
        return '<div class="prepaid-balance-row"><div><b>' + api.escapeHTML(item.name || '额度卡额度') + '</b><span>有效期至 ' + api.date(item.expires_at) + '</span></div><div><b>' + api.moneyFromQuota(item.quota_remaining, 2) + '</b><span>原始 ' + api.moneyFromQuota(item.quota_total, 2) + '</span></div></div>';
      }).join('');
    } catch (error) {
      showError(error);
    } finally {
      api.showBusy(button, false);
    }
  }

  form.addEventListener('submit', function (event) {
    event.preventDefault();
    const code = form.elements.code.value.trim();
    if (!code) return;
    if (!gateway.authenticated()) {
      try { login('redeem', code); } catch (error) { showError(error); }
      return;
    }
    redeem(code);
  });

  document.querySelector('[data-prepaid-balance-login]').addEventListener('click', function () {
    if (!gateway.authenticated()) {
      try { login('balance'); } catch (error) { showError(error); }
      return;
    }
    balance();
  });

  document.querySelector('[data-copy-prepaid-key]').addEventListener('click', async function () {
    const key = document.querySelector('[data-prepaid-key]').textContent;
    if (!key) return;
    try { await navigator.clipboard.writeText(key); api.toast('Key 已复制'); }
    catch (_) { api.toast('复制失败，请手动保存', 'danger'); }
  });

  try {
    const pending = JSON.parse(sessionStorage.getItem(pendingKey) || 'null');
    if (pending && gateway.authenticated()) {
      // Resume at most once after the existing login flow. A refresh must not
      // silently replay a financial operation whose response was interrupted.
      sessionStorage.removeItem(pendingKey);
      if (typeof pending.createdAt === 'number' && Date.now() - pending.createdAt < 30 * 60 * 1000) {
        if (pending.action === 'redeem' && typeof pending.code === 'string') {
          form.elements.code.value = pending.code;
          redeem(pending.code);
        } else if (pending.action === 'balance') balance();
      }
    }
  } catch (error) { showError(error); }
}());
