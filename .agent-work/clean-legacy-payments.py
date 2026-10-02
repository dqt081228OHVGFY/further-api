from pathlib import Path
import re
root = Path(__file__).resolve().parent.parent

def update(name, convert):
    path = root / name
    source = path.read_text(encoding='utf-8-sig')
    path.write_text(convert(source), encoding='utf-8')

def wallet_api(source):
    names = ['RedemptionRequest','AffiliateTransferRequest','ApiResponse','RedemptionResponse','AffiliateCodeResponse','AffiliateTransferResponse','BillingHistoryResponse','CompleteOrderRequest']
    start = source.index('import type {')
    end = source.index("} from './types'", start) + len("} from './types'")
    source = source[:start] + 'import type { '+', '.join(names)+" } from './types'" + source[end:]
    source = re.sub(r'/\*\*\s*\* Get topup configuration info.*?(?=/\*\*\s*\* Redeem a topup code)', '', source, flags=re.S)
    start = source.index('/**\n * Calculate payment amount for regular payment')
    end = source.index('/**\n * Get affiliate code', start)
    return source[:start] + source[end:]
update('web/src/features/wallet/api.ts', wallet_api)

def wallet_types(source):
    header = source[:source.index('// ============================================================================')]
    tail = source[source.index('/**\n * Affiliate quota transfer request'):]
    return header + '''export interface ApiResponse<T = unknown> { success?: boolean; message?: string; data?: T }
export type RedemptionResponse = ApiResponse<number>
export type AffiliateCodeResponse = ApiResponse<string>
export type AffiliateTransferResponse = ApiResponse
export interface RedemptionRequest { key: string }

''' + tail
update('web/src/features/wallet/types.ts', wallet_types)
update('web/src/features/wallet/lib/index.ts', lambda s: re.sub(r"^export \* from './(?:format|payment|ui)'\n", '', s, flags=re.M))
update('web/src/features/wallet/hooks/index.ts', lambda s: re.sub(r"^export \* from './(?:use-topup-info|use-payment|use-creem-payment|use-waffo-payment|use-waffo-pancake-payment)'\n", '', s, flags=re.M))
update('relayfor-ui/console/index.html', lambda s: s.replace('<label class="switch-line"><input type="checkbox" name="show_seconds"><span class="switch-body"><span class="switch-title">时间显示秒数</span></span></label>', ''))
update('relayfor-ui/assets/console.js', lambda s: s.replace('\tform.elements.show_seconds.checked = Boolean(prefs.showSeconds);\n', '').replace('show_seconds: form.elements.show_seconds.checked', 'show_seconds: Boolean(api.preferences().showSeconds)').replace('showSeconds: form.elements.show_seconds.checked', 'showSeconds: Boolean(api.preferences().showSeconds)'))
