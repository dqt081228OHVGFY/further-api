(function () {
  'use strict';
  const api = window.ProxyLan;
  let overviewDays = 7;

  function renderBalance(site, balance) {
    const values = balance || {};
    ['permanent_quota', 'timed_quota', 'group_quota', 'subscription_quota', 'estimated_quota'].forEach(function (key) {
      api.setText('[data-admin-site-balance="' + site + '_' + key + '"]', api.moneyFromQuota(values[key], 2));
    });
    api.setText('[data-admin-site-balance="' + site + '_users"]', api.number(values.users));
    api.setText('[data-admin-site-balance="' + site + '_unlimited_subscriptions"]', api.number(values.unlimited_subscriptions));
  }

  async function load() {
    const data = await api.request('/api/admin/overview?days=' + overviewDays);
    Object.keys(data).forEach(function (key) {
      const element = document.querySelector('[data-admin-stat="' + key + '"]');
      if (element) element.textContent = api.number(data[key]);
    });
    const today = data.today || {};
    api.setText('[data-admin-today="quota"]', api.moneyFromQuota(today.quota, 3));
    api.setText('[data-admin-today="tokens"]', api.number(today.tokens));
    api.setText('[data-admin-today="requests"]', api.number(today.requests));
    api.setText('[data-admin-today="active_users"]', api.number(today.active_users));
    api.setText('[data-admin-today="errors"]', api.number(today.errors));
    const todayErrorRate = Number(today.error_rate || 0);
    api.setText('[data-admin-today="error_rate"]', todayErrorRate.toFixed(1) + '%');
    const errorRateCard = document.querySelector('[data-admin-error-rate-card]');
    if (errorRateCard) {
      errorRateCard.classList.toggle('error-rate-warning', todayErrorRate > 5 && todayErrorRate <= 10);
      errorRateCard.classList.toggle('error-rate-critical', todayErrorRate > 10);
    }
    const primaryError = document.querySelector('[data-admin-primary-error]');
    if (primaryError) {
      const primaryErrorLabel = String(today.primary_error || '').trim();
      primaryError.textContent = primaryErrorLabel ? '主要错误：' + primaryErrorLabel : '';
      primaryError.classList.toggle('hidden', !primaryErrorLabel || Number(today.errors || 0) <= 0);
    }
    api.setText('[data-admin-stat-money="quota"]', api.moneyFromQuota(data.quota, 2));
    const siteBalances = data.site_balances || {};
    renderBalance('public', siteBalances.public);
    renderBalance('pay', siteBalances.pay);
    renderTrends(Array.isArray(data.trend) ? data.trend : []);
    renderRankings(data.rankings || {});
  }

  function trendValue(metric, value) {
    if (metric === 'quota') return api.moneyFromQuota(value, 3);
    if (metric === 'error_rate') return Number(value || 0).toFixed(1) + '%';
    return api.number(value);
  }

  function trendDateLabel(value) {
    const parts = String(value || '').split('-');
    return parts.length === 3 ? Number(parts[1]) + '月' + Number(parts[2]) + '日' : String(value || '');
  }

  function renderTrends(points) {
    const body = document.querySelector('[data-admin-trends]');
    if (!body) return;
    if (!points.length) {
      body.innerHTML = '<div class="empty">当前没有趋势数据。</div>';
      return;
    }
    const metrics = [
      { key: 'quota', label: '扣费', icon: 'payments' },
      { key: 'tokens', label: 'Token', icon: 'data_usage' },
      { key: 'requests', label: '请求', icon: 'swap_horiz' },
      { key: 'active_users', label: '活跃用户', icon: 'person_check' },
      { key: 'error_rate', label: '错误率', icon: 'error' }
    ];
    body.innerHTML = metrics.map(function (metric) {
      const values = points.map(function (point) { return Math.max(0, Number(point[metric.key]) || 0); });
      const maximum = Math.max.apply(Math, values);
      const latest = values[values.length - 1];
      const bars = points.map(function (point, index) {
        const value = values[index];
        const height = maximum > 0 ? Math.max(4, value / maximum * 100) : 4;
        const title = trendDateLabel(point.date) + ' · ' + metric.label + ' ' + trendValue(metric.key, value);
        return '<span class="trend-bar-column" title="' + api.escapeHTML(title) + '"><span class="trend-bar-fill" style="--bar-height:' + height.toFixed(2) + '%"></span></span>';
      }).join('');
      return '<article class="trend-card" data-trend-metric="' + metric.key + '"><div class="trend-card-head"><span><span class="material-symbol" aria-hidden="true">' + metric.icon + '</span>' + metric.label + '</span><b>' + trendValue(metric.key, latest) + '</b></div><div class="trend-bars" role="img" aria-label="近 ' + overviewDays + ' 天' + metric.label + '柱状趋势">' + bars + '</div><div class="trend-range"><span>' + trendDateLabel(points[0].date) + '</span><span>' + trendDateLabel(points[points.length - 1].date) + '</span></div></article>';
    }).join('');
  }

  function rankingRows(items, renderItem, emptyMessage) {
    if (!items.length) return '<div class="ranking-empty">' + api.escapeHTML(emptyMessage) + '</div>';
    return items.map(function (item, index) {
      return '<div class="ranking-row" role="listitem"><span class="ranking-position">' + (index + 1) + '</span>' + renderItem(item) + '</div>';
    }).join('');
  }

  function renderRankings(rankings) {
    const days = Number(rankings.window_days || overviewDays) === 30 ? 30 : 7;
    api.setText('[data-admin-ranking-window]', '近 ' + days + ' 天');
    const topUsers = Array.isArray(rankings.top_users) ? rankings.top_users : [];
    const topModels = Array.isArray(rankings.top_models) ? rankings.top_models : [];
    const errorChannels = Array.isArray(rankings.top_error_channels) ? rankings.top_error_channels : [];
    const highFrequencyKeys = Array.isArray(rankings.high_frequency_keys) ? rankings.high_frequency_keys : [];

    const userBody = document.querySelector('[data-admin-ranking-list="top_users"]');
    if (userBody) userBody.innerHTML = rankingRows(topUsers, function (item) {
      return '<div class="ranking-copy"><b>' + api.escapeHTML(item.username || ('用户 #' + item.user_id)) + '</b><small>' + api.number(item.requests) + ' 次请求 · ' + api.number(item.tokens) + ' Token</small></div><div class="ranking-value"><b>' + api.moneyFromQuota(item.quota, 3) + '</b><small>扣费</small></div>';
    }, '当前周期还没有用户消费记录。');

    const modelBody = document.querySelector('[data-admin-ranking-list="top_models"]');
    if (modelBody) modelBody.innerHTML = rankingRows(topModels, function (item) {
      return '<div class="ranking-copy"><b>' + api.escapeHTML(item.model || '未知模型') + '</b><small>' + api.number(item.requests) + ' 次请求 · ' + api.number(item.tokens) + ' Token</small></div><div class="ranking-value"><b>' + api.moneyFromQuota(item.quota, 3) + '</b><small>扣费</small></div>';
    }, '当前周期还没有模型调用记录。');

    const channelBody = document.querySelector('[data-admin-ranking-list="top_error_channels"]');
    if (channelBody) channelBody.innerHTML = rankingRows(errorChannels, function (item) {
      return '<div class="ranking-copy"><b>' + api.escapeHTML(item.channel_name || ('渠道 #' + item.channel_id)) + '</b><small>' + api.number(item.requests) + ' 次请求 · 429 ' + api.number(item.rate_limited) + ' · 5xx ' + api.number(item.server_errors) + '</small></div><div class="ranking-value ranking-value-error"><b>' + api.number(item.errors) + '</b><small>' + Number(item.error_rate || 0).toFixed(1) + '% 错误率</small></div>';
    }, '当前周期没有渠道错误。');

    const keyBody = document.querySelector('[data-admin-ranking-list="high_frequency_keys"]');
    if (keyBody) keyBody.innerHTML = rankingRows(highFrequencyKeys, function (item) {
      const ratio = Number(item.frequency_ratio || 0);
      return '<div class="ranking-copy"><b>' + api.escapeHTML(item.token_name || ('Key #' + item.token_id)) + '</b><small>' + api.escapeHTML(item.username || ('用户 #' + item.user_id)) + ' · Key #' + api.number(item.token_id) + ' · 错误率 ' + Number(item.error_rate || 0).toFixed(1) + '%</small></div><div class="ranking-value ranking-value-warning"><b>' + api.number(item.requests) + '</b><small>同级平均 ' + (ratio > 0 ? ratio.toFixed(1) + '×' : '—') + '</small></div>';
    }, '未发现达到阈值的异常高频 Key。');
  }

  function bind() {
    document.querySelectorAll('[data-admin-trend-days]').forEach(function (button) {
      button.addEventListener('click', async function () {
        const days = Number(button.dataset.adminTrendDays);
        if (days === overviewDays) return;
        overviewDays = days === 30 ? 30 : 7;
        document.querySelectorAll('[data-admin-trend-days]').forEach(function (item) {
          const active = Number(item.dataset.adminTrendDays) === overviewDays;
          item.classList.toggle('active', active);
          item.setAttribute('aria-pressed', active ? 'true' : 'false');
        });
        const body = document.querySelector('[data-admin-trends]');
        if (body) body.innerHTML = '<div class="empty">加载趋势中…</div>';
        document.querySelectorAll('[data-admin-ranking-list]').forEach(function (list) { list.innerHTML = '<div class="ranking-empty">加载排行中…</div>'; });
        try { await load(); }
        catch (error) { api.toast(error.message, 'danger'); }
      });
    });
  }

  window.ProxyLanAdminOverview = { load: load, bind: bind };
}());
