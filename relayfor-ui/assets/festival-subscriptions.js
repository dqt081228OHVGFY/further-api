(function () {
  'use strict';
  let plans = [], billing = {}, fresh = false, pending = null, errorMessage = '';
  const purchases = new Set();
  const feedback = new Map();
  const api = function () { return window.ProxyLan; };
  const escape = function (value) { return api().escapeHTML(String(value == null ? '' : value)); };
  const price = function (plan) { return Number(plan.price_amount).toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 6 }) + ' ' + (plan.currency || 'USD'); };
  const paymentButton = function () { const logo = document.querySelector('.rail-logo img'); return '<img class="festival-payment-logo" src="' + escape(logo && logo.getAttribute('src') || '/relayfor-logo.svg') + '" alt="">使用 LDC 开通'; };
  const duration = function (plan) {
    if (plan.duration_unit === 'custom') return Number(plan.custom_seconds) + ' 秒';
    return Number(plan.duration_value || 1) + ' ' + ({ year: '年', month: '个月', day: '天', hour: '小时' }[plan.duration_unit] || '个月');
  };
  const ids = function (campaign) { return Array.isArray(campaign.subscription_plan_ids) ? campaign.subscription_plan_ids.map(Number).filter(function (id) { return Number.isSafeInteger(id) && id > 0; }) : []; };

  async function load() {
    if (pending) return pending;
    fresh = false;
    pending = (async function () {
      const admin = document.body.dataset.page === 'admin';
      const results = await Promise.allSettled([
        api().request(admin ? '/api/subscription/admin/plans' : '/api/subscription/plans'),
        admin ? Promise.resolve({}) : api().request('/api/user/ldc/info')
      ]);
      if (results[0].status !== 'fulfilled' || !Array.isArray(results[0].value)) {
        errorMessage = '套餐读取失败，请刷新重试。';
        plans = []; billing = {}; return;
      }
      plans = results[0].value.map(function (row) { return row.plan || row; });
      billing = results[1].status === 'fulfilled' ? results[1].value || {} : {};
      errorMessage = results[1].status === 'rejected' ? '支付状态读取失败，请刷新后再开通。' : '';
      fresh = true;
    }()).catch(function () { plans = []; billing = {}; errorMessage = '套餐读取失败，请刷新重试。'; }).finally(function () { pending = null; });
    return pending;
  }

  function render(campaign) {
    const selected = ids(campaign);
    const available = selected.map(function (id) { return plans.find(function (plan) { return Number(plan.id) === id && plan.enabled; }); }).filter(Boolean);
    const cards = available.map(function (plan, index) {
      const busy = purchases.has(Number(plan.id));
      const enabled = fresh && (billing.enabled && billing.configured) && !busy;
      const reset = { never: '有效期内不重置', daily: '每日重置', weekly: '每周重置', monthly: '每月重置', custom: '每 ' + Number(plan.quota_reset_custom_seconds || 0) + ' 秒重置' }[plan.quota_reset_period] || '有效期内不重置';
      const message = feedback.get(Number(plan.id));
      return '<article class="festival-plan-card"><div class="festival-plan-top"><span>节日精选 / 0' + (index + 1) + '</span><span>付费订阅</span></div><h3>' + escape(plan.title) + '</h3><p>' + escape(plan.subtitle || '让下一次创造，多一份从容。') + '</p><div class="festival-plan-price"><strong>' + escape(price(plan)) + '</strong><span>/ ' + escape(duration(plan)) + '</span></div><dl><div><dt>调用额度</dt><dd>' + (Number(plan.total_amount) > 0 ? escape(api().moneyFromQuota(plan.total_amount, 2)) : '不限额') + '</dd></div><div><dt>额度周期</dt><dd>' + escape(reset) + '</dd></div><div><dt>权益分组</dt><dd>' + escape(plan.upgrade_group || '保持当前分组') + '</dd></div><div><dt>购买上限</dt><dd>' + (Number(plan.max_purchase_per_user) > 0 ? Number(plan.max_purchase_per_user) + ' 次 / 人' : '不限次数') + '</dd></div></dl><button type="button" class="md-btn md-btn-filled" data-festival-plan-buy="' + Number(plan.id) + '"' + (!enabled ? ' disabled' : '') + '>' + (busy ? '正在处理…' : (!fresh ? '请刷新套餐状态' : (!(billing.enabled && billing.configured) ? '暂未开放支付' : paymentButton()))) + '</button><p class="festival-plan-feedback' + (message && message.error ? ' is-error' : '') + '" data-festival-plan-feedback="' + Number(plan.id) + '" role="status">' + escape(message && message.text || '') + '</p></article>';
    }).join('');
    return '<section class="festival-plans" data-festival-plans="' + escape(campaign.key || '') + '" data-festival-plan-ids="' + escape(JSON.stringify(selected)) + '"><div class="festival-plans-heading"><div><span class="festival-plans-eyebrow">FOR YOUR NEXT CREATION</span><h2>把好时光，续成创造力</h2><p>节日精选订阅套餐 · 价格、额度与有效期以套餐配置为准</p></div><a href="#subscriptions">全部套餐与支付方式 <span aria-hidden="true">↗</span></a></div>' + (errorMessage ? '<p class="festival-plans-notice" role="status">' + escape(errorMessage) + '</p>' : '') + (cards ? '<div class="festival-plan-grid">' + cards + '</div>' : '<div class="festival-plans-empty"><span aria-hidden="true">✦</span><div><h3>' + (fresh ? '节日套餐正在准备中' : '暂时无法读取套餐') + '</h3><p>' + (fresh ? '上架后会在这里相见，也可以先去看看现有订阅计划。' : '刷新活动页面后重试，其他活动仍可照常参与。') + '</p></div><a href="#subscriptions">浏览订阅计划 →</a></div>') + '<p class="festival-plans-footnote">活动关注免费。开通套餐需单独支付，不影响每日签到和节日签到。</p></section>';
  }

  function adminMarkup(campaign) {
    const selected = ids(campaign);
    const options = plans.map(function (plan) {
      return '<label class="festival-plan-option"><input type="checkbox" name="festival_plan_ids" value="' + Number(plan.id) + '"' + (selected.includes(Number(plan.id)) ? ' checked' : '') + '><span><b>' + escape(plan.title) + '</b><small>' + escape(price(plan)) + ' / ' + escape(duration(plan)) + (plan.enabled ? '' : ' · 已停用') + '</small></span></label>';
    }).join('');
    const missing = fresh ? selected.filter(function (id) { return !plans.some(function (plan) { return Number(plan.id) === id; }); }).map(function (id) { return '<label class="festival-plan-option"><input type="checkbox" name="festival_plan_ids" value="' + id + '" checked><span><b>套餐 #' + id + '（已不可用）</b><small>请取消关联后保存</small></span></label>'; }).join('') : '';
    return '<div class="festival-admin-plans" data-festival-plan-selection data-selection-ready="' + fresh + '" data-original-plan-ids="' + escape(JSON.stringify(selected)) + '"><div class="festival-admin-plans-heading"><b>关联付费订阅套餐</b><a href="#vip">创建 / 管理套餐 ↗</a></div><p>最多选择 12 个。专场展示套餐现价，价格与权益在 VIP 权益管理中配置。</p>' + (!fresh ? '<p role="status">套餐暂未读取成功，保存时会保留原有关联。</p>' : (options || '<p>还没有套餐，请先创建套餐，再刷新活动配置。</p>')) + missing + '</div>';
  }

  function selectedIDs(form) {
    const host = form.querySelector('[data-festival-plan-selection]');
    if (!host) return [];
    if (host.dataset.selectionReady !== 'true') return JSON.parse(host.dataset.originalPlanIds || '[]');
    const selected = Array.from(host.querySelectorAll('input[name="festival_plan_ids"]:checked')).map(function (input) { return Number(input.value); });
    if (selected.length > 12) throw new Error('每个节日最多关联 12 个付费套餐');
    return selected;
  }

  function updatePurchaseButtons(id) {
    document.querySelectorAll('[data-festival-plan-buy="' + id + '"]').forEach(function (button) {
      const plan = plans.find(function (item) { return Number(item.id) === id; });
      button.disabled = purchases.has(id) || !fresh || !plan || !plan.enabled || !(billing.enabled && billing.configured);
      button.innerHTML = purchases.has(id) ? '正在处理…' : paymentButton();
    });
    const message = feedback.get(id);
    document.querySelectorAll('[data-festival-plan-feedback="' + id + '"]').forEach(function (element) {
      element.textContent = message && message.text || '';
      element.classList.toggle('is-error', Boolean(message && message.error));
    });
  }

  async function buy(button) {
    const id = Number(button.dataset.festivalPlanBuy);
    const plan = plans.find(function (item) { return Number(item.id) === id; });
    const host = button.closest('[data-festival-plan-ids]');
    if (button.disabled || purchases.has(id) || !fresh || !plan || !plan.enabled || !host || !JSON.parse(host.dataset.festivalPlanIds || '[]').includes(id) || !billing.enabled || !billing.configured) return;
    purchases.add(id); feedback.delete(id); updatePurchaseButtons(id);
    try {
      const quote = await api().request('/api/user/ldc/quote', { method: 'POST', body: { plan_id: id } });
      if (quote.currency !== 'LDC' || !/^\d+\.\d{2}$/.test(String(quote.credits)) || Number(quote.credits) <= 0 || Number(quote.plan_id) !== id) throw new Error('LDC 报价无效，请刷新后重试。');
      const confirmed = await api().confirm('开通“' + (quote.plan_title || plan.title) + '”，有效期 ' + duration(plan) + '。本次需支付 ' + quote.credits + ' LDC，确认后前往 LDC 完成付款。', { title: '确认节日套餐', confirmText: '前往 LDC 支付' });
      if (!confirmed) return;
      const order = await api().request('/api/subscription/ldc/pay', { method: 'POST', body: { plan_id: id, expected_credits: String(quote.credits) } });
      const target = new URL(order.url);
      if (target.origin !== 'https://credit.linux.do' || target.pathname !== '/epay/pay/submit.php' || target.username || target.password || target.search || target.hash || order.method !== 'POST' || !order.trade_no || !order.params || order.currency !== 'LDC') throw new Error('LDC 支付地址无效，请刷新后重试。');
      const form = document.createElement('form'); form.method = 'POST'; form.action = target.href; form.hidden = true;
      Object.entries(order.params).forEach(function (entry) { const input = document.createElement('input'); input.type = 'hidden'; input.name = entry[0]; input.value = String(entry[1]); form.append(input); });
      document.body.append(form);
      feedback.set(id, { text: '订单已创建，前往 LDC 付款；支付结果以订单记录为准。', error: false });
      try { HTMLFormElement.prototype.submit.call(form); } finally { form.remove(); }
    } catch (error) {
      feedback.set(id, { text: error.message || '未能创建 LDC 订单，请稍后重试。', error: true });
      api().toast(error.message || '未能创建 LDC 订单', 'danger');
    } finally { purchases.delete(id); updatePurchaseButtons(id); }
  }

  document.addEventListener('click', function (event) {
    const button = event.target.closest && event.target.closest('[data-festival-plan-buy]');
    if (button) buy(button);
  });
  document.addEventListener('change', function (event) {
    if (!event.target.matches('input[name="festival_plan_ids"]')) return;
    const form = event.target.closest('[data-activity-campaign-form]');
    if (form) { form.dataset.activityDirty = 'true'; event.target.dispatchEvent(new Event('input', { bubbles: true })); }
  });
  window.ProxyLanFestivalPlans = { load: load, render: render, adminMarkup: adminMarkup, selectedIDs: selectedIDs };
}());
