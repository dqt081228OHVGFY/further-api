const assert = require('node:assert/strict');
const { test } = require('node:test');
const { browserDestination } = require('./navigation-policy.cjs');

test('keeps local application and HTTPS OAuth navigation available', () => {
  for (const url of ['http://127.0.0.1:3000/', 'http://localhost:3000/oauth/callback', 'http://[::1]:3000/auth/', 'https://github.com/login/oauth/authorize', 'https://new.ambition.qzz.io/']) {
    assert.equal(browserDestination(url, 3000).href, new URL(url).href);
  }
});

test('denies executable protocols, credential URLs and other loopback ports', () => {
  for (const url of ['file:///C:/Windows/System32/config/SAM', 'javascript:alert(1)', 'data:text/html,hello', 'http://localhost:8080/', 'http://127.0.0.1/', 'http://example.com/', 'https://user:password@example.com/', 'https://localhost:3000/', 'not a URL']) {
    assert.equal(browserDestination(url, 3000), null, url);
  }
});
