(function () {
  'use strict';
  const api = window.FurtherUI;
  const colors = ['#1a73e8', '#188038', '#a142f4', '#e37400', '#007b83', '#c5221f', '#7c6f00', '#4758a6'];
  let panel;
  let snapshot = null;
  let snapshotFilters = null;
  let requestVersion = 0;

  function shanghaiDate(offsetDays) {
    return new Date(Date.now() + (8 * 60 * 60 * 1000) + offsetDays * 86400000).toISOString().slice(0, 10);
  }

  function mount() {
    if (panel && panel.isConnected) return panel;
    const area = document.querySelector('.content-area');
    const usersNav = document.querySelector('.nav-rail [data-nav="users"]');
    if (!area || !usersNav) return null;
    if (!document.querySelector('.nav-rail [data-nav="multi-user-stats"]')) {
      const nav = document.createElement('button');
      nav.type = 'button'; nav.className = 'nav-item'; nav.dataset.nav = 'multi-user-stats';
      nav.innerHTML = '<span class="material-symbol" aria-hidden="true">monitoring</span>多用户令牌统计';
      usersNav.after(nav);
    }
    panel = document.createElement('section');
    panel.className = 'view multi-user-page'; panel.dataset.panel = 'multi-user-stats';
    panel.dataset.title = '多用户令牌统计'; panel.dataset.subtitle = '对比多个用户的全部令牌、费用与模型调用';
    panel.innerHTML = '<div class="multi-user-heading"><div><span class="multi-user-eyebrow">USAGE ANALYSIS</span><h1>多用户令牌统计</h1><p>把每位用户的调用、费用与模型分布放在一起比较。</p></div><span class="multi-user-heading-icon material-symbol" aria-hidden="true">monitoring</span></div>' +
      '<form class="multi-user-form" data-multi-user-form><div class="multi-user-form-title"><span class="material-symbol" aria-hidden="true">manage_search</span><h2>查询范围</h2></div><div class="multi-user-fields"><label class="multi-user-name-field">用户名<textarea class="md-input" name="usernames" rows="2" placeholder="例如 alice, bob, charlie" required maxlength="4096"></textarea><small>用逗号分隔，最多 50 位。名字本身含逗号时用双引号包住，例如 &quot;alice,bob&quot;, carol；名字中的双引号写成两个双引号。</small></label><label>开始日期<input class="md-input" type="date" name="start_time" required></label><label>结束日期<input class="md-input" type="date" name="end_time" required><small>包含结束日期当天，按北京时间统计。</small></label></div><div class="multi-user-form-actions"><span>最长 180 天</span><button class="md-btn md-btn-filled" type="submit"><span class="material-symbol" aria-hidden="true">search</span>查询统计</button><button class="md-btn md-btn-outlined" type="button" data-multi-user-export disabled><span class="material-symbol" aria-hidden="true">download</span>导出 CSV</button></div></form>' +
      '<div class="multi-user-message hidden" data-multi-user-message role="status" aria-live="polite"></div><div data-multi-user-results></div><div class="multi-user-empty" data-multi-user-empty><span class="material-symbol" aria-hidden="true">query_stats</span><h2>暂无查询结果</h2><p>请选择用户和日期。</p></div>';
    area.append(panel);
    const form = panel.querySelector('[data-multi-user-form]');
    form.elements.start_time.value = shanghaiDate(-29);
    form.elements.end_time.value = shanghaiDate(0);
    form.addEventListener('submit', function (event) { event.preventDefault(); query(); });
    panel.querySelector('[data-multi-user-export]').addEventListener('click', exportCSV);
    if (api.initializeVisuals) api.initializeVisuals(panel);
    return panel;
  }

  function message(text, danger) {
    const element = panel.querySelector('[data-multi-user-message]');
    element.textContent = text;
    element.classList.toggle('hidden', !text);
    element.classList.toggle('danger', Boolean(danger));
  }

  function parseUsernames(input) {
    const names = []; let value = ''; let quoted = false; let closedQuote = false;
    function finish() { const name = value.trim(); if (name && names.indexOf(name) === -1) names.push(name); value = ''; closedQuote = false; }
    for (let index = 0; index < input.length; index++) {
      const character = input[index];
      if (quoted) {
        if (character === '"') {
          if (input[index + 1] === '"') { value += '"'; index++; }
          else { quoted = false; closedQuote = true; }
        } else value += character;
      } else if (character === ',' || character === '，') finish();
      else if (closedQuote) {
        if (!/\s/.test(character)) throw new Error('双引号后的用户名请用逗号分隔。');
      } else if (character === '"') {
        if (value.trim()) throw new Error('名字中的双引号请写成两个双引号，并把完整名字用双引号包住。');
        value = ''; quoted = true;
      } else value += character;
    }
    if (quoted) throw new Error('用户名的双引号尚未闭合。');
    finish();
    if (!names.length || names.length > 50) throw new Error('请输入 1 至 50 个完整用户名。');
    return names;
  }

  async function query() {
    if (!mount()) return;
    const version = ++requestVersion;
    const form = panel.querySelector('[data-multi-user-form]');
    const button = form.querySelector('[type="submit"]');
    panel.removeAttribute('aria-busy'); api.showBusy(button, false);
    snapshot = null; snapshotFilters = null;
    panel.querySelector('[data-multi-user-export]').disabled = true;
    panel.querySelector('[data-multi-user-results]').innerHTML = '';
    const empty = panel.querySelector('[data-multi-user-empty]');
    empty.classList.remove('hidden');
    empty.querySelector('h2').textContent = '查询条件需要确认';
    empty.querySelector('p').textContent = '填写完整用户名与日期范围，再查询使用情况。';
    const filters = { usernames_json: '', start_time: form.elements.start_time.value, end_time: form.elements.end_time.value };
    if (!form.elements.usernames.value.trim() || !filters.start_time || !filters.end_time) { message('请输入用户名，并选择完整日期范围。', true); return; }
    try { filters.usernames_json = JSON.stringify(parseUsernames(form.elements.usernames.value)); }
    catch (error) { message(error.message, true); return; }
    if (filters.start_time > filters.end_time || (new Date(filters.end_time) - new Date(filters.start_time)) / 86400000 > 179) { message('结束日期不能早于开始日期，统计范围最多 180 天。', true); return; }
    panel.setAttribute('aria-busy', 'true'); api.showBusy(button, true);
    empty.classList.add('hidden');
    message('正在查询所有令牌的调用记录…', false);
    try {
      const data = await api.request('/api/admin/multi-user-stats?' + new URLSearchParams(filters));
      if (version !== requestVersion) return;
      if (!data || !Array.isArray(data.items) || !Array.isArray(data.model_usage)) throw new Error('统计数据格式无效');
      snapshot = data; snapshotFilters = filters;
      render(data);
      const notes = [];
      if (data.unknown_usernames && data.unknown_usernames.length) notes.push('未找到账户：' + data.unknown_usernames.join('、'));
      if (data.no_usage_usernames && data.no_usage_usernames.length) notes.push('所选日期没有调用记录：' + data.no_usage_usernames.join('、'));
      message(notes.join('。'), false);
      panel.querySelector('[data-multi-user-export]').disabled = data.items.length === 0;
    } catch (error) {
      if (version !== requestVersion) return;
      message(error.message || '查询失败，请稍后重试。', true);
      const empty = panel.querySelector('[data-multi-user-empty]');
      empty.classList.remove('hidden');
      empty.querySelector('h2').textContent = '统计暂时未能加载';
      empty.querySelector('p').textContent = '调整查询范围或再次点击“查询统计”重试。';
    } finally {
      if (version === requestVersion) { panel.removeAttribute('aria-busy'); api.showBusy(button, false); }
    }
  }

  function metric(label, value, extra) { return '<div class="multi-user-metric"><span>' + label + '</span><b>' + value + '</b>' + (extra ? '<small>' + extra + '</small>' : '') + '</div>'; }
  function render(data) {
    const users = data.items;
    const empty = panel.querySelector('[data-multi-user-empty]');
    if (!users.length) {
      empty.classList.remove('hidden'); empty.querySelector('h2').textContent = '这个范围没有调用记录';
      empty.querySelector('p').textContent = '可检查用户名或尝试其他日期范围。';
      return;
    }
    const cards = users.map(function (user, index) {
      const color = colors[index % colors.length];
      return '<article class="multi-user-card" style="--usage-color:' + color + ';--usage-order:' + index + '"><div class="multi-user-card-heading"><div class="multi-user-avatar">' + api.escapeHTML(Array.from(user.username || '?')[0]) + '</div><div><h2>' + api.escapeHTML(user.username) + '</h2><span>全部令牌 · 用户 #' + Number(user.user_id) + '</span></div></div><div class="multi-user-card-primary">' + metric('总调用次数', api.number(user.request_count), '失败尝试 ' + api.number(user.error_count)) + metric('额度消耗', api.moneyFromQuota(user.quota, 6)) + '</div><div class="multi-user-card-secondary">' + metric('输入 Token', api.number(user.prompt_tokens)) + metric('输出 Token', api.number(user.completion_tokens)) + metric('累计请求耗时', (Number(user.request_time || 0) / 1000).toFixed(2) + ' s') + '</div></article>';
    }).join('');
    const totals = users.reduce(function (sum, user) { sum.requests += Number(user.request_count || 0); sum.quota += Number(user.quota || 0); sum.tokens += Number(user.prompt_tokens || 0) + Number(user.completion_tokens || 0); return sum; }, { requests: 0, quota: 0, tokens: 0 });
    panel.querySelector('[data-multi-user-results]').innerHTML = '<div class="multi-user-result-heading"><div><h2>查询结果</h2><p>' + api.escapeHTML(snapshotFilters.start_time + ' 至 ' + snapshotFilters.end_time) + ' · 北京时间 · ' + users.length + ' 位用户有调用记录</p></div><span>' + api.number(totals.requests) + ' 次调用 · ' + api.moneyFromQuota(totals.quota, 6) + ' · ' + api.number(totals.tokens) + ' Token</span></div><div class="multi-user-cards">' + cards + '</div><div class="multi-user-charts"><article class="multi-user-chart-card"><h2>用户额度消耗对比</h2><p>同一日期范围内，成功消费日志的实际费用。</p><div class="multi-user-bars">' + barMarkup(users) + '</div></article><article class="multi-user-chart-card"><h2>不同用户的模型调用分布</h2><p>按用户与模型汇总，包含已记录的失败尝试。</p>' + donutMarkup(data.model_usage) + '</article></div><div class="multi-user-model-card"><div><h2>模型调用明细</h2><p>调用次数包含失败尝试；Token 与费用仅统计成功消费，耗时为已记录日志的累计值。</p></div><div class="multi-user-table-scroll"><table class="multi-user-table"><thead><tr><th>用户</th><th>模型</th><th>调用次数</th><th>输入 Token</th><th>输出 Token</th><th>费用</th><th>累计耗时</th></tr></thead><tbody>' + data.model_usage.map(function (model) { return '<tr><td>' + api.escapeHTML(model.username) + '</td><td>' + api.escapeHTML(model.model_name || '未记录模型') + '</td><td>' + api.number(model.request_count) + '</td><td>' + api.number(model.prompt_tokens) + '</td><td>' + api.number(model.completion_tokens) + '</td><td>' + api.moneyFromQuota(model.quota, 6) + '</td><td>' + (Number(model.request_time || 0) / 1000).toFixed(2) + ' s</td></tr>'; }).join('') + '</tbody></table></div></div>';
  }

  function barMarkup(users) {
    const max = Math.max.apply(null, users.map(function (user) { return Math.max(0, Number(user.quota || 0)); }));
    return users.map(function (user, index) {
      const percent = max > 0 ? Math.max(0, Number(user.quota || 0)) / max * 100 : 0;
      return '<div class="multi-user-bar-row"><div><b>' + api.escapeHTML(user.username) + '</b><span>' + api.moneyFromQuota(user.quota, 6) + '</span></div><svg viewBox="0 0 100 8" preserveAspectRatio="none" role="img" aria-label="' + api.escapeHTML(user.username + ' 额度消耗 ' + api.moneyFromQuota(user.quota, 6)) + '"><rect width="100" height="8" rx="4" class="multi-user-bar-track"/><rect width="' + percent.toFixed(4) + '" height="8" rx="4" fill="' + colors[index % colors.length] + '"/></svg></div>';
    }).join('');
  }

  function donutMarkup(models) {
    const total = models.reduce(function (sum, item) { return sum + Math.max(0, Number(item.request_count || 0)); }, 0);
    if (!total) return '<div class="multi-user-chart-empty">暂无模型调用记录。</div>';
    let offset = 0;
    const segments = models.map(function (model, index) {
      const percent = Math.max(0, Number(model.request_count || 0)) / total * 100;
      const segment = '<circle cx="60" cy="60" r="44" pathLength="100" fill="none" stroke="' + colors[index % colors.length] + '" stroke-width="15" stroke-dasharray="' + percent.toFixed(6) + ' ' + (100 - percent).toFixed(6) + '" stroke-dashoffset="' + (-offset).toFixed(6) + '" transform="rotate(-90 60 60)"><title>' + api.escapeHTML(model.username + ' · ' + (model.model_name || '未记录模型') + '：' + api.number(model.request_count) + ' 次') + '</title></circle>';
      offset += percent; return segment;
    }).join('');
    return '<div class="multi-user-donut-layout"><svg viewBox="0 0 120 120" class="multi-user-donut" role="img" aria-label="模型调用分布，总计 ' + total + ' 次">' + segments + '<text x="60" y="58" text-anchor="middle" class="multi-user-donut-total">' + api.number(total) + '</text><text x="60" y="72" text-anchor="middle" class="multi-user-donut-label">总调用次数</text></svg><ul class="multi-user-legend">' + models.map(function (model, index) { return '<li><span class="multi-user-legend-dot" style="background:' + colors[index % colors.length] + '"></span><span>' + api.escapeHTML(model.username) + ' · ' + api.escapeHTML(model.model_name || '未记录模型') + '</span><b>' + api.number(model.request_count) + '</b></li>'; }).join('') + '</ul></div>';
  }

  function csvCell(value) {
    let text = String(value == null ? '' : value);
    if (/^[\s]*[=+\-@]/.test(text)) text = "'" + text;
    return '"' + text.replace(/"/g, '""') + '"';
  }

  function exportCSV() {
    if (!snapshot || !snapshotFilters || !snapshot.items.length || panel.getAttribute('aria-busy') === 'true') return;
    const lines = [['统计周期', snapshotFilters.start_time, snapshotFilters.end_time, 'Asia/Shanghai'], ['用户 ID', '用户名', '调用次数', '失败尝试', '额度(原始单位)', '费用(USD)', '输入 Token', '输出 Token', '累计耗时(ms)']];
    snapshot.items.forEach(function (user) { lines.push([user.user_id, user.username, user.request_count, user.error_count, user.quota, api.usdFromQuota(user.quota), user.prompt_tokens, user.completion_tokens, user.request_time]); });
    lines.push([], ['用户 ID', '用户名', '模型', '调用次数', '失败尝试', '额度(原始单位)', '费用(USD)', '输入 Token', '输出 Token', '累计耗时(ms)']);
    snapshot.model_usage.forEach(function (model) { lines.push([model.user_id, model.username, model.model_name, model.request_count, model.error_count, model.quota, api.usdFromQuota(model.quota), model.prompt_tokens, model.completion_tokens, model.request_time]); });
    const content = '\ufeff' + lines.map(function (line) { return line.map(csvCell).join(','); }).join('\r\n');
    const blob = new Blob([content], { type: 'text/csv;charset=utf-8' });
    const url = URL.createObjectURL(blob); const link = document.createElement('a');
    link.href = url; link.download = 'multi_user_stats_' + snapshotFilters.start_time + '_' + snapshotFilters.end_time + '.csv';
    document.body.append(link); link.click(); link.remove();
    window.setTimeout(function () { URL.revokeObjectURL(url); }, 1000);
    api.toast('统计 CSV 已导出');
  }

  document.addEventListener('further:ready', mount);
  window.FurtherUIMultiUserStats = { mount: mount, query: query, exportCSV: exportCSV };
}());
