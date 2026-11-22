// 隔离证据测量脚本：在真实 Ant Layout.Header 内测量铃铛浮层的实际排版。
import { chromium } from '@playwright/test'
import { mkdirSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

const root = fileURLToPath(new URL('../../output/bell-evidence/', import.meta.url))
mkdirSync(root, { recursive: true })

const browser = await chromium.launch({ channel: 'chrome', headless: true })
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } })
await page.goto('http://127.0.0.1:5199/?mode=empty')
await page.getByRole('button', { name: /消息中心/ }).hover()
await page.locator('#bell-panel').waitFor()

const metrics = await page.evaluate(() => {
  const pick = (selector) => {
    const el = document.querySelector(selector)
    if (!el) return null
    const cs = getComputedStyle(el)
    const r = el.getBoundingClientRect()
    return {
      height: Math.round(r.height * 10) / 10,
      fontSize: cs.fontSize,
      lineHeight: cs.lineHeight,
      padding: cs.padding,
    }
  }
  const header = document.querySelector('.admin-header')
  return {
    headerLineHeight: header ? getComputedStyle(header).lineHeight : null,
    panel: pick('.bell-panel'),
    head: pick('.bell-panel__head'),
    headStrong: pick('.bell-panel__head strong'),
    state: pick('.bell-panel__state'),
  }
})
console.log(JSON.stringify(metrics, null, 2))
await page.screenshot({ path: `${root}before-empty.png` })
await browser.close()
