// Unified local preview server for the RelayFor rebuild.
// It serves the copied public site, user console, management shell and
// deterministic synthetic auth/prepaid endpoints. No request is proxied to
// relayfor.xyz and no real credentials are accepted or stored.
import http from 'node:http';
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import { fileURLToPath } from 'node:url';

const adminRoot = path.dirname(fileURLToPath(import.meta.url));
const projectRoot = path.resolve(adminRoot, '..');
const mirrorRoot = path.join(projectRoot, 'mirror');
// Runtime fixture adapters live outside the read-only mirror.  The copied
// mirror keeps the original browser shell and its older evidence fixtures;
// the local preview must serve the enhanced synthetic contracts from this
// directory instead of mutating mirror/.
const runtimeFixtureRoot = path.join(adminRoot, 'runtime-fixtures');
const port = Number(process.argv[2] || 23000);

const mime = {
  '.html': 'text/html; charset=utf-8', '.js': 'application/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8', '.json': 'application/json; charset=utf-8',
  '.txt': 'text/plain; charset=utf-8', '.tsv': 'text/tab-separated-values; charset=utf-8',
  '.svg': 'image/svg+xml', '.png': 'image/png', '.jpg': 'image/jpeg', '.jpeg': 'image/jpeg',
  '.gif': 'image/gif', '.ico': 'image/x-icon', '.woff': 'font/woff', '.woff2': 'font/woff2'
};

const sessions = new Map();
const prepaidTickets = new Map();

function json(response, status, value, headers = {}) {
  response.writeHead(status, { 'Content-Type': 'application/json; charset=utf-8', 'Cache-Control': 'no-store', ...headers });
  response.end(JSON.stringify(value));
}
function text(response, status, value, contentType = 'text/plain; charset=utf-8') {
  response.writeHead(status, { 'Content-Type': contentType, 'Cache-Control': 'no-store' });
  response.end(value);
}
function bodyOf(request) {
  return new Promise((resolve, reject) => {
    let raw = '';
    request.setEncoding('utf8');
    request.on('data', chunk => { raw += chunk; if (raw.length > 1024 * 1024) reject(new Error('request body too large')); });
    request.on('end', () => { if (!raw) return resolve({}); try { resolve(JSON.parse(raw)); } catch (_) { resolve({}); } });
    request.on('error', reject);
  });
}
function tokenUser(request) {
  const token = String(request.headers.authorization || '').replace(/^Bearer\s+/i, '').trim();
  if (!token) return null;
  if (sessions.has(token)) return sessions.get(token);
  if (token === 'fixture-admin-session') return { id: 9001, username: 'fixture_admin', role: 100, group: 'default', status: 1, created_at: '2025-01-01T00:00:00.000Z' };
  if (token === 'fixture-user-session') return { id: 10001, username: 'fixture_user', role: 1, group: 'default', status: 1, created_at: '2024-01-01T00:00:00.000Z' };
  return null;
}
function makeSession(username, role = 1) {
  const safeName = String(username || (role >= 25 ? 'fixture_admin' : 'fixture_user')).trim().slice(0, 64);
  const user = { id: role >= 25 ? 9001 : 10001, username: safeName, role, group: 'default', status: 1, created_at: role >= 25 ? '2025-01-01T00:00:00.000Z' : '2024-01-01T00:00:00.000Z' };
  const token = 'fixture-' + (role >= 25 ? 'admin' : 'user') + '-' + crypto.randomBytes(12).toString('hex');
  sessions.set(token, user);
  return { token, user };
}
function isAdminName(username) { return /^(admin|administrator|fixture_admin|demo_operator)$/i.test(String(username || '').trim()); }
function authConfig() {
  return { is_pro_host: false, linuxdo_login_enabled: false, linuxdo_registration_enabled: false, local_registration_enabled: true, registration_code_configured: false, registration_code_required: false, required_user_group: '', site_name: 'RelayFor 公益站', system_name: 'RelayFor' };
}
function announcement() { return { site: 'default', title: '公益站公告', content: '这是本地复刻预览。所有账户、额度与 Key 均为合成数据，不连接真实服务。', user_docs_visible: true }; }
function authUser(user) { return { id: user.id, username: user.username, role: user.role, group: user.group, status: user.status, created_at: user.created_at }; }
function prepaidResult(registered = false) {
  const expires = new Date(Date.now() + 30 * 86400000).toISOString();
  return { registered_user: registered, merged: false, user_group: 'default', added_quota: 2500000, expires_at: expires, api_key: registered ? null : 'sk-fixture-prepaid-demo-0001', total_remaining: 2500000, key_hint: 'sk-fi…0001', key_status: 1, buckets: [{ name: '本地演示预付卡', quota_remaining: 2500000, quota_total: 2500000, expires_at: expires }] };
}
function resolveStatic(pathname) {
  const clean = decodeURIComponent(pathname.split('?')[0]);
  if (clean === '/' || clean === '') return path.join(mirrorRoot, 'index.html');
  if (clean === '/admin' || clean === '/admin/') return path.join(adminRoot, 'admin', 'index.html');
  if (clean === '/console' || clean === '/console/') return path.join(mirrorRoot, 'console', 'index.html');
  if (clean === '/prepaid' || clean === '/prepaid/') return path.join(mirrorRoot, 'prepaid', 'index.html');
  if (clean === '/auth' || clean === '/auth/') return path.join(mirrorRoot, 'auth@@mode=login', 'index.html');
  if (clean.startsWith('/fixtures/')) return path.join(runtimeFixtureRoot, clean.slice('/fixtures/'.length));
  if (clean === '/runtime-auth-config.js') return path.join(adminRoot, 'runtime-auth-config.js');
  if (clean.startsWith('/admin-assets/')) return path.join(adminRoot, clean.slice('/admin-assets/'.length));
  if (clean.startsWith('/mirror-assets/')) return path.join(mirrorRoot, clean.slice('/mirror-assets/'.length));
  if (clean.startsWith('/')) return path.join(mirrorRoot, clean.slice(1));
  return null;
}
function safeStatic(pathname) {
  const target = resolveStatic(pathname);
  if (!target) return null;
  const resolved = path.resolve(target);
  const allowed = [path.resolve(mirrorRoot), path.resolve(adminRoot), path.resolve(runtimeFixtureRoot)];
  if (!allowed.some(root => resolved === root || resolved.startsWith(root + path.sep))) return null;
  return resolved;
}
async function handleApi(request, response, pathname) {
  const method = String(request.method || 'GET').toUpperCase();
  if (method === 'GET' && pathname === '/api/auth/config') return json(response, 200, authConfig());
  if (method === 'GET' && pathname === '/api/site/announcement') return json(response, 200, announcement());
  if (method === 'POST' && (pathname === '/api/auth/login' || pathname === '/api/auth/register')) {
    const body = await bodyOf(request); const username = String(body.username || '').trim(); const password = String(body.password || '');
    if (!username || !password) return json(response, 400, { error: '请输入用户名和密码' });
    const result = makeSession(username, pathname.endsWith('/register') ? 1 : (isAdminName(username) ? 100 : 1));
    return json(response, 200, { token: result.token, user: authUser(result.user) });
  }
  if (method === 'POST' && pathname === '/api/auth/linuxdo/start') return json(response, 200, { authorization_url: '/auth/?mode=login&ticket=fixture-linuxdo-ticket' });
  if (method === 'POST' && pathname === '/api/auth/linuxdo/complete') { const result = makeSession('fixture_linuxdo', 1); return json(response, 200, { token: result.token, user: authUser(result.user) }); }
  if (method === 'GET' && pathname === '/api/user/self') { const user = tokenUser(request); return user ? json(response, 200, authUser(user)) : json(response, 401, { error: '未登录' }); }
  if (pathname.startsWith('/api/prepaid/linuxdo/')) {
    if (method === 'POST' && pathname.endsWith('/start')) {
      const body = await bodyOf(request); const code = String(body.code || '').trim();
      if (!code) return json(response, 400, { error: '请输入预付卡兑换码' });
      const ticket = 'fixture-prepaid-' + crypto.randomBytes(5).toString('hex'); prepaidTickets.set(ticket, code);
      return json(response, 200, { authorization_url: '/prepaid/?ticket=' + encodeURIComponent(ticket) });
    }
    if (method === 'POST' && pathname.endsWith('/balance/start')) return json(response, 200, { authorization_url: '/prepaid/?balance_ticket=fixture-balance-ticket' });
    if (method === 'POST' && pathname.endsWith('/complete')) { await bodyOf(request); return json(response, 200, pathname.includes('/balance/') ? prepaidResult(true) : prepaidResult(Boolean(tokenUser(request)))); }
  }
  if (pathname.startsWith('/api/user/')) {
    const user = tokenUser(request); if (!user) return json(response, 401, { error: '未登录' });
    return json(response, 200, { user: authUser(user), items: [], total: 0, total_pages: 1 });
  }
  return json(response, 404, { error: 'Not found' });
}

const server = http.createServer(async (request, response) => {
  try {
    const url = new URL(request.url || '/', 'http://127.0.0.1'); const pathname = url.pathname;
    if (pathname === '/favicon.ico') return response.writeHead(204).end();
    if (pathname === '/console') { response.writeHead(302, { Location: '/console/' }).end(); return; }
    if (pathname === '/prepaid') { response.writeHead(302, { Location: '/prepaid/' }).end(); return; }
    if (pathname === '/auth') { response.writeHead(302, { Location: '/auth/?mode=login' }).end(); return; }
    if (pathname.startsWith('/api/')) return await handleApi(request, response, pathname);
    let target = safeStatic(pathname);
    if (pathname === '/auth/') target = url.searchParams.get('mode') === 'register' ? path.join(mirrorRoot, 'auth@@mode=register', 'index.html') : path.join(mirrorRoot, 'auth@@mode=login', 'index.html');
    if (!target || !fs.existsSync(target) || fs.statSync(target).isDirectory()) return text(response, 404, 'Not found');
    const isAuthShell = target.startsWith(path.join(mirrorRoot, 'auth@@mode=')) && path.extname(target).toLowerCase() === '.html';
    if (isAuthShell) {
      let html = fs.readFileSync(target, 'utf8');
      if (!html.includes('/runtime-auth-config.js')) html = html.replace('</body>', '  <script defer src="/runtime-auth-config.js"></script>\n</body>');
      return text(response, 200, html, 'text/html; charset=utf-8');
    }
    response.writeHead(200, { 'Content-Type': mime[path.extname(target).toLowerCase()] || 'application/octet-stream', 'Cache-Control': 'no-store' }); fs.createReadStream(target).pipe(response);
  } catch (error) { if (!response.headersSent) json(response, 500, { error: error.message || 'Server error' }); else response.destroy(); }
});
server.listen(port, '127.0.0.1', () => { console.log(`RelayFor local rebuild: http://127.0.0.1:${port}/`); console.log('Synthetic accounts: any non-empty username/password; admin names: admin, fixture_admin, demo_operator'); });
