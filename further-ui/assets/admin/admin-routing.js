(function () {
  'use strict';
  const api = window.FurtherUI;
  let channels = [], modelName = '', sequence = 0, loading = null, bound = false, savingBinding = false;
  const split = function (value) { return String(value || '').split(',').map(function (name) { return name.trim(); }).filter(Boolean); };
  const root = function () { return Number(document.body.dataset.gatewayRole || 100) >= 100; };
  const body = function () { return document.querySelector('[data-model-bindings]'); };
  function setChannels(rows) {
    channels = (Array.isArray(rows) ? rows : []).map(function (row) {
      let mapping = row.model_mapping || {};
      if (typeof mapping === 'string') { try { mapping = JSON.parse(mapping || '{}'); } catch (_) { mapping = {}; } }
      return Object.assign({}, row, { model_mapping: mapping });
    });
  }
  function channelOptions(id) {
    return '<option value="">请选择渠道</option>' + channels.map(function (channel) {
      return '<option value="' + Number(channel.id) + '"' + (Number(channel.id) === Number(id) ? ' selected' : '') + '>' + api.escapeHTML(channel.name || ('渠道 #' + channel.id)) + (Number(channel.status) === 1 ? '' : '（停用）') + '</option>';
    }).join('');
  }
  function rowMarkup(channel, existing) {
    const id = channel ? Number(channel.id) : 0;
    const upstream = existing && Object.hasOwn(channel.model_mapping, modelName) ? channel.model_mapping[modelName] : modelName;
    const disabled = root() ? '' : ' disabled';
    const expected = { bound: Boolean(existing), upstream_model: upstream || modelName, priority: Number(channel && channel.priority || 0), weight: Number(channel && channel.weight != null ? channel.weight : 1) };
    return '<section class="model-route-row md-card" data-model-binding-row data-binding-channel="' + id + '" data-binding-existing="' + Boolean(existing) + '" data-binding-expected="' + api.escapeHTML(JSON.stringify(expected)) + '">' +
      '<div class="model-route-heading"><b>' + (existing ? api.escapeHTML(channel.name || ('渠道 #' + id)) : '添加渠道') + '</b><span class="tag' + (channel && Number(channel.status) !== 1 ? ' error' : ' muted') + '">' + (channel && Number(channel.status) !== 1 ? '渠道停用' : existing ? '已关联' : '待保存') + '</span></div>' +
      '<div class="form-grid"><div class="md-textfield"><select class="md-input" data-binding-channel-select' + (existing ? ' disabled' : disabled) + '>' + channelOptions(id) + '</select><label class="md-input-label">渠道</label></div>' +
      '<div class="md-textfield"><input class="md-input" data-binding-upstream value="' + api.escapeHTML(upstream || modelName) + '" maxlength="255" placeholder=" "' + disabled + '><label class="md-input-label">上游模型 / 别名目标</label></div>' +
      '<div class="md-textfield"><input class="md-input" data-binding-priority type="number" min="-2147483648" max="2147483647" step="1" value="' + Number(channel && channel.priority || 0) + '" placeholder=" "' + disabled + '><label class="md-input-label">渠道优先级</label></div>' +
      '<div class="md-textfield"><input class="md-input" data-binding-weight type="number" min="0" max="10000" step="1" value="' + Number(channel && channel.weight != null ? channel.weight : 1) + '" placeholder=" "' + disabled + '><label class="md-input-label">渠道权重</label></div></div>' +
      '<p class="field-hint">请求 <code>' + api.escapeHTML(modelName || '当前模型') + '</code> → 上游模型；优先级与权重影响此渠道的全部模型。</p>' +
      '<div class="model-route-actions"><button type="button" class="md-btn md-btn-outlined md-btn-compact" data-route-preset="primary"' + disabled + '>主用</button><button type="button" class="md-btn md-btn-outlined md-btn-compact" data-route-preset="backup"' + disabled + '>备用</button><button type="button" class="md-btn md-btn-outlined md-btn-compact" data-route-preset="peer"' + disabled + '>同级分流</button><button type="button" class="md-btn md-btn-text md-btn-compact" data-binding-same-name' + disabled + '>使用同名上游</button><button type="button" class="md-btn md-btn-filled md-btn-compact" data-binding-save' + disabled + '>' + (existing ? '保存此渠道配置' : '绑定此渠道') + '</button><button type="button" class="md-btn md-btn-text md-btn-compact danger" data-binding-remove' + disabled + '>' + (existing ? '移除此模型关联' : '取消添加') + '</button></div><p class="field-hint" data-binding-status role="status"></p></section>';
  }
  function render() {
    if (!body()) return;
    const rows = channels.filter(function (channel) { return split(channel.models).includes(modelName); });
    body().innerHTML = rows.length ? rows.map(function (channel) { return rowMarkup(channel, true); }).join('') : '<div class="empty">暂无渠道关联。添加渠道后，填写上游模型并保存。</div>';
    const add = document.querySelector('[data-add-model-binding]');
    if (add) add.disabled = !root() || !modelName;
    if (api.initializeVisuals) api.initializeVisuals(body());
  }
  async function loadRouting() {
    if (!loading) loading = api.request('/api/admin/routing/channels').then(function (rows) {
      if (!Array.isArray(rows)) throw new Error('服务器未返回有效渠道列表');
      setChannels(rows);
    }).finally(function () { loading = null; });
    return loading;
  }
  function resetModel(name) { ++sequence; modelName = String(name || '').trim(); render(); }
  async function editModel(name) {
    const current = ++sequence; modelName = String(name || '').trim(); render();
    try { await loadRouting(); if (sequence === current && !hasUnsaved()) render(); }
    catch (error) { if (sequence === current && body()) body().innerHTML = '<div class="empty" role="alert">渠道读取失败：' + api.escapeHTML(error.message) + '。<button type="button" class="md-btn md-btn-text" data-binding-retry>重新读取</button></div>'; }
  }
  function addBinding() {
    if (!root()) return;
    modelName = document.querySelector('[data-model-form]').elements.model.value.trim();
    if (!modelName) { api.toast('请先填写模型标识', 'danger'); return; }
    if (!channels.length) { api.toast('还没有可用渠道，请先创建渠道', 'danger'); return; }
    const existing = new Set(Array.from(body().querySelectorAll('[data-binding-channel-select]')).map(function (select) { return Number(select.value); }));
    const available = channels.find(function (channel) { return Number(channel.status) === 1 && !existing.has(Number(channel.id)) && !split(channel.models).includes(modelName); }) || channels.find(function (channel) { return !existing.has(Number(channel.id)) && !split(channel.models).includes(modelName); });
    if (!available) { api.toast('此模型已关联所有渠道', 'danger'); return; }
    const empty = body().querySelector('.empty'); if (empty) empty.remove();
    body().insertAdjacentHTML('beforeend', rowMarkup(available, false));
    if (api.initializeVisuals) api.initializeVisuals(body());
  }
  function preset(row, mode) {
    const id = Number(row.querySelector('[data-binding-channel-select]').value);
    const channel = channels.find(function (entry) { return Number(entry.id) === id; }); if (!channel) return;
    const groups = new Set(split(channel.group || channel.groups || 'default'));
    const peers = channels.filter(function (entry) { return Number(entry.id) !== id && Number(entry.status) === 1 && split(entry.group || entry.groups || 'default').some(function (group) { return groups.has(group); }) && split(entry.models).includes(modelName); });
    const priorities = peers.map(function (entry) { return Number(entry.priority || 0); });
    const highest = priorities.length ? Math.max.apply(null, priorities) : 0, lowest = priorities.length ? Math.min.apply(null, priorities) : 0;
    const value = mode === 'primary' ? highest + 1 : mode === 'backup' ? lowest - 1 : highest;
    if (!Number.isSafeInteger(value) || value < -2147483648 || value > 2147483647) { api.toast('优先级已到边界，请手动调整', 'danger'); return; }
    row.querySelector('[data-binding-priority]').value = value; row.querySelector('[data-binding-weight]').value = 1; markDirty(row);
    row.querySelector('[data-binding-status]').textContent = '已填写优先级 ' + value + '、权重 1，请保存此渠道配置。';
  }
  function markDirty(row) { row.dataset.dirty = 'true'; row.querySelector('[data-binding-status]').textContent = '此渠道配置尚未保存'; }
  function hasUnsaved() { return savingBinding || Boolean(body() && body().querySelector('[data-dirty="true"],[data-binding-existing="false"]')); }
  async function saveBinding(row, remove) {
    if (!root() || savingBinding || row.dataset.saving === 'true') return;
    const name = document.querySelector('[data-model-form]').elements.model.value.trim(), id = Number(row.querySelector('[data-binding-channel-select]').value);
    if (!name || !id) { api.toast('请填写模型标识并选择渠道', 'danger'); return; }
    if (remove && row.dataset.bindingExisting !== 'true') { row.remove(); return; }
    if (remove && !(await api.confirm('只移除此模型在该渠道的关联和上游别名，其余模型保持现有配置。', { title: '移除模型关联', confirmText: '移除' }))) return;
    const payload = { model: name, upstream_model: row.querySelector('[data-binding-upstream]').value.trim(), priority: Number(row.querySelector('[data-binding-priority]').value), weight: Number(row.querySelector('[data-binding-weight]').value), expected: JSON.parse(row.dataset.bindingExpected) };
    if (!remove && (!payload.upstream_model || !Number.isInteger(payload.priority) || payload.priority < -2147483648 || payload.priority > 2147483647 || !Number.isInteger(payload.weight) || payload.weight < 0 || payload.weight > 10000)) { api.toast('请检查上游模型、优先级和权重', 'danger'); return; }
    const dialog = document.getElementById('model-dialog'); savingBinding = true; row.dataset.saving = 'true'; dialog.dataset.dialogBusy = 'true';
    row.querySelectorAll('button,input,select').forEach(function (element) { element.disabled = true; });
    try {
      await api.request('/api/admin/routing/channels/' + id + '/binding', { method: remove ? 'DELETE' : 'PUT', body: payload });
      delete row.dataset.dirty; row.dataset.bindingExisting = 'true';
      row.querySelector('[data-binding-status]').textContent = remove ? '模型关联已移除' : '渠道路由与上游别名已保存';
      api.toast(remove ? '模型关联已移除' : '渠道路由与上游别名已保存');
      try {
        await loadRouting();
        if (remove) row.remove(); else {
          const channel = channels.find(function (entry) { return Number(entry.id) === id; });
          if (channel) row.querySelector('.model-route-heading b').textContent = channel.name;
          if (channel) row.dataset.bindingExpected = JSON.stringify({ bound: true, upstream_model: channel.model_mapping[name] || name, priority: Number(channel.priority || 0), weight: Number(channel.weight == null ? 1 : channel.weight) });
          row.querySelector('[data-binding-save]').textContent = '保存此渠道配置';
          row.querySelector('[data-binding-remove]').textContent = '移除此模型关联';
        }
        if (window.FurtherUIAdminModels) await window.FurtherUIAdminModels.refresh();
      } catch (error) { api.toast('渠道配置已保存，列表刷新失败：' + error.message, 'danger'); }
    } catch (error) { row.querySelector('[data-binding-status]').textContent = '保存失败：' + error.message; api.toast(error.message, 'danger'); }
    finally { savingBinding = false; delete row.dataset.saving; delete dialog.dataset.dialogBusy; row.querySelectorAll('button,input,select').forEach(function (element) { element.disabled = !root() || element.hasAttribute('data-binding-channel-select') && row.dataset.bindingExisting === 'true'; }); }
  }
  function bindRouting() {
    if (bound || !body()) return; bound = true;
    document.querySelector('[data-add-model-binding]').addEventListener('click', addBinding);
    body().addEventListener('input', function (event) { const row = event.target.closest('[data-model-binding-row]'); if (row) markDirty(row); });
    body().addEventListener('change', function (event) {
      const select = event.target.closest('[data-binding-channel-select]'); if (!select) return;
      const channel = channels.find(function (entry) { return Number(entry.id) === Number(select.value); }), row = select.closest('[data-model-binding-row]');
      row.querySelector('[data-binding-priority]').value = channel ? Number(channel.priority || 0) : 0;
      row.querySelector('[data-binding-weight]').value = channel && channel.weight != null ? Number(channel.weight) : 1; markDirty(row);
      if (channel) row.dataset.bindingExpected = JSON.stringify({ bound: false, upstream_model: modelName, priority: Number(channel.priority || 0), weight: Number(channel.weight == null ? 1 : channel.weight) });
    });
    body().addEventListener('click', function (event) {
      if (event.target.closest('[data-binding-retry]')) { editModel(modelName); return; }
      const row = event.target.closest('[data-model-binding-row]'); if (!row || !root()) return;
      const button = event.target.closest('button'); if (!button || button.disabled) return;
      if (button.dataset.routePreset) preset(row, button.dataset.routePreset);
      else if (button.hasAttribute('data-binding-same-name')) { row.querySelector('[data-binding-upstream]').value = modelName; markDirty(row); }
      else if (button.hasAttribute('data-binding-save')) saveBinding(row, false);
      else if (button.hasAttribute('data-binding-remove')) saveBinding(row, true);
    });
    document.querySelector('[data-model-form]').elements.model.addEventListener('input', function () { resetModel(this.value); });
  }
  window.FurtherUIAdminRouting = { bind: bindRouting, load: loadRouting, setChannels: setChannels, setModels: function () {}, resetModel: resetModel, editModel: editModel, addBinding: addBinding, hasUnsaved: hasUnsaved };
}());
