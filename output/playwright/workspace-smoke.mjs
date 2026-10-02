import { createRequire } from 'node:module'
import { mkdir, readFile, writeFile } from 'node:fs/promises'
import { randomBytes } from 'node:crypto'
import { resolve } from 'node:path'
const require = createRequire(import.meta.url)
const { chromium, request } = require('C:/Users/Administrator/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules/playwright')
const base = process.argv[2] || 'http://127.0.0.1:3002'
if (!/^http:\/\/(127\.0\.0\.1|localhost):\d+$/.test(base)) throw new Error('QA must use loopback')
const output = resolve(import.meta.dirname)
const credentialFile = resolve(output, '../../.agent-work/local-preview/qa-20261001/qa-credentials.json')
await mkdir(output, { recursive:true })
let credentials
try { credentials = JSON.parse(await readFile(credentialFile, 'utf8')) } catch { credentials = { username:'reviewadmin', password:'Qa!' + randomBytes(16).toString('hex'), user:'reviewuser', userPassword:'Qa!' + randomBytes(16).toString('hex') }; await writeFile(credentialFile, JSON.stringify(credentials)) }
const client = await request.newContext({ baseURL:base })
const checks = [], failures = [], consoleErrors = [], failedHTTP = []
async function check(name, work) { try { await work(); checks.push({name,passed:true}); console.log('PASS ' + name) } catch (error) { const message = error.message.split('\n')[0]; checks.push({name,passed:false,message}); failures.push(name); console.log('FAIL ' + name + ': ' + message) } }
async function json(method, path, body, token) {
  const response = await client.fetch(path, { method, data:body, headers:token ? { Authorization:'Bearer ' + token } : {} })
  const value = await response.json()
  if (!response.ok() || value.success === false) throw new Error(path + ': ' + (value.message || response.status()))
  return value
}
const setup = await json('GET','/api/setup')
if (!setup.data.status) await json('POST','/api/setup',{username:credentials.username,password:credentials.password,confirmPassword:credentials.password,SelfUseModeEnabled:false,DemoSiteEnabled:false})
const adminAuth = await json('POST','/api/user/login',{username:credentials.username,password:credentials.password})
const adminToken = adminAuth.data.access_token
const browser = await chromium.launch({ channel:'chrome', headless:true })
const context = await browser.newContext({ viewport:{width:1440,height:1000}, permissions:['clipboard-read','clipboard-write'] })
let page = await context.newPage()
function safeURL(value) { const url = new URL(value); for(const name of ['password','password_encrypted','access_token','key','token','registration_code','code']) { if(url.searchParams.has(name)) url.searchParams.set(name,'REDACTED') } return url.href.replace(base,'') }
function watch(target) { target.on('pageerror', error => consoleErrors.push(error.message)); target.on('response', response => { if (response.status() >= 400 && !/\/api\/auth\/refresh/.test(response.url())) failedHTTP.push({url:safeURL(response.url()),status:response.status()}) }) }
watch(page)
async function ready() { await page.waitForFunction(() => Boolean(window.RelayForGatewayReady),undefined,{timeout:30000}); await page.evaluate(() => window.RelayForGatewayReady); await page.waitForTimeout(700) }
async function screenshot(name) { await page.screenshot({path:resolve(output,name + '.png'),fullPage:false}) }
async function noOverflow() { const overflow = await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 2); if(overflow) throw new Error('horizontal overflow') }
await check('public homepage and protocol controls', async () => {
  await page.goto(base); await ready(); await page.getByRole('heading',{name:/让每一个想法/}).waitFor();
  await page.locator('[data-home-protocol="chat"]').click(); await page.waitForFunction(() => document.querySelector('[data-home-endpoint]').textContent === '/v1/chat/completions');
  await page.evaluate(() => scrollTo(0,0)); await noOverflow(); await screenshot('home-desktop')
})
await check('registration and prepaid page layouts', async () => {
  await page.goto(base+'/auth/?mode=register'); await ready(); await noOverflow(); await screenshot('register-desktop');
  await page.goto(base+'/prepaid/'); await ready(); await page.locator('[data-prepaid-form]').waitFor(); await noOverflow(); await screenshot('prepaid-desktop')
})
await check('auth and administrator role redirect', async () => {
  await page.goto(base + '/auth/?mode=login'); await ready();
  await page.locator('#linuxdo-login').waitFor(); if(await page.locator('#linuxdo-login').isEnabled()) throw new Error('unconfigured OAuth must remain disabled');
  await page.locator('#auth-username').fill(credentials.username); await page.locator('#auth-password').fill(credentials.password);
  await page.locator('#auth-password-toggle').click(); if(await page.locator('#auth-password').getAttribute('type') !== 'text') throw new Error('password visibility'); await page.locator('#auth-password-toggle').click();
  await screenshot('auth-desktop'); await page.locator('#auth-submit').click(); await page.waitForURL('**/admin/**',{timeout:30000}); await ready(); await screenshot('admin-desktop')
})
await check('all native administrator pages', async () => {
  const names = ['overview','channels','models','subscriptions','users','redemptions','registration-codes','prepaid-cards','logs','settings','risk'];
  for (const name of names) { await page.locator('.nav-rail [data-nav="' + name + '"]').first().click(); await page.locator('[data-panel="' + name + '"].active').waitFor(); await page.waitForTimeout(300); await noOverflow() }
})
await check('administrator embedded feature pages and system settings', async () => {
  for (const name of ['overview','models','users','subscriptions','logs']) {
    await page.locator('.nav-rail [data-nav="'+name+'"]').first().click();
    const panel = page.locator('[data-panel="'+name+'"]'); const tabs = panel.locator('[data-relayfor-inline-tabs] button');
    await tabs.nth(1).click(); const frame = panel.locator('iframe[data-relayfor-extension]'); await frame.waitFor();
    await frame.contentFrame().locator('#root').waitFor(); await page.waitForTimeout(450); await tabs.nth(0).click()
  }
  for (const slug of ['system-info','request-policies','passkey','ssrf','gemini','payment','chat','alerts']) {
    await page.locator('.nav-rail [data-nav="settings"]').first().click(); await page.locator('[data-settings-page="'+slug+'"]').click();
    const host = page.locator('[data-settings-subpage="'+slug+'"]'); await host.locator('iframe').waitFor(); await host.locator('iframe').contentFrame().locator('#root').waitFor(); await page.waitForTimeout(300)
  }
})
await check('health and routing real endpoints', async () => {
  await page.locator('.nav-rail [data-nav="models"]').click();
  for (const tab of ['health','routing','catalog']) { await page.locator('[data-admin-model-tab="' + tab + '"]').click(); await page.waitForTimeout(450) }
  await json('GET','/api/admin/model-health',undefined,adminToken)
})
await check('model creation, pricing persistence and alias routing', async () => {
  const channels = await json('GET','/api/channel/?p=1&page_size=100',undefined,adminToken);
  let channel = (channels.data.items || []).find(item => item.name === '验收渠道');
  if (!channel) { await json('POST','/api/channel/',{mode:'single',channel:{type:1,name:'验收渠道',key:'qa-placeholder-no-provider-access',base_url:'https://api.example.invalid',models:'qa-model',group:'default',status:1,priority:0,weight:1}},adminToken); const saved = await json('GET','/api/channel/?p=1&page_size=100',undefined,adminToken); channel = saved.data.items.find(item => item.name === '验收渠道') }
  await page.evaluate(async () => { await window.ProxyLan.request('/api/admin/models',{method:'POST',body:{model:'qa-model',display_name:'qa-model',billing_mode:'token',input_price:1,output_price:2,cache_read_price:-1,cache_write_price:-1}}) });
  await page.evaluate(async channelID => { await window.ProxyLan.request('/api/admin/routing/channels/'+channelID+'/aliases',{method:'PUT',body:{alias:'qa-public',target_model:'qa-model'}}) },channel.id);
  const savedChannel = await json('GET','/api/channel/'+channel.id,undefined,adminToken); if(JSON.parse(savedChannel.data.model_mapping)['qa-public'] !== 'qa-model') throw new Error('alias did not persist');
  await page.reload(); await ready(); await page.locator('.nav-rail [data-nav="models"]').click(); await page.waitForTimeout(500); await screenshot('models-desktop');
  await page.locator('[data-admin-model-tab="routing"]').click(); await page.waitForTimeout(400); await screenshot('routing-desktop');
  await page.locator('[data-admin-model-tab="health"]').click(); await page.waitForTimeout(400); await screenshot('health-desktop')
})
await check('site name settings, persistence and callback copy', async () => {
  await page.locator('.nav-rail [data-open-settings="site"]').click(); await page.locator('[data-site-settings]').waitFor({state:'visible'});
  const form = page.locator('[data-site-settings]'); await form.locator('[name="system_name"]').fill('RelayFor QA'); await form.locator('[type="submit"]').click();
  await page.waitForFunction(() => document.querySelector('.rail-title').textContent === 'RelayFor QA');
  const status = await json('GET','/api/status'); if(status.data.system_name !== 'RelayFor QA') throw new Error('name did not persist');
  await page.locator('[data-linuxdo-copy-callback]').click(); await screenshot('site-settings-desktop');
  await form.locator('[name="system_name"]').fill('RelayFor'); await form.locator('[type="submit"]').click(); await page.waitForFunction(() => document.querySelector('.rail-title').textContent === 'RelayFor')
})
await check('single redemption batch with expandable details and copying', async () => {
  await json('POST','/api/redemption/',{name:'验收批次',quota:500000,count:3,expired_time:0},adminToken);
  await page.locator('.nav-rail [data-nav="redemptions"]').click(); await page.waitForTimeout(600);
  const batch = page.locator('.redemption-batch').first(); await batch.waitFor(); await batch.locator('[data-code-batch-toggle]').click();
  await screenshot('redemptions-desktop');
  const count = await batch.locator('.redemption-table tbody tr').count(); if(count !== 3) throw new Error('expected 3 generated codes, got ' + count)
})
await check('multi-user token statistics real API and empty-state controls', async () => {
  await page.locator('.nav-rail [data-nav="users"]').click(); await page.locator('[data-token-usage-card]').waitFor();
  await page.waitForTimeout(600); await json('GET','/api/admin/token-usage?days=7&page=1&page_size=25',undefined,adminToken); await screenshot('users-desktop')
})
await check('page search, keyboard shortcut and reduced motion', async () => {
  await page.keyboard.press('Control+k'); await page.locator('#workspace-jump.show').waitFor(); await page.locator('.workspace-jump-input').fill('渠道'); await page.locator('.workspace-jump-results button').first().click(); await page.locator('[data-panel="channels"].active').waitFor();
  await page.locator('[data-gateway-motion]').click(); const motion = await page.evaluate(() => document.documentElement.dataset.relayforMotion); if(motion !== 'off') throw new Error('motion toggle failed'); await page.locator('[data-gateway-motion]').click()
})
await check('administrator mobile navigation', async () => { await page.setViewportSize({width:390,height:844}); await noOverflow(); await screenshot('admin-mobile'); await page.setViewportSize({width:1440,height:1000}) })
await check('ordinary administrator model permissions', async () => {
  const operatorPassword = credentials.password + 'Op';
  const users = await json('GET','/api/user/?p=1&page_size=100',undefined,adminToken);
  if(!(users.data.items || []).some(item => item.username === 'reviewop')) await json('POST','/api/user/',{username:'reviewop',password:operatorPassword,display_name:'验收管理员',role:10,group:'default'},adminToken);
  const rootPage = page;
  const operatorContext = await browser.newContext({viewport:{width:1440,height:1000}}); page = await operatorContext.newPage(); watch(page);
  try { await page.goto(base + '/auth/?mode=login'); await ready(); await page.locator('#auth-username').fill('reviewop'); await page.locator('#auth-password').fill(operatorPassword); await page.locator('#auth-submit').click(); await page.waitForURL('**/admin/**',{timeout:30000}); await ready(); await page.locator('.nav-rail [data-nav="models"]').click(); await page.waitForTimeout(600); const models = await page.evaluate(async () => window.ProxyLan.request('/api/admin/models')); if(!Array.isArray(models)) throw new Error('read-only catalog unavailable'); if(models.some(item => item.pricing_read_only !== true)) throw new Error('ordinary administrator pricing must be read-only') } finally { await operatorContext.close(); page = rootPage }
})
await check('normal user creation and login', async () => {
  await context.storageState({path:resolve(output,'../../.agent-work/local-preview/qa-20261001/admin-storage.json')});
  const users = await json('GET','/api/user/?p=1&page_size=100',undefined,adminToken); const items = users.data.items || [];
  if(!items.some(item => item.username === credentials.user)) await json('POST','/api/user/',{username:credentials.user,password:credentials.userPassword,display_name:'验收用户',role:1,group:'default',quota:0},adminToken);
  const userContext = await browser.newContext({viewport:{width:1440,height:1000},permissions:['clipboard-read','clipboard-write']}); page = await userContext.newPage(); watch(page); await page.goto(base + '/auth/?mode=login'); await ready(); await page.locator('#auth-username').fill(credentials.user); await page.locator('#auth-password').fill(credentials.userPassword); await page.locator('#auth-submit').click(); await page.waitForURL('**/console/**',{timeout:30000}); await ready(); await screenshot('console-desktop')
})
await check('all native user pages and health privacy', async () => {
  for(const name of ['overview','tokens','models','docs','usage','redeem','subscriptions','profile']) { await page.locator('.nav-rail [data-nav="'+name+'"]').first().click(); await page.locator('[data-panel="'+name+'"].active').waitFor(); await page.waitForTimeout(300); await noOverflow() }
  await page.locator('.nav-rail [data-nav="models"]').click(); await page.locator('[data-console-model-tab="health"]').click(); await page.waitForTimeout(500);
  const response = await page.evaluate(async () => window.ProxyLan.request('/api/user/model-health')); if(JSON.stringify(response).includes('channel_name')) throw new Error('private channel data leaked')
})
await check('user key dialog and mobile layout', async () => {
  await page.locator('.nav-rail [data-nav="tokens"]').click(); await page.locator('[data-panel="tokens"] [data-open-dialog="token-dialog"]').first().click(); await page.locator('#token-dialog.show').waitFor(); await screenshot('key-dialog-desktop'); await page.keyboard.press('Escape');
  await page.setViewportSize({width:390,height:844}); await noOverflow(); await screenshot('console-mobile'); await page.goto(base + '/auth/?mode=login'); await ready(); await noOverflow(); await screenshot('auth-mobile'); await page.goto(base); await ready(); await noOverflow(); await screenshot('home-mobile')
})
await check('browser console and HTTP error audit', async () => { if(consoleErrors.length) throw new Error(consoleErrors.join('; ')); if(failedHTTP.length) throw new Error(JSON.stringify(failedHTTP)) })
await writeFile(resolve(output,'workspace-smoke-report.json'), JSON.stringify({base,checkedAt:new Date().toISOString(),checks,consoleErrors,failedHTTP},null,2))
await browser.close(); await client.dispose();
console.log(`${checks.length - failures.length}/${checks.length} browser checks passed`)
if(failures.length) process.exitCode = 1
