import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { Window } from '../web/node_modules/happy-dom/lib/index.js';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const read = name => fs.readFile(path.join(root, 'relayfor-ui/assets', name), 'utf8');
const [icons, app, material, gateway, colors, appearance] = await Promise.all([
  read('material-icons.js'), read('app.js'), read('material.css'), read('gateway-theme.css'), read('workspace-colors.css'), read('appearance.js'),
]);
const window = new Window({url:'http://localhost/console/', settings:{disableCSSFileLoading:true, disableJavaScriptFileLoading:true}});
const document = window.document;
const tick = () => new Promise(resolve => setTimeout(resolve, 15));
document.body.innerHTML = '<header class="top-app-bar"></header><h1>活动与签到</h1><button id="save" class="md-btn"><span class="material-symbol">save</span>保存设置</button><span id="password" class="material-symbol">visibility</span><span id="custom" class="material-symbol"><svg viewBox="0 0 24 24"><title>moon</title><path d="M1 1L2 2"/></svg></span><span id="label" class="material-symbol"><b>设置说明</b></span><span id="prose" class="material-symbol">save your settings</span><span id="unknown" class="material-symbol">future_icon</span><span id="prototype" class="material-symbol">constructor</span><span id="sun" class="material-symbol">wb_sunny</span><div class="usage-cost-section-head"><span id="tile" class="material-symbol">save</span></div>';
window.eval(app);
window.eval(icons);
await tick();
assert.equal(document.querySelector('h1').textContent, '活动与签到');
assert.equal(document.querySelector('#save').textContent, '保存设置');
assert.equal(document.querySelector('#password').dataset.materialIcon, 'visibility');
assert.equal(document.querySelector('#custom title').textContent, 'moon');
assert.equal(document.querySelector('#label b').textContent, '设置说明');
assert.equal(document.querySelector('#prose').textContent, 'save your settings');
assert(document.querySelector('#unknown svg'));
assert(document.querySelector('#unknown').hasAttribute('data-material-icon-fallback'));
assert(document.querySelector('#prototype svg'));
assert(!document.querySelector('#sun').hasAttribute('data-material-icon-fallback'));

// Refreshing a rendered icon must neither replace its node nor consume SVG titles.
const svg = document.querySelector('#save svg');
window.RelayForIcons.refresh();
window.RelayForIcons.refresh(document);
assert.equal(document.querySelector('#save svg'), svg);
const title = document.createElementNS('http://www.w3.org/2000/svg', 'title');
title.textContent = 'Save';
svg.prepend(title);
await tick();
assert.equal(document.querySelector('#save svg'), svg);
assert.equal(document.querySelector('#save svg title').textContent, 'Save');
title.remove();

// Password visibility controls replace ligature text after initial SVG rendering.
document.querySelector('#password').textContent = 'visibility_off';
await tick();
assert.equal(document.querySelector('#password').dataset.materialIcon, 'visibility_off');
assert.equal(document.querySelectorAll('#password svg').length, 1);

// Exercise the production busy helper with its cached SVG markup.
const button = document.querySelector('#save');
window.ProxyLan.showBusy(button, true);
await tick();
assert.equal(button.textContent, '处理中…');
window.ProxyLan.showBusy(button, false);
await tick();
assert.equal(button.textContent, '保存设置');
assert.equal(button.querySelectorAll('svg').length, 1);
assert.equal(button.querySelector('.material-symbol').dataset.materialIcon, 'save');
assert(!button.disabled);

// New subtrees and detached fragments both receive icons without touching labels.
const panel = document.createElement('div');
panel.innerHTML = '<button><span class="material-symbol">calendar_month</span>查看日历</button>';
document.body.append(panel);
await tick();
assert.equal(panel.textContent, '查看日历');
assert(panel.querySelector('svg'));
const fragment = document.createDocumentFragment();
const leaf = document.createElement('span');
leaf.className = 'material-symbol'; leaf.textContent = 'favorite'; fragment.append(leaf);
window.RelayForIcons.refresh(fragment);
assert(leaf.querySelector('svg'));

// Every literal icon currently used by HTML/JS templates must have a local SVG.
const names = new Set();
async function collect(directory) {
  for (const entry of await fs.readdir(directory, {withFileTypes:true})) {
    const filename = path.join(directory, entry.name);
    if (entry.isDirectory()) await collect(filename);
    else if (/\.(html|js)$/.test(entry.name) && entry.name !== 'material-icons.js') {
      const source = await fs.readFile(filename, 'utf8');
      for (const match of source.matchAll(/class=["'][^"']*\bmaterial-symbol\b[^"']*["'][^>]*>\s*([a-z][a-z0-9_]*)\s*</g)) names.add(match[1]);
      for (const match of source.matchAll(/\bicon\(['"]([a-z][a-z0-9_]*)['"]\)/g)) names.add(match[1]);
    }
  }
}
await collect(path.join(root, 'relayfor-ui'));
for (const name of names) {
  const icon = document.createElement('span'); icon.className = 'material-symbol'; icon.textContent = name;
  window.RelayForIcons.refresh(icon);
  assert(icon.querySelector('svg'), `${name} did not render`);
  assert(!icon.hasAttribute('data-material-icon-fallback'), `${name} has no local icon`);
}

const style = document.createElement('style');
style.textContent = await read('app.css') + gateway + material + colors;
document.head.append(style);
assert.equal(window.getComputedStyle(document.querySelector('#tile')).width, '38px', 'decorated icon container must keep its layout width');
assert.equal(window.getComputedStyle(document.querySelector('#tile')).height, '38px');
const html = document.documentElement;
html.dataset.theme = 'light'; html.dataset.relayforPalette = 'blue';
const token = name => window.getComputedStyle(html).getPropertyValue(name).trim();
assert.equal(token('--workspace-action'), '#cce6fb');
assert.equal(token('--workspace-yellow'), '#fff5d6');
for (const theme of ['light', 'dark']) {
  html.dataset.theme = theme;
  for (const festival of ['duanwu', 'qixi', 'mid_autumn', 'chongyang', 'new_year_eve']) {
    html.dataset.holiday = festival;
    html.dataset.relayforPalette = 'blue';
    const action = token('--workspace-action');
    const primary = token('--md-sys-color-primary');
    for (const palette of ['yellow', 'white']) {
      html.dataset.relayforPalette = palette;
      assert.equal(token('--workspace-action'), action, `${festival}/${theme} action overridden by ${palette}`);
      assert.equal(token('--md-sys-color-primary'), primary, `${festival}/${theme} primary overridden by ${palette}`);
    }
  }
}
delete html.dataset.holiday;
html.dataset.theme = 'light'; html.dataset.relayforPalette = 'yellow';
assert.equal(token('--workspace-action'), '#f8e8b8', 'normal personal palette resumes after the festival');
window.eval(appearance);
document.dispatchEvent(new window.CustomEvent('relayfor:ready'));
assert.equal(document.querySelector('[data-palette="blue"]').textContent, '浅蓝');
assert.equal(document.querySelector('[data-palette="yellow"]').textContent, '浅黄');
document.querySelector('[data-palette="blue"]').click();
assert.equal(html.dataset.relayforPalette, 'blue');
assert.equal(window.localStorage.getItem('newapi:theme:v1:relayfor-palette'), 'blue');
assert.equal(document.querySelector('[data-palette="blue"]').getAttribute('aria-pressed'), 'true');
window.happyDOM.abort();
await window.happyDOM.close();
console.log(`PASS: ${names.size} template icons; dynamic refresh, SVG preservation, busy restore, decorated sizing, 10 holiday/theme combinations, palette persistence.`);
