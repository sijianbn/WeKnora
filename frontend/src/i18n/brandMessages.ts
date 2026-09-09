import { APP_NAME, CLOUD_SERVICE_NAME, CLOUD_SERVICE_NAME_COMPACT, DEFAULT_APP_NAME, DEFAULT_CLOUD_SERVICE_NAME } from '../config/brand'

/**
 * 启动期品牌替换：把语言包文案里的默认品牌名换成运行时配置的品牌名。
 *
 * 为什么在消息对象上做整体替换，而不是把几百处文案改成 {name} 插值：
 * 语言包 5 个 locale 共约 250 处品牌名，插值方案要求每个 t() 调用点都传参，
 * 漏一处就渲染出字面 {name}；这里一次性遍历消息树，调用点零改动。
 *
 * 替换规则（顺序敏感）：
 * 1. 先摘除协议标识 X-WeKnora-Signature / aud=weknora —— 它们是与后端的契约，
 *    不跟随品牌，最后原样还原。
 * 2. "WeKnoraCloud"（紧凑写法）→ 云服务名去空格变体。
 * 3. "WeKnora Cloud" → 云服务名。
 * 4. 其余 "WeKnora" → 产品名。云服务名是独立参数：只覆盖 APP_NAME 时云服务
 *    文案保持默认，不会跟着产品名变。小写 "weknora"（CLI 二进制名、
 *    @lyingbug/weknora 包名、weknora-app/weknora-ui 服务名等）一律不动。
 *
 * 只处理消息 value；对象 key（i18n 键名）原样保留。
 * 默认品牌未被覆盖时直接返回原对象，保证默认部署的行为逐字节不变。
 */

export interface BrandNames {
  appName: string
  cloudServiceName: string
  cloudServiceNameCompact: string
}

const MASK_SIGNATURE = '\u0000brand-sig\u0000'
const MASK_AUDIENCE = '\u0000brand-aud\u0000'
const MASK_CLOUD_COMPACT = '\u0000brand-cloud-c\u0000'
const MASK_CLOUD_SPACED = '\u0000brand-cloud-s\u0000'

function replaceBrandTokens(value: string, brand: BrandNames): string {
  let s = value
    .split('X-WeKnora-Signature').join(MASK_SIGNATURE)
    .split('aud=weknora').join(MASK_AUDIENCE)
    .split('WeKnoraCloud').join(MASK_CLOUD_COMPACT)
    .split('WeKnora Cloud').join(MASK_CLOUD_SPACED)
    .split('WeKnora').join(brand.appName)
  return s
    .split(MASK_SIGNATURE).join('X-WeKnora-Signature')
    .split(MASK_AUDIENCE).join('aud=weknora')
    .split(MASK_CLOUD_COMPACT).join(brand.cloudServiceNameCompact)
    .split(MASK_CLOUD_SPACED).join(brand.cloudServiceName)
}

function isBrandCustomized(brand: BrandNames): boolean {
  return brand.appName !== DEFAULT_APP_NAME || brand.cloudServiceName !== DEFAULT_CLOUD_SERVICE_NAME
}

export function applyBrandToMessages<T>(node: T, brand?: BrandNames): T {
  if (!brand) brand = { appName: APP_NAME, cloudServiceName: CLOUD_SERVICE_NAME, cloudServiceNameCompact: CLOUD_SERVICE_NAME_COMPACT }
  if (!isBrandCustomized(brand)) return node
  if (typeof node === 'string') return replaceBrandTokens(node, brand) as unknown as T
  if (Array.isArray(node)) return node.map((child) => applyBrandToMessages(child, brand)) as unknown as T
  if (node && typeof node === 'object') {
    const out: Record<string, unknown> = {}
    for (const [key, child] of Object.entries(node)) {
      out[key] = applyBrandToMessages(child, brand)
    }
    return out as unknown as T
  }
  return node
}
