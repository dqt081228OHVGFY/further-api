import { readFile, writeFile, mkdir, readdir } from 'node:fs/promises';
import { createHash } from 'node:crypto';
import { resolve, relative } from 'node:path';
const project = resolve(import.meta.dirname, '../../../..');
const root = resolve(project, 'relayfor-ui');
const output = import.meta.dirname;
await mkdir(output, { recursive: true });
const baseline = await readFile(resolve(root, 'assets/material-icons.js'), 'utf8');
const iconNodes = JSON.parse(baseline.match(/const iconNodes = (\{[^\n]+\});/)[1]);
async function walk(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  return (await Promise.all(entries.map(entry => entry.isDirectory() ? walk(resolve(directory, entry.name)) : /\.(html|js)$/.test(entry.name) && entry.name !== 'material-icons.js' ? [resolve(directory, entry.name)] : []))).flat();
}
const usages = [], dynamic = [], assignmentNames = new Set();
for (const file of await walk(root)) {
  const source = await readFile(file, 'utf8');
  for (const match of source.matchAll(/<([a-z]+)\b[^>]*\bclass=(?:"[^"]*\bmaterial-symbol\b[^"]*"|'[^']*\bmaterial-symbol\b[^']*')[^>]*>([\s\S]*?)<\/\1>/g)) {
    const text = match[2].trim();
    const location = { file: relative(project, file).replaceAll('\\', '/'), line: source.slice(0, match.index).split('\n').length };
    if (/^[a-z][a-z0-9_]*$/.test(text)) usages.push({ name: text, ...location });
    else {
      const candidates = [...text.matchAll(/['"]([a-z][a-z0-9_]*)['"]/g)].map(item => item[1]);
      dynamic.push({ expression: text, candidates, ...location });
      candidates.forEach(name => usages.push({ name, dynamic: true, ...location }));
    }
  }
  for (const match of source.matchAll(/\.textContent\s*=\s*([^;\n]+)/g)) {
    const expression = match[1];
    for (const literal of expression.matchAll(/['"]([a-z][a-z0-9_]*)['"]/g)) if (iconNodes[literal[1]]) assignmentNames.add(literal[1]);
  }
}
const names = [...new Set([...usages.map(item => item.name), ...assignmentNames])].sort();
const audit = {
  project,
  baselineSha256: createHash('sha256').update(baseline).digest('hex'),
  definitionCount: Object.keys(iconNodes).length - 1,
  sourceFilesScanned: (await walk(root)).length,
  names,
  missingNames: names.filter(name => !Object.hasOwn(iconNodes, name)),
  dynamic,
  assignmentNames: [...assignmentNames].sort(),
  usages,
  focusedDefinitions: Object.fromEntries(['search', 'settings', 'tune'].map(name => [name, iconNodes[name]])),
};
await writeFile(resolve(output, 'mapping-audit.json'), JSON.stringify(audit, null, 2) + '\n');
await writeFile(resolve(output, 'material-icons.baseline.js'), baseline);
console.log(JSON.stringify({ baselineSha256: audit.baselineSha256, sourceFilesScanned: audit.sourceFilesScanned, definitionCount: audit.definitionCount, distinctNames: names.length, missingNames: audit.missingNames, dynamic: audit.dynamic, assignmentNames: audit.assignmentNames }, null, 2));
