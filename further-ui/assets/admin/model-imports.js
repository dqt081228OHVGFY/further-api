(function () {
  'use strict';
  const api = window.FurtherUI;
  const endpoint = '/api/further/models/pricing/models-dev';
  let channels = [];
  let preview = null;
  let fetching = false;
  let applying = false;
  let previewSequence = 0;
  let bound = false;
  let modelRows = [];
  let scopedModels = null;
  let savedChannelContext = false;
  const choices = new Map();

  function escape(value) { return api.escapeHTML(value == null ? '' : String(value)); }
  function money(value) {
    return value == null ? '保留现价' : '$' + Number(value).toLocaleString('en-US', { maximumFractionDigits: 8 });
  }
  function statusLabel(status) {
    return ({ matched: '可导入', ambiguous: '选择数据源', unmatched: '未找到价格', protected: '计费模式受保护', unbound: '未关联渠道', configured: '已有价格', reference: '参考价格' })[status] || '已跳过';
  }
  function importable(item) { return item.status === 'matched' || item.status === 'ambiguous'; }
  function selectedCandidate(item) {
    const choice = choices.get(item.model);
    return choice && choice.selected ? item.candidates[choice.index] : null;
  }
  function priceCells(current, proposed) {
    return ['input_price', 'output_price', 'cache_read_price', 'cache_write_price'].map(function (key, index) {
      const effective = proposed && proposed.effective_price;
      const value = effective && effective[key] != null ? effective[key] : proposed && proposed[key] != null ? proposed[key] : current && current[key];
      const inherited = proposed && proposed[key] == null && current && current[key] == null;
      return '<div><span>' + ['输入', '输出', '缓存读', '缓存写'][index] + '</span><small>' + (current && current[key] != null ? money(current[key]) : '未配置') + '</small><b>' + money(value) + (inherited && value != null ? '（默认倍率）' : '') + '</b></div>';
    }).join('');
  }
  function renderItem(item) {
    const candidates = Array.isArray(item.candidates) ? item.candidates : [];
    const choice = choices.get(item.model);
    const candidate = selectedCandidate(item);
    const canImport = importable(item) && candidates.length > 0;
    const reference = canImport && choice && choice.index >= 0 ? candidates[choice.index] : null;
    const selector = canImport ? '<label class="model-import-source"><span>价格数据源</span><select class="md-input" data-price-source="' + escape(item.model) + '" aria-label="' + escape(item.model) + ' 的价格数据源"><option value="">请选择供应商</option>' + candidates.map(function (source, index) {
      return '<option value="' + index + '"' + (choice && choice.index === index ? ' selected' : '') + '>' + escape(source.provider_name || source.provider_id) + ' · ' + escape(source.model_id) + ' · 输入 ' + money(source.input_price) + ' / 输出 ' + money(source.output_price) + '</option>';
    }).join('') + '</select></label>' : '';
    return '<article class="model-import-item' + (canImport ? '' : ' is-skipped') + '" data-price-model="' + escape(item.model) + '">' +
      '<div class="model-import-item-head"><label class="model-import-check"><input type="checkbox" data-price-select="' + escape(item.model) + '"' + (candidate ? ' checked' : '') + (canImport && choice && choice.index >= 0 ? '' : ' disabled') + '><span><b>' + escape(item.model) + '</b><small>' + Number(item.channel_count || 0) + ' 个渠道关联</small></span></label><span class="tag' + (candidate ? ' success' : ' muted') + '">' + statusLabel(item.status) + '</span></div>' +
      (item.reason ? '<p class="model-import-reason">' + escape(item.reason) + '</p>' : '') + selector +
      (reference ? '<div class="model-import-price-grid" aria-label="每百万 Token 美元价格，原价与导入价">' + priceCells(item.current, reference) + '</div>' : '') + '</article>';
  }
  function selectedItems() {
    return preview ? preview.items.filter(function (item) { return importable(item) && selectedCandidate(item); }) : [];
  }
  function syncApplyButton() {
    const button = document.querySelector('[data-price-import-apply]');
    if (!button) return;
    const count = selectedItems().length;
    if (!applying) button.textContent = count ? '一键设置 ' + count + ' 个模型价格' : '请选择可导入的模型';
    button.disabled = applying || fetching || !count;
  }
  function renderPreview() {
    const body = document.querySelector('[data-price-import-body]');
    if (!body || !preview) return;
    const ready = preview.items.filter(importable).length;
    const missing = preview.items.filter(function (item) { return item.status === 'unmatched'; }).length;
    const protectedCount = preview.items.filter(function (item) { return item.status === 'protected'; }).length;
    const retained = preview.items.filter(function (item) { return ['unbound', 'configured'].includes(item.status); }).length;
    body.innerHTML = '<div class="model-import-summary"><div><span>价格匹配</span><b>' + ready + '</b></div><div><span>缺少价格</span><b>' + missing + '</b></div><div><span>保护计费</span><b>' + protectedCount + '</b></div><div><span>保留配置</span><b>' + retained + '</b></div></div>' +
      '<p class="model-import-readout">美元 / 1M Token · 每项显示原价和参考价。已有定价默认保留；未匹配模型需手动定价，零价来源需手动勾选。预览有效期 10 分钟。</p>' +
      (preview.items.length ? '<div class="model-import-items">' + preview.items.map(renderItem).join('') + '</div>' : '<div class="model-import-empty"><span class="material-symbol" aria-hidden="true">hub</span><b>还没有可匹配的模型</b><p>先从渠道拉取模型，再读取价格。</p></div>');
    syncApplyButton();
  }
  async function fetchPreview() {
    if (fetching || applying) return;
    const body = document.querySelector('[data-price-import-body]');
    const retry = document.querySelector('[data-price-import-refresh]');
    const sequence = ++previewSequence;
    fetching = true;
    document.querySelector('[data-price-missing-only]').disabled = true;
    preview = null;
    choices.clear();
    document.querySelector('[data-price-import-save-error]').hidden = true;
    api.showBusy(retry, true);
    syncApplyButton();
    if (body) body.innerHTML = '<div class="model-import-loading" role="status"><span class="model-import-loading-dot"></span><b>正在读取 models.dev 价格</b><p>匹配渠道模型并检查现有计费方式…</p></div>';
    try {
      const missingOnly = document.querySelector('[data-price-missing-only]').checked;
      const result = await api.request(endpoint + (missingOnly ? '?missing_only=true' : ''));
      if (sequence !== previewSequence) return;
      if (!result || !result.snapshot_id || !Array.isArray(result.items)) throw new Error('价格数据格式无效，请重新读取');
      preview = Object.assign({}, result, { items: scopedModels ? result.items.filter(function (item) { return scopedModels.has(item.model); }) : result.items });
      preview.items.forEach(function (item) {
        item.candidates = Array.isArray(item.candidates) ? item.candidates : [];
        const recommended = item.candidates.map(function (candidate, index) { return candidate.recommended ? index : -1; }).filter(function (index) { return index >= 0; });
        const index = !importable(item) ? -1 : recommended.length === 1 ? recommended[0] : item.candidates.length === 1 ? 0 : -1;
        choices.set(item.model, { index: index, selected: index >= 0 && (Number(item.candidates[index].input_price) > 0 || Number(item.candidates[index].output_price) > 0) });
      });
      renderPreview();
    } catch (error) {
      if (sequence !== previewSequence) return;
      if (body) body.innerHTML = '<div class="model-import-error" role="alert"><span class="material-symbol" aria-hidden="true">error</span><b>价格读取失败</b><p>' + escape(error.message) + '</p><button type="button" class="md-btn md-btn-outlined" data-price-import-retry>重试读取</button></div>';
      api.toast((savedChannelContext ? '渠道已保存，价格读取失败：' : '价格读取失败：') + error.message, 'danger');
    } finally {
      if (sequence === previewSequence) { fetching = false; document.querySelector('[data-price-missing-only]').disabled = false; api.showBusy(retry, false); syncApplyButton(); }
    }
  }
  async function applyPrices() {
    if (applying || fetching || !preview) return;
    const items = selectedItems();
    if (!items.length) return;
    const dialog = document.getElementById('model-price-import-dialog');
    const button = document.querySelector('[data-price-import-apply]');
    applying = true;
    dialog.dataset.dialogBusy = 'true';
    dialog.querySelectorAll('button,input,select').forEach(function (element) { element.disabled = true; });
    api.showBusy(button, true);
    try {
      const result = await api.request(endpoint, { method: 'POST', body: { snapshot_id: preview.snapshot_id, selections: items.map(function (item) {
        const source = selectedCandidate(item);
        return { model: item.model, provider_id: source.provider_id, model_id: source.model_id };
      }) } });
      preview = null;
      choices.clear();
      delete dialog.dataset.dialogBusy;
      api.closeDialog(dialog.id);
      api.toast('已保存 ' + Number(result.updated_count || 0) + ' 个模型价格' + (result.skipped_count ? '，' + Number(result.skipped_count) + ' 项已跳过' : ''));
      try { await window.FurtherUIAdminModels.refresh(); }
      catch (error) { api.toast('价格已保存，模型列表刷新失败：' + error.message, 'danger'); }
    } catch (error) {
      const notice = document.querySelector('[data-price-import-save-error]');
      const stale = Number(error.status || error.response && error.response.status) === 409;
      if (stale) {
        preview = null; choices.clear();
        document.querySelector('[data-price-import-body]').innerHTML = '<div class="model-import-error" role="alert"><span class="material-symbol" aria-hidden="true">refresh</span><b>需要更新预览</b><p>模型配置已变更或本次预览已过期，旧价格选择已取消。</p><button type="button" class="md-btn md-btn-outlined" data-price-import-retry>重新读取价格</button></div>';
      }
      notice.textContent = stale ? '预览已过期或模型配置已更新。请重新读取价格后导入。' : '导入未完成：' + error.message + '。可重试，或重新读取最新价格。';
      notice.hidden = false;
      api.toast(error.message, 'danger');
    } finally {
      applying = false;
      delete dialog.dataset.dialogBusy;
      dialog.querySelectorAll('button,input,select').forEach(function (element) { element.disabled = false; });
      api.showBusy(button, false);
      if (preview) renderPreview();
      syncApplyButton();
    }
  }
  function trapDialogKeyboard(event, dialog) {
    if (event.key === 'Escape') { event.preventDefault(); api.closeDialog(dialog.id); return; }
    if (event.key !== 'Tab') return;
    const targets = Array.from(dialog.querySelectorAll('button:not(:disabled),input:not(:disabled),select:not(:disabled),a[href]')).filter(function (element) { return element.getClientRects().length > 0; });
    const first = targets[0];
    const last = targets[targets.length - 1];
    if (!first) { event.preventDefault(); return; }
    if (event.shiftKey && (document.activeElement === first || !dialog.contains(document.activeElement))) { event.preventDefault(); last.focus(); }
    else if (!event.shiftKey && (document.activeElement === last || !dialog.contains(document.activeElement))) { event.preventDefault(); first.focus(); }
  }
  function openPriceImport(options) {
    if (fetching || applying) return;
    if (Number(document.body.dataset.gatewayRole || 100) < 100) { api.toast('仅超级管理员可导入模型价格', 'danger'); return; }
    const dialog = document.getElementById('model-price-import-dialog');
    if (!dialog) return;
    scopedModels = options && Array.isArray(options.models) ? new Set(options.models) : null;
    savedChannelContext = Boolean(options && options.savedChannel);
    document.querySelector('[data-price-missing-only]').checked = true;
    document.querySelector('[data-price-import-save-error]').hidden = true;
    api.openDialog(dialog.id);
    fetchPreview();
  }
  function setChannels(rows) {
    channels = Array.isArray(rows) ? rows : [];
    const count = document.querySelector('[data-model-channel-count]');
    if (count) count.textContent = channels.length + ' 个渠道';
    if (window.FurtherUIAdminChannelModels) window.FurtherUIAdminChannelModels.setChannels(channels);
  }
  function bind() {
    if (bound) return;
    const catalog = document.querySelector('[data-admin-model-page="catalog"]');
    if (!catalog) return;
    bound = true;
    const actions = catalog.querySelector(':scope > .card-head > .list-actions');
    const intro = document.createElement('section');
    intro.className = 'md-card model-channel-bridge';
    intro.innerHTML = '<div><span class="model-import-eyebrow">渠道 → 模型 → 计费</span><h2>模型与价格一站配置</h2><p>从渠道读取上游模型，选择后关联到 further-api。拉取模型后使用“一键设置价格”补齐定价，也可在新建模型时一键填写参考价格。</p><span class="tag" data-model-channel-count>正在加载渠道</span> <span class="tag" data-model-pricing-count>价格统一管理</span></div><button type="button" class="md-btn md-btn-tonal" data-model-pull-upstream><span class="material-symbol" aria-hidden="true">download</span>从渠道拉取模型</button>';
    const outdatedIntro = catalog.querySelector(':scope > .md-card');
    if (outdatedIntro && !outdatedIntro.querySelector('[data-model-rows]')) outdatedIntro.replaceWith(intro);
    else catalog.querySelector(':scope > .card-head').after(intro);
    if (actions) {
      const button = document.createElement('button');
      button.type = 'button'; button.className = 'md-btn md-btn-outlined'; button.dataset.modelPriceImport = '';
      button.innerHTML = '<span class="material-symbol" aria-hidden="true">payments</span>一键设置价格';
      actions.insertBefore(button, actions.querySelector('[data-open-dialog]'));
      button.addEventListener('click', openPriceImport);
    }
    if (Number(document.body.dataset.gatewayRole || 100) < 100) {
      [actions && actions.querySelector('[data-model-price-import]')].filter(Boolean).forEach(function (button) {
        button.disabled = true; button.title = '仅超级管理员可使用';
      });
      const permissionNotice = document.createElement('p');
      permissionNotice.className = 'field-hint'; permissionNotice.textContent = '仅超级管理员可导入模型价格。';
      intro.querySelector('div').append(permissionNotice);
    }
    intro.querySelector('[data-model-pull-upstream]').addEventListener('click', function () {
      if (window.FurtherUIAdminChannelModels) window.FurtherUIAdminChannelModels.open({});
      else api.toast('渠道模型读取功能尚未就绪，请重新载入', 'danger');
    });
    const dialog = document.createElement('div');
    dialog.id = 'model-price-import-dialog'; dialog.className = 'dialog-backdrop'; dialog.setAttribute('aria-hidden', 'true');
    dialog.innerHTML = '<div class="md-dialog model-price-import-dialog" aria-labelledby="model-price-import-title"><header class="model-import-head"><div><span class="model-import-eyebrow">外部价格数据源 · models.dev</span><h2 id="model-price-import-title">一键设置模型价格</h2></div><button type="button" class="md-icon-button" data-price-import-close aria-label="关闭价格导入"><span class="material-symbol" aria-hidden="true">close</span></button></header><p class="model-import-intro">默认只补齐缺失的按量价格，已有价格、免费模型与特殊计费保持现有配置。参考价格来自 models.dev，核对后再保存。</p><label class="model-import-intro switch-line"><input type="checkbox" data-price-missing-only checked><span>只补齐未定价模型（关闭后可选择更新已有价格）</span></label><div data-price-import-body></div><p class="model-import-save-error" data-price-import-save-error role="alert" hidden></p><footer class="model-import-footer"><button type="button" class="md-btn md-btn-outlined" data-price-import-refresh><span class="material-symbol" aria-hidden="true">refresh</span>重新读取</button><button type="button" class="md-btn md-btn-filled" data-price-import-apply disabled>请选择可导入的模型</button></footer></div>';
    document.body.append(dialog);
    dialog.addEventListener('click', function (event) {
      if (event.target === dialog || event.target.closest('[data-price-import-close]')) api.closeDialog(dialog.id);
      if (event.target.closest('[data-price-import-retry]')) fetchPreview();
    });
    dialog.addEventListener('keydown', function (event) { trapDialogKeyboard(event, dialog); });
    dialog.querySelector('[data-price-import-refresh]').addEventListener('click', fetchPreview);
    dialog.querySelector('[data-price-missing-only]').addEventListener('change', fetchPreview);
    dialog.querySelector('[data-price-import-apply]').addEventListener('click', applyPrices);
    dialog.addEventListener('change', function (event) {
      if (!preview || applying || fetching) return;
      const source = event.target.closest('[data-price-source]');
      const checked = event.target.closest('[data-price-select]');
      if (source) {
        const index = source.value === '' ? -1 : Number(source.value);
        choices.set(source.dataset.priceSource, { index: index, selected: index >= 0 });
        const item = preview.items.find(function (entry) { return entry.model === source.dataset.priceSource; });
        const row = source.closest('[data-price-model]');
        const temporary = document.createElement('div');
        temporary.innerHTML = renderItem(item);
        const next = temporary.firstElementChild;
        const surface = dialog.querySelector('.md-dialog');
        const scrollTop = surface.scrollTop;
        row.replaceWith(next);
        surface.scrollTop = scrollTop;
        next.querySelector('[data-price-source]').focus({ preventScroll: true });
        const prices = next.querySelector('.model-import-price-grid');
        if (prices && typeof prices.animate === 'function') {
          const animation = prices.animate([{ opacity: .7, transform: 'translateY(4px)' }, { opacity: 1, transform: 'none' }], { duration: 180, easing: 'cubic-bezier(.2,.8,.2,1)' });
          animation.finished.then(function () { animation.cancel(); }).catch(function () {});
        }
        syncApplyButton();
      } else if (checked) {
        const choice = choices.get(checked.dataset.priceSelect);
        if (choice) choice.selected = checked.checked;
        syncApplyButton();
      }
    });
    bindSetupEntrypoints();
    if (api.initializeVisuals) api.initializeVisuals(document);
  }
  function setModels(rows) {
    modelRows = Array.isArray(rows) ? rows : [];
    const missing = modelRows.filter(function (item) { return item.pricing_unavailable && item.pricing_editable !== false; }).length;
    document.querySelectorAll('[data-model-pricing-count]').forEach(function (element) {
      element.textContent = missing ? missing + ' 个模型待定价' : '模型价格已就绪';
    });
  }
  async function fillModelPricing(form, button) {
    const name = form.elements.model.value.trim();
    if (!name) { api.toast('请先输入模型名称', 'danger'); return; }
    if (form.dataset.pricingEditable === 'false' || button.disabled || Number(document.body.dataset.gatewayRole || 100) < 100) return;
    const signature = function () { return JSON.stringify(['input_price', 'output_price', 'cache_read_price', 'cache_write_price', 'billing_mode', 'id'].map(function (key) { return form.elements[key].value; }).concat([form.elements.free_model.checked, form.dataset.pricingEditable])); };
    const before = signature();
    api.showBusy(button, true);
    try {
      const result = await api.request(endpoint + '?model=' + encodeURIComponent(name));
      if (form.elements.model.value.trim() !== name || before !== signature()) {
        api.toast('表单已修改，本次参考价格未填入，请重新读取', 'danger'); return;
      }
      const item = result && Array.isArray(result.items) && result.items.find(function (entry) { return entry.model === name; });
      const sources = item && Array.isArray(item.candidates) ? item.candidates : [];
      const recommended = sources.filter(function (source) { return source.recommended; });
      const source = recommended.length === 1 ? recommended[0] : sources.length === 1 ? sources[0] : null;
      if (!source) { api.toast(item && item.reason || '未找到唯一参考价格，请手动填写，或在一键设置中选择价格来源', 'danger'); return; }
      if (item.completion_ratio_info && item.completion_ratio_info.locked) form.dataset.lockedCompletionRatio = String(item.completion_ratio_info.ratio);
      else delete form.dataset.lockedCompletionRatio;
      form.elements.billing_mode.value = 'token';
      ['input_price', 'output_price', 'cache_read_price', 'cache_write_price'].forEach(function (key) {
        if (source[key] != null) form.elements[key].value = source[key];
      });
      if (Number(source.input_price) > 0 || Number(source.output_price) > 0) form.elements.free_model.checked = false;
      form.elements.billing_mode.dispatchEvent(new Event('change', { bubbles: true }));
      api.toast('已填写 ' + (source.provider_name || source.provider_id) + ' 参考价格，请核对后保存' + (Number(source.input_price) === 0 && Number(source.output_price) === 0 ? '；零价格需勾选免费模型' : ''));
    } catch (error) { api.toast('参考价格读取失败：' + error.message + '。仍可手动填写。', 'danger'); }
    finally { api.showBusy(button, false); button.disabled = form.dataset.pricingEditable === 'false' || Number(document.body.dataset.gatewayRole || 100) < 100; }
  }
  function afterChannelSave(models, refreshFailed) {
    const selected = new Set(String(models || '').split(',').map(function (name) { return name.trim(); }).filter(Boolean));
    const missing = modelRows.filter(function (item) { return selected.has(item.model) && item.pricing_unavailable && item.pricing_editable !== false; });
    if ((missing.length || refreshFailed && selected.size) && Number(document.body.dataset.gatewayRole || 100) >= 100) openPriceImport({ models: refreshFailed ? Array.from(selected) : missing.map(function (item) { return item.model; }), savedChannel: true });
  }
  function bindSetupEntrypoints() {
    const root = Number(document.body.dataset.gatewayRole || 100) >= 100;
    const channelHead = document.querySelector('[data-panel="channels"] .card-head .list-actions');
    if (channelHead) {
      const button = document.createElement('button'); button.type = 'button'; button.className = 'md-btn md-btn-outlined'; button.dataset.channelPriceSetup = '';
      button.innerHTML = '<span class="material-symbol" aria-hidden="true">payments</span>一键设置价格'; button.disabled = !root;
      button.addEventListener('click', openPriceImport); channelHead.append(button);
    }
    const form = document.querySelector('[data-model-form]');
    if (form) {
      const button = document.createElement('button'); button.type = 'button'; button.className = 'md-btn md-btn-outlined'; button.dataset.modelFillPricing = ''; button.disabled = !root;
      button.innerHTML = '<span class="material-symbol" aria-hidden="true">auto_fix_high</span>一键填写参考价格';
      form.elements.billing_mode.closest('.md-textfield').after(button);
      button.addEventListener('click', function () { fillModelPricing(form, button); });
    }
    const channelForm = document.querySelector('[data-channel-form]');
    if (root && channelForm) {
      const option = document.createElement('label'); option.className = 'switch-line';
      option.innerHTML = '<input type="checkbox" data-channel-setup-pricing checked><span>保存后检查并一键设置缺失价格</span>';
      channelForm.querySelector('.form-actions').before(option);
    }
  }
  window.FurtherUIModelImports = { bind: bind, setChannels: setChannels, setModels: setModels, openPricing: openPriceImport, afterChannelSave: afterChannelSave };
}());
