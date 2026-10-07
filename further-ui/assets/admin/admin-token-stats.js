(function () {
  'use strict';
  const api = window.FurtherUI;
  const state = { page: 1, pageSize: 25, days: 7, keyword: '', total: 0, totalPages: 0 };
  let rows = [];
  let selected = new Set();
  let version = 0;
  let card;

  function mount() {
    if (card && card.isConnected) return card;
    const panel = document.querySelector('[data-panel="users"]');
    if (!panel) return null;
    card = document.createElement('section');
    card.className = 'md-card token-usage-card';
    card.dataset.tokenUsageCard = '';
    card.innerHTML = '<div class="card-head"><div><h2 class="md-card-title">多用户令牌统计</h2><p class="card-hint"></p></div><button class="md-btn md-btn-outlined" type="button" data-token-usage-copy disabled><span class="material-symbol" aria-hidden="true">content_copy</span><span data-token-usage-copy-label>复制本页统计</span></button></div>' +
      '<form class="list-filter token-usage-filter" data-token-usage-filter><div class="md-textfield"><input class="md-input" name="keyword" type="search" placeholder=" "><label class="md-input-label">用户、令牌、模型或请求 ID</label></div><div class="md-textfield"><select class="md-input" name="days"><option value="1">今天</option><option value="7" selected>最近 7 天</option><option value="30">最近 30 天</option></select><label class="md-input-label">统计周期 · UTC+8</label></div><div class="md-textfield"><select class="md-input" name="page_size"><option value="25">每页 25 个令牌</option><option value="50">每页 50 个令牌</option><option value="100">每页 100 个令牌</option></select><label class="md-input-label">每页数量</label></div><div class="list-filter-actions"><button class="md-btn md-btn-text" type="reset">重置</button><button class="md-btn md-btn-filled" type="submit">统计</button></div></form>' +
      '<div class="token-usage-summary settings-meta-grid" data-token-usage-summary></div><p class="field-hint">汇总覆盖全部匹配日志。调用次数包含失败尝试；Token 和费用仅统计成功消费日志。</p><div class="usage-table-scroll"><table class="usage-table token-usage-table"><thead><tr><th><input type="checkbox" data-token-usage-select-all aria-label="选择本页全部令牌"></th><th>用户 / 令牌</th><th>调用 / 失败</th><th>输入 Token</th><th>输出 Token</th><th>费用</th><th>最近调用</th></tr></thead><tbody data-token-usage-rows><tr><td class="empty" colspan="7">打开用户页后加载统计。</td></tr></tbody></table></div>' +
      '<div class="pagination-bar"><span class="pagination-summary" data-token-usage-page></span><button class="md-btn md-btn-outlined md-btn-compact" type="button" data-token-usage-prev disabled>上一页</button><button class="md-btn md-btn-outlined md-btn-compact" type="button" data-token-usage-next disabled>下一页</button></div><textarea class="generated-code hidden" data-token-usage-copy-output readonly rows="6" aria-label="令牌统计复制内容"></textarea>';
    const filter = panel.querySelector('[data-admin-user-filter]');
    panel.insertBefore(card, filter ? filter.closest('.md-card') : null);
    const form = card.querySelector('[data-token-usage-filter]');
    form.addEventListener('submit', function (event) {
      event.preventDefault();
      state.keyword = form.elements.keyword.value.trim();
      state.days = Number(form.elements.days.value);
      state.pageSize = Number(form.elements.page_size.value);
      state.page = 1;
      load();
    });
    form.addEventListener('reset', function () {
      state.keyword = ''; state.days = 7; state.pageSize = 25; state.page = 1;
      Promise.resolve().then(load);
    });
    card.querySelector('[data-token-usage-prev]').addEventListener('click', function () { if (state.page > 1) { state.page--; load(); } });
    card.querySelector('[data-token-usage-next]').addEventListener('click', function () { if (state.page < state.totalPages) { state.page++; load(); } });
    card.querySelector('[data-token-usage-select-all]').addEventListener('change', function (event) {
      selected = event.target.checked ? new Set(rows.map(function (_, index) { return index; })) : new Set();
      card.querySelectorAll('[data-token-usage-select]').forEach(function (input) { input.checked = selected.has(Number(input.dataset.tokenUsageSelect)); });
      syncSelection();
    });
    card.querySelector('[data-token-usage-copy]').addEventListener('click', copy);
    return card;
  }

  function syncSelection() {
    const all = card.querySelector('[data-token-usage-select-all]');
    all.checked = rows.length > 0 && selected.size === rows.length;
    all.indeterminate = selected.size > 0 && selected.size < rows.length;
    all.disabled = rows.length === 0;
    card.querySelector('[data-token-usage-copy]').disabled = rows.length === 0 || card.getAttribute('aria-busy') === 'true';
    card.querySelector('[data-token-usage-copy-label]').textContent = selected.size ? '复制已选 ' + selected.size + ' 个令牌统计' : '复制本页统计';
  }

  function render(payload) {
    rows = Array.isArray(payload.items) ? payload.items : [];
    selected.clear();
    state.total = Number(payload.total) || 0;
    state.totalPages = Number(payload.total_pages) || 0;
    const summary = payload.summary || {};
    const metrics = [['统计用户', api.number(summary.active_users)], ['调用次数', api.number(summary.requests)], ['输入 + 输出 Token', api.number(Number(summary.prompt_tokens || 0) + Number(summary.completion_tokens || 0))], ['总费用', api.moneyFromQuota(summary.quota, 4)]];
    card.querySelector('[data-token-usage-summary]').innerHTML = metrics.map(function (metric) { return '<div class="token-usage-stat"><span>' + metric[0] + '</span><b>' + metric[1] + '</b></div>'; }).join('');
    card.querySelector('[data-token-usage-rows]').innerHTML = rows.length ? rows.map(function (row, index) {
      const identity = row.username || '用户 #' + Number(row.user_id);
      const tokenName = row.token_name || (Number(row.token_id) > 0 ? '令牌 #' + Number(row.token_id) : '未记录令牌');
      return '<tr><td><input type="checkbox" data-token-usage-select="' + index + '" aria-label="选择 ' + api.escapeHTML(identity + ' / ' + tokenName) + '"></td><td><b>' + api.escapeHTML(identity) + '</b><div class="list-item-sub">#' + Number(row.user_id) + ' · ' + api.escapeHTML(tokenName) + ' · Key #' + Number(row.token_id) + '</div></td><td>' + api.number(row.requests) + '<div class="list-item-sub">失败 ' + api.number(row.errors) + '</div></td><td>' + api.number(row.prompt_tokens) + '</td><td>' + api.number(row.completion_tokens) + '</td><td>' + api.moneyFromQuota(row.quota, 4) + '</td><td>' + api.date(row.last_used_at) + '</td></tr>';
    }).join('') : '<tr><td class="empty" colspan="7">当前筛选下没有调用记录。</td></tr>';
    card.querySelectorAll('[data-token-usage-select]').forEach(function (input) { input.addEventListener('change', function () { const index = Number(input.dataset.tokenUsageSelect); if (input.checked) selected.add(index); else selected.delete(index); syncSelection(); }); });
    card.querySelector('[data-token-usage-page]').textContent = '第 ' + state.page + ' / ' + Math.max(1, state.totalPages) + ' 页 · ' + api.number(state.total) + ' 个用户令牌组合';
    card.querySelector('[data-token-usage-prev]').disabled = state.page <= 1;
    card.querySelector('[data-token-usage-next]').disabled = state.page >= state.totalPages;
  }

  async function load() {
    if (!mount()) return;
    const requestVersion = ++version;
    card.setAttribute('aria-busy', 'true');
    syncSelection();
    card.querySelector('[data-token-usage-copy-output]').classList.add('hidden');
    const button = card.querySelector('[type="submit"]');
    api.showBusy(button, true);
    try {
      const params = new URLSearchParams({ page: String(state.page), page_size: String(state.pageSize), days: String(state.days) });
      if (state.keyword) params.set('keyword', state.keyword);
      const payload = await api.request('/api/admin/token-usage?' + params);
      if (requestVersion !== version) return;
      render(payload || {});
    } catch (error) {
      if (requestVersion !== version) return;
      rows = []; selected.clear();
      card.querySelector('[data-token-usage-summary]').textContent = '';
      card.querySelector('[data-token-usage-rows]').innerHTML = '<tr><td class="empty" colspan="7">' + api.escapeHTML(error.message || '读取令牌统计失败') + ' <button class="md-btn md-btn-text" type="button" data-token-usage-retry>重试</button></td></tr>';
      card.querySelector('[data-token-usage-retry]').addEventListener('click', load);
      card.querySelector('[data-token-usage-page]').textContent = '统计加载失败';
      card.querySelector('[data-token-usage-prev]').disabled = true;
      card.querySelector('[data-token-usage-next]').disabled = true;
    } finally {
      if (requestVersion === version) { card.removeAttribute('aria-busy'); api.showBusy(button, false); syncSelection(); }
    }
  }

  function textCell(value) {
    const text = String(value == null ? '' : value).replace(/[\t\r\n]+/g, ' ');
    return /^[\s]*[=+\-@]/.test(text) ? "'" + text : text;
  }

  async function copy() {
    const copied = selected.size ? rows.filter(function (_, index) { return selected.has(index); }) : rows;
    if (!copied.length || card.getAttribute('aria-busy') === 'true') return;
    const period = state.days === 1 ? '今天' : '最近 ' + state.days + ' 天';
    const report = ['多用户令牌统计 · ' + period + ' · UTC+8 · 第 ' + state.page + ' 页', '用户 ID\t用户名\t令牌 ID\t令牌名称\t调用次数\t失败次数\t输入 Token\t输出 Token\t费用\t最近调用'].concat(copied.map(function (row) {
      return [row.user_id, row.username, row.token_id, row.token_name, row.requests, row.errors, row.prompt_tokens, row.completion_tokens, api.moneyFromQuota(row.quota, 4), api.date(row.last_used_at)].map(textCell).join('\t');
    })).join('\n');
    const output = card.querySelector('[data-token-usage-copy-output]');
    output.value = report;
    try {
      await navigator.clipboard.writeText(report);
      api.toast('已复制 ' + copied.length + ' 个令牌的统计');
    } catch (_) {
      output.classList.remove('hidden'); output.focus(); output.select();
      api.toast('浏览器未允许剪贴板访问，可从展开的文本框手动复制', 'danger');
    }
  }

  window.FurtherUIAdminTokenStats = { bind: mount, load: load };
}());
