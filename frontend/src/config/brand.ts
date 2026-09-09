/**
 * 品牌参数唯一出口：UI 展示的产品名统一从这里取，禁止在组件里散落硬编码。
 *
 * 取值优先级（与 utils/index.ts 的 MAX_FILE_SIZE_MB 一致）：
 *   运行时配置 window.__RUNTIME_CONFIG__（Docker entrypoint 生成 config.js）
 *   > 构建时环境变量 VITE_APP_NAME / VITE_CLOUD_SERVICE_NAME
 *   > 默认值 "WeKnora" / "WeKnora Cloud"。
 *
 * window / import.meta.env 的存在性守卫是为了让纯 Node 环境（tsx --test）
 * 也能 import 本模块而不抛错。
 *
 * 注意区分三类名字，只有第一类跟随本模块：
 * 1. 展示名（页面标题、欢迎语、设置页文案）—— APP_NAME / CLOUD_SERVICE_NAME。
 * 2. 协议标识（X-WeKnora-Signature 请求头、aud=weknora、ClawHub 包名 @lyingbug/weknora、
 *    weknora-app/weknora-ui 服务名、weknora CLI 二进制名）—— 与后端/生态的契约，不参数化。
 * 3. 本地存储键（WeKnora_theme、WeKnora_settings、weknora_token 等）—— 改动会丢老用户数据，不参数化。
 */

export const DEFAULT_APP_NAME = 'WeKnora'
export const DEFAULT_CLOUD_SERVICE_NAME = 'WeKnora Cloud'

const BUILD_ENV = (import.meta as ImportMeta & { env?: Record<string, string | undefined> }).env

// entrypoint 侧已做白名单，这里再防御一层：去掉可能破坏 vue-i18n 插值（{}）
// 或被拼进 HTML 的字符，并限制长度。
function sanitizeBrandName(value: unknown, fallback: string, maxLength = 32): string {
  const cleaned = typeof value === 'string'
    ? value.replace(/[`"'\\<>{}$]/g, '').trim().slice(0, maxLength)
    : ''
  return cleaned || fallback
}

function resolveBrandName(runtimeValue: unknown, buildValue: unknown, fallback: string): string {
  const raw = typeof runtimeValue === 'string' && runtimeValue.trim()
    ? runtimeValue
    : (typeof buildValue === 'string' && buildValue.trim() ? buildValue : '')
  return sanitizeBrandName(raw, fallback)
}

export const APP_NAME = resolveBrandName(
  typeof window !== 'undefined' ? window.__RUNTIME_CONFIG__?.APP_NAME : undefined,
  BUILD_ENV?.VITE_APP_NAME,
  DEFAULT_APP_NAME,
)

export const CLOUD_SERVICE_NAME = resolveBrandName(
  typeof window !== 'undefined' ? window.__RUNTIME_CONFIG__?.CLOUD_SERVICE_NAME : undefined,
  BUILD_ENV?.VITE_CLOUD_SERVICE_NAME,
  DEFAULT_CLOUD_SERVICE_NAME,
)

// 文案里的紧凑写法（"WeKnoraCloud 凭据"）。默认值与现有文案逐字一致；
// 覆盖时由部署方提供的云服务名去空格派生。
export const CLOUD_SERVICE_NAME_COMPACT = CLOUD_SERVICE_NAME.replace(/\s+/g, '')

// meta description 覆盖项：仅当部署方显式提供时生效，否则保留 index.html 默认文案。
const runtimeDescription = typeof window !== 'undefined'
  ? window.__RUNTIME_CONFIG__?.APP_DESCRIPTION
  : undefined
export const APP_DESCRIPTION = (typeof runtimeDescription === 'string'
  ? runtimeDescription.replace(/[`"'\\<>]/g, '').trim().slice(0, 300)
  : '')

// 品牌 Logo / favicon 覆盖项：接受 http(s) URL、同源根路径（如 /branding/logo.png，
// 配合 volume 挂载）或 data:image URI；留空回退内置资源。前缀白名单同时挡掉
// javascript: 等危险协议。仅用于 :src / href 绑定，不做二次转义。
export function isValidBrandAssetUrl(value: string): boolean {
  return /^(https?:\/\/|\/|data:image\/)/i.test(value)
}

function sanitizeAssetUrl(value: unknown): string {
  if (typeof value !== 'string') return ''
  const trimmed = value.trim().slice(0, 8192)
  return isValidBrandAssetUrl(trimmed) ? trimmed : ''
}

function resolveAssetUrl(runtimeValue: unknown, buildValue: unknown): string {
  const fromRuntime = sanitizeAssetUrl(runtimeValue)
  if (fromRuntime) return fromRuntime
  return sanitizeAssetUrl(buildValue)
}

export const APP_LOGO_URL = resolveAssetUrl(
  typeof window !== 'undefined' ? window.__RUNTIME_CONFIG__?.APP_LOGO_URL : undefined,
  BUILD_ENV?.VITE_APP_LOGO_URL,
)

export const APP_FAVICON_URL = resolveAssetUrl(
  typeof window !== 'undefined' ? window.__RUNTIME_CONFIG__?.APP_FAVICON_URL : undefined,
  BUILD_ENV?.VITE_APP_FAVICON_URL,
)

// 组合好默认值的最终 logo 地址；组件里 <img :src="brandLogoSrc(bundledLogo)">，
// bundledLogo 为构建期哈希资源（import logo from '@/assets/img/weknora.png'）。
export function brandLogoSrc(bundledLogo: string): string {
  return APP_LOGO_URL || bundledLogo
}
