const LOCAL_HOSTS = new Set(['127.0.0.1', 'localhost', '[::1]']);

function browserDestination(value, localPort) {
  try {
    const url = new URL(value);
    if (url.username || url.password) return null;
    if (LOCAL_HOSTS.has(url.hostname)) {
      return url.protocol === 'http:' && url.port === String(localPort) ? url : null;
    }
    return url.protocol === 'https:' ? url : null;
  } catch {
    return null;
  }
}

module.exports = { browserDestination };
