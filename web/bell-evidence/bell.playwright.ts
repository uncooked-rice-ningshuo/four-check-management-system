import { expect, test } from '@playwright/test'
import { mkdirSync, writeFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

const finalDir = fileURLToPath(new URL('../../output/bell-evidence-final/', import.meta.url))
mkdirSync(finalDir, { recursive: true })

type Metrics = Record<string, { height: number; lineHeight: string; fontSize: string } | null | string>

async function measure(page: import('@playwright/test').Page): Promise<Metrics> {
  return page.evaluate(() => {
    const pick = (selector: string) => {
      const el = document.querySelector(selector)
      if (!el) return null
      const cs = getComputedStyle(el)
      const r = el.getBoundingClientRect()
      return {
        height: Math.round(r.height * 10) / 10,
        lineHeight: cs.lineHeight,
        fontSize: cs.fontSize,
      }
    }
    const header = document.querySelector('.admin-header')
    return {
      headerLineHeight: header ? getComputedStyle(header).lineHeight : null,
      panel: pick('.bell-panel'),
      head: pick('.bell-panel__head'),
      headStrong: pick('.bell-panel__head strong'),
      tabs: pick('.bell-panel__tabs'),
      state: pick('.bell-panel__state'),
      list: pick('.bell-panel__list'),
      firstItem: pick('.bell-item'),
    }
  })
}

async function openPanel(page: import('@playwright/test').Page) {
  await page.getByRole('button', { name: /消息中心/ }).hover()
  await expect(page.locator('#bell-panel')).toBeVisible()
}

test('铃铛浮层：同宽三态截图、实际高度、切换/焦点/移动端慢速录屏', async ({ page }, testInfo) => {
  test.setTimeout(120_000)
  const report: Record<string, unknown> = {}

  // 1. 空态（与有数据同宽）
  await page.goto('/?mode=empty')
  await openPanel(page)
  await page.waitForTimeout(600)
  await expect(page.getByText('暂无消息', { exact: true })).toBeVisible()
  await page.screenshot({ path: `${finalDir}01-empty-1440.png` })
  report.empty = await measure(page)
  const unreadTabInEmpty = page.getByRole('tab', { name: /未读/ })
  await unreadTabInEmpty.click()
  await expect(page.getByText('暂无未读消息', { exact: true })).toBeVisible()
  report.emptyUnread = await measure(page)

  // 2. 有数据（全部页签）
  await page.goto('/?mode=data')
  await openPanel(page)
  await page.waitForTimeout(700)
  await expect(page.getByRole('tab', { name: /全部/ })).toHaveAttribute('aria-selected', 'true')
  await expect(page.getByText('部门归属已调整为计算机学院学生会文艺部')).toBeVisible()
  await page.screenshot({ path: `${finalDir}02-data-1440.png` })
  report.data = await measure(page)
  const scroll = await page.locator('.bell-panel__list').evaluate(el => ({
    scrollHeight: el.scrollHeight,
    clientHeight: el.clientHeight,
  }))
  expect(scroll.scrollHeight).toBeGreaterThan(scroll.clientHeight)
  report.dataScroll = scroll

  // 3. 未读页签（服务端 filter 后的数据）
  await page.getByRole('tab', { name: /未读/ }).click()
  await page.waitForTimeout(700)
  await expect(page.getByRole('tab', { name: /未读/ })).toHaveAttribute('aria-selected', 'true')
  await expect(page.getByText('个人授权调整：通报查看范围已更新')).toHaveCount(0)
  await expect(page.getByText('部门归属已调整为计算机学院学生会文艺部')).toBeVisible()
  await page.screenshot({ path: `${finalDir}03-unread-1440.png` })
  report.unread = await measure(page)

  // 4. 切换往返：反复点击不应产生已读写入请求；焦点仍在面板内时移开鼠标不关闭。
  const writeRequests: string[] = []
  page.on('request', req => {
    if (req.url().includes('/notifications/read')) writeRequests.push(req.url())
  })
  for (let i = 0; i < 3; i++) {
    await page.getByRole('tab', { name: /全部/ }).click()
    await page.getByRole('tab', { name: /未读/ }).click()
  }
  await page.waitForTimeout(400)
  report.writeRequests = writeRequests

  const viewAll = page.getByRole('link', { name: '查看全部' })
  await viewAll.focus()
  await expect(viewAll).toBeFocused()
  await page.mouse.move(2, 2)
  await page.waitForTimeout(400)
  await expect(page.locator('#bell-panel')).toBeVisible()
  await page.keyboard.press('ArrowRight')
  await page.waitForTimeout(300)

  // 5. Escape 关闭并把焦点还给铃铛
  await page.keyboard.press('Escape')
  await expect(page.locator('#bell-panel')).toBeHidden()
  await expect(page.getByRole('button', { name: /消息中心/ })).toBeFocused()
  await page.waitForTimeout(400)

  // 6. 移动端（390px）空态与有数据
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/?mode=empty')
  await openPanel(page)
  await page.waitForTimeout(500)
  await page.screenshot({ path: `${finalDir}04-empty-390.png` })
  report.mobileEmpty = await measure(page)
  await page.goto('/?mode=data')
  await openPanel(page)
  await page.waitForTimeout(500)
  await page.screenshot({ path: `${finalDir}05-data-390.png` })
  report.mobileData = await measure(page)

  writeFileSync(`${finalDir}metrics.json`, JSON.stringify(report, null, 2))

  // 录像由 Playwright 在用例结束后写入 testInfo.outputDir，汇总步骤再复制到稳定目录。
  console.log(`video outputDir: ${testInfo.outputDir}`)
  expect(writeRequests).toHaveLength(0)
})
