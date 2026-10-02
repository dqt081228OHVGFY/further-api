"""Recover RelayFor source from the verified local snapshot; never enable legacy APIs."""
from pathlib import Path
from hashlib import sha256
from html.parser import HTMLParser
import json
import re
import shutil

SOURCE = Path(r'C:\Users\Administrator\Desktop\relayfor-rebuild-summary')
ROOT = Path(r'D:\test\relayfor-rebuild')
DEST = ROOT / 'relayfor-ui'
ARCHIVE = ROOT / '.agent-work' / 'relayfor-original-source'
PREFIX = '/relayfor-assets/'

files = {
    'index.html': 'mirror/index.html',
    'auth/index.html': 'mirror/auth@@mode=login/index.html',
    'console/index.html': 'probe-assets/a.html',
    'admin/index.html': 'admin-mirror/admin/index.html',
    'assets/app.css': 'mirror/app.css',
    'assets/app.js': 'mirror/app.js',
    'assets/site.js': 'mirror/site.js',
    'assets/resource-loader.js': 'mirror/resource-loader.js',
    'assets/console.js': 'mirror/console.js',
    'assets/console-models.js': 'mirror/console-models.js',
    'assets/material-symbols-outlined-9cee10d72b8e.woff2': 'mirror/assets/material-symbols-outlined-9cee10d72b8e.woff2',
}
for name in ['admin.js', 'admin-overview.js', 'admin-settings.js', 'admin-models.js', 'admin-routing.js', 'channel-keys.js', 'disabled-channels.js']:
    files['assets/admin/' + name] = 'admin-mirror/' + name

url_map = {'/' + Path(relative).name: PREFIX + relative.removeprefix('assets/') for relative in files if relative.startswith('assets/')}
url_map['/assets/material-symbols-outlined-9cee10d72b8e.woff2'] = PREFIX + 'material-symbols-outlined-9cee10d72b8e.woff2'

records = []
for target_name, source_name in files.items():
    source = SOURCE / source_name
    target = DEST / target_name
    if target.exists():
        raise SystemExit('Refusing to overwrite restored/user file: ' + str(target))
    raw = source.read_bytes()
    archived = ARCHIVE / source_name
    archived.parent.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(source, archived)
    output = raw
    transforms = []
    if target.suffix in {'.html', '.css'}:
        content = raw.decode('utf-8')
        for old, new in url_map.items():
            content = content.replace('"' + old + '"', '"' + new + '"').replace("'" + old + "'", "'" + new + "'")
        transforms.append('Local script/style/font resource URLs use /relayfor-assets/.')
        if target.suffix == '.html':
            content = re.sub(r'<script\b(?=[^>]*\bsrc=)', '<script type="application/x-relayfor-legacy" data-relayfor-source-only="true"', content)
            transforms.append('Legacy scripts are inert source references until real New API integration replaces their API/auth/business initialization.')
        output = content.encode('utf-8')
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_bytes(output)
    records.append({'target': target_name, 'sourcePath': str(source), 'sourceBytes': len(raw), 'sourceSha256': sha256(raw).hexdigest(), 'restoredBytes': len(output), 'restoredSha256': sha256(output).hexdigest(), 'originalArchivePath': str(archived), 'transformations': transforms, 'runtimeReady': False if target.suffix in {'.js', '.html'} else True})

conflicts = []
for filename in ['app.css', 'app.js', 'resource-loader.js']:
    left = SOURCE / 'mirror' / filename
    right = SOURCE / 'admin-mirror' / filename
    conflicts.append({'name': filename, 'userSource': str(left), 'adminSource': str(right), 'identical': left.read_bytes() == right.read_bytes(), 'sha256': sha256(left.read_bytes()).hexdigest(), 'resolution': 'One shared assets/' + filename})

class References(HTMLParser):
    def __init__(self):
        super().__init__()
        self.assets = []
        self.links = []
    def handle_starttag(self, tag, attrs):
        attrs = dict(attrs)
        if tag in {'script', 'img', 'source', 'video'} and attrs.get('src'):
            self.assets.append(attrs['src'])
        if tag == 'link' and attrs.get('href'):
            self.assets.append(attrs['href'])
        if tag == 'a' and attrs.get('href'):
            self.links.append(attrs['href'])

references = []
navigation = set()
for file in DEST.rglob('*.html'):
    parser = References()
    parser.feed(file.read_text(encoding='utf-8'))
    for url in parser.assets:
        local = DEST / 'assets' / url.removeprefix(PREFIX) if url.startswith(PREFIX) else None
        references.append({'page': str(file.relative_to(DEST)), 'url': url, 'exists': bool(local and local.is_file())})
    navigation.update(parser.links)
for file in DEST.rglob('*.css'):
    for url in re.findall(r'url\([\'\"]?([^\)\'\"]+)', file.read_text(encoding='utf-8')):
        local = DEST / 'assets' / url.removeprefix(PREFIX) if url.startswith(PREFIX) else None
        references.append({'page': str(file.relative_to(DEST)), 'url': url, 'exists': bool(local and local.is_file())})

manifest = {'restorationVersion': 1, 'sourceRoot': str(SOURCE), 'resourceUrlPrefix': PREFIX, 'assetDirectory': 'assets', 'productionReady': False, 'legacyPolicy': 'Original source is recovered for faithful UI/API integration. Legacy business scripts and auth handlers must not execute. All HTML script references are inert until the integration owner replaces obsolete behavior. Do not serve .agent-work.', 'pages': {'/': 'index.html', '/auth/': 'auth/index.html', '/console/': 'console/index.html', '/admin/': 'admin/index.html'}, 'files': records, 'sameNameCollisions': conflicts, 'resourceClosure': {'allPresent': all(ref['exists'] for ref in references), 'references': references}, 'navigationTargetsForIntegration': sorted(navigation), 'excluded': ['All fixture scripts/data and cached /api JSON', 'Legacy Node preview server and runtime auth configuration', 'Legacy prepaid page/script (original business feature)', 'No synthetic accounts, tokens, or credentials copied']}
(DEST / 'source-provenance.json').write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
(DEST / 'RESTORATION.md').write_text('''# RelayFor 原前端恢复源码

来源：`C:/Users/Administrator/Desktop/relayfor-rebuild-summary`。原始哈希、归档位置和资源闭包见 `source-provenance.json`。

恢复首页、登录、用户控制台、管理员页面的原 DOM 与布局；`assets/app.css` 保留原样式和动画（仅字体 URL 调整），`assets/app.js` 保留原文件字节。共享资源使用 `/relayfor-assets/`，管理员专用脚本位于该命名空间的 `admin/`。

这是待接入真实 New API 的恢复源码。HTML 中原脚本均使用 `application/x-relayfor-legacy`，不会执行。集成时应由真实接口适配与原动画初始化替换这些引用，不可直接把旧商业、认证或页面初始化整体重新启用。原始文件完整归档于项目 `.agent-work/relayfor-original-source`，不得发布此归档目录。

没有复制 fixture、旧预览服务、演示认证、预付卡页面或缓存 API 响应。页面中的旧商业 DOM 暂作为恢复证据，正式发布时由集成层删除旧专有功能；不能按支付或订阅关键词删除 New API 对应实现。
''', encoding='utf-8')
print(json.dumps({'restoredFiles': len(records), 'assetReferences': len(references), 'resourceClosure': manifest['resourceClosure']['allPresent'], 'sameNameCollisionsIdentical': all(c['identical'] for c in conflicts), 'output': str(DEST)}, ensure_ascii=False))
if not manifest['resourceClosure']['allPresent']:
    raise SystemExit('Unresolved asset references')
