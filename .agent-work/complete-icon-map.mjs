import { readFile, writeFile } from 'node:fs/promises'
import vm from 'node:vm'
const target = new URL('../relayfor-ui/assets/material-icons.js', import.meta.url)
let source = await readFile(target, 'utf8')
const match = source.match(/const iconNodes = (\{[^\r\n]+\});/)
if (!match) throw new Error('Local icon map missing')
const nodes = JSON.parse(match[1])
const additions = { edit:'pencil', manage_search:'list-filter', pause_circle:'circle-pause', play_circle:'circle-play', query_stats:'chart-no-axes-combined', workspace_premium:'badge-check', price_check:'badge-dollar-sign', groups:'users', celebration:'party-popper', festival:'party-popper', local_florist:'flower-2', volunteer_activism:'heart-handshake' }
for (const [key,file] of Object.entries(additions)) {
  const module = await readFile(new URL(`../web/node_modules/lucide-react/dist/esm/icons/${file}.mjs`,import.meta.url),'utf8')
  const expression = module.match(/const __iconNode = ([\s\S]+?);/)
  if(!expression) throw new Error('SVG nodes missing: '+file)
  nodes[key] = vm.runInNewContext(expression[1])
}
source = source.replace(match[0], 'const iconNodes = ' + JSON.stringify(nodes) + ';')
await writeFile(target,source)
console.log(Object.keys(nodes).length+' local icons mapped')
