import { mkdirSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { expect } from '@playwright/test'
import { test } from './fixture'
import { records, setReadStatus } from './inbox-protable-helpers'

test.use({ video: { mode: 'on', size: { width: 1440, height: 900 } } })

const shots = fileURLToPath(new URL('../../output/midscene/inbox-protable-shots/', import.meta.url))
mkdirSync(shots, { recursive: true })

test('消息中心末页回退：读掉仅看未读末项后回到有效页', async ({ page, aiAssert, recordToReport }) => {
  test.setTimeout(240_000)
  const items = Array.from({ length: 21 }, (_, index) => ({
    id: String(21 - index),
    category: 'organization',
    body: `部门变更 ${21 - index}`,
    read: false,
    createdAt: '2026-11-22T02:00:00Z',
  }))
  await page.route('**/api/v1/**', async route => {
    const url = new URL(route.request().url())
    if (url.pathname.endsWith('/auth/refresh')) return route.fulfill({ json: { accessToken: 'fixture' } })
    if (url.pathname.endsWith('/auth/me')) return route.fulfill({ json: { id: 'demo', name: '演示老师', status: 'ACTIVE', systemRole: 'MEMBER' } })
    if (url.pathname.endsWith('/notifications/read')) {
      const input = route.request().postDataJSON()
      for (const item of items) if (input.all || item.id === input.id) item.read = true
      return route.fulfill({ json: { ok: true } })
    }
    if (url.pathname.endsWith('/notifications')) {
      const unreadOnly = url.searchParams.get('unread') === 'true'
      const pageNumber = Number(url.searchParams.get('page') ?? '1')
      const visible = items.filter(item => !unreadOnly || !item.read)
      return route.fulfill({
        json: {
          items: visible.slice((pageNumber - 1) * 20, pageNumber * 20),
          total: visible.length,
          unread: items.filter(item => !item.read).length,
          through: '21',
        },
      })
    }
    return route.fulfill({ status: 403, json: { code: 'FORBIDDEN' } })
  })
  await page.goto('/notifications')
  const pause = () => page.waitForTimeout(3000)

  await expect(records(page)).toHaveCount(20)
  await expect(page.getByText('共 21 条', { exact: true })).toBeVisible()
  await pause()

  await setReadStatus(page, '仅未读')
  await expect(records(page)).toHaveCount(20)
  await expect(page.getByText('未读 21 条')).toBeVisible()
  await expect(page.locator('.ant-pagination-item-2')).toBeVisible()
  await pause()
  await page.screenshot({ path: `${shots}/protable-pagination-01-unread-page1.png` })
  await recordToReport('末页回退：仅看未读第一页共 21 条，存在第二页')

  await page.locator('.ant-pagination-item-2').click()
  await expect(records(page)).toHaveCount(1)
  await expect(records(page).first()).toContainText('部门变更 1')
  await pause()
  await page.screenshot({ path: `${shots}/protable-pagination-02-unread-lastpage.png` })
  await recordToReport('末页回退：切换到仅看未读的第二页，仅剩一条')

  await page.getByRole('button', { name: '标为已读', exact: true }).click()
  await expect(page.locator('.ant-pagination-item-2')).toHaveCount(0)
  await expect(records(page)).toHaveCount(20)
  await expect(page.getByText('未读 20 条')).toBeVisible()
  await expect(page.getByText('共 20 条', { exact: true })).toBeVisible()
  await expect(page.locator('.ant-pagination-item-active')).toContainText('1')
  await pause()
  await page.screenshot({ path: `${shots}/protable-pagination-03-clamped.png` })
  await recordToReport('末页回退：读掉最后一项后自动回到有效第一页，没有空列表')

  await setReadStatus(page, '全部')
  await expect(page.getByText('共 21 条', { exact: true })).toBeVisible()
  await expect(records(page)).toHaveCount(20)
  await page.locator('.ant-pagination-item-2').click()
  await expect(records(page)).toHaveCount(1)
  await pause()
  await page.screenshot({ path: `${shots}/protable-pagination-04-back-to-page2.png` })
  await recordToReport('末页回退：切回全部阅读状态后仍可进入第二页查看已读消息')

  await page.locator('.ant-pagination-item-1').click()
  await expect(records(page)).toHaveCount(20)
  await expect(page.locator('.ant-pagination-item-active')).toContainText('1')
  await pause()
  await page.screenshot({ path: `${shots}/protable-pagination-05-final.png` })
  await recordToReport('末页回退：返回第一页后列表与分页状态一致')
  if (process.env.INBOX_DOM_ONLY !== '1') await aiAssert('仅看未读末项读掉后自动回到第一页并显示 20 条未读消息')
})
