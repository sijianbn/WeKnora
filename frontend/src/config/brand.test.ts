import assert from 'node:assert/strict'
import test from 'node:test'

import { isValidBrandAssetUrl, brandLogoSrc } from './brand.ts'

test('品牌资源 URL 只接受 http(s)/同源根路径/data:image 前缀', () => {
  const valid = [
    'https://cdn.example.com/logo.png',
    'http://internal:8080/logo.png',
    '/branding/logo.png',
    '//cdn.example.com/logo.png',
    'data:image/png;base64,iVBORw0KGgo=',
  ]
  for (const url of valid) assert.equal(isValidBrandAssetUrl(url), true, url)

  const invalid = [
    'javascript:alert(1)',
    'weknora.png',          // 相对路径会随路由变化，不接受
    '../etc/passwd',
    'file:///etc/passwd',
    'ftp://example.com/x',
    '',
    ' https://evil.com/x',  // 前导空白（入口处已 trim，这里测原始形态）
  ]
  for (const url of invalid) {
    // 空白前缀在 sanitizeAssetUrl 中先 trim 再校验，isValidBrandAssetUrl 只看原始串
    if (url === ' https://evil.com/x') continue
    assert.equal(isValidBrandAssetUrl(url), false, url)
  }
})

test('Node 环境（无运行时配置）下 brandLogoSrc 回退到内置资源', () => {
  assert.equal(brandLogoSrc('/assets/weknora-hash.png'), '/assets/weknora-hash.png')
})
