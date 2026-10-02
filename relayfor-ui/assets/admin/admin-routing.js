(function () {
  'use strict';
  const api = window.ProxyLan;
  let channelRows = [];
  let loading = null;

  function channelOptions(selected) {
    return channelRows.map(function (channel) {
      return '<option value="' + Number(channel.id) + '"' + (Number(channel.id) === Number(selected) ? ' selected' : '') + '>' + api.escapeHTML(channel.name || ('渠道 #' + channel.id)) + '</option>';
    }).join('');
  }

  function setChannels() {}
  function setModels() {}

  function resetRuleForm() {
    const form = document.querySelector('[data-model-routing-rule-form]');
    form.reset();
    form.elements.channel_id.innerHTML = channelOptions();
    syncRoutingForm();
  }

  function syncRoutingForm() {
    const form = document.querySelector('[data-model-routing-rule-form]');
    const channel = channelRows.find(function (item) { return Number(item.id) === Number(form.elements.channel_id.value); });
    form.elements.priority.value = channel ? Number(channel.priority || 0) : 0;
    form.elements.weight.value = channel ? Number(channel.weight || 0) : 0;
    api.setText('[data-routing-channel-models]', channel ? channel.models : '暂无渠道');
    const submit = form.querySelector('[type="submit"]');
    if (submit) submit.disabled = !channel;
  }

  function resetAliasForm() {
    const form = document.querySelector('[data-model-alias-form]');
    form.reset();
    form.elements.channel_id.innerHTML = channelOptions();
    form.elements.channel_id.disabled = false;
    form.elements.original_alias.value = '';
    api.setText('[data-model-alias-dialog-title]', '新建模型别名');
    const submit = form.querySelector('[type="submit"]');
    if (submit) submit.disabled = !channelRows.length;
  }

  function render() {
    const rules = document.querySelector('[data-model-routing-rule-rows]');
    const aliases = document.querySelector('[data-model-alias-rows]');
    rules.innerHTML = channelRows.length ? '<div class="usage-table-scroll"><table class="usage-table"><thead><tr><th>渠道</th><th>模型</th><th>分组</th><th>优先级</th><th>权重</th><th>状态</th><th>操作</th></tr></thead><tbody>' + channelRows.map(function (channel) {
      return '<tr><td><b>' + api.escapeHTML(channel.name || ('渠道 #' + channel.id)) + '</b></td><td><code>' + api.escapeHTML(channel.models) + '</code></td><td>' + api.escapeHTML(channel.group) + '</td><td>' + Number(channel.priority || 0) + '</td><td>' + Number(channel.weight || 0) + '</td><td>' + api.statusLabel(channel.status, 'channel') + '</td><td><button class="md-icon-button" type="button" title="编辑渠道路由" data-root-only data-edit-routing-channel="' + Number(channel.id) + '"><span class="material-symbol" aria-hidden="true">edit</span></button></td></tr>';
    }).join('') + '</tbody></table></div>' : '<div class="empty">暂无渠道配置。</div>';

    const aliasRows = [];
    channelRows.forEach(function (channel) {
      Object.entries(channel.model_mapping || {}).forEach(function (entry) {
        aliasRows.push({ channel: channel, alias: entry[0], target: entry[1] });
      });
    });
    aliases.innerHTML = aliasRows.length ? '<div class="usage-table-scroll"><table class="usage-table"><thead><tr><th>请求模型别名</th><th>上游模型</th><th>渠道</th><th>操作</th></tr></thead><tbody>' + aliasRows.map(function (row, index) {
      return '<tr><td><code>' + api.escapeHTML(row.alias) + '</code></td><td><code>' + api.escapeHTML(row.target) + '</code></td><td>' + api.escapeHTML(row.channel.name) + '</td><td><div class="table-actions"><button class="md-icon-button" type="button" title="编辑别名" data-root-only data-edit-model-alias="' + index + '"><span class="material-symbol" aria-hidden="true">edit</span></button><button class="md-icon-button danger" type="button" title="删除别名" data-root-only data-delete-model-alias="' + index + '"><span class="material-symbol" aria-hidden="true">delete</span></button></div></td></tr>';
    }).join('') + '</tbody></table></div>' : '<div class="empty">暂无模型别名。</div>';

    rules.querySelectorAll('[data-edit-routing-channel]').forEach(function (button) {
      button.addEventListener('click', function () {
        resetRuleForm();
        document.querySelector('[data-model-routing-rule-form]').elements.channel_id.value = button.dataset.editRoutingChannel;
        syncRoutingForm();
        api.openDialog('model-routing-rule-dialog');
      });
    });
    aliases.querySelectorAll('[data-edit-model-alias]').forEach(function (button) {
      button.addEventListener('click', function () {
        const row = aliasRows[Number(button.dataset.editModelAlias)];
        resetAliasForm();
        const form = document.querySelector('[data-model-alias-form]');
        form.elements.channel_id.value = row.channel.id;
        form.elements.channel_id.disabled = true;
        form.elements.original_alias.value = row.alias;
        form.elements.alias.value = row.alias;
        form.elements.target_model.value = row.target;
        api.setText('[data-model-alias-dialog-title]', '编辑模型别名');
        api.openDialog('model-alias-dialog');
      });
    });
    aliases.querySelectorAll('[data-delete-model-alias]').forEach(function (button) {
      button.addEventListener('click', async function () {
        const row = aliasRows[Number(button.dataset.deleteModelAlias)];
        if (!(await api.confirm('删除后，该渠道不再接受此模型别名。', { title: '删除模型别名', confirmText: '删除' }))) return;
        try {
          await api.request('/api/admin/routing/channels/' + row.channel.id + '/aliases', { method: 'DELETE', body: { original_alias: row.alias, alias: row.alias } });
          await loadRouting();
          if (window.ProxyLanAdminModels) await window.ProxyLanAdminModels.load();
          api.toast('模型别名已删除');
        } catch (error) { api.toast(error.message, 'danger'); }
      });
    });
  }

  async function loadRouting() {
    if (loading) return loading;
    loading = (async function () {
      try {
        const result = await api.request('/api/admin/routing/channels');
        if (!Array.isArray(result)) throw new Error('服务器未返回有效渠道列表');
        channelRows = result;
        render();
      } catch (error) {
        ['[data-model-routing-rule-rows]', '[data-model-alias-rows]'].forEach(function (selector) {
          const body = document.querySelector(selector);
          if (body) body.innerHTML = '<div class="empty">加载失败：' + api.escapeHTML(error.message) + '。请点击刷新重试。</div>';
        });
        throw error;
      } finally { loading = null; }
    }());
    return loading;
  }

  function bindRouting() {
    const routing = document.querySelector('[data-model-routing-rule-form]');
    const aliases = document.querySelector('[data-model-alias-form]');
    routing.elements.channel_id.addEventListener('change', syncRoutingForm);
    routing.addEventListener('submit', async function (event) {
      event.preventDefault();
      const button = routing.querySelector('[type="submit"]');
      if (!routing.elements.channel_id.value) return;
      api.showBusy(button, true);
      try {
        await api.request('/api/admin/routing/channels/' + routing.elements.channel_id.value, { method: 'PUT', body: {
          priority: Number(routing.elements.priority.value), weight: Number(routing.elements.weight.value)
        } });
        api.closeDialog('model-routing-rule-dialog');
        await loadRouting();
        if (window.ProxyLanAdminModels) await window.ProxyLanAdminModels.load();
        api.toast('渠道路由已保存');
      } catch (error) { api.toast(error.message, 'danger'); }
      finally { api.showBusy(button, false); }
    });
    aliases.addEventListener('submit', async function (event) {
      event.preventDefault();
      const button = aliases.querySelector('[type="submit"]');
      if (!aliases.elements.channel_id.value) return;
      api.showBusy(button, true);
      try {
        await api.request('/api/admin/routing/channels/' + aliases.elements.channel_id.value + '/aliases', { method: 'PUT', body: {
          original_alias: aliases.elements.original_alias.value,
          alias: aliases.elements.alias.value.trim(), target_model: aliases.elements.target_model.value.trim()
        } });
        api.closeDialog('model-alias-dialog');
        await loadRouting();
        if (window.ProxyLanAdminModels) await window.ProxyLanAdminModels.load();
        api.toast('模型别名已保存');
      } catch (error) { api.toast(error.message, 'danger'); }
      finally { api.showBusy(button, false); }
    });
    const refresh = document.querySelector('[data-admin-routing-refresh]');
    refresh.addEventListener('click', function () {
      api.showBusy(refresh, true);
      loadRouting().catch(function (error) { api.toast(error.message, 'danger'); }).finally(function () { api.showBusy(refresh, false); });
    });
  }

  window.ProxyLanAdminRouting = { load: loadRouting, bind: bindRouting, setChannels: setChannels, setModels: setModels, resetRuleForm: resetRuleForm, resetAliasForm: resetAliasForm };
}());
