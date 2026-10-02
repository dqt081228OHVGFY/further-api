(function () {
  'use strict';
  const api = window.ProxyLan;
  let bound = false;
  let activeUser = null;
  let actor = null;
  let generation = 0;
  let busy = false;
  let createGeneration = 0;
  const esc = function (value) { return api.escapeHTML(String(value == null ? '' : value)); };
  const builtins = [ ['email', 'email', '邮箱'], ['github', 'github_id', 'GitHub'], ['discord', 'discord_id', 'Discord'], ['wechat', 'wechat_id', '微信'], ['oidc', 'oidc_id', 'OIDC'], ['telegram', 'telegram_id', 'Telegram'], ['linuxdo', 'linux_do_id', 'LinuxDO'] ];
  // The shell exposes administrators as 50; the direct account API uses 10.
  function normalizeActor(user) { return Object.assign({}, user, { role: Number(user.role) === 50 ? 10 : Number(user.role) }); }

  function dialog(id, title) {
    let node = document.getElementById(id);
    if (!node) {
      node = document.createElement('div');
      node.id = id;
      node.className = 'dialog-backdrop';
      node.innerHTML = '<div class="md-dialog" role="dialog" aria-modal="true" aria-labelledby="' + id + '-title" style="width:min(760px,calc(100vw - 32px));max-height:90vh;overflow:auto"><div class="md-dialog-title" id="' + id + '-title"></div><div data-user-action-body></div><div class="form-actions"><button type="button" class="md-btn md-btn-text" data-dialog-close>关闭</button></div></div>';
      document.body.appendChild(node);
      node.addEventListener('click', function (event) {
        if (event.target === node || event.target.closest('[data-dialog-close]')) api.closeDialog(id);
      });
      node.addEventListener('keydown', function (event) {
        if (event.key === 'Escape') { event.preventDefault(); api.closeDialog(id); }
      });
    }
    node.querySelector('.md-dialog-title').textContent = title;
    return node;
  }

  function button(action, label, value, disabled) {
    return '<button type="button" class="md-btn md-btn-tonal" data-account-action="' + action + '" data-value="' + esc(value || '') + '"' + (disabled ? ' disabled' : '') + '>' + label + '</button>';
  }
  function canManage() { return actor && activeUser && (Number(actor.role) === 100 || Number(actor.role) > Number(activeUser.role)); }
  function updated() { document.dispatchEvent(new CustomEvent('relayfor:users-updated')); }
  function section(title, content) { return '<section class="md-card" style="margin:16px 0;padding:16px"><h3 class="md-card-title">' + title + '</h3>' + content + '</section>'; }
  function failure(error) { return '<p class="field-hint" role="alert">加载失败：' + esc(error.message || '请重试') + '</p>'; }

  function validPassword(value) {
    // Match accountPasswordSchema and the backend's Unicode character count.
    const length = Array.from(value).length;
    if (length >= 8 && length <= 128) return true;
    api.toast('密码须包含 8–128 个字符', 'danger');
    return false;
  }

  const permissionLabels = {
    'Audit Logs': '审计日志', 'Channel Management': '渠道管理', 'Task Plugin': '任务插件',
    "View other accounts' audit logs": '查看其他账户的审计日志',
    'View audit records from user and admin roles. Root records are always excluded.': '查看用户及管理员的审计记录，不包含超级管理员记录。',
    'Read channels': '查看渠道', 'View channel lists and details without secrets.': '查看渠道列表和详情，不包含密钥。',
    'Operate channels': '操作渠道', 'Test channels, refresh balances, and enable/disable individual, batch, or tagged channels.': '测试渠道、刷新余额，以及按单个、批量或标签启用和停用渠道。',
    'Edit channel routing': '编辑渠道路由', 'Edit non-sensitive settings such as models, groups, and routing rules.': '编辑模型、分组及路由规则等普通设置。',
    'Edit sensitive channel settings': '编辑渠道敏感设置', 'Create channels or edit keys, base URLs, and overrides.': '创建渠道，编辑密钥、上游地址及覆盖参数。',
    'View channel secrets': '查看渠道密钥', 'Reserved for viewing complete channel keys after secure verification.': '通过安全验证后查看完整渠道密钥。',
    'Bind task plugins': '绑定任务插件', 'List registered task plugins and bind them when creating or editing task plugin channels.': '查看已注册的任务插件，并在创建或编辑渠道时绑定插件。'
  };
  function permissionLabel(key) { return esc(permissionLabels[key] || key); }

  async function loadPermissions(node, user, isCurrent) {
    node.innerHTML = '<p role="status">正在加载管理员权限…</p>';
    node.dataset.loaded = 'false';
    try {
      const catalog = await api.request('/api/authz/catalog');
      if (!isCurrent()) return;
      if (!catalog || !Array.isArray(catalog.resources) || !Array.isArray(catalog.roles)) throw new Error('权限目录格式无效');
      const baseline = (catalog.roles.find(function (role) { return role.key === 'admin'; }) || {}).grants || {};
      const current = user.admin_permissions || {};
      node.innerHTML = '<p class="card-hint">权限依据系统目录显示，修改后覆盖该管理员的默认权限。</p>' + catalog.resources.map(function (resource) {
        return '<fieldset style="margin:12px 0;padding:12px"><legend>' + permissionLabel(resource.label_key) + '</legend>' + resource.actions.map(function (action) {
          const supplied = current[resource.resource] && current[resource.resource][action.action];
          const allowed = supplied == null ? !!(baseline[resource.resource] && baseline[resource.resource][action.action]) : supplied === true;
          return '<label style="display:block;margin:12px 0"><input type="checkbox" data-permission-resource="' + esc(resource.resource) + '" data-permission-action="' + esc(action.action) + '"' + (allowed ? ' checked' : '') + '> ' + permissionLabel(action.label_key) + '<span class="field-hint" style="display:block">' + permissionLabel(action.description_key || '') + '</span></label>';
        }).join('') + '</fieldset>';
      }).join('');
      node.dataset.loaded = 'true';
      const submit = node.closest('form').querySelector('[type="submit"]');
      if (submit) submit.disabled = false;
    } catch (error) {
      if (isCurrent()) node.innerHTML = failure(error) + button('reload-permissions', '重试');
    }
  }

  function permissionValues(node) {
    const matrix = {};
    node.querySelectorAll('[data-permission-resource]').forEach(function (input) {
      const resource = input.dataset.permissionResource;
      if (!matrix[resource]) matrix[resource] = {};
      matrix[resource][input.dataset.permissionAction] = input.checked;
    });
    return matrix;
  }

  async function updateAccount(id, changes) {
    // PUT also writes profile fields. Preserve the latest values when changing
    // only a password or the permission matrix.
    const user = await api.request('/api/user/' + id);
    if (!actor || (Number(actor.role) !== 100 && Number(actor.role) <= Number(user.role))) throw new Error('当前角色无权修改此账户');
    if (changes.admin_permissions && (Number(actor.role) !== 100 || Number(user.role) < 10)) throw new Error('只有超级管理员可以修改管理员权限');
    return api.request('/api/user/', { method: 'PUT', body: Object.assign({ id: id, username: user.username, display_name: user.display_name || '', group: user.group || '', remark: user.remark || '' }, changes) });
  }

  async function openActions(id) {
    const version = ++generation;
    const node = dialog('user-actions-dialog', '用户账户管理');
    const body = node.querySelector('[data-user-action-body]');
    activeUser = null;
    body.innerHTML = '<p role="status">正在加载账户…</p>';
    api.openDialog(node.id);
    try {
      const result = await Promise.all([api.request('/api/user/' + id), api.request('/api/user/self')]);
      if (version !== generation) return;
      activeUser = result[0]; actor = normalizeActor(result[1]);
      const editable = canManage();
      node.querySelector('.md-dialog-title').textContent = (activeUser.display_name || activeUser.username) + ' · 账户管理';
      body.innerHTML = '<p class="card-hint">用户 #' + Number(activeUser.id) + ' · ' + esc(activeUser.username) + '</p>' +
        (!editable ? '<p role="status">当前角色无权修改此账户。</p>' : '') +
        section('账户绑定', '<div data-account-bindings>正在加载绑定…</div>') +
        section('用户订阅', '<div data-account-subscriptions>正在加载订阅…</div>') +
        section('账户密码', '<p class="card-hint">设置新密码后，该账户已有登录会话将失效。</p><form data-reset-user-password><label class="md-textfield">新密码<input class="md-input" name="password" type="password" required autocomplete="new-password" placeholder="8–128 个字符"' + (!editable ? ' disabled' : '') + '></label><div class="form-actions"><button type="submit" class="md-btn md-btn-tonal"' + (!editable ? ' disabled' : '') + '>重设用户密码</button></div></form>') +
        (Number(actor.role) === 100 && Number(activeUser.role) >= 10 ? section('管理员权限', '<form data-update-admin-permissions><div data-account-permissions></div><div class="form-actions"><button type="submit" class="md-btn md-btn-filled" disabled>保存管理员权限</button></div></form>') : '') +
        section('安全验证', '<p class="card-hint">重置后用户需要重新绑定相应的验证方式。</p><div class="form-actions">' + button('passkey', '重置通行密钥', '', !editable) + button('twofa', '重置双重验证', '', !editable) + '</div>') +
        section('删除账户', '<p class="card-hint">永久删除该账户，无法撤销。</p>' + button('delete-user', '永久删除账户', '', !editable || Number(actor.role) <= Number(activeUser.role) || Number(actor.id) === Number(activeUser.id)));
      const permissions = body.querySelector('[data-account-permissions]');
      await Promise.all([loadBindings(version), loadSubscriptions(version), permissions ? loadPermissions(permissions, activeUser, function () { return version === generation; }) : Promise.resolve()]);
    } catch (error) {
      if (version === generation) body.innerHTML = failure(error) + '<button type="button" class="md-btn md-btn-tonal" data-user-actions="' + Number(id) + '">重新加载</button>';
    }
  }

  async function loadBindings(version) {
    const node = document.querySelector('[data-account-bindings]');
    try {
      const bindings = await api.request('/api/user/' + activeUser.id + '/oauth/bindings');
      if (version !== generation) return;
      const rows = builtins.filter(function (item) { return activeUser[item[1]]; }).map(function (item) {
        return '<div class="list-item"><div class="list-item-body"><b>' + item[2] + '</b><p>' + esc(activeUser[item[1]]) + '</p></div>' + button('binding', '解除绑定', item[0], !canManage()) + '</div>';
      });
      (Array.isArray(bindings) ? bindings : []).forEach(function (item) {
        rows.push('<div class="list-item"><div class="list-item-body"><b>' + esc(item.provider_name) + '</b><p>' + esc(item.provider_user_id) + '</p></div>' + button('oauth', '解除绑定', item.provider_id, !canManage()) + '</div>');
      });
      node.innerHTML = rows.join('') || '<p class="card-hint">此账户尚未绑定第三方登录或邮箱。</p>';
    } catch (error) { if (version === generation) node.innerHTML = failure(error) + button('reload-bindings', '重试'); }
  }

  async function loadSubscriptions(version) {
    const node = document.querySelector('[data-account-subscriptions]');
    try {
      const result = await Promise.all([api.request('/api/subscription/admin/users/' + activeUser.id + '/subscriptions'), api.request('/api/subscription/admin/plans')]);
      if (version !== generation) return;
      const plans = (Array.isArray(result[1]) ? result[1] : []).map(function (record) { return record.plan; });
      const records = Array.isArray(result[0]) ? result[0] : [];
      const enabled = plans.filter(function (plan) { return plan.enabled; });
      const options = function (items) { return items.map(function (plan) { return '<option value="' + Number(plan.id) + '">' + esc(plan.title) + '</option>'; }).join(''); };
      let html = '<form data-grant-user-subscription><label class="md-textfield">分配套餐<select class="md-input" name="plan_id" required' + (!enabled.length || !canManage() ? ' disabled' : '') + '>' + options(enabled) + '</select></label><button class="md-btn md-btn-filled" type="submit"' + (!enabled.length || !canManage() ? ' disabled' : '') + '>分配订阅</button></form>';
      if (!enabled.length) html += '<p class="field-hint">暂无可分配的启用套餐。</p>';
      html += records.map(function (record) {
        const sub = record.subscription;
        const plan = plans.find(function (item) { return Number(item.id) === Number(sub.plan_id); });
        const status = { active: '生效中', expired: '已到期', cancelled: '已作废' }[sub.status] || sub.status;
        return '<div class="list-item"><div class="list-item-body"><b>' + esc(plan ? plan.title : '套餐 #' + sub.plan_id) + '</b><p>' + esc(status) + ' · 截止 ' + esc(api.date(sub.end_time)) + '</p><p>已用 ' + esc(api.moneyFromQuota(sub.amount_used)) + ' / ' + (Number(sub.amount_total) === 0 ? '不限额' : esc(api.moneyFromQuota(sub.amount_total))) + '</p><div class="form-actions">' + button('invalidate-sub', '作废订阅', sub.id, !canManage() || sub.status !== 'active') + button('delete-sub', '删除记录', sub.id, !canManage()) + '</div></div></div>';
      }).join('') || '';
      if (!records.length) html += '<p class="card-hint">暂无订阅记录。</p>';
      const activePlanIds = new Set(records.filter(function (record) { return record.subscription.status === 'active'; }).map(function (record) { return Number(record.subscription.plan_id); }));
      const resetPlans = plans.filter(function (plan) { return activePlanIds.has(Number(plan.id)); });
      if (resetPlans.length) html += '<form data-reset-user-subscriptions><label class="md-textfield">重置套餐用量<select class="md-input" name="plan_id">' + options(resetPlans) + '</select></label><label><input type="checkbox" name="advance_reset_time"> 同时推进下一重置时间</label><div class="form-actions"><button class="md-btn md-btn-tonal" type="submit"' + (!canManage() ? ' disabled' : '') + '>重置此套餐用量</button></div></form>';
      node.innerHTML = html;
    } catch (error) { if (version === generation) node.innerHTML = failure(error) + button('reload-subscriptions', '重试'); }
  }

  async function mutate(trigger, message, operation, after) {
    if (busy) return;
    busy = true;
    try {
      if (message && !await api.confirm(message)) return;
      api.showBusy(trigger, true);
      await operation();
      api.toast('操作成功');
      updated();
      if (after) await after();
    } catch (error) { api.toast(error.message || '操作失败，请重试', 'danger'); }
    finally { api.showBusy(trigger, false); busy = false; }
  }

  async function openCreate() {
    const version = ++createGeneration;
    const node = dialog('create-user-dialog', '新建用户');
    const body = node.querySelector('[data-user-action-body]');
    body.innerHTML = '<p role="status">正在检查权限…</p>';
    api.openDialog(node.id);
    try {
      actor = normalizeActor(await api.request('/api/user/self'));
      if (version !== createGeneration) return;
      if (Number(actor.role) < 10) throw new Error('当前账户没有创建用户权限');
      body.innerHTML = '<form data-create-user-form><div class="md-textfield"><input class="md-input" name="username" maxlength="20" required autocomplete="off" placeholder=" "><label class="md-input-label">用户名</label></div><div class="md-textfield"><input class="md-input" name="display_name" maxlength="20" placeholder=" "><label class="md-input-label">显示名称</label></div><div class="md-textfield"><input class="md-input" name="password" type="password" required autocomplete="new-password" placeholder=" "><label class="md-input-label">初始密码（8–128 个字符）</label></div><div class="md-textfield"><select class="md-input" name="role"><option value="1">普通用户</option>' + (Number(actor.role) === 100 ? '<option value="10">管理员</option>' : '') + '</select><label class="md-input-label">角色</label></div>' + (Number(actor.role) === 100 ? '<section data-create-permissions-section hidden><h3>管理员权限</h3><div data-create-permissions></div></section>' : '') + '<div class="form-actions"><button type="submit" class="md-btn md-btn-filled">创建用户</button></div></form>';
      const role = body.querySelector('[name="role"]');
      const permissions = body.querySelector('[data-create-permissions]');
      role.addEventListener('change', function () {
        if (!permissions) return;
        permissions.parentElement.hidden = Number(role.value) !== 10;
        body.querySelector('[type="submit"]').disabled = Number(role.value) === 10 && permissions.dataset.loaded !== 'true';
        if (Number(role.value) === 10 && permissions.dataset.loaded !== 'true') loadPermissions(permissions, {}, function () { return version === createGeneration; });
      });
    } catch (error) { body.innerHTML = failure(error); }
  }

  function bind() {
    if (bound) return;
    bound = true;
    document.addEventListener('click', async function (event) {
      const target = event.target.closest('[data-user-actions], [data-create-user], [data-account-action]');
      if (!target || target.disabled || busy) return;
      if (target.hasAttribute('data-user-actions')) { await openActions(Number(target.dataset.userActions)); return; }
      if (target.hasAttribute('data-create-user')) { await openCreate(); return; }
      const action = target.dataset.accountAction;
      if (action === 'reload-permissions') {
        if (!actor || Number(actor.role) !== 100) return;
        const node = target.closest('[data-account-permissions], [data-create-permissions]');
        const creating = node.hasAttribute('data-create-permissions');
        const version = creating ? createGeneration : generation;
        await loadPermissions(node, creating ? {} : activeUser, function () { return version === (creating ? createGeneration : generation); });
        return;
      }
      if (action === 'reload-bindings') { await loadBindings(generation); return; }
      if (action === 'reload-subscriptions') { await loadSubscriptions(generation); return; }
      if (!canManage()) return;
      const id = Number(activeUser.id);
      const value = target.dataset.value;
      const paths = {
        passkey: '/api/user/' + id + '/reset_passkey', twofa: '/api/user/' + id + '/2fa',
        binding: '/api/user/' + id + '/bindings/' + encodeURIComponent(value), oauth: '/api/user/' + id + '/oauth/bindings/' + Number(value),
        'delete-user': '/api/user/' + id + '/', 'delete-sub': '/api/subscription/admin/user_subscriptions/' + Number(value),
        'invalidate-sub': '/api/subscription/admin/user_subscriptions/' + Number(value) + '/invalidate'
      };
      if (!paths[action]) return;
      if (action === 'delete-user' && (Number(actor.role) <= Number(activeUser.role) || Number(actor.id) === id)) return;
      const warning = action === 'delete-user' ? '确认永久删除用户「' + activeUser.username + '」？此操作无法撤销。' : '确认对用户「' + activeUser.username + '」执行「' + target.textContent.trim() + '」？';
      await mutate(target, warning, function () { return api.request(paths[action], { method: action === 'invalidate-sub' ? 'POST' : 'DELETE' }); }, function () {
        if (action === 'delete-user') { ++generation; activeUser = null; api.closeDialog('user-actions-dialog'); return; }
        return openActions(id);
      });
    });
    document.addEventListener('submit', async function (event) {
      const form = event.target;
      const create = form.matches('[data-create-user-form]');
      const grant = form.matches('[data-grant-user-subscription]');
      const reset = form.matches('[data-reset-user-subscriptions]');
      const password = form.matches('[data-reset-user-password]');
      const permissions = form.matches('[data-update-admin-permissions]');
      if (!create && !grant && !reset && !password && !permissions) return;
      event.preventDefault();
      if (busy || !form.reportValidity()) return;
      const trigger = form.querySelector('[type="submit"]');
      if (create) {
        const body = { username: form.elements.username.value.trim(), display_name: form.elements.display_name.value.trim(), password: form.elements.password.value, role: Number(form.elements.role.value) };
        if (!body.username) { api.toast('请输入用户名', 'danger'); return; }
        if (!actor || Number(actor.role) < 10 || ![1, 10].includes(body.role) || body.role >= Number(actor.role)) { api.toast('当前角色无权创建该用户', 'danger'); return; }
        if (!validPassword(body.password)) return;
        const matrix = form.querySelector('[data-create-permissions]');
        if (body.role === 10 && (!matrix || matrix.dataset.loaded !== 'true')) { api.toast('请先加载管理员权限后再创建', 'danger'); return; }
        if (body.role === 10 && matrix && matrix.dataset.loaded === 'true') body.admin_permissions = permissionValues(matrix);
        await mutate(trigger, '', function () { return api.request('/api/user/', { method: 'POST', body: body }); }, function () { form.reset(); api.closeDialog('create-user-dialog'); });
      } else if (password || permissions) {
        if (!canManage()) return;
        const id = Number(activeUser.id);
        const body = {};
        if (password) {
          body.password = form.elements.password.value;
          if (!validPassword(body.password)) return;
        } else {
          const matrix = form.querySelector('[data-account-permissions]');
          if (Number(actor.role) !== 100 || Number(activeUser.role) < 10 || matrix.dataset.loaded !== 'true') return;
          body.admin_permissions = permissionValues(matrix);
        }
        await mutate(trigger, password ? '确认重设该用户密码并使其现有登录会话失效？' : '确认更新该管理员的操作权限？', function () { return updateAccount(id, body); }, function () {
          if (password) form.reset();
          return openActions(id);
        });
      } else {
        if (!canManage()) return;
        const id = Number(activeUser.id);
        const body = { plan_id: Number(form.elements.plan_id.value) };
        if (!body.plan_id) return;
        if (reset) body.advance_reset_time = form.elements.advance_reset_time.checked;
        await mutate(trigger, reset ? '确认清零该用户此套餐全部生效订阅的已用额度？' : '确认向该用户分配所选套餐？', function () { return api.request('/api/subscription/admin/users/' + id + '/subscriptions' + (reset ? '/reset' : ''), { method: 'POST', body: body }); }, function () { return loadSubscriptions(generation); });
      }
    });
  }
  window.ProxyLanAdminUserActions = { bind: bind };
})();
