(function () {
  'use strict';
  const api = window.FurtherUI;

  let modelRows = [];
	let modelCatalogLoaded = false;
	let modelLoading = null;
	let hideDisabledModels = api.adminListPreference('hideDisabledModels');
  // 模型要绑定渠道，但渠道列表由 admin.js 的 loadChannels 拉取。
  // 与其让本模块重复请求一次渠道接口，由 admin.js 加载完直接推进来。
  let channelRows = [];
	let healthLoaded = false;
	let healthLoading = null;

	function adminModelPageFromHash() {
	  const parts = location.hash.slice(1).split('/');
	  if (parts[0] === 'models') {
	    if (parts[1] === 'health') return 'health';
	    if (parts[1] === 'pricing' || parts[1] === 'routing') history.replaceState(null, '', '#models');
	  }
	  return 'catalog';
	}

	function showAdminModelPage(name, historyMode) {
	  const page = name === 'health' ? 'health' : 'catalog';
	  document.querySelectorAll('[data-admin-model-page]').forEach(function (element) {
        const visible = element.dataset.adminModelPage === page;
        const apply = function (shown) { element.classList.toggle('hidden', !shown); };
        if (api.transitionVisibility) api.transitionVisibility(element, visible, apply, { skipMotion: document.documentElement.dataset.shellReady !== 'true' || element.closest('[data-panel]')?.classList.contains('rf-entering') });
        else apply(visible);
      });
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
	    }
		api.setText('[data-page-title]', title);
		api.setText('[data-page-subtitle]', subtitle);
	  }
	  const target = page === 'health' ? '#models/health' : '#models';
	  if (historyMode === 'push' && location.hash !== target) history.pushState(null, '', target);
	  if (historyMode === 'replace' && location.hash !== target) history.replaceState(null, '', target);
	  if (page === 'catalog' && panel?.classList.contains('active') && !modelCatalogLoaded) loadModels().catch(function (error) { api.toast(error.message, 'danger'); });
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
	  if (healthLoading) return healthLoading;
	  healthLoading = (async function () {
	    try {
	      const payload = await api.request('/api/admin/model-health');
	      renderAdminModelHealth(payload);
	      healthLoaded = true;
	    } catch (error) {
	      ['[data-admin-model-health-rows]', '[data-admin-channel-health-rows]'].forEach(function (selector) {
	        const body = document.querySelector(selector);
	        if (body) body.innerHTML = '<tr><td class="empty" colspan="5">加载失败：' + api.escapeHTML(error.message) + '。请点击刷新重试。</td></tr>';
	      });
	      throw error;
	    } finally { healthLoading = null; }
	  }());
	  return healthLoading;
	}

	function bindAdminModelSubpages() {
	  document.querySelectorAll('[data-admin-model-tab]').forEach(function (button) {
		button.addEventListener('click', function () {
		  const page = button.dataset.adminModelTab;
		  showAdminModelPage(page, 'push');
		  if (page === 'health' && !healthLoaded) loadAdminModelHealth().catch(function (error) { api.toast(error.message, 'danger'); });
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
	  }
	  window.addEventListener('popstate', syncFromHistory);
	  window.addEventListener('hashchange', syncFromHistory);
	  window.addEventListener('further:model-pricing-saved', function () { modelCatalogLoaded = false; });
	  syncFromHistory();
	}

  function modelRow(item) {
	const rowKey = api.escapeHTML(item.row_key || String(item.id));
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
    const prepaid = item.prepaid_compatible ? '<span class="tag success">额度卡可用</span>' : '<span class="tag muted">额度卡禁用</span>';
	const freeModel = item.free_model ? '<span class="tag success">免费模型</span>' : '';
	const dataTrainModel = item.data_train ? '<span class="tag error">data-train 模型</span>' : '';
	const sessionModel = item.session_only ? '<span class="tag error">仅会话</span>' : (item.session_enabled ? '<span class="tag success">支持认领</span>' : '');
	const upstreamFree = Number(item.upstream_free_binding_count || 0) > 0 ? '<span class="tag">上游免费 ×' + Number(item.upstream_free_binding_count) + '</span>' : '';
	const requestBilled = item.billing_mode === 'request';
	const billingTag = requestBilled ? '<span class="tag">按次计费</span>' : '<span class="tag muted">按量计费</span>';
	const remoteCompaction = item.remote_compaction_enabled ? '<span class="tag success">支持远程压缩</span>' : '';
	const advancedRouting = '';
	const longContext = !requestBilled && Number(item.long_context_threshold || 0) > 0 ? '<span class="tag">长上下文 &gt;' + api.number(item.long_context_threshold) + ' · 入/缓存 ×' + Number(item.long_context_input_multiplier || 1).toFixed(2) + ' · 出 ×' + Number(item.long_context_output_multiplier || 1).toFixed(2) + '</span>' : '';
    const bindingCount = Array.isArray(item.bindings) ? item.bindings.length : 0;
    const bindChannel = '<button type="button" class="md-btn md-btn-text md-btn-compact" data-root-only data-bind-model-channel="' + rowKey + '"><span class="material-symbol" aria-hidden="true">add_link</span>渠道路由与别名</button>';
	const bindingChannels = Array.isArray(item.bindings) ? item.bindings.map(function (binding) {
	  const channel = channelRows.find(function (entry) { return Number(entry.id) === Number(binding.channel_id); });
	  return '<span class="model-channel-chip' + (binding.enabled === false ? ' is-disabled' : '') + '"><span class="material-symbol" aria-hidden="true">hub</span>' + api.escapeHTML(binding.channel_name || channel && channel.name || ('渠道 #' + binding.channel_id)) + (binding.upstream_model && binding.upstream_model !== item.model ? ' → ' + api.escapeHTML(binding.upstream_model) : '') + '</span>';
	}).join('') : '';
	const routeSummary = item.advanced_routing_enabled ? '渠道路由 · ' + Number(item.available_channel_count || 0) + ' / ' + Number(item.enabled_binding_count || 0) + ' 可用' : Number(item.available_channel_count || 0) + ' / ' + Number(item.enabled_binding_count || 0) + ' 个启用渠道可用';
	const statusReason = item.availability_reason ? ' · ' + api.escapeHTML(item.availability_reason) : '';
    const price = function (value) { return value == null || Number(value) < 0 ? '跟随输入' : '$' + Number(value || 0).toFixed(6); };
	const normalPricing = item.pricing_unavailable ? '<div><b>' + (item.pricing_editable === false && !item.pricing_read_only ? '特殊计费' : '待设置价格') + '</b><small>' + api.escapeHTML(item.unsupported_reason || '请使用一键设置或编辑模型价格') + '</small></div>' : requestBilled ? '<div><b>$' + Number(item.request_price || 0).toFixed(6) + '</b>模型按次</div>' : '<div><b>$' + Number(item.input_price || 0).toFixed(6) + '</b>输入 / 1M</div><div><b>' + price(item.cache_read_price) + '</b>缓存读 / 1M</div><div><b>' + price(item.cache_write_price) + '</b>缓存写 / 1M</div><div><b>$' + Number(item.output_price || 0).toFixed(6) + '</b>输出 / 1M</div>';
	const toggle = item.catalog_visibility_only ? '' : '<button type="button" class="md-icon-button" title="切换状态" data-toggle-model="' + rowKey + '"><span class="material-symbol" aria-hidden="true">refresh</span></button>';
	const remove = Number(item.metadata_id || item.id) > 0 ? '<button type="button" class="md-icon-button danger" title="删除" data-delete-model="' + Number(item.metadata_id || item.id) + '"><span class="material-symbol" aria-hidden="true">delete</span></button>' : '';
	return '<div class="list-item admin-model-item"><span class="material-symbol" aria-hidden="true">tune</span><div class="list-item-body"><div class="list-item-title">' + api.escapeHTML(item.display_name || item.model) + '</div><div class="list-item-sub"><code>' + api.escapeHTML(item.model) + '</code> · ' + bindingCount + ' 个渠道绑定 · ' + routeSummary + statusReason + '</div><div class="model-channel-bindings">' + (bindingChannels || '<span class="field-hint">尚未关联渠道，请从渠道拉取模型或配置渠道路由。</span>') + '</div><div class="list-item-sub">' + '通用额度' + ' · 模型倍率 ×' + Number(item.price_multiplier || 1).toFixed(4) + '</div></div><div class="list-metrics">' + normalPricing + '<div><b>$' + Number(item.compaction_price || 0).toFixed(6) + '</b>远程压缩</div></div><div class="list-actions">' + freeModel + dataTrainModel + sessionModel + upstreamFree + longContext + remoteCompaction + advancedRouting + billingTag + access + prepaid + status + visibility + bindChannel + '<button type="button" class="md-icon-button" title="编辑" data-edit-model="' + rowKey + '"><span class="material-symbol" aria-hidden="true">edit</span></button>' + toggle + remove + '</div></div>';
  }

  function syncBillingMode(form) {
    const requestBilled = form.elements.billing_mode.value === 'request';
    form.querySelectorAll('[data-request-pricing]').forEach(function (section) { section.classList.toggle('hidden', !requestBilled); });
    form.querySelectorAll('[data-token-pricing]').forEach(function (section) { section.classList.toggle('hidden', requestBilled || section.dataset.modelControlUnavailable === 'true'); });
    syncLockedModelOutput(form);
  }

  function syncLockedModelOutput(form) {
    const ratio = Number(form.dataset.lockedCompletionRatio);
    const locked = form.dataset.lockedCompletionRatio !== undefined && Number.isFinite(ratio);
    const output = form.elements.output_price;
    output.readOnly = locked && form.elements.billing_mode.value === 'token';
    let hint = form.querySelector('[data-model-output-ratio-hint]');
    if (!hint) {
      hint = document.createElement('p'); hint.className = 'field-hint'; hint.dataset.modelOutputRatioHint = '';
      output.closest('.md-textfield').append(hint);
    }
    hint.hidden = !output.readOnly;
    hint.textContent = output.readOnly ? '当前模型输出倍率固定为 ' + ratio + '，输出价格随输入价格自动计算。' : '';
    if (output.readOnly) {
      const input = Number(form.elements.input_price.value);
      if (Number.isFinite(input) && input >= 0) output.value = String(input * ratio);
    }
  }

  function setModelPricingEditable(form, editable, reason) {
    form.dataset.pricingEditable = String(editable);
    ['billing_mode', 'input_price', 'output_price', 'request_price',
      'cache_read_price', 'cache_write_price', 'free_model'].forEach(function (name) {
      if (form.elements[name]) form.elements[name].disabled = !editable;
    });
    const submit = form.querySelector('[type="submit"]');
    if (submit) submit.disabled = !editable;
    let notice = form.querySelector('[data-model-pricing-notice]');
    if (!notice) {
      notice = document.createElement('p');
      notice.className = 'field-hint';
      notice.dataset.modelPricingNotice = '';
      form.querySelector('.form-actions').before(notice);
    }
    notice.hidden = editable;
    notice.textContent = editable ? '' : (reason || '当前模型使用特殊计费，保留现有配置') + '。';
    const fill = form.querySelector('[data-model-fill-pricing]');
    if (fill) fill.disabled = !editable;
  }

  function showModelPricingRules(form, item) {
    let details = form.querySelector('[data-model-pricing-readonly]');
    if (!item || !item.pricing_details || Number(document.body.dataset.gatewayRole || 100) < 100) {
      if (details) details.hidden = true;
      return;
    }
    if (!details) {
      details = document.createElement('details');
      details.className = 'md-card'; details.dataset.modelPricingReadonly = '';
      details.innerHTML = '<summary>查看当前计费规则（只读）</summary><p class="field-hint">现有规则继续生效，一键设置与普通价格编辑不会覆盖这些规则。</p><pre style="white-space:pre-wrap;overflow-wrap:anywhere"></pre>';
      form.querySelector('.form-actions').before(details);
    }
    details.querySelector('pre').textContent = JSON.stringify(item.pricing_details, null, 2);
    details.hidden = false; details.open = false;
  }

  function resetModelForm() {
    const form = document.querySelector('[data-model-form]');
    if (!form) return;
    form.reset();
    delete form.dataset.dialogBusy;
    delete form.dataset.lockedCompletionRatio;
    showModelPricingRules(form, null);
    form.elements.id.value = '';
    form.dataset.editModelName = '';
    form.elements.model.readOnly = false;
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
    const canEditPricing = !document.body.dataset.gatewayRole || Number(document.body.dataset.gatewayRole) >= 100;
    setModelPricingEditable(form, canEditPricing, '仅超级管理员可编辑模型计费');
    if (window.FurtherUIAdminRouting) window.FurtherUIAdminRouting.resetModel('');
	syncBillingMode(form);
  }

  function editModel(item) {
    const form = document.querySelector('[data-model-form]');
    if (!form) return;
    form.elements.id.value = Number(item.metadata_id || item.id) > 0 ? Number(item.metadata_id || item.id) : '';
    form.dataset.editModelName = item.model || '';
    form.elements.model.readOnly = true;
    form.elements.model.value = item.model || '';
    if (item.completion_ratio_info && item.completion_ratio_info.locked) form.dataset.lockedCompletionRatio = String(item.completion_ratio_info.ratio);
    else delete form.dataset.lockedCompletionRatio;
    form.elements.display_name.value = item.display_name || item.model || '';
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
    if (window.FurtherUIAdminRouting) window.FurtherUIAdminRouting.editModel(item.model);
	setModelPricingEditable(form, item.pricing_editable !== false, item.unsupported_reason);
    showModelPricingRules(form, item);
	syncBillingMode(form);
    api.openDialog('model-dialog');
  }

  async function loadModels(refresh) {
	if (refresh !== false) {
	  if (!modelLoading) modelLoading = (async function () {
	    const result = await api.request('/api/admin/models');
	    modelRows = Array.isArray(result) ? result : [];
	    modelCatalogLoaded = true;
        if (window.FurtherUIModelImports) window.FurtherUIModelImports.setModels(modelRows);
	    if (window.FurtherUIAdminRouting) window.FurtherUIAdminRouting.setModels(modelRows);
	  }()).finally(function () { modelLoading = null; });
	  await modelLoading;
	}
	showAdminModelPage(adminModelPageFromHash());
	const displayedRows = hideDisabledModels ? modelRows.filter(function (item) { return item.enabled !== false; }) : modelRows;
    const body = document.querySelector('[data-model-rows]');
    if (!body) return;
	body.innerHTML = displayedRows.length ? displayedRows.map(modelRow).join('') : '<div class="empty">' + (modelRows.length ? '停用模型已隐藏，可关闭筛选后查看。' : '还没有模型配置。') + '</div>';
    body.querySelectorAll('[data-bind-model-channel]').forEach(function (button) {
      button.addEventListener('click', function () {
        const item = modelRows.find(function (entry) { return String(entry.row_key || entry.id) === button.dataset.bindModelChannel; });
        if (item) { editModel(item); document.querySelector('[data-model-routing-editor]').scrollIntoView?.({ block: 'start', behavior: 'smooth' }); }
      });
    });
    body.querySelectorAll('[data-edit-model]').forEach(function (button) {
      button.addEventListener('click', function () {
        const item = modelRows.find(function (model) { return String(model.row_key || model.id) === button.dataset.editModel; });
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

  async function refreshModels() {
    if (modelLoading) await modelLoading.catch(function () {});
    healthLoaded = false;
    return loadModels();
  }

  function bindModelForm() {
    const form = document.querySelector('[data-model-form]');
    if (!form) return;
    // These fields have no separate engine policy in the gateway. Keep their
    // defaults visible and let channel routing own model availability.
    ['display_name', 'enabled', 'price_multiplier', 'compaction_price',
      'remote_compaction_enabled', 'long_context_threshold',
      'long_context_input_multiplier', 'long_context_output_multiplier',
      'quota_group', 'prepaid_compatible', 'data_train', 'session_enabled',
      'session_only'].forEach(function (name) {
      const input = form.elements[name];
      if (input) {
        input.disabled = true;
        const row = input.closest('.md-textfield, .switch-line');
        if (row) { row.hidden = true; row.classList.add('hidden'); }
      }
    });
    form.querySelectorAll('.field-title').forEach(function (title) {
      if (title.textContent === '模型 Key 会话认领') title.hidden = true;
    });
    const longContext = form.elements.long_context_threshold.closest('[data-token-pricing]');
    if (longContext) { longContext.hidden = true; longContext.dataset.modelControlUnavailable = 'true'; longContext.classList.add('hidden'); }
    const legacyCompactionHint = form.querySelector(':scope > .card-hint');
    if (legacyCompactionHint) legacyCompactionHint.textContent = '价格、渠道路由与上游别名在本窗口统一管理。';
    form.elements.billing_mode.addEventListener('change', function () { syncBillingMode(form); });
    form.elements.input_price.addEventListener('input', function () { syncLockedModelOutput(form); });
    form.elements.session_only.addEventListener('change', function () {
      if (form.elements.session_only.checked) form.elements.session_enabled.checked = true;
    });
    form.elements.session_enabled.addEventListener('change', function () {
      if (!form.elements.session_enabled.checked) form.elements.session_only.checked = false;
    });
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      if (form.dataset.pricingEditable === 'false' || form.dataset.saving === 'true') return;
      syncLockedModelOutput(form);
      const values = Object.fromEntries(new FormData(form).entries());
      ['display_name', 'price_multiplier', 'compaction_price',
        'long_context_threshold', 'long_context_input_multiplier',
        'long_context_output_multiplier', 'quota_group'].forEach(function (name) {
        values[name] = form.elements[name].value;
      });
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
		 quota_group: '',
         enabled: form.elements.enabled.checked,
        subscription_only: form.elements.subscription_only.checked,
        prepaid_compatible: form.elements.prepaid_compatible.checked,
		free_model: form.elements.free_model.checked,
		data_train: form.elements.data_train.checked,
		session_enabled: form.elements.session_enabled.checked,
		session_only: form.elements.session_only.checked
      };
      if (window.FurtherUIAdminRouting && window.FurtherUIAdminRouting.hasUnsaved()) { api.toast('请先保存或取消尚未保存的渠道配置，再保存模型价格', 'danger'); return; }
      const zeroPrice = data.billing_mode === 'request' ? data.request_price === 0 : data.input_price === 0 && data.output_price === 0;
      if (zeroPrice && !data.free_model) { api.toast('请填写模型价格，或明确勾选“免费模型”。可使用一键填写参考价格。', 'danger'); return; }
      const editing = Boolean(values.id);
      const configuring = Boolean(form.dataset.editModelName);
      const button = form.querySelector('[type="submit"]');
      form.dataset.saving = 'true';
      api.showBusy(button, true);
      try {
        await api.request(editing ? '/api/admin/models/' + values.id : '/api/admin/models', { method: editing ? 'PUT' : 'POST', body: data });
        api.closeDialog('model-dialog');
        api.toast(editing || configuring ? '模型已更新' : '模型已创建');
        await loadModels();
      } catch (error) { api.toast(error.message, 'danger'); }
      finally { form.dataset.saving = 'false'; api.showBusy(button, false); }
    });
  }

  // rows 供订阅块渲染套餐可用模型的勾选列表；setChannels 由 loadChannels 回填。
  function rows() {
    return modelRows;
  }

  function setChannels(list) {
    channelRows = Array.isArray(list) ? list : [];
    if (window.FurtherUIAdminRouting) {
      window.FurtherUIAdminRouting.setChannels(channelRows);
    }
    if (window.FurtherUIModelImports) window.FurtherUIModelImports.setChannels(channelRows);
    if (modelRows.length) loadModels(false).catch(function (error) { api.toast(error.message, 'danger'); });
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
	  if (window.FurtherUIModelImports) window.FurtherUIModelImports.bind();
	}

  window.FurtherUIAdminModels = { load: loadModels, refresh: refreshModels, bind: bindAdminModels, resetForm: resetModelForm, rows: rows, setChannels: setChannels, loadHealth: loadAdminModelHealth };
}());
