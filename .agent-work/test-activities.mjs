import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { Window } from '../web/node_modules/happy-dom/lib/index.js';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const script = await fs.readFile(path.join(root, 'relayfor-ui/assets/activities.js'), 'utf8');
const lunar = await fs.readFile(path.join(root, 'relayfor-ui/assets/lunar-schedule.js'), 'utf8');
const originalHtml = {
  console: await fs.readFile(path.join(root, 'relayfor-ui/console/index.html'), 'utf8'),
  admin: await fs.readFile(path.join(root, 'relayfor-ui/admin/index.html'), 'utf8'),
};
const tick = async () => { for (let i = 0; i < 5; i++) await Promise.resolve(); await new Promise(resolve => setTimeout(resolve, 10)); };

async function fixture(page, options = {}) {
  const window = new Window({url: 'http://localhost/' + page + '/#benefits', settings: {disableCSSFileLoading: true, disableJavaScriptFileLoading: true, disableIframePageLoading: true}});
  window.document.write(originalHtml[page]);
  const requests = [], toasts = [];
  let checked = false, claimed = false, benefitClaimed = false;
  let renderOptions;
  const campaigns = ['duanwu', 'qixi', 'mid_autumn', 'chongyang', 'new_year_eve'].map((key, index) => ({key, title: key + ' title', description: '真实说明', date: '2026-09-25', enabled: true, days_before: 0, days_after: 0, reward_quota: 1250, daily_budget_quota: 0, daily_claim_limit: 0, target_group: '', today_claims: 0, today_awarded_quota: 0, active: index === 2, can_claim: true, claimed_today: false, ends_at: '2026-09-25T23:59:59+08:00'}));
  const view = () => ({server_date: '2026-09-25', server_time: '2026-09-25T12:00:00+08:00', timezone: 'Asia/Shanghai', theme: options.noTheme ? '' : 'mid_autumn', calendar_min_year: 2025, calendar_max_year: 2050, campaigns: campaigns.map(item => ({...item, can_claim: item.key === 'mid_autumn' ? !claimed : item.can_claim, claimed_today: item.key === 'mid_autumn' && claimed})), checkin: {enabled: true, min_quota: 1000, max_quota: 2000, turnstile_required: Boolean(options.turnstile), turnstile_site_key: options.turnstile ? 'test-site-key' : ''}, history: claimed ? [{campaign_key: 'mid_autumn', claim_date: '2026-09-25', reward_quota: 1250}] : []});
  const quotaUnit = options.quotaUnit || 1000;
  const api = {
    escapeHTML: value => String(value ?? '').replace(/[&<>"']/g, character => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[character])),
    moneyFromQuota: (value, digits = 2) => '$' + (Number(value || 0) / quotaUnit).toFixed(digits),
    quotaFromUSD: value => Math.round(Number(value) * quotaUnit),
    usdFromQuota: value => String(Number(value) / quotaUnit),
    number: value => String(value), date: value => String(value),
    preferences: () => ({defaultPage: 'benefits'}), savePreferences: preferences => { api.savedPreferences = preferences; },
    setText: (selector, value) => window.document.querySelectorAll(selector).forEach(element => {element.textContent = value;}),
    showBusy: (button, busy) => {if (busy) {button.dataset.previousHTML = button.innerHTML; button.replaceChildren(); button.textContent = '处理中…'; button.dataset.busy = 'true'; button.disabled = true;} else {if (button.dataset.previousHTML) button.innerHTML = button.dataset.previousHTML; delete button.dataset.busy; delete button.dataset.previousHTML; button.disabled = false;}},
    toast: (message, tone) => toasts.push({message, tone}),
    confirm: async () => true, initializeVisuals: () => {}, syncQuotaUSD: () => {},
    openDialog: id => window.document.getElementById(id).classList.add('show'),
    request: async (endpoint, request = {}) => {
      requests.push({endpoint, ...request});
      if (endpoint === '/api/relayfor/activities/theme') return view();
      if (endpoint === '/api/user/self') return {role: options.role ?? 100, quota: 5000};
      if (endpoint === '/api/user/activities' || (endpoint === '/api/admin/activities' && !request.method)) return view();
      if (endpoint === '/api/user/checkin') {
        if (request.method === 'POST') {checked = true; return {quota_awarded: 1500, checkin_date: '2026-09-25'};}
        return {enabled: true, min_quota: 1000, max_quota: 2000, stats: {checked_in_today: checked, total_checkins: checked ? 8 : 7, total_quota: checked ? 9500 : 8000, records: [{checkin_date: '2026-09-24', quota_awarded: 1000}]}};
      }
      if (endpoint.startsWith('/api/user/checkin?')) {assert.equal(request.method, 'POST'); assert.equal(endpoint, '/api/user/checkin?turnstile=verified%2Btoken'); checked = true; return {quota_awarded: 1500, checkin_date: '2026-09-25'};}
      if (endpoint === '/api/user/activities/claim') {if(options.failClaim) throw new Error('本日预算已用完'); claimed = true; return {campaign_key: request.body.key, reward_quota: 1250};}
      if (endpoint === '/api/user/benefits') return {items: [{id: 11, name: '套餐奖励', plan_name: 'Annual', permanent_quota: 3000, can_claim: !benefitClaimed, status: benefitClaimed ? 2 : 1}]};
      if (endpoint === '/api/user/benefits/11/claim') {benefitClaimed = true; return {id: 11};}
      if (endpoint === '/api/user/benefits/11') return {};
      if (endpoint === '/api/user/public-benefits/token-loan') return {visible: false};
      if (endpoint === '/api/admin/subscription-benefits') return [{id: 11, name: '会员福利', plan_name: 'Annual', recipient_count: 5, pending_count: 3, claimed_count: 2}];
      if (endpoint === '/api/admin/subscription-benefits/11/pending') return {revoked_count: 3};
      if (endpoint === '/api/admin/subscription-plans') return [{id: 7, name: '真实套餐', code: 'actual-plan'}];
      if (endpoint === '/api/admin/activities' && request.method === 'PUT') {Object.assign(campaigns.find(item => item.key === request.body.key), request.body); return request.body;}
      if (endpoint === '/api/admin/activities/checkin') return request.body;
      throw new Error('Unexpected endpoint ' + endpoint);
    },
  };
  window.ProxyLan = api;
  if (options.turnstile) window.turnstile = {render: (_container, settings) => {renderOptions = settings; return 'native-widget';}, remove: () => {}, reset: () => {}};
  window.eval(lunar); window.eval(script);
  const loan = window.document.querySelector('[data-token-loan-card]');
  const benefitRows = window.document.querySelector(page === 'admin' ? '[data-subscription-benefit-campaign-rows]' : '[data-user-benefit-rows]');
  const oldLoanButton = window.document.querySelector('[data-token-loan-checkin]');
  let existingLoanClicks = 0;
  if(oldLoanButton) oldLoanButton.addEventListener('click', () => {existingLoanClicks++;});
  window.document.dispatchEvent(new window.CustomEvent('relayfor:ready'));
  const panel = window.document.querySelector('[data-panel="activities"]'); panel.classList.add('active');
  await window.ProxyLanActivities.load(); await tick();
  return {window, api, panel, requests, toasts, loan, benefitRows, oldLoanButton, get existingLoanClicks(){return existingLoanClicks;}, get renderOptions(){return renderOptions;}, close: async () => {await window.happyDOM.close();}};
}

let completed = 0;
async function test(name, body) { await body(); completed++; console.log('PASS ' + name); }

await test('single entrance migration retains original loan/benefit nodes and listeners', async () => {
  const f = await fixture('console'); const doc = f.window.document;
  assert.equal(f.window.location.hash, '#activities');
  assert.equal(doc.querySelectorAll('.nav-rail [data-nav="activities"]').length, 1);
  assert.equal(doc.querySelectorAll('[data-panel="benefits"], [data-nav="benefits"], [data-nav="checkin"]').length, 0);
  assert.equal(f.api.savedPreferences.defaultPage, 'activities');
  assert.ok(f.panel.contains(f.loan)); assert.ok(f.panel.contains(f.benefitRows));
  f.oldLoanButton.disabled = false; f.oldLoanButton.click(); assert.equal(f.existingLoanClicks, 1);
  assert.equal(f.panel.querySelectorAll('iframe').length, 0); await f.close();
});
await test('only server-active holiday renders, actual award and history refresh after claim', async () => {
  const f = await fixture('console');
  assert.equal(f.window.document.documentElement.dataset.holiday, 'mid_autumn');
  assert.equal(f.panel.querySelectorAll('[data-activity-claim]').length, 1);
  assert.equal(f.panel.querySelector('[data-activity-claim]').dataset.activityClaim, 'mid_autumn');
  f.panel.querySelector('[data-activity-claim]').click(); await tick();
  assert.equal(f.requests.filter(item => item.endpoint === '/api/user/activities/claim').length, 1);
  assert.equal(JSON.stringify(f.requests.find(item => item.endpoint === '/api/user/activities/claim').body), JSON.stringify({key:'mid_autumn'}));
  assert.equal(f.panel.querySelector('[data-activity-claim]').disabled, true);
  assert.match(f.panel.querySelector('[data-activity-history]').textContent, /2026-09-25/);
  assert.ok(f.toasts.some(item => item.message.includes('$1.25'))); await f.close();
});
await test('failed claim shows server error and never reports success', async () => {
  const f = await fixture('console', {failClaim:true});
  f.panel.querySelector('[data-activity-claim]').click(); await tick();
  assert.match(f.panel.querySelector('[data-activity-status]').textContent, /本日预算已用完/);
  assert.ok(!f.toasts.some(item => item.message.includes('已到账')));
  assert.equal(f.panel.querySelector('[data-activity-claim]').disabled, false); await f.close();
});
await test('real daily checkin API prevents second claim and updates recorded total', async () => {
  const f = await fixture('console'); const button = f.panel.querySelector('[data-activity-checkin]');
  assert.equal(button.disabled, false); button.click(); await tick();
  assert.equal(f.requests.filter(item => item.endpoint === '/api/user/checkin' && item.method === 'POST').length, 1);
  assert.equal(button.disabled, true); assert.match(button.textContent, /今日已签到/);
  assert.match(f.panel.querySelector('[data-activity-checkin-total]').textContent, /\$9.50/);
  button.dispatchEvent(new f.window.MouseEvent('click', {bubbles:true})); await tick();
  assert.equal(f.requests.filter(item => item.endpoint === '/api/user/checkin' && item.method === 'POST').length, 1); await f.close();
});
await test('Turnstile required: token gated, expired token blocked, verified token forwarded', async () => {
  const f = await fixture('console', {turnstile:true}); const button = f.panel.querySelector('[data-activity-checkin]');
  assert.equal(button.disabled, true); assert.equal(f.renderOptions.sitekey, 'test-site-key');
  button.dispatchEvent(new f.window.MouseEvent('click', {bubbles:true})); await tick();
  assert.equal(f.requests.filter(item => item.endpoint.startsWith('/api/user/checkin') && item.method === 'POST').length, 0);
  f.renderOptions.callback('old-token'); assert.equal(button.disabled, false);
  f.renderOptions['expired-callback'](); assert.equal(button.disabled, true);
  f.renderOptions.callback('verified+token'); button.click(); await tick();
  assert.ok(f.requests.some(item => item.endpoint === '/api/user/checkin?turnstile=verified%2Btoken' && item.method === 'POST'));
  assert.equal(button.disabled, true); await f.close();
});
await test('user subscription benefit uses real claim endpoint and then shows claimed state', async () => {
  const f = await fixture('console'); f.panel.querySelector('[data-activity-benefit-claim]').click(); await tick();
  assert.ok(f.requests.some(item => item.endpoint === '/api/user/benefits/11/claim' && item.method === 'POST'));
  assert.equal(f.panel.querySelector('[data-activity-benefit-claim]'), null);
  assert.match(f.panel.querySelector('[data-user-benefit-rows]').textContent, /已领取/); await f.close();
});
await test('root configuration persists exact shared-helper quota and legacy benefit opener fetches plans', async () => {
  const f = await fixture('admin'); const form = f.panel.querySelector('[data-activity-campaign-form="mid_autumn"]');
  assert.ok(f.panel.contains(f.benefitRows));
  assert.equal(f.window.document.querySelector('[data-open-dialog="subscription-benefit-dialog"]'), null);
  form.elements.reward_usd.value = '2.345'; form.elements.budget_usd.value = '21.25'; form.elements.days_before.value = '3';
  form.dispatchEvent(new f.window.SubmitEvent('submit', {bubbles:true,cancelable:true,submitter:form.querySelector('button')})); await tick();
  const put = f.requests.find(item => item.endpoint === '/api/admin/activities' && item.method === 'PUT');
  assert.equal(put.body.reward_quota, 2345); assert.equal(put.body.daily_budget_quota, 21250); assert.equal(put.body.days_before, 3);
  f.panel.querySelector('[data-activity-benefit-open]').click(); await tick();
  assert.ok(f.requests.some(item => item.endpoint === '/api/admin/subscription-plans'));
  assert.match(f.window.document.querySelector('[data-benefit-plan]').textContent, /真实套餐/);
  assert.ok(f.window.document.getElementById('subscription-benefit-dialog').classList.contains('show'));
  const checkinForm = f.panel.querySelector('[data-activity-checkin-form]'); checkinForm.elements.min_usd.value = '3'; checkinForm.elements.max_usd.value = '1';
  checkinForm.dispatchEvent(new f.window.SubmitEvent('submit', {bubbles:true,cancelable:true,submitter:checkinForm.querySelector('button')})); await tick();
  assert.ok(!f.requests.some(item => item.endpoint === '/api/admin/activities/checkin' && item.method === 'PUT'));
  assert.match(checkinForm.textContent, /最高奖励不能低于/); await f.close();
});
await test('nonroot cannot load root configuration and configuration form stays disabled', async () => {
  const f = await fixture('admin', {role:50});
  assert.equal(f.requests.filter(item => item.endpoint === '/api/admin/activities').length, 0);
  assert.equal(f.panel.querySelector('[data-activity-checkin-form] fieldset').disabled, true);
  assert.equal(f.panel.querySelector('[data-activity-benefit-open]').disabled, true);
  assert.match(f.panel.querySelector('[data-activity-status]').textContent, /仅超级管理员/); await f.close();
});
await test('refresh preserves unsaved admin inputs while server theme can end', async () => {
  const options = {}; const f = await fixture('admin', options);
  const form = f.panel.querySelector('[data-activity-campaign-form="mid_autumn"]');
  form.elements.title.value = '尚未保存的标题'; form.elements.title.dispatchEvent(new f.window.Event('input', {bubbles:true}));
  const checkinForm = f.panel.querySelector('[data-activity-checkin-form]');
  checkinForm.elements.min_usd.value = '0.876'; checkinForm.elements.min_usd.dispatchEvent(new f.window.Event('input', {bubbles:true}));
  await f.window.ProxyLanActivities.load();
  assert.equal(f.panel.querySelector('[data-activity-campaign-form="mid_autumn"]'), form);
  assert.equal(form.elements.title.value, '尚未保存的标题'); assert.equal(checkinForm.elements.min_usd.value, '0.876');
  options.noTheme = true; await f.window.ProxyLanActivities.refreshTheme();
  assert.equal(f.window.document.documentElement.dataset.holiday, undefined);
  assert.equal(f.window.document.querySelector('[data-activity-season-pill]'), null); await f.close();
});
await test('small real quota awards retain precision instead of displaying zero dollars', async () => {
  const f = await fixture('console', {quotaUnit:500000});
  assert.match(f.panel.querySelector('[data-activity-checkin-range]').textContent, /\$0\.002 – \$0\.004/);
  assert.match(f.panel.querySelector('.activity-festival-reward').textContent, /\$0\.0025/);
  f.panel.querySelector('[data-activity-claim]').click(); await tick();
  assert.ok(f.toasts.some(item => item.message.includes('$0.0025'))); await f.close();
});
console.log(`${completed} meaningful activities UI interactions passed.`);
