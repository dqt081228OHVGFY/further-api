(function () {
  'use strict';
  const api = window.ProxyLan;

  let modelRows = [];
	let hideDisabledModels = api.adminListPreference('hideDisabledModels');
  // 模型要绑定渠道，但渠道列表由 admin.js 的 loadChannels 拉取。
  // 与其让本模块重复请求一次渠道接口，由 admin.js 加载完直接推进来。
  let channelRows = [];
	let healthLoaded = false;
	let editingModelRoutingManaged = false;

	function adminModelPageFromHash() {
	  const parts = location.hash.slice(1).split('/');
	  if (parts[0] === 'models') {
	    if (parts[1] === 'health') return 'health';
	    if (parts[1] === 'routing') return 'routing';
	  }
	  return 'catalog';
	}

	function showAdminModelPage(name, historyMode) {
	  const page = name === 'health' ? 'health' : (name === 'routing' ? 'routing' : 'catalog');
	  document.querySelectorAll('[data-admin-model-page]').forEach(function (element) { element.classList.toggle('hidden', element.dataset.adminModelPage !== page); });
	  document.querySelectorAll('[data-admin-model-tab]').forEach(function (button) {
		const active = button.dataset.adminModelTab === page;
		button.classList.toggle('active', active);
		button.setAttribute('aria-pressed', String(active));
	  });
	  const panel = document.querySelector('[data-panel="models"]');
	  if (panel && panel.classList.contains('active')) {
	    let title = panel.dataset.title;
	    let subtitle = panel.dataset.subtitle;
	    if (page === 'health') {
	      title = '模型健康与收费';
	      subtitle = '查看模型、渠道、缓存和实际收费指标';
	    } else if (page === 'routing') {
	      title = '模型高级路由与别名';
	      subtitle = '配置按模型的百分比分流、成功率加权、首选原地重试及兜底渠道';
	    }
		api.setText('[data-page-title]', title);
		api.setText('[data-page-subtitle]', subtitle);
	  }
	  const target = page === 'health' ? '#models/health' : (page === 'routing' ? '#models/routing' : '#models');
	  if (historyMode === 'push' && location.hash !== target) history.pushState(null, '', target);
	  if (historyMode === 'replace' && location.hash !== target) history.replaceState(null, '', target);
	}

	function healthRateMarkup(windowData) {
	  const requests = Number(windowData && windowData.requests || 0);
	  const successful = Number(windowData && windowData.successful || 0);
	  const rate = Number(windowData && windowData.success_rate || 0);
	  const tone = !requests ? 'empty' : rate >= 99 ? 'good' : rate >= 95 ? 'warn' : 'bad';
	  return '<span class="health-rate health-' + tone + '">' + (requests ? rate.toFixed(1) + '%' : '—') + '</span><small>' + api.number(successful) + ' / ' + api.number(requests) + '</small>';
	}

	function formatTTFT(value) {
	  const milliseconds = Number(value || 0);
	  if (milliseconds <= 0) return '—';
	  return milliseconds < 1000 ? Math.round(milliseconds) + ' ms' : (milliseconds / 1000).toFixed(2) + ' s';
	}

	function renderAdminModelHealth(payload) {
	  const summary = payload.summary || {};
	  api.setText('[data-admin-health-requests]', api.number(summary.today && summary.today.requests));
	  api.setText('[data-admin-health-success]', Number(summary.today && summary.today.requests) ? Number(summary.today.success_rate || 0).toFixed(1) + '%' : '—');
	  api.setText('[data-admin-health-cache]', Number(summary.prompt_tokens || 0) ? Number(summary.cache_hit_rate || 0).toFixed(1) + '%' : '—');
	  api.setText('[data-admin-health-charge]', api.moneyFromQuota(summary.charge_quota || 0, 3));
	  const updated = document.querySelector('[data-admin-model-health-updated]');
	  if (updated) updated.textContent = api.date(payload.generated_at) + ' 更新 · ' + (payload.timezone || '服务器时区');

	  const modelBody = document.querySelector('[data-admin-model-health-rows]');
	  const models = Array.isArray(payload.models) ? payload.models : [];
	  if (modelBody) modelBody.innerHTML = models.length ? models.map(function (item) {
		const prompt = Number(item.prompt_tokens || 0);
		const cached = Number(item.cached_tokens || 0);
		const cache = prompt ? Number(item.cache_hit_rate || 0).toFixed(1) + '%' : '—';
		return '<tr><td><b>' + api.escapeHTML(item.display_name || item.model) + '</b><small><code>' + api.escapeHTML(item.model) + '</code> · ' + (item.enabled ? '已启用' : '已停用') + '</small></td><td>' + healthRateMarkup(item.recent_60) + '</td><td>' + healthRateMarkup(item.today) + '</td><td><span class="health-rate">' + cache + '</span><small>' + api.number(cached) + ' / ' + api.number(prompt) + ' Token</small></td><td><b>' + api.moneyFromQuota(item.today_charge_quota || 0, 4) + '</b><small>' + api.number(item.today && item.today.requests) + ' 次请求</small></td></tr>';
	  }).join('') : '<tr><td class="empty" colspan="5">还没有模型配置。</td></tr>';

	  const channelBody = document.querySelector('[data-admin-channel-health-rows]');
	  const channels = Array.isArray(payload.channels) ? payload.channels : [];
	  if (channelBody) channelBody.innerHTML = channels.length ? channels.map(function (item) {
		const speed = Number(item.average_tokens_per_second || 0);
		return '<tr><td><b>' + api.escapeHTML(item.name || ('渠道 #' + item.channel_id)) + '</b><small>#' + Number(item.channel_id || 0) + '</small></td><td>' + api.statusLabel(item.status, 'channel') + '</td><td>' + healthRateMarkup(item.recent_60) + '</td><td>' + healthRateMarkup(item.today) + '</td><td><b>' + formatTTFT(item.average_ttft_ms) + '</b><small>' + (speed > 0 ? speed.toFixed(1) + ' tok/s' : '暂无生成速度') + '</small></td></tr>';
	  }).join('') : '<tr><td class="empty" colspan="5">还没有渠道配置。</td></tr>';
	}

	async function loadAdminModelHealth() {
	  const payload = await api.request('/api/admin/model-health');
	  renderAdminModelHealth(payload);
	  healthLoaded = true;
	}

	function bindAdminModelSubpages() {
	  document.querySelectorAll('[data-admin-model-tab]').forEach(function (button) {
		button.addEventListener('click', function () {
		  const page = button.dataset.adminModelTab;
		  showAdminModelPage(page, 'push');
		  if (page === 'health' && !healthLoaded) loadAdminModelHealth().catch(function (error) { api.toast(error.message, 'danger'); });
		  if (page === 'routing' && window.ProxyLanAdminRouting) window.ProxyLanAdminRouting.load().catch(function (error) { api.toast(error.message, 'danger'); });
		});
	  });
	  const refresh = document.querySelector('[data-admin-model-health-refresh]');
	  if (refresh) refresh.addEventListener('click', function () {
		api.showBusy(refresh, true);
		loadAdminModelHealth().catch(function (error) { api.toast(error.message, 'danger'); }).finally(function () { api.showBusy(refresh, false); });
	  });
	  function syncFromHistory() {
		const page = adminModelPageFromHash();
		showAdminModelPage(page);
		if (page === 'health' && !healthLoaded) loadAdminModelHealth().catch(function (error) { api.toast(error.message, 'danger'); });
		if (page === 'routing' && window.ProxyLanAdminRouting) window.ProxyLanAdminRouting.load().catch(function (error) { api.toast(error.message, 'danger'); });
	  }
	  window.addEventListener('popstate', syncFromHistory);
	  window.addEventListener('hashchange', syncFromHistory);
	  showAdminModelPage(adminModelPageFromHash());
	}

  function modelBindingOptions(selectedID) {
    const options = channelRows.map(function (channel) {
      const selected = Number(channel.id) === Number(selectedID) ? ' selected' : '';
      return '<option value="' + channel.id + '"' + selected + '>' + api.escapeHTML(channel.name) + '</option>';
    });
    return '<option value="">请选择渠道</option>' + options.join('');
  }

  function renderModelBindings(bindings) {
    const body = document.querySelector('[data-model-bindings]');
    if (!body) return;
    const rows = Array.isArray(bindings) ? bindings : [];
    body.innerHTML = rows.length ? rows.map(function (binding) {
      const channelName = binding.channel_name || ('渠道 #' + binding.channel_id);
      return '<div class="md-card" data-model-binding-row>' +
        '<div class="form-grid">' +
        '<div class="md-textfield"><select class="md-input" name="channel_id">' + modelBindingOptions(binding.channel_id) + '</select><label class="md-input-label">渠道</label></div>' +
        '<div class="md-textfield"><input class="md-input" name="upstream_model" value="' + api.escapeHTML(binding.upstream_model || '') + '" placeholder=" "><label class="md-input-label">上游模型</label></div>' +
        '<div class="md-textfield"><input class="md-input" name="priority" type="number" value="' + Number(binding.priority || 0) + '" placeholder=" "><label class="md-input-label">优先级</label></div>' +
        '<div class="md-textfield"><input class="md-input" name="weight" type="number" min="1" value="' + Number(binding.weight || 1) + '" placeholder=" "><label class="md-input-label">权重</label></div>' +
        '</div>' +
        '<label class="switch-line"><input type="checkbox" name="binding_enabled"' + (binding.enabled !== false ? ' checked' : '') + '><span class="switch-body"><span class="switch-title">启用此绑定</span><span class="switch-desc">' + api.escapeHTML(channelName) + '</span></span></label>' +
		'<label class="switch-line"><input type="checkbox" name="upstream_free"' + (binding.upstream_free ? ' checked' : '') + '><span class="switch-body"><span class="switch-title">上游免费</span><span class="switch-desc">允许使用零额度凭据。</span></span></label>' +
        '<button type="button" class="md-btn md-btn-text" data-remove-model-binding>移除此绑定</button>' +
        '</div>';
    }).join('') : '<div class="empty">暂无绑定，模型暂时不可用。</div>';
  }

  function applyModelBindingManagement(managed) {
    editingModelRoutingManaged = Boolean(managed);
    const fieldset = document.querySelector('[data-model-bindings-fieldset]');
    const notice = document.querySelector('[data-model-bindings-routing-managed]');
    if (fieldset) fieldset.disabled = editingModelRoutingManaged;
    if (notice) notice.hidden = !editingModelRoutingManaged;
  }

  function modelRow(item) {
	const availability = {
	  available: ['success', '正常可用'],
	  degraded: ['', '部分渠道异常'],
	  data_train_only: ['', '仅授权用户可用'],
	  unavailable: ['error', '无可用渠道'],
	  hidden: ['muted', '已停用']
	}[item.availability_status] || ['muted', '状态未知'];
	const status = '<span class="tag ' + availability[0] + '">' + availability[1] + '</span>';
	const visibility = item.visible_to_users ? '<span class="tag success">对用户显示</span>' : '<span class="tag error">对用户隐藏</span>';
    const access = item.subscription_only ? '<span class="tag">订阅专属</span>' : '<span class="tag muted">公开模型</span>';
    const prepaid = item.prepaid_compatible ? '<span class="tag success">预付卡可用</span>' : '<span class="tag muted">预付卡禁用</span>';
	const freeModel = item.free_model ? '<span class="tag success">免费模型</span>' : '';
	const dataTrainModel = item.data_train ? '<span class="tag error">data-train 模型</span>' : '';
	const sessionModel = item.session_only ? '<span class="tag error">仅会话</span>' : (item.session_enabled ? '<span class="tag success">支持认领</span>' : '');
	const upstreamFree = Number(item.upstream_free_binding_count || 0) > 0 ? '<span class="tag">上游免费 ×' + Number(item.upstream_free_binding_count) + '</span>' : '';
	const requestBilled = item.billing_mode === 'request';
	const billingTag = requestBilled ? '<span class="tag">按次计费</span>' : '<span class="tag muted">按量计费</span>';
	const remoteCompaction = item.remote_compaction_enabled ? '<span class="tag success">支持远程压缩</span>' : '';
	const advancedRouting = item.advanced_routing_enabled ? '<span class="tag info">高级路由接管</span>' : '';
	const longContext = !requestBilled && Number(item.long_context_threshold || 0) > 0 ? '<span class="tag">长上下文 &gt;' + api.number(item.long_context_threshold) + ' · 入/缓存 ×' + Number(item.long_context_input_multiplier || 1).toFixed(2) + ' · 出 ×' + Number(item.long_context_output_multiplier || 1).toFixed(2) + '</span>' : '';
    const bindingCount = Array.isArray(item.bindings) ? item.bindings.length : 0;
	const routeSummary = item.advanced_routing_enabled ? '渠道由高级路由管理 · ' + Number(item.available_channel_count || 0) + ' / ' + Number(item.enabled_binding_count || 0) + ' 可用' : Number(item.available_channel_count || 0) + ' / ' + Number(item.enabled_binding_count || 0) + ' 个启用渠道可用';
	const statusReason = item.availability_reason ? ' · ' + api.escapeHTML(item.availability_reason) : '';
    const price = function (value) { return value == null || Number(value) < 0 ? '跟随输入' : '$' + Number(value || 0).toFixed(6); };
	const normalPricing = requestBilled ? '<div><b>$' + Number(item.request_price || 0).toFixed(6) + '</b>模型按次</div>' : '<div><b>$' + Number(item.input_price || 0).toFixed(6) + '</b>输入 / 1M</div><div><b>' + price(item.cache_read_price) + '</b>缓存读 / 1M</div><div><b>' + price(item.cache_write_price) + '</b>缓存写 / 1M</div><div><b>$' + Number(item.output_price || 0).toFixed(6) + '</b>输出 / 1M</div>';
	return '<div class="list-item admin-model-item"><span class="material-symbol" aria-hidden="true">tune</span><div class="list-item-body"><div class="list-item-title">' + api.escapeHTML(item.display_name || item.model) + '</div><div class="list-item-sub"><code>' + api.escapeHTML(item.model) + '</code> · ' + bindingCount + ' 个渠道绑定 · ' + routeSummary + statusReason + '</div><div class="list-item-sub">' + (item.quota_group ? '额度组 ' + api.escapeHTML(item.quota_group) : '通用额度') + ' · 模型倍率 ×' + Number(item.price_multiplier || 1).toFixed(4) + '</div></div><div class="list-metrics">' + normalPricing + '<div><b>$' + Number(item.compaction_price || 0).toFixed(6) + '</b>远程压缩</div></div><div class="list-actions">' + freeModel + dataTrainModel + sessionModel + upstreamFree + longContext + remoteCompaction + advancedRouting + billingTag + access + prepaid + status + visibility + '<button type="button" class="md-icon-button" title="编辑" data-edit-model="' + item.id + '"><span class="material-symbol" aria-hidden="true">edit</span></button><button type="button" class="md-icon-button" title="切换状态" data-toggle-model="' + item.id + '"><span class="material-symbol" aria-hidden="true">refresh</span></button><button type="button" class="md-icon-button danger" title="删除" data-delete-model="' + item.id + '"><span class="material-symbol" aria-hidden="true">delete</span></button></div></div>';
  }

  function syncBillingMode(form) {
    const requestBilled = form.elements.billing_mode.value === 'request';
    form.querySelectorAll('[data-request-pricing]').forEach(function (section) { section.classList.toggle('hidden', !requestBilled); });
    form.querySelectorAll('[data-token-pricing]').forEach(function (section) { section.classList.toggle('hidden', requestBilled); });
  }

  function resetModelForm() {
    const form = document.querySelector('[data-model-form]');
    if (!form) return;
    form.reset();
    form.elements.id.value = '';
    form.elements.enabled.checked = true;
    form.elements.subscription_only.checked = false;
	form.elements.prepaid_compatible.checked = true;
	form.elements.free_model.checked = false;
	form.elements.data_train.checked = false;
	form.elements.session_enabled.checked = false;
	form.elements.session_only.checked = false;
	form.elements.long_context_threshold.value = 0;
	form.elements.billing_mode.value = 'token';
	form.elements.request_price.value = 0;
	form.elements.compaction_price.value = 0;
	form.elements.remote_compaction_enabled.checked = false;
	form.elements.long_context_input_multiplier.value = 1;
	form.elements.long_context_output_multiplier.value = 1;
	form.elements.quota_group.value = '';
    document.querySelector('[data-model-dialog-title]').textContent = '添加模型';
    applyModelBindingManagement(false);
    renderModelBindings([]);
	syncBillingMode(form);
  }

  function editModel(item) {
    const form = document.querySelector('[data-model-form]');
    if (!form) return;
    form.elements.id.value = item.id;
    form.elements.model.value = item.model || '';
    form.elements.display_name.value = item.display_name || '';
    form.elements.input_price.value = item.input_price || 0;
	form.elements.output_price.value = item.output_price || 0;
	form.elements.billing_mode.value = item.billing_mode === 'request' ? 'request' : 'token';
	form.elements.request_price.value = item.request_price || 0;
	form.elements.compaction_price.value = item.compaction_price || 0;
	form.elements.remote_compaction_enabled.checked = Boolean(item.remote_compaction_enabled);
    form.elements.cache_read_price.value = item.cache_read_price == null ? -1 : item.cache_read_price;
    form.elements.cache_write_price.value = item.cache_write_price == null ? -1 : item.cache_write_price;
    form.elements.price_multiplier.value = item.price_multiplier == null ? 1 : item.price_multiplier;
	form.elements.long_context_threshold.value = Number(item.long_context_threshold || 0);
	form.elements.long_context_input_multiplier.value = Number(item.long_context_input_multiplier || 1);
	form.elements.long_context_output_multiplier.value = Number(item.long_context_output_multiplier || 1);
	form.elements.quota_group.value = item.quota_group || '';
    form.elements.enabled.checked = item.enabled !== false;
    form.elements.subscription_only.checked = Boolean(item.subscription_only);
    form.elements.prepaid_compatible.checked = item.prepaid_compatible !== false;
	form.elements.free_model.checked = Boolean(item.free_model);
	form.elements.data_train.checked = Boolean(item.data_train);
	form.elements.session_enabled.checked = Boolean(item.session_enabled || item.session_only);
	form.elements.session_only.checked = Boolean(item.session_only);
    document.querySelector('[data-model-dialog-title]').textContent = '编辑模型';
    renderModelBindings(item.bindings);
	applyModelBindingManagement(item.advanced_routing_enabled);
	syncBillingMode(form);
    api.openDialog('model-dialog');
  }

  async function loadModels(refresh) {
	if (refresh !== false) {
	  const result = await api.request('/api/admin/models');
	  modelRows = Array.isArray(result) ? result : [];
	  if (window.ProxyLanAdminRouting) {
	    window.ProxyLanAdminRouting.setModels(modelRows);
	  }
	}
	showAdminModelPage(adminModelPageFromHash());
	const displayedRows = hideDisabledModels ? modelRows.filter(function (item) { return item.enabled !== false; }) : modelRows;
    const body = document.querySelector('[data-model-rows]');
    if (!body) return;
	body.innerHTML = displayedRows.length ? displayedRows.map(modelRow).join('') : '<div class="empty">' + (modelRows.length ? '停用模型已隐藏，可关闭筛选后查看。' : '还没有模型配置。') + '</div>';
    body.querySelectorAll('[data-edit-model]').forEach(function (button) {
      button.addEventListener('click', function () {
        const item = modelRows.find(function (model) { return String(model.id) === button.dataset.editModel; });
        if (item) editModel(item);
      });
    });
    body.querySelectorAll('[data-toggle-model]').forEach(function (button) {
      button.addEventListener('click', async function () {
        const item = modelRows.find(function (model) { return String(model.id) === button.dataset.toggleModel; });
        if (!item) return;
        try {
          await api.request('/api/admin/models/' + item.id + '/status', { method: 'PATCH', body: { enabled: !item.enabled } });
          api.toast('模型状态已更新');
          await loadModels();
        } catch (error) { api.toast(error.message, 'danger'); }
      });
    });
    body.querySelectorAll('[data-delete-model]').forEach(function (button) {
      button.addEventListener('click', async function () {
        if (!(await api.confirm('模型及其渠道绑定将被永久删除。', { title: '删除模型', confirmText: '删除' }))) return;
        try {
          await api.request('/api/admin/models/' + button.dataset.deleteModel, { method: 'DELETE' });
          api.toast('模型已删除');
          await loadModels();
        } catch (error) { api.toast(error.message, 'danger'); }
      });
    });
	if (adminModelPageFromHash() === 'health' && !healthLoaded) await loadAdminModelHealth();
  }

  function collectModelBindings(form) {
    return Array.from(form.querySelectorAll('[data-model-binding-row]')).map(function (row) {
      const field = function (name) { return row.querySelector('[name="' + name + '"]'); };
      const channelID = Number(field('channel_id').value);
      if (!channelID) return null;
      return {
        channel_id: channelID,
        upstream_model: field('upstream_model').value.trim(),
        priority: Number(field('priority').value) || 0,
        weight: Number(field('weight').value) || 1,
		enabled: field('binding_enabled').checked,
		upstream_free: field('upstream_free').checked
      };
    }).filter(Boolean);
  }

  function bindModelForm() {
    const form = document.querySelector('[data-model-form]');
    if (!form) return;
    const bindingBody = document.querySelector('[data-model-bindings]');
	const openRouting = document.querySelector('[data-open-model-routing]');
	form.elements.billing_mode.addEventListener('change', function () { syncBillingMode(form); });
	if (openRouting) openRouting.addEventListener('click', function () {
	  api.closeDialog('model-dialog');
	  showAdminModelPage('routing', 'push');
	  if (window.ProxyLanAdminRouting) window.ProxyLanAdminRouting.load().catch(function (error) { api.toast(error.message, 'danger'); });
	});
    document.querySelector('[data-add-model-binding]').addEventListener('click', function () {
      renderModelBindings(collectModelBindings(form).concat([{ channel_id: 0, upstream_model: form.elements.model.value.trim(), priority: 0, weight: 1, enabled: true }]));
    });
    bindingBody.addEventListener('click', function (event) {
      const button = event.target.closest('[data-remove-model-binding]');
      if (!button) return;
      const row = button.closest('[data-model-binding-row]');
      if (row) row.remove();
      if (!bindingBody.querySelector('[data-model-binding-row]')) bindingBody.innerHTML = '<div class="empty">暂无绑定，模型暂时不可用。</div>';
    });
    form.elements.session_only.addEventListener('change', function () {
      if (form.elements.session_only.checked) form.elements.session_enabled.checked = true;
    });
    form.elements.session_enabled.addEventListener('change', function () {
      if (!form.elements.session_enabled.checked) form.elements.session_only.checked = false;
    });
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      const values = Object.fromEntries(new FormData(form).entries());
      const data = {
        model: values.model.trim(),
        display_name: values.display_name.trim(),
		billing_mode: values.billing_mode === 'request' ? 'request' : 'token',
        input_price: Number(values.input_price) || 0,
		output_price: Number(values.output_price) || 0,
		request_price: Number(values.request_price) || 0,
		compaction_price: Number(values.compaction_price) || 0,
		remote_compaction_enabled: form.elements.remote_compaction_enabled.checked,
         cache_read_price: Number(values.cache_read_price),
         cache_write_price: Number(values.cache_write_price),
         price_multiplier: Number(values.price_multiplier),
		 long_context_threshold: Number(values.long_context_threshold) || 0,
		 long_context_input_multiplier: Number(values.long_context_input_multiplier) || 1,
		 long_context_output_multiplier: Number(values.long_context_output_multiplier) || 1,
		 quota_group: values.quota_group.trim(),
         enabled: form.elements.enabled.checked,
        subscription_only: form.elements.subscription_only.checked,
        prepaid_compatible: form.elements.prepaid_compatible.checked,
		free_model: form.elements.free_model.checked,
		data_train: form.elements.data_train.checked,
		session_enabled: form.elements.session_enabled.checked,
		session_only: form.elements.session_only.checked
      };
	  if (!editingModelRoutingManaged) data.bindings = collectModelBindings(form);
      const editing = Boolean(values.id);
      try {
        await api.request(editing ? '/api/admin/models/' + values.id : '/api/admin/models', { method: editing ? 'PUT' : 'POST', body: data });
        api.closeDialog('model-dialog');
        api.toast(editing ? '模型已更新' : '模型已创建');
        await loadModels();
      } catch (error) { api.toast(error.message, 'danger'); }
    });
  }

  // rows 供订阅块渲染套餐可用模型的勾选列表；setChannels 由 loadChannels 回填。
  function rows() {
    return modelRows;
  }

  function setChannels(list) {
    channelRows = Array.isArray(list) ? list : [];
    if (window.ProxyLanAdminRouting) {
      window.ProxyLanAdminRouting.setChannels(channelRows);
    }
  }

	function bindAdminModels() {
	  const visibility = document.querySelector('[data-hide-disabled-models]');
	  if (visibility) {
		visibility.checked = hideDisabledModels;
		visibility.addEventListener('change', function () {
		  hideDisabledModels = api.saveAdminListPreference('hideDisabledModels', visibility.checked);
		  loadModels(false).catch(function (error) { api.toast(error.message, 'danger'); });
		});
	  }
	  bindModelForm();
	  bindAdminModelSubpages();
	}

  window.ProxyLanAdminModels = { load: loadModels, bind: bindAdminModels, resetForm: resetModelForm, rows: rows, setChannels: setChannels, loadHealth: loadAdminModelHealth };
}());
