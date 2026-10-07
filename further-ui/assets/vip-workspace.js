(function () {
  'use strict';
  const api = window.FurtherUI;
  let page, panel, plans = [], self = {}, owned = [], billing = {}, pending;
  let loadedAt = 0, billingPending, billingState = 'loading', queuedRefresh = null;
  let renderedPlans = '', memberGeneration = 0;
  const planMutations = new Set();
  const purchases = new Set();
  const escape = function (value) { return api.escapeHTML(String(value == null ? '' : value)); };
  const price = function (value) { return Number(value || 0).toLocaleString('en-US', {minimumFractionDigits:2,maximumFractionDigits:6}); };
  const exactDuration = function (seconds) { let left=Number(seconds || 0); const parts=[]; for(const pair of [[86400,'天'],[3600,'小时'],[60,'分钟'],[1,'秒']]) {const count=Math.floor(left/pair[0]);if(count){parts.push(count+' '+pair[1]);left-=count*pair[0];}}return parts.join(' ') || '0 秒'; };
  const duration = function (plan) { return plan.duration_unit === 'custom' ? exactDuration(plan.custom_seconds) : Number(plan.duration_value || 1) + ' ' + ({year:'年',month:'个月',day:'天',hour:'小时'}[plan.duration_unit] || '个月'); };
  function state(message, danger) { const target = panel.querySelector('[data-vip-status]'); target.textContent = message || ''; target.classList.toggle('hidden', !message); target.classList.toggle('status-err', Boolean(danger)); target.setAttribute('role', danger ? 'alert' : 'status'); }
  function paymentNotice() {
    const notice = panel.querySelector('[data-vip-availability]');
    if (page === 'admin' && billingState !== 'ready') {
      notice.classList.remove('hidden');
      notice.textContent = billingState === 'loading' ? '正在读取支付状态…' : '支付状态暂时无法读取，可继续管理权益套餐或点击刷新重试。';
      return;
    }
    const ready = billing.enabled === true && billing.configured === true;
    notice.classList.toggle('hidden', ready);
    notice.innerHTML = '<span class="material-symbol" aria-hidden="true">info</span><div><b>LDC 支付尚未开放</b><p>' + (page === 'admin' ? '请在支付设置中配置 LDC 应用并启用支付，然后返回刷新。' : '站点管理员完成配置后，可在这里查看并开通权益。') + '</p></div>' + (page === 'admin' && Number(document.body.dataset.gatewayRole) >= 100 ? '<button type="button" class="md-btn md-btn-outlined" data-vip-open-payment>前往支付设置</button>' : '');
    const button = notice.querySelector('[data-vip-open-payment]');
    if (button) button.addEventListener('click', function () { const nav = document.querySelector('.nav-rail [data-nav="settings"]'); if(nav) nav.click(); const settings = document.querySelector('[data-settings-page="payment"]'); if(settings) settings.click(); });
  }
  function planCard(plan) {
    const quota = Number(plan.total_amount) > 0 ? api.moneyFromQuota(plan.total_amount, 2) : '不限额';
    const reset = {never:'有效期内不重置',daily:'每日重置',weekly:'每周重置',monthly:'每月重置',custom:'按自定义周期重置'}[plan.quota_reset_period] || '有效期内不重置';
    return '<article class="vip-plan-card" data-vip-plan="' + Number(plan.id) + '"><div class="vip-card-heading"><span class="vip-plan-symbol material-symbol" aria-hidden="true">workspace_premium</span><span class="tag ' + (plan.enabled ? 'success' : 'muted') + '">' + (plan.enabled ? '已启用' : '已停用') + '</span></div><h2>' + escape(plan.title) + '</h2><p class="vip-plan-description">' + escape(plan.subtitle || '按套餐配置提供订阅权益与调用额度。') + '</p><div class="vip-plan-price"><b>$' + price(plan.price_amount) + '</b><span>USD / ' + escape(duration(plan)) + '</span></div><div class="vip-plan-features"><div><span>周期额度</span><b>' + quota + '</b></div><div><span>额度周期</span><b>' + reset + '</b></div><div><span>每人购买上限</span><b>' + (Number(plan.max_purchase_per_user) > 0 ? Number(plan.max_purchase_per_user) + ' 次' : '不限次数') + '</b></div></div><div class="vip-card-actions">' + (page === 'admin' ? '<button type="button" class="md-btn md-btn-outlined" data-root-only data-vip-edit="' + Number(plan.id) + '">编辑权益</button><button type="button" class="md-icon-button" data-root-only data-vip-toggle="' + Number(plan.id) + '" title="' + (plan.enabled ? '停用套餐' : '启用套餐') + '"><span class="material-symbol" aria-hidden="true">' + (plan.enabled ? 'pause_circle' : 'play_circle') + '</span></button>' : '<button type="button" class="md-btn md-btn-filled" data-vip-buy="' + Number(plan.id) + '"' + (!billing.enabled || !billing.configured || purchases.has(Number(plan.id)) ? ' disabled' : '') + '>' + '<img src="' + escape((document.querySelector('.rail-logo img') || {}).src || '/further-api-logo.svg?v=handshake-20261004') + '" alt="" style="width:20px;height:20px;object-fit:contain">使用 LDC 开通</button>') + '</div></article>';
  }
  function render() {
    paymentNotice();
    const counts = { 'plan': plans.length, 'enabled': plans.filter(function (plan) { return plan.enabled; }).length, 'active': owned.filter(function(row){const item=row.subscription || row;return item.status === 'active' && Number(item.end_time) > Date.now()/1000;}).length };
    Object.keys(counts).forEach(function (key) { const target = panel.querySelector('[data-vip-' + key + '-count]'); if (target) target.textContent = api.number(counts[key]); });
    if (page === 'admin') { const count = document.querySelector('[data-subscription-count="paid"]'); if (count) count.textContent = api.number(plans.length); }
    const rows = panel.querySelector('[data-vip-plans]');
    const signature = JSON.stringify([plans, page, billing.enabled, billing.configured]);
    if (signature !== renderedPlans) {
    renderedPlans = signature;
    rows.innerHTML = plans.length ? plans.map(planCard).join('') : '<div class="md-card vip-empty-card"><span class="material-symbol" aria-hidden="true">workspace_premium</span><h2>还没有可用权益套餐</h2><p>' + (page === 'admin' ? '从套餐名称、价格和有效期开始，创建第一个付费权益套餐。' : '管理员发布套餐后，它们会显示在这里。') + '</p></div>';
    rows.querySelectorAll('[data-vip-edit]').forEach(function(button){ button.addEventListener('click',function(){ edit(plans.find(function(plan){return Number(plan.id) === Number(button.dataset.vipEdit);})); }); });
    rows.querySelectorAll('[data-vip-toggle]').forEach(function (button) { button.addEventListener('click', async function () { const id = Number(button.dataset.vipToggle), plan = plans.find(function (item) { return Number(item.id) === id; }); if (!plan || planMutations.has(id)) return; planMutations.add(id); api.showBusy(button, true); try { await api.request('/api/subscription/admin/plans/' + id, { method: 'PATCH', body: { enabled: !plan.enabled } }); api.toast(plan.enabled ? '套餐已停用' : '套餐已启用'); await load({ force: true }); } catch (error) { state(error.message, true); api.toast(error.message, 'danger'); } finally { planMutations.delete(id); if (button.isConnected) api.showBusy(button, false); } }); });
    rows.querySelectorAll('[data-vip-buy]').forEach(function(button){ button.addEventListener('click',function(){ buy(Number(button.dataset.vipBuy),button); }); });
    }
    if(page === 'admin') { panel.querySelector('[data-vip-create]').disabled = false; }
    else {
      const current = panel.querySelector('[data-vip-current]');
      const active = owned.filter(function(row){const item=row.subscription || row;return item.status === 'active' && Number(item.end_time) > Date.now()/1000;});
      const end = active.map(function(row){return Number((row.subscription || row).end_time);}).sort(function(a,b){return a-b;})[0];
      current.innerHTML = '<div class="vip-current-identity"><span class="material-symbol" aria-hidden="true">verified</span><div><span class="workspace-eyebrow">我的账户 · 当前权益</span><h2>' + (active.length ? 'VIP 订阅已生效' : '基础账户') + '</h2><p>' + (active.length ? active.length+' 项生效权益 · 最近到期 '+api.date(end * 1000) : '当前没有生效中的 VIP 订阅') + '</p></div></div><div class="vip-current-wallet"><span>账户可用余额</span><b>' + api.moneyFromQuota(self.quota,2) + '</b></div>';
      renderSubscriptions(owned, panel.querySelector('[data-vip-owned]'), false);
    }
  }
  function loadAdminPayment() {
    if (billingPending) return billingPending;
    billingState = 'loading';
    paymentNotice();
    billingPending = api.request('/api/user/ldc/info').then(function (value) {
      billing = value || {}; billingState = 'ready';
    }).catch(function () {
      billingState = 'error';
    }).finally(function () {
      billingPending = null; paymentNotice();
    });
    return billingPending;
  }
  async function load(options) {
    if (pending) {
      if (!options || options.force !== true) return pending;
      if (!queuedRefresh) queuedRefresh = pending.then(function () { queuedRefresh = null; return load({ force: true }); });
      return queuedRefresh;
    }
    if (options && options.reuse && loadedAt) return;
    if (!loadedAt) state('正在读取权益数据…');
    panel.setAttribute('aria-busy', 'true'); panel.querySelector('[data-vip-refresh]').disabled = true;
    // Plan management remains available while payment availability is being checked.
    if (page === 'admin') loadAdminPayment();
    pending = (async function () {
      if (page === 'admin') {
        const response = await api.request('/api/subscription/admin/plans');
        plans = (Array.isArray(response) ? response : []).map(function (row) { return row.plan || row; });
        render(); loadedAt = Date.now(); state(''); return;
      }
      const endpoints = ['/api/subscription/plans', '/api/user/ldc/info', '/api/user/self', '/api/subscription/self'];
      const results = await Promise.allSettled(endpoints.map(function (path) { return api.request(path); }));
      if (results[0].status === 'rejected') throw results[0].reason;
      plans = (Array.isArray(results[0].value) ? results[0].value : []).map(function (row) { return row.plan || row; });
      billing = results[1].status === 'fulfilled' ? results[1].value || {} : { enabled: false, configured: false };
      if (results[2].status === 'fulfilled') self = results[2].value || {};
      if (results[3].status === 'fulfilled') owned = ((results[3].value || {}).all_subscriptions || []).filter(function (row) { return row.plan_internal !== true; });
      render(); loadedAt = Date.now();
      const warnings = results.slice(1).filter(function (result) { return result.status === 'rejected'; });
      state(warnings.length ? '部分权益信息暂时无法更新，请点击刷新重试。已读取的套餐可继续查看。' : '', Boolean(warnings.length));
    })().catch(function (error) { state((loadedAt ? '当前保留上次读取的套餐。更新失败：' : '读取失败：') + error.message, true); }).finally(function () { pending = null; panel.removeAttribute('aria-busy'); panel.querySelector('[data-vip-refresh]').disabled = false; });
    return pending;
  }
  function edit(plan) {
    const form = document.querySelector('[data-vip-plan-form]'); form.reset();
    form.elements.id.value = plan ? plan.id : '';
    for(const name of ['title','subtitle','price_amount','duration_unit','duration_value','quota_reset_period','sort_order','max_purchase_per_user']) { if(plan && form.elements[name] && plan[name] != null) form.elements[name].value = plan[name]; }
    form.elements.quota_usd.value = plan ? api.usdFromQuota(plan.total_amount) : 10;
    form.elements.custom_seconds.value = plan && plan.custom_seconds ? plan.custom_seconds : 86400;
    form.elements.quota_reset_custom_seconds.value = plan && plan.quota_reset_custom_seconds ? plan.quota_reset_custom_seconds : 86400;
    form.elements.enabled.checked = plan ? Boolean(plan.enabled) : true;
    form.elements.allow_wallet_overflow.checked = plan ? plan.allow_wallet_overflow !== false : true;
    form.dataset.existingPlan = plan ? JSON.stringify(plan) : '';
    dialogFeedback(document.getElementById('vip-plan-dialog'), '');
    form.dispatchEvent(new Event('change', {bubbles:true}));
    showPlanSection('basics', false);
    document.querySelector('[data-vip-dialog-title]').textContent = plan ? '编辑付费权益' : '创建付费权益';
    api.openDialog('vip-plan-dialog');
  }
  async function buy(id, button) {
    const plan = plans.find(function(item){return Number(item.id) === id;});
    if(!plan || !plan.enabled || !billing.enabled || !billing.configured || purchases.has(id) || button.disabled) return;
    purchases.add(id); api.showBusy(button,true);
    try {
      const quote = await api.request('/api/user/ldc/quote',{method:'POST',body:{plan_id:id}});
      if(!quote || quote.currency !== 'LDC' || !/^\d+\.\d{2}$/.test(String(quote.credits)) || Number(quote.credits) <= 0 || Number(quote.plan_id) !== id) throw new Error('套餐报价无效，请刷新后重试');
      if(!await api.confirm('使用 '+quote.credits+' LDC，开通“'+plan.title+'”，有效期 '+duration(plan)+'。付款确认后权益生效。',{title:'确认 LDC 开通',confirmText:'前往 LDC 支付'})) return;
      const order = await api.request('/api/subscription/ldc/pay',{method:'POST',body:{plan_id:id,expected_credits:quote.credits}});
      const target = new URL(order.url);
      if(target.origin !== 'https://credit.linux.do' || target.pathname !== '/epay/pay/submit.php' || target.username || target.password || target.search || target.hash || order.method !== 'POST' || !order.params || !order.trade_no || order.currency !== 'LDC') throw new Error('LDC 支付地址无效');
      const form = document.createElement('form'); form.method='POST';form.action=target.href;form.hidden=true;
      Object.entries(order.params).forEach(function(pair){const input=document.createElement('input');input.type='hidden';input.name=pair[0];input.value=String(pair[1]);form.append(input);});
      document.body.append(form);try{HTMLFormElement.prototype.submit.call(form);}finally{form.remove();}
      state('订单已创建，正在前往 LDC 支付。付款完成后刷新查看权益。');
    } catch(error){state(error.message,true);api.toast(error.message,'danger');}
    finally{purchases.delete(id);if(button.isConnected)api.showBusy(button,false);panel.querySelectorAll('[data-vip-buy="'+id+'"]').forEach(function(item){item.disabled=!billing.enabled||!billing.configured;});}
  }
  function renderSubscriptions(records, target, admin) {
    target.innerHTML = records.length ? records.map(function(row){ const item=row.subscription || row, plan=plans.find(function(p){return Number(p.id)===Number(item.plan_id);}), active=item.status==='active' && item.end_time > Date.now()/1000; return '<article class="vip-owned-card"><div><b>'+escape(row.plan_title || (plan && plan.title) || '权益计划 #'+item.plan_id)+'</b><span class="tag '+(active?'success':'muted')+'">'+(active?'生效中':item.status==='cancelled'?'已终止':'已到期')+'</span></div><p>到期 '+api.date(Number(item.end_time) * 1000)+'</p><div class="vip-owned-values"><span>已使用 <b>'+api.moneyFromQuota(item.amount_used,2)+'</b></span><span>总额度 <b>'+(Number(item.amount_total)>0?api.moneyFromQuota(item.amount_total,2):'不限额')+'</b></span></div>'+(admin && active ? '<button type="button" class="md-btn md-btn-text" data-vip-invalidate="'+Number(item.id)+'">终止此项权益</button>':'')+'</article>'; }).join('') : '<div class="empty">当前没有权益记录。</div>';
    if (admin) target.querySelectorAll('[data-vip-invalidate]').forEach(function (button) { button.dataset.rootOnly = ''; });
    target.querySelectorAll('[data-vip-invalidate]').forEach(function(button){button.addEventListener('click',async function(){if(!await api.confirm('此项权益将立即终止。',{title:'终止用户权益',confirmText:'终止'}))return;api.showBusy(button,true);try{await api.request('/api/subscription/admin/user_subscriptions/'+button.dataset.vipInvalidate+'/invalidate',{method:'POST'});await readMember();}catch(error){state(error.message,true);}finally{api.showBusy(button,false);}});});
  }
  async function readMember() {
    const form = panel.querySelector('[data-vip-member-form]'), id = Number(form.elements.user_id.value);
    const target = panel.querySelector('[data-vip-members]'), notice = panel.querySelector('[data-vip-member-status]');
    if (!Number.isSafeInteger(id) || id < 1) { state('请输入有效的用户 ID', true); form.elements.user_id.focus(); return; }
    const current = ++memberGeneration;
    target.setAttribute('aria-busy', 'true'); notice.hidden = false; notice.textContent = '正在读取用户 #' + id + ' 的权益…'; notice.setAttribute('role', 'status');
    try {
      const records = await api.request('/api/subscription/admin/users/' + id + '/subscriptions');
      if (current !== memberGeneration) return;
      if (Number(form.elements.user_id.value) !== id) { notice.textContent = '用户 ID 已变化，请查询当前用户的权益。'; return; }
      renderSubscriptions(Array.isArray(records) ? records : [], target, true); target.dataset.userId = String(id); notice.textContent = '用户 #' + id + ' · ' + (Array.isArray(records) ? api.number(records.length) : 0) + ' 项权益记录'; state('');
    } catch (error) { if (current === memberGeneration) { notice.textContent = '查询失败：' + error.message; notice.setAttribute('role', 'alert'); } }
    finally { if (current === memberGeneration) target.removeAttribute('aria-busy'); }
  }
  function mountPayments(rail, area) {
    const nav = document.createElement('button');
    nav.type = 'button'; nav.className = 'nav-item'; nav.dataset.nav = 'wallet';
    nav.innerHTML = '<span class="material-symbol" aria-hidden="true">account_balance_wallet</span>充值与支付';
    rail.insertBefore(nav, rail.querySelector('[data-nav="subscriptions"]'));
    const workspace = document.createElement('section');
    workspace.className = 'view'; workspace.dataset.panel = 'wallet';
    workspace.dataset.title = '充值与支付'; workspace.dataset.subtitle = '选择支付方式，核对订单与到账记录';
    const host = document.createElement('div');
    host.className = 'further-inline-host further-native-feature-host';
    host.dataset.nativeFeaturePath = '/wallet'; workspace.append(host); area.append(workspace);
    let pending = null, mounted = false;
    function loadPayments() {
      if (mounted || pending) return pending;
      host.textContent = '正在载入充值与订单…'; host.setAttribute('aria-busy', 'true');
      pending = Promise.resolve().then(function () {
        if (!window.FurtherFeatures) throw new Error('充值页面暂时无法加载，请重试。');
        return window.FurtherFeatures.mount(host, { path: '/wallet' });
      }).then(function () { host.replaceChildren(); mounted = true; }).catch(function (error) {
        host.replaceChildren();
        const notice = document.createElement('p'); notice.setAttribute('role', 'alert'); notice.textContent = error.message;
        const retry = document.createElement('button'); retry.type = 'button'; retry.className = 'md-btn md-btn-outlined'; retry.textContent = '重新载入'; retry.addEventListener('click', loadPayments);
        host.append(notice, retry);
      }).finally(function () { pending = null; host.removeAttribute('aria-busy'); });
      return pending;
    }
    function migratePaymentLink() {
      if (location.hash !== '#vip/payment') return;
      history.replaceState(history.state, '', '#wallet');
      nav.click();
    }
    window.addEventListener('further-ui:page', function (event) { if (event.detail && event.detail.name === 'wallet') void loadPayments(); });
    window.addEventListener('hashchange', migratePaymentLink);
    window.addEventListener('popstate', migratePaymentLink);
    if (location.hash === '#vip/payment') history.replaceState(history.state, '', '#wallet');
    return nav;
  }
  function mount() {
    page=document.body.dataset.page; if(page !== 'admin' && page !== 'console')return;
    const rail=document.querySelector('.nav-rail'), area=document.querySelector('.content-area'); if(!rail||!area)return;
    const adminHost = page === 'admin' ? document.querySelector('[data-subscription-page="paid"]') : null;
    if (page === 'admin' && !adminHost) return;
    const paymentNav = page === 'console' ? mountPayments(rail, area) : null;
    if (page === 'console') {
      const nav=document.createElement('button');nav.type='button';nav.className='nav-item';nav.dataset.nav='vip';nav.innerHTML='<span class="material-symbol" aria-hidden="true">workspace_premium</span>VIP 权益';rail.insertBefore(nav,rail.querySelector('[data-nav="subscriptions"]'));
    }
    panel=document.createElement('section');panel.className=page === 'admin' ? 'vip-workspace' : 'view vip-workspace';
    if (page === 'console') panel.dataset.panel='vip';
    panel.dataset.title=page==='admin'?'付费权益':'VIP 权益';panel.dataset.subtitle='管理订阅权益、有效期与套餐额度';
    panel.innerHTML='<div class="workspace-panel-head"><div><h1>'+panel.dataset.title+'</h1><p>'+(page==='admin'?'管理套餐价格、有效期、周期额度与用户记录。':'查看你的订阅权益、到期时间和可开通的套餐。')+'</p></div><div class="list-actions"><button type="button" class="md-icon-button" data-vip-refresh title="刷新"><span class="material-symbol" aria-hidden="true">refresh</span></button>'+(page==='admin'?'<button type="button" class="md-btn md-btn-filled" data-vip-create disabled><span class="material-symbol" aria-hidden="true">add</span>创建权益套餐</button>':'')+'</div></div><div class="field-hint hidden" data-vip-status role="status"></div><div class="vip-availability hidden" data-vip-availability></div>'+(page==='console'?'<div class="md-card vip-current" data-vip-current></div>':'')+'<div class="vip-summary-grid"><div class="md-card"><span>权益套餐</span><b data-vip-plan-count>—</b></div><div class="md-card"><span>已启用套餐</span><b data-vip-enabled-count>—</b></div><div class="md-card"><span>生效权益</span><b data-vip-active-count>—</b></div></div><div class="vip-plan-grid" data-vip-plans></div>'+(page==='admin'?'<section class="md-card vip-member-management"><div class="card-head"><div><h2 class="md-card-title">用户权益与到期管理</h2><p class="card-hint">按用户查询已开通权益，或为用户分配套餐。</p></div></div><form class="vip-member-filter" data-vip-member-form><div class="md-textfield"><input class="md-input" type="number" min="1" name="user_id" placeholder=" " required><label class="md-input-label">用户 ID</label></div><button class="md-btn md-btn-outlined" type="submit">查询权益</button><button class="md-btn md-btn-filled" type="button" data-vip-assign>分配套餐</button></form><p class="vip-member-status" data-vip-member-status hidden role="status" aria-live="polite"></p><div class="vip-owned-grid" data-vip-members><div class="empty">输入用户 ID 查看权益。</div></div></section>':'<section class="md-card"><div class="card-head"><h2 class="md-card-title">我的权益记录</h2><button type="button" class="md-btn md-btn-outlined" data-vip-more-payment>充值与订单</button></div><div class="vip-owned-grid" data-vip-owned></div></section>');(adminHost || area).append(panel);
    if (page === 'admin') {
      const heading = panel.querySelector('.workspace-panel-head');
      heading.className = 'subscription-section-head';
      const headingCopy = heading.firstElementChild;
      headingCopy.innerHTML = '<h2>付费套餐</h2><p>设置价格、周期额度与有效期。用户开通记录在下方统一管理。</p>';
      panel.querySelector('.vip-summary-grid').remove();
      panel.querySelectorAll('[data-vip-create], [data-vip-assign]').forEach(function (button) { button.dataset.rootOnly = ''; });
    }
    panel.querySelector('[data-vip-refresh]').addEventListener('click',load);
    if(page==='admin')bindAdmin();else panel.querySelector('[data-vip-more-payment]').addEventListener('click',function(){paymentNav.click();});
    window.addEventListener('further-ui:page', function (event) { if (!event.detail || event.detail.name !== (page === 'admin' ? 'subscriptions' : 'vip')) return; if (page === 'admin' && !/^#subscriptions(?:\/paid)?$/.test(location.hash)) return; load({ reuse: true }); });
    if (page === 'admin') {
      const migrateLegacyLink = function () {
        if (!/^#vip(?:\/|$)/.test(location.hash)) return;
        history.replaceState(null, '', '#subscriptions/paid');
        const subscriptions = document.querySelector('.nav-rail [data-nav="subscriptions"]');
        if (document.querySelector('.view.active[data-panel="subscriptions"]')) {
          window.dispatchEvent(new CustomEvent('further-ui:page', { detail: { name: 'subscriptions' } }));
        } else if (subscriptions) {
          subscriptions.click();
        }
      };
      window.addEventListener('hashchange', migrateLegacyLink);
      window.addEventListener('popstate', migrateLegacyLink);
      migrateLegacyLink();
    }
    if (page === 'admin' ? /^#subscriptions(?:\/paid)?$/.test(location.hash) : /^#vip(?:\/|$)/.test(location.hash)) load({ reuse: true });
  }
  function dialogFeedback(dialog, message, danger) {
    const target = dialog.querySelector('[data-vip-dialog-feedback]');
    if (!target) return;
    target.textContent = message || '';
    target.hidden = !message;
    target.classList.toggle('is-error', Boolean(danger));
    target.setAttribute('role', danger ? 'alert' : 'status');
  }
  function setDialogBusy(dialog, busy, button) {
    if (busy) dialog.dataset.dialogBusy = 'true';
    else delete dialog.dataset.dialogBusy;
    dialog.querySelector('form').setAttribute('aria-busy', String(busy));
    dialog.querySelectorAll('input, select, textarea, [data-dialog-close], [data-vip-editor-tab]').forEach(function (control) { control.disabled = busy; });
    api.showBusy(button, busy);
    if (!busy && dialog.id === 'vip-plan-dialog') updatePlanEditor();
  }
  let editorAnimation = null;
  function showPlanSection(name, animate) {
    const dialog = document.getElementById('vip-plan-dialog');
    if (!dialog) return;
    const selected = ['basics', 'quota', 'publication'].includes(name) ? name : 'basics';
    if (editorAnimation) { editorAnimation.cancel(); editorAnimation = null; }
    dialog.querySelectorAll('[data-vip-editor-tab]').forEach(function (button) { const active = button.dataset.vipEditorTab === selected; button.setAttribute('aria-selected', String(active)); button.tabIndex = active ? 0 : -1; });
    dialog.querySelectorAll('[data-vip-editor-section]').forEach(function (section) { section.hidden = section.dataset.vipEditorSection !== selected; });
    const target = dialog.querySelector('[data-vip-editor-section="' + selected + '"]');
    if (animate && target && typeof target.animate === 'function') {
      const animation = target.animate([{ opacity: 0, transform: 'translateY(8px)' }, { opacity: 1, transform: 'translateY(0)' }], { duration: 220, easing: 'cubic-bezier(.16,1,.3,1)' });
      editorAnimation = animation;
      animation.onfinish = function () { if (editorAnimation === animation) { animation.cancel(); editorAnimation = null; } };
    }
  }
  function updatePlanEditor() {
    const dialog = document.getElementById('vip-plan-dialog');
    if (!dialog) return;
    const form = dialog.querySelector('form'), values = form.elements;
    const customDuration = values.duration_unit.value === 'custom', customReset = values.quota_reset_period.value === 'custom';
    form.querySelector('[data-vip-custom-duration]').hidden = !customDuration;
    form.querySelector('[data-vip-custom-reset]').hidden = !customReset;
    values.duration_value.disabled = customDuration || dialog.dataset.dialogBusy === 'true';
    values.custom_seconds.disabled = !customDuration || dialog.dataset.dialogBusy === 'true';
    values.quota_reset_custom_seconds.disabled = !customReset || dialog.dataset.dialogBusy === 'true';
    const set = function (name, text) { const node = dialog.querySelector('[data-vip-preview="' + name + '"]'); if (node) node.textContent = text; };
    set('title', values.title.value.trim() || '你的新套餐');
    set('subtitle', values.subtitle.value.trim() || '说明用户将获得的模型或调用权益。');
    set('price', values.price_amount.validity.valid ? '$' + price(values.price_amount.value) : '填写售价');
    set('duration', duration({ duration_unit: values.duration_unit.value, duration_value: values.duration_value.value, custom_seconds: values.custom_seconds.value }));
    set('quota', Number(values.quota_usd.value) > 0 ? api.moneyFromQuota(api.quotaFromUSD(Number(values.quota_usd.value)), 6) : '不限额');
    set('reset', { never: '有效期内不重置', daily: '每日重置', weekly: '每周重置', monthly: '每月重置', custom: '每 ' + exactDuration(values.quota_reset_custom_seconds.value) + ' 重置' }[values.quota_reset_period.value]);
    set('status', values.enabled.checked ? '保存后启用' : '保存为停用套餐');
  }
  function bindAdmin() {
    panel.querySelector('[data-vip-create]').addEventListener('click', function () { edit(null); });
    panel.querySelector('[data-vip-member-form]').addEventListener('submit', function (event) { event.preventDefault(); readMember(); });
    const field = function (name, title, type, extra, hint) {
      return '<label class="vip-editor-field" for="vip-editor-' + name + '"><span>' + title + '</span><input class="md-input" id="vip-editor-' + name + '" name="' + name + '" type="' + (type || 'text') + '" ' + (extra || '') + '>' + (hint ? '<small>' + hint + '</small>' : '') + '</label>';
    };
    const choice = function (name, title, options) {
      return '<label class="vip-editor-field" for="vip-editor-' + name + '"><span>' + title + '</span><select class="md-input" name="' + name + '" id="vip-editor-' + name + '">' + options.map(function (pair) { return '<option value="' + pair[0] + '">' + pair[1] + '</option>'; }).join('') + '</select></label>';
    };
    const dialog = document.createElement('div'); dialog.className = 'dialog-backdrop'; dialog.id = 'vip-plan-dialog';
    dialog.innerHTML = '<div class="md-dialog wide vip-plan-editor-surface" role="dialog" aria-modal="true" aria-labelledby="vip-plan-title"><div class="vip-editor-heading"><span>further-api · 付费权益</span><h2 id="vip-plan-title" data-vip-dialog-title>创建付费权益</h2><p>从一份清楚的价格与规则，开始一段完整的订阅。</p></div><form data-vip-plan-form novalidate><input type="hidden" name="id"><div class="vip-dialog-feedback" data-vip-dialog-feedback hidden role="status" aria-live="polite"></div><div class="vip-editor-layout"><div class="vip-editor-main"><nav class="vip-editor-tabs" role="tablist" aria-label="套餐配置分类">' + [['basics', '基础信息'], ['quota', '周期用量'], ['publication', '上架规则']].map(function (tab) { return '<button type="button" role="tab" id="vip-editor-tab-' + tab[0] + '" aria-controls="vip-editor-section-' + tab[0] + '" aria-selected="false" data-vip-editor-tab="' + tab[0] + '">' + tab[1] + '</button>'; }).join('') + '</nav>' +
      '<fieldset class="vip-editor-section" id="vip-editor-section-basics" role="tabpanel" aria-labelledby="vip-editor-tab-basics" data-vip-editor-section="basics"><legend>为权益取一个名字</legend>' + field('title', '权益名称', 'text', 'maxlength="128" required', '用户看到的套餐名称。') + field('subtitle', '权益说明', 'text', 'maxlength="255"') + '<div class="vip-editor-fields">' + field('price_amount', '价格（USD）', 'number', 'min="0" max="9999" step="0.000001" value="1" required') + choice('duration_unit', '有效期单位', [['day', '天'], ['month', '月'], ['year', '年'], ['hour', '小时'], ['custom', '自定义秒数']]) + field('duration_value', '有效期数值', 'number', 'min="1" step="1" value="30" required') + '<div data-vip-custom-duration hidden>' + field('custom_seconds', '自定义有效期（秒）', 'number', 'min="1" step="1" value="86400" required') + '</div></div></fieldset>' +
      '<fieldset class="vip-editor-section" id="vip-editor-section-quota" role="tabpanel" aria-labelledby="vip-editor-tab-quota" data-vip-editor-section="quota" hidden><legend>安排周期额度与重置规则</legend><div class="vip-editor-fields">' + field('quota_usd', '周期额度（USD）', 'number', 'min="0" step="0.000001" value="10" required', '0 表示不限制套餐额度。') + choice('quota_reset_period', '额度重置周期', [['never', '有效期内不重置'], ['daily', '每日'], ['weekly', '每周'], ['monthly', '每月'], ['custom', '自定义秒数']]) + '<div data-vip-custom-reset hidden>' + field('quota_reset_custom_seconds', '自定义重置周期（秒）', 'number', 'min="1" step="1" value="86400" required') + '</div>' + '</div><label class="vip-editor-switch"><input type="checkbox" name="allow_wallet_overflow" checked><span><b>允许钱包续扣</b><small>套餐额度不足时，继续使用用户的钱包余额。</small></span></label></fieldset>' +
      '<fieldset class="vip-editor-section" id="vip-editor-section-publication" role="tabpanel" aria-labelledby="vip-editor-tab-publication" data-vip-editor-section="publication" hidden><legend>决定用户如何看到套餐</legend>' + field('sort_order', '显示排序', 'number', 'step="1" value="0" required', '使用整数调整套餐在列表中的位置。') + field('max_purchase_per_user', '每人购买上限', 'number', 'min="0" step="1" value="0" required', '0 表示不限制购买次数。') + '<label class="vip-editor-switch"><input type="checkbox" name="enabled" checked><span><b>启用此套餐</b><small>保存后向用户开放。停用会保留已经开通的权益记录。</small></span></label></fieldset></div>' +
      '<aside class="vip-editor-preview" aria-label="套餐即时预览"><span class="vip-preview-kicker"><span class="material-symbol" aria-hidden="true">visibility</span>用户视角 · 即时预览</span><div class="vip-preview-card"><span class="material-symbol vip-preview-emblem" aria-hidden="true">workspace_premium</span><h3 data-vip-preview="title"></h3><p data-vip-preview="subtitle"></p><strong data-vip-preview="price"></strong><small data-vip-preview="duration"></small><dl><div><dt>周期额度</dt><dd data-vip-preview="quota"></dd></div><div><dt>额度重置</dt><dd data-vip-preview="reset"></dd></div></dl><span class="vip-preview-state" data-vip-preview="status"></span></div><p class="vip-preview-note">价格、周期额度与到期规则会一并保存。用户付款确认后，网关按照套餐规则开通权益。</p></aside></div><div class="vip-editor-footer"><span>调整完毕后，保存完整的套餐规则。</span><div><button type="button" class="md-btn md-btn-text" data-dialog-close>取消</button><button type="submit" class="md-btn md-btn-filled">保存权益套餐</button></div></div></form></div>';
    document.body.append(dialog);
    const planForm = dialog.querySelector('form');
    const presets = document.createElement('section');
    presets.className = 'form-presets';
    presets.setAttribute('aria-label', '套餐周期一键设置');
    presets.innerHTML = '<b>一键设置套餐周期</b><div class="list-actions"><button type="button" class="md-btn md-btn-tonal" data-vip-cycle-preset="monthly">月度额度 · 1个月</button><button type="button" class="md-btn md-btn-tonal" data-vip-cycle-preset="daily">日额度 · 30天</button><button type="button" class="md-btn md-btn-tonal" data-vip-cycle-preset="weekly-total">7天总额度</button></div><p class="field-hint" data-vip-cycle-status role="status">配置范围：有效期、额度重置周期。应用后可继续调整并保存。</p>';
    planForm.querySelector('.vip-editor-layout').before(presets);
    presets.querySelectorAll('[data-vip-cycle-preset]').forEach(function (button) {
      button.addEventListener('click', function () {
        if (dialog.dataset.dialogBusy === 'true') return;
        const preset = { monthly: ['month', '1', 'monthly'], daily: ['day', '30', 'daily'], 'weekly-total': ['day', '7', 'never'] }[button.dataset.vipCyclePreset];
        if (!preset) return;
        planForm.elements.duration_unit.value = preset[0];
        planForm.elements.duration_value.value = preset[1];
        planForm.elements.quota_reset_period.value = preset[2];
        updatePlanEditor();
        presets.querySelector('[data-vip-cycle-status]').textContent = '已填写“' + button.textContent + '”的周期规则。请检查价格、额度与生效规则后保存。';
      });
    });
    planForm.addEventListener('reset', function () { presets.querySelector('[data-vip-cycle-status]').textContent = '配置范围：有效期、额度重置周期。应用后可继续调整并保存。'; });
    planForm.addEventListener('input', updatePlanEditor);
    planForm.addEventListener('change', updatePlanEditor);
    dialog.querySelectorAll('[data-vip-editor-tab]').forEach(function (button) {
      button.addEventListener('click', function () { showPlanSection(button.dataset.vipEditorTab, true); });
      button.addEventListener('keydown', function (event) {
        if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return;
        event.preventDefault(); const buttons = Array.from(button.parentElement.children), index = buttons.indexOf(button);
        const next = event.key === 'Home' ? 0 : event.key === 'End' ? buttons.length - 1 : (index + (event.key === 'ArrowRight' ? 1 : -1) + buttons.length) % buttons.length;
        buttons[next].click(); buttons[next].focus();
      });
    });
    const assign = document.createElement('div'); assign.className = 'dialog-backdrop'; assign.id = 'vip-assign-dialog';
    assign.innerHTML = '<div class="md-dialog vip-assign-surface" role="dialog" aria-modal="true" aria-labelledby="vip-assign-title"><div class="vip-editor-heading"><span>further-api · 用户权益</span><h2 id="vip-assign-title">分配付费权益</h2></div><p class="vip-assign-description">为用户 <b data-vip-assign-user></b> 开通选定的套餐，周期额度与到期规则会一起生效。</p><form data-vip-assign-form><div class="vip-dialog-feedback" data-vip-dialog-feedback hidden role="status" aria-live="polite"></div><label class="vip-editor-field" for="vip-assign-plan"><span>权益套餐</span><select class="md-input" id="vip-assign-plan" name="plan_id" required></select></label><div class="form-actions"><button type="button" class="md-btn md-btn-text" data-dialog-close>取消</button><button type="submit" class="md-btn md-btn-filled">确认分配</button></div></form></div>';
    document.body.append(assign);
    panel.querySelector('[data-vip-assign]').addEventListener('click', function () {
      const form = panel.querySelector('[data-vip-member-form]'), id = Number(form.elements.user_id.value);
      if (!Number.isSafeInteger(id) || id < 1) { state('请先输入有效的用户 ID', true); form.elements.user_id.focus(); return; }
      const enabled = plans.filter(function (plan) { return plan.enabled; });
      if (!enabled.length) { state('请先启用至少一个权益套餐', true); return; }
      assign.querySelector('select').innerHTML = enabled.map(function (plan) { return '<option value="' + Number(plan.id) + '">' + escape(plan.title) + '</option>'; }).join('');
      assign.dataset.userId = String(id); assign.querySelector('[data-vip-assign-user]').textContent = '#' + id;
      dialogFeedback(assign, ''); api.openDialog(assign.id);
    });
    assign.querySelector('form').addEventListener('submit', async function (event) {
      event.preventDefault(); if (assign.dataset.dialogBusy === 'true') return;
      const button = event.submitter || event.target.querySelector('button[type="submit"]'), id = Number(assign.dataset.userId), planID = Number(assign.querySelector('select').value);
      if (!Number.isSafeInteger(id) || id < 1 || !plans.some(function (plan) { return Number(plan.id) === planID && plan.enabled; })) { dialogFeedback(assign, '用户或套餐已变化，请关闭后重新选择。', true); return; }
      setDialogBusy(assign, true, button); dialogFeedback(assign, '正在分配权益，请稍候…');
      try {
        await api.request('/api/subscription/admin/users/' + id + '/subscriptions', { method: 'POST', body: { plan_id: planID } });
        setDialogBusy(assign, false, button); dialogFeedback(assign, '权益已分配。'); api.closeDialog(assign.id); api.toast('用户 #' + id + ' 的权益已分配');
        if (Number(panel.querySelector('[data-vip-member-form]').elements.user_id.value) === id) void readMember();
      } catch (error) { dialogFeedback(assign, error.message, true); api.toast(error.message, 'danger'); }
      finally { setDialogBusy(assign, false, button); }
    });
    planForm.addEventListener('submit', async function (event) {
      event.preventDefault(); if (dialog.dataset.dialogBusy === 'true') return;
      const form = event.target, button = event.submitter || form.querySelector('button[type="submit"]'), id = Number(form.elements.id.value);
      dialogFeedback(dialog, '');
      if (!form.checkValidity()) {
        const invalid = form.querySelector('input:invalid, select:invalid');
        const section = invalid && invalid.closest('[data-vip-editor-section]');
        if (section) showPlanSection(section.dataset.vipEditorSection, false);
        if (invalid) { invalid.focus(); invalid.reportValidity(); }
        dialogFeedback(dialog, '请检查必填项与金额、有效期，再保存套餐。', true); return;
      }
      try {
        const values = form.elements, title = values.title.value.trim(), quota = api.quotaFromUSD(Number(values.quota_usd.value));
        if (!title) { showPlanSection('basics', false); values.title.focus(); throw new Error('请填写有效的权益名称。'); }
        if (!Number.isSafeInteger(quota) || quota < 0) { showPlanSection('quota', false); values.quota_usd.focus(); throw new Error('周期额度过大或格式不正确，请填写有效金额。'); }
        const existing = form.dataset.existingPlan ? JSON.parse(form.dataset.existingPlan) : {};
        const plan = Object.assign({}, existing, { title: title, subtitle: values.subtitle.value.trim(), price_amount: Number(values.price_amount.value), currency: 'USD', duration_unit: values.duration_unit.value, duration_value: Number(values.duration_value.value), custom_seconds: Number(values.custom_seconds.value), quota_reset_custom_seconds: Number(values.quota_reset_custom_seconds.value), total_amount: quota, upgrade_group: '', downgrade_group: '', quota_reset_period: values.quota_reset_period.value, sort_order: Number(values.sort_order.value), max_purchase_per_user: Number(values.max_purchase_per_user.value), enabled: values.enabled.checked, allow_wallet_overflow: values.allow_wallet_overflow.checked });
        delete plan.allow_balance_pay; delete plan.stripe_price_id; delete plan.creem_product_id; delete plan.waffo_pancake_product_id;
        setDialogBusy(dialog, true, button); dialogFeedback(dialog, '正在保存套餐规则，请稍候…');
        await api.request('/api/subscription/admin/plans' + (id ? '/' + id : ''), { method: id ? 'PUT' : 'POST', body: { plan: plan } });
        setDialogBusy(dialog, false, button); dialogFeedback(dialog, '套餐已保存。'); api.closeDialog(dialog.id); api.toast('付费权益配置已保存'); void load({ force: true });
      } catch (error) { dialogFeedback(dialog, error.message, true); api.toast(error.message, 'danger'); }
      finally { setDialogBusy(dialog, false, button); }
    });
    for (const target of [dialog, assign]) {
      target.querySelectorAll('[data-dialog-close]').forEach(function (button) { button.addEventListener('click', function () { if (target.dataset.dialogBusy !== 'true') api.closeDialog(target.id); }); });
      target.addEventListener('click', function (event) { if (event.target === target && target.dataset.dialogBusy !== 'true') api.closeDialog(target.id); });
    }
  }
  document.addEventListener('further:ready',mount);
  window.FurtherUIVIP={load:load};
}());
