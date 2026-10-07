(function () {
  'use strict';
  const festivals = {
    duanwu: { label: '端午', icon: 'eco', lunar: '农历五月初五', headline: '端午节', meaning: '纪念先贤 · 祈福避灾', blessing: '纪念先贤，祈福避灾。', customs: [['吃粽子', '用粽叶包裹糯米，蒸煮后食用。'], ['赛龙舟', '划龙舟竞渡，纪念先贤。'], ['挂艾草', '在门前悬挂艾草，祈求安康。']] },
    qixi: { label: '七夕', icon: 'favorite', lunar: '农历七月初七', headline: '七夕节', meaning: '爱情 · 婚姻 · 智慧', blessing: '乞巧祈愿，珍惜姻缘。', customs: [['乞巧', '穿针乞巧，祈求巧艺与智慧。'], ['祈愿', '祈求姻缘美满、家庭和睦。'], ['鹊桥相会', '七夕相传为牛郎织女鹊桥相会之日。']] },
    mid_autumn: { label: '中秋', icon: 'dark_mode', lunar: '农历八月十五', headline: '中秋节', meaning: '家庭团圆', blessing: '赏月、吃月饼、团圆宴。', customs: [['赏月', '在农历八月十五夜赏月。'], ['吃月饼', '分食月饼，寓意团圆。'], ['团圆宴', '家人团聚，共享团圆宴。']] },
    chongyang: { label: '重阳', icon: 'landscape', lunar: '农历九月初九', headline: '重阳节', meaning: '长寿 · 敬老', blessing: '登高赏菊，敬老祈寿。', customs: [['登高', '登高远眺，祈福避灾。'], ['赏菊', '观赏秋菊，饮菊花酒。'], ['敬老', '探望和陪伴长辈，祝愿健康长寿。']] },
    new_year_eve: { label: '除夕', icon: 'celebration', lunar: '农历一年最后一天', headline: '除夕', meaning: '辞旧迎新 · 阖家团聚', blessing: '辞旧迎新，阖家团聚。', customs: [['年夜饭', '家人团聚，共吃年夜饭。'], ['守岁', '除夕夜守岁，迎接新年。'], ['贴春联', '张贴春联和福字，辞旧迎新。']] },
    national_day: { label: '国庆', icon: 'flag', lunar: '公历十月一日', headline: '国庆节', meaning: '山河锦绣 · 万家同庆', blessing: '共赏锦绣山河，共度美好假期。', customs: [['升旗仪式', '以庄严的仪式，迎接共同的节日。'], ['城市漫步', '走进街巷，看见生活里的日新月异。'], ['亲友相聚', '让假日成为一次有温度的相逢。']] }
  };
  let api, page, panel, view = null, checkin = null, pending = null;
  let themePending = null, verificationToken = '', widgetID = null, widgetKey = '', turnstilePending = null;
  let checkinPending = false, activityAvailable = false, mounted = false, viewFresh = false;
  let boundaryTimer = null, queuedRefresh = null, queuedAfter = null;
  let activityViewSignature = '', renderedFestivalKey = '', renderedFestivalSignature = '', renderedPlansMarkup = '';
  let activeActivityRoute = null;
  const activityRouteTimers = new WeakMap();
  const benefitMarkupCache = new WeakMap();
  const claimsInFlight = new Set();
  const historyAnimations = new WeakMap();
  const adminSections = new Map();
  const adminTransitions = new WeakMap();
  let selectedAdminCampaign = '';
  let sceneObserver = null;
  let sceneSerial = 0, serverReceivedAt = 0;
  const festivalStories = {
    mid_autumn: { title: '把月光，\n寄给想念的人。', eyebrow: '月满人间 · 中秋雅集', note: '今夜的月亮，照见每一种团圆。', poem: '但愿人长久，千里共婵娟。', source: '苏轼 ·《水调歌头》', chapter: '月下团圆', ritual: [['备一壶好茶', '把茶水斟满，让忙碌在此刻慢下来。', 'local_cafe'], ['分一块月饼', '甜咸皆有滋味，与你分享的人才是主角。', 'cookie'], ['拨一通电话', '见面或许很远，问候可以就在今晚。', 'call']], intro: '中秋在农历八月十五。圆月寄托着人们对团聚的期盼；赏月、分食月饼，也让遥远的思念有了具体的形状。各地过节方式不同，那份牵挂始终相通。', tag: '赏月 · 团圆 · 心意' },
    duanwu: { title: '一叶粽香，\n一岁安康。', eyebrow: '五月初五 · 端午游园', note: '江上龙舟起，门前艾草青。', poem: '彩线轻缠红玉臂，小符斜挂绿云鬟。', source: '苏轼 ·《浣溪沙·端午》', chapter: '江上清风', ritual: [['亲手包一只粽', '把喜欢的馅料，藏进一片清香粽叶。', 'eco'], ['系一缕五彩线', '把平安的祝愿，轻轻系在腕间。', 'palette'], ['赴一场龙舟会', '听见鼓点，也听见人群里热烈的夏天。', 'rowing']], intro: '端午在农历五月初五，是仲夏时节的重要传统节日。龙舟竞渡、包粽子、悬挂艾草等习俗，承载着纪念先贤、祈愿安康的心意，因地域而各具特色。', tag: '龙舟 · 粽香 · 安康' },
    qixi: { title: '隔着星河，\n也要遇见你。', eyebrow: '七月初七 · 星河来信', note: '把认真生活的心意，交给今夜的星光。', poem: '金风玉露一相逢，便胜却人间无数。', source: '秦观 ·《鹊桥仙》', chapter: '星河相逢', ritual: [['写一封短信', '不必华丽，把想说的话认真写下来。', 'mail'], ['做一件手作', '从穿针乞巧到今日手作，让心意有迹可循。', 'gesture'], ['一起看星空', '留一点安静的时间，给身边的人与自己。', 'auto_awesome']], intro: '七夕在农历七月初七，也称乞巧节。穿针乞巧、祈愿巧艺是传统习俗的一部分；牛郎织女的传说，又为这一天添上相知相守的浪漫意涵。', tag: '乞巧 · 星河 · 相知' },
    chongyang: { title: '向山而行，\n与岁月重逢。', eyebrow: '九月初九 · 重阳山行', note: '山有回声，爱有回响。', poem: '遥知兄弟登高处，遍插茱萸少一人。', source: '王维 ·《九月九日忆山东兄弟》', chapter: '秋山有信', ritual: [['走一段山路', '按自己的步调登高，让秋色填满视野。', 'landscape'], ['陪长辈吃顿饭', '听一段旧事，让陪伴成为今天的礼物。', 'restaurant'], ['寻一抹菊香', '在公园散散步，看看秋天盛开的颜色。', 'local_florist']], intro: '重阳在农历九月初九。登高、赏菊、佩茱萸等传统习俗与秋日相伴；今天，它也承载着敬老、爱老的心意。一次探望、一份耐心，都是可珍惜的节日仪式。', tag: '登高 · 赏菊 · 敬老' },
    new_year_eve: { title: '灯火可亲，\n岁岁有你。', eyebrow: '岁末相聚 · 除夕家宴', note: '让旧岁的故事，在一桌热饭里圆满。', poem: '故岁今宵尽，新年明旦来。', source: '张说 ·《钦州守岁》', chapter: '灯火人间', ritual: [['准备一道家常菜', '熟悉的味道，就是回家的方向。', 'restaurant'], ['贴一副新春联', '把祝福写在门边，迎接新一年的光。', 'edit_note'], ['留一张全家福', '记下这一刻，给未来留一份温暖。', 'photo_camera']], intro: '除夕是农历一年的最后一天。年夜饭、贴春联、守岁等习俗，让辞旧迎新成为一场温暖的相聚。各地年俗不尽相同，人们对平安与团圆的期盼却相通。', tag: '家宴 · 守岁 · 迎新' },
    national_day: { title: '山河为卷，\n万家同庆。', eyebrow: '十月一日 · 山河共此时', note: '把假日交给山河，也交给身边的人。', poem: '看见山河的辽阔，也珍惜万家的灯火。', source: '国庆 · 假日寄语', chapter: '山河同庆', ritual: [['迎一次晨光', '看一场升旗仪式，认真迎接这个特别的清晨。', 'flag'], ['走进一座城市', '探访博物馆或老街，听听这片土地的故事。', 'museum'], ['留一段团聚时光', '与亲友吃顿饭，让长假也有慢下来的温度。', 'groups']], intro: '国庆节在每年公历十月一日。这一天，人们共庆中华人民共和国成立，也在城市与乡野间感受日常生活的美好。走进一座博物馆、发现身边的风景，或与家人团聚，都是属于自己的假日记忆。', tag: '山河 · 假日 · 同庆' }
  };
  const find = function (selector) { return panel && panel.querySelector(selector); };
  const escape = function (value) { return api.escapeHTML(String(value == null ? '' : value)); };
  const money = function (quota) { return api.moneyFromQuota(Number(quota || 0), 6).replace(/(\.\d{2}\d*?)0+$/, '$1'); };
  const icon = function (name) { return '<span class="material-symbol" aria-hidden="true">' + name + '</span>'; };
  const dateLabel = function (date) { return /^\d{4}-\d{2}-\d{2}$/.test(String(date)) ? date.slice(5, 7) + '月' + date.slice(8, 10) + '日' : '日期待确认'; };
  const lastEligibleDate = function (exclusiveEnd) { const end = Date.parse(exclusiveEnd); return Number.isFinite(end) ? new Date(end - 1 + 8 * 3600000).toISOString().slice(0, 10) : ''; };

  function state(message, error) {
    const target = find('[data-activity-status]');
    if (!target) return;
    target.textContent = message || '';
    target.hidden = !message;
    target.classList.toggle('is-error', Boolean(error));
    target.setAttribute('role', error ? 'alert' : 'status');
  }

  function syncBanner() {
    const banner = document.querySelector('[data-user-benefits-banner]');
    if (!banner) return;
    banner.dataset.nav = 'activities';
    const title = banner.querySelector('[data-user-benefits-banner-title]');
    if (title && title.textContent !== '活动与签到') title.textContent = '活动与签到';
    if (activityAvailable && banner.classList.contains('hidden')) banner.classList.remove('hidden');
  }

  function setSeasonTheme(theme) {
    if (window.FurtherAppearance && typeof window.FurtherAppearance.applySeason === 'function') window.FurtherAppearance.applySeason(theme);
    else if (theme) document.documentElement.dataset.holiday = theme;
    else delete document.documentElement.dataset.holiday;
  }

  function applyTheme(data) {
    const theme = data && festivals[data.theme] ? data.theme : '';
    setSeasonTheme(theme);
    if (data && data.server_date) document.documentElement.dataset.activityServerDate = data.server_date;
    activityAvailable = Boolean(data && ((data.checkin && data.checkin.enabled) || (data.campaigns || []).some(function (campaign) { return campaign.active === true; })));
    document.querySelectorAll('[data-activity-season-pill]').forEach(function (pill) { pill.remove(); });
    if (theme) {
      const host = document.querySelector('.top-app-bar') || document.querySelector('.public-header nav') || document.querySelector('.auth-header');
      if (host) {
        const pill = document.createElement('span');
        pill.className = 'activity-season-pill'; pill.dataset.activitySeasonPill = '';
        pill.innerHTML = (theme === 'mid_autumn' ? '<span class="activity-season-art" aria-hidden="true"><i class="activity-season-moon"></i><i class="activity-season-mooncake"><b>月</b></i></span>' : icon(festivals[theme].icon)) + '<span>' + festivals[theme].label + '好时光</span>';
        host.append(pill);
      }
    }
    syncBanner();
    scheduleBoundaryRefresh(data);
    window.dispatchEvent(new CustomEvent('further-ui:activity-theme', { detail: { theme: theme, label: theme ? festivals[theme].label : '', serverDate: data && data.server_date || '' } }));
  }

  function scheduleBoundaryRefresh(data) {
    if (boundaryTimer !== null) window.clearTimeout(boundaryTimer);
    boundaryTimer = null;
    const now = Date.parse(data && data.server_time);
    if (!Number.isFinite(now)) return; // The regular refresh remains available.
    const day = 86400000, offset = 8 * 3600000;
    let boundary = (Math.floor((now + offset) / day) + 1) * day - offset;
    (data.campaigns || []).forEach(function (campaign) {
      const end = Date.parse(campaign.ends_at);
      if (campaign.active === true && end > now) boundary = Math.min(boundary, end);
    });
    boundaryTimer = window.setTimeout(function () {
      // Never carry yesterday's colors or claim controls across the boundary
      // while the server is confirming the new day.
      setSeasonTheme('');
      document.querySelectorAll('[data-activity-season-pill]').forEach(function (pill) { pill.remove(); });
      viewFresh = false;
      if (panel && page === 'console') {
        panel.querySelectorAll('[data-activity-claim]').forEach(function (button) { button.disabled = true; });
        updateCheckinButton();
      }
      if (panel && page === 'console') { syncActivityControls(); panel.dataset.activityStale = 'true'; state('活动日期或领取窗口已变化，请手动刷新后继续签到。'); }
      // Expiry invalidates claims immediately; the open page never reloads itself.
      if (!panel || !panel.classList.contains('active')) refreshTheme(true);
    }, Math.max(250, boundary - now + 75));
  }

  async function refreshTheme(force) {
    if (!api) return;
    if (themePending) return force === true ? themePending.then(function () { return refreshTheme(); }) : themePending;
    themePending = api.request('/api/further/activities/theme', { _suppressAuthRedirect: true }).then(applyTheme).catch(function () {
      // A failed refresh cannot establish that a seasonal campaign is active.
      setSeasonTheme('');
      document.querySelectorAll('[data-activity-season-pill]').forEach(function (pill) { pill.remove(); });
    }).finally(function () { themePending = null; });
    return themePending;
  }

  function field(name, label, type, extra) {
    return '<label class="activity-field"><span>' + label + '</span><input class="md-input" name="' + name + '" type="' + (type || 'text') + '" ' + (extra || '') + '></label>';
  }

  function shellMarkup() {
    const intro = page === 'admin' ? '安排一年的相逢，把每一份节日心意配置妥当。' : '签到、节日活动与套餐福利';
    return '<div class="activity-status" data-activity-status role="status" hidden></div><div data-activity-center><div class="activity-page-head"><div>' + (page === 'admin' ? '<span class="activity-workspace-kicker">further-api · 岁时运营</span>' : '') + '<h1>' + (page === 'admin' ? '节日与日常' : '活动与签到') + '</h1><p>' + intro + '</p></div><div class="activity-head-actions"><span class="activity-server-date" data-activity-date>正在读取日期</span><button type="button" class="md-icon-button" data-activity-refresh aria-label="刷新活动">' + icon('refresh') + '</button></div></div>' +
      (page === 'admin' ? adminMarkup() : userMarkup()) +
      (page === 'console' ? '<section class="activity-benefits-section" data-activity-benefits-section><div class="activity-section-head"><div><h2>套餐福利</h2><p>查看和领取套餐福利。</p></div></div><div data-activity-benefits-host></div></section>' : '') + '</div><div data-activity-festival-page hidden></div>';
  }

  function userMarkup() {
    return '<div data-activity-lobby class="festival-lobby-host"></div><div class="activity-daily-grid"><section class="activity-checkin-card"><div class="activity-checkin-copy"><div class="activity-checkin-heading"><h2>每日签到</h2><span class="activity-round-icon">' + icon('event_available') + '</span></div><p data-activity-checkin-hint>读取今天的签到状态…</p><div class="activity-checkin-reward"><span>本次可得</span><b data-activity-checkin-range>—</b></div><div class="activity-checkin-meta"><div><span>累计签到</span><b data-activity-checkin-count>—</b></div><div><span>累计获得</span><b data-activity-checkin-total>—</b></div></div><div data-activity-turnstile class="activity-turnstile" hidden></div><p class="activity-verification-hint" data-activity-verification-hint hidden></p><button type="button" class="md-btn md-btn-filled activity-checkin-button" data-activity-checkin disabled>' + icon('done') + '<span data-activity-checkin-label>正在读取状态</span></button><div class="activity-checkin-result" data-activity-checkin-result role="status" aria-live="polite" hidden></div></div><div class="activity-stamp-art" aria-hidden="true"><span class="activity-stamp-ring"></span><span class="activity-stamp-disc">' + icon('event_available') + '</span><i></i><i></i><i></i></div></section><aside class="activity-note-card"><h2>签到记录</h2><p>每日签到、节日奖励、套餐福利，都收进同一个地方。</p><div class="activity-note-ticket">' + icon('redeem') + '<div><b>奖励记录</b><span>日期按 UTC+8 计算</span></div></div><div class="activity-checkin-calendar" data-activity-checkin-records></div></aside></div>' +
      '<section class="activity-holidays"><div class="festival-section-heading"><div><span>岁时长卷 · 一年的好时光</span><h2>一年六场，相逢有时。</h2><p>从初夏粽香到国庆山河、除夕灯火，每个节日都有自己的好风景。</p></div><span class="festival-collection-count">06 <small>个美好节日</small></span></div><div data-activity-campaigns><div class="activity-empty">正在展开节日长卷…</div></div></section>' +
      '<section data-activity-loan-host class="activity-loan-section"></section>' +
      '<details class="activity-history"><summary><span>' + icon('history') + '节日奖励记录</span>' + icon('expand_more') + '</summary><div data-activity-history></div></details>';
  }

  function adminMarkup() {
    return '<div class="activity-operations-summary" aria-label="活动运营概况"><div><span>' + icon('calendar_month') + '节日专场</span><b data-activity-admin-total>—</b><small>每场独立配置与保存</small></div><div><span>' + icon('celebration') + '正在进行</span><b data-activity-admin-active>—</b><small>以站点当前日期为准</small></div><div><span>' + icon('redeem') + '今日领取</span><b data-activity-admin-claims>—</b><small>所有节日活动合计</small></div><div><span>' + icon('payments') + '今日已发放</span><b data-activity-admin-awarded>—</b><small>节日签到额度 · USD</small></div></div>' +
      '<form class="activity-admin-checkin activity-checkin-control" data-activity-checkin-form novalidate><fieldset disabled><div class="activity-checkin-control-head"><span class="activity-control-icon">' + icon('event_available') + '</span><div><span class="activity-workspace-kicker">日常福利</span><h2>每日签到</h2><p>每天一次，随机额度。与节日奖励独立发放。</p></div><label class="activity-switch"><input type="checkbox" name="enabled"><span>启用签到</span></label></div><div class="activity-checkin-control-fields"><div class="activity-form-grid">' + field('min_usd', '奖励下限 · USD', 'number', 'min="0" step="0.000001" required') + field('max_usd', '奖励上限 · USD', 'number', 'min="0" step="0.000001" required') + '</div><div class="activity-form-footer"><span data-activity-checkin-save-status role="status">读取已保存配置后可编辑</span><button class="md-btn md-btn-filled" type="submit">保存签到规则</button></div></div></fieldset></form>' +
      '<section class="activity-admin-campaigns activity-campaign-manager"><div class="activity-section-head"><div><span class="activity-workspace-kicker">六场节日，一份心意</span><h2>节日活动工作台</h2><p>选择节日，在左侧配置内容，在右侧查看即时预览。</p></div><label class="activity-reference-year"><span>预览日历年份</span><select class="md-input" data-activity-reference-year aria-label="预览日历年份"></select></label></div><div class="activity-management-layout"><aside class="activity-campaign-index"><nav data-activity-admin-navigation aria-label="选择节日活动"></nav></aside><div class="activity-admin-campaign-grid activity-campaign-editor-host" data-activity-admin-campaigns><div class="activity-empty activity-editor-loading"><span class="activity-loading-orbit">' + icon('local_florist') + '</span><div><b>正在展开节日日历</b><p>读取活动内容、奖励与已保存的规则…</p></div></div></div></div></section>';
  }

  function migrateEntrances() {
    document.querySelectorAll('[data-nav="benefits"], [data-nav="checkin"], [data-nav="activity"]').forEach(function (element) { element.dataset.nav = 'activities'; });
    document.querySelectorAll('a[href="#benefits"], a[href="#checkin"], a[href="#activity"]').forEach(function (element) { element.href = '#activities'; });
    document.querySelectorAll('option[value="benefits"], option[value="checkin"], option[value="activity"]').forEach(function (option) { option.value = 'activities'; option.textContent = '活动与签到'; });
    const name = location.hash.slice(1).split('/')[0];
    if (['benefits', 'checkin', 'activity'].includes(name)) history.replaceState(null, '', '#activities');
    const preferences = typeof api.preferences === 'function' ? api.preferences() : null;
    if (preferences && ['benefits', 'checkin', 'activity'].includes(preferences.defaultPage)) api.savePreferences(Object.assign({}, preferences, { defaultPage: 'activities' }));
  }

  function moveBenefits() {
    if (page !== 'console') return;
    const host = find('[data-activity-benefits-host]');
      const loan = document.querySelector('[data-token-loan-card]');
      if (loan) find('[data-activity-loan-host]').append(loan);
      const rows = document.querySelector('[data-user-benefit-rows]');
      if (rows) host.append(rows);
      else host.innerHTML = '<div data-user-benefit-rows class="console-benefit-catalog"></div>';
      const old = document.querySelector('[data-panel="benefits"]');
      if (old) old.remove();
      const banner = document.querySelector('[data-user-benefits-banner]');
      if (banner) new MutationObserver(syncBanner).observe(banner, { childList: true, subtree: true, attributes: true, attributeFilter: ['class'] });
    syncBanner();
  }

  function renderHeader(data) {
    find('[data-activity-date]').textContent = data.server_date ? dateLabel(data.server_date) + ' · UTC+8' : '日期待确认';
    applyTheme(data);
  }

  function updateCheckinButton() {
    const button = find('[data-activity-checkin]');
    if (!button) return;
    const settings = view && view.checkin || {};
    const checked = Boolean(checkin && checkin.stats && checkin.stats.checked_in_today);
    button.disabled = !viewFresh || !settings.enabled || !checkin || checked || checkinPending || (settings.turnstile_required && !verificationToken);
    const label = button.querySelector('[data-activity-checkin-label]');
    if (label && !checkinPending) label.textContent = !viewFresh ? '请刷新签到状态' : (checked ? '今日已签到' : (!settings.enabled ? '签到暂未开启' : (!checkin ? '状态读取失败，请刷新' : (settings.turnstile_required && !verificationToken ? '请先完成验证' : '签到领取'))));
  }

  function renderCheckin() {
    const settings = view.checkin || {};
    const stats = checkin && checkin.stats || {};
    const result = find('[data-activity-checkin-result]');
    if (result && result.dataset.checkinDate && result.dataset.checkinDate !== view.server_date) { result.hidden = true; result.replaceChildren(); }
    const min = checkin && checkin.min_quota != null ? checkin.min_quota : settings.min_quota;
    const max = checkin && checkin.max_quota != null ? checkin.max_quota : settings.max_quota;
    find('[data-activity-checkin-hint]').textContent = !settings.enabled ? '签到暂未开启。' : (stats.checked_in_today ? '今日已签到。' : '每天可签到一次。');
    find('[data-activity-checkin-range]').textContent = settings.enabled ? (Number(min) === Number(max) ? money(min) : money(min) + ' – ' + money(max)) : '暂未开启';
    find('[data-activity-checkin-count]').textContent = checkin ? api.number(stats.total_checkins || 0) + ' 天' : '—';
    find('[data-activity-checkin-total]').textContent = checkin ? money(stats.total_quota || 0) : '—';
    const records = Array.isArray(stats.records) ? stats.records.slice(0, 7) : [];
    find('[data-activity-checkin-records]').innerHTML = records.length ? '<span class="activity-mini-label">最近签到</span><div>' + records.map(function (record) { return '<span title="' + escape(record.checkin_date + ' · ' + money(record.quota_awarded)) + '">' + escape(record.checkin_date.slice(8)) + '</span>'; }).join('') + '</div>' : '';
    updateCheckinButton();
  }

  // Each scene is original, local vector artwork. Layered groups keep motion
  // independent of the drawing coordinates and need no remote image service.
  function festivalArtwork(key) {
    const id = 'festival-' + (++sceneSerial);
    const palette = {
      mid_autumn: ['#101f3e', '#253e60', '#fae5ac', '#ccaa65'],
      duanwu: ['#123f39', '#24634b', '#dfdfa2', '#87b789'],
      qixi: ['#171b39', '#383464', '#fff0c7', '#9c9ad4'],
      chongyang: ['#384948', '#708078', '#ffdf9b', '#d6aa60'],
      new_year_eve: ['#511d23', '#842e30', '#ffe0a2', '#eab476'],
      national_day: ['#6a202c', '#a44338', '#ffe0a0', '#e1ae65']
    }[key] || ['#101f3e', '#253e60', '#fae5ac', '#ccaa65'];
    const gold = palette[2];
    const stars = Array.from({ length: 19 }, function (_, i) {
      const x = 30 + (i * 173) % 940, y = 24 + (i * 67) % 350;
      return '<circle cx="' + x + '" cy="' + y + '" r="' + (i % 5 === 0 ? 2 : 1) + '" fill="' + gold + '" opacity="' + (.22 + i % 4 * .15) + '"/>';
    }).join('');
    const cloud = function (x, y, scale) { return '<g transform="translate(' + x + ' ' + y + ') scale(' + scale + ')"><path d="M0 30h38q-12-18 7-25 21-5 29 17h15q-2-29 25-24 21 3 20 22h22q23 0 26 16H4m32 13h130m-110 12h72" fill="none" stroke="' + gold + '" stroke-width="1.5" opacity=".5"/></g>'; };
    const mountain = '<path d="M-70 562 90 455 154 483 250 384 355 458 454 404 531 483 655 360 781 451 868 404 1070 557V650H-70Z" fill="' + palette[1] + '" opacity=".55"/><path d="M-50 610 70 536 148 568 267 477 364 527 509 468 676 532 796 491 1030 590V660H-50Z" fill="' + palette[0] + '"/>';
    const blossom = function (x, y, scale) { return '<g transform="translate(' + x + ' ' + y + ') scale(' + scale + ')"><g fill="' + gold + '">' + [0, 60, 120].map(function (r) { return '<ellipse rx="5" ry="17" transform="rotate(' + r + ')"/>'; }).join('') + '</g><circle r="4" fill="#b67f40"/></g>'; };
    const lantern = function (x, y, size) { return '<g class="scene-lantern"><g transform="translate(' + x + ' ' + y + ') scale(' + size + ')"><path d="M0-220V-75" stroke="#d4a268" stroke-width="2"/><rect x="-23" y="-82" width="46" height="14" rx="4" fill="#e4b471"/><ellipse rx="58" ry="73" fill="#cc604a"/><ellipse rx="38" ry="73" fill="none" stroke="#f2b26f" stroke-width="2"/><ellipse rx="17" ry="73" fill="none" stroke="#f2b26f" stroke-width="1.5"/><path d="M-53-30h106M-53 30h106" stroke="#e4a064" stroke-width="1.5"/><rect x="-23" y="67" width="46" height="12" rx="3" fill="#e4b471"/><path d="M0 80v32m-10 0v38m5-38v45m5-45v48m5-48v45m5-45v38" stroke="#e0a466" stroke-width="3"/><circle r="24" fill="#a94339"/><text y="9" text-anchor="middle" fill="#ffdda1" font-size="28" font-family="serif">福</text></g></g>'; };
    const fivePointStar = function (x, y, radius, direction) {
      const points = Array.from({ length: 10 }, function (_, index) { const angle = direction + index * Math.PI / 5, r = index % 2 ? radius * .382 : radius; return (x + Math.cos(angle) * r).toFixed(3) + ',' + (y + Math.sin(angle) * r).toFixed(3); });
      return '<polygon points="' + points.join(' ') + '" fill="#ffdf77"/>';
    };
    const fireworks = function (x, y, scale) { return '<g class="scene-national-firework"><g transform="translate(' + x + ' ' + y + ') scale(' + scale + ')" stroke="#ffda88" stroke-width="2">' + Array.from({ length: 12 }, function (_, index) { return '<g transform="rotate(' + index * 30 + ')"><path d="M0-23V-47M0-57V-67"/><circle cy="-76" r="2" fill="#ffe9b0"/></g>'; }).join('') + '</g></g>'; };
    let scene = '';
    if (key === 'mid_autumn') {
      scene = '<circle class="scene-full-moon" cx="694" cy="222" r="170" fill="url(#' + id + '-moon)"/><circle cx="694" cy="222" r="188" stroke="' + gold + '" stroke-opacity=".16"/><circle cx="694" cy="222" r="208" stroke="' + gold + '" stroke-opacity=".07"/><g opacity=".12" fill="#a17d43"><ellipse cx="750" cy="155" rx="48" ry="28" transform="rotate(-25 750 155)"/><ellipse cx="613" cy="231" rx="32" ry="55"/><circle cx="716" cy="309" r="24"/></g><g class="scene-cloud-drift">' + cloud(453, 260, 1.1) + cloud(753, 352, .95) + '</g>' + mountain + '<g class="scene-water" fill="none" stroke="#819ba8" opacity=".35"><path d="M372 527q220-20 544 2M320 553q261-20 680 4M408 580q230-13 510 0M470 602h310"/></g><g transform="translate(787 447)"><path d="M-69 0Q-50-9-39-27H39Q52-8 69 0Z" fill="#0c1a30" stroke="#b8995d"/><path d="M-48-28Q-32-36-23-49H23Q35-34 49-28Z" fill="#182744" stroke="#b8995d"/><path d="M-23-49q23-27 46 0M-48 1v76m96-76v76M-33 1v76m66-76v76" stroke="#b8995d" stroke-width="4"/><path d="M-59 76H60M-39 16h78m-78 34h78" stroke="#b8995d" stroke-width="3"/><path d="M-33 1h66v64h-66Z" fill="#f4d78e" opacity=".13"/></g><path d="M1000 84q-103 23-121 83t-15 111m44-110q-25-12-54-6m12 64q-27-10-48 1m134-110q-14-28-49-29" fill="none" stroke="#665b48" stroke-width="5"/>' + blossom(897, 114, .65) + blossom(925, 126, .7) + blossom(858, 164, .55) + blossom(864, 224, .7) + blossom(816, 230, .5) + '<g class="scene-rabbit"><g transform="translate(558 534)"><ellipse cx="0" cy="7" rx="27" ry="20" fill="#fff0c9"/><circle cx="19" cy="-11" r="17" fill="#fff0c9"/><ellipse cx="15" cy="-35" rx="6" ry="23" transform="rotate(-15 15 -35)" fill="#fff0c9"/><ellipse cx="29" cy="-35" rx="6" ry="22" transform="rotate(10 29 -35)" fill="#fff0c9"/><circle cx="26" cy="-15" r="2" fill="#4b3c3a"/><circle cx="-28" cy="6" r="8" fill="#fff0c9"/></g></g><g class="scene-mooncake"><g transform="translate(931 561) rotate(-13)"><circle r="43" fill="#bc884b"/><circle r="34" fill="#d4a65f" stroke="#edca88" stroke-width="9" stroke-dasharray="5 6"/><rect x="-21" y="-21" width="42" height="42" rx="9" stroke="#9b6939"/><text y="7" text-anchor="middle" fill="#8c5e34" font-size="20" font-family="serif">月</text></g></g>';
    } else if (key === 'duanwu') {
      scene = '<circle cx="715" cy="213" r="144" fill="url(#' + id + '-moon)" opacity=".9"/>' + mountain + '<path d="M0 451Q290 396 533 467t467-14V650H0Z" fill="#357166"/><g class="scene-water" fill="none" stroke="#8fbfaf" opacity=".5">' + Array.from({length:9},function(_,i){return '<path d="M'+(220+i%3*70)+' '+(485+i*15)+'q100-14 205 0t220 0t180 0"/>';}).join('') + '</g><g class="scene-dragon-boat"><g transform="translate(600 448)"><path d="M-120 0q133 47 280-3l-27 48H-80Z" fill="#af8149" stroke="#eed391" stroke-width="3"/><path d="m151 2 10-48 39-20 32 10-26 17-22-6-5 40" fill="#9fc68d" stroke="#e0daa0" stroke-width="2"/><path d="m163-44-11-22 16 5 8-18 6 13 17-12 4 14" fill="#e5c971"/><circle cx="199" cy="-50" r="4" fill="#153d32"/><path d="M-100 10q104 28 229 0" stroke="#e8cd7f" stroke-width="3"/>' + [-64,-14,36,86].map(function(x){return '<g transform="translate('+x+' -13)"><circle cy="-15" r="9" fill="#e2c99a"/><path d="M-12-4h23l7 26H-16Z" fill="#bf6147"/><path d="m4 8-18 66" stroke="#ebd8a1" stroke-width="5"/><path d="m-14 60-7 20" stroke="#b99665" stroke-width="10"/></g>';}).join('') + '</g></g><g class="scene-mugwort" fill="none" stroke="#96bd8a" stroke-width="4"><path d="M935 642q-40-214 21-375m-26 214-49-38m47 16 48-38m-40-28-42-41m43 19 48-42m-37-28-30-43"/></g><g fill="#80a675"><path d="M881 443q-60-46-28-47 34 2 37 46M973 419q15-57 41-52 18 27-41 52M894 352q-47-43-40-62 26-8 49 65M978 328q35-66 50-46-6 36-50 46"/></g><g class="scene-zongzi"><g transform="translate(862 540) rotate(12)"><path d="M0-66Q9-80 19-64L77 43Q84 57 67 61L-62 59Q-78 57-68 37Z" fill="#71945f" stroke="#adc588" stroke-width="2"/><path d="M7-66-8 59-67 42" fill="#416e4d"/><path d="m-41-10 92 3M6-65-8 59" fill="none" stroke="#e4d3a0" stroke-width="5"/><path d="m-1-10 25-18q17 7-25 18-32-10-27-19Q-19-39-1-10" fill="none" stroke="#e4d3a0" stroke-width="3"/></g></g><g class="scene-cloud-drift">' + cloud(614, 220, 1) + '</g>';
    } else if (key === 'qixi') {
      scene = '<g class="scene-orbit"><ellipse cx="699" cy="254" rx="264" ry="170" transform="rotate(-31 699 254)" stroke="#b3a6d6" stroke-opacity=".14"/><ellipse cx="699" cy="254" rx="309" ry="198" transform="rotate(-31 699 254)" stroke="#b3a6d6" stroke-opacity=".1"/></g><path d="M357 639C406 435 877 482 768 224S675 13 857-59" stroke="#9290c8" stroke-width="86" opacity=".14"/><path class="scene-star-river" d="M357 639C406 435 877 482 768 224S675 13 857-59" stroke="#f2e6c4" stroke-width="2" stroke-dasharray="2 17" opacity=".65"/><circle cx="803" cy="139" r="74" fill="' + gold + '"/><circle cx="773" cy="117" r="73" fill="#262744"/>' + mountain + '<g fill="none" stroke="#b39cae"><path d="M361 494Q653 266 948 496" stroke-width="11"/><path d="M361 513Q653 285 948 515" stroke-width="3"/><path d="M390 477v27m37-51v26m38-48v26m40-44v26m43-40v25m44-32v25m45-28v25m45-24v25m45-20v24m45-14v25m43-8v25m40 0v25m38 5v25m36 8v25" stroke-width="3"/></g><g class="scene-magpies" fill="#c1bad7">' + [[559,265],[622,228],[684,263]].map(function(v){return '<g transform="translate('+v[0]+' '+v[1]+')"><path d="M0 0q-23-25-35-12L-6 6q5 17 14 2L37-8Q20-28 0 0Z"/><path d="m-2 8 10 14 2-18"/></g>';}).join('') + '</g><g transform="translate(585 373)"><circle cy="-25" r="10" fill="#ead2ba"/><path d="M-8-18q-10 26-25 50h57Q9 7 8-18Z" fill="#cb91a3"/><path d="M-8-31q9-18 19 2" fill="#22233b"/></g><g transform="translate(696 375)"><circle cy="-25" r="10" fill="#ead2ba"/><path d="M-8-17-18 28h38L8-17Z" fill="#869dbb"/><path d="M-10-28q10-17 21 0" fill="#22233b"/></g><g class="scene-constellation" fill="' + gold + '" stroke="' + gold + '" stroke-width="1"><path d="m470 93 69 24 41-43 51 37" fill="none" opacity=".35"/><circle cx="470" cy="93" r="4"/><circle cx="539" cy="117" r="3"/><circle cx="580" cy="74" r="5"/><circle cx="631" cy="111" r="3"/></g>';
    } else if (key === 'chongyang') {
      scene = '<circle cx="702" cy="178" r="107" fill="url(#' + id + '-moon)"/><path d="M240 497 412 241 515 322 651 204 811 389 925 292 1060 532Z" fill="#829188"/><path d="M412 241 424 387 366 418m285-214 16 192 68 26m190-130-30 146" stroke="#b4b6a0" stroke-width="3"/><path d="M233 624 501 361 622 445 761 306 1013 586V640Z" fill="#556f67"/><path d="M435 624 663 452 765 493 874 418 1040 590V650Z" fill="#304e48"/><path d="M728 615q-70-39-48-63t59-45l-1-33q22-10 45-18" fill="none" stroke="#d8c7a2" stroke-width="5"/><g class="scene-mist"><path d="M286 428q228-19 554 0M490 475q193-17 481 0" stroke="#e1debe" stroke-width="25" stroke-linecap="round" opacity=".17"/></g><g transform="translate(746 436)"><path d="M-28 0q13-5 17-14h22q5 9 17 14Z" fill="#caa974"/><path d="M-16 0v26m32-26v26m-41 0h50" stroke="#d9b882" stroke-width="3"/></g><g fill="none" stroke="#e8d4a6" stroke-width="2"><path d="M820 157q10-12 21 0 10-12 21 0M870 190q8-10 17 0 8-10 17 0"/></g><path d="M967 628q-19-110-10-166m-20 85-24-45m39 2 26-33" fill="none" stroke="#75926a" stroke-width="4"/>' + blossom(954, 464, 2.2) + blossom(916, 509, 1.3) + blossom(981, 480, 1.5) + '<g class="scene-falling-leaf"><path d="M544 229q15-30 34-12-3 25-34 12" fill="#cf9e64"/><path d="M915 347q14-27 31-11-4 23-31 11" fill="#e3bb78"/></g>';
    } else if (key === 'national_day') {
      const skyline = [[425,61,35],[474,107,44],[536,150,35],[590,87,47],[654,125,37],[711,71,45],[774,171,39],[833,97,44],[893,132,39],[951,64,54]].map(function (building) { const x = building[0], height = building[1], width = building[2]; return '<g><path d="M' + x + ' 511v-' + height + 'h' + width + 'v' + height + 'Z" fill="#75302f" stroke="#dc9c64" stroke-opacity=".45"/><path d="M' + (x + 8) + ' ' + (522 - height) + 'v' + (height - 20) + 'm10-' + (height - 20) + 'v' + (height - 20) + '" stroke="#e5b479" stroke-opacity=".35" stroke-dasharray="4 8"/></g>'; }).join('');
      // Official flag construction: 30 x 20 grid, large star at (5,5),
      // small stars at (10,2),(12,4),(12,7),(10,9), pointing to the large star.
      const flag = fivePointStar(17.5, 17.5, 10.5, -Math.PI / 2) + [[10,2],[12,4],[12,7],[10,9]].map(function (point) { return fivePointStar(point[0] * 3.5, point[1] * 3.5, 3.5, Math.atan2(5 - point[1], 5 - point[0])); }).join('');
      scene = '<circle cx="718" cy="279" r="193" fill="#cf7148" opacity=".15"/><circle cx="718" cy="279" r="211" stroke="#f3cf8e" stroke-opacity=".22"/><circle cx="718" cy="279" r="230" stroke="#f3cf8e" stroke-opacity=".09"/>' + fireworks(869, 161, 1) + fireworks(510, 252, .57) + '<path d="M286 547q155-108 338-46t376-37V650H286Z" fill="#70272b"/>' + skyline + '<path d="M590 462V112" stroke="#e7c595" stroke-width="4"/><circle cx="590" cy="109" r="5" fill="#f0d49b"/><g class="scene-national-flag"><g transform="translate(593 123) scale(1.52)"><path d="M0 0H105V70H0Z" fill="#df393b"/>' + flag + '<path d="M0 0q42 13 73 0v70q-31 13-73 0Z" fill="#fff" opacity=".04"/></g></g><g transform="translate(705 440)"><path d="M-114 27H115v58h-229Z" fill="#a54836"/><path d="M-106 18q29-12 37-35H68q10 22 39 35Z" fill="#d39b54"/><path d="M-123 28h246M-79-16h157" stroke="#f4d090" stroke-width="3"/><path d="M-80-21q18-8 26-25H53q9 18 28 25Z" fill="#c7904d"/><path d="M-55-22V17m27-39V17m28-39V17m28-39V17m27-39V17" stroke="#af503a" stroke-width="5"/><path d="M-18 85V58q18-25 36 0v27m-86 0V62q13-19 26 0v23m78 0V62q13-19 26 0v23" fill="#5e252a"/><path d="M-119 89H122M-128 97H131" stroke="#ddac77" stroke-width="4"/></g><g class="scene-national-ribbon"><path d="M282 594q135-65 281-11t337-8l110-30v53q-204 73-407 14t-321 9" fill="#c0543b"/><path d="M282 594q135-65 281-11t337-8l110-30" stroke="#edc080" stroke-width="2"/></g><g class="scene-cloud-drift">' + cloud(751, 295, .7) + cloud(470, 365, .8) + '</g>';
    } else {
      scene = '<circle cx="712" cy="274" r="186" fill="#b7503d" opacity=".24"/><circle cx="712" cy="274" r="209" stroke="#e2a369" stroke-opacity=".15"/><path d="M318 75q317 107 735-8" fill="none" stroke="#d69b68" stroke-width="2"/>' + lantern(511, 171, .55) + lantern(709, 234, 1.18) + lantern(926, 143, .72) + '<g class="scene-firework" stroke="#ffd895" stroke-width="2">' + Array.from({length:12},function(_,i){return '<g transform="translate(906 319) rotate('+(i*30)+')"><path d="M0 20v24M0 51v9"/><circle cy="69" r="2" fill="#ffd895"/></g>';}).join('') + '</g><path d="M249 490q80-30 164-9l102-27 60 21 89-26 77 14 115-25 146 55V650H249Z" fill="#471c27"/><g fill="#381c26" stroke="#b97951" stroke-width="2"><path d="M416 500h237v107H416Z"/><path d="M400 500q32-16 53-48h164q21 34 54 48Z"/><path d="M735 466h207v149H735Z"/><path d="M715 467q32-17 50-41h147q17 25 49 41Z"/></g><g fill="#d99b5d" opacity=".65"><path d="M444 521h38v32h-38Zm69 0h38v32h-38Zm69 0h38v32h-38ZM763 491h39v43h-39Zm68 0h39v43h-39Zm67 0h23v43h-23Z"/></g><path d="M330 612q276-37 671-2" fill="none" stroke="#e5aa6b" stroke-opacity=".45"/><g class="scene-cloud-drift">' + cloud(478, 330, .9) + cloud(801, 229, .7) + '</g><g transform="translate(537 566) rotate(-9)"><path d="M0-49 49 0 0 49-49 0Z" fill="#b24537" stroke="#e5b578" stroke-width="2"/><text y="14" text-anchor="middle" fill="#ffdda3" font-size="39" font-family="serif">春</text></g>';
    }
    return '<svg class="festival-landscape" viewBox="0 0 1000 650" fill="none" aria-hidden="true" focusable="false"><defs><linearGradient id="' + id + '-sky" x2="1" y2="1"><stop stop-color="' + palette[0] + '"/><stop offset="1" stop-color="' + palette[1] + '"/></linearGradient><radialGradient id="' + id + '-moon"><stop stop-color="#fff4d1"/><stop offset=".72" stop-color="' + gold + '"/><stop offset="1" stop-color="#d8b979"/></radialGradient></defs><path d="M0 0H1000V650H0Z" fill="url(#' + id + '-sky)"/><g class="scene-stars">' + stars + '</g>' + scene + '</svg>';
  }

  function syncSceneMotion() {
    if (!panel) return;
    const pageVisible = !document.hidden && panel.classList.contains('active');
    panel.querySelectorAll('.festival-hero-v2, .festival-gallery-card').forEach(function (scene) {
      scene.classList.toggle('is-scene-paused', !pageVisible || !scene.classList.contains('is-scene-visible') || Boolean(scene.closest('[hidden]')));
    });
  }

  function observeFestivalScenes() {
    if (!panel) return;
    if (!sceneObserver && typeof window.IntersectionObserver === 'function') {
      sceneObserver = new window.IntersectionObserver(function (entries) {
        entries.forEach(function (entry) { entry.target.classList.toggle('is-scene-visible', entry.isIntersecting); });
        syncSceneMotion();
      }, { threshold: .04 });
    }
    if (sceneObserver) sceneObserver.disconnect();
    panel.querySelectorAll('.festival-hero-v2, .festival-gallery-card').forEach(function (scene) {
      scene.classList.add('is-scene-paused');
      if (sceneObserver) sceneObserver.observe(scene);
      else scene.classList.add('is-scene-visible');
    });
    syncSceneMotion();
  }

  function campaignFor(key) { return view && (view.campaigns || []).find(function (row) { return row.key === key; }); }
  function festivalDateFor(key, campaign) {
    const today = String(view && view.server_date || ''), year = Number(today.slice(0, 4));
    const schedule = window.FurtherLunarSchedule && window.FurtherLunarSchedule.dates || {};
    if (campaign && campaign.active && campaign.date) return campaign.date;
    if (campaign && campaign.next_date) return campaign.next_date;
    if (key === 'national_day' && year) return String(today > year + '-10-01' ? year + 1 : year) + '-10-01';
    const date = schedule[year] && schedule[year][key];
    return date && date < today ? (schedule[year + 1] && schedule[year + 1][key] || date) : date;
  }
  function festivalStatus(key, campaign) {
    if (!viewFresh) return { label: '状态待确认', detail: '正在确认活动状态，刷新后即可查看参与资格。' };
    if (campaign && campaign.active === true) return { label: '进行中', detail: '活动期间，每个节日每天可独立签到一次。' };
    if (campaign) return { label: '即将相逢', detail: '活动尚未开始，可查看活动日期并导出日历。' };
    return { label: '节日展馆', detail: '节日内容全年开放，活动签到尚未开放。' };
  }
  function campaignWindow(campaign) {
    if (!campaign || !campaign.starts_at || !campaign.ends_at) return '活动日期待公布';
    return dateLabel(campaign.starts_at.slice(0, 10)) + ' — ' + dateLabel(lastEligibleDate(campaign.ends_at)) + ' · UTC+8';
  }
  function syncActivityControls() {
    if (!panel) return;
    updateCheckinButton();
    panel.querySelectorAll('[data-activity-claim]').forEach(function (button) {
      const key = button.dataset.activityClaim, campaign = campaignFor(key);
      const active = Boolean(viewFresh && campaign && campaign.active);
      const busy = claimsInFlight.has(key);
      const enabled = active && campaign.can_claim === true && !campaign.claimed_today && Number(campaign.reward_quota) > 0 && !busy;
      button.disabled = !enabled;
      if (button.dataset.busy === 'true') return;
      const label = busy ? '签到到账中…' : !viewFresh ? '请刷新活动状态' : !active ? '等待活动开启' : campaign.claimed_today ? '今日活动已签到' : Number(campaign.reward_quota) <= 0 ? '本场未配置签到额度' : enabled ? '活动签到 · 领取额度' : '当前暂不可签到';
      if (button.dataset.activityControlLabel !== label) { button.innerHTML = icon(campaign && campaign.claimed_today ? 'check_circle' : 'redeem') + escape(label); button.dataset.activityControlLabel = label; }
    });
  }
  function countdownMarkup(campaign, key) {
    const target = campaign && (campaign.active ? campaign.ends_at : campaign.starts_at);
    const date = festivalDateFor(key, campaign);
    return '<div class="festival-countdown"><span>' + (campaign && campaign.active ? '距本场结束' : '距' + (campaign ? '活动开启' : '节日到来')) + '</span><div data-festival-countdown="' + escape(target || (date ? date + 'T00:00:00+08:00' : '')) + '"><b>—</b></div><small>' + (campaign ? escape(campaignWindow(campaign)) : escape(date || '日历待更新')) + '</small></div>';
  }
  function updateCountdowns() {
    if (!panel || !viewFresh || !view) return;
    const server = Date.parse(view.server_time);
    if (!Number.isFinite(server)) return;
    const now = server + Math.max(0, Date.now() - serverReceivedAt);
    panel.querySelectorAll('[data-festival-countdown]').forEach(function (node) {
      const end = Date.parse(node.dataset.festivalCountdown);
      if (!Number.isFinite(end)) { node.textContent = '日期待公布'; return; }
      let seconds = Math.max(0, Math.floor((end - now) / 1000));
      if (!seconds) { node.innerHTML = '<b>今日相逢</b>'; return; }
      const days = Math.floor(seconds / 86400); seconds %= 86400;
      const hours = Math.floor(seconds / 3600); seconds %= 3600;
      const minutes = Math.floor(seconds / 60);
      node.innerHTML = '<b>' + days + '</b><em>天</em><b>' + String(hours).padStart(2, '0') + '</b><em>时</em><b>' + String(minutes).padStart(2, '0') + '</b><em>分</em><b>' + String(seconds % 60).padStart(2, '0') + '</b><em>秒</em>';
    });
  }
  function heroMarkup(key, campaign, lobby) {
    const festival = festivals[key], story = festivalStories[key], status = festivalStatus(key, campaign);
    return '<section class="festival-hero-v2' + (lobby ? ' is-lobby' : '') + '" data-festival="' + key + '"><div class="festival-stage">' + festivalArtwork(key) + '</div><div class="festival-stage-shade"></div><div class="festival-hero-v2-copy"><span class="festival-kicker">further-api · 岁时节日馆</span><span class="festival-chapter">' + story.eyebrow + '</span><h1>' + (lobby ? story.title.split('\n').join('<br>') : escape(campaign && campaign.title || story.title).split('\n').join('<br>')) + '</h1><p>' + escape(campaign && campaign.description || story.note) + '</p><div class="festival-meta-v2"><span class="festival-live' + (campaign && campaign.active ? ' is-active' : '') + '"><i></i>' + status.label + '</span><span>' + festival.lunar + '</span></div><div class="festival-hero-actions">' + (lobby ? '<a class="festival-primary-link" href="#activities/' + key + '">走进' + festival.label + '专场 ' + icon('arrow_forward') + '</a>' : '<a class="festival-primary-link" href="#festival-gift" data-festival-jump="festival-gift">' + icon('redeem') + '查看活动签到</a>') + '</div></div><div class="festival-seal" aria-hidden="true">岁<br>时<br>有<br>礼</div><div class="festival-hero-bottom"><span>' + story.tag + '</span><span>' + escape(festivalDateFor(key, campaign) || '全年开放') + ' / ' + festival.label + '</span></div></section>';
  }
  function renderFestivalPlans(campaign) {
    if (window.FurtherUIFestivalPlans) return window.FurtherUIFestivalPlans.render(campaign || {});
    return '<section class="festival-plans-section" data-festival-plans="' + escape(campaign && campaign.key || '') + '"><div class="festival-section-heading"><div><span>节日专享 · 订阅权益</span><h2>把好时光，续得更长。</h2></div><a href="#subscriptions">全部订阅套餐 ' + icon('arrow_forward') + '</a></div><div data-festival-plan-rows><p class="festival-muted">' + (campaign && campaign.subscription_plan_ids && campaign.subscription_plan_ids.length ? '正在读取节日专属套餐…' : '节日专属套餐待上架。你也可以前往订阅中心查看现有套餐。') + '</p></div></section>';
  }
  function renderFestivalPage(key) {
    const host = find('[data-activity-festival-page]'), festival = festivals[key];
    if (!festival) { host.innerHTML = '<a class="festival-back" href="#activities">' + icon('arrow_back') + '返回活动与签到</a><div class="activity-empty"><h1>暂时没有这个节日专场</h1><p>请返回活动与签到，探索其他节日。</p></div>'; return; }
    const campaign = campaignFor(key), story = festivalStories[key], status = festivalStatus(key, campaign);
    const active = Boolean(viewFresh && campaign && campaign.active);
    const canClaim = active && campaign.can_claim === true && !campaign.claimed_today && Number(campaign.reward_quota) > 0 && !claimsInFlight.has(key);
    const label = claimsInFlight.has(key) ? '签到到账中…' : !viewFresh ? '请刷新活动状态' : !active ? '等待活动开启' : campaign.claimed_today ? '今日活动已签到' : Number(campaign.reward_quota) <= 0 ? '本场未配置签到额度' : canClaim ? '活动签到 · 领取额度' : '当前暂不可签到';
    const records = (view && Array.isArray(view.history) ? view.history : []).filter(function (record) { return record.campaign_key === key; });
    const total = records.reduce(function (sum, record) { return sum + Number(record.reward_quota || 0); }, 0);
    const rules = campaign && campaign.rules || '活动期间，每个节日每天可签到一次，按 UTC+8 零点刷新。\n活动签到与每日主签到独立，可在同一天分别领取；不同节日也分别计算。\n奖励需满足活动开放时间、每日名额和预算限制，以实际领取结果为准。\n活动奖励直接计入账户额度，不占用主签到次数。';
    host.innerHTML = '<article class="festival-page festival-page-v2" data-festival="' + key + '"><nav class="festival-breadcrumb" aria-label="当前位置"><a class="festival-back" href="#activities">' + icon('arrow_back') + '活动与签到</a><span>/</span><span>' + festival.label + '专场</span><button type="button" class="md-icon-button" data-activity-refresh aria-label="刷新节日活动">' + icon('refresh') + '</button></nav>' + heroMarkup(key, campaign, false) + '<div class="festival-ribbon"><span>' + icon('event_available') + '节日签到独立领取</span><span>' + icon('event') + '独立导出日历提醒</span><span>' + icon('calendar_month') + '岁时相约 · 如期赴约</span></div>' +
      '<div class="festival-main-grid"><div class="festival-editorial"><section class="festival-introduction"><div class="festival-section-heading"><div><span>01 / ' + story.chapter + '</span><h2>让节日，回到生活里。</h2></div><span class="festival-brush-mark" aria-hidden="true">' + festival.label + '</span></div><p>' + escape(story.intro) + '</p>' + (campaign && campaign.details ? '<div class="festival-organizer-copy"><span>本场活动</span><p>' + escape(campaign.details).replaceAll('\n', '<br>') + '</p></div>' : '') + '<blockquote><span>“</span><p>' + story.poem + '</p><cite>' + story.source + '</cite></blockquote></section><section class="festival-rituals"><div class="festival-section-heading"><div><span>02 / 今日仪式</span><h2>给今天，一点特别。</h2></div><small>三件节日小事</small></div><div class="festival-ritual-grid">' + story.ritual.map(function (item, index) { return '<article class="festival-ritual-card"><span class="festival-ritual-no">0' + (index + 1) + '</span><div class="festival-ritual-symbol">' + icon(item[2]) + '</div><h3>' + item[0] + '</h3><p>' + item[1] + '</p></article>'; }).join('') + '</div></section></div>' +
      '<aside class="festival-gift-column"><section class="festival-gift" id="festival-gift"><div class="festival-gift-head"><span>专属节日礼</span>' + icon('redeem') + '</div><span class="festival-gift-eyebrow">' + festival.label + ' · 活动签到额度</span><div class="festival-gift-value">' + (campaign && Number(campaign.reward_quota) > 0 ? '<strong>' + money(campaign.reward_quota) + '</strong><span>每人 / 每日</span>' : '<strong class="is-unset">额度待开放</strong>') + '</div><p>与每日主签到独立，<br>同一天，两份好心意。</p><button type="button" class="festival-claim-v2" data-activity-claim="' + key + '"' + (!canClaim ? ' disabled' : '') + '>' + icon(campaign && campaign.claimed_today ? 'check_circle' : 'redeem') + escape(label) + '</button><small>' + (active && !canClaim && !campaign.claimed_today && Number(campaign.reward_quota) > 0 ? '领取资格、每日名额或预算暂不满足，请以活动规则为准。' : status.detail) + '</small><div class="festival-ticket-perforation"></div>' + countdownMarkup(campaign, key) + '</section><section class="festival-calendar-card"><div>' + icon('calendar_month') + '<h3>下一次相逢，先记下。</h3></div><p>把本场活动日期导入自己的日历，方便安排节日行程。提醒时间可在日历应用中设置。</p>' + '<button type="button" class="festival-calendar-export" data-activity-calendar="' + key + '"' + (!festivalDateFor(key, campaign) ? ' disabled' : '') + '>' + icon('event') + '导出日历 .ics</button><small>日期与活动窗口以页面当前配置为准。</small></section></aside></div>' +
      renderFestivalPlans(campaign) +
      '<div class="festival-detail-grid"><section class="festival-footprints"><div class="festival-section-heading"><div><span>03 / 我的节日足迹</span><h2>把每次相逢，收进日历。</h2></div></div><div class="festival-record-summary"><div><strong>' + records.length + '</strong><span>次已展示的签到</span></div><div><strong>' + money(total) + '</strong><span>这些记录累计获得</span></div></div>' + (records.length ? '<div class="festival-footprint-list">' + records.slice(0, 6).map(function (record) { return '<div><span>' + icon('verified') + escape(record.claim_date) + '</span><b>+' + money(record.reward_quota) + '</b></div>'; }).join('') + '</div>' : '<div class="festival-footprint-empty">' + icon('history_edu') + '<p>还没有' + festival.label + '活动签到记录。<br><span>活动开启后，来盖下你的第一枚节日印记。</span></p></div>') + '</section><section class="festival-rules"><div class="festival-section-heading"><div><span>04 / 参与须知</span><h2>好时光，也有小约定。</h2></div></div><details open><summary>本场活动规则 ' + icon('expand_more') + '</summary><p>' + escape(rules).replaceAll('\n', '<br>') + '</p></details><details><summary>活动签到会影响每日主签到吗？ ' + icon('expand_more') + '</summary><p>不会。活动签到使用独立的领取记录和额度配置，与每日主签到分别计算。你可以在同一天完成两种签到。</p></details><details><summary>节日套餐如何开通？ ' + icon('expand_more') + '</summary><p>选择节日专属套餐后，确认价格、权益和有效期，再按页面提示完成支付。套餐信息在「订阅与福利」中统一维护。</p></details></section></div>' +
      '<section class="festival-next-chapters"><div class="festival-section-heading"><div><span>四时流转 / 下一个好时节</span><h2>还有这些美好，值得相逢。</h2></div><a href="#activities">全部节日 ' + icon('arrow_forward') + '</a></div><div class="festival-next-grid">' + Object.keys(festivals).filter(function (other) { return other !== key; }).map(function (other) { return '<a class="festival-next-card" data-festival="' + other + '" href="#activities/' + other + '"><span>' + icon(festivals[other].icon) + '</span><div><b>' + festivals[other].label + '</b><small>' + festivals[other].lunar + '</small></div>' + icon('arrow_outward') + '</a>'; }).join('') + '</div></section><footer class="festival-footer-v2"><span>further-api · 岁时节日馆</span><span>愿每一个平常日，都有值得期待的事。</span></footer></article>';
    updateCountdowns();
    observeFestivalScenes();
  }

  function syncRoute() {
    if (page !== 'console' || !panel) return;
    const parts = location.hash.slice(1).split('/');
    const key = parts[0] === 'activities' ? parts.slice(1).join('/') : '';
    const routeChanged = activeActivityRoute !== null && activeActivityRoute !== key;
    activeActivityRoute = key;
    find('[data-activity-center]').hidden = Boolean(key);
    find('[data-activity-festival-page]').hidden = !key;
    panel.classList.toggle('is-festival-page', Boolean(key));
    panel.dataset.title = key && festivals[key] ? festivals[key].label + '专场' : '活动与签到';
    panel.dataset.subtitle = key && festivals[key] ? festivals[key].lunar + ' · ' + festivals[key].meaning : '每日签到、节日奖励与套餐福利';
    if (key && (renderedFestivalKey !== key || renderedFestivalSignature !== activityViewSignature || !find('[data-activity-festival-page]').firstElementChild)) {
      renderFestivalPage(key); renderedFestivalKey = key; renderedFestivalSignature = activityViewSignature; renderedPlansMarkup = festivals[key] ? renderFestivalPlans(campaignFor(key)) : '';
    } else if (key && festivals[key]) {
      const markup = renderFestivalPlans(campaignFor(key));
      if (markup !== renderedPlansMarkup) { const plansHost = find('[data-activity-festival-page] [data-festival-plans]'); if (plansHost) plansHost.outerHTML = markup; renderedPlansMarkup = markup; }
    }
    syncSceneMotion();
    if (panel.classList.contains('active')) {
      // Local festival routes share one shell panel. Only a real route change
      // moves its incoming body; data updates keep the current surface still.
      const parentMoving = panel.classList.contains('rf-entering') || panel.classList.contains('entering') || (typeof panel.getAnimations === 'function' && panel.getAnimations().some(function (animation) { return animation.playState === 'running' || animation.pending; }));
      if (routeChanged && !parentMoving) {
        const incoming = find(key ? '[data-activity-festival-page]' : '[data-activity-center]');
        if (typeof api.transitionVisibility === 'function') api.transitionVisibility(incoming, true, function () {}, { forceEnter: true });
        else {
          // The CSS path is only used when an older shell lacks the shared API.
          window.clearTimeout(activityRouteTimers.get(incoming));
          incoming.classList.add('rf-entering');
          activityRouteTimers.set(incoming, window.setTimeout(function () { incoming.classList.remove('rf-entering'); activityRouteTimers.delete(incoming); }, 320));
        }
      }
      const title = document.querySelector('[data-page-title]'), subtitle = document.querySelector('[data-page-subtitle]');
      if (title) title.textContent = panel.dataset.title;
      if (subtitle) subtitle.textContent = panel.dataset.subtitle;
    }
  }

  function renderUserCampaigns() {
    const campaigns = (view.campaigns || []).filter(function (campaign) { return festivals[campaign.key]; });
    const active = campaigns.filter(function (campaign) { return campaign.active === true; });
    const upcoming = campaigns.slice().sort(function (a, b) { return String(a.next_date || a.date).localeCompare(String(b.next_date || b.date)); });
    const featured = active[0] || upcoming[0];
    const featuredKey = featured ? featured.key : 'mid_autumn';
    const lobby = find('[data-activity-lobby]');
    if (lobby) lobby.innerHTML = heroMarkup(featuredKey, featured, true);
    find('[data-activity-campaigns]').innerHTML = '<div class="festival-year-gallery">' + Object.keys(festivals).map(function (key, index) {
      const festival = festivals[key], campaign = campaignFor(key), status = festivalStatus(key, campaign), date = festivalDateFor(key, campaign);
      return '<article class="festival-gallery-card" data-festival="' + key + '"><a class="festival-gallery-art" href="#activities/' + key + '" aria-label="进入' + festival.label + '专场">' + festivalArtwork(key) + '<span class="festival-gallery-order">0' + (index + 1) + '</span><span class="festival-gallery-status' + (campaign && campaign.active ? ' is-active' : '') + '">' + status.label + '</span><div><span>' + festival.lunar + '</span><h3>' + festival.label + '</h3><p>' + festivalStories[key].chapter + '</p></div><span class="festival-gallery-arrow">' + icon('arrow_outward') + '</span></a><div class="festival-gallery-info"><div><b>' + (campaign && Number(campaign.reward_quota) > 0 ? '每日签到 ' + money(campaign.reward_quota) : '节日内容全年开放') + '</b><span>' + escape(date || '日历待更新') + '</span></div>' + '</div></article>';
    }).join('') + '</div>';
    const history = Array.isArray(view.history) ? view.history : [];
    find('[data-activity-history]').innerHTML = history.length ? '<div class="activity-record-list">' + history.map(function (record) { return '<div><span><b>' + escape((festivals[record.campaign_key] || {}).label || record.campaign_key) + '</b><small>' + escape(record.claim_date) + '</small></span><strong>+' + money(record.reward_quota) + '</strong></div>'; }).join('') + '</div>' : '<p class="activity-empty-small">还没有活动签到记录，下一场节日见。</p>';
    updateCountdowns();
    observeFestivalScenes();
  }

  function exportCalendar(key) {
    const campaign = campaignFor(key), date = festivalDateFor(key, campaign);
    if (!date || !/^\d{4}-\d{2}-\d{2}$/.test(date)) return;
    const startDate = campaign && campaign.starts_at ? campaign.starts_at.slice(0, 10) : date;
    const endDate = campaign && campaign.ends_at ? campaign.ends_at.slice(0, 10) : new Date(Date.parse(date + 'T00:00:00Z') + 86400000).toISOString().slice(0, 10);
    const icsEscape = function (value) { return String(value).replace(/\\/g, '\\\\').replace(/\r?\n/g, '\\n').replace(/,/g, '\\,').replace(/;/g, '\\;'); };
    const body = ['BEGIN:VCALENDAR', 'VERSION:2.0', 'PRODID:-//further-api//Festival Calendar//ZH', 'CALSCALE:GREGORIAN', 'BEGIN:VEVENT', 'UID:' + key + '-' + startDate + '@further-api', 'DTSTAMP:' + new Date().toISOString().replace(/[-:]/g, '').replace(/\.\d{3}Z$/, 'Z'), 'DTSTART;VALUE=DATE:' + startDate.replaceAll('-', ''), 'DTEND;VALUE=DATE:' + endDate.replaceAll('-', ''), 'SUMMARY:' + icsEscape(campaign && campaign.title || festivals[key].label), 'DESCRIPTION:' + icsEscape((campaign && campaign.description || festivalStories[key].note) + '\n活动以站内实时状态为准。可在日历应用中自行设置提醒。'), 'END:VEVENT', 'END:VCALENDAR', ''].join('\r\n');
    const url = URL.createObjectURL(new Blob([body], { type: 'text/calendar;charset=utf-8' }));
    const anchor = document.createElement('a'); anchor.href = url; anchor.download = 'further-' + key + '-' + date + '.ics'; document.body.append(anchor); anchor.click(); anchor.remove(); window.setTimeout(function () { URL.revokeObjectURL(url); }, 1000);
    api.toast('已导出节日日历，可在日历应用中设置提醒');
  }

  function toggleRewardHistory(details) {
    const previous = historyAnimations.get(details);
    const opening = details.dataset.activityHistoryTarget ? details.dataset.activityHistoryTarget !== 'open' : !details.open;
    const from = details.getBoundingClientRect().height;
    if (previous) previous.cancel();
    const summary = details.querySelector('summary');
    summary.setAttribute('aria-expanded', String(opening));
    details.dataset.activityHistoryTarget = opening ? 'open' : 'closed';
    const finish = function () {
      details.open = opening;
      details.style.height = '';
      details.classList.remove('is-history-animating', 'is-history-closing');
      delete details.dataset.activityHistoryTarget;
      historyAnimations.delete(details);
    };
    if (typeof details.animate !== 'function') { finish(); return; }
    details.open = true;
    details.style.height = '';
    const to = opening ? details.getBoundingClientRect().height : summary.getBoundingClientRect().height;
    details.classList.add('is-history-animating');
    details.classList.toggle('is-history-closing', !opening);
    details.style.height = from + 'px';
    const animation = details.animate([{ height: from + 'px' }, { height: to + 'px' }], { duration: opening ? 220 : 140, easing: opening ? 'cubic-bezier(.2,.8,.2,1)' : 'cubic-bezier(.4,0,1,1)', fill: 'forwards' });
    historyAnimations.set(details, animation);
    animation.onfinish = function () { if (historyAnimations.get(details) === animation) { finish(); animation.cancel(); } };
  }

  function quotaInput(value, label) {
    if (String(value).trim() === '') throw new Error('请填写' + label);
    const amount = Number(value);
    const quota = api.quotaFromUSD(amount);
    if (!Number.isFinite(amount) || amount < 0 || !Number.isSafeInteger(quota) || quota < 0) throw new Error(label + '必须是有效的非负金额');
    return quota;
  }

  function adminCampaignMarkup(campaign) {
    const festival = festivals[campaign.key];
    if (!festival) return '';
    const key = escape(campaign.key);
    return '<form class="activity-admin-campaign activity-campaign-editor" id="activity-editor-' + key + '" data-activity-campaign-form="' + key + '" data-festival="' + key + '" novalidate hidden><fieldset><div class="activity-admin-campaign-head"><div class="activity-festival-emblem">' + icon(festival.icon) + '</div><div><span class="activity-editor-eyebrow">' + festival.meaning + '</span><h3>' + festival.label + '活动</h3><span>' + festival.lunar + ' · <b data-activity-reference-date="' + key + '">' + escape(campaign.date || '暂无参考日期') + '</b></span></div><label class="activity-switch"><input type="checkbox" name="enabled"' + (campaign.enabled ? ' checked' : '') + '><span>启用活动</span></label></div>' +
      '<div class="activity-editor-snapshot"><span class="activity-campaign-state' + (campaign.active ? ' is-active' : '') + '">' + (campaign.active ? '正在进行' : (campaign.enabled ? '等待节日' : '已停用')) + '</span><span>今日领取 <b>' + api.number(campaign.today_claims || 0) + '</b> 次</span><span>已发放 <b>' + money(campaign.today_awarded_quota || 0) + '</b></span></div>' +
      '<div class="activity-editor-body"><div class="activity-editor-main">' +
      '<nav class="activity-editor-tabs" role="tablist" aria-label="' + festival.label + '配置分类">' + [['content', 'edit_note', '活动内容'], ['rewards', 'redeem', '时间与奖励'], ['subscriptions', 'subscriptions', '节日套餐']].map(function (tab) { return '<button type="button" role="tab" id="activity-tab-' + key + '-' + tab[0] + '" aria-controls="activity-section-' + key + '-' + tab[0] + '" aria-selected="false" data-activity-editor-tab="' + tab[0] + '">' + icon(tab[1]) + '<span>' + tab[2] + '</span></button>'; }).join('') + '</nav>' +
      '<div class="activity-editor-sections"><section class="activity-editor-section" id="activity-section-' + key + '-content" role="tabpanel" aria-labelledby="activity-tab-' + key + '-content" data-activity-editor-section="content"><div class="activity-editor-section-heading"><span class="activity-section-step">01</span><div><h4>写下这场节日的故事</h4><p>标题和引言出现在活动卡片，详情与规则展示在专场内。</p></div></div>' +
      field('title', '专场主标题', 'text', 'maxlength="60" value="' + escape(campaign.title) + '" required') +
      '<small class="activity-input-counter" data-activity-count="title"></small>' +
      '<label class="activity-field"><span>专场引言</span><textarea class="md-input" name="description" maxlength="400" rows="2">' + escape(campaign.description) + '</textarea></label>' +
      '<small class="activity-input-counter" data-activity-count="description"></small>' +
      '<label class="activity-field"><span>专场活动详情</span><textarea class="md-input" name="details" maxlength="4000" rows="3" placeholder="介绍本场节日活动、参与方式或特别安排">' + escape(campaign.details || '') + '</textarea></label>' +
      '<label class="activity-field"><span>本场活动规则</span><textarea class="md-input" name="rules" maxlength="2000" rows="3" placeholder="留空展示默认规则，可按行填写活动说明">' + escape(campaign.rules || '') + '</textarea></label>' +
      '</section><section class="activity-editor-section" id="activity-section-' + key + '-rewards" role="tabpanel" aria-labelledby="activity-tab-' + key + '-rewards" data-activity-editor-section="rewards" hidden><div class="activity-editor-section-heading"><span class="activity-section-step">02</span><div><h4>安排时间，准备好节日礼物</h4><p>每个用户每天可领取一次；活动时间按北京时间计算。</p></div></div><div class="activity-editor-rule-group"><h5>' + icon('schedule') + '活动窗口</h5><div class="activity-form-grid">' +
      field('days_before', '提前开启（天）', 'number', 'min="0" max="7" step="1" value="' + Number(campaign.days_before || 0) + '" required') + field('days_after', '延后结束（天）', 'number', 'min="0" max="7" step="1" value="' + Number(campaign.days_after || 0) + '" required') +
      '</div><p class="activity-field-note">提前和延后都为 0 时，仅在节日当天开放。每项最多 7 天。</p></div><div class="activity-editor-rule-group"><h5>' + icon('redeem') + '奖励额度</h5><div class="activity-form-grid">' +
      field('reward_usd', '活动签到额度 / 每日（USD）', 'number', 'min="0" step="0.000001" value="' + api.usdFromQuota(Number(campaign.reward_quota || 0)) + '" required') + field('daily_claim_limit', '每日名额（0 为不限）', 'number', 'min="0" max="100000" step="1" value="' + Number(campaign.daily_claim_limit || 0) + '" required') +
      field('budget_usd', '每日预算（USD，0 为不限）', 'number', 'min="0" step="0.000001" value="' + api.usdFromQuota(Number(campaign.daily_budget_quota || 0)) + '" required') +
      '</div><p class="activity-field-note">奖励为 0 时只展示节日内容。名额或预算为 0 时不设置对应上限；任一上限耗尽后，停止发放。</p></div></section>' +
      '<section class="activity-editor-section" id="activity-section-' + key + '-subscriptions" role="tabpanel" aria-labelledby="activity-tab-' + key + '-subscriptions" data-activity-editor-section="subscriptions" hidden><div class="activity-editor-section-heading"><span class="activity-section-step">03</span><div><h4>让心意延续到下一次相逢</h4><p>这里关联节日套餐，价格与套餐内容在「订阅与福利」中统一维护。</p></div></div>' +
      (window.FurtherUIFestivalPlans ? window.FurtherUIFestivalPlans.adminMarkup(campaign) : '<p class="activity-field-note">套餐关联暂不可用，请检查订阅与福利模块。</p>') + '</section></div></div>' +
      '<aside class="activity-editor-preview" aria-label="节日活动即时预览"><div class="activity-preview-heading"><span>' + icon('visibility') + '用户视角</span><small>即时预览</small></div><div class="activity-preview-card"><div class="activity-preview-art" aria-hidden="true"><span>' + icon(festival.icon) + '</span><i></i><i></i><b>' + festival.label + '</b></div><span class="activity-preview-eyebrow">' + festival.lunar + '</span><h4 data-activity-preview="title"></h4><p data-activity-preview="description"></p><div class="activity-preview-reward"><span>每日节日礼物</span><strong data-activity-preview="reward"></strong></div><div class="activity-preview-claim">' + icon('redeem') + '<span data-activity-preview="claim"></span></div></div><div class="activity-preview-facts"><div><span>' + icon('date_range') + '参考活动时间</span><b data-activity-preview="window"></b></div><div><span>' + icon('groups') + '参与用户</span><b data-activity-preview="audience"></b></div><div><span>' + icon('account_balance_wallet') + '每日可发放</span><b data-activity-preview="capacity"></b></div></div><p class="activity-preview-calendar-note">日期随预览年份变化；每年按对应节日自动安排，不改变当前活动状态。</p><div class="activity-preview-readiness"><span>' + icon('task_alt') + '配置提示</span><p data-activity-preview="readiness" role="status" aria-live="polite"></p></div></aside></div>' +
      '<div class="activity-form-footer activity-editor-savebar"><span data-activity-campaign-save-status role="status" aria-live="polite">配置已同步</span><div><button type="button" class="md-btn md-btn-text" data-activity-reset="' + key + '">撤销修改</button><button class="md-btn md-btn-filled" type="submit">' + icon('check') + '保存' + festival.label + '活动</button></div></div></fieldset></form>';
  }

  function transitionAdminPanels(host, target, animate) {
    if (!host || !target) return;
    const running = adminTransitions.get(host);
    if (animate && running && running.target === target) return;
    if (running && running.animation) running.animation.cancel();
    adminTransitions.delete(host);
    delete host.dataset.activityTransition;
    host.style.minHeight = '';
    const previous = Array.from(host.children).find(function (item) { return item !== target && !item.hidden; });
    const reveal = function () { Array.from(host.children).forEach(function (item) { item.hidden = item !== target; }); };
    if (!animate || !previous || typeof previous.animate !== 'function') { reveal(); return; }
    const state = { target: target };
    adminTransitions.set(host, state);
    host.dataset.activityTransition = 'true';
    host.style.minHeight = Math.min(previous.getBoundingClientRect().height, 240) + 'px';
    state.animation = previous.animate([{ opacity: 1, transform: 'translateY(0)' }, { opacity: 0, transform: 'translateY(-6px)' }], { duration: 120, easing: 'ease-out', fill: 'forwards' });
    state.animation.onfinish = function () {
      if (adminTransitions.get(host) !== state) return;
      state.animation.cancel(); reveal();
      state.animation = target.animate([{ opacity: 0, transform: 'translateY(8px)' }, { opacity: 1, transform: 'translateY(0)' }], { duration: 220, easing: 'cubic-bezier(.2,.7,.2,1)', fill: 'both' });
      state.animation.onfinish = function () { if (adminTransitions.get(host) === state) { state.animation.cancel(); adminTransitions.delete(host); delete host.dataset.activityTransition; host.style.minHeight = ''; } };
    };
  }

  function showAdminSection(form, name, animate) {
    if (!form) return;
    const selected = ['content', 'rewards', 'subscriptions'].includes(name) ? name : 'content';
    adminSections.set(form.dataset.activityCampaignForm, selected);
    form.querySelectorAll('[data-activity-editor-tab]').forEach(function (button) { const active = button.dataset.activityEditorTab === selected; button.classList.toggle('active', active); button.setAttribute('aria-selected', String(active)); button.tabIndex = active ? 0 : -1; });
    transitionAdminPanels(form.querySelector('.activity-editor-sections'), form.querySelector('[data-activity-editor-section="' + selected + '"]'), animate);
  }

  function showAdminCampaign(key, animate) {
    const host = find('[data-activity-admin-campaigns]');
    const form = host && Array.from(host.children).find(function (item) { return item.dataset.activityCampaignForm === key; });
    if (!form) return;
    selectedAdminCampaign = key;
    panel.querySelectorAll('[data-activity-edit-campaign]').forEach(function (button) { const active = button.dataset.activityEditCampaign === key; button.classList.toggle('active', active); button.setAttribute('aria-pressed', String(active)); });
    showAdminSection(form, adminSections.get(key), false);
    transitionAdminPanels(host, form, animate);
    updateAdminPreview(form);
  }

  function updateAdminPreview(form) {
    if (!form || !form.hasAttribute('data-activity-campaign-form')) return;
    const festival = festivals[form.dataset.activityCampaignForm];
    if (!festival) return;
    const values = form.elements;
    const put = function (name, value) { const element = form.querySelector('[data-activity-preview="' + name + '"]'); if (element) element.textContent = value; };
    const reward = Number(values.reward_usd.value);
    const budget = Number(values.budget_usd.value);
    const limit = Number(values.daily_claim_limit.value);
    const before = Number(values.days_before.value), after = Number(values.days_after.value);
    const validReward = values.reward_usd.validity.valid && Number.isFinite(reward) && reward >= 0;
    const validBudget = values.budget_usd.validity.valid && Number.isFinite(budget) && budget >= 0;
    const validWindow = values.days_before.validity.valid && values.days_after.validity.valid && Number.isInteger(before) && Number.isInteger(after);
    const reference = find('[data-activity-reference-year]');
    const year = Number(reference && reference.value);
    const schedule = window.FurtherLunarSchedule;
    const date = form.dataset.activityCampaignForm === 'national_day' && year ? year + '-10-01' : schedule && schedule.dates && schedule.dates[year] && schedule.dates[year][form.dataset.activityCampaignForm];
    const shiftedDate = function (days) { return new Date(Date.parse(date + 'T00:00:00Z') + days * 86400000).toISOString().slice(0, 10).replaceAll('-', '.'); };
    put('title', values.title.value.trim() || festival.label + '好时光');
    put('description', values.description.value.trim() || festival.blessing);
    put('reward', validReward ? money(api.quotaFromUSD(reward)) : '待填写');
    put('claim', !values.enabled.checked ? '活动尚未启用' : validReward && reward === 0 ? '节日内容展示' : '领取节日礼物');
    put('window', validWindow && /^\d{4}-\d{2}-\d{2}$/.test(String(date)) ? shiftedDate(-before) + ' — ' + shiftedDate(after) + ' · ' + (before + after + 1) + ' 天' : '等待有效日历与活动窗口');
    put('audience', '全部用户');
    let capacity = '等待奖励与预算';
    if (validReward && validBudget && values.daily_claim_limit.validity.valid) {
      if (reward === 0) capacity = '仅展示内容，不发放额度';
      else {
        const quotaReward = api.quotaFromUSD(reward), quotaBudget = api.quotaFromUSD(budget);
        const maximum = Math.min(limit > 0 ? limit : Infinity, budget > 0 && quotaReward > 0 ? Math.floor(quotaBudget / quotaReward) : Infinity);
        capacity = Number.isFinite(maximum) ? '最多 ' + api.number(maximum) + ' 份 / 天' : '名额与预算均不限';
      }
    }
    put('capacity', capacity);
    let readiness = !values.enabled.checked ? '保存后保留配置；启用活动后，将在节日窗口内向用户开放。' : '已启用；保存后将按活动窗口和发放规则执行。';
    if (!values.title.value.trim()) readiness = '填写专场主标题后，再保存这场活动。';
    else if (!validWindow) readiness = '提前与延后天数必须是 0–7 之间的整数。';
    else if (!validReward || !validBudget || !values.daily_claim_limit.validity.valid) readiness = '请填写有效的奖励、预算与每日名额。';
    else if (reward > 0 && budget > 0 && api.quotaFromUSD(budget) < api.quotaFromUSD(reward)) readiness = '每日预算低于单份奖励，当前配置无法发放奖励。请调整预算或奖励额度。';
    else if (reward === 0) readiness = '这是一场内容活动：展示节日内容，提供日历导出，不发放签到额度。';
    put('readiness', readiness);
    form.querySelectorAll('[data-activity-count]').forEach(function (element) { const input = values[element.dataset.activityCount]; element.textContent = input.value.length + ' / ' + input.maxLength + ' 字'; });
    form.dataset.activityEnabled = String(values.enabled.checked);
  }

  function markAdminDraft(form) {
    if (!form) return;
    const dirty = form.dataset.activityDirty === 'true';
    const status = form.querySelector('[data-activity-campaign-save-status], [data-activity-checkin-save-status]');
    if (status) status.textContent = dirty ? '有修改，尚未保存' : '配置已同步';
    const reset = form.querySelector('[data-activity-reset]');
    if (reset) reset.disabled = !dirty;
    const key = form.dataset.activityCampaignForm;
    if (key) panel.querySelectorAll('[data-activity-nav-dirty="' + key + '"]').forEach(function (item) { item.hidden = !dirty; });
    updateAdminPreview(form);
  }

  function updateReferenceDates() {
    const reference = find('[data-activity-reference-year]');
    if (!reference) return;
    const year = Number(reference.value);
    const schedule = window.FurtherLunarSchedule;
    const dates = schedule && schedule.dates && schedule.dates[year];
    panel.querySelectorAll('[data-activity-reference-date]').forEach(function (element) { element.textContent = element.dataset.activityReferenceDate === 'national_day' ? year + '-10-01' : (dates && dates[element.dataset.activityReferenceDate] || '暂无该年份日期'); });
    panel.querySelectorAll('[data-activity-campaign-form]').forEach(updateAdminPreview);
  }

  function renderAdmin() {
    const form = find('[data-activity-checkin-form]');
    const settings = view.checkin || {};
    if (form.dataset.activityDirty !== 'true') {
      form.elements.enabled.checked = Boolean(settings.enabled);
      form.elements.min_usd.value = api.usdFromQuota(Number(settings.min_quota || 0));
      form.elements.max_usd.value = api.usdFromQuota(Number(settings.max_quota || 0));
    }
    form.querySelector('fieldset').disabled = false;
    markAdminDraft(form);
    const campaigns = (view.campaigns || []).filter(function (campaign) { return festivals[campaign.key]; });
    const metrics = { total: campaigns.length, active: campaigns.filter(function (campaign) { return campaign.active; }).length, claims: campaigns.reduce(function (sum, campaign) { return sum + Number(campaign.today_claims || 0); }, 0), awarded: campaigns.reduce(function (sum, campaign) { return sum + Number(campaign.today_awarded_quota || 0); }, 0) };
    Object.keys(metrics).forEach(function (key) { const node = find('[data-activity-admin-' + key + ']'); if (node) node.textContent = key === 'awarded' ? money(metrics[key]) : api.number(metrics[key]); });
    const reference = find('[data-activity-reference-year]');
    const currentYear = Number(String(view.server_date).slice(0, 4));
    const selected = Number(reference.value) || currentYear;
    const schedule = window.FurtherLunarSchedule;
    const min = Number(view.calendar_min_year || (schedule && schedule.minYear));
    const max = Number(view.calendar_max_year || (schedule && schedule.maxYear));
    reference.replaceChildren();
    if (Number.isInteger(min) && Number.isInteger(max) && max >= min && max - min < 100) {
      for (let year = min; year <= max; year++) { const option = document.createElement('option'); option.value = String(year); option.textContent = year + ' 年'; reference.append(option); }
      reference.value = String(selected >= min && selected <= max ? selected : min);
    }
    const host = find('[data-activity-admin-campaigns]');
    const dirty = new Map(Array.from(host.querySelectorAll('[data-activity-campaign-form][data-activity-dirty="true"]')).map(function (item) { return [item.dataset.activityCampaignForm, item]; }));
    const incoming = document.createElement('div');
    incoming.innerHTML = campaigns.map(adminCampaignMarkup).join('') || '<div class="activity-empty">没有可配置的节日活动。</div>';
    host.replaceChildren.apply(host, Array.from(incoming.children).map(function (item) { return dirty.get(item.dataset.activityCampaignForm) || item; }));
    find('[data-activity-admin-navigation]').innerHTML = campaigns.map(function (campaign) { const festival = festivals[campaign.key]; return '<button type="button" class="activity-campaign-choice" data-festival="' + escape(campaign.key) + '" data-activity-edit-campaign="' + escape(campaign.key) + '" aria-controls="activity-editor-' + escape(campaign.key) + '" aria-pressed="false"><span class="activity-campaign-choice-icon">' + icon(festival.icon) + '</span><span class="activity-campaign-choice-copy"><b>' + festival.label + '<i data-activity-nav-dirty="' + escape(campaign.key) + '" aria-label="有未保存修改" hidden></i></b><small data-activity-reference-date="' + escape(campaign.key) + '">' + escape(campaign.date || festival.lunar) + '</small></span><span class="activity-campaign-choice-state' + (campaign.active ? ' is-active' : '') + '">' + (campaign.active ? '进行中' : campaign.enabled ? '已启用' : '未启用') + '</span></button>'; }).join('');
    host.querySelectorAll('[data-activity-campaign-form]').forEach(markAdminDraft);
    if (!campaigns.some(function (campaign) { return campaign.key === selectedAdminCampaign; })) selectedAdminCampaign = (campaigns.find(function (campaign) { return campaign.active; }) || campaigns[0] || {}).key || '';
    showAdminCampaign(selectedAdminCampaign, false);
    if (schedule) updateReferenceDates();
  }

  async function loadTurnstile() {
    if (window.turnstile && typeof window.turnstile.render === 'function') return window.turnstile;
    if (turnstilePending) return turnstilePending;
    turnstilePending = new Promise(function (resolve, reject) {
      const existing = document.querySelector('script[data-activity-turnstile-script]');
      const script = existing || document.createElement('script');
      const timer = window.setTimeout(function () { reject(new Error('人机验证加载超时，请刷新活动后重试。')); }, 20000);
      function finish() { window.clearTimeout(timer); if (window.turnstile) resolve(window.turnstile); else reject(new Error('人机验证不可用，请稍后重试。')); }
      script.addEventListener('load', finish, { once: true });
      script.addEventListener('error', function () { window.clearTimeout(timer); if (!existing) script.remove(); reject(new Error('人机验证加载失败，请检查网络后刷新。')); }, { once: true });
      if (!existing) { script.src = 'https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit'; script.async = true; script.defer = true; script.dataset.activityTurnstileScript = ''; document.head.append(script); }
    }).catch(function (error) { turnstilePending = null; throw error; });
    return turnstilePending;
  }

  async function ensureVerification() {
    const settings = view && view.checkin || {};
    const container = find('[data-activity-turnstile]');
    const hint = find('[data-activity-verification-hint]');
    const required = settings.enabled && settings.turnstile_required && !(checkin && checkin.stats && checkin.stats.checked_in_today);
    container.hidden = !required; hint.hidden = !required;
    if (!required) return;
    hint.textContent = '完成验证后即可签到领取。';
    if (!settings.turnstile_site_key) { hint.textContent = '站点尚未配置人机验证，请联系管理员。'; verificationToken = ''; updateCheckinButton(); return; }
    try {
      const turnstile = await loadTurnstile();
      if (widgetID !== null && widgetKey === settings.turnstile_site_key) return;
      if (widgetID !== null) turnstile.remove(widgetID);
      widgetKey = settings.turnstile_site_key;
      verificationToken = '';
      widgetID = turnstile.render(container, {
        sitekey: widgetKey, theme: 'auto', size: 'flexible',
        callback: function (token) { verificationToken = token; hint.textContent = '验证已完成。'; updateCheckinButton(); },
        'expired-callback': function () { verificationToken = ''; hint.textContent = '验证已过期，请重新完成验证。'; updateCheckinButton(); },
        'error-callback': function () { verificationToken = ''; hint.textContent = '验证未完成，请重试。'; updateCheckinButton(); }
      });
    } catch (error) { hint.textContent = error.message; verificationToken = ''; updateCheckinButton(); }
  }

  async function refreshBalance() {
    try {
      const self = await api.request('/api/user/self');
      api.setText('[data-user-quota], [data-card-quota]', money(self.quota));
      window.dispatchEvent(new CustomEvent('further:account-updated', { detail: { user: self } }));
    } catch (_) { /* Award is already confirmed; leave balance to the next account refresh. */ }
  }

  async function doCheckin(button) {
    const settings = view && view.checkin || {};
    if (!viewFresh || !settings.enabled || !checkin || checkinPending || (checkin.stats && checkin.stats.checked_in_today)) return;
    if (settings.turnstile_required && !verificationToken) { state('请先完成人机验证。', true); return; }
    checkinPending = true; api.showBusy(button, true); updateCheckinButton();
    try {
      const suffix = settings.turnstile_required ? '?turnstile=' + encodeURIComponent(verificationToken) : '';
      const result = await api.request('/api/user/checkin' + suffix, { method: 'POST' });
      api.toast('签到成功，获得 ' + money(result.quota_awarded));
      const feedback = find('[data-activity-checkin-result]');
      if (feedback) {
        feedback.dataset.checkinDate = result.checkin_date || view.server_date || '';
        feedback.innerHTML = '<span class="activity-checkin-success-icon">' + icon('check_circle') + '</span><span><b>今日签到成功</b><small>实际到账 <strong>+' + escape(money(result.quota_awarded)) + '</strong></small></span><span class="activity-checkin-sparks" aria-hidden="true"><i></i><i></i><i></i><i></i><i></i><i></i></span>';
        feedback.hidden = false;
        const card = feedback.closest('.activity-checkin-card');
        card.classList.remove('is-checkin-success');
        void card.offsetWidth;
        card.classList.add('is-checkin-success');
      }
      verificationToken = '';
      if (widgetID !== null && window.turnstile) window.turnstile.reset(widgetID);
      await Promise.all([load(true), refreshBalance()]);
    } catch (error) {
      state(error.message, true); api.toast(error.message, 'danger');
      verificationToken = '';
      if (widgetID !== null && window.turnstile) window.turnstile.reset(widgetID);
    } finally { checkinPending = false; api.showBusy(button, false); updateCheckinButton(); }
  }

  async function claimCampaign(key, button) {
    const campaign = view && (view.campaigns || []).find(function (item) { return item.key === key; });
    if (!viewFresh || !campaign || campaign.active !== true || !campaign.can_claim || campaign.claimed_today || Number(campaign.reward_quota) <= 0 || claimsInFlight.has(key) || button.dataset.busy === 'true') return;
    claimsInFlight.add(key);
    api.showBusy(button, true);
    try {
      const result = await api.request('/api/user/activities/claim', { method: 'POST', body: { key: key } });
      api.toast('节日奖励已到账，获得 ' + money(result.reward_quota));
      await Promise.all([load(true), refreshBalance()]);
    } catch (error) { state(error.message, true); api.toast(error.message, 'danger'); }
    finally {
      claimsInFlight.delete(key);
      if (button.isConnected) api.showBusy(button, false);
      syncActivityControls();
    }
  }

  function benefitSummary(item) {
    const parts = [];
    if (Number(item.permanent_quota) > 0) parts.push('永久 ' + money(item.permanent_quota));
    if (Number(item.timed_quota) > 0) parts.push('限时 ' + money(item.timed_quota));
    if (Number(item.restore_quota_percent) > 0) parts.push('恢复套餐额度 ' + Number(item.restore_quota_percent) + '%');
    if (Number(item.extend_subscription_days) > 0) parts.push('展期 ' + Number(item.extend_subscription_days) + ' 天');
    if (item.reset_subscription_quota) parts.push('重置套餐额度');
    return parts.join(' · ');
  }

  async function loadBenefits() {
    if (page === 'admin') return;
    const body = document.querySelector('[data-user-benefit-rows]');
    if (!body) return;
    try {
      const result = await api.request('/api/user/benefits');
      const rows = Array.isArray(result && result.items) ? result.items : [];
      const markup = rows.length ? rows.map(function (item) {
        let actions = '';
        if (item.can_claim) actions += '<button class="md-btn md-btn-filled" type="button" data-activity-benefit-claim="' + Number(item.id) + '">领取福利</button>';
        else actions += '<span class="tag">' + ({ 1: '待领取', 2: '已领取', 4: '已撤回' }[Number(item.status)] || '已失效') + '</span>';
        if (Number(item.status) === 2) actions += '<button class="md-btn md-btn-text" type="button" data-activity-benefit-dismiss="' + Number(item.id) + '">忽略记录</button>';
        return '<article class="activity-benefit-row">' + icon('redeem') + '<div class="activity-benefit-copy"><h3>' + escape(item.name) + '</h3><p>' + escape(item.plan_name || '') + (item.description ? ' · ' + escape(item.description) : '') + '</p><small>' + escape(benefitSummary(item)) + '</small>' + (item.claim_expires_at ? '<small>可领取至 ' + escape(api.date(item.claim_expires_at)) + '</small>' : '') + '</div><div class="activity-benefit-actions">' + actions + '</div></article>';
      }).join('') : '<div class="activity-empty-small">当前没有套餐福利。</div>';
      if (benefitMarkupCache.get(body) !== markup) { body.innerHTML = markup; benefitMarkupCache.set(body, markup); }
    } catch (error) { benefitMarkupCache.delete(body); body.innerHTML = '<div class="activity-section-error" role="alert">套餐福利加载失败：' + escape(error.message) + '</div>'; }
  }

  async function benefitAction(action, id, button) {
    if (button.dataset.busy === 'true') return;
    api.showBusy(button, true);
    try {
      const paths = { claim: '/api/user/benefits/' + id + '/claim', dismiss: '/api/user/benefits/' + id };
      await api.request(paths[action], { method: action === 'claim' ? 'POST' : 'DELETE' });
      api.toast({ claim: '套餐福利已领取', dismiss: '已忽略这条记录' }[action]);
      await loadBenefits();
      if (action === 'claim') await refreshBalance();
    } catch (error) { state(error.message, true); api.toast(error.message, 'danger'); }
    finally { if (button.isConnected) api.showBusy(button, false); }
  }

  async function loadLoan() {
    const card = document.querySelector('[data-token-loan-card]');
    if (!card) return;
    const oldError = card.querySelector('[data-activity-loan-error]'); if (oldError) oldError.remove();
    try {
      const status = await api.request('/api/user/public-benefits/token-loan');
      card.classList.toggle('hidden', !status.visible);
      if (!status.visible) return;
      const loan = status.loan || {};
      const active = Number(loan.status) === 1;
      const hint = card.querySelector('.card-hint');
      if (hint) hint.textContent = '按实际签到额度还款，足额即结清，第 4 次必定结清；溢出额度会增加下一笔借款上限。';
      api.setText('[data-token-loan-max]', money(status.max_borrow_quota));
      api.setText('[data-token-loan-remaining]', active ? money(loan.remaining_quota) : '已还清');
      api.setText('[data-token-loan-progress]', active ? Number(loan.checkin_count || 0) + ' / 4 天' : (status.borrow_paused ? '亏损池暂停' : '可再次借款'));
      api.setText('[data-token-loan-bonus]', Number(status.temporary_bonus_quota) > 0 ? '+' + money(status.temporary_bonus_quota) : '暂无');
      const form = card.querySelector('[data-token-loan-borrow-form]');
      if (form) { form.classList.toggle('hidden', !status.can_borrow); form.dataset.maxBorrowQuota = String(status.max_borrow_quota || 0); const max = Math.floor(api.usdFromQuota(Number(status.max_borrow_quota || 0)) * 100) / 100; form.elements.borrow_usd.max = max.toFixed(2); if (Number(form.elements.borrow_usd.value) > max) form.elements.borrow_usd.value = max.toFixed(2); }
      card.querySelector('[data-token-loan-active]').classList.toggle('hidden', !active);
      const button = card.querySelector('[data-token-loan-checkin]');
      button.disabled = !status.can_checkin; button.textContent = status.checked_in_today ? '今日已签到' : '今日签到还款';
      api.setText('[data-token-loan-status]', status.checked_in_today ? '今日已签到' : (active ? '今日可签到还款' : (status.enabled ? '暂无借款' : '词元贷已关闭')));
      const pool = status.loss_pool || {};
      const poolBox = card.querySelector('[data-token-loan-loss-pool]');
      if (poolBox) poolBox.classList.toggle('hidden', pool.visible === false || Number(pool.total_quota || 0) <= 0);
      api.setText('[data-token-loan-loss-pool-title]', pool.title || '帮助站长砍一刀');
      api.setText('[data-token-loan-loss-pool-status]', pool.description || (pool.active ? '今日已砍 ' + Number(pool.user_cuts_today || 0) + ' 次；每天最多 ' + Number(pool.daily_cut_limit || 0) + ' 次。' : '亏损池已清零，公益贷恢复发放。'));
      api.setText('[data-token-loan-loss-pool-remaining]', money(pool.remaining_quota)); api.setText('[data-token-loan-loss-pool-total]', money(pool.total_quota));
      const progress = Math.max(0, Math.min(100, Number(pool.progress_percent || 0)));
      api.setText('[data-token-loan-loss-pool-percent]', progress.toFixed(2) + '%');
      const track = card.querySelector('[data-token-loan-loss-pool-progress]');
      if (track) { track.setAttribute('aria-valuenow', String(progress)); track.querySelector('span').style.width = progress + '%'; }
      const cut = card.querySelector('[data-token-loan-loss-pool-cut]');
      if (cut) { cut.disabled = !pool.can_cut; cut.textContent = pool.active ? (pool.can_cut ? '帮站长砍一刀' : '今日次数已用完') : '已经砍完'; }
      const records = Array.isArray(status.checkins) ? status.checkins : [];
      card.querySelector('[data-token-loan-history]').innerHTML = records.map(function (record) { return '<div class="token-loan-history-row"><span>' + escape(record.checkin_date) + '</span><b>签到 ' + money(record.awarded_quota || record.repayment_quota) + '</b><small>还款 ' + money(record.repayment_quota) + ' · 剩余 ' + money(record.remaining_after) + '</small></div>'; }).join('');
    } catch (error) { card.classList.remove('hidden'); const notice = document.createElement('p'); notice.className = 'activity-section-error'; notice.dataset.activityLoanError = ''; notice.setAttribute('role', 'alert'); notice.textContent = '词元贷加载失败：' + error.message; card.prepend(notice); card.querySelectorAll('button').forEach(function (button) { button.disabled = true; }); }
  }

  async function load(force) {
    if (!panel) return;
    if (pending) {
      if (force !== true) return queuedRefresh || pending;
      // Coalesce force calls queued behind the same read. A mutation finishing
      // during the next read queues another one, so no earlier snapshot wins.
      if (!queuedRefresh || queuedAfter !== pending) {
        queuedAfter = pending;
        const next = pending.then(function () { return load(true); });
        queuedRefresh = next;
        next.finally(function () { if (queuedRefresh === next) { queuedRefresh = null; queuedAfter = null; } });
      }
      return queuedRefresh;
    }
    // Navigating back to the page reuses its snapshot. Only an explicit refresh
    // or a completed mutation asks for new data, even after a long absence.
    if (view && force !== true) { syncRoute(); syncActivityControls(); return view; }
    const initial = !view;
    if (initial) state('正在读取活动与签到…');
    panel.setAttribute('aria-busy', 'true');
    panel.querySelectorAll('[data-activity-refresh]').forEach(function (button) { button.disabled = true; });
    pending = (async function () {
      if (page === 'admin') {
        const self = await api.request('/api/user/self');
        if (Number(self.role) < 100) { panel.querySelectorAll('fieldset').forEach(function (fieldset) { fieldset.disabled = true; }); throw new Error('仅超级管理员可以配置活动与签到。'); }
      }
      const nextView = await api.request(page === 'admin' ? '/api/admin/activities' : '/api/user/activities');
      const receivedAt = Date.now();
      let nextCheckin = checkin, checkinError = '';
      const jobs = [];
      if (window.FurtherUIFestivalPlans) jobs.push(window.FurtherUIFestivalPlans.load());
      if (page === 'console') {
        if (nextView.checkin && nextView.checkin.enabled) jobs.push(api.request('/api/user/checkin').then(function (result) { nextCheckin = result; }).catch(function (error) { nextCheckin = null; checkinError = error.message; }));
        else nextCheckin = { enabled: false, stats: {} };
      }
      await Promise.all(jobs);
      const signature = JSON.stringify([nextView.server_date, nextView.theme, nextView.campaigns, nextView.history, nextView.checkin, nextView.calendar_min_year, nextView.calendar_max_year]);
      const changed = signature !== activityViewSignature;
      view = nextView; checkin = nextCheckin; serverReceivedAt = receivedAt;
      activityViewSignature = signature; viewFresh = true;
      delete panel.dataset.activityStale;
      renderHeader(view);
      if (page === 'admin') { if (initial || changed || force === true) renderAdmin(); }
      else {
        if (initial || changed) renderUserCampaigns();
        syncRoute(); syncActivityControls(); renderCheckin(); ensureVerification();
      }
      state(checkinError ? '签到状态读取失败：' + checkinError : '', Boolean(checkinError));
      await Promise.all([loadBenefits(), page === 'console' ? loadLoan() : Promise.resolve()]);
    })().catch(function (error) {
      viewFresh = false; panel.dataset.activityStale = 'true';
      syncActivityControls();
      if (!view) { applyTheme(null); syncRoute(); }
      state((view ? '当前显示上次读取的内容，领取已暂停。请手动刷新：' : '加载失败：') + error.message, true);
    }).finally(function () { pending = null; panel.removeAttribute('aria-busy'); panel.querySelectorAll('[data-activity-refresh]').forEach(function (button) { button.disabled = false; }); });
    return pending;
  }

  async function saveForm(form, button) {
    if (button.dataset.busy === 'true') return;
    if (!form.checkValidity()) {
      const invalid = form.querySelector('input:invalid, select:invalid, textarea:invalid');
      const section = invalid && invalid.closest('[data-activity-editor-section]');
      if (section) showAdminSection(form, section.dataset.activityEditorSection, false);
      if (invalid) { invalid.focus(); invalid.reportValidity(); }
      return;
    }
    const status = form.querySelector('[data-activity-campaign-save-status], [data-activity-checkin-save-status]');
    status.textContent = '';
    const editVersion = form.dataset.activityEditVersion || '0';
    api.showBusy(button, true);
    try {
      let body, endpoint;
      if (form.hasAttribute('data-activity-checkin-form')) {
        body = { enabled: form.elements.enabled.checked, min_quota: quotaInput(form.elements.min_usd.value, '最低奖励'), max_quota: quotaInput(form.elements.max_usd.value, '最高奖励') };
        if (body.max_quota < body.min_quota) throw new Error('最高奖励不能低于最低奖励');
        endpoint = '/api/admin/activities/checkin';
      } else {
        body = { key: form.dataset.activityCampaignForm, title: form.elements.title.value.trim(), description: form.elements.description.value.trim(), details: form.elements.details.value.trim(), rules: form.elements.rules.value.trim(), subscription_plan_ids: window.FurtherUIFestivalPlans ? window.FurtherUIFestivalPlans.selectedIDs(form) : [], enabled: form.elements.enabled.checked, days_before: Number(form.elements.days_before.value), days_after: Number(form.elements.days_after.value), reward_quota: quotaInput(form.elements.reward_usd.value, '每日奖励'), daily_claim_limit: Number(form.elements.daily_claim_limit.value), daily_budget_quota: quotaInput(form.elements.budget_usd.value, '每日预算'), target_group: '' };
        if (!body.title || !Number.isInteger(body.days_before) || body.days_before < 0 || body.days_before > 7 || !Number.isInteger(body.days_after) || body.days_after < 0 || body.days_after > 7 || !Number.isInteger(body.daily_claim_limit) || body.daily_claim_limit < 0 || body.daily_claim_limit > 100000) throw new Error('请填写标题、0–7 天的活动窗口及有效的每日名额');
        endpoint = '/api/admin/activities';
      }
      await api.request(endpoint, { method: 'PUT', body: body });
      if ((form.dataset.activityEditVersion || '0') === editVersion) delete form.dataset.activityDirty;
      status.textContent = '已保存'; api.toast('活动配置已保存');
      await load(true);
    } catch (error) { status.textContent = error.message; state(error.message, true); api.toast(error.message, 'danger'); }
    finally { api.showBusy(button, false); }
  }

  function mount() {
    if (mounted) return;
    api = window.FurtherUI;
    if (!api) return;
    mounted = true; refreshTheme();
    window.setInterval(function () { if (!document.hidden && panel && panel.classList.contains('active')) updateCountdowns(); }, 1000);
    window.setInterval(function () { if (!document.hidden && (!panel || !panel.classList.contains('active'))) refreshTheme(); }, 60000);
    window.addEventListener('focus', function () { if (!panel || !panel.classList.contains('active')) refreshTheme(); });
    document.addEventListener('visibilitychange', function () { if (!document.hidden && (!panel || !panel.classList.contains('active'))) refreshTheme(); });
    document.addEventListener('visibilitychange', syncSceneMotion);
    window.addEventListener('further-ui:page', syncSceneMotion);
    page = document.body.dataset.page;
    if (page !== 'admin' && page !== 'console') return;
    const rail = document.querySelector('.nav-rail'), area = document.querySelector('.content-area');
    if (!rail || !area) return;
    migrateEntrances();
    let nav = rail.querySelector('[data-nav="activities"]');
    rail.querySelectorAll('[data-nav="activities"]').forEach(function (item) { if (item !== nav) item.remove(); });
    if (!nav) { nav = document.createElement('button'); nav.type = 'button'; nav.className = 'nav-item'; nav.dataset.nav = 'activities'; rail.insertBefore(nav, rail.querySelector('[data-nav="profile"]') || rail.querySelector('.rail-bottom')); }
    nav.innerHTML = icon('redeem') + '活动与签到';
    if (page === 'admin') nav.dataset.rootOnly = '';
    panel = document.createElement('section'); panel.className = 'view activities-workspace'; panel.dataset.panel = 'activities'; panel.dataset.title = '活动与签到'; panel.dataset.subtitle = '每日签到、节日奖励与套餐福利'; panel.innerHTML = shellMarkup(); area.append(panel);
    moveBenefits();
    panel.addEventListener('pointerover', function (event) { const card = event.target.closest('.festival-gallery-card'); if (card && event.pointerType !== 'touch' && (!event.relatedTarget || !card.contains(event.relatedTarget))) card.classList.add('is-scene-engaged'); });
    panel.addEventListener('pointerout', function (event) { const card = event.target.closest('.festival-gallery-card'); if (card && (!event.relatedTarget || !card.contains(event.relatedTarget)) && !card.contains(document.activeElement)) card.classList.remove('is-scene-engaged'); });
    panel.addEventListener('focusin', function (event) { const card = event.target.closest('.festival-gallery-card'); if (card) card.classList.add('is-scene-engaged'); });
    panel.addEventListener('focusout', function (event) { const card = event.target.closest('.festival-gallery-card'); if (card && (!event.relatedTarget || !card.contains(event.relatedTarget)) && !card.matches(':hover')) card.classList.remove('is-scene-engaged'); });
    panel.addEventListener('click', function (event) {
      const historySummary = event.target.closest('.activity-history > summary');
      if (historySummary && panel.contains(historySummary)) { event.preventDefault(); toggleRewardHistory(historySummary.parentElement); return; }
      const jump = event.target.closest('[data-festival-jump]'); if (jump) { event.preventDefault(); const target = find('#' + jump.dataset.festivalJump); if (target) target.scrollIntoView({ behavior: 'smooth', block: 'center' }); return; }
      const button = event.target.closest('button'); if (!button || !panel.contains(button)) return;
      if (button.hasAttribute('data-activity-refresh')) load(true);
      if (button.hasAttribute('data-activity-edit-campaign')) showAdminCampaign(button.dataset.activityEditCampaign, true);
      if (button.hasAttribute('data-activity-editor-tab')) showAdminSection(button.closest('[data-activity-campaign-form]'), button.dataset.activityEditorTab, true);
      if (button.hasAttribute('data-activity-reset')) {
        const campaign = (view && view.campaigns || []).find(function (item) { return item.key === button.dataset.activityReset; });
        const current = button.closest('[data-activity-campaign-form]');
        if (campaign && current) { const replacement = document.createElement('div'); replacement.innerHTML = adminCampaignMarkup(campaign); current.replaceWith(replacement.firstElementChild); showAdminCampaign(campaign.key, false); markAdminDraft(find('[data-activity-campaign-form="' + campaign.key + '"]')); updateReferenceDates(); api.toast('已恢复' + festivals[campaign.key].label + '活动的已保存配置'); }
      }
      if (button.hasAttribute('data-activity-checkin')) doCheckin(button);
      if (button.hasAttribute('data-activity-calendar')) exportCalendar(button.dataset.activityCalendar);
      if (button.hasAttribute('data-activity-claim')) claimCampaign(button.dataset.activityClaim, button);
      ['claim', 'dismiss'].forEach(function (action) { const attribute = 'data-activity-benefit-' + action; if (button.hasAttribute(attribute)) benefitAction(action, Number(button.getAttribute(attribute)), button); });
    });
    panel.addEventListener('submit', function (event) { const form = event.target; if (!form.matches('[data-activity-checkin-form], [data-activity-campaign-form]')) return; event.preventDefault(); saveForm(form, event.submitter || form.querySelector('button[type="submit"]')); });
    panel.addEventListener('input', function (event) { const form = event.target.closest('form'); if (form && form.matches('[data-activity-checkin-form], [data-activity-campaign-form]')) { form.dataset.activityDirty = 'true'; form.dataset.activityEditVersion = String(Number(form.dataset.activityEditVersion || 0) + 1); markAdminDraft(form); } });
    panel.addEventListener('keydown', function (event) { const button = event.target.closest('[data-activity-editor-tab]'); if (!button || !['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return; event.preventDefault(); const buttons = Array.from(button.parentElement.querySelectorAll('[data-activity-editor-tab]')); const index = buttons.indexOf(button); const next = event.key === 'Home' ? 0 : event.key === 'End' ? buttons.length - 1 : (index + (event.key === 'ArrowRight' ? 1 : -1) + buttons.length) % buttons.length; buttons[next].click(); buttons[next].focus(); });
    const reference = find('[data-activity-reference-year]'); if (reference) reference.addEventListener('change', updateReferenceDates);
    window.addEventListener('further-ui:page', function (event) { if (event.detail && event.detail.name === 'activities') { syncRoute(); if (!view) load(); else syncActivityControls(); } });
    window.addEventListener('hashchange', function () { const old = location.hash.slice(1).split('/')[0]; if (['benefits', 'checkin', 'activity'].includes(old)) { history.replaceState(null, '', '#activities'); nav.click(); } syncRoute(); });
    syncRoute();
    if (typeof api.initializeVisuals === 'function') api.initializeVisuals();
  }

  document.addEventListener('further:ready', mount);
  window.FurtherUIActivities = { mount: mount, load: load, refreshTheme: refreshTheme };
}());
