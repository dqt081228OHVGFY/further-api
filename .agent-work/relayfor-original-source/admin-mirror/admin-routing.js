(function () {
  'use strict';
  const api = window.ProxyLan;

  let ruleRows = [];
  let aliasRows = [];
  let channelRows = [];
  let modelList = [];

  function setChannels(list) {
    channelRows = Array.isArray(list) ? list : [];
    updateAvailableDatalist();
  }

  function setModels(list) {
    modelList = Array.isArray(list) ? list.map(function (m) { return m.model || m; }) : [];
    updateAvailableDatalist();
  }

  function updateAvailableDatalist() {
    const datalist = document.getElementById('available-models-datalist');
    if (!datalist) return;
    const set = new Set();
    modelList.forEach(function (m) { if (m) set.add(m); });
    channelRows.forEach(function (ch) {
      if (ch.models) {
        ch.models.split(',').forEach(function (m) {
          const trimmed = m.trim();
          if (trimmed && trimmed !== '*') set.add(trimmed);
        });
      }
    });
    datalist.innerHTML = Array.from(set).sort().map(function (m) {
      return '<option value="' + api.escapeHTML(m) + '">';
    }).join('');
  }

  function channelName(id) {
    const num = Number(id);
    const found = channelRows.find(function (c) { return Number(c.id) === num; });
    return found ? (found.name || ('渠道 #' + num)) : ('渠道 #' + num);
  }

  function fallbackConfig(item) {
    return item && typeof item === 'object' ? item : { channel_id: Number(item || 0) };
  }

  function routingBindingSummary(item, requestModel) {
    const upstreamModel = String(item && item.upstream_model || requestModel || '').trim();
    const mapped = upstreamModel && upstreamModel !== String(requestModel || '').trim() ? ' <span class="routing-binding-arrow">→</span> <code>' + api.escapeHTML(upstreamModel) + '</code>' : '';
    const upstreamFree = item && item.upstream_free ? ' <span class="tag success">上游免费</span>' : '';
    return mapped + upstreamFree;
  }

  function strategyBadge(strategy) {
    switch (strategy) {
      case 'percentage':
        return '<span class="tag info">百分比流量分流</span>';
      case 'success_rate':
        return '<span class="tag success">成功率加权 (近60次)</span>';
      case 'priority_weight':
      default:
        return '<span class="tag">基础优先级与权重</span>';
    }
  }

  function refreshModelCatalog() {
    if (window.ProxyLanAdminModels && typeof window.ProxyLanAdminModels.load === 'function') {
      return window.ProxyLanAdminModels.load();
    }
    return Promise.resolve();
  }

  function renderRules(rules) {
    const container = document.querySelector('[data-model-routing-rule-rows]');
    if (!container) return;
    ruleRows = Array.isArray(rules) ? rules : [];
    if (!ruleRows.length) {
      container.innerHTML = '<div class="empty">暂未配置高级路由规则，所有模型将遵循默认渠道路由。</div>';
      return;
    }

    container.innerHTML = '<div class="usage-table-scroll"><table class="usage-table"><thead><tr>' +
      '<th>请求模型</th>' +
      '<th>路由策略</th>' +
      '<th>首选原地重试</th>' +
      '<th>主候选渠道</th>' +
      '<th>有序兜底渠道</th>' +
      '<th>启用状态</th>' +
      '<th>操作</th>' +
      '</tr></thead><tbody>' +
      ruleRows.map(function (r) {
        const primaries = Array.isArray(r.primary_channels) ? r.primary_channels : [];
        const primaryMarkup = primaries.length ? primaries.map(function (p) {
          return '<span class="routing-channel-summary"><span class="tag">' + api.escapeHTML(channelName(p.channel_id)) + ' <b>' + Number(p.percent || 0) + '%</b></span>' + routingBindingSummary(p, r.model) + '</span>';
        }).join(' ') : '<span class="card-hint">除兜底外的全部匹配渠道</span>';

        const fallbacks = Array.isArray(r.fallback_channels) ? r.fallback_channels : [];
        const fallbackMarkup = fallbacks.length ? fallbacks.map(function (raw, idx) {
          const item = fallbackConfig(raw);
          return '<span class="routing-channel-summary"><span class="tag warn">#' + (idx + 1) + ' ' + api.escapeHTML(channelName(item.channel_id)) + '</span>' + routingBindingSummary(item, r.model) + '</span>';
        }).join(' → ') : '<span class="card-hint">无兜底</span>';

        const retryMarkup = r.retry_on_primary > 0 ?
          '<span class="tag info">' + r.retry_on_primary + ' 次</span>' :
          '<span class="card-hint">不原地重试</span>';

        return '<tr>' +
          '<td><b><code>' + api.escapeHTML(r.model) + '</code></b></td>' +
          '<td>' + strategyBadge(r.strategy) + '</td>' +
          '<td>' + retryMarkup + '</td>' +
          '<td>' + primaryMarkup + '</td>' +
          '<td>' + fallbackMarkup + '</td>' +
          '<td><label class="switch-mini"><input type="checkbox" data-rule-toggle="' + r.id + '"' + (r.enabled ? ' checked' : '') + '><span class="switch-mini-slider"></span></label></td>' +
          '<td><div class="table-actions routing-table-actions">' +
          '<button type="button" class="md-icon-button" title="编辑路由规则" aria-label="编辑路由规则" data-edit-routing-rule="' + r.id + '"><span class="material-symbol" aria-hidden="true">edit</span></button>' +
          '<button type="button" class="md-icon-button danger" title="删除路由规则" aria-label="删除路由规则" data-delete-routing-rule="' + r.id + '"><span class="material-symbol" aria-hidden="true">delete</span></button>' +
          '</div></td>' +
          '</tr>';
      }).join('') +
      '</tbody></table></div>';

    // 绑定行内事件
    container.querySelectorAll('[data-rule-toggle]').forEach(function (toggle) {
      toggle.addEventListener('change', async function () {
        const id = toggle.dataset.ruleToggle;
        const enabled = toggle.checked;
        try {
          await api.request('/api/admin/routing/rules/' + id + '/status', {
            method: 'PATCH',
            body: { enabled: enabled }
          });
          api.toast('规则状态已更新');
		  await Promise.all([loadRouting(), refreshModelCatalog()]);
        } catch (err) {
          toggle.checked = !enabled;
          api.toast(err.message, 'danger');
        }
      });
    });

    container.querySelectorAll('[data-edit-routing-rule]').forEach(function (btn) {
      btn.addEventListener('click', function () {
        const id = Number(btn.dataset.editRoutingRule);
        const rule = ruleRows.find(function (item) { return item.id === id; });
        if (rule) editRule(rule);
      });
    });

    container.querySelectorAll('[data-delete-routing-rule]').forEach(function (btn) {
      btn.addEventListener('click', async function () {
        const id = btn.dataset.deleteRoutingRule;
        const confirmed = await api.confirm('确定要删除该模型的路由规则吗？未配置规则的模型将恢复为默认路由。');
        if (!confirmed) return;
        try {
          await api.request('/api/admin/routing/rules/' + id, { method: 'DELETE' });
          api.toast('路由规则已删除');
          await Promise.all([loadRouting(), refreshModelCatalog()]);
        } catch (err) {
          api.toast(err.message, 'danger');
        }
      });
    });
  }

  function renderAliases(aliases) {
    const container = document.querySelector('[data-model-alias-rows]');
    if (!container) return;
    aliasRows = Array.isArray(aliases) ? aliases : [];
    if (!aliasRows.length) {
      container.innerHTML = '<div class="empty">暂无模型别名映射。</div>';
      return;
    }

    container.innerHTML = '<div class="usage-table-scroll"><table class="usage-table"><thead><tr>' +
      '<th>外部请求别名 (Alias)</th>' +
      '<th>内部目标模型 (Target)</th>' +
      '<th>说明</th>' +
      '<th>启用状态</th>' +
      '<th>操作</th>' +
      '</tr></thead><tbody>' +
      aliasRows.map(function (a) {
        return '<tr>' +
          '<td><b><code>' + api.escapeHTML(a.alias) + '</code></b></td>' +
          '<td><span class="material-symbol" style="font-size:16px;vertical-align:middle;color:var(--md-sys-color-primary);" aria-hidden="true">arrow_forward</span> <code>' + api.escapeHTML(a.target_model) + '</code></td>' +
          '<td><span class="card-hint">' + api.escapeHTML(a.description || '—') + '</span></td>' +
          '<td><label class="switch-mini"><input type="checkbox" data-alias-toggle="' + a.id + '"' + (a.enabled ? ' checked' : '') + '><span class="switch-mini-slider"></span></label></td>' +
          '<td><div class="table-actions routing-table-actions">' +
          '<button type="button" class="md-icon-button" title="编辑模型别名" aria-label="编辑模型别名" data-edit-model-alias="' + a.id + '"><span class="material-symbol" aria-hidden="true">edit</span></button>' +
          '<button type="button" class="md-icon-button danger" title="删除模型别名" aria-label="删除模型别名" data-delete-model-alias="' + a.id + '"><span class="material-symbol" aria-hidden="true">delete</span></button>' +
          '</div></td>' +
          '</tr>';
      }).join('') +
      '</tbody></table></div>';

    // 绑定别名行内事件
    container.querySelectorAll('[data-alias-toggle]').forEach(function (toggle) {
      toggle.addEventListener('change', async function () {
        const id = toggle.dataset.aliasToggle;
        const enabled = toggle.checked;
        try {
          await api.request('/api/admin/routing/aliases/' + id + '/status', {
            method: 'PATCH',
            body: { enabled: enabled }
          });
          api.toast('别名状态已更新');
        } catch (err) {
          toggle.checked = !enabled;
          api.toast(err.message, 'danger');
        }
      });
    });

    container.querySelectorAll('[data-edit-model-alias]').forEach(function (btn) {
      btn.addEventListener('click', function () {
        const id = Number(btn.dataset.editModelAlias);
        const alias = aliasRows.find(function (item) { return item.id === id; });
        if (alias) editAlias(alias);
      });
    });

    container.querySelectorAll('[data-delete-model-alias]').forEach(function (btn) {
      btn.addEventListener('click', async function () {
        const id = btn.dataset.deleteModelAlias;
        const confirmed = await api.confirm('确定要删除该模型别名吗？');
        if (!confirmed) return;
        try {
          await api.request('/api/admin/routing/aliases/' + id, { method: 'DELETE' });
          api.toast('模型别名已删除');
          await loadRouting();
        } catch (err) {
          api.toast(err.message, 'danger');
        }
      });
    });
  }

  async function loadRouting() {
    try {
      const [rules, aliases] = await Promise.all([
        api.request('/api/admin/routing/rules'),
        api.request('/api/admin/routing/aliases')
      ]);
      renderRules(rules);
      renderAliases(aliases);
      updateAvailableDatalist();
    } catch (err) {
      api.toast('加载高级路由与别名失败: ' + err.message, 'danger');
    }
  }

  // ---------------------------------------------------------
  // 规则表单交互 (主候选行与兜底行动态编辑)
  // ---------------------------------------------------------

  function channelSelectOptions(selectedID) {
    if (!channelRows.length) {
      return '<option value="">(无可用渠道)</option>';
    }
    return channelRows.map(function (ch) {
      const isSel = Number(ch.id) === Number(selectedID);
      const label = '#' + ch.id + ' ' + (ch.name || '未命名') + (ch.status === 1 ? '' : ' [已禁用]');
      return '<option value="' + ch.id + '"' + (isSel ? ' selected' : '') + '>' + api.escapeHTML(label) + '</option>';
    }).join('');
  }

  function addPrimaryChannelRow(channelID, percent, upstreamModel, upstreamFree) {
    const container = document.querySelector('[data-primary-channel-rows]');
    if (!container) return;
    const row = document.createElement('div');
    row.className = 'routing-builder-row routing-primary-row';
    row.innerHTML =
      '<div class="routing-builder-fields">' +
      '<div class="md-textfield"><select class="md-input" name="primary_channel_id">' + channelSelectOptions(channelID) + '</select><label class="md-input-label">主候选渠道</label></div>' +
      '<div class="md-textfield"><input class="md-input" name="primary_channel_percent" type="number" min="0" max="100" step="1" value="' + (percent !== undefined ? percent : 50) + '" placeholder=" "><label class="md-input-label">流量占比 (%)</label></div>' +
      '<div class="md-textfield"><input class="md-input" name="primary_upstream_model" value="' + api.escapeHTML(upstreamModel || '') + '" placeholder=" "><label class="md-input-label">上游模型（留空则同请求模型）</label></div>' +
      '<label class="switch-line routing-upstream-free-option"><input type="checkbox" name="primary_upstream_free"' + (upstreamFree ? ' checked' : '') + '><span class="switch-body"><span class="switch-title">上游免费</span><span class="switch-desc">忽略该渠道的零余额限制</span></span></label>' +
      '</div>' +
      '<button type="button" class="md-icon-button danger" title="移除此候选" data-remove-row><span class="material-symbol" aria-hidden="true">delete</span></button>';

    row.querySelector('[data-remove-row]').addEventListener('click', function () {
      row.remove();
      updatePercentSummary();
    });

    const percentInput = row.querySelector('[name="primary_channel_percent"]');
    if (percentInput) {
      percentInput.addEventListener('input', updatePercentSummary);
    }
    container.appendChild(row);
    updatePercentSummary();
  }

  function addFallbackChannelRow(channelID, upstreamModel, upstreamFree) {
    const container = document.querySelector('[data-fallback-channel-rows]');
    if (!container) return;
    const row = document.createElement('div');
    row.className = 'routing-builder-row routing-fallback-row';
    row.innerHTML =
      '<span class="fallback-index-badge" data-fallback-index>1</span>' +
      '<div class="routing-builder-fields">' +
      '<div class="md-textfield"><select class="md-input" name="fallback_channel_id">' + channelSelectOptions(channelID) + '</select><label class="md-input-label">兜底渠道</label></div>' +
      '<div class="md-textfield"><input class="md-input" name="fallback_upstream_model" value="' + api.escapeHTML(upstreamModel || '') + '" placeholder=" "><label class="md-input-label">上游模型（留空则同请求模型）</label></div>' +
      '<label class="switch-line routing-upstream-free-option"><input type="checkbox" name="fallback_upstream_free"' + (upstreamFree ? ' checked' : '') + '><span class="switch-body"><span class="switch-title">上游免费</span><span class="switch-desc">忽略该渠道的零余额限制</span></span></label>' +
      '</div>' +
      '<div class="row-move-actions">' +
      '<button type="button" class="md-btn md-btn-text md-btn-compact" title="上移" data-move-up>↑</button>' +
      '<button type="button" class="md-btn md-btn-text md-btn-compact" title="下移" data-move-down>↓</button>' +
      '</div>' +
      '<button type="button" class="md-icon-button danger" title="移除此兜底" data-remove-row><span class="material-symbol" aria-hidden="true">delete</span></button>';

    row.querySelector('[data-remove-row]').addEventListener('click', function () {
      row.remove();
      refreshFallbackIndices();
    });
    row.querySelector('[data-move-up]').addEventListener('click', function () {
      if (row.previousElementSibling) {
        container.insertBefore(row, row.previousElementSibling);
        refreshFallbackIndices();
      }
    });
    row.querySelector('[data-move-down]').addEventListener('click', function () {
      if (row.nextElementSibling) {
        container.insertBefore(row.nextElementSibling, row);
        refreshFallbackIndices();
      }
    });
    container.appendChild(row);
    refreshFallbackIndices();
  }

  function refreshFallbackIndices() {
    const badges = document.querySelectorAll('[data-fallback-index]');
    badges.forEach(function (b, idx) {
      b.textContent = '顺位 ' + (idx + 1);
    });
  }

  function updatePercentSummary() {
    const inputs = document.querySelectorAll('[name="primary_channel_percent"]');
    let total = 0;
    inputs.forEach(function (input) {
      total += Number(input.value || 0);
    });
    const summaryTotal = document.querySelector('[data-percent-total]');
    if (summaryTotal) {
      summaryTotal.textContent = total + '%';
      summaryTotal.style.color = total === 100 ? 'var(--md-sys-color-primary)' : 'var(--md-sys-color-tertiary)';
    }
  }

  function resetRuleForm() {
    const form = document.querySelector('[data-model-routing-rule-form]');
    if (!form) return;
    form.reset();
    form.querySelector('[name="id"]').value = '';
    const title = document.querySelector('[data-routing-rule-dialog-title]');
    if (title) title.textContent = '新建高级路由规则';
    const errEl = document.querySelector('[data-routing-rule-form-error]');
    if (errEl) { errEl.hidden = true; errEl.textContent = ''; }
    const primaries = document.querySelector('[data-primary-channel-rows]');
    if (primaries) primaries.innerHTML = '';
    const fallbacks = document.querySelector('[data-fallback-channel-rows]');
    if (fallbacks) fallbacks.innerHTML = '';
    updatePercentSummary();
  }

  function editRule(rule) {
    resetRuleForm();
    const form = document.querySelector('[data-model-routing-rule-form]');
    if (!form) return;
    form.querySelector('[name="id"]').value = rule.id;
    form.querySelector('[name="model"]').value = rule.model;
    form.querySelector('[name="strategy"]').value = rule.strategy || 'percentage';
    form.querySelector('[name="retry_on_primary"]').value = rule.retry_on_primary || 0;
    form.querySelector('[name="enabled"]').checked = Boolean(rule.enabled);

    const title = document.querySelector('[data-routing-rule-dialog-title]');
    if (title) title.textContent = '编辑高级路由规则 (' + rule.model + ')';

    const primaries = Array.isArray(rule.primary_channels) ? rule.primary_channels : [];
    primaries.forEach(function (p) {
      addPrimaryChannelRow(p.channel_id, p.percent, p.upstream_model, p.upstream_free);
    });

    const fallbacks = Array.isArray(rule.fallback_channels) ? rule.fallback_channels : [];
    fallbacks.forEach(function (raw) {
      const item = fallbackConfig(raw);
      addFallbackChannelRow(item.channel_id, item.upstream_model, item.upstream_free);
    });

    updatePercentSummary();
    api.openDialog('model-routing-rule-dialog');
  }

  function resetAliasForm() {
    const form = document.querySelector('[data-model-alias-form]');
    if (!form) return;
    form.reset();
    form.querySelector('[name="id"]').value = '';
    const title = document.querySelector('[data-model-alias-dialog-title]');
    if (title) title.textContent = '新建模型别名';
    const errEl = document.querySelector('[data-model-alias-form-error]');
    if (errEl) { errEl.hidden = true; errEl.textContent = ''; }
  }

  function editAlias(alias) {
    resetAliasForm();
    const form = document.querySelector('[data-model-alias-form]');
    if (!form) return;
    form.querySelector('[name="id"]').value = alias.id;
    form.querySelector('[name="alias"]').value = alias.alias;
    form.querySelector('[name="target_model"]').value = alias.target_model;
    form.querySelector('[name="description"]').value = alias.description || '';
    form.querySelector('[name="enabled"]').checked = Boolean(alias.enabled);

    const title = document.querySelector('[data-model-alias-dialog-title]');
    if (title) title.textContent = '编辑模型别名 (' + alias.alias + ')';

    api.openDialog('model-alias-dialog');
  }

  function bindRouting() {
    // 刷新按钮
    const refreshBtn = document.querySelector('[data-admin-routing-refresh]');
    if (refreshBtn) {
      refreshBtn.addEventListener('click', function () {
        api.showBusy(refreshBtn, true);
        loadRouting().finally(function () { api.showBusy(refreshBtn, false); });
      });
    }

    // 添加主候选按钮
    const addPrimaryBtn = document.querySelector('[data-add-primary-channel]');
    if (addPrimaryBtn) {
      addPrimaryBtn.addEventListener('click', function () {
        addPrimaryChannelRow(channelRows[0] ? channelRows[0].id : 0, 50);
      });
    }

    // 添加兜底渠道按钮
    const addFallbackBtn = document.querySelector('[data-add-fallback-channel]');
    if (addFallbackBtn) {
      addFallbackBtn.addEventListener('click', function () {
        addFallbackChannelRow(channelRows[0] ? channelRows[0].id : 0);
      });
    }

    // 规则提交
    const ruleForm = document.querySelector('[data-model-routing-rule-form]');
    if (ruleForm) {
      ruleForm.addEventListener('submit', async function (e) {
        e.preventDefault();
        const errEl = document.querySelector('[data-routing-rule-form-error]');
        if (errEl) { errEl.hidden = true; errEl.textContent = ''; }

        const id = ruleForm.querySelector('[name="id"]').value.trim();
        const model = ruleForm.querySelector('[name="model"]').value.trim();
        const strategy = ruleForm.querySelector('[name="strategy"]').value;
        const retryOnPrimary = parseInt(ruleForm.querySelector('[name="retry_on_primary"]').value, 10) || 0;
        const enabled = ruleForm.querySelector('[name="enabled"]').checked;

        if (!model) {
          if (errEl) { errEl.textContent = '模型名称不能为空'; errEl.hidden = false; }
          return;
        }
        if (retryOnPrimary < 0 || retryOnPrimary > 10) {
          if (errEl) { errEl.textContent = '首选原地重试次数必须在 0 到 10 之间'; errEl.hidden = false; }
          return;
        }

        // 收集主候选
        const primaryChannelIDs = ruleForm.querySelectorAll('[name="primary_channel_id"]');
        const primaryPercents = ruleForm.querySelectorAll('[name="primary_channel_percent"]');
        const primaryUpstreamModels = ruleForm.querySelectorAll('[name="primary_upstream_model"]');
        const primaryUpstreamFree = ruleForm.querySelectorAll('[name="primary_upstream_free"]');
        const primaryChannels = [];
        const seenPrimary = new Set();
        let validationError = '';
        let percentTotal = 0;
        for (let i = 0; i < primaryChannelIDs.length; i++) {
          const chID = parseInt(primaryChannelIDs[i].value, 10);
          const rawPercent = Number(primaryPercents[i].value);
          const percent = Number.isFinite(rawPercent) ? rawPercent : 0;
          if (!(chID > 0)) { validationError = '请选择有效的主候选渠道'; break; }
          if (seenPrimary.has(chID)) { validationError = '同一渠道不能重复添加为主候选'; break; }
          if (!Number.isInteger(percent) || percent < 0 || percent > 100) { validationError = '流量占比必须是 0 到 100 之间的整数'; break; }
          seenPrimary.add(chID);
          percentTotal += percent;
          primaryChannels.push({
            channel_id: chID,
            percent: percent,
            upstream_model: primaryUpstreamModels[i].value.trim(),
            upstream_free: primaryUpstreamFree[i].checked
          });
        }
        if (!validationError && strategy === 'percentage' && (!primaryChannels.length || percentTotal <= 0)) {
          validationError = '百分比流量策略至少需要一个占比大于 0 的主候选渠道';
        }
        if (validationError) {
          if (errEl) { errEl.textContent = validationError; errEl.hidden = false; }
          return;
        }

        // 收集兜底候选
        const fallbackChannelIDs = ruleForm.querySelectorAll('[name="fallback_channel_id"]');
        const fallbackUpstreamModels = ruleForm.querySelectorAll('[name="fallback_upstream_model"]');
        const fallbackUpstreamFree = ruleForm.querySelectorAll('[name="fallback_upstream_free"]');
        const fallbackChannels = [];
        const seenFallback = new Set();
        for (let i = 0; i < fallbackChannelIDs.length; i++) {
          const chID = parseInt(fallbackChannelIDs[i].value, 10);
          if (!(chID > 0)) { validationError = '请选择有效的兜底渠道'; break; }
          if (seenFallback.has(chID)) { validationError = '同一渠道不能重复添加为兜底渠道'; break; }
          if (seenPrimary.has(chID)) { validationError = '同一渠道不能同时作为主候选和兜底渠道'; break; }
          seenFallback.add(chID);
          fallbackChannels.push({
            channel_id: chID,
            upstream_model: fallbackUpstreamModels[i].value.trim(),
            upstream_free: fallbackUpstreamFree[i].checked
          });
        }
        if (validationError) {
          if (errEl) { errEl.textContent = validationError; errEl.hidden = false; }
          return;
        }

        const payload = {
          model: model,
          strategy: strategy,
          retry_on_primary: retryOnPrimary,
          primary_channels: primaryChannels,
          fallback_channels: fallbackChannels,
          enabled: enabled
        };

        try {
          const url = id ? ('/api/admin/routing/rules/' + id) : '/api/admin/routing/rules';
          const method = id ? 'PUT' : 'POST';
          await api.request(url, { method: method, body: payload });
          api.closeDialog('model-routing-rule-dialog');
          api.toast(id ? '高级路由规则已更新' : '高级路由规则已创建');
          await Promise.all([loadRouting(), refreshModelCatalog()]);
        } catch (err) {
          if (errEl) {
            errEl.textContent = err.message;
            errEl.hidden = false;
          } else {
            api.toast(err.message, 'danger');
          }
        }
      });
    }

    // 别名提交
    const aliasForm = document.querySelector('[data-model-alias-form]');
    if (aliasForm) {
      aliasForm.addEventListener('submit', async function (e) {
        e.preventDefault();
        const errEl = document.querySelector('[data-model-alias-form-error]');
        if (errEl) { errEl.hidden = true; errEl.textContent = ''; }

        const id = aliasForm.querySelector('[name="id"]').value.trim();
        const alias = aliasForm.querySelector('[name="alias"]').value.trim();
        const targetModel = aliasForm.querySelector('[name="target_model"]').value.trim();
        const description = aliasForm.querySelector('[name="description"]').value.trim();
        const enabled = aliasForm.querySelector('[name="enabled"]').checked;

        if (!alias || !targetModel) {
          if (errEl) { errEl.textContent = '别名和目标模型均不能为空'; errEl.hidden = false; }
          return;
        }
        if (alias.toLowerCase() === targetModel.toLowerCase()) {
          if (errEl) { errEl.textContent = '别名不能映射为自身（会产生自环）'; errEl.hidden = false; }
          return;
        }

        const payload = {
          alias: alias,
          target_model: targetModel,
          description: description,
          enabled: enabled
        };

        try {
          const url = id ? ('/api/admin/routing/aliases/' + id) : '/api/admin/routing/aliases';
          const method = id ? 'PUT' : 'POST';
          await api.request(url, { method: method, body: payload });
          api.closeDialog('model-alias-dialog');
          api.toast(id ? '模型别名已更新' : '模型别名已创建');
          await loadRouting();
        } catch (err) {
          if (errEl) {
            errEl.textContent = err.message;
            errEl.hidden = false;
          } else {
            api.toast(err.message, 'danger');
          }
        }
      });
    }
  }

  window.ProxyLanAdminRouting = {
    load: loadRouting,
    bind: bindRouting,
    setChannels: setChannels,
    setModels: setModels,
    resetRuleForm: resetRuleForm,
    resetAliasForm: resetAliasForm
  };
}());
