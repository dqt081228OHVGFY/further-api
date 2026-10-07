(function () {
  'use strict';
  const api = window.FurtherUI;
  let channels = [];
  let refreshChannels = null;
  let savedPicker = null;
  const draftPickers = new WeakMap();

  function ids(value) {
    return Array.from(new Set((Array.isArray(value) ? value : String(value || '').split(',')).map(function (id) { return String(id).trim(); }).filter(Boolean)));
  }
  function canImport() { return Number(document.body.dataset.gatewayRole || 0) >= 10; }
  function controls() {
    return '<div class="channel-model-toolbar"><button type="button" class="md-btn md-btn-outlined" data-upstream-fetch><span class="material-symbol" aria-hidden="true">cloud_download</span>拉取上游目录</button><span class="field-hint" data-upstream-status role="status" aria-live="polite">从渠道上游获取模型，选择后加入。</span></div>' +
      '<div class="channel-model-results hidden" data-upstream-results><div class="channel-model-filters"><div class="md-textfield"><input type="search" class="md-input" placeholder=" " data-upstream-search aria-label="筛选上游模型"><label class="md-input-label">筛选模型 ID</label></div><button type="button" class="md-btn md-btn-text md-btn-compact" data-upstream-select>选择筛选结果</button><button type="button" class="md-btn md-btn-text md-btn-compact" data-upstream-clear>清空选择</button></div><div class="channel-model-list" data-upstream-list></div><div class="channel-model-footer"><span data-upstream-count role="status" aria-live="polite"></span><div class="list-actions"><button type="button" class="md-btn md-btn-outlined" data-upstream-all>一键加入全部</button><button type="button" class="md-btn md-btn-filled" data-upstream-add>加入所选</button></div></div></div>';
  }
  function status(picker, message, error) {
    const target = picker.root.querySelector('[data-upstream-status]');
    target.textContent = message;
    target.classList.toggle('channel-model-error', Boolean(error));
  }
  function filtered(picker) {
    const query = picker.root.querySelector('[data-upstream-search]').value.trim().toLowerCase();
    return picker.models.filter(function (id) { return !query || id.toLowerCase().includes(query); });
  }
  function existing(picker) {
    if (picker.form) return new Set(ids(picker.form.elements.models.value));
    const channel = channels.find(function (item) { return Number(item.id) === picker.channelID; });
    return new Set(ids(channel && channel.models));
  }
  function updateCount(picker) {
    const bound = existing(picker);
    const count = Array.from(picker.selected).filter(function (id) { return !bound.has(id); }).length;
    picker.root.querySelector('[data-upstream-count]').textContent = '上游 ' + picker.models.length + ' 个 · 已选择 ' + count + ' 个新模型';
    picker.root.querySelector('[data-upstream-add]').disabled = picker.loading || picker.saving || count === 0;
    picker.root.querySelector('[data-upstream-all]').disabled = picker.loading || picker.saving || !picker.models.some(function (id) { return !bound.has(id); });
  }
  function render(picker) {
    const bound = existing(picker), visible = filtered(picker);
    picker.root.querySelector('[data-upstream-list]').innerHTML = visible.length ? visible.slice(0, 200).map(function (id) {
      const already = bound.has(id);
      return '<label class="channel-model-choice' + (already ? ' is-bound' : '') + '"><input type="checkbox" data-upstream-model value="' + api.escapeHTML(id) + '"' + (already || picker.selected.has(id) ? ' checked' : '') + (already ? ' disabled' : '') + '><code>' + api.escapeHTML(id) + '</code><span>' + (already ? '已关联' : '可加入') + '</span></label>';
    }).join('') + (visible.length > 200 ? '<p class="field-hint">显示前 200 个模型，可输入模型 ID 筛选；“选择筛选结果”和“一键加入全部”包含完整目录。</p>' : '') : '<div class="empty">' + (picker.models.length ? '没有匹配的模型。' : '上游返回的模型目录为空，请检查供应商配置后重新拉取。') + '</div>';
    const results = picker.root.querySelector('[data-upstream-results]');
    if (api.transitionVisibility) api.transitionVisibility(results, true, function (shown) { results.classList.toggle('hidden', !shown); });
    else results.classList.remove('hidden');
    updateCount(picker);
  }
  function invalidate(picker) {
    picker.sequence++;
    picker.loading = false;
    api.showBusy(picker.root.querySelector('[data-upstream-fetch]'), false);
    picker.models = []; picker.selected.clear();
    picker.root.querySelector('[data-upstream-results]').classList.add('hidden');
    status(picker, picker.form ? '填写上游地址及 Key 后拉取；编辑时 Key 留空使用已保存的凭据。' : '选择渠道后拉取上游模型。');
    updateCount(picker);
  }
  function draftBody(form) {
    const body = { type: Number(form.elements.provider_type.value), key: form.elements.key.value, base_url: form.elements.base_url.value.trim() };
    if (Number(form.elements.id.value) > 0) body.channel_id = Number(form.elements.id.value);
    ['header_override', 'setting', 'settings', 'other', 'openai_organization'].forEach(function (name) { if (form.elements[name]) body[name] = form.elements[name].value.trim(); });
    ['header_override', 'setting', 'settings'].forEach(function (name) {
      if (!body[name]) return;
      let parsed;
      try { parsed = JSON.parse(body[name]); } catch (_) { throw new Error('请先填写有效的高级配置 JSON：' + name); }
      if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) throw new Error('高级配置应为 JSON 对象：' + name);
    });
    if (!body.channel_id && !String(body.key).trim() && ![4, 61].includes(body.type)) throw new Error('请先填写上游 Key。');
    return body;
  }
  async function fetchModels(picker) {
    if (picker.loading || picker.saving) return;
    if (!picker.form && !picker.channelID) { status(picker, '请先选择一个渠道。', true); return; }
    const sequence = ++picker.sequence;
    const button = picker.root.querySelector('[data-upstream-fetch]');
    picker.loading = true; api.showBusy(button, true); updateCount(picker);
    status(picker, '正在连接上游并读取模型目录…');
    try {
      const result = picker.form ? await api.request('/api/channel/fetch_models', { method: 'POST', body: draftBody(picker.form) }) : await api.request('/api/channel/fetch_models/' + picker.channelID);
      if (sequence !== picker.sequence) return;
      if (!Array.isArray(result)) throw new Error('上游模型目录格式无效，请重试。');
      picker.models = ids(result);
      const bound = existing(picker);
      picker.selected = new Set(picker.models.filter(function (id) { return !bound.has(id); }));
      render(picker);
      status(picker, picker.models.length ? '已获取 ' + picker.models.length + ' 个模型。已有模型保持关联。' : '上游目录为空，请检查配置后重试。', !picker.models.length);
    } catch (error) {
      if (sequence !== picker.sequence) return;
      picker.models = []; picker.selected.clear(); picker.root.querySelector('[data-upstream-results]').classList.add('hidden');
      status(picker, error.message + ' 可点击“拉取上游目录”重试。', true); api.toast(error.message, 'danger');
    } finally {
      if (sequence === picker.sequence) {
        picker.loading = false; api.showBusy(button, false); updateCount(picker);
      }
    }
  }
  async function addModels(picker, all) {
    if (picker.loading || picker.saving || !canImport()) return;
    const bound = existing(picker);
    const selected = (all ? picker.models : Array.from(picker.selected)).filter(function (id) { return !bound.has(id); });
    if (!selected.length) return;
    if (picker.form) {
      picker.form.elements.models.value = ids(Array.from(bound).concat(selected)).join(',');
      picker.form.elements.models.dispatchEvent(new Event('input', { bubbles: true }));
      picker.selected.clear(); render(picker);
      status(picker, '已加入 ' + selected.length + ' 个模型，保存渠道后立即生效。');
      api.toast('已加入渠道草稿，点击“保存”生效');
      return;
    }
    picker.saving = true;
    const dialog = picker.root.closest('.dialog-backdrop');
    if (dialog) dialog.dataset.dialogBusy = 'true';
    const button = picker.root.querySelector(all ? '[data-upstream-all]' : '[data-upstream-add]');
    const select = picker.root.querySelector('[data-upstream-channel]');
    select.disabled = true; api.showBusy(button, true); updateCount(picker);
    status(picker, '正在核对上游目录并关联模型…');
    try {
      const result = await api.request('/api/channel/' + picker.channelID + '/models/import', { method: 'POST', body: { models: selected } });
      const channel = channels.find(function (item) { return Number(item.id) === picker.channelID; });
      if (channel && result && typeof result.models === 'string') channel.models = result.models;
      picker.selected.clear();
      status(picker, '已关联 ' + ids(result && result.added_models).length + ' 个新模型。请点击“一键设置模型价格”补齐定价。');
      api.toast('渠道模型已关联，模型配置已同步');
      try {
        if (refreshChannels) await refreshChannels();
        const models = window.FurtherUIAdminModels;
        if (models) await (models.refresh ? models.refresh() : models.load());
      } catch (error) { status(picker, '模型已保存，目录刷新失败：' + error.message + '。可使用页面刷新按钮重试。', true); }
      render(picker);
    } catch (error) { status(picker, error.message + '，请重试或重新拉取。', true); api.toast(error.message, 'danger'); }
    finally {
      picker.saving = false; select.disabled = false;
      if (dialog) delete dialog.dataset.dialogBusy;
      api.showBusy(button, false); updateCount(picker);
    }
  }
  function bindPicker(root, form) {
    const picker = { root: root, form: form || null, channelID: 0, models: [], selected: new Set(), sequence: 0, loading: false, saving: false };
    root.querySelector('[data-upstream-fetch]').addEventListener('click', function () { fetchModels(picker); });
    root.querySelector('[data-upstream-search]').addEventListener('input', function () { render(picker); });
    root.querySelector('[data-upstream-list]').addEventListener('change', function (event) {
      const input = event.target.closest('[data-upstream-model]');
      if (!input) return;
      if (input.checked) picker.selected.add(input.value); else picker.selected.delete(input.value);
      updateCount(picker);
    });
    root.querySelector('[data-upstream-select]').addEventListener('click', function () { const bound = existing(picker); filtered(picker).forEach(function (id) { if (!bound.has(id)) picker.selected.add(id); }); render(picker); });
    root.querySelector('[data-upstream-clear]').addEventListener('click', function () { picker.selected.clear(); render(picker); });
    root.querySelector('[data-upstream-add]').addEventListener('click', function () { addModels(picker, false); });
    root.querySelector('[data-upstream-all]').addEventListener('click', function () { addModels(picker, true); });
    return picker;
  }
  function updateChannelOptions(picker) {
    const select = picker.root.querySelector('[data-upstream-channel]');
    select.innerHTML = '<option value="">请选择渠道</option>' + channels.map(function (item) { return '<option value="' + Number(item.id) + '">' + api.escapeHTML(item.name || ('渠道 #' + item.id)) + (Number(item.status) === 1 ? '' : '（停用）') + '</option>'; }).join('');
    select.value = String(picker.channelID || '');
  }
  function ensureSavedPicker() {
    if (savedPicker && savedPicker.root.isConnected) return savedPicker;
    const dialog = document.createElement('div'); dialog.id = 'channel-model-picker-dialog'; dialog.className = 'dialog-backdrop';
    dialog.innerHTML = '<div class="md-dialog wide channel-model-dialog" aria-labelledby="channel-model-picker-title"><div class="md-dialog-title" id="channel-model-picker-title">从渠道拉取模型</div><p class="field-hint">读取渠道上游目录，将所选模型关联到渠道并同步至模型配置。</p><div data-upstream-picker><div class="md-textfield"><select class="md-input" data-upstream-channel aria-label="上游渠道"><option value="">请选择渠道</option></select><label class="md-input-label">渠道</label></div>' + controls() + '</div><div class="form-actions"><button type="button" class="md-btn md-btn-outlined" data-upstream-pricing>一键设置模型价格</button><button type="button" class="md-btn md-btn-text" data-upstream-close>完成</button></div></div>';
    document.body.append(dialog); savedPicker = bindPicker(dialog.querySelector('[data-upstream-picker]'));
    function close() { if (savedPicker.saving) return; invalidate(savedPicker); api.closeDialog(dialog.id); }
    dialog.querySelector('[data-upstream-close]').addEventListener('click', close);
    const pricing = dialog.querySelector('[data-upstream-pricing]');
    pricing.hidden = Number(document.body.dataset.gatewayRole || 100) < 100;
    pricing.addEventListener('click', function () {
      if (savedPicker.saving || !window.FurtherUIModelImports) return;
      const channel = channels.find(function (entry) { return Number(entry.id) === savedPicker.channelID; });
      close(); window.FurtherUIModelImports.openPricing(channel ? { models: ids(channel.models) } : {});
    });
    dialog.addEventListener('click', function (event) { if (event.target === dialog) close(); });
    dialog.querySelector('[data-upstream-channel]').addEventListener('change', function (event) { savedPicker.channelID = Number(event.target.value); invalidate(savedPicker); });
    dialog._onClose = function () { if (!savedPicker.saving) invalidate(savedPicker); };
    return savedPicker;
  }
  async function open(options) {
    if (!canImport()) { api.toast('当前账号没有渠道配置权限', 'danger'); return; }
    const picker = ensureSavedPicker();
    if (picker.saving) return;
    invalidate(picker); picker.channelID = Number(options && options.channelID || 0);
    picker.root.closest('.dialog-backdrop')._onClose = function () { if (!picker.saving) invalidate(picker); };
    updateChannelOptions(picker); api.openDialog('channel-model-picker-dialog');
    if (!channels.length) {
      status(picker, '正在加载渠道…');
      try { setChannels(await api.request('/api/admin/channels')); updateChannelOptions(picker); }
      catch (error) { status(picker, '渠道加载失败：' + error.message + '。请刷新页面后重试。', true); return; }
    }
    if (picker.channelID) fetchModels(picker);
    else if (!channels.length) status(picker, '还没有渠道，请先创建渠道。');
    else status(picker, '选择渠道后拉取上游模型。');
  }
  function mountForm(form) {
    if (!form || draftPickers.has(form)) return;
    const container = document.createElement('section'); container.className = 'channel-model-draft'; container.dataset.upstreamDraft = '';
    container.innerHTML = controls(); form.elements.models.closest('.md-textfield').after(container);
    const picker = bindPicker(container, form); draftPickers.set(form, picker);
    const fill = document.createElement('button'); fill.type = 'button'; fill.className = 'md-btn md-btn-tonal'; fill.dataset.upstreamFillDraft = '';
    fill.textContent = '一键读取并补齐模型';
    container.querySelector('.channel-model-toolbar').prepend(fill);
    fill.addEventListener('click', async function () {
      if (picker.loading || picker.saving || fill.disabled) return;
      api.showBusy(fill, true);
      const beforeSequence = picker.sequence;
      await fetchModels(picker);
      if (picker.sequence === beforeSequence + 1 && picker.models.length) await addModels(picker, true);
      api.showBusy(fill, false);
    });
    form.addEventListener('input', function (event) {
      if (['key', 'base_url', 'setting', 'settings', 'header_override', 'other', 'openai_organization'].includes(event.target.name)) invalidate(picker);
      if (event.target.name === 'models' && picker.models.length) render(picker);
    });
    form.elements.provider_type.addEventListener('change', function () { invalidate(picker); });
    form.addEventListener('reset', function () { invalidate(picker); });
    invalidate(picker);
  }
  function resetForm(form) { const picker = draftPickers.get(form); if (picker) invalidate(picker); }
  function setChannels(rows) { channels = Array.isArray(rows) ? rows : []; }
  window.FurtherUIAdminChannelModels = { open: open, mountForm: mountForm, resetForm: resetForm, setChannels: setChannels, setRefreshHandler: function (handler) { refreshChannels = handler; } };
}());
