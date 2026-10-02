import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import { Window } from '../web/node_modules/happy-dom/lib/index.js';

const root = new URL('../', import.meta.url);
const script = await fs.readFile(new URL('relayfor-ui/assets/activities.js', root), 'utf8');
const lunar = await fs.readFile(new URL('relayfor-ui/assets/lunar-schedule.js', root), 'utf8');
const html = await fs.readFile(new URL('relayfor-ui/console/index.html', root), 'utf8');
const keys = ['duanwu', 'qixi', 'mid_autumn', 'chongyang', 'new_year_eve'];
const labels = ['端午', '七夕', '中秋', '重阳', '除夕'];
const dates = ['农历五月初五', '农历七月初七', '农历八月十五', '农历九月初九', '农历一年最后一天'];
const tick = async () => { await new Promise(resolve => setTimeout(resolve, 15)); };
const escapeHTML = value => String(value ?? '').replace(/[&<>"']/g, ch => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch]));

async function fixture(key = '', options = {}) {
  const window = new Window({ url: 'http://localhost/console/#activities' + (key ? '/' + key : ''), settings: {disableCSSFileLoading:true, disableJavaScriptFileLoading:true} });
  window.document.write(html);
  const requests = [];
  const campaign = {key: options.activeKey || key || 'mid_autumn', title:'节日心意', description:'来自服务器的说明', date:'2026-09-25', active:true, can_claim:true, claimed_today:false, reward_quota:1250, starts_at:'2026-09-24T00:00:00+08:00', ends_at:'2026-09-27T00:00:00+08:00'};
  const data = {server_date: options.serverDate || '2026-09-25', theme:campaign.key, campaigns:[campaign], checkin:{enabled:false}, history:[]};
  window.ProxyLan = {
    escapeHTML, moneyFromQuota: v => '$' + (v / 1000).toFixed(2), number:String, date:String,
    preferences:()=>({}), savePreferences:()=>{}, initializeVisuals:()=>{}, setText:()=>{}, toast:()=>{},
    showBusy:(button,busy)=>{button.dataset.busy=String(busy);button.disabled=busy;},
    request:async(endpoint, request={})=>{
      requests.push({endpoint,...request});
      if(endpoint === '/api/user/activities' && options.failLoad) throw Error('offline');
      if(endpoint === '/api/user/activities' || endpoint === '/api/relayfor/activities/theme') return data;
      if(endpoint === '/api/user/benefits') return {items:[]};
      if(endpoint === '/api/user/public-benefits/token-loan') return {visible:false};
      if(endpoint === '/api/user/self') return {quota:5000};
      if(endpoint === '/api/user/activities/claim') { campaign.claimed_today=true;campaign.can_claim=false;return {reward_quota:1250}; }
      throw Error('Unexpected endpoint '+endpoint);
    }
  };
  window.eval(lunar); window.eval(script);
  window.document.dispatchEvent(new window.CustomEvent('relayfor:ready'));
  const panel = window.document.querySelector('[data-panel="activities"]');
  panel.classList.add('active');
  await window.ProxyLanActivities.load(); await tick();
  return {window,panel,data,campaign,options,requests,route:async key=>{window.location.hash='#activities'+(key?'/'+key:'');window.dispatchEvent(new window.HashChangeEvent('hashchange'));await tick();},close:()=>window.happyDOM.close()};
}

for (const [index,key] of keys.entries()) {
  const f = await fixture(key);
  assert.equal(f.panel.querySelector('[data-activity-center]').hidden,true);
  assert.equal(f.panel.querySelector('[data-activity-festival-page]').hidden,false);
  assert.equal(f.panel.querySelector('.festival-page').dataset.festival,key);
  assert.equal(f.panel.dataset.title,labels[index]+'专场');
  assert.ok(f.panel.querySelector('.festival-lunar').textContent.includes(dates[index]));
  assert.equal(f.panel.querySelectorAll('.festival-custom').length,3);
  assert.ok(f.panel.querySelector('.festival-scene path'));
  assert.equal(f.panel.querySelector('.festival-claim').disabled,false);
  assert.match(f.panel.querySelector('.festival-reward-note').textContent,/09月24日 — 09月26日/);
  f.panel.querySelector('.festival-claim').click(); await tick();
  assert.equal(f.requests.filter(r=>r.endpoint==='/api/user/activities/claim').length,1);
  assert.equal(f.panel.querySelector('.festival-claim').disabled,true);
  f.panel.querySelector('.festival-claim').dispatchEvent(new f.window.MouseEvent('click',{bubbles:true}));await tick();
  assert.equal(f.requests.filter(r=>r.endpoint==='/api/user/activities/claim').length,1);
  await f.route('');
  assert.equal(f.panel.querySelector('[data-activity-center]').hidden,false);
  assert.equal(f.panel.querySelector('[data-activity-festival-page]').hidden,true);
  assert.ok(f.panel.querySelector('[data-activity-checkin]'));
  assert.equal(f.panel.querySelectorAll('.activity-culture-link').length,5);
  await f.close();
  console.log('PASS '+key+' direct link, cultural content, real claim, duplicate prevention, return');
}

{
  const f = await fixture('mid_autumn');
  f.data.campaigns=[]; f.data.theme=''; f.data.server_date='2026-09-28';
  await f.window.ProxyLanActivities.load();
  assert.match(f.panel.querySelector('.festival-state').textContent,/本年度节日已过/);
  assert.equal(f.panel.querySelector('.festival-claim').disabled,true);
  assert.equal(f.panel.querySelectorAll('.activity-festival-card').length,0);
  assert.equal(f.window.document.documentElement.dataset.holiday,undefined);
  assert.equal(f.window.document.querySelector('[data-activity-season-pill]'),null);
  await f.route('chongyang');
  assert.match(f.panel.querySelector('.festival-state').textContent,/活动尚未开始/);
  assert.match(f.panel.querySelector('.festival-hero-meta').textContent,/2026.10.18/);
  f.panel.querySelector('.festival-claim').dispatchEvent(new f.window.MouseEvent('click',{bubbles:true}));await tick();
  assert.equal(f.requests.filter(r=>r.endpoint==='/api/user/activities/claim').length,0);
  await f.route('unknown');
  assert.match(f.panel.querySelector('[data-activity-festival-page]').textContent,/暂时没有这个节日专场/);
  assert.equal(f.panel.querySelector('.festival-claim'),null);
  await f.close(); console.log('PASS expired and upcoming deep links, no inactive awards, theme removal, unknown route');
}

{
  const f = await fixture('mid_autumn');
  f.options.failLoad=true;
  await f.window.ProxyLanActivities.load();
  assert.match(f.panel.querySelector('.festival-state').textContent,/活动状态待确认/);
  assert.equal(f.panel.querySelector('.festival-claim').disabled,true);
  f.panel.querySelector('.festival-claim').dispatchEvent(new f.window.MouseEvent('click',{bubbles:true}));await tick();
  assert.equal(f.requests.filter(r=>r.endpoint==='/api/user/activities/claim').length,0);
  assert.match(f.panel.querySelector('[data-activity-status]').textContent,/offline/);
  f.options.failLoad=false;
  await f.window.ProxyLanActivities.load();
  assert.equal(f.panel.querySelector('.festival-claim').disabled,false);
  await f.close(); console.log('PASS failed refresh blocks stale awards and recovery restores claim');
}

{
  const f = await fixture('mid_autumn');
  const originalRequest = f.window.ProxyLan.request;
  let releaseClaim, releaseOldLoad, staleRefreshStarted = false;
  f.window.ProxyLan.request = async (endpoint,request={}) => {
    if(endpoint === '/api/user/activities/claim') {
      await new Promise(resolve=>{releaseClaim=resolve;});
      return originalRequest(endpoint,request);
    }
    if(endpoint === '/api/user/activities' && !staleRefreshStarted) {
      staleRefreshStarted=true;
      const stale = JSON.parse(JSON.stringify(f.data));
      await new Promise(resolve=>{releaseOldLoad=resolve;});
      return stale;
    }
    return originalRequest(endpoint,request);
  };
  f.panel.querySelector('.festival-claim').click(); await tick();
  const oldRefresh = f.window.ProxyLanActivities.load(); await tick();
  releaseClaim(); await tick();
  releaseOldLoad(); await oldRefresh; await tick();
  assert.equal(f.panel.querySelector('.festival-claim').disabled,true);
  assert.match(f.panel.querySelector('.festival-claim').textContent,/今日已领取/);
  assert.equal(f.requests.filter(r=>r.endpoint==='/api/user/activities/claim').length,1);
  assert.equal(f.requests.filter(r=>r.endpoint==='/api/user/activities').length,2);
  await f.close(); console.log('PASS award forces a fresh read after an older in-flight refresh');
}

{
  const f = await fixture('');
  const icons = await fs.readFile(new URL('relayfor-ui/assets/material-icons.js',root),'utf8');
  f.window.eval(icons);
  f.data.checkin = {enabled:true,min_quota:1000,max_quota:1000};
  const originalRequest = f.window.ProxyLan.request;
  f.window.ProxyLan.request = (endpoint,request) => endpoint === '/api/user/checkin' ? Promise.resolve({stats:{checked_in_today:false,records:[]}}) : originalRequest(endpoint,request);
  const button = f.panel.querySelector('[data-activity-checkin]');
  const ripple = f.window.document.createElement('span'); ripple.className='md-ripple';button.append(ripple);
  await f.window.ProxyLanActivities.load();await tick();
  assert.equal(button.querySelector('[data-activity-checkin-label]').textContent,'签到领取');
  assert.equal(ripple.textContent,'');
  assert.ok(button.querySelector('.material-symbol svg path'));
  f.options.failLoad=true;
  await f.window.ProxyLanActivities.load();
  assert.equal(button.disabled,true);
  assert.ok(button.querySelector('.material-symbol svg path'));
  await f.close(); console.log('PASS check-in label survives ripple and SVG replacement; failed refresh disables it');
}

{
  const f = await fixture('mid_autumn');
  const timers=[];
  f.window.setTimeout=(callback,delay)=>{timers.push({callback,delay});return 700+timers.length;};
  f.data.server_time='2026-09-26T23:59:59+08:00';
  f.data.server_date='2026-09-26';
  await f.window.ProxyLanActivities.load();
  const midnight=timers.find(timer=>timer.delay===1075);
  assert.ok(midnight,'refresh is scheduled at the server UTC+8 date boundary');
  assert.match(f.panel.querySelector('.activity-festival-footer').textContent,/09月26日 结束/);
  f.data.server_time='2026-09-27T00:00:00+08:00';
  f.data.server_date='2026-09-27';
  f.data.theme='';f.data.campaigns=[];
  midnight.callback();
  assert.equal(f.panel.querySelector('.festival-claim').disabled,true);
  assert.equal(f.window.document.documentElement.dataset.holiday,undefined);
  await tick();
  assert.equal(f.panel.querySelectorAll('.activity-festival-card').length,0);
  assert.match(f.panel.querySelector('.festival-state').textContent,/本年度节日已过/);
  await f.close();console.log('PASS server UTC+8 midnight expires cards, theme and claim controls immediately');
}
