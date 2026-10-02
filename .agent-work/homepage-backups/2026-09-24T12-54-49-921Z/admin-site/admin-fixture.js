// Admin Fixture - synthetic management API for the local rebuild
// Source contract: admin/index.html, admin.js, admin-overview.js,
// admin-settings.js, admin-models.js, admin-routing.js,
// disabled-channels.js and channel-keys.js from admin-mirror.
//
// This fixture deliberately never forwards /api/admin requests. It provides
// deterministic, obviously synthetic data so the public management shell can
// be inspected without an authenticated administrator or real secrets.
(function () {
  'use strict';

  const STORAGE_KEY = 'relayfor_admin_fixture_state';
  const TOKEN_KEY = 'topublic_proxy_token';

  function isoDaysAgo(days) {
    const value = new Date(Date.now() - days * 86400000);
    return value.toISOString();
  }

  function dateKey(daysAgo) {
    return isoDaysAgo(daysAgo).slice(0, 10);
  }

  function defaults() {
    return {
      self: { id: 9001, username: 'fixture_admin', role: 100, group: 'default', status: 1, created_at: '2025-01-01T00:00:00.000Z' },
      channels: [
        { id: 1, name: 'Fixture OpenAI', type: 1, base_url: 'https://synthetic.invalid/v1', upstream_format: 'openai_chat', upstream_formats: ['openai_chat'], priority: 10, weight: 100, groups: 'default,pay', models: 'gpt-5.6-sol,claude-opus-5', model_mapping: '', quota_total: -1, quota_remaining: -1, requests_per_minute: 600, concurrent_requests: 24, status: 1, key_count: 2, data_train: false, remote_compaction: true, failure_count: 0, last_probe: { success: true, latency_ms: 82 }, last_error_at: null, last_probe_at: isoDaysAgo(0) },
        { id: 2, name: 'Fixture Anthropic', type: 2, base_url: 'https://synthetic.invalid/anthropic', upstream_format: 'anthropic', upstream_formats: ['anthropic'], priority: 20, weight: 60, groups: 'default', models: 'claude-opus-5', model_mapping: '', quota_total: 50000000, quota_remaining: 41800000, requests_per_minute: 300, concurrent_requests: 12, status: 1, key_count: 1, data_train: false, remote_compaction: false, failure_count: 1, last_probe: { success: true, latency_ms: 134 }, last_error_at: isoDaysAgo(1), last_probe_at: isoDaysAgo(0) }
      ],
      models: [
        { id: 1, model: 'gpt-5.6-sol', display_name: 'GPT 5.6 Sol', input_price: 2.5, output_price: 10, cache_read_price: 0.25, cache_write_price: 3, price_multiplier: 1, quota_group: '', enabled: true, subscription_only: false, prepaid_compatible: true, free_model: false, data_train: false, session_enabled: true, session_only: false, bindings: [{ channel_id: 1, upstream_model: 'gpt-5.6-sol', priority: 0, weight: 1, enabled: true, upstream_free: false }] },
        { id: 2, model: 'claude-opus-5', display_name: 'Claude Opus 5', input_price: 15, output_price: 75, cache_read_price: 1.5, cache_write_price: 18.75, price_multiplier: 1.05, quota_group: '', enabled: true, subscription_only: false, prepaid_compatible: true, free_model: false, data_train: false, session_enabled: false, session_only: false, bindings: [{ channel_id: 2, upstream_model: 'claude-opus-5', priority: 0, weight: 1, enabled: true, upstream_free: false }] },
        { id: 3, model: 'fixture-free', display_name: 'Fixture Free', input_price: 0, output_price: 0, cache_read_price: 0, cache_write_price: 0, price_multiplier: 1, quota_group: '', enabled: true, subscription_only: false, prepaid_compatible: false, free_model: true, data_train: false, session_enabled: false, session_only: false, bindings: [] }
        ,{ id: 4, model: 'gpt-6-astra', display_name: 'GPT 6 Astra【不降智】【禁止逆向】', input_price: 8, output_price: 32, cache_read_price: 0.8, cache_write_price: 9.6, price_multiplier: 1, quota_group: '', enabled: true, subscription_only: true, prepaid_compatible: false, free_model: false, data_train: false, session_enabled: false, session_only: false, bindings: [{ channel_id: 1, upstream_model: 'gpt-6-astra', priority: 0, weight: 1, enabled: true, upstream_free: false }] }
      ],
      users: [
        { id: 10001, username: 'fixture_user', group: 'default', role: 1, status: 1, quota: 1250000, used_quota: 342000, created_at: '2025-04-02T10:00:00.000Z', token_count: 2 },
        { id: 10002, username: 'demo_operator', group: 'pay', role: 50, status: 1, quota: 8400000, used_quota: 5200000, created_at: '2025-05-16T08:00:00.000Z', token_count: 4 },
        { id: 10003, username: 'audit_viewer', group: 'default', role: 25, status: 2, quota: 200000, used_quota: 80000, created_at: '2025-06-11T09:00:00.000Z', token_count: 1 }
      ],
      // The public catalogue mirrors the eight plans visible in the captured
      // RelayFor console.  `collapsed` controls the "更多套餐" dialog.
      subscription_catalog_version: 2,
      subscription_plans: [
        { id: 1, code: '基础计划', name: 'RelayFor Go', description: '基础使用计划\n适合日常调用与轻量开发，提供基础折扣、标准访问上限和专属模型权益。', duration_days: 30, quota: 10000000, unlimited_quota: false, status: 1, display_order: 1, discount_multiplier: 0.95, discount_savings_limit: 0, reset_interval_days: 0, requests_per_minute: 15, concurrent_requests: 2, model_rules: [{ model_id: 1, multiplier: 0 }, { model_id: 4, multiplier: 0 }], collapsed: false },
        { id: 2, code: '升级计划', name: 'RelayFor Plus', description: '进阶使用计划\n适合更频繁的 API 调用与开发工作流，提供更低折扣倍率、更高访问上限和专属模型权益。', duration_days: 30, quota: 50000000, unlimited_quota: false, status: 1, display_order: 2, discount_multiplier: 0.91, discount_savings_limit: 0, reset_interval_days: 0, requests_per_minute: 25, concurrent_requests: 4, model_rules: [{ model_id: 1, multiplier: 0.9 }], collapsed: false },
        { id: 3, code: '专业计划', name: 'RelayFor Pro', description: '专业使用计划\n面向持续开发、Agent 与较高并发场景，提供更优价格、更高吞吐和专业级访问权益。', duration_days: 30, quota: 100000000, unlimited_quota: false, status: 1, display_order: 3, discount_multiplier: 0.88, discount_savings_limit: 0, reset_interval_days: 0, requests_per_minute: 35, concurrent_requests: 6, model_rules: [{ model_id: 1, multiplier: 0.85 }], collapsed: false },
        { id: 4, code: '每日计划', name: 'RelayFor EveryDay', description: '每天享受一定的0.01倍率&少量赠额\n最高节省23.3刀\n持续10天', duration_days: 10, quota: 115000, unlimited_quota: false, status: 1, display_order: 4, discount_multiplier: 0.01, discount_savings_limit: 500000, reset_interval_days: 2, requests_per_minute: 15, concurrent_requests: 2, model_rules: [], collapsed: true },
        { id: 5, code: '折扣计划', name: 'RelayFor Discount', description: '短期高强度使用计划\n1 天有效 · 15 RPM / 2 并发\n公共模型全局 ×0.50', duration_days: 1, quota: 50000000, unlimited_quota: false, status: 1, display_order: 5, discount_multiplier: 0.5, discount_savings_limit: 125000000, reset_interval_days: 0, requests_per_minute: 15, concurrent_requests: 2, model_rules: [], collapsed: true },
        { id: 6, code: '专业计划 2x', name: 'RelayFor Pro 2x', description: '专业计划 2x\n为已有 RelayFor Pro 的用户提供额外外消费承诺，并进一步提升折扣与请求上限。', duration_days: 30, quota: 100000000, unlimited_quota: false, status: 1, display_order: 6, discount_multiplier: 0.87, discount_savings_limit: 0, reset_interval_days: 0, requests_per_minute: 40, concurrent_requests: 6, upgrade_from_plan_id: 3, model_rules: [{ model_id: 1, multiplier: 0.85 }], collapsed: true },
        { id: 7, code: '终极计划', name: 'RelayFor Ultra', description: '高负载使用计划\n面向持续高消费、多 Agent 与高并发工作流，提供大额额度、更低折扣倍率和更高访问上限。', duration_days: 30, quota: 500000000, unlimited_quota: false, status: 1, display_order: 7, discount_multiplier: 0.85, discount_savings_limit: 0, reset_interval_days: 0, requests_per_minute: 65, concurrent_requests: 12, model_rules: [{ model_id: 1, multiplier: 0.83 }], collapsed: true },
        { id: 8, code: '终极计划 5x', name: 'RelayFor Ultra 5x', description: '高负载计划 5x\n为已有 RelayFor Ultra 的大规模使用场景提供更高消费承诺、更优折扣与更高并发能力。', duration_days: 30, quota: 2000000000, unlimited_quota: false, status: 1, display_order: 8, discount_multiplier: 0.83, discount_savings_limit: 0, reset_interval_days: 0, requests_per_minute: 70, concurrent_requests: 18, upgrade_from_plan_id: 7, model_rules: [{ model_id: 1, multiplier: 0.81 }], collapsed: true }
      ],
      benefits: [],
      promotions: [],
      logs: [],
      traces: [],
      redemptions: [],
      registration_codes: [],
      prepaid_cards: [],
      audit: [],
      routing_rules: [{ id: 1, request_model: 'gpt-5.6-sol', strategy: 'weighted_success', enabled: true, primary_channels: [{ channel_id: 1, percent: 100, upstream_model: 'gpt-5.6-sol', upstream_free: false }], fallback_channels: [] }],
      aliases: [{ id: 1, alias: 'gpt-5.6', target_model: 'gpt-5.6-sol', enabled: true }],
      channel_keys: [
        { id: 11, channel_id: 1, channel_name: 'Fixture OpenAI', label: 'Primary synthetic key', masked_key: 'sk-fi…0001', weight: 1, status: 1, quota_total: -1, quota_remaining: -1, failure_count: 0, last_used_at: isoDaysAgo(0), last_error_at: null, channel_status: 1 },
        { id: 12, channel_id: 2, channel_name: 'Fixture Anthropic', label: 'Backup synthetic key', masked_key: 'sk-fi…0002', weight: 1, status: 1, quota_total: 50000000, quota_remaining: 41800000, failure_count: 1, last_used_at: isoDaysAgo(1), last_error_at: isoDaysAgo(1), channel_status: 1 }
      ],
      settings: {
        security: { totp_enabled: false, pending: false, admin_proof_lifetime_minutes: 5 },
        announcements: { public: '这是本地管理端 Fixture 预览。', pay: '' },
        registration: { local_registration_enabled: true, pay_local_registration_enabled: false, linuxdo_login_enabled: false, linuxdo_registration_enabled: false, registration_code_required: false, pay_registration_code_required: false, public_initial_quota: 500000, pay_initial_quota: 0, linuxdo_extra_quota: 0, registration_code_configured: false, pay_registration_code_configured: false },
        linuxdo: { client_id: 'fixture-linuxdo-client', redirect_url: 'http://127.0.0.1:23000/auth/?mode=login', client_secret_configured: false },
        routing: { channel_affinity_minutes: 30, channel_key_affinity_minutes: 10, channel_failure_threshold: 3, channel_key_recovery_minutes: 20, retry_enabled: true, retry_max_attempts: 2, retry_status_codes: [429, 500, 502, 503], session_lease_minutes: 30, session_requests_per_minute: 120 },
        operational: { global_ip_requests_per_minute: 1200, default_user_requests_per_minute: 120, default_user_concurrent_requests: 8, stream_heartbeat_seconds: 15, realtime_handshake_timeout_seconds: 20, realtime_write_timeout_seconds: 30, realtime_max_message_mb: 16, default_max_output_tokens: 16384, unknown_image_token_estimate: 1024, user_docs_visible: true },
        token_loan: { enabled: true, max_borrow_quota: 500000, min_checkin_quota: 1440000, daily_cut_limit: 2, loss_pool: { visible: true, title: '帮助站长砍一刀', description: '本地合成亏损池，不会影响真实服务。', total_quota: 460970646, remaining_quota: 460970646, cut_min_quota: 500000, cut_max_quota: 1500000 } },
        payment: { enabled: false, pro_site_host: 'pro.synthetic.invalid', gateway_url: 'https://pay.synthetic.invalid', merchant_id: '', merchant_key_configured: false, cny_per_usd: 7.3, min_cny: 1, max_cny: 5000, subscription_markup: 1.04 },
        emergency: { enabled: false, indefinite: false, remaining_seconds: 0, until: null }
      }
    };
  }

  function loadState() {
    try {
      const stored = JSON.parse(localStorage.getItem(STORAGE_KEY) || 'null');
      if (stored && stored.self && stored.settings) {
        // Migrate the earlier two-plan fixture once.  Later edits made in the
        // admin UI are preserved because the catalog version is now current.
        if (Number(stored.subscription_catalog_version || 0) < 2) {
          const seed = defaults();
          stored.models = seed.models;
          stored.subscription_plans = seed.subscription_plans;
          stored.subscription_catalog_version = 2;
        }
        if (!stored.settings.linuxdo) stored.settings.linuxdo = defaults().settings.linuxdo;
        localStorage.setItem(STORAGE_KEY, JSON.stringify(stored));
        return stored;
      }
    } catch (_) {}
    return defaults();
  }

  let state = loadState();
  localStorage.setItem(TOKEN_KEY, 'fixture-admin-session');

  function save() {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
  }

  function bodyOf(init) {
    if (!init || !init.body) return {};
    if (typeof init.body === 'string') {
      try { return JSON.parse(init.body) || {}; } catch (_) { return {}; }
    }
    return init.body || {};
  }

  function error(message, status) {
    const value = new Error(message);
    value.status = status || 400;
    throw value;
  }

  function trend(days) {
    const count = days === 30 ? 30 : 7;
    return Array.from({ length: count }, function (_, index) {
      const age = count - index - 1;
      return { date: dateKey(age), quota: 21000 + index * 1700, tokens: 18000 + index * 1300, requests: 42 + index * 4, active_users: 6 + (index % 4), errors: index % 6 === 0 ? 2 : 0, error_rate: index % 6 === 0 ? 3.2 : 0.8 };
    });
  }

  function overview(url) {
    const days = Number(new URL(url, location.origin).searchParams.get('days')) === 30 ? 30 : 7;
    return {
      users: state.users.length, enabled_users: state.users.filter(function (item) { return Number(item.status) === 1; }).length,
      channels: state.channels.length, active_tokens: 7, logs: 1842, input_tokens: 1884200, output_tokens: 792500,
      quota: 186420, today: { quota: 34200, tokens: 268420, requests: 238, active_users: 18, errors: 4, error_rate: 1.7, primary_error: 'rate_limit_exceeded' },
      site_balances: { public: { users: 24, permanent_quota: 41200000, timed_quota: 5400000, group_quota: 1200000, subscription_quota: 16800000, estimated_quota: 64600000, unlimited_subscriptions: 1 }, pay: { users: 8, permanent_quota: 28400000, timed_quota: 0, group_quota: 0, subscription_quota: 42000000, estimated_quota: 70400000, unlimited_subscriptions: 0 } },
      trend: trend(days), rankings: { window_days: days, top_users: [{ username: 'demo_operator', user_id: 10002, requests: 80, tokens: 120000, quota: 64000 }, { username: 'fixture_user', user_id: 10001, requests: 51, tokens: 72000, quota: 24000 }], top_models: [{ model: 'gpt-5.6-sol', requests: 142, tokens: 182000, quota: 52000 }, { model: 'claude-opus-5', requests: 61, tokens: 78000, quota: 31000 }], top_error_channels: [{ channel_name: 'Fixture Anthropic', channel_id: 2, requests: 22, errors: 3, rate_limited: 2, server_errors: 1, error_rate: 13.6 }], high_frequency_keys: [{ token_name: 'demo-operator-key', token_id: 2002, username: 'demo_operator', user_id: 10002, requests: 66, error_rate: 1.5, frequency_ratio: 2.8 }] }
    };
  }

  function paged(items, url) {
    const query = new URL(url, location.origin).searchParams;
    const pageSize = Math.max(1, Number(query.get('page_size')) || 50);
    const page = Math.max(1, Number(query.get('page')) || 1);
    const start = (page - 1) * pageSize;
    return { items: items.slice(start, start + pageSize), total: items.length, total_pages: Math.max(1, Math.ceil(items.length / pageSize)), page: page, page_size: pageSize };
  }

  function groupedCodes(items) {
    const groups = {};
    (items || []).forEach(function (item) {
      const key = item.batch_id || ('batch-' + item.id);
      if (!groups[key]) groups[key] = { batch_id: key, name: item.name || '本地合成批次', user_group: item.user_group || 'default', created_at: item.created_at, items: [] };
      groups[key].items.push(item);
    });
    return Object.keys(groups).map(function (key) {
      const group = groups[key];
      const itemsInBatch = group.items;
      group.count = itemsInBatch.length;
      group.unused_count = itemsInBatch.filter(function (item) { return Number(item.status || 1) === 1; }).length;
      group.used_count = itemsInBatch.filter(function (item) { return Number(item.status) === 2; }).length;
      group.deletable_count = group.unused_count;
      group.redeemed_count = itemsInBatch.reduce(function (sum, item) { return sum + Number(item.redeemed_count || 0); }, 0);
      group.quota = itemsInBatch[0] && itemsInBatch[0].quota;
      group.validity_days = itemsInBatch[0] && itemsInBatch[0].validity_days;
      return group;
    });
  }

  function syntheticCode(prefix, index) {
    const suffix = String(Date.now()).slice(-6) + '-' + String(index + 1).padStart(3, '0');
    return 'FIXTURE-' + prefix + '-' + suffix;
  }

  function settingsResponse(name) {
    if (name === 'announcements') return { announcements: state.settings.announcements };
    if (name === 'emergency-stop') {
      const emergency = state.settings.emergency;
      return Object.assign({}, emergency, { status: emergency });
    }
    if (name === 'registration') return { settings: state.settings.registration || {}, linuxdo: state.settings.linuxdo || {} };
    return { settings: state.settings[name] || {} };
  }

  function handle(method, url, init) {
    const parsed = new URL(url, location.origin);
    const path = parsed.pathname;
    if (method === 'GET' && path === '/api/user/self') return state.self;
    if (path === '/api/admin/overview') return overview(parsed.href);
    if (method === 'GET' && path === '/api/admin/security') return state.settings.security;
    if (method === 'POST' && path === '/api/admin/security/verify') return { proof: 'fixture-admin-proof', expires_at: new Date(Date.now() + 300000).toISOString() };
    if (method === 'GET' && path === '/api/admin/security/audit') return paged(state.audit, parsed.href);
    if (method === 'GET' && path === '/api/admin/security/proof-lifetime') return { admin_proof_lifetime_minutes: state.settings.security.admin_proof_lifetime_minutes };
    if (method === 'GET' && path === '/api/admin/model-health') return { summary: { today: { requests: 238, success_rate: 98.3 } }, models: state.models.map(function (item) { return { model: item.model, today: { requests: 120, successful: 118, success_rate: 98.3 }, recent: { requests: 60, successful: 59, success_rate: 98.3 }, ttft_ms: 640 }; }), channels: state.channels.map(function (item) { return { channel: item.name, requests: 120, successful: 118, success_rate: 98.3, ttft_ms: 640 }; }), updated_at: new Date().toISOString() };
    if (method === 'GET' && path === '/api/admin/channels') return state.channels;
    if (method === 'GET' && path === '/api/admin/channels/disabled') return state.channels.filter(function (item) { return Number(item.status) !== 1; });
    if (method === 'GET' && path === '/api/admin/channel-keys') return state.channel_keys;
    if (method === 'GET' && path === '/api/admin/models') return state.models;
    if (method === 'GET' && path === '/api/admin/subscription-plans') return state.subscription_plans;
    if (method === 'GET' && path === '/api/admin/subscription-benefits') return { items: state.benefits, total: state.benefits.length, total_pages: 1 };
    if (method === 'GET' && path === '/api/admin/billing-promotions') return state.promotions;
    if (method === 'GET' && path === '/api/admin/users') return paged(state.users, parsed.href);
    if (method === 'GET' && path === '/api/admin/logs') return paged(state.logs, parsed.href);
    if (method === 'GET' && path === '/api/admin/traces') return paged(state.traces, parsed.href);
    if (method === 'GET' && path === '/api/admin/redemptions') return paged(parsed.searchParams.get('group_by') === 'batch' ? groupedCodes(state.redemptions) : state.redemptions, parsed.href);
    if (method === 'GET' && path === '/api/admin/registration-codes') return paged(parsed.searchParams.get('group_by') === 'batch' ? groupedCodes(state.registration_codes) : state.registration_codes, parsed.href);
    if (method === 'GET' && path === '/api/admin/prepaid-cards') return paged(parsed.searchParams.get('group_by') === 'batch' ? groupedCodes(state.prepaid_cards) : state.prepaid_cards, parsed.href);
    if (method === 'GET' && path === '/api/admin/payment-orders') return [];
    if (method === 'GET' && path === '/api/admin/routing/rules') return state.routing_rules;
    if (method === 'GET' && path === '/api/admin/routing/aliases') return state.aliases;
    if (method === 'GET' && path.indexOf('/api/admin/settings/') === 0) return settingsResponse(path.slice('/api/admin/settings/'.length));
    if (method === 'GET' && path === '/api/admin/settings/payment') return { settings: state.settings.payment };
    if (method === 'GET' && path === '/api/admin/settings/emergency-stop') return state.settings.emergency;

    const parts = path.split('/').filter(Boolean);
    const collection = parts[2];
    const id = Number(parts[3]);
    const payload = bodyOf(init);
    if (collection === 'channels' && method === 'POST' && parts[3] === 'disabled') return [];
    if (collection === 'channels' && parts[3] && parts[4] === 'operation') {
      const item = state.channels.find(function (row) { return row.id === id; });
      if (item) { item.status = payload.action === 'drain' ? 2 : 1; item.failure_count = 0; save(); }
      return { message: 'Fixture 渠道状态已更新', channel: item };
    }
    if (collection === 'channels' && parts[3] && parts[4] === 'probe') return { success: true, latency_ms: 82, status_code: 200 };
    if (collection === 'channels' && parts[3] && parts[4] === 'keys') return { message: 'Fixture Key 操作已完成' };
    if (collection === 'models' && parts[3] && parts[4] === 'status') { const item = state.models.find(function (row) { return row.id === id; }); if (item) item.enabled = payload.enabled !== false; save(); return item || {}; }
    if (collection === 'models' && method === 'DELETE') { state.models = state.models.filter(function (row) { return row.id !== id; }); save(); return { message: 'Fixture 模型已删除' }; }
    if (collection === 'channels' && method === 'DELETE') { state.channels = state.channels.filter(function (row) { return row.id !== id; }); save(); return { message: 'Fixture 渠道已删除' }; }
    if (collection === 'models' && (method === 'POST' || method === 'PUT')) { const item = Object.assign({ id: method === 'POST' ? Date.now() : id }, payload); if (method === 'PUT') state.models = state.models.map(function (row) { return row.id === id ? Object.assign(row, item) : row; }); else state.models.push(item); save(); return item; }
    if (collection === 'channels' && (method === 'POST' || method === 'PUT')) { const item = Object.assign({ id: method === 'POST' ? Date.now() : id, status: 1, key_count: 0 }, payload); if (method === 'PUT') state.channels = state.channels.map(function (row) { return row.id === id ? Object.assign(row, item) : row; }); else state.channels.push(item); save(); return item; }
    if (collection === 'subscription-plans' && parts[4] === 'status' && method === 'PATCH') {
      const item = state.subscription_plans.find(function (row) { return row.id === id; });
      if (!item) return { message: 'Fixture 订阅计划不存在' };
      item.status = Number(payload.status) === 1 ? 1 : 2;
      save();
      return item;
    }
    if (collection === 'subscription-plans' && method === 'DELETE') {
      state.subscription_plans = state.subscription_plans.filter(function (row) { return row.id !== id; });
      save();
      return { message: 'Fixture 订阅计划已删除' };
    }
    if (collection === 'subscription-plans' && (method === 'POST' || method === 'PUT')) {
      if (method === 'PUT') {
        const existing = state.subscription_plans.find(function (row) { return row.id === id; });
        if (!existing) return { message: 'Fixture 订阅计划不存在' };
        Object.assign(existing, payload, { id: id });
        save();
        return existing;
      }
      const nextID = state.subscription_plans.reduce(function (max, row) { return Math.max(max, Number(row.id) || 0); }, 0) + 1;
      const item = Object.assign({ id: nextID, status: 1, display_order: nextID, collapsed: false, model_rules: [] }, payload, { id: nextID });
      state.subscription_plans.push(item);
      save();
      return item;
    }
    if (collection === 'security' && parts[3] === 'totp' && parts[4] === 'setup') return { secret: 'FIXTUREONLY000000', otpauth_uri: 'otpauth://totp/RelayFor:fixture_admin?secret=FIXTUREONLY000000', qr_code_data_uri: '' };
    if (collection === 'security' && parts[3] === 'totp') { state.settings.security.totp_enabled = parts[4] !== 'disable'; save(); return state.settings.security; }
    if (collection === 'security' && parts[3] === 'proof-lifetime') { state.settings.security.admin_proof_lifetime_minutes = Number(payload.admin_proof_lifetime_minutes) || 5; save(); return state.settings.security; }
    if (collection === 'routing') { save(); return { message: 'Fixture 路由已更新' }; }
    if (collection === 'settings') {
      const name = parts[3];
      if (name === 'registration') {
        const secret = String(payload.new_linuxdo_client_secret || '');
        delete payload.new_linuxdo_client_secret;
        const clientID = payload.linuxdo_client_id;
        const redirectURL = payload.linuxdo_redirect_url;
        delete payload.linuxdo_client_id;
        delete payload.linuxdo_redirect_url;
        if (state.settings.registration) state.settings.registration = Object.assign(state.settings.registration, payload);
        state.settings.linuxdo = Object.assign(state.settings.linuxdo || {}, {
          client_id: clientID == null ? (state.settings.linuxdo && state.settings.linuxdo.client_id) : String(clientID),
          redirect_url: redirectURL == null ? (state.settings.linuxdo && state.settings.linuxdo.redirect_url) : String(redirectURL)
        });
        if (secret) state.settings.linuxdo.client_secret_configured = true;
        save();
        return { settings: state.settings.registration, linuxdo: state.settings.linuxdo, message: 'Fixture 注册设置已保存' };
      }
      if (state.settings[name]) state.settings[name] = Object.assign(state.settings[name], payload);
      save();
      return { settings: state.settings[name], message: 'Fixture 设置已保存' };
    }
    if (collection === 'subscription-benefits' || collection === 'billing-promotions') { save(); return { message: 'Fixture 订阅配置已更新' }; }
    if (collection === 'users') { const item = state.users.find(function (row) { return row.id === id; }); if (item) Object.assign(item, payload); save(); return item || { message: 'Fixture 用户已更新' }; }
    if (collection === 'redemptions' && parts[3] === 'generate' && method === 'POST') {
      const publicCode = Boolean(payload.is_public);
      const count = publicCode ? 1 : Math.max(1, Math.min(100, Number(payload.count) || 1));
      const batchID = 'fixture-redemption-' + Date.now().toString(36);
      const createdAt = new Date().toISOString();
      const codes = Array.from({ length: count }, function (_, index) {
        const item = Object.assign({}, payload, { id: Date.now() + index, code: syntheticCode('REDEEM', index), batch_id: batchID, created_at: createdAt, status: 1, redeemed_count: 0, redeem_limit: publicCode ? Number(payload.redeem_limit) || 1 : 1 });
        state.redemptions.push(item);
        return item.code;
      });
      save();
      return { codes: codes, count: count, user_group: payload.user_group || 'default', message: '已生成 ' + count + ' 个合成兑换码' };
    }
    if (collection === 'registration-codes' && parts[3] === 'generate' && method === 'POST') {
      const count = Math.max(1, Math.min(100, Number(payload.count) || 1));
      const batchID = 'fixture-registration-' + Date.now().toString(36);
      const createdAt = new Date().toISOString();
      const codes = Array.from({ length: count }, function (_, index) {
        const item = Object.assign({}, payload, { id: Date.now() + index, code: syntheticCode('REGISTER', index), batch_id: batchID, created_at: createdAt, status: 1 });
        state.registration_codes.push(item);
        return item.code;
      });
      save();
      return { codes: codes, count: count, message: '已生成 ' + count + ' 个合成注册码' };
    }
    if (collection === 'prepaid-cards' && parts[3] === 'generate' && method === 'POST') {
      const count = Math.max(1, Math.min(100, Number(payload.count) || 1));
      const batchID = 'fixture-prepaid-' + Date.now().toString(36);
      const createdAt = new Date().toISOString();
      const codes = Array.from({ length: count }, function (_, index) {
        const item = Object.assign({}, payload, { id: Date.now() + index, code: syntheticCode('CARD', index), batch_id: batchID, created_at: createdAt, status: 1 });
        state.prepaid_cards.push(item);
        return item.code;
      });
      save();
      return { codes: codes, count: count, message: '已生成 ' + count + ' 张合成预付卡' };
    }
    if (collection === 'redemptions' || collection === 'registration-codes' || collection === 'prepaid-cards') { save(); return { message: 'Fixture 凭证操作已完成' }; }
    return { message: 'Fixture 已处理', synthetic: true };
  }

  const originalFetch = window.fetch.bind(window);
  window.fetch = async function adminFixtureFetch(input, init) {
    const requestURL = typeof input === 'string' ? input : input.url;
    const url = new URL(requestURL, location.origin);
    const method = String((init && init.method) || (input && input.method) || 'GET').toUpperCase();
    if (url.pathname.indexOf('/api/admin') !== 0 && url.pathname !== '/api/user/self') return originalFetch(input, init);
    try {
      const result = handle(method, url.href, init || {});
      return new Response(JSON.stringify(result), { status: 200, headers: { 'Content-Type': 'application/json' } });
    } catch (caught) {
      return new Response(JSON.stringify({ error: caught.message || 'Fixture error' }), { status: caught.status || 500, headers: { 'Content-Type': 'application/json' } });
    }
  };

  window.ADMIN_FIXTURE = {
    get state() { return state; },
    reset: function () { state = defaults(); save(); location.reload(); },
    save: save,
    clearSession: function () { localStorage.removeItem(TOKEN_KEY); }
  };
})();
