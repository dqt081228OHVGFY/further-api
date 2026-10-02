(function () {
  'use strict';
  const api = window.ProxyLan;
  let page, panel, plans = [], self = {}, owned = [], billing = {}, pending;
  let loadedAt = 0, billingPending, billingState = 'loading';
  const purchases = new Set();
  const escape = function (value) { return api.escapeHTML(String(value == null ? '' : value)); };
  const price = function (value) { return Number(value || 0).toLocaleString('en-US', {minimumFractionDigits:2,maximumFractionDigits:6}); };
  const exactDuration = function (seconds) { let left=Number(seconds || 0); const parts=[]; for(const pair of [[86400,'天'],[3600,'小时'],[60,'分钟'],[1,'秒']]) {const count=Math.floor(left/pair[0]);if(count){parts.push(count+' '+pair[1]);left-=count*pair[0];}}return parts.join(' ') || '0 秒'; };
  const duration = function (plan) { return plan.duration_unit === 'custom' ? exactDuration(plan.custom_seconds) : Number(plan.duration_value || 1) + ' ' + ({year:'年',month:'个月',day:'天',hour:'小时'}[plan.duration_unit] || '个月'); };
  function state(message, danger) { const target = panel.querySelector('[data-vip-status]'); target.textContent = message || ''; target.classList.toggle('hidden', !message); target.classList.toggle('status-err', Boolean(danger)); }
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
    return '<article class="vip-plan-card" data-vip-plan="' + Number(plan.id) + '"><div class="vip-card-heading"><span class="vip-plan-symbol material-symbol" aria-hidden="true">workspace_premium</span><span class="tag ' + (plan.enabled ? 'success' : 'muted') + '">' + (plan.enabled ? '已启用' : '已停用') + '</span></div><h2>' + escape(plan.title) + '</h2><p class="vip-plan-description">' + escape(plan.subtitle || '按套餐配置提供分组权益与调用额度。') + '</p><div class="vip-plan-price"><b>$' + price(plan.price_amount) + '</b><span>USD / ' + escape(duration(plan)) + '</span></div><div class="vip-plan-features"><div><span>权益分组</span><b>' + escape(plan.upgrade_group || '保持当前分组') + '</b></div><div><span>周期额度</span><b>' + quota + '</b></div><div><span>额度周期</span><b>' + reset + '</b></div><div><span>每人购买上限</span><b>' + (Number(plan.max_purchase_per_user) > 0 ? Number(plan.max_purchase_per_user) + ' 次' : '不限次数') + '</b></div></div><div class="vip-card-actions">' + (page === 'admin' ? '<button type="button" class="md-btn md-btn-outlined" data-vip-edit="' + Number(plan.id) + '">编辑权益</button><button type="button" class="md-icon-button" data-vip-toggle="' + Number(plan.id) + '" title="' + (plan.enabled ? '停用套餐' : '启用套餐') + '"><span class="material-symbol" aria-hidden="true">' + (plan.enabled ? 'pause_circle' : 'play_circle') + '</span></button>' : '<button type="button" class="md-btn md-btn-filled" data-vip-buy="' + Number(plan.id) + '"' + (!billing.enabled || !billing.configured || purchases.has(Number(plan.id)) ? ' disabled' : '') + '>' + '<img src="' + escape((document.querySelector('.rail-logo img') || {}).src || '/relayfor-logo.svg') + '" alt="" style="width:20px;height:20px;object-fit:contain">使用 LDC 开通</button>') + '</div></article>';
  }
  function render() {
    paymentNotice();
    panel.querySelector('[data-vip-plan-count]').textContent = api.number(plans.length);
    panel.querySelector('[data-vip-enabled-count]').textContent = api.number(plans.filter(function (plan) { return plan.enabled; }).length);
    panel.querySelector('[data-vip-group-count]').textContent = api.number(new Set(plans.map(function(plan){return plan.upgrade_group;}).filter(Boolean)).size);
    const rows = panel.querySelector('[data-vip-plans]');
    rows.innerHTML = plans.length ? plans.map(planCard).join('') : '<div class="md-card vip-empty-card"><span class="material-symbol" aria-hidden="true">workspace_premium</span><h2>还没有可用权益套餐</h2><p>' + (page === 'admin' ? '从套餐名称、价格和有效期开始，创建你的第一个 VIP 权益。' : '管理员发布套餐后，它们会显示在这里。') + '</p></div>';
    rows.querySelectorAll('[data-vip-edit]').forEach(function(button){ button.addEventListener('click',function(){ edit(plans.find(function(plan){return Number(plan.id) === Number(button.dataset.vipEdit);})); }); });
    rows.querySelectorAll('[data-vip-toggle]').forEach(function(button){ button.addEventListener('click',async function(){ const plan = plans.find(function(item){return Number(item.id) === Number(button.dataset.vipToggle);}); api.showBusy(button,true); try { await api.request('/api/subscription/admin/plans/'+plan.id,{method:'PATCH',body:{enabled:!plan.enabled}}); await load(); } catch(error){state(error.message,true);} finally{api.showBusy(button,false);} }); });
    rows.querySelectorAll('[data-vip-buy]').forEach(function(button){ button.addEventListener('click',function(){ buy(Number(button.dataset.vipBuy),button); }); });
    if(page === 'admin') { panel.querySelector('[data-vip-create]').disabled = false; }
    else {
      const current = panel.querySelector('[data-vip-current]');
      const active = owned.filter(function(row){const item=row.subscription || row;return item.status === 'active' && Number(item.end_time) > Date.now()/1000;});
      const end = active.map(function(row){return Number((row.subscription || row).end_time);}).sort(function(a,b){return a-b;})[0];
      current.innerHTML = '<div class="vip-current-identity"><span class="material-symbol" aria-hidden="true">verified</span><div><span class="workspace-eyebrow">YOUR MEMBERSHIP / 当前权益</span><h2>' + escape(self.group || 'default') + '</h2><p>' + (active.length ? active.length+' 项生效权益 · 最近到期 '+api.date(end * 1000) : '当前没有生效中的 VIP 订阅') + '</p></div></div><div class="vip-current-wallet"><span>账户可用余额</span><b>' + api.moneyFromQuota(self.quota,2) + '</b></div>';
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
    if(pending) return pending;
    if(options && options.reuse && loadedAt && Date.now() - loadedAt < 30000) return;
    state('正在读取权益数据…'); panel.setAttribute('aria-busy','true');
    // Payment availability must not delay or prevent admin plan management.
    if(page === 'admin') loadAdminPayment();
    pending = (async function(){
      if(page === 'admin') {
        const response = await api.request('/api/subscription/admin/plans');
        plans = (Array.isArray(response) ? response : []).map(function(row){return row.plan || row;});
        render(); loadedAt = Date.now(); state('');
        return;
      }
      const responses = await Promise.all([api.request(page === 'admin' ? '/api/subscription/admin/plans' : '/api/subscription/plans'),api.request('/api/user/ldc/info'),page === 'console' ? api.request('/api/user/self') : Promise.resolve({}),page === 'console' ? api.request('/api/subscription/self') : Promise.resolve({})]);
      plans = (Array.isArray(responses[0]) ? responses[0] : []).map(function(row){return row.plan || row;}); billing=responses[1] || {}; self=responses[2] || {}; owned=(responses[3].all_subscriptions || []).filter(function(row){return row.plan_internal !== true;}); render(); loadedAt = Date.now(); state('');
    })().catch(function(error){state('读取失败：'+error.message,true);}).finally(function(){pending=null;panel.removeAttribute('aria-busy');});
    return pending;
  }
  function edit(plan) {
    const form = document.querySelector('[data-vip-plan-form]'); form.reset();
    form.elements.id.value = plan ? plan.id : '';
    for(const name of ['title','subtitle','price_amount','duration_unit','duration_value','upgrade_group','downgrade_group','quota_reset_period','sort_order','max_purchase_per_user']) { if(plan && form.elements[name]) form.elements[name].value = plan[name] == null ? '' : plan[name]; }
    form.elements.quota_usd.value = plan ? api.usdFromQuota(plan.total_amount) : 10;
    form.elements.custom_seconds.value = plan && plan.custom_seconds ? plan.custom_seconds : 86400;
    form.elements.quota_reset_custom_seconds.value = plan && plan.quota_reset_custom_seconds ? plan.quota_reset_custom_seconds : 86400;
    form.elements.enabled.checked = plan ? Boolean(plan.enabled) : true;
    form.elements.allow_wallet_overflow.checked = plan ? plan.allow_wallet_overflow !== false : true;
    form.dataset.existingPlan = plan ? JSON.stringify(plan) : '';
    form.dispatchEvent(new Event('change', {bubbles:true}));
    document.querySelector('[data-vip-dialog-title]').textContent = plan ? '编辑 VIP 权益' : '创建 VIP 权益';
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
    target.querySelectorAll('[data-vip-invalidate]').forEach(function(button){button.addEventListener('click',async function(){if(!await api.confirm('此项权益将立即终止。',{title:'终止用户权益',confirmText:'终止'}))return;api.showBusy(button,true);try{await api.request('/api/subscription/admin/user_subscriptions/'+button.dataset.vipInvalidate+'/invalidate',{method:'POST'});await readMember();}catch(error){state(error.message,true);}finally{api.showBusy(button,false);}});});
  }
  async function readMember() {
    const form = panel.querySelector('[data-vip-member-form]'), id=Number(form.elements.user_id.value);
    if(!Number.isInteger(id)||id<1){state('请输入有效的用户 ID',true);return;}
    try{const records=await api.request('/api/subscription/admin/users/'+id+'/subscriptions');renderSubscriptions(Array.isArray(records)?records:[],panel.querySelector('[data-vip-members]'),true);state('');}catch(error){state(error.message,true);}
  }
  function mount() {
    page=document.body.dataset.page; if(page !== 'admin' && page !== 'console')return;
    const rail=document.querySelector('.nav-rail'), area=document.querySelector('.content-area'); if(!rail||!area)return;
    const nav=document.createElement('button');nav.type='button';nav.className='nav-item';nav.dataset.nav='vip';nav.innerHTML='<span class="material-symbol" aria-hidden="true">workspace_premium</span>'+(page==='admin'?'VIP 权益管理':'VIP 权益');rail.insertBefore(nav,rail.querySelector('[data-nav="subscriptions"]'));
    panel=document.createElement('section');panel.className='view vip-workspace';panel.dataset.panel='vip';panel.dataset.title=page==='admin'?'VIP 权益管理':'VIP 权益';panel.dataset.subtitle='管理权益分组、有效期与套餐额度';
    panel.innerHTML='<div class="workspace-panel-head"><div><h1>'+panel.dataset.title+'</h1><p>'+(page==='admin'?'统一配置套餐价格、权益分组、有效期与会员记录。':'查看你的分组权益、到期时间和可开通的套餐。')+'</p></div><div class="list-actions"><button type="button" class="md-icon-button" data-vip-refresh title="刷新"><span class="material-symbol" aria-hidden="true">refresh</span></button>'+(page==='admin'?'<button type="button" class="md-btn md-btn-filled" data-vip-create disabled><span class="material-symbol" aria-hidden="true">add</span>创建权益套餐</button>':'')+'</div></div><div class="field-hint hidden" data-vip-status role="status"></div><div class="vip-availability hidden" data-vip-availability></div>'+(page==='console'?'<div class="md-card vip-current" data-vip-current></div>':'')+'<div class="vip-summary-grid"><div class="md-card"><span>权益套餐</span><b data-vip-plan-count>—</b></div><div class="md-card"><span>已启用套餐</span><b data-vip-enabled-count>—</b></div><div class="md-card"><span>可升级分组</span><b data-vip-group-count>—</b></div></div><div class="vip-plan-grid" data-vip-plans></div>'+(page==='admin'?'<section class="md-card vip-member-management"><div class="card-head"><div><h2 class="md-card-title">用户 VIP 与到期管理</h2><p class="card-hint">按用户查询已开通权益，或为用户分配套餐。</p></div></div><form class="vip-member-filter" data-vip-member-form><div class="md-textfield"><input class="md-input" type="number" min="1" name="user_id" placeholder=" " required><label class="md-input-label">用户 ID</label></div><button class="md-btn md-btn-outlined" type="submit">查询权益</button><button class="md-btn md-btn-filled" type="button" data-vip-assign>分配套餐</button></form><div class="vip-owned-grid" data-vip-members><div class="empty">输入用户 ID 查看权益。</div></div></section>':'<section class="md-card"><div class="card-head"><h2 class="md-card-title">我的权益记录</h2><button type="button" class="md-btn md-btn-outlined" data-vip-more-payment>LDC 充值与订单</button></div><div class="vip-owned-grid" data-vip-owned></div><div class="vip-wallet-host hidden" data-vip-wallet-host></div></section>');area.append(panel);
    panel.querySelector('[data-vip-refresh]').addEventListener('click',load);
    if(page==='admin')bindAdmin();else panel.querySelector('[data-vip-more-payment]').addEventListener('click',async function(event){const button=event.currentTarget,host=panel.querySelector('[data-vip-wallet-host]');host.classList.remove('hidden');if(host.dataset.mounted==='true')return;api.showBusy(button,true);try{if(!window.RelayForFeatures)throw new Error('暂时无法载入 LDC 充值，请刷新后重试。');await window.RelayForFeatures.mount(host,{path:'/wallet'});host.dataset.mounted='true';}catch(error){state(error.message,true);host.replaceChildren();}finally{api.showBusy(button,false);}});
    window.addEventListener('proxylan:page',function(event){if(event.detail&&event.detail.name==='vip')load({reuse:true});});
    if(location.hash.slice(1).split('/')[0]==='vip')load({reuse:true});
  }
  function bindAdmin() {
    panel.querySelector('[data-vip-create]').addEventListener('click',function(){edit(null);});
    panel.querySelector('[data-vip-member-form]').addEventListener('submit',function(event){event.preventDefault();readMember();});
    panel.querySelector('[data-vip-assign]').addEventListener('click',async function(){const form=panel.querySelector('[data-vip-member-form]'),id=Number(form.elements.user_id.value);if(!Number.isInteger(id)||id<1){state('请先输入用户 ID',true);return;}const enabled=plans.filter(function(plan){return plan.enabled;});if(!enabled.length){state('请先启用至少一个权益套餐',true);return;} const dialog=document.getElementById('vip-assign-dialog');dialog.querySelector('select').innerHTML=enabled.map(function(plan){return '<option value="'+Number(plan.id)+'">'+escape(plan.title)+'</option>';}).join('');dialog.dataset.userId=String(id);api.openDialog(dialog.id);});
    const dialog=document.createElement('div');dialog.className='dialog-backdrop';dialog.id='vip-plan-dialog';
    dialog.innerHTML='<div class="md-dialog wide" role="dialog" aria-modal="true" aria-labelledby="vip-plan-title"><h2 class="md-dialog-title" id="vip-plan-title" data-vip-dialog-title>创建 VIP 权益</h2><form data-vip-plan-form><input type="hidden" name="id"><div class="form-grid two"><div class="md-textfield"><input class="md-input" name="title" maxlength="128" placeholder=" " required><label class="md-input-label">权益名称</label></div><div class="md-textfield"><input class="md-input" name="price_amount" type="number" min="0" max="9999" step="0.000001" value="1" placeholder=" " required><label class="md-input-label">价格（USD）</label></div></div><div class="md-textfield"><input class="md-input" name="subtitle" maxlength="255" placeholder=" "><label class="md-input-label">权益说明</label></div><div class="form-grid two"><div class="md-textfield"><input class="md-input" name="duration_value" type="number" min="1" value="30" placeholder=" " required><label class="md-input-label">有效期数值</label></div><div class="md-textfield"><select class="md-input" name="duration_unit"><option value="day">天</option><option value="month">月</option><option value="year">年</option><option value="hour">小时</option><option value="custom">自定义（秒）</option></select><label class="md-input-label">有效期单位</label></div><div class="md-textfield"><input class="md-input" name="upgrade_group" placeholder=" "><label class="md-input-label">升级分组（留空保持原组）</label></div><div class="md-textfield"><input class="md-input" name="downgrade_group" placeholder=" "><label class="md-input-label">到期分组（可选）</label></div><div class="md-textfield"><input class="md-input" type="number" min="0" step="0.01" name="quota_usd" value="10" placeholder=" "><label class="md-input-label">周期额度（USD，0 不限额）</label></div><div class="md-textfield"><select class="md-input" name="quota_reset_period"><option value="never">不重置</option><option value="daily">每日</option><option value="weekly">每周</option><option value="monthly">每月</option><option value="custom">自定义（秒）</option></select><label class="md-input-label">额度重置周期</label></div><div class="md-textfield"><input class="md-input" name="sort_order" type="number" value="0" placeholder=" "><label class="md-input-label">显示排序</label></div><div class="md-textfield"><input class="md-input" name="max_purchase_per_user" type="number" min="0" value="0" placeholder=" "><label class="md-input-label">每人购买上限（0 不限）</label></div></div><div class="vip-form-switches"><label class="switch-line"><input type="checkbox" name="enabled" checked><span>启用此套餐</span></label><label class="switch-line"><input type="checkbox" name="allow_wallet_overflow" checked><span>套餐额度不足时允许钱包续扣</span></label></div><div class="form-actions"><button type="button" class="md-btn md-btn-text" data-dialog-close>取消</button><button type="submit" class="md-btn md-btn-filled">保存权益套餐</button></div></form></div>';document.body.append(dialog);
    const planForm = dialog.querySelector('form');
    planForm.querySelector('.form-grid').insertAdjacentHTML('beforeend', '<div class="md-textfield hidden" data-vip-custom-duration><input class="md-input" type="number" min="1" name="custom_seconds" value="86400" placeholder=" "><label class="md-input-label">自定义有效期（秒）</label></div><div class="md-textfield hidden" data-vip-custom-reset><input class="md-input" type="number" min="1" name="quota_reset_custom_seconds" value="86400" placeholder=" "><label class="md-input-label">自定义重置周期（秒）</label></div>');
    planForm.addEventListener('change', function () { planForm.querySelector('[data-vip-custom-duration]').classList.toggle('hidden', planForm.elements.duration_unit.value !== 'custom'); planForm.querySelector('[data-vip-custom-reset]').classList.toggle('hidden', planForm.elements.quota_reset_period.value !== 'custom'); });
    const assign=document.createElement('div');assign.className='dialog-backdrop';assign.id='vip-assign-dialog';assign.innerHTML='<div class="md-dialog" role="dialog" aria-modal="true" aria-labelledby="vip-assign-title"><h2 class="md-dialog-title" id="vip-assign-title">分配 VIP 权益</h2><form data-vip-assign-form><div class="md-textfield"><select class="md-input" name="plan_id" required></select><label class="md-input-label">权益套餐</label></div><div class="form-actions"><button type="button" class="md-btn md-btn-text" data-dialog-close>取消</button><button type="submit" class="md-btn md-btn-filled">确认分配</button></div></form></div>';document.body.append(assign);
    assign.querySelector('form').addEventListener('submit',async function(event){event.preventDefault();const button=event.submitter;api.showBusy(button,true);try{await api.request('/api/subscription/admin/users/'+Number(assign.dataset.userId)+'/subscriptions',{method:'POST',body:{plan_id:Number(assign.querySelector('select').value)}});api.closeDialog(assign.id);await readMember();api.toast('权益已分配');}catch(error){state(error.message,true);}finally{api.showBusy(button,false);}});
    dialog.querySelector('form').addEventListener('submit',async function(event){event.preventDefault();const form=event.target,button=event.submitter,id=Number(form.elements.id.value);api.showBusy(button,true);try{const existing=form.dataset.existingPlan?JSON.parse(form.dataset.existingPlan):{};const plan=Object.assign({},existing,{title:form.elements.title.value.trim(),subtitle:form.elements.subtitle.value.trim(),price_amount:Number(form.elements.price_amount.value),currency:'USD',duration_unit:form.elements.duration_unit.value,duration_value:Number(form.elements.duration_value.value),custom_seconds:Number(form.elements.custom_seconds.value),quota_reset_custom_seconds:Number(form.elements.quota_reset_custom_seconds.value),total_amount:api.quotaFromUSD(Number(form.elements.quota_usd.value)),upgrade_group:form.elements.upgrade_group.value.trim(),downgrade_group:form.elements.downgrade_group.value.trim(),quota_reset_period:form.elements.quota_reset_period.value,sort_order:Number(form.elements.sort_order.value),max_purchase_per_user:Number(form.elements.max_purchase_per_user.value),enabled:form.elements.enabled.checked,allow_wallet_overflow:form.elements.allow_wallet_overflow.checked});delete plan.allow_balance_pay;delete plan.stripe_price_id;delete plan.creem_product_id;delete plan.waffo_pancake_product_id;if((plan.duration_unit==='custom'&&!Number(plan.custom_seconds))||(plan.quota_reset_period==='custom'&&!Number(plan.quota_reset_custom_seconds)))throw new Error('自定义有效期和重置周期必须填写大于 0 的秒数。');await api.request('/api/subscription/admin/plans'+(id?'/'+id:''),{method:id?'PUT':'POST',body:{plan:plan}});api.closeDialog(dialog.id);await load();api.toast('VIP 权益配置已保存');}catch(error){state(error.message,true);api.toast(error.message,'danger');}finally{api.showBusy(button,false);}});
    for(const target of [dialog,assign]){target.querySelectorAll('[data-dialog-close]').forEach(function(button){button.addEventListener('click',function(){api.closeDialog(target.id);});});target.addEventListener('click',function(event){if(event.target===target)api.closeDialog(target.id);});}
  }
  document.addEventListener('relayfor:ready',mount);
  window.ProxyLanVIP={load:load};
}());

