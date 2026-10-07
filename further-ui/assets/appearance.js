(function () {
  // oxlint-disable-next-line unicorn/prefer-module -- Loaded as a classic workspace script.
  'use strict';
  function publishPreferences() {
    document.documentElement.dataset.furtherMotion = 'on';
    document.documentElement.dataset.reducedMotion = 'false';
    const state = window.FurtherAppearance && window.FurtherAppearance.state();
    const message = { type: 'further:preferences', palette: state ? state.palette : document.documentElement.dataset.furtherPalette || 'blue', paletteMode: state ? state.mode : 'fixed', motion: 'on', theme: document.documentElement.dataset.theme || 'light' };
    document.querySelectorAll('iframe[data-further-extension]').forEach(function (frame) { if (frame.contentWindow) frame.contentWindow.postMessage(message, location.origin); });
  }
  function mountAppearance() {
    const api = window.FurtherUI;
    const toolbar = document.querySelector('.top-app-bar') || document.querySelector('.further-public-actions') || document.querySelector('.public-header nav') || document.querySelector('.auth-header');
    if (!toolbar || !api || toolbar.querySelector('.gateway-appearance-controls')) return;
    const controls = document.createElement('div');
    controls.className = 'gateway-appearance-controls';
    controls.innerHTML = '<button type="button" class="md-icon-button" data-gateway-palette title="固定配色" aria-label="配色"><svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" aria-hidden="true"><path d="M12 3a9 9 0 1 0 0 18h1a2 2 0 0 0 1-3.7 1 1 0 0 1 .5-1.8H17a4 4 0 0 0 4-4A9 9 0 0 0 12 3Z"/><circle cx="7.5" cy="10" r="1"/><circle cx="11" cy="7" r="1"/><circle cx="15.5" cy="8" r="1"/></svg></button>';
    toolbar.append(controls);
    const dialog = document.createElement('div');
    dialog.className = 'dialog-backdrop';
    dialog.id = 'gateway-appearance-dialog';
    const options = [['white', '云白', '干净、轻盈的日常配色'], ['blue', '晴空蓝', '柔和的蓝色工作空间'], ['yellow', '暖阳黄', '温暖明亮的日常配色'], ['duanwu', '端午 · 粽叶青', '竹青与浅绿'], ['qixi', '七夕 · 星河紫', '暮蓝与淡紫'], ['mid_autumn', '中秋 · 月光金', '月白与桂花金'], ['chongyang', '重阳 · 秋菊黄', '秋色与暖金'], ['new_year_eve', '除夕 · 灯火红', '朱红与暖杏'], ['national_day', '国庆 · 山河红', '赤陶与金色']];
    dialog.innerHTML = `<div class="md-dialog palette-dialog" role="dialog" aria-modal="true" aria-labelledby="gateway-appearance-title"><div class="palette-dialog-heading"><span class="material-symbol" aria-hidden="true">palette</span><div><h2 class="md-dialog-title" id="gateway-appearance-title">你的空间，你的颜色</h2><p>固定所选配色，跨页面保持一致。也可以主动开启自动节日配色。</p></div></div><div class="gateway-palette-mode" role="group" aria-label="配色模式"><button type="button" data-palette-mode="fixed" aria-pressed="false"><b>固定配色</b><small>保持当前颜色，不自动切换</small></button><button type="button" data-palette-mode="auto" aria-pressed="false"><b>自动节日</b><small>根据站点当前节日换色</small></button></div><div class="gateway-palette-options">${  options.map(function (item) { return `<button type="button" data-palette="${item[0]}" aria-pressed="false"><i aria-hidden="true"></i><span><b>${item[1]}</b><small>${item[2]}</small></span><span class="palette-check material-symbol" aria-hidden="true">check_circle</span></button>`; }).join('')  }</div><div class="md-dialog-actions"><span data-palette-status role="status"></span><button type="button" class="md-btn md-btn-filled" data-palette-close>完成</button></div></div>`;
    document.body.append(dialog);
    function sync() {
      const appearance = window.FurtherAppearance;
      if (!appearance) return;
      const state = appearance.state();
      dialog.querySelectorAll('[data-palette-mode]').forEach(function (item) { item.setAttribute('aria-pressed', String(item.dataset.paletteMode === state.mode)); });
      dialog.querySelectorAll('[data-palette]').forEach(function (item) { item.setAttribute('aria-pressed', String(item.dataset.palette === state.effectivePalette)); });
      const title = options.find(function (item) { return item[0] === state.effectivePalette; });
      dialog.querySelector('[data-palette-status]').textContent = (state.mode === 'fixed' ? '已固定 · ' : '自动节日 · ') + (title ? title[1] : '晴空蓝');
      controls.querySelector('[data-gateway-palette]').title = state.mode === 'fixed' ? '配色已固定' : '自动节日配色';
    }
    controls.querySelector('[data-gateway-palette]').addEventListener('click', function () { sync(); api.openDialog(dialog.id); });
    dialog.querySelector('[data-palette-close]').addEventListener('click', function () { api.closeDialog(dialog.id); });
    dialog.addEventListener('click', function (event) { if (event.target === dialog) api.closeDialog(dialog.id); });
    dialog.querySelectorAll('[data-palette]').forEach(function (button) {
      button.addEventListener('click', function () { if (window.FurtherAppearance) window.FurtherAppearance.select(button.dataset.palette); sync(); publishPreferences(); });
    });
    dialog.querySelectorAll('[data-palette-mode]').forEach(function (button) {
      button.addEventListener('click', function () { if (window.FurtherAppearance) window.FurtherAppearance.setMode(button.dataset.paletteMode); sync(); publishPreferences(); });
    });
    window.addEventListener('further-ui:palette', function () { sync(); publishPreferences(); });
    window.addEventListener('further-ui:preferences', publishPreferences);
    window.addEventListener('message', function (event) { if (event.origin === location.origin && event.data && event.data.type === 'further:extension-ready') publishPreferences(); });
    sync();
  }
  document.addEventListener('further:ready', mountAppearance);
  if (document.body.dataset.gatewayReady === 'true') mountAppearance();
}());
