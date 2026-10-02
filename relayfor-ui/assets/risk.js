(function () {
  'use strict';
  const signals = [['regular_timing','定时规律'],['repeated_input','输入重复'],['multi_model','多模型'],['post_402','402后续'],['non_stream','非流式'],['night_activity','深夜'],['post_channel_unavailable','渠道不可用'],['blocked_liveness','测活被拦截'],['multi_ip','多IP']];
  const levels = { low:'正常', medium:'关注', high:'高风险', critical:'严重' };
  const statuses = { unreviewed:'待处理', watching:'观察中', confirmed:'已确认', dismissed:'已排除' };
  const evidenceLabels = { requests:'请求总数', distinct_models:'模型数量', distinct_ips:'IP数量', repeated_inputs:'重复输入', regular_intervals:'规律间隔', post_402_requests:'402后续请求', non_stream_requests:'非流式请求', night_requests:'深夜请求', post_channel_unavailable_requests:'渠道不可用后续请求', blocked_liveness_requests:'被拦截测活请求' };
  function score(value) { return Math.max(0, Math.min(100, Math.round(Number(value) || 0))); }
  function mount(panel) {
    const api = window.ProxyLan;
    const escape = api.escapeHTML;
    let page = 1, total = 0, sequence = 0, initialized = false;
    panel.className = 'view gateway-risk-panel';
    panel.innerHTML = '<div class="risk-tabs" role="tablist" aria-label="风控视图"><button type="button" class="md-btn md-btn-text" role="tab" aria-selected="false" data-risk-tab="warnings">风险预警</button><button type="button" class="md-btn md-btn-text active" role="tab" aria-selected="true" data-risk-tab="accounts">反测活</button></div><div data-risk-warnings hidden><h2>风险预警</h2></div><div data-risk-accounts><h2>反测活账户识别</h2><p class="field-hint">基于请求行为特征识别疑似测活、滥用和分发账户。分数用于辅助复核。</p><div class="md-card risk-list-card"><div class="card-head"><h3 class="md-card-title">疑似测活账户列表</h3><button type="button" class="md-icon-button" data-risk-refresh aria-label="刷新"><span class="material-symbol" aria-hidden="true">refresh</span></button></div><form class="risk-filters"><input class="md-input" name="username" placeholder="搜索用户名" aria-label="搜索用户名"><button class="md-btn md-btn-filled" type="submit">搜索</button><label>等级筛选<select class="md-input" name="level"><option value="">全部等级</option>' + Object.keys(levels).map(function (key) { return '<option value="' + key + '">' + levels[key] + '</option>'; }).join('') + '</select></label><label>状态筛选<select class="md-input" name="status"><option value="">全部状态</option>' + Object.keys(statuses).map(function (key) { return '<option value="' + key + '"' + (key === 'unreviewed' ? ' selected' : '') + '>' + statuses[key] + '</option>'; }).join('') + '</select></label><label>采样窗口<select class="md-input" name="window"><option value="7d">最近7天</option><option value="3d">最近3天</option></select></label></form><p data-risk-message role="status" aria-live="polite"></p><div class="risk-table-scroll"><table class="risk-table"><thead><tr><th>ID</th><th>用户名</th><th>综合分</th><th>等级</th>' + signals.map(function (entry) { return '<th>' + entry[1] + '</th>'; }).join('') + '<th>最近请求</th><th>状态</th><th>操作</th></tr></thead><tbody data-risk-rows></tbody></table></div><div class="risk-pagination"><span data-risk-total></span><button type="button" class="md-btn md-btn-text" data-risk-prev>上一页</button><span data-risk-page></span><button type="button" class="md-btn md-btn-text" data-risk-next>下一页</button></div></div></div>';
    // Both tabs share the real account list and review actions.
    const listCard = panel.querySelector('.risk-list-card');
    listCard.querySelector('.md-card-title').textContent = '风险账户列表';
    panel.append(listCard);
    const form = panel.querySelector('form');
    const message = panel.querySelector('[data-risk-message]');
    const rows = panel.querySelector('[data-risk-rows]');
    const dialog = document.createElement('div');
    dialog.id = 'gateway-risk-detail'; dialog.className = 'dialog-backdrop';
    dialog.innerHTML = '<div class="md-dialog risk-detail-dialog" role="dialog" aria-modal="true" aria-labelledby="risk-detail-title" tabindex="-1"><h2 id="risk-detail-title" class="md-dialog-title">风险详情</h2><div class="md-dialog-content" data-risk-detail></div><div class="md-dialog-actions"><button type="button" class="md-btn md-btn-text" data-risk-close>关闭</button></div></div>';
    document.body.append(dialog);
    dialog.querySelector('[data-risk-close]').addEventListener('click', function () { api.closeDialog(dialog.id); });
    dialog.addEventListener('click', function (event) { if (event.target === dialog) api.closeDialog(dialog.id); });
    function badge(value) { return '<span class="risk-score">' + score(value) + '</span>'; }
    function date(value) { if (!value) return '—'; const parsed = new Date(typeof value === 'number' ? value * 1000 : value); return Number.isNaN(parsed.getTime()) ? '—' : parsed.toLocaleString('zh-CN', { timeZone:'Asia/Shanghai', hour12:false }); }
    function updatePager() { panel.querySelector('[data-risk-total]').textContent = '共 ' + total + ' 个账户'; panel.querySelector('[data-risk-page]').textContent = '第 ' + page + ' / ' + Math.max(1, Math.ceil(total / 25)) + ' 页'; panel.querySelector('[data-risk-prev]').disabled = page <= 1; panel.querySelector('[data-risk-next]').disabled = page * 25 >= total; }
    async function load() {
      const version = ++sequence;
      const params = new URLSearchParams({ window:form.elements.window.value, page:String(page), page_size:'25', username:form.elements.username.value.trim(), level:form.elements.level.value, status:form.elements.status.value });
      message.textContent = '正在加载请求行为评分…'; rows.setAttribute('aria-busy','true');
      try {
        const data = await api.request('/api/risk/users?' + params.toString());
        if (version !== sequence) return;
        const items = Array.isArray(data.items) ? data.items : []; total = Number(data.total) || 0;
        if (!items.length && page > 1 && (page - 1) * 25 >= total) { page = Math.max(1, Math.ceil(total / 25)); return load(); }
        rows.innerHTML = items.map(function (item) {
          const id = Number(item.user_id); const level = levels[item.level] ? item.level : 'low';
          return '<tr><td>' + id + '</td><td>' + escape(item.username || '用户 #' + id) + '</td><td>' + badge(item.score) + '</td><td><span class="risk-level risk-level-' + level + '">' + levels[level] + '</span></td>' + signals.map(function (entry) { return '<td>' + badge((item.signals || {})[entry[0]]) + '</td>'; }).join('') + '<td>' + escape(date(item.last_request_at)) + '</td><td>' + escape(statuses[item.status] || '待处理') + '</td><td class="risk-actions">' + [['detail','info','查看详情'],['watching','visibility','标记观察'],['confirmed','check','确认风险'],['dismissed','close','排除风险']].map(function (action) { return '<button type="button" class="md-icon-button" data-risk-action="' + action[0] + '" data-user-id="' + id + '" title="' + action[2] + '" aria-label="' + action[2] + '"><span class="material-symbol" aria-hidden="true">' + action[1] + '</span></button>'; }).join('') + '</td></tr>';
        }).join('');
        message.textContent = items.length ? '' : '当前筛选条件下暂无账户记录。'; updatePager();
      } catch (error) { if (version === sequence) { rows.replaceChildren(); message.textContent = '加载失败：' + error.message + '，可点击刷新重试。'; } }
      finally { if (version === sequence) rows.removeAttribute('aria-busy'); }
    }
    form.addEventListener('submit', function (event) { event.preventDefault(); page = 1; load(); });
    form.querySelectorAll('select').forEach(function (select) { select.addEventListener('change', function () { page = 1; load(); }); });
    panel.querySelector('[data-risk-refresh]').addEventListener('click', load);
    panel.querySelector('[data-risk-prev]').addEventListener('click', function () { if (page > 1) { page--; load(); } });
    panel.querySelector('[data-risk-next]').addEventListener('click', function () { if (page * 25 < total) { page++; load(); } });
    panel.querySelectorAll('[data-risk-tab]').forEach(function (button) { button.addEventListener('click', function () { panel.querySelectorAll('[data-risk-tab]').forEach(function (tab) { tab.classList.toggle('active', tab === button); tab.setAttribute('aria-selected', String(tab === button)); }); panel.querySelector('[data-risk-warnings]').hidden = button.dataset.riskTab !== 'warnings'; panel.querySelector('[data-risk-accounts]').hidden = button.dataset.riskTab !== 'accounts'; }); });
    rows.addEventListener('click', async function (event) {
      const button = event.target.closest('[data-risk-action]'); if (!button) return;
      const id = Number(button.dataset.userId); if (!Number.isSafeInteger(id) || id < 1) return;
      api.showBusy(button, true);
      try {
        if (button.dataset.riskAction === 'detail') {
          const detail = dialog.querySelector('[data-risk-detail]'); detail.textContent = '正在加载…'; api.openDialog(dialog.id);
          const item = await api.request('/api/risk/users/' + id + '?window=' + form.elements.window.value);
          dialog.querySelector('#risk-detail-title').textContent = '风险详情 · ' + (item.username || id);
          detail.innerHTML = '<p>综合评分 ' + score(item.score) + ' · 请求 ' + (Number(item.request_count) || 0) + ' 次 · IP ' + (Number(item.ip_count) || 0) + ' 个</p><div class="risk-detail-signals">' + signals.map(function (entry) { const value = score((item.signals || {})[entry[0]]); return '<div><span>' + entry[1] + '</span><b>' + value + '</b><progress max="100" value="' + value + '" aria-label="' + entry[1] + '"></progress></div>'; }).join('') + '</div><h3>聚合证据</h3><dl class="risk-evidence">' + Object.entries(item.evidence || {}).map(function (entry) { return '<div><dt>' + escape(evidenceLabels[entry[0]] || entry[0]) + '</dt><dd>' + escape(entry[1] == null ? '—' : String(entry[1])) + '</dd></div>'; }).join('') + '</dl><p class="field-hint">仅展示聚合统计，不展示请求原文或原始IP。标记结果不会自动封禁用户。</p>';
        } else {
          await api.request('/api/risk/users/' + id + '/status', { method:'PATCH', body:{ status:button.dataset.riskAction } });
          api.toast('已标记为' + statuses[button.dataset.riskAction]); await load();
        }
      } catch (error) { if (button.dataset.riskAction === 'detail') dialog.querySelector('[data-risk-detail]').textContent = '加载失败：' + error.message; api.toast(error.message, 'danger'); }
      finally { api.showBusy(button, false); }
    });
    window.addEventListener('proxylan:page', function (event) { if (event.detail && event.detail.name === 'risk' && !initialized) { initialized = true; load(); } });
  }
  window.ProxyLanRisk = { mount:mount };
}());
