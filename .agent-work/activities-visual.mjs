// Isolated UI fixture only. Product assets continue to use real server APIs.
import fs from 'node:fs/promises';
import http from 'node:http';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const festivals = [
  {key:'duanwu',title:'端午 · 粽享好时光',date:'2026-06-19'},
  {key:'qixi',title:'七夕 · 与灵感相遇',date:'2026-08-19'},
  {key:'mid_autumn',title:'中秋 · 月满好礼',date:'2026-09-25'},
  {key:'chongyang',title:'重阳 · 登高望远',date:'2026-10-18'},
  {key:'new_year_eve',title:'除夕 · 辞旧迎新',date:'2026-02-16'}
].map(item=>({...item,enabled:true,active:item.key==='mid_autumn',can_claim:item.key==='mid_autumn',claimed_today:false,description:'把这份小小的心意，留给下一次灵感与创造。',reward_quota:1000,daily_claim_limit:1000,daily_budget_quota:1000000,target_group:'',days_before:0,days_after:0,today_claims:183,today_awarded_quota:183000,ends_at:'2026-09-25T23:59:59+08:00'}));
festivals.forEach(item=>Object.assign(item,{subscription_enabled:true,subscribed:item.key==='chongyang',can_subscribe:true,next_date:item.date,starts_at:item.date+'T00:00:00+08:00',status:item.active?'active':'upcoming',subscription_plan_ids:[1],details:'把节日过成值得收藏的一天。与亲友一起感受传统习俗，也给自己的下一次创造留一点期待。',rules:'活动期间每人每天可领取一次。\n节日签到与主签到可在同一天分别领取。'}));
const data={server_date:'2026-09-25',server_time:'2026-09-25T12:00:00+08:00',timezone:'Asia/Shanghai',theme:'mid_autumn',calendar_min_year:2025,calendar_max_year:2050,campaigns:festivals,checkin:{enabled:true,min_quota:1000,max_quota:2000,turnstile_required:false},history:[{campaign_key:'mid_autumn',claim_date:'2026-09-25',reward_quota:1000}]};
const harness = `<script>
window.ProxyLan=Object.assign(window.ProxyLan||{}, {
escapeHTML:v=>String(v??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c])),
moneyFromQuota:(q,d=2)=>'$'+(Number(q||0)/500000).toFixed(d),quotaFromUSD:v=>Math.round(Number(v)*500000),usdFromQuota:v=>String(Number(v)/500000),number:v=>new Intl.NumberFormat('zh-CN').format(v),date:v=>String(v),preferences:()=>({}),savePreferences:()=>{},initializeVisuals:()=>{},syncQuotaUSD:()=>{},toast:console.log,confirm:async()=>false,setText:(s,v)=>document.querySelectorAll(s).forEach(e=>e.textContent=v),showBusy:(b,on)=>{b.disabled=on;b.dataset.busy=String(on)},openDialog:id=>document.getElementById(id).classList.add('show'),request:async(url,o={})=>{
if(url==='/api/user/self')return {role:100,quota:5000};
if(url==='/api/subscription/plans'||url==='/api/subscription/admin/plans')return [{plan:{id:1,title:'月光创作计划',subtitle:'把团圆的灵感，留给每一次创造。',enabled:true,price_amount:9.9,currency:'USD',duration_unit:'day',duration_value:30,total_amount:5000000,quota_reset_period:'monthly',allow_balance_pay:true}}];
if(url==='/api/user/topup/info'||url==='/api/user/ldc/info')return {payment_compliance_confirmed:true,enabled:true,credits_per_unit:10};
if(url.includes('/activities'))return ${JSON.stringify(data)};
if(url==='/api/user/checkin')return {enabled:true,min_quota:1000,max_quota:2000,stats:{checked_in_today:false,total_checkins:28,total_quota:32000,records:[{checkin_date:'2026-09-24',quota_awarded:1000},{checkin_date:'2026-09-23',quota_awarded:1500},{checkin_date:'2026-09-22',quota_awarded:1300}]}};
if(url.includes('/subscription-benefits'))return [];
if(url==='/api/user/benefits')return {items:[{id:1,name:'会员灵感礼包',plan_name:'创造者年度套餐',description:'谢谢你与我们一起创造。',permanent_quota:15000,status:1,can_claim:true}]};
if(url.includes('/token-loan'))return {visible:false};
return [];
}});
</script><script src='/relayfor-assets/lunar-schedule.js'></script><script src='/relayfor-assets/festival-subscriptions.js'></script><script src='/relayfor-assets/activities.js'></script><script>if(!location.hash)history.replaceState(null,'','#activities');document.dispatchEvent(new CustomEvent('relayfor:ready'));window.ProxyLan.mountShell();window.ProxyLanActivities.load();</script>`;
const server=http.createServer(async(req,res)=>{
  try {
    const url=new URL(req.url,'http://127.0.0.1:8314');
    if(url.pathname.startsWith('/relayfor-assets/')){
      const asset=path.resolve(root,'relayfor-ui/assets',decodeURIComponent(url.pathname.slice('/relayfor-assets/'.length)));
      if(!asset.startsWith(path.join(root,'relayfor-ui/assets')+path.sep))throw Error('invalid path');
      res.setHeader('Content-Type',asset.endsWith('.css')?'text/css':asset.endsWith('.js')?'text/javascript':asset.endsWith('.woff2')?'font/woff2':'application/octet-stream');res.end(await fs.readFile(asset));return;
    }
    const page=url.pathname.startsWith('/admin')?'admin':'console';
    let html=await fs.readFile(path.join(root,'relayfor-ui',page,'index.html'),'utf8');
    html=html.replace(/<script\b[\s\S]*?<\/script>/gi,'');
    html=html.replace('</head>',`<link rel='stylesheet' href='/relayfor-assets/activities.css'><link rel='stylesheet' href='/relayfor-assets/workspace-colors.css'></head>`);
    html=html.replace('</body>',()=>`<script src='/relayfor-assets/material-icons.js'></script><script src='/relayfor-assets/app.js'></script>${harness}</body>`);
    res.setHeader('Content-Type','text/html;charset=utf-8');res.end(html);
  }catch(error){res.statusCode=404;res.end(error.message);}
});
server.listen(8317,'127.0.0.1',()=>console.log('Isolated activity visual fixtures: http://127.0.0.1:8317/console/ and /admin/'));
