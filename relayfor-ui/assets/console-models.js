(function () {
  'use strict';
  const api = window.ProxyLan;
	let consentBound = false;
	let sessionActionsBound = false;
	let healthLoaded = false;

  function modelPrice(value) {
    if (value == null || !Number.isFinite(Number(value))) return '动态计费 / 未配置';
    return '$' + Number(value || 0).toFixed(6);
  }

  function modelEndpointLabel(endpoint) {
    const labels = {
      '/v1/chat/completions': 'Chat Completions',
      '/v1/responses': 'Responses',
	  '/v1/responses/compact': '远程压缩 v2',
      '/v1/messages': 'Messages',
      '/v1/realtime': 'Realtime'
    };
    return labels[endpoint] || endpoint;
  }

  function modelPriceCell(label, effective, base, followsInput) {
    const baseNote = followsInput ? '跟随输入' : '基础 ' + modelPrice(base);
    return '<div>' + label + '<b>' + modelPrice(effective) + '</b><small>' + baseNote + '</small></div>';
  }

	function modelHealthPageFromHash() {
	  const parts = location.hash.slice(1).split('/');
	  return parts[0] === 'models' && parts[1] === 'health' ? 'health' : 'catalog';
	}

	function showModelPage(name, historyMode) {
	  const page = name === 'health' ? 'health' : 'catalog';
	  document.querySelectorAll('[data-console-model-page]').forEach(function (element) { element.classList.toggle('hidden', element.dataset.consoleModelPage !== page); });
	  document.querySelectorAll('[data-console-model-tab]').forEach(function (button) {
		const active = button.dataset.consoleModelTab === page;
		button.classList.toggle('active', active);
		button.setAttribute('aria-pressed', String(active));
	  });
	  const panel = document.querySelector('[data-panel="models"]');
	  if (panel && panel.classList.contains('active')) {
		api.setText('[data-page-title]', page === 'health' ? '模型健康度公示' : panel.dataset.title);
		api.setText('[data-page-subtitle]', page === 'health' ? '查看模型近期与今日调用成功率' : panel.dataset.subtitle);
	  }
	  const target = page === 'health' ? '#models/health' : '#models';
	  if (historyMode === 'push' && location.hash !== target) history.pushState(null, '', target);
	  if (historyMode === 'replace' && location.hash !== target) history.replaceState(null, '', target);
	}

	function healthRateMarkup(windowData) {
	  const requests = Number(windowData && windowData.requests || 0);
	  const successful = Number(windowData && windowData.successful || 0);
	  const rate = Number(windowData && windowData.success_rate || 0);
	  const tone = !requests ? 'empty' : rate >= 99 ? 'good' : rate >= 95 ? 'warn' : 'bad';
	  return '<span class="health-rate health-' + tone + '">' + (requests ? rate.toFixed(1) + '%' : '—') + '</span><small>' + api.number(successful) + ' / ' + api.number(requests) + ' 成功</small>';
	}

	function renderUserModelHealth(payload) {
	  const body = document.querySelector('[data-user-model-health-rows]');
	  if (!body) return;
	  const items = Array.isArray(payload && payload.items) ? payload.items : [];
	  body.innerHTML = items.length ? items.map(function (item) {
		return '<tr><td><b>' + api.escapeHTML(item.display_name || item.model) + '</b><small><code>' + api.escapeHTML(item.model) + '</code></small></td><td>' + healthRateMarkup(item.recent_60) + '</td><td>' + healthRateMarkup(item.today) + '</td><td><b>' + api.number(item.today && item.today.requests) + '</b><small>有效请求</small></td></tr>';
	  }).join('') : '<tr><td class="empty" colspan="4">当前没有可公示的模型。</td></tr>';
	  const updated = document.querySelector('[data-user-model-health-updated]');
	  if (updated) updated.textContent = api.date(payload.generated_at) + ' 更新 · ' + (payload.timezone || '服务器时区');
	}

	async function loadUserModelHealth() {
	  const payload = await api.request('/api/user/model-health');
	  renderUserModelHealth(payload);
	  healthLoaded = true;
	}

	function bindModelSubpages() {
	  document.querySelectorAll('[data-console-model-tab]').forEach(function (button) {
		button.addEventListener('click', function () {
		  const page = button.dataset.consoleModelTab;
		  showModelPage(page, 'push');
		  if (page === 'health' && !healthLoaded) loadUserModelHealth().catch(function (error) { api.toast(error.message, 'danger'); });
		});
	  });
	  const refresh = document.querySelector('[data-user-model-health-refresh]');
	  if (refresh) refresh.addEventListener('click', function () {
		api.showBusy(refresh, true);
		loadUserModelHealth().catch(function (error) { api.toast(error.message, 'danger'); }).finally(function () { api.showBusy(refresh, false); });
	  });
	  function syncFromHistory() {
		const page = modelHealthPageFromHash();
		showModelPage(page);
		if (page === 'health' && !healthLoaded) loadUserModelHealth().catch(function (error) { api.toast(error.message, 'danger'); });
	  }
	  window.addEventListener('popstate', syncFromHistory);
	  window.addEventListener('hashchange', syncFromHistory);
	  showModelPage(modelHealthPageFromHash());
	}

	function sessionAllocationLabel(session) {
	  return '#' + Number(session.channel_id || 0) + ' 渠道 · #' + Number(session.channel_key_id || 0) + ' Key';
	}

	function modelSessionRow(item) {
	  const session = item.active_session;
	  const modelName = api.escapeHTML(item.display_name || item.model);
	  if (session) {
		return '<div class="model-session-row">' +
		  '<span class="material-symbol" aria-hidden="true">key</span>' +
		  '<div class="list-item-body"><div class="list-item-title">' + modelName + ' <span class="tag success">认领中</span></div>' +
		  '<code class="model-session-allocation">' + sessionAllocationLabel(session) + '</code>' +
		  '<div class="list-item-sub">' + Number(session.requests_per_minute || 0) + ' RPM · ' + Number(session.concurrent_requests || 2) + ' 并发 · ' + api.date(session.expires_at) + ' 到期 · 正常计费</div></div>' +
		  '<div class="model-session-actions"><button type="button" class="md-btn md-btn-text md-btn-compact" data-release-model-session="' + session.id + '">释放认领</button></div></div>';
	  }
	  const reason = item.session_claimable ? '点击后自动分配一个暂时独享的渠道 Key，认领期间不会分配给其他用户。' : api.escapeHTML(item.access_reason || '当前账户暂不可认领此模型。');
	  return '<div class="model-session-row">' +
		'<span class="material-symbol" aria-hidden="true">key_off</span>' +
		'<div class="list-item-body"><div class="list-item-title">' + modelName + (item.session_only ? ' <span class="tag error">仅会话</span>' : '') + '</div><div class="list-item-sub">' + reason + '</div></div>' +
		'<div class="model-session-actions"><button type="button" class="md-btn md-btn-filled md-btn-compact" data-claim-model-session="' + api.escapeHTML(item.model) + '" data-claim-model-title="' + modelName + '"' + (item.session_claimable ? '' : ' disabled') + '>认领独享 Key</button></div></div>';
	}

	function renderModelSessionHub(items) {
	  const hub = document.querySelector('[data-model-session-hub]');
	  const body = document.querySelector('[data-model-session-rows]');
	  if (!hub || !body) return;
	  const sessionModels = (Array.isArray(items) ? items : []).filter(function (item) { return item.session_enabled; });
	  hub.classList.toggle('hidden', sessionModels.length === 0);
	  body.innerHTML = sessionModels.map(modelSessionRow).join('');
	}

  function modelPricingRow(item) {
    const base = item.base_prices || {};
    const effective = item.effective_prices || {};
    const endpoints = Array.isArray(item.supported_endpoints) ? item.supported_endpoints : [];
    const endpointTags = endpoints.map(function (endpoint) {
      return '<span class="tag">' + api.escapeHTML(modelEndpointLabel(endpoint)) + '</span>';
    }).join('');
    const accessible = item.accessible !== false;
	const requestBilled = item.billing_mode === 'request';
	const billingTag = requestBilled ? '<span class="tag">按次计费</span>' : '<span class="tag muted">按量计费</span>';
    const accessTag = accessible ? (Number(item.subscription_multiplier || 1) < 1 ? '<span class="tag success">订阅优惠</span>' : '') : '<span class="tag error">无权限访问</span>';
	const dataTrainingTag = item.data_training_consent_required ? '<span class="tag error">需授权 data-train</span>' : (item.data_training_channel_available ? '<span class="tag muted">含 data-train 渠道</span>' : '');
    const freeTag = item.free_model ? '<span class="tag success">免费</span>' : '';
	const longThreshold = Number(item.long_context_threshold || 0);
	const longEffective = item.long_context_effective_prices || {};
	const longContextNote = !requestBilled && longThreshold > 0 ? '<div class="model-endpoints"><span class="tag">长上下文 &gt; ' + api.number(longThreshold) + ' 输入 Token</span><span class="list-item-sub">整次请求：输入 ' + modelPrice(longEffective.input) + ' · 缓存读 ' + modelPrice(longEffective.cache_read) + ' · 缓存写 ' + modelPrice(longEffective.cache_write) + ' · 输出 ' + modelPrice(longEffective.output) + ' / 1M</span></div>' : '';
	const session = item.active_session;
	const sessionTag = item.session_enabled ? (item.session_only ? '<span class="tag error">仅会话</span>' : '<span class="tag">支持独享会话</span>') : '';
	const sessionStatus = session ? '<span class="tag success">已认领 ' + sessionAllocationLabel(session) + '</span>' : '';
    const benefitNote = accessible && item.subscription_plan ? ' · ' + api.escapeHTML(item.subscription_plan) + ' 权益 ×' + Number(item.subscription_multiplier || 1).toFixed(2) : (!accessible ? ' · ' + api.escapeHTML(item.access_reason || '需要订阅计划解锁') : '');
	const normalPriceCells = requestBilled ? modelPriceCell('模型按次', item.effective_request_price, item.base_request_price, false) : modelPriceCell('输入 / 1M', effective.input, base.input, false) + modelPriceCell('输出 / 1M', effective.output, base.output, false) + modelPriceCell('缓存读取 / 1M', effective.cache_read, base.cache_read, item.cache_read_follows_input) + modelPriceCell('缓存写入 / 1M', effective.cache_write, base.cache_write, item.cache_write_follows_input);
	const compactionPriceCell = endpoints.indexOf('/v1/responses/compact') >= 0 ? modelPriceCell('远程压缩 / 次', item.effective_compaction_price, item.base_compaction_price, false) : '';
    return '<div class="list-item model-item' + (accessible ? '' : ' model-locked') + '"' + (accessible ? '' : ' aria-disabled="true"') + '>' +
      '<span class="material-symbol" aria-hidden="true">tune</span>' +
      '<div class="list-item-body">' +
	  '<div class="list-item-title">' + api.escapeHTML(item.display_name || item.model) + ' ' + freeTag + billingTag + accessTag + dataTrainingTag + sessionTag + '</div>' +
      '<div class="list-item-sub"><code>' + api.escapeHTML(item.model) + '</code> · ' + (item.quota_group ? '额度组 ' + api.escapeHTML(item.quota_group) : '通用额度') + ' · 模型倍率 ×' + Number(item.price_multiplier || 1).toFixed(4) + ' · 实际倍率 ×' + Number(item.effective_multiplier || 1).toFixed(4) + benefitNote + '</div>' +
      '<div class="model-price-grid">' +
	  normalPriceCells + compactionPriceCell +
      '</div>' +
	  longContextNote + (sessionStatus ? '<div class="model-endpoints">' + sessionStatus + '</div>' : '') +
      '<div class="model-endpoints"><span class="list-item-sub">支持端点</span>' + (endpointTags || '<span class="tag muted">暂无</span>') + '</div>' +
       '</div></div>';
  }

  function modelAccessLimitsText(limits) {
    const requestsPerMinute = Number(limits && limits.requests_per_minute);
    const concurrentRequests = Number(limits && limits.concurrent_requests);
    if (!Number.isFinite(requestsPerMinute) || requestsPerMinute <= 0 || !Number.isFinite(concurrentRequests) || concurrentRequests <= 0) {
      return '当前账户访问限制暂不可用。';
    }
    const scope = limits.shared_across_api_keys ? '所有 API Key 合计' : '每个 API Key';
    let text = scope + '每分钟最多 ' + requestsPerMinute + ' 次请求，最多 ' + concurrentRequests + ' 个并发请求。';
    if (limits.realtime_uses_concurrent) text += ' Realtime 连接会占用一个并发位，连接内每次 response.create 也计入请求频率。';
    return text;
  }

	function renderDataTrainingConsent(payload) {
	  const card = document.querySelector('[data-data-training-consent-card]');
	  const input = document.querySelector('[data-data-training-consent]');
	  const status = document.querySelector('[data-data-training-consent-status]');
	  const available = Boolean(payload && payload.data_training_channels_available);
	  if (card) card.classList.toggle('hidden', !available);
	  if (input) input.checked = Boolean(payload && payload.data_training_consent);
	  if (status) status.textContent = input && input.checked ? '已授权：路由时可以使用 data-train 渠道。你可以随时撤回。' : '未授权：系统不会将任何请求发送到 data-train 渠道。';
	}

	function bindModelSessionActions() {
	  if (sessionActionsBound) return;
	  const body = document.querySelector('[data-panel="models"]');
	  if (!body) return;
	  sessionActionsBound = true;
	  body.addEventListener('click', async function (event) {
		const claim = event.target.closest('[data-claim-model-session]');
		const release = event.target.closest('[data-release-model-session]');
		const button = claim || release;
		if (!button) return;
		if (release && !(await api.confirm('释放后，这个独享 Key 会立即回到公共池。', { title:'释放模型会话', confirmText:'释放' }))) return;
		if (claim && !(await api.confirm('系统会自动分配一个渠道 Key，认领期间由你独享；请求仍按模型价格正常计费，并受会话 RPM、2 并发和到期时间限制。', { title:'认领 ' + (claim.dataset.claimModelTitle || claim.dataset.claimModelSession), confirmText:'确认认领' }))) return;
		api.showBusy(button, true);
		try {
		  if (claim) {
			const result = await api.request('/api/user/model-sessions', { method:'POST', body:{ model:claim.dataset.claimModelSession } });
			const allocation = result && result.session ? sessionAllocationLabel(result.session) : '';
			api.toast(allocation ? '已认领 ' + allocation : (result.message || '模型会话已认领'));
		  } else {
			await api.request('/api/user/model-sessions/' + release.dataset.releaseModelSession, { method:'DELETE' });
			api.toast('模型会话已释放');
		  }
		  await loadModels();
		} catch (error) {
		  api.toast(error.message, 'danger');
		} finally {
		  api.showBusy(button, false);
		}
	  });
	}

	function bindDataTrainingConsent() {
	  if (consentBound) return;
	  const input = document.querySelector('[data-data-training-consent]');
	  if (!input) return;
	  consentBound = true;
	  input.addEventListener('change', async function () {
		const allowed = input.checked;
		input.disabled = true;
		try {
		  await api.request('/api/user/data-training-consent', { method: 'PUT', body: { allowed: allowed } });
		  api.toast(allowed ? '已允许使用 data-train 渠道' : '已撤回 data-train 渠道授权');
		  await loadModels();
		} catch (error) {
		  input.checked = !allowed;
		  api.toast(error.message, 'danger');
		} finally {
		  input.disabled = false;
		}
	  });
	}

  async function loadModels() {
    const payload = await api.request('/api/user/models');
	showModelPage(modelHealthPageFromHash());
    const body = document.querySelector('[data-model-pricing-rows]');
    const context = document.querySelector('[data-model-pricing-context]');
    const accessLimits = document.querySelector('[data-model-access-limits]');
    if (context) context.textContent = '分组 ' + (payload.group || 'default') + ' · 实际倍率 ×' + Number(payload.group_multiplier || 1).toFixed(4);
    if (accessLimits) accessLimits.textContent = modelAccessLimitsText(payload.access_limits);
	renderDataTrainingConsent(payload);
    if (!body) return;
    let rows = Array.isArray(payload && payload.items) ? payload.items : [];
    if (api.preferences().hideLockedModels) rows = rows.filter(function (item) { return item.accessible !== false; });
	renderModelSessionHub(rows);
    body.innerHTML = rows.length ? rows.map(modelPricingRow).join('') : '<div class="empty">当前没有可用模型。</div>';
	if (modelHealthPageFromHash() === 'health' && !healthLoaded) await loadUserModelHealth();
  }

	bindDataTrainingConsent();
	bindModelSessionActions();
	bindModelSubpages();
  window.ProxyLanConsoleModels = { load: loadModels };
}());
