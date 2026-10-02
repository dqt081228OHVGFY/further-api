(function () {
  'use strict';

  const tabs = Array.from(document.querySelectorAll('[data-home-protocol]'));
  const endpoint = document.querySelector('[data-home-endpoint]');
  const method = document.querySelector('[data-home-method]');
  const code = document.querySelector('[data-home-code]');
  const caption = document.querySelector('[data-home-caption]');
  if (!tabs.length || !endpoint || !method || !code || !caption) return;

  const origin = location.origin;
  const examples = {
    responses: {
      endpoint: '/v1/responses',
      caption: '适合文本生成与多轮交互。替换示例 Key 和模型名即可接入。',
      code: `POST ${origin}/v1/responses
Authorization: Bearer YOUR_API_KEY
Content-Type: application/json

{"model":"YOUR_MODEL","input":"Hello, world."}`
    },
    chat: {
      endpoint: '/v1/chat/completions',
      caption: '使用常见的 Chat Completions 请求格式。',
      code: `POST ${origin}/v1/chat/completions
Authorization: Bearer YOUR_API_KEY
Content-Type: application/json

{"model":"YOUR_MODEL","messages":[{"role":"user","content":"Hello"}]}`
    },
    anthropic: {
      endpoint: '/v1/messages',
      caption: 'Messages 格式需要 x-api-key 与 anthropic-version 请求头。',
      code: `POST ${origin}/v1/messages
x-api-key: YOUR_API_KEY
anthropic-version: 2023-06-01
Content-Type: application/json

{"model":"YOUR_MODEL","max_tokens":1024,"messages":[{"role":"user","content":"Hello"}]}`
    },
    gemini: {
      endpoint: '/v1beta/models/YOUR_MODEL:generateContent',
      caption: 'Gemini 原生格式使用 x-goog-api-key 请求头。',
      code: `POST ${origin}/v1beta/models/YOUR_MODEL:generateContent
x-goog-api-key: YOUR_API_KEY
Content-Type: application/json

{"contents":[{"parts":[{"text":"Hello"}]}]}`
    }
  };

  let selected = 0;
  let lastInteraction = 0;
  let protocolVisible = true;
  const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)');
  function motionEnabled() {
    return !reduceMotion.matches && document.documentElement.dataset.relayforMotion !== 'off' && document.documentElement.dataset.reducedMotion !== 'true';
  }
  function select(index, manual) {
    selected = (index + tabs.length) % tabs.length;
    const current = tabs[selected];
    const example = examples[current.dataset.homeProtocol];
    if (!example) return;
    tabs.forEach(function (tab, position) {
      const active = position === selected;
      tab.setAttribute('aria-selected', String(active));
      tab.tabIndex = active ? 0 : -1;
    });
    method.textContent = 'POST';
    endpoint.textContent = example.endpoint;
    code.textContent = example.code;
    caption.textContent = example.caption;
    if (manual) lastInteraction = Date.now();
    if (motionEnabled() && code.animate) {
      code.animate([{ opacity: .5, transform: 'translate3d(0,5px,0)' }, { opacity: 1, transform: 'translate3d(0,0,0)' }], { duration: 260, easing: 'cubic-bezier(.05,.7,.1,1)' });
    }
  }
  tabs.forEach(function (tab, index) {
    tab.addEventListener('click', function () { select(index, true); });
    tab.addEventListener('keydown', function (event) {
      if (event.key !== 'ArrowRight' && event.key !== 'ArrowLeft') return;
      event.preventDefault();
      const next = (index + (event.key === 'ArrowRight' ? 1 : -1) + tabs.length) % tabs.length;
      select(next, true);
      tabs[next].focus();
    });
  });
  select(0, false);

  const protocolSection = document.querySelector('#api-protocols');
  if ('IntersectionObserver' in window && protocolSection) {
    const observer = new IntersectionObserver(function (entries) { protocolVisible = entries[0].isIntersecting; }, { threshold: .08 });
    observer.observe(protocolSection);
  }
  window.setInterval(function () {
    if (!protocolVisible || document.hidden || !motionEnabled() || Date.now() - lastInteraction < 12000) return;
    select(selected + 1, false);
  }, 6000);

  const reveals = Array.from(document.querySelectorAll('.home-reveal'));
  if ('IntersectionObserver' in window && motionEnabled()) {
    document.documentElement.dataset.homeMotion = 'ready';
    const observer = new IntersectionObserver(function (entries) {
      entries.forEach(function (entry) {
        if (entry.isIntersecting) {
          entry.target.classList.add('is-visible');
          observer.unobserve(entry.target);
        }
      });
    }, { rootMargin: '0px 0px -8% 0px', threshold: .08 });
    reveals.forEach(function (section) { observer.observe(section); });
  } else {
    reveals.forEach(function (section) { section.classList.add('is-visible'); });
  }
}());
