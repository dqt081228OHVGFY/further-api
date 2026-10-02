import { readFileSync, writeFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { fileURLToPath } from 'node:url'

const require = createRequire(new URL('../web/package.json', import.meta.url))
const { JSDOM } = require('jsdom')
const target = fileURLToPath(new URL('../relayfor-ui/console/index.html', import.meta.url))
const source = readFileSync(target, 'utf8')
const dom = new JSDOM(source)
const document = dom.window.document
const before = [...document.querySelectorAll('*')].flatMap((element) => [...element.attributes].filter((attribute) => attribute.name.startsWith('data-')).map((attribute) => `${attribute.name}=${attribute.value}`))
function node(tag, className, html = '') {
  const element = document.createElement(tag)
  element.className = className
  element.innerHTML = html
  return element
}
function panel(name) { return document.querySelector(`[data-panel="${name}"]`) }
function shell(parent, elements, className, tag = 'div') {
  const wrapper = node(tag, className)
  const anchor = elements.find((element) => element.parentNode === parent)
  if (!anchor) throw new Error(`No attached anchor for ${className}`)
  anchor.before(wrapper)
  elements.forEach((element) => wrapper.append(element))
  return wrapper
}
function icon(name) { return `<span class="material-symbol" aria-hidden="true">${name}</span>` }
function kicker(text) { return `<span class="console-kicker">${text}</span>` }
function hint(title, text, iconName = 'info') {
  return node('div', 'console-note', `${icon(iconName)}<div><b>${title}</b><p>${text}</p></div>`)
}
const link = document.createElement('link')
link.rel = 'stylesheet'
link.href = '/relayfor-assets/console-workspace.css'
document.querySelector('link[href="/relayfor-assets/material.css"]').after(link)

// Overview: account totals, today's activity and a usable integration checklist.
const overview = panel('overview')
overview.querySelector('.dashboard-grid').classList.add('console-metric-strip')
const today = overview.querySelector('[data-today-requests]').closest('.md-card')
today.classList.add('console-today-card')
today.querySelector('.card-head').prepend(node('div', 'console-section-label', kicker('TODAY / 今日活动')))
today.querySelector('.md-card-title').textContent = '今天，进展如何？'
const todayRows = [...today.querySelectorAll('.list-item')]
shell(today, todayRows, 'console-today-metrics')
todayRows[0].classList.add('console-today-primary')
today.append(node('button', 'console-text-link', '查看调用记录 ' + icon('arrow_forward')))
today.lastElementChild.type = 'button'
today.lastElementChild.dataset.nav = 'usage'
const start = [...overview.querySelectorAll(':scope > .md-card')].find((card) => card.querySelector('[data-nav="tokens"]'))
start.classList.add('console-start-card')
start.querySelector('.card-head .md-card-title').textContent = '从第一条请求开始'
start.querySelector('.list-item').remove()
start.prepend(node('div', 'console-section-label', kicker('QUICK START / 接入指南')))
start.append(node('ol', 'console-connect-steps', '<li><span>01</span><div><b>为应用创建 API Key</b><p>每个应用使用独立令牌，便于管理访问与用量。</p></div><button type="button" class="md-icon-button" data-nav="tokens" aria-label="打开 API Key">' + icon('arrow_forward') + '</button></li><li><span>02</span><div><b>选择合适的模型</b><p>确认模型名称、价格与支持的请求端点。</p></div><button type="button" class="md-icon-button" data-nav="models" aria-label="查看模型与价格">' + icon('arrow_forward') + '</button></li><li><span>03</span><div><b>发送第一条请求</b><p>复制文档中的示例，替换 API Key 后即可接入。</p></div><button type="button" class="md-icon-button" data-nav="docs" aria-label="打开接入文档">' + icon('arrow_forward') + '</button></li>'))
shell(overview, [today, start], 'console-overview-split')
const extras = [overview.querySelector('[data-active-subscription-card]'), overview.querySelector('[data-group-quota-card]'), overview.querySelector('[data-user-benefits-banner]')]
const extrasWrap = node('div', 'console-account-extras')
overview.append(extrasWrap)
extras.forEach((element) => extrasWrap.append(element))

// API keys: a collection with a separate compact operating guide.
const tokens = panel('tokens')
tokens.querySelector(':scope > .card-head').classList.add('console-collection-head')
tokens.querySelector(':scope > .card-head .md-card-title').textContent = '你的访问凭据'
const keyCollection = tokens.querySelector('[data-token-rows]').closest('.md-card')
keyCollection.classList.add('console-key-collection')
const keyNotice = [...tokens.querySelectorAll(':scope > .md-card')].find((card) => card !== keyCollection)
const keyAside = node('aside', 'console-side-notes')
keyAside.append(node('div', 'console-side-title', kicker('KEY GUIDE') + '<h2>让访问井井有条</h2>'))
keyAside.append(hint('按应用建立令牌', '给令牌起一个容易识别的名称，在调用记录中快速找到对应应用。', 'apps'))
keyAside.append(hint('控制使用范围', '使用模型白名单、IP 白名单和过期时间限定访问范围。', 'shield'))
keyAside.append(keyNotice)
shell(tokens, [keyCollection, keyAside], 'console-collection-layout')

// Models: browsing area and account-specific access explanation side by side.
const models = panel('models')
const catalog = models.querySelector('[data-console-model-page="catalog"]')
catalog.querySelector(':scope > .card-head').classList.add('console-collection-head')
const priceCollection = catalog.querySelector('[data-model-pricing-rows]').closest('.md-card')
priceCollection.classList.add('console-model-collection')
const modelAside = node('aside', 'console-side-notes console-model-brief')
modelAside.append(node('div', 'console-side-title', kicker('YOUR ACCESS') + '<h2>账户访问规则</h2>'))
;[...catalog.querySelectorAll(':scope > .md-card')].filter((card) => card !== priceCollection).forEach((card) => modelAside.append(card))
shell(catalog, [priceCollection, modelAside], 'console-model-layout')
const healthPage = models.querySelector('[data-console-model-page="health"]')
healthPage.classList.add('console-health-workspace')
healthPage.querySelector('.health-table-card').before(node('div', 'console-health-caption', kicker('LIVE REQUEST OUTCOMES') + '<p>同一份真实调用数据，同时观察近期表现与今日表现。</p>'))

// Docs: section descriptions and their runnable examples become paired panels.
const docs = panel('docs')
const docsHero = docs.querySelector('.docs-hero')
docsHero.classList.add('console-docs-intro')
docsHero.querySelector('.md-card-title').innerHTML = kicker('DEVELOPER REFERENCE') + '<h2>几行代码，连接你的应用。</h2><p>从认证、模型选择到请求排查，在这里找到可直接使用的示例。</p>'
const toc = docs.querySelector('.docs-toc')
toc.classList.add('console-docs-directory')
toc.querySelector('b').textContent = '接入路线'
toc.querySelectorAll('[data-doc-anchor]').forEach((button, index) => {
  button.prepend(node('span', 'console-toc-number', String(index + 1).padStart(2, '0')))
})
toc.append(node('div', 'console-directory-foot', '<p>已经发起请求？</p><button type="button" class="console-text-link" data-nav="usage">前往调用记录 ' + icon('arrow_forward') + '</button>'))
docs.querySelector('.docs-layout').classList.add('console-docs-layout')
docs.querySelectorAll('.docs-section').forEach((section, index) => {
  section.classList.add('console-doc-chapter')
  const explanation = node('div', 'console-doc-explanation', kicker(`CHAPTER ${String(index + 1).padStart(2, '0')}`))
  explanation.append(section.querySelector('.docs-section-head'))
  const example = node('div', 'console-doc-example')
  if (section.id === 'docs-quickstart') {
    explanation.append(section.querySelector('.docs-steps'))
    example.append(section.querySelector('.docs-endpoint-row'))
    example.append(hint('Bearer API Key', '在 Authorization 请求头中使用你的 API Key。示例代码会使用当前站点地址。', 'key'))
    example.append(node('div', 'console-inline-actions', '<button type="button" class="md-btn md-btn-filled" data-nav="tokens">创建 API Key</button><button type="button" class="md-btn md-btn-text" data-nav="models">选择模型 ' + icon('arrow_forward') + '</button>'))
  } else if (section.id === 'docs-errors') {
    explanation.append(section.querySelector('.status-line'))
    example.append(section.querySelector('.docs-error-grid'))
  } else {
    [...section.children].forEach((child) => example.append(child))
  }
  section.append(explanation, example)
})

// Logs: a stable filter rail and a roomy table workspace.
const usage = panel('usage')
const filter = usage.querySelector('.filter-card')
const summary = filter.querySelector('[data-user-log-summary]')
const table = usage.querySelector('.usage-table-card')
const records = node('div', 'console-records-main')
records.append(summary, table)
filter.prepend(node('div', 'console-side-title', kicker('FILTER REQUESTS') + '<h2>查找你的请求</h2>'))
filter.classList.add('console-records-filter')
shell(usage, [filter, records], 'console-records-workspace')
table.prepend(node('div', 'console-table-caption', '<h2>请求明细</h2><p>点击费用展开实际扣费，点击表头调整排序。</p>'))

// Redemption: a generous input surface, with an explanation of accepted codes.
const redeem = panel('redeem')
const redeemCard = redeem.querySelector(':scope > .md-card')
redeemCard.classList.add('console-redeem-entry')
redeemCard.querySelector('.card-head').remove()
redeemCard.prepend(node('div', 'console-redeem-symbol', icon('redeem')))
redeemCard.prepend(node('div', 'console-redeem-copy', kicker('REDEEM YOUR CODE') + '<h2>把兑换码变成<br>下一次创作的动力。</h2><p>粘贴你的兑换码，我们会识别对应的额度或订阅权益。</p>'))
const redeemAside = node('aside', 'console-redeem-guide', '<div class="console-side-title">' + kicker('HOW IT WORKS') + '<h2>你可以兑换什么？</h2></div>')
redeemAside.append(hint('额度兑换码', '兑换成功后，额度会加入你的账户。', 'account_balance_wallet'))
redeemAside.append(hint('订阅兑换码', '对应的订阅计划与模型权益将在兑换成功后生效。', 'confirmation_number'))
redeemAside.append(hint('预付卡', '支持兑换管理员发放的预付卡，请以卡片对应的权益为准。', 'credit_card'))
redeemAside.append(node('div', 'console-directory-foot', '<p>多个兑换码？使用批量兑换逐个处理，单个失败不会影响其他兑换码。</p><button type="button" class="console-text-link" data-nav="subscriptions">查看已获得的权益 ' + icon('arrow_forward') + '</button>'))
shell(redeem, [redeemCard, redeemAside], 'console-redeem-layout')

// Subscription plans: catalog first; owned plans and timed quota in their own zones.
const subscriptions = panel('subscriptions')
const planCatalog = subscriptions.querySelector('[data-subscription-plan-catalog]')
const planHead = planCatalog.previousElementSibling
planHead.classList.add('console-collection-head')
planHead.append(node('button', 'console-text-link', '已有兑换码 ' + icon('arrow_forward')))
planHead.lastElementChild.type = 'button'
planHead.lastElementChild.dataset.nav = 'redeem'
shell(subscriptions, [planHead, planCatalog], 'console-plan-catalog')
const owned = subscriptions.querySelector('[data-user-subscriptions]').closest('.md-card')
const ownedHead = owned.previousElementSibling
owned.prepend(ownedHead)
owned.classList.add('console-owned-plans')
const timed = subscriptions.querySelector('[data-user-timed-quotas]').closest('.md-card')
timed.classList.add('console-timed-plans')
shell(subscriptions, [owned, timed], 'console-subscription-records')
timed.append(node('p', 'console-section-footnote', '限时额度有独立的有效期；可在账户偏好中设置是否优先使用。'))

// Benefit entries stay data driven, with a distinct collection surface.
const benefits = panel('benefits')
benefits.querySelector('[data-user-benefit-rows]').classList.add('console-benefit-catalog')
benefits.querySelectorAll('.benefit-section-head').forEach((heading) => {
  if (heading.textContent.includes('进行中的计费活动')) heading.remove()
})
benefits.querySelector('[data-user-benefit-rows]').before(node('div', 'console-benefit-intro', icon('redeem') + '<div><h2>属于你的福利，集中在这里。</h2><p>查看待领取的福利与已经获得的权益。</p></div>'))

// Account: a real scrolling directory and a stack of clear, independent forms.
const profile = panel('profile')
const settings = profile.querySelector('.settings-grid')
settings.classList.add('console-account-sections')
const accountCards = [...settings.children]
const accountDirectory = node('aside', 'console-account-directory', kicker('PERSONAL SPACE') + '<h2>你的账户</h2><p>资料、偏好和安全设置，都有自己的位置。</p>')
const accountLabels = [['identity', '个人资料', 'person'], ['preferences', '使用偏好', 'tune'], ['security', '安全与访问', 'verified_user'], ['operations', '账户操作', 'download']]
accountCards.forEach((card, index) => {
  const [key, title, iconName] = accountLabels[index]
  card.id = `account-${key}`
  card.classList.add(`console-account-${key}`)
  accountDirectory.append(node('button', 'console-account-directory-item', icon(iconName) + `<span>${title}</span>` + icon('arrow_forward')))
  const button = accountDirectory.lastElementChild
  button.type = 'button'
  button.dataset.docAnchor = card.id
})
accountDirectory.append(node('div', 'console-directory-foot', icon('lock') + '<p>安全设置与你的 API Key 分开管理，修改前请确认当前登录方式。</p>'))
shell(profile, [accountDirectory, settings], 'console-account-layout')
const identity = settings.querySelector('.console-account-identity')
shell(identity, [identity.querySelector('.settings-meta-grid'), identity.querySelector('[data-profile-form]')], 'console-identity-body')
const preferences = settings.querySelector('[data-preferences-form]')
shell(preferences, [...preferences.querySelectorAll(':scope > .switch-line')], 'console-preference-switches')
const security = settings.querySelector('[data-account-security-card]')
security.classList.add('md-card')
const securityFrame = security.querySelector('iframe')
const securityDetails = node('details', 'console-security-details', '<summary><span>' + icon('shield') + '<b>打开账户安全中心</b></span><span class="material-symbol" aria-hidden="true">expand_more</span></summary>')
securityDetails.append(securityFrame)
security.append(securityDetails)

// All existing data hooks and form contracts must survive the layout moves.
const after = [...document.querySelectorAll('*')].flatMap((element) => [...element.attributes].filter((attribute) => attribute.name.startsWith('data-')).map((attribute) => `${attribute.name}=${attribute.value}`))
const remaining = [...after]
for (const hook of before) {
  const index = remaining.indexOf(hook)
  if (index === -1) throw new Error(`Lost existing hook: ${hook}`)
  remaining.splice(index, 1)
}
writeFileSync(target, dom.serialize() + '\n', 'utf8')
console.log(`Console layout rewritten; preserved ${before.length} existing data hooks across ${document.querySelectorAll('[data-panel]').length} views.`)
