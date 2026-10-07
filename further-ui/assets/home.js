(function () {
  const tabs = [...document.querySelectorAll('[data-home-protocol]')];
  const endpoint = document.querySelector('[data-home-endpoint]');
  const method = document.querySelector('[data-home-method]');
  const code = document.querySelector('[data-home-code]');
  const caption = document.querySelector('[data-home-caption]');
  const codePanel = document.querySelector('#home-code-stage');
  const origin = location.origin;
  const examples = {
    responses: { endpoint: '/v1/responses', caption: '适合文本生成与多轮交互。替换示例 Key 和模型名即可接入。', code: `curl -X POST '${origin}/v1/responses' \\
  -H 'Authorization: Bearer YOUR_API_KEY' \\
  -H 'Content-Type: application/json' \\
  -d '{"model":"YOUR_MODEL","input":"Hello, world."}'` },
    chat: { endpoint: '/v1/chat/completions', caption: '使用常见的 Chat Completions 请求格式。', code: `curl -X POST '${origin}/v1/chat/completions' \\
  -H 'Authorization: Bearer YOUR_API_KEY' \\
  -H 'Content-Type: application/json' \\
  -d '{"model":"YOUR_MODEL","messages":[{"role":"user","content":"Hello"}]}'` },
    anthropic: { endpoint: '/v1/messages', caption: 'Messages 格式需要 x-api-key 与 anthropic-version 请求头。', code: `curl -X POST '${origin}/v1/messages' \\
  -H 'x-api-key: YOUR_API_KEY' \\
  -H 'anthropic-version: 2023-06-01' \\
  -H 'Content-Type: application/json' \\
  -d '{"model":"YOUR_MODEL","max_tokens":1024,"messages":[{"role":"user","content":"Hello"}]}'` },
    gemini: { endpoint: '/v1beta/models/YOUR_MODEL:generateContent', caption: 'Gemini 原生格式使用 x-goog-api-key 请求头。', code: `curl -X POST '${origin}/v1beta/models/YOUR_MODEL:generateContent' \\
  -H 'x-goog-api-key: YOUR_API_KEY' \\
  -H 'Content-Type: application/json' \\
  -d '{"contents":[{"parts":[{"text":"Hello"}]}]}'` }
  };
  let codeAnimation = null;
  function selectProtocol(index, manual) {
    if (!endpoint || !method || !code || !caption || !tabs.length) return;
    const selected = (index + tabs.length) % tabs.length;
    const current = tabs[selected];
    const example = examples[current.dataset.homeProtocol];
    if (!example) return;
    tabs.forEach(function (tab, position) { const active = position === selected; tab.setAttribute('aria-selected', String(active)); tab.tabIndex = active ? 0 : -1; });
    if (codeAnimation) codeAnimation.cancel();
    method.textContent = 'POST'; endpoint.textContent = example.endpoint; code.textContent = example.code; caption.textContent = example.caption;
    if (codePanel) codePanel.setAttribute('aria-labelledby', current.id);
    if (manual && typeof code.animate === 'function') codeAnimation = code.animate([{ opacity: .45 }, { opacity: 1 }], { duration: 180, easing: 'cubic-bezier(.2,.8,.2,1)' });
  }
  tabs.forEach(function (tab, index) {
    tab.addEventListener('click', function () { selectProtocol(index, true); });
    tab.addEventListener('keydown', function (event) {
      let next = index;
      if (event.key === 'ArrowRight') next = (index + 1) % tabs.length;
      else if (event.key === 'ArrowLeft') next = (index - 1 + tabs.length) % tabs.length;
      else if (event.key === 'Home') next = 0;
      else if (event.key === 'End') next = tabs.length - 1;
      else return;
      event.preventDefault(); selectProtocol(next, true); tabs[next].focus();
    });
  });
  selectProtocol(0, false);

  // Reveal each section once. Scrolling never moves, scales or blurs content.
  const records = [...document.querySelectorAll('[data-home-scroll]')].map(function (anchor) {
    return { anchor: anchor, surface: anchor.querySelector('.home-motion-surface'), revealed: false };
  }).filter(function (record) { return Boolean(record.surface); });
  const animations = new Set();
  function reveal(record) {
    if (record.revealed) return;
    record.revealed = true;
    record.anchor.classList.add('is-visible');
    record.surface.classList.add('is-visible');
    if (typeof record.surface.animate !== 'function') return;
    const animation = record.surface.animate([
      { opacity: 0 },
      { opacity: 1 }
    ], { duration: 260, easing: 'cubic-bezier(.2,.8,.2,1)', fill: 'none' });
    animations.add(animation);
    animation.finished?.then(function () { animations.delete(animation); }, function () { animations.delete(animation); });
  }
  records.forEach(function (record) { record.surface.classList.add('home-reveal-card'); });
  document.documentElement.dataset.homeMotion = 'ready';
  if ('IntersectionObserver' in window) {
    const byElement = new Map(records.map(function (record) { return [record.anchor, record]; }));
    const observer = new IntersectionObserver(function (entries) {
      entries.forEach(function (entry) {
        if (!entry.isIntersecting) return;
        const record = byElement.get(entry.target);
        if (record) reveal(record);
        observer.unobserve(entry.target);
      });
    }, { rootMargin: '0px 0px -24px 0px', threshold: 0 });
    records.forEach(function (record) { observer.observe(record.anchor); });
    window.addEventListener('pagehide', function () { observer.disconnect(); }, { once: true });
  } else {
    records.forEach(function (record) { record.anchor.classList.add('is-visible'); record.surface.classList.add('is-visible'); });
  }
  window.addEventListener('pagehide', function () {
    animations.forEach(function (animation) { animation.cancel(); });
    animations.clear();
    if (codeAnimation) codeAnimation.cancel();
  });
}());
