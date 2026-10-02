from pathlib import Path
import re

root = Path(__file__).resolve().parent.parent

def transform(relative, fn):
    path = root / relative
    before = path.read_text(encoding='utf-8-sig')
    after = fn(before)
    assert after != before, relative
    path.write_text(after, encoding='utf-8')

def drawer(s):
    s = s.replace('CalendarClock, CreditCard, RefreshCw, Settings2', 'CalendarClock, RefreshCw, Settings2')
    s = s.replace('  createWaffoPancakeSubscriptionProduct,\n', '').replace('  listWaffoPancakeSubscriptionProductOptions,\n', '')
    start = s.index('  const [creatingPancakeProduct')
    end = s.index('  const schema =', start)
    s = s[:start] + s[end:]
    start = s.index('      // Best-effort — empty list')
    end = s.index('\n    }\n  }, [open', start)
    s = s[:start] + s[end:]
    start = s.index('  // Gate "+ Create on Pancake"')
    end = s.index('  const onSubmit', start)
    s = s[:start] + s[end:]
    start = s.index('  // Mints a Pancake')
    end = s.index('  const durationUnitOpts', start)
    s = s[:start] + s[end:]
    start = s.index('            {/* Payment Config */}')
    end = s.index('            </SideDrawerSection>', start) + len('            </SideDrawerSection>')
    s = s[:start] + s[end:]
    s, count = re.subn(r"\s*<FormField\s+control=\{form.control\}\s+name='allow_balance_pay'.*?(?=\s*<FormField)", '', s, count=1, flags=re.S)
    assert count == 1
    return s

transform('web/src/features/subscriptions/components/subscriptions-mutate-drawer.tsx', drawer)

def plan_types(s):
    s = re.sub(r'^.*(?:allow_balance_pay|stripe_price_id|creem_product_id|waffo_pancake_product_id):.*\n', '', s, flags=re.M)
    return s

transform('web/src/features/subscriptions/lib/plan-form.ts', plan_types)
transform('web/src/features/subscriptions/types.ts', lambda s: re.sub(r'export interface SubscriptionPayRequest.*?(?=export interface CreateUserSubscriptionRequest)', '', plan_types(s), flags=re.S))

def subscription_api(s):
    s = re.sub(r'^  Subscription(?:PayResponse|PayRequest|BalancePayRequest),\n', '', s, flags=re.M)
    start = s.index('export async function paySubscriptionStripe')
    end = s.index('// User Self Subscriptions', start)
    end = s.rfind('// ============================================================================', start, end)
    return s[:start] + s[end:]
transform('web/src/features/subscriptions/api.ts', subscription_api)

def plan_columns(s):
    start = s.index("        id: 'payment',")
    end = s.index("        size: 140,", start)
    return s[:start] + "        id: 'payment',\n        header: t('Payment Channel'),\n        meta: { mobileHidden: true },\n        cell: () => <StatusBadge label='LDC' variant='neutral' copyable={false} />,\n" + s[end:]
transform('web/src/features/subscriptions/components/subscriptions-columns.tsx', plan_columns)

def vip(s):
    s = s.replace('  const escape =', '  const purchases = new Set();\n  const escape =', 1)
    s = s.replace('billing.payment_compliance_confirmed === true', 'billing.enabled === true && billing.configured === true')
    s = s.replace('VIP、兑换与支付功能尚未启用', 'LDC 支付尚未开放').replace('请在支付设置中完成站点所需的功能启用确认，然后返回刷新。', '请在支付设置中配置 LDC 应用并启用支付，然后返回刷新。')
    s = s.replace('!billing.payment_compliance_confirmed || plan.allow_balance_pay === false', '!billing.enabled || !billing.configured || purchases.has(Number(plan.id))')
    s = s.replace('>使用余额开通</button>', '>使用 LDC 开通</button>')
    s = s.replace("panel.querySelector('[data-vip-create]').disabled = !billing.payment_compliance_confirmed;", "panel.querySelector('[data-vip-create]').disabled = false;")
    s = s.replace("api.request('/api/user/topup/info')", "api.request('/api/user/ldc/info')")
    s = s.replace(",'stripe_price_id','creem_product_id','waffo_pancake_product_id'", '')
    s = s.replace('    form.elements.allow_balance_pay.checked = plan ? plan.allow_balance_pay !== false : true;\n', '')
    start = s.index('  async function buy(id, button) {')
    end = s.index('  function renderSubscriptions', start)
    s = s[:start] + '''  async function buy(id, button) {
    const plan = plans.find(function(item){return Number(item.id) === id;});
    if(!plan || !plan.enabled || !billing.enabled || !billing.configured || purchases.has(id) || button.disabled) return;
    purchases.add(id); api.showBusy(button,true);
    try {
      const quote = await api.request('/api/user/ldc/quote',{method:'POST',body:{plan_id:id}});
      if(!quote || !quote.credits || Number(quote.plan_id) !== id) throw new Error('套餐报价无效，请刷新后重试');
      if(!await api.confirm('使用 '+quote.credits+' LDC，开通“'+plan.title+'”，有效期 '+duration(plan)+'。付款确认后权益生效。',{title:'确认 LDC 开通',confirmText:'前往 LDC 支付'})) return;
      const order = await api.request('/api/subscription/ldc/pay',{method:'POST',body:{plan_id:id,expected_credits:quote.credits}});
      const target = new URL(order.url);
      if(target.protocol !== 'https:' || target.hostname !== 'credit.linux.do' || order.method !== 'POST' || !order.params || !order.trade_no) throw new Error('LDC 支付地址无效');
      const form = document.createElement('form'); form.method='POST';form.action=target.href;form.hidden=true;
      Object.entries(order.params).forEach(function(pair){const input=document.createElement('input');input.type='hidden';input.name=pair[0];input.value=String(pair[1]);form.append(input);});
      document.body.append(form);try{HTMLFormElement.prototype.submit.call(form);}finally{form.remove();}
      state('订单已创建，正在前往 LDC 支付。付款完成后刷新查看权益。');
    } catch(error){state(error.message,true);api.toast(error.message,'danger');}
    finally{purchases.delete(id);if(button.isConnected)api.showBusy(button,false);panel.querySelectorAll('[data-vip-buy="'+id+'"]').forEach(function(item){item.disabled=!billing.enabled||!billing.configured;});}
  }
''' + s[end:]
    s = re.sub(r'<details class="vip-advanced">.*?</details>', '', s, count=1)
    s = s.replace('<label class="switch-line"><input type="checkbox" name="allow_balance_pay" checked><span>允许余额购买</span></label>', '')
    s = s.replace('allow_balance_pay:form.elements.allow_balance_pay.checked,', '')
    s = s.replace("for(const field of ['stripe_price_id','creem_product_id','waffo_pancake_product_id'])plan[field]=form.elements[field].value.trim();", "delete plan.allow_balance_pay;delete plan.stripe_price_id;delete plan.creem_product_id;delete plan.waffo_pancake_product_id;")
    s = s.replace('更多支付方式', 'LDC 充值与订单').replace('暂时无法载入支付方式', '暂时无法载入 LDC 充值')
    return s
transform('relayfor-ui/assets/vip-workspace.js', vip)
