import http from 'node:http';

// Preserve the old preview link while all pages and APIs use the Go gateway.
// A redirect avoids depending on a second backend or stale disk-only assets.
const gatewayOrigin = 'http://127.0.0.1:3000';
http.createServer((req, res) => {
  const legacyUrl = new URL(req.url, 'http://127.0.0.1:8314');
  const target = new URL(gatewayOrigin);
  target.pathname = legacyUrl.pathname;
  target.search = legacyUrl.search;
  res.writeHead(307, {
    Location: target.href,
    'Cache-Control': 'no-store',
    'Content-Type': 'text/plain; charset=utf-8',
  });
  res.end('RelayFor preview moved to ' + target.href);
}).listen(8314, '127.0.0.1', () => {
  console.log('Legacy preview redirects to ' + gatewayOrigin);
});
