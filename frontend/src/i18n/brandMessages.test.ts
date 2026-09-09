import assert from 'node:assert/strict'
import test from 'node:test'

import { applyBrandToMessages } from './brandMessages.ts'

// Node 测试环境没有 window.__RUNTIME_CONFIG__，applyBrandToMessages 不传
// brand 时会解析为默认品牌名；覆盖场景通过显式注入 brand 参数测试。
function brand(appName: string, cloudServiceName: string) {
  return { appName, cloudServiceName, cloudServiceNameCompact: cloudServiceName.replace(/\s+/g, '') }
}

test('默认品牌未被覆盖时，消息对象原样返回（同一引用）', () => {
  const messages = { a: 'WeKnora', nested: { b: 'WeKnora Cloud' } }
  assert.equal(applyBrandToMessages(messages), messages)
})

test('覆盖产品名后替换展示名，协议串与生态标识保持不动', () => {
  const messages = {
    welcome: 'Hi，我是 WeKnora，让你的知识触手可及',
    cloudSpaced: '使用 WeKnora Cloud 进行文档解析',
    cloudCompact: 'WeKnoraCloud 凭据已配置',
    webhook: 'HMAC-SHA256 签名（X-WeKnora-Signature）',
    jwt: 'JWT 须包含 sub、tenant_id、aud=weknora、exp',
    clawhub: "Skill 托管在 ClawHub（{'@'}lyingbug/weknora）",
    serviceNames: 'weknora-app / weknora-ui 版本号',
    list: ['WeKnora CLI', 'WeKnora Skill'],
  }
  assert.deepEqual(applyBrandToMessages(messages, brand('Acme', 'WeKnora Cloud')), {
    welcome: 'Hi，我是 Acme，让你的知识触手可及',
    cloudSpaced: '使用 WeKnora Cloud 进行文档解析',
    cloudCompact: 'WeKnoraCloud 凭据已配置',
    webhook: 'HMAC-SHA256 签名（X-WeKnora-Signature）',
    jwt: 'JWT 须包含 sub、tenant_id、aud=weknora、exp',
    clawhub: "Skill 托管在 ClawHub（{'@'}lyingbug/weknora）",
    serviceNames: 'weknora-app / weknora-ui 版本号',
    list: ['Acme CLI', 'Acme Skill'],
  })
})

test('同时覆盖云服务名时，紧凑写法由云服务名去空格派生', () => {
  const messages = {
    cloudSpaced: '使用 WeKnora Cloud 进行文档解析',
    cloudCompact: 'WeKnoraCloud 凭据已配置',
  }
  assert.deepEqual(applyBrandToMessages(messages, brand('Acme', 'Acme Cloud')), {
    cloudSpaced: '使用 Acme Cloud 进行文档解析',
    cloudCompact: 'AcmeCloud 凭据已配置',
  })
})

test('只处理消息 value，i18n 键名不受影响', () => {
  const branded = applyBrandToMessages(
    { weknoraCloud: { title: 'WeKnora Cloud' }, weknoracloud: { desc: 'WeKnora' } },
    brand('Acme', 'WeKnora Cloud'),
  )
  assert.deepEqual(Object.keys(branded), ['weknoraCloud', 'weknoracloud'])
  assert.equal(branded.weknoraCloud.title, 'WeKnora Cloud')
  assert.equal(branded.weknoracloud.desc, 'Acme')
})

test('非字符串叶子（数字/布尔/null）原样保留', () => {
  assert.deepEqual(
    applyBrandToMessages({ n: 1, b: false, nil: null, s: 'WeKnora' }, brand('Acme', 'WeKnora Cloud')),
    { n: 1, b: false, nil: null, s: 'Acme' },
  )
})

test('真实 zh-CN 语言包：覆盖产品名后，剩余 WeKnora 只出现在受保护 token 中', async () => {
  const { default: zhCN } = await import('./locales/zh-CN.ts')
  const branded = applyBrandToMessages(zhCN, brand('Acme', 'WeKnora Cloud'))
  assert.notEqual(branded, zhCN)
  const json = JSON.stringify(branded)
  assert.ok(json.includes('Acme'), '应出现覆盖后的产品名')
  // 摘除三类受保护 token 后，不允许再残留任何 "WeKnora"。
  const residual = json
    .replaceAll('X-WeKnora-Signature', '')
    .replaceAll('WeKnoraCloud', '')
    .replaceAll('WeKnora Cloud', '')
  assert.ok(!residual.includes('WeKnora'), `未预期的残留: ${residual.match(/.{0,20}WeKnora.{0,20}/g)?.join(' | ')}`)
  // 原始语言包对象不被就地修改。
  assert.ok(JSON.stringify(zhCN).includes('Hi，我是 WeKnora'))
})
