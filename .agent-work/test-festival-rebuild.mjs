import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import { Window } from '../web/node_modules/happy-dom/lib/index.js';

// Contract tests exercise the shipped static runtime with real form and click
// events. The only replacement is the HTTP boundary; server state survives
// complete fixture recreation so subscription persistence is observable.
const root = new URL('../', import.meta.url);
const runtime = await fs.readFile(new URL('relayfor-ui/assets/activities.js', root), 'utf8');
const lunar = await fs.readFile(new URL('relayfor-ui/assets/lunar-schedule.js', root), 'utf8');
const paidPlans = await fs.readFile(new URL('relayfor-ui/assets/festival-subscriptions.js', root), 'utf8');
const markup = Object.fromEntries(await Promise.all(['console', 'admin'].map(async page => [page, await fs.readFile(new URL(`relayfor-ui/${page}/index.html`, root), 'utf8')])));
const escapeHTML = value => String(value ?? '').replace(/[&<>"']/g, character => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[character]));
const tick = async () => { for (let i = 0; i < 8; i++) await Promise.resolve(); await new Promise(resolve => setTimeout(resolve, 15)); };
const deferred = () => { let resolve; const promise = new Promise(done => { resolve = done; }); return { promise, resolve }; };
const fixtures = new Set();
const keys = ['duanwu', 'qixi', 'mid_autumn', 'chongyang', 'new_year_eve', 'national_day'];

function serverState(active = false) {
  return {
    checked: false,
    purchases: [],
    plans: [{ id: 7, title: '节日相聚套餐', subtitle: '来自服务器的付费权益', enabled: true, price_amount: 9.9, currency: 'USD', duration_unit: 'month', duration_value: 1, allow_balance_pay: true, total_amount: 25000, quota_reset_period: 'never' }],
    history: [],
    campaigns: keys.map(key => ({
      key, title: `${key} 后台配置标题`, description: `${key} 后台配置摘要`,
      details: '第一行活动详情\n第二行活动详情', rules: '每个账号每天签到一次\n活动签到与主签到分别计算',
      enabled: true, subscription_enabled: true, subscribed: false, can_subscribe: true,
      subscription_plan_ids: [7],
      date: key === 'mid_autumn' ? '2026-09-25' : '2026-10-18', next_date: '2027-09-15',
      status: active && key === 'mid_autumn' ? 'active' : 'upcoming',
      active: active && key === 'mid_autumn', can_claim: active && key === 'mid_autumn', claimed_today: false,
      starts_at: '2026-09-24T00:00:00+08:00', ends_at: '2026-09-27T00:00:00+08:00',
      reward_quota: 1250, daily_budget_quota: 10000, daily_claim_limit: 10,
      days_before: 1, days_after: 1, target_group: '', today_claims: 0, today_awarded_quota: 0,
    })),
  };
}

async function fixture({ page = 'console', key = 'mid_autumn', store = serverState(), options = {} } = {}) {
  const window = new Window({ url: `http://localhost/${page}/#activities${key ? `/${key}` : ''}`, settings: { disableCSSFileLoading: true, disableJavaScriptFileLoading: true, disableIframePageLoading: true } });
  window.document.write(markup[page]);
  const requests = [], toasts = [], confirmations = [], paymentForms = [];
  window.HTMLFormElement.prototype.submit = function () { paymentForms.push({ action: this.action, method: this.method, params: Object.fromEntries(new window.FormData(this)) }); };
  let verification;
  const view = () => ({ server_date: store.server_date || '2026-09-25', server_time: store.server_time || '2026-09-25T12:00:00+08:00', timezone: 'Asia/Shanghai', theme: store.campaigns.some(campaign => campaign.active) ? 'mid_autumn' : '', calendar_min_year: 2025, calendar_max_year: 2050, campaigns: structuredClone(store.campaigns), checkin: { enabled: true, min_quota: 1000, max_quota: 2000, turnstile_required: Boolean(options.turnstile), turnstile_site_key: options.turnstile ? 'verification-site-key' : '' }, history: structuredClone(store.history) });
  const api = {
    escapeHTML, moneyFromQuota: (quota, digits = 2) => '$' + (Number(quota || 0) / 1000).toFixed(digits),
    quotaFromUSD: value => Math.round(Number(value) * 1000), usdFromQuota: value => String(Number(value) / 1000),
    number: String, date: String, preferences: () => ({}), savePreferences: () => {}, initializeVisuals: () => {}, syncQuotaUSD: () => {},
    setText: (selector, text) => window.document.querySelectorAll(selector).forEach(element => { element.textContent = text; }),
    toast: (message, tone) => toasts.push({ message, tone }), confirm: async message => { confirmations.push(message); return options.confirm !== false; },
    showBusy: (button, busy) => {
      if (busy) { button.dataset.previousHTML = button.innerHTML; button.textContent = '处理中…'; button.dataset.busy = 'true'; button.disabled = true; }
      else { if (button.dataset.previousHTML) button.innerHTML = button.dataset.previousHTML; delete button.dataset.previousHTML; delete button.dataset.busy; button.disabled = false; }
    },
    request: async (endpoint, request = {}) => {
      requests.push({ endpoint, ...request, ...(request.body ? { body: structuredClone(request.body) } : {}) });
      if (endpoint === '/api/relayfor/activities/theme') return view();
      if (endpoint === '/api/user/self') return { role: 100, quota: 50000, group: 'default' };
      if (endpoint === '/api/subscription/plans' || endpoint === '/api/subscription/admin/plans') {
        if (options.failPlans) throw Error('套餐服务不可用');
        return store.plans.map(plan => ({ plan: structuredClone(plan) }));
      }
      if (endpoint === '/api/user/ldc/info') {
        if (options.failPaymentState) throw Error('支付状态不可用');
        return { enabled: options.paymentEnabled !== false, configured: true, currency: 'LDC', credits_per_unit: '100' };
      }
      if (endpoint === '/api/user/ldc/quote') {
        const plan = store.plans.find(plan => plan.id === request.body.plan_id);
        return { kind: 'subscription', plan_id: plan.id, plan_title: plan.title, credits: (plan.price_amount * 100).toFixed(2), currency: 'LDC' };
      }
      if (endpoint === '/api/subscription/self') return { all_subscriptions: [] };
      if (endpoint === '/api/subscription/ldc/pay') {
        assert.equal(request.method, 'POST');
        assert.ok(confirmations.length, 'purchase must be preceded by explicit confirmation');
        if (options.failPayment) throw Error('LDC 订单创建失败');
        store.purchases.push(request.body);
        return { trade_no: 'LDC-1', kind: 'subscription', status: 'pending', credits: request.body.expected_credits, currency: 'LDC', url: 'https://credit.linux.do/epay/pay/submit.php', method: 'POST', params: { client_id: 'test-client', sign: 'test-signature', money: request.body.expected_credits } };
      }
      if (endpoint === '/api/user/activities' || endpoint === '/api/admin/activities' && !request.method) {
        if (options.failLoad) throw Error('活动网络不可用');
        return view();
      }
      if (endpoint === '/api/user/activities/subscription') {
        assert.equal(request.method, 'PUT');
        assert.equal(typeof request.body.subscribed, 'boolean');
        if (options.failSubscription) throw Error('订阅服务不可用');
        const campaign = store.campaigns.find(item => item.key === request.body.key);
        assert.ok(campaign, 'subscription must target a server-known campaign');
        campaign.subscribed = request.body.subscribed;
        return { key: campaign.key, subscribed: campaign.subscribed };
      }
      if (endpoint === '/api/user/activities/claim') {
        assert.equal(request.method, 'POST');
        if (options.failClaim) throw Error('节日奖励预算已用完');
        const campaign = store.campaigns.find(item => item.key === request.body.key);
        assert.ok(campaign?.active && !campaign.claimed_today);
        campaign.claimed_today = true; campaign.can_claim = false;
        store.history.push({ campaign_key: campaign.key, claim_date: '2026-09-25', reward_quota: campaign.reward_quota });
        return { campaign_key: campaign.key, reward_quota: campaign.reward_quota };
      }
      if (endpoint === '/api/user/checkin') {
        if (request.method === 'POST') { assert.equal(store.checked, false); store.checked = true; return { quota_awarded: 1500, checkin_date: '2026-09-25' }; }
        return { enabled: true, stats: { checked_in_today: store.checked, total_checkins: store.checked ? 8 : 7, total_quota: store.checked ? 9500 : 8000, records: [] } };
      }
      if (endpoint.startsWith('/api/user/checkin?')) {
        assert.equal(endpoint, '/api/user/checkin?turnstile=verified%2Btoken'); assert.equal(request.method, 'POST');
        assert.equal(store.checked, false); store.checked = true; return { quota_awarded: 1500, checkin_date: '2026-09-25' };
      }
      if (endpoint === '/api/user/benefits') return { items: [] };
      if (endpoint === '/api/user/public-benefits/token-loan') return { visible: false };
      if (endpoint === '/api/admin/subscription-benefits') return [];
      if (endpoint === '/api/admin/activities' && request.method === 'PUT') {
        if (options.failSave) throw Error('配置保存失败');
        Object.assign(store.campaigns.find(item => item.key === request.body.key), request.body);
        return request.body;
      }
      if (endpoint === '/api/admin/activities/checkin') return request.body;
      throw Error('Unexpected endpoint ' + endpoint);
    },
  };
  const intervals = [], scheduleInterval = window.setInterval.bind(window);
  window.setInterval = (callback, delay, ...args) => { intervals.push({ callback, delay }); return scheduleInterval(callback, delay, ...args); };
  window.ProxyLan = api;
  if (options.turnstile) window.turnstile = { render: (_host, settings) => { verification = settings; return 'test-widget'; }, remove: () => {}, reset: () => {} };
  window.eval(lunar); window.eval(paidPlans); window.eval(runtime);
  window.document.dispatchEvent(new window.CustomEvent('relayfor:ready'));
  const panel = window.document.querySelector('[data-panel="activities"]');
  panel.classList.add('active');
  const f = { window, panel, api, options, store, requests, toasts, confirmations, paymentForms, view, intervals,
    get verification() { return verification; },
    select: selector => panel.querySelector(selector),
    subscriptions: () => requests.filter(item => item.endpoint === '/api/user/activities/subscription'),
    mutations: endpoint => requests.filter(item => item.endpoint === endpoint && ['POST', 'PUT'].includes(item.method)),
    route: async key => { window.location.hash = '#activities' + (key ? '/' + key : ''); window.dispatchEvent(new window.HashChangeEvent('hashchange')); await tick(); },
    close: async () => { fixtures.delete(f); await window.happyDOM.close(); },
  };
  fixtures.add(f);
  await window.ProxyLanActivities.load(true); await tick();
  return f;
}

const subscribe = f => f.select('[data-activity-festival-page] [data-activity-subscribe="mid_autumn"]');
const claim = f => f.select('[data-activity-festival-page] [data-activity-claim="mid_autumn"]');
const purchase = f => f.select('[data-festival-plan-buy="7"]');
const submit = (f, form) => form.dispatchEvent(new f.window.SubmitEvent('submit', { bubbles: true, cancelable: true, submitter: form.querySelector('button[type="submit"]') }));
const edit = (f, element, value) => { element.value = value; element.dispatchEvent(new f.window.Event('input', { bubbles: true })); };
const syntheticClick = (f, button) => button.dispatchEvent(new f.window.MouseEvent('click', { bubbles: true }));
const tests = [];
function test(name, body) { tests.push({ name, body }); }

test('enabled upcoming festivals remain discoverable and show configured details', async () => {
  const f = await fixture({ key: '' });
  assert.equal(f.window.document.documentElement.dataset.holiday, undefined, 'upcoming campaigns do not activate the live seasonal theme');
  for (const key of keys) {
    assert.ok(f.select(`[href="#activities/${key}"]`), `${key} must remain reachable outside its reward window`);
  }
  await f.route('mid_autumn');
  assert.match(f.select('[data-activity-festival-page]').textContent, /mid_autumn 后台配置标题/);
  assert.match(f.select('[data-activity-festival-page]').textContent, /第一行活动详情/);
  assert.match(f.select('[data-activity-festival-page]').textContent, /每个账号每天签到一次/);
  assert.ok(subscribe(f)); assert.equal(subscribe(f).disabled, false);
  assert.equal(claim(f).disabled, true);
});

test('National Day has a dedicated reachable page with configured activities and national flag art', async () => {
  const f = await fixture({ key: 'national_day' });
  assert.equal(f.select('[data-activity-festival-page] .festival-page').dataset.festival, 'national_day');
  assert.match(f.select('[data-activity-festival-page]').textContent, /国庆/);
  assert.match(f.select('[data-activity-festival-page]').textContent, /national_day 后台配置标题/);
  assert.ok(f.select('[data-activity-festival-page] .scene-national-flag'));
  assert.ok(f.select('[data-activity-festival-page] [data-activity-subscribe="national_day"]'));
});

test('subscribe and cancel use persisted server state across page recreation', async () => {
  const store = serverState();
  const first = await fixture({ store });
  subscribe(first).click(); await tick();
  assert.deepEqual(first.subscriptions().map(item => item.body), [{ key: 'mid_autumn', subscribed: true }]);
  assert.equal(store.campaigns[2].subscribed, true);
  await first.close();
  const next = await fixture({ store });
  assert.match(subscribe(next).textContent, /已订阅|取消订阅/);
  subscribe(next).click(); await tick();
  assert.deepEqual(next.subscriptions().map(item => item.body), [{ key: 'mid_autumn', subscribed: false }]);
  assert.equal(store.campaigns[2].subscribed, false);
  assert.doesNotMatch(subscribe(next).textContent, /已订阅|取消订阅/);
});

test('subscribers can cancel after new subscriptions are disabled', async () => {
  const store = serverState();
  Object.assign(store.campaigns[2], { subscribed: true, subscription_enabled: false, can_subscribe: false });
  const f = await fixture({ store });
  assert.ok(subscribe(f)); assert.equal(subscribe(f).disabled, false);
  subscribe(f).click(); await tick();
  assert.deepEqual(f.subscriptions().map(item => item.body), [{ key: 'mid_autumn', subscribed: false }]);
  assert.equal(store.campaigns[2].subscribed, false);
});

test('failed subscription remains unsubscribed, surfaces the error and can retry', async () => {
  const f = await fixture({ options: { failSubscription: true } });
  subscribe(f).click(); await tick();
  assert.equal(f.store.campaigns[2].subscribed, false);
  assert.doesNotMatch(subscribe(f).textContent, /已订阅|取消订阅/);
  assert.equal(subscribe(f).disabled, false);
  assert.ok(f.toasts.some(item => item.message.includes('订阅服务不可用')));
  assert.ok(!f.toasts.some(item => /订阅成功|已订阅/.test(item.message)));
  f.options.failSubscription = false;
  subscribe(f).click(); await tick();
  assert.equal(f.store.campaigns[2].subscribed, true);
});

test('in-flight subscriptions survive rerender and a stale refresh cannot undo success', async () => {
  const f = await fixture();
  const mutate = deferred(), staleLoad = deferred();
  const original = f.api.request;
  let mutationStarted = false, refreshStarted = false;
  f.api.request = async (endpoint, request) => {
    if (endpoint === '/api/user/activities/subscription') { mutationStarted = true; await mutate.promise; return original(endpoint, request); }
    if (endpoint === '/api/user/activities' && !refreshStarted) {
      refreshStarted = true;
      const stale = f.view();
      await staleLoad.promise;
      return stale;
    }
    return original(endpoint, request);
  };
  subscribe(f).click(); await tick(); assert.equal(mutationStarted, true);
  await f.route('qixi'); await f.route('mid_autumn');
  assert.equal(subscribe(f).disabled, true, 'rerendered control remains busy');
  syntheticClick(f, subscribe(f)); await tick();
  const oldRefresh = f.window.ProxyLanActivities.load(true); await tick();
  mutate.resolve(); await tick();
  staleLoad.resolve(); await oldRefresh; await tick();
  assert.equal(f.subscriptions().length, 1, 'only one subscription mutation is sent');
  assert.equal(f.store.campaigns[2].subscribed, true);
  assert.match(subscribe(f).textContent, /已订阅|取消订阅/);
  assert.equal(subscribe(f).disabled, false);
});

test('failed activities refresh blocks stale subscriptions and awards until recovery', async () => {
  const f = await fixture({ store: serverState(true) });
  f.options.failLoad = true;
  await f.window.ProxyLanActivities.load(true); await tick();
  assert.equal(claim(f).disabled, true);
  assert.equal(subscribe(f).disabled, true);
  syntheticClick(f, claim(f)); syntheticClick(f, subscribe(f)); await tick();
  assert.equal(f.subscriptions().length, 0);
  assert.equal(f.mutations('/api/user/activities/claim').length, 0);
  f.options.failLoad = false;
  await f.window.ProxyLanActivities.load(true); await tick();
  assert.equal(claim(f).disabled, false); assert.equal(subscribe(f).disabled, false);
});

test('festival and main check-in are independently claimable on the same day', async () => {
  const f = await fixture({ store: serverState(true) });
  claim(f).click(); await tick();
  assert.equal(f.mutations('/api/user/activities/claim').length, 1);
  assert.equal(f.mutations('/api/user/checkin').length, 0);
  assert.equal(f.store.checked, false);
  await f.route('');
  const main = f.select('[data-activity-checkin]');
  assert.equal(main.disabled, false); main.click(); await tick();
  assert.equal(f.mutations('/api/user/checkin').length, 1);
  assert.equal(f.store.checked, true);
  assert.equal(f.store.campaigns[2].claimed_today, true);
  await f.route('mid_autumn');
  assert.equal(claim(f).disabled, true);
  syntheticClick(f, claim(f)); await tick();
  assert.equal(f.mutations('/api/user/activities/claim').length, 1);
});

test('failed festival check-in never reports an award or consumes main check-in', async () => {
  const f = await fixture({ store: serverState(true), options: { failClaim: true } });
  claim(f).click(); await tick();
  assert.equal(f.store.checked, false); assert.equal(f.store.campaigns[2].claimed_today, false);
  assert.equal(claim(f).disabled, false);
  assert.ok(f.toasts.some(item => item.message.includes('节日奖励预算已用完')));
  assert.ok(!f.toasts.some(item => /已到账|签到成功/.test(item.message)));
});

test('main check-in still requires a valid Turnstile token without gating festival rewards', async () => {
  const f = await fixture({ store: serverState(true), options: { turnstile: true } });
  assert.equal(claim(f).disabled, false);
  await f.route('');
  const main = f.select('[data-activity-checkin]');
  assert.equal(main.disabled, true); assert.equal(f.verification.sitekey, 'verification-site-key');
  syntheticClick(f, main); await tick();
  assert.equal(f.mutations('/api/user/checkin').length, 0);
  f.verification.callback('expired-token'); assert.equal(main.disabled, false);
  f.verification['expired-callback'](); assert.equal(main.disabled, true);
  f.verification.callback('verified+token'); main.click(); await tick();
  assert.equal(f.mutations('/api/user/checkin?turnstile=verified%2Btoken').length, 1);
  assert.equal(main.disabled, true);
  await f.route('mid_autumn'); assert.equal(claim(f).disabled, false);
});

test('a stale refresh cannot restore an already granted festival reward', async () => {
  const f = await fixture({ store: serverState(true) });
  const mutate = deferred(), staleLoad = deferred(), original = f.api.request;
  let refreshStarted = false;
  f.api.request = async (endpoint, request) => {
    if (endpoint === '/api/user/activities/claim') { await mutate.promise; return original(endpoint, request); }
    if (endpoint === '/api/user/activities' && !refreshStarted) { refreshStarted = true; const stale = f.view(); await staleLoad.promise; return stale; }
    return original(endpoint, request);
  };
  claim(f).click(); await tick();
  const refresh = f.window.ProxyLanActivities.load(true); await tick();
  mutate.resolve(); await tick(); staleLoad.resolve(); await refresh; await tick();
  assert.equal(claim(f).disabled, true);
  assert.equal(f.store.campaigns[2].claimed_today, true);
  syntheticClick(f, claim(f)); await tick();
  assert.equal(f.mutations('/api/user/activities/claim').length, 1);
});

test('server UTC+8 midnight immediately clears yesterday\'s seasonal reward controls', async () => {
  const f = await fixture({ store: serverState(true) });
  const timers = [], originalTimeout = f.window.setTimeout;
  f.window.setTimeout = (callback, delay) => { timers.push({ callback, delay }); return 700 + timers.length; };
  f.store.server_date = '2026-09-26'; f.store.server_time = '2026-09-26T23:59:59+08:00';
  await f.window.ProxyLanActivities.load(true); await tick();
  const boundary = timers.find(timer => timer.delay === 1075);
  assert.ok(boundary, 'the refresh boundary uses server time instead of local device time');
  f.store.server_date = '2026-09-27'; f.store.server_time = '2026-09-27T00:00:00+08:00';
  Object.assign(f.store.campaigns[2], { active: false, can_claim: false, status: 'ended' });
  const requestsBeforeBoundary = f.requests.length;
  boundary.callback();
  assert.equal(claim(f).disabled, true);
  assert.equal(f.window.document.documentElement.dataset.holiday, undefined);
  await tick();
  assert.equal(claim(f).disabled, true);
  assert.equal(f.window.document.querySelector('[data-activity-season-pill]'), null);
  assert.equal(f.requests.length, requestsBeforeBoundary, 'expiry must not reload the open activity page');
  assert.match(f.select('[data-activity-status]').textContent, /手动刷新/);
  f.window.setTimeout = originalTimeout;
});

test('admin saves content, subscriptions and independent festival quota in one campaign mutation', async () => {
  const f = await fixture({ page: 'admin', key: '' });
  const form = f.select('[data-activity-campaign-form="mid_autumn"]');
  edit(f, form.elements.title, '配置后的活动'); edit(f, form.elements.details, '新详情第一行\n新详情第二行'); edit(f, form.elements.rules, '新活动规则');
  edit(f, form.elements.reward_usd, '2.345'); edit(f, form.elements.budget_usd, '21.25');
  form.elements.subscription_enabled.checked = false;
  submit(f, form); await tick();
  const writes = f.mutations('/api/admin/activities'); assert.equal(writes.length, 1);
  assert.equal(writes[0].body.details, '新详情第一行\n新详情第二行'); assert.equal(writes[0].body.rules, '新活动规则');
  assert.equal(writes[0].body.subscription_enabled, false); assert.equal(writes[0].body.reward_quota, 2345); assert.equal(writes[0].body.daily_budget_quota, 21250);
  assert.equal(f.mutations('/api/admin/activities/checkin').length, 0);
});

test('refresh preserves unsaved campaign content and independent main-checkin settings', async () => {
  const f = await fixture({ page: 'admin', key: '' });
  const form = f.select('[data-activity-campaign-form="mid_autumn"]');
  const main = f.select('[data-activity-checkin-form]');
  edit(f, form.elements.details, '尚未保存的活动详情'); edit(f, form.elements.rules, '尚未保存的活动规则'); edit(f, main.elements.min_usd, '0.456');
  await f.window.ProxyLanActivities.load(true); await tick();
  assert.equal(f.select('[data-activity-campaign-form="mid_autumn"]'), form);
  assert.equal(form.elements.details.value, '尚未保存的活动详情'); assert.equal(form.elements.rules.value, '尚未保存的活动规则');
  assert.equal(main.elements.min_usd.value, '0.456');
});

test('failed admin save retains edited content through subsequent refresh', async () => {
  const f = await fixture({ page: 'admin', key: '', options: { failSave: true } });
  const form = f.select('[data-activity-campaign-form="mid_autumn"]');
  edit(f, form.elements.details, '网络失败也应保留的详情');
  submit(f, form); await tick();
  assert.match(form.textContent, /配置保存失败/);
  assert.ok(!f.toasts.some(item => item.message.includes('已保存')));
  await f.window.ProxyLanActivities.load(true); await tick();
  assert.equal(f.select('[data-activity-campaign-form="mid_autumn"]').elements.details.value, '网络失败也应保留的详情');
});

test('admin edits made while save is pending remain unsaved after that save completes', async () => {
  const f = await fixture({ page: 'admin', key: '' });
  const form = f.select('[data-activity-campaign-form="mid_autumn"]');
  const saved = deferred(), original = f.api.request;
  f.api.request = async (endpoint, request) => { if (endpoint === '/api/admin/activities' && request?.method === 'PUT') await saved.promise; return original(endpoint, request); };
  edit(f, form.elements.details, '已提交内容'); submit(f, form); await tick();
  edit(f, form.elements.details, '提交后继续编辑的内容');
  saved.resolve(); await tick();
  await f.window.ProxyLanActivities.load(true); await tick();
  assert.equal(f.store.campaigns[2].details, '已提交内容');
  assert.equal(f.select('[data-activity-campaign-form="mid_autumn"]').elements.details.value, '提交后继续编辑的内容');
});

test('configured details and rules render as text rather than executable markup', async () => {
  const store = serverState();
  store.campaigns[2].details = '<img src=x onerror="window.__festivalInjected=true">';
  store.campaigns[2].rules = '<script>window.__festivalInjected=true</script>';
  const f = await fixture({ store });
  const page = f.select('[data-activity-festival-page]');
  assert.match(page.textContent, /<img src=x/); assert.match(page.textContent, /<script>/);
  assert.equal(page.querySelector('img[onerror], script'), null);
  assert.equal(f.window.__festivalInjected, undefined);
});

test('paid festival plans display server prices and canceling confirmation sends no payment', async () => {
  const f = await fixture({ options: { confirm: false } });
  assert.match(f.select('.festival-plan-card').textContent, /节日相聚套餐/);
  assert.match(f.select('.festival-plan-card').textContent, /9\.90 USD/);
  purchase(f).click(); await tick();
  assert.equal(f.confirmations.length, 1);
  assert.match(f.confirmations[0], /990\.00 LDC/);
  assert.equal(f.mutations('/api/subscription/ldc/pay').length, 0);
  assert.equal(purchase(f).disabled, false);
  assert.equal(f.store.purchases.length, 0);
});

test('payment confirms the exact server quote and does not affect either check-in', async () => {
  const store = serverState(true);
  store.plans[0].price_amount = 0.001;
  const f = await fixture({ store });
  purchase(f).click(); await tick();
  const writes = f.mutations('/api/subscription/ldc/pay');
  assert.equal(writes.length, 1);
  assert.deepEqual(writes[0].body, { plan_id: 7, expected_credits: '0.10' });
  assert.equal(store.checked, false); assert.equal(store.campaigns[2].claimed_today, false);
  assert.match(f.select('[data-festival-plan-feedback="7"]').textContent, /订单已创建/);
  assert.ok(!f.toasts.some(item => item.message.includes('已开通')));
  assert.equal(f.paymentForms.length, 1);
  assert.equal(f.paymentForms[0].params.money, '0.10');
  assert.equal(f.paymentForms[0].params.sign, 'test-signature');
});

test('purchase is locked throughout confirmation, payment and route rerenders', async () => {
  const f = await fixture();
  const consent = deferred(), payment = deferred();
  const originalConfirm = f.api.confirm, originalRequest = f.api.request;
  let consentCount = 0;
  f.api.confirm = async message => { consentCount++; await consent.promise; return originalConfirm(message); };
  f.api.request = async (endpoint, request) => { if (endpoint === '/api/subscription/ldc/pay') await payment.promise; return originalRequest(endpoint, request); };
  purchase(f).click(); await tick();
  syntheticClick(f, purchase(f)); await tick();
  assert.equal(consentCount, 1);
  assert.equal(purchase(f).disabled, true);
  await f.route('qixi'); await f.route('mid_autumn');
  assert.equal(purchase(f).disabled, true, 'route rendering must not permit a second purchase while confirmation is pending');
  consent.resolve(); await tick();
  syntheticClick(f, purchase(f)); await tick();
  assert.equal(consentCount, 1);
  payment.resolve(); await tick();
  assert.equal(f.mutations('/api/subscription/ldc/pay').length, 1);
  assert.equal(f.store.purchases.length, 1);
  assert.equal(purchase(f).disabled, false);
});

test('failed payment reports the server error without inventing purchase success', async () => {
  const f = await fixture({ options: { failPayment: true } });
  purchase(f).click(); await tick();
  assert.equal(f.store.purchases.length, 0);
  assert.match(f.select('[data-festival-plan-feedback="7"]').textContent, /LDC 订单创建失败/);
  assert.ok(!f.toasts.some(item => item.message.includes('已开通')));
  assert.equal(purchase(f).disabled, false);
});

test('unavailable payment state blocks purchases while free festival actions remain available', async () => {
  const f = await fixture({ options: { failPaymentState: true } });
  assert.equal(purchase(f).disabled, true);
  syntheticClick(f, purchase(f)); await tick();
  assert.equal(f.confirmations.length, 0); assert.equal(f.mutations('/api/subscription/ldc/pay').length, 0);
  assert.equal(subscribe(f).disabled, false);
  subscribe(f).click(); await tick();
  assert.equal(f.store.campaigns[2].subscribed, true);
});

test('admin saves selected paid-plan IDs and preserves associations when plan loading fails', async () => {
  const store = serverState();
  store.plans.push({ ...store.plans[0], id: 8, title: '另一个节日套餐' });
  const f = await fixture({ page: 'admin', key: '', store });
  const form = f.select('[data-activity-campaign-form="mid_autumn"]');
  const oldPlan = form.querySelector('input[name="festival_plan_ids"][value="7"]');
  const newPlan = form.querySelector('input[name="festival_plan_ids"][value="8"]');
  assert.equal(oldPlan.checked, true);
  oldPlan.checked = false; oldPlan.dispatchEvent(new f.window.Event('change', { bubbles: true }));
  newPlan.checked = true; newPlan.dispatchEvent(new f.window.Event('change', { bubbles: true }));
  await f.window.ProxyLanActivities.load(true); await tick();
  assert.equal(f.select('[data-activity-campaign-form="mid_autumn"]'), form);
  submit(f, form); await tick();
  assert.deepEqual(f.mutations('/api/admin/activities')[0].body.subscription_plan_ids, [8]);
  await f.close();
  const offline = await fixture({ page: 'admin', key: '', store, options: { failPlans: true } });
  const offlineForm = offline.select('[data-activity-campaign-form="mid_autumn"]');
  edit(offline, offlineForm.elements.title, '只修改活动标题');
  submit(offline, offlineForm); await tick();
  assert.deepEqual(offline.mutations('/api/admin/activities')[0].body.subscription_plan_ids, [8]);
});

test('re-entering, focusing and interval ticks never automatically reload an open activities page', async () => {
  const f = await fixture({ store: serverState(true) });
  const hero = f.select('[data-activity-festival-page] .festival-hero-v2');
  const reads = () => f.requests.filter(item => item.endpoint === '/api/user/activities').length;
  const before = reads(), allRequests = f.requests.length;
  for (let index = 0; index < 3; index++) {
    f.window.dispatchEvent(new f.window.CustomEvent('proxylan:page', { detail: { name: 'activities' } }));
    f.window.dispatchEvent(new f.window.Event('focus'));
    f.window.document.dispatchEvent(new f.window.Event('visibilitychange'));
    f.intervals.forEach(timer => timer.callback());
  }
  await f.window.ProxyLanActivities.load(); await tick();
  assert.equal(reads(), before);
  assert.equal(f.requests.length, allRequests, 'neither activity nor theme endpoints poll while this page is open');
  assert.equal(f.select('[data-activity-festival-page] .festival-hero-v2'), hero);
  assert.equal(f.panel.hasAttribute('aria-busy'), false);
});

test('unchanged explicit refresh preserves hero, focus and open details without loading placeholders', async () => {
  const f = await fixture({ store: serverState(true) });
  const hero = f.select('[data-activity-festival-page] .festival-hero-v2');
  const gallery = f.select('.festival-gallery-card');
  const rules = f.select('.festival-rules details'); rules.open = false;
  const button = subscribe(f); button.focus();
  const wait = deferred(), request = f.api.request;
  f.store.server_time = '2026-09-25T12:00:05+08:00';
  f.api.request = async (endpoint, options) => { if (endpoint === '/api/user/activities') await wait.promise; return request(endpoint, options); };
  const refresh = f.window.ProxyLanActivities.load(true);
  await tick();
  assert.equal(f.panel.hasAttribute('aria-busy'), false);
  assert.equal(claim(f).disabled, false, 'a routine explicit read keeps the last confirmed state until a response or error');
  assert.doesNotMatch(f.select('[data-activity-status]').textContent, /正在读取/);
  wait.resolve(); await refresh; await tick();
  assert.equal(f.select('[data-activity-festival-page] .festival-hero-v2'), hero);
  assert.equal(f.select('.festival-gallery-card'), gallery);
  assert.equal(f.select('.festival-rules details'), rules);
  assert.equal(rules.open, false);
  assert.equal(f.window.document.activeElement, button);
});

test('manual refresh bypasses the entry cache and mutation refreshes still read awarded state', async () => {
  const f = await fixture({ store: serverState(true) });
  const reads = () => f.requests.filter(item => item.endpoint === '/api/user/activities').length;
  const initial = reads();
  await f.window.ProxyLanActivities.load(); await tick();
  assert.equal(reads(), initial);
  f.select('[data-activity-festival-page] [data-activity-refresh]').click(); await tick();
  assert.equal(reads(), initial + 1);
  claim(f).click(); await tick();
  assert.equal(reads(), initial + 2);
  assert.equal(claim(f).disabled, true);
  assert.equal(f.store.campaigns[2].claimed_today, true);
});

let passed = 0;
const failures = [];
for (const { name, body } of tests) {
  try { await body(); passed++; console.log('PASS ' + name); }
  catch (error) { failures.push({ name, error }); console.error('FAIL ' + name + '\n' + error.stack); }
  finally { for (const f of [...fixtures]) await f.close(); }
}
console.log(`${passed}/${tests.length} festival rebuild contract scenarios passed.`);
if (failures.length) process.exitCode = 1;
