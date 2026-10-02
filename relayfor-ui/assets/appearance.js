(function () {
  'use strict';
  function publishPreferences() {
    const message = { type: 'relayfor:preferences', palette: document.documentElement.dataset.relayforPalette || 'white', motion: document.documentElement.dataset.relayforMotion || '', theme: document.documentElement.dataset.theme || 'light' };
    document.querySelectorAll('iframe[data-relayfor-extension]').forEach(function (frame) { if (frame.contentWindow) frame.contentWindow.postMessage(message, location.origin); });
  }
  document.addEventListener('relayfor:ready', function () {
    const api = window.ProxyLan;
    const toolbar = document.querySelector('.top-app-bar') || document.querySelector('.public-header nav') || document.querySelector('.auth-header');
    if (!toolbar) return;
    const controls = document.createElement('div');
    controls.className = 'gateway-appearance-controls';
    controls.innerHTML = '<button type="button" class="md-icon-button" data-gateway-palette title="配色" aria-label="配色"><svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" aria-hidden="true"><path d="M12 3a9 9 0 1 0 0 18h1a2 2 0 0 0 1-3.7 1 1 0 0 1 .5-1.8H17a4 4 0 0 0 4-4A9 9 0 0 0 12 3Z"/><circle cx="7.5" cy="10" r="1"/><circle cx="11" cy="7" r="1"/><circle cx="15.5" cy="8" r="1"/></svg></button><button type="button" class="md-icon-button" data-gateway-motion title="动效" aria-label="动效"><svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" aria-hidden="true"><path d="M3 12c2-8 4-8 6 0s4 8 6 0 4-8 6 0"/></svg></button>';
    toolbar.append(controls);
    const dialog = document.createElement('div');
    dialog.className = 'dialog-backdrop';
    dialog.id = 'gateway-appearance-dialog';
    dialog.innerHTML = '<div class="md-dialog" role="dialog" aria-modal="true" aria-labelledby="gateway-appearance-title"><h2 class="md-dialog-title" id="gateway-appearance-title">自定义配色</h2><div class="gateway-palette-options"><button type="button" class="md-btn md-btn-outlined" data-palette="white">云白</button><button type="button" class="md-btn md-btn-outlined" data-palette="blue">浅蓝</button><button type="button" class="md-btn md-btn-outlined" data-palette="yellow">浅黄</button></div><div class="md-dialog-actions"><button type="button" class="md-btn md-btn-text" data-palette-close>完成</button></div></div>';
    document.body.append(dialog);
    controls.querySelector('[data-gateway-palette]').addEventListener('click', function () { api.openDialog(dialog.id); });
    dialog.querySelector('[data-palette-close]').addEventListener('click', function () { api.closeDialog(dialog.id); });
    dialog.addEventListener('click', function (event) { if (event.target === dialog) api.closeDialog(dialog.id); });
    dialog.querySelectorAll('[data-palette]').forEach(function (button) {
      button.addEventListener('click', function () {
        const palette = button.dataset.palette;
        document.documentElement.dataset.relayforPalette = palette;
        try { localStorage.setItem('newapi:theme:v1:relayfor-palette', palette); } catch (_) {}
        dialog.querySelectorAll('[data-palette]').forEach(function (item) { item.setAttribute('aria-pressed', String(item === button)); });
        publishPreferences();
      });
      button.setAttribute('aria-pressed', String(button.dataset.palette === (document.documentElement.dataset.relayforPalette || 'white')));
    });
    const motionButton = controls.querySelector('[data-gateway-motion]');
    function syncMotionButton() {
      const explicit = document.documentElement.dataset.relayforMotion;
      const enabled = explicit ? explicit === 'on' : !window.matchMedia('(prefers-reduced-motion: reduce)').matches;
      motionButton.title = enabled ? '关闭动效' : '开启动效';
      motionButton.setAttribute('aria-label', motionButton.title);
      motionButton.setAttribute('aria-pressed', String(enabled));
      return enabled;
    }
    motionButton.addEventListener('click', function () {
      const enabled = !syncMotionButton();
      document.documentElement.dataset.relayforMotion = enabled ? 'on' : 'off';
      try { localStorage.setItem('relayfor-motion', enabled ? 'on' : 'off'); } catch (_) {}
      api.savePreferences(Object.assign({}, api.preferences(), { reducedMotion: !enabled }));
      syncMotionButton();
      publishPreferences();
      api.toast(enabled ? '已开启原版动效' : '已关闭动效');
    });
    syncMotionButton();
    window.addEventListener('proxylan:preferences', publishPreferences);
    window.addEventListener('message', function (event) { if (event.origin === location.origin && event.data && event.data.type === 'relayfor:extension-ready') publishPreferences(); });
  });
}());
