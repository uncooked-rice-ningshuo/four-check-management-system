import { mkdirSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { expect } from '@playwright/test'
import { test } from './fixture'
import { records, searchForm } from './inbox-protable-helpers'

test.use({ video: { mode: 'on', size: { width: 1440, height: 900 } } })

// 新截图写入独立目录，保留旧卡片布局截图用于对照。
const shots = fileURLToPath(new URL('../../output/midscene/inbox-protable-shots/', import.meta.url))
mkdirSync(shots, { recursive: true })

test('消息中心 ProTable 宽窄屏、806px 卡宽与键盘焦点', async ({ page, aiAssert }) => {
  test.setTimeout(180_000)
  const items = [
    { id: '2', category: 'organization', body: '部门与角色已更新，即时生效。当前主部门：学习部；学习部（老师）', read: false, createdAt: '2026-11-22T02:00:00Z' },
    { id: '1', category: 'permission', body: '个人授权规则已启用。动作 检查查看，开始日期 2026-11-23。', read: false, createdAt: '2026-11-22T01:00:00Z' },
  ]
  await page.route('**/api/v1/**', async route => {
    const url = new URL(route.request().url())
    if (url.pathname.endsWith('/auth/refresh')) return route.fulfill({ json: { accessToken: 'fixture' } })
    if (url.pathname.endsWith('/auth/me')) return route.fulfill({ json: { id: 'demo', name: '演示老师', status: 'ACTIVE', systemRole: 'MEMBER' } })
    if (url.pathname.endsWith('/notifications')) return route.fulfill({
      json: { items, total: items.length, unread: items.filter(item => !item.read).length, through: '2' },
    })
    return route.fulfill({ status: 403, json: { code: 'FORBIDDEN' } })
  })

  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/notifications')
  await expect(records(page)).toHaveCount(2)
  await expect(searchForm(page)).toBeVisible()
  await page.screenshot({ path: `${shots}/protable-1440.png`, fullPage: true })

  // 1122 = 256 展开侧栏 + 30×2 内容内边距 + 806 卡片宽度。
  await page.setViewportSize({ width: 1122, height: 900 })
  await expect.poll(() => page.locator('.inbox-table').evaluate(el => Math.round(el.getBoundingClientRect().width))).toBe(806)
  await page.screenshot({ path: `${shots}/protable-806-card.png`, fullPage: true })

  // 行内操作获得焦点时整行有可见描边，按钮本身保持焦点。
  const firstMark = page.getByRole('button', { name: '标为已读', exact: true }).first()
  await firstMark.focus()
  await expect(firstMark).toBeFocused()
  await expect(records(page).first().locator('td').first()).toHaveCSS('outline-style', 'solid')
  await page.screenshot({ path: `${shots}/protable-keyboard-focus.png` })

  const sider = page.locator('#admin-navigation')
  for (const width of [700, 390]) {
    await page.setViewportSize({ width, height: 900 })
    await expect(sider).toHaveClass(/ant-layout-sider-collapsed/)
    await expect.poll(() => sider.evaluate(el => Math.round(el.getBoundingClientRect().width))).toBe(80)
    await expect(records(page)).toHaveCount(2)
    const noHorizontalOverflow = await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)
    expect(noHorizontalOverflow).toBe(true)
    await expect(page.getByRole('button', { name: '全部标为已读' })).toBeVisible()
    await expect(searchForm(page).getByRole('combobox', { name: '消息类别' })).toBeVisible()
    await expect(page.getByRole('link', { name: '工作台' })).toBeVisible()
    await page.screenshot({ path: `${shots}/protable-${width}.png`, fullPage: true })
    const tableMetrics = await page.locator('.inbox-page .ant-table-container').evaluate(el => ({ scroll: el.scrollWidth, client: el.clientWidth }))
    console.log(`viewport ${width} table metrics`, tableMetrics)
    await page.locator('.inbox-page').screenshot({ path: `${shots}/protable-${width}-page.png` })
    if (width === 390) {
      await page.locator('#main-content').evaluate(el => { el.scrollTop = el.scrollHeight })
      await page.waitForTimeout(300)
      await page.screenshot({ path: `${shots}/protable-390-table.png` })
    }
  }

  // 铃铛浮窗保持紧凑：宽度在 320–336px，条目间行距 8px。
  await page.setViewportSize({ width: 1440, height: 900 })
  const bell = page.getByRole('button', { name: '消息中心，未读 2' })
  await bell.hover()
  const panel = page.getByRole('dialog', { name: '近期消息' })
  await expect(panel).toBeVisible()
  const width = await panel.evaluate(el => Math.round(el.getBoundingClientRect().width))
  expect(width).toBeGreaterThanOrEqual(320)
  expect(width).toBeLessThanOrEqual(336)
  await expect(panel.locator('.bell-item').first()).toHaveCSS('gap', '8px')
  await page.screenshot({ path: `${shots}/protable-bell.png` })

  if (process.env.INBOX_DOM_ONLY !== '1') await aiAssert('806px 卡片与 390px 窄屏下原生筛选、工具栏、分页和焦点均可触达，铃铛浮窗紧凑')
})
