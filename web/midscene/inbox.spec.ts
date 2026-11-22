import { mkdirSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { expect } from '@playwright/test'
import { test } from './fixture'
import { records, resetSearch, setCategory, setReadStatus } from './inbox-protable-helpers'

test.use({ video: { mode: 'on', size: { width: 1440, height: 900 } } })

const shots = fileURLToPath(new URL('../../output/midscene/inbox-protable-shots/', import.meta.url))
mkdirSync(shots, { recursive: true })

test('消息中心反向：失败提示、重试与原生查询回退', async ({ page, aiAssert, recordToReport }) => {
  test.setTimeout(240_000)
  let allowLoad = false
  let readFailed = true
  const items = [
    { id: '2', category: 'organization', body: '部门与角色已更新，即时生效。当前主部门：学习部；学习部（老师）', read: false, createdAt: '2026-11-22T02:00:00Z' },
    { id: '1', category: 'permission', body: '个人授权规则已启用。动作 检查查看，开始日期 2026-11-23。', read: false, createdAt: '2026-11-22T01:00:00Z' },
  ]
  await page.route('**/api/v1/**', async route => {
    const url = new URL(route.request().url())
    if (url.pathname.endsWith('/auth/refresh')) return route.fulfill({ json: { accessToken: 'fixture' } })
    if (url.pathname.endsWith('/auth/me')) return route.fulfill({ json: { id: 'demo', name: '演示老师', status: 'ACTIVE', systemRole: 'MEMBER' } })
    if (url.pathname.endsWith('/notifications/read')) {
      if (readFailed) { readFailed = false; return route.fulfill({ status: 500, json: { code: 'INBOX_FAILED' } }) }
      const input = route.request().postDataJSON()
      for (const item of items) if (input.all || item.id === input.id) item.read = true
      return route.fulfill({ json: { ok: true, changed: 1 } })
    }
    if (url.pathname.endsWith('/notifications')) {
      if (!allowLoad) return route.fulfill({ status: 500, json: { code: 'INBOX_FAILED' } })
      const filtered = items.filter(item =>
        (url.searchParams.get('unread') !== 'true' || !item.read) &&
        (!url.searchParams.get('category') || item.category === url.searchParams.get('category')))
      return route.fulfill({ json: { items: filtered, total: filtered.length, unread: items.filter(item => !item.read).length, through: '2' } })
    }
    return route.fulfill({ status: 403, json: { code: 'FORBIDDEN' } })
  })
  await page.goto('/notifications')
  const pause = () => page.waitForTimeout(3000)

  await expect(page.getByText('读取消息失败', { exact: true })).toBeVisible({ timeout: 20000 })
  await expect(page.getByText('未读数读取失败')).toBeVisible()
  await expect(page.getByText('共 0 条', { exact: true })).toHaveCount(0)
  await expect(page.getByText('未读 0 条')).toHaveCount(0)
  await page.screenshot({ path: `${shots}/protable-reverse-01-load-failed.png` })
  await recordToReport('反向：读取失败时页头与工具栏不把计数兜底为 0')
  await pause()

  allowLoad = true
  await page.getByRole('alert').filter({ hasText: '读取消息失败' }).getByRole('button', { name: /重\s*试/ }).click()
  await expect(records(page).filter({ hasText: '部门与角色变更' })).toBeVisible()
  await expect(page.getByText('未读 2 条')).toBeVisible()
  await expect(page.getByText('共 2 条', { exact: true })).toBeVisible()
  await pause()
  await page.screenshot({ path: `${shots}/protable-reverse-02-recovered.png` })
  await recordToReport('反向：重试成功后恢复真实计数')

  await page.getByRole('button', { name: '标为已读', exact: true }).first().click()
  await expect(page.getByText('更新失败，请重试')).toBeVisible()
  await expect(page.getByRole('button', { name: '标为已读', exact: true })).toHaveCount(2)
  await pause()
  await recordToReport('反向：更新失败保留未读状态')
  await page.getByRole('button', { name: '标为已读', exact: true }).first().click()
  await expect(page.getByRole('button', { name: '消息中心，未读 1' })).toBeVisible()
  await pause()

  await setReadStatus(page, '仅未读')
  await expect(records(page).filter({ hasText: '个人授权变更' })).toBeVisible()
  await page.getByRole('button', { name: '全部标为已读' }).click()
  await expect(page.getByRole('dialog')).toContainText('不受当前筛选影响')
  await page.getByRole('button', { name: '确认标为已读' }).click()
  await expect(page.getByText('当前仅看未读，切换到全部消息可查看已读记录。')).toBeVisible()
  await expect(page.getByText('未读 0 条')).toBeVisible()
  await pause()
  await page.screenshot({ path: `${shots}/protable-reverse-03-unread-empty.png` })
  await recordToReport('反向：确认全局范围后仅看未读为空')

  await resetSearch(page)
  await expect(records(page)).toHaveCount(2)
  await setCategory(page, '个人授权')
  await expect(records(page)).toHaveCount(1)
  await pause()
  await page.screenshot({ path: `${shots}/protable-reverse-04-category-back.png` })
  await recordToReport('反向：从空态回退为全部消息再按类别查看')

  await setCategory(page, '全部类别')
  await expect(records(page)).toHaveCount(2)
  await expect(page.getByText('共 2 条', { exact: true })).toBeVisible()
  await pause()
  await page.screenshot({ path: `${shots}/protable-reverse-05-final.png` })
  await recordToReport('反向：回到全部类别与全部消息，计数与内容一致')
  if (process.env.INBOX_DOM_ONLY !== '1') await aiAssert('消息列表显示两条已读消息')
})
