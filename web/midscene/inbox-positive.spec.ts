import { mkdirSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { expect } from '@playwright/test'
import { test } from './fixture'
import { records, resetSearch, setCategory, setReadStatus } from './inbox-protable-helpers'

test.use({ video: { mode: 'on', size: { width: 1440, height: 900 } } })

const shots = fileURLToPath(new URL('../../output/midscene/inbox-protable-shots/', import.meta.url))
mkdirSync(shots, { recursive: true })

test('消息中心正向：加载提示、日期区分、原生筛选与全部已读确认', async ({ page, aiAssert, recordToReport }) => {
  test.setTimeout(240_000)
  const items = [
    { id: '3', category: 'organization', body: '组织角色已更新，当前主部门 学习部，岗位 委员。', read: false, createdAt: '2026-11-22T03:00:00Z' },
    { id: '2', category: 'permission', action: 'DISCIPLINE_LEDGER_VIEW', body: '个人授权规则已启用。动作 检查查看，开始日期 2026-11-23。', read: false, createdAt: '2026-11-22T02:00:00Z' },
    { id: '1', category: 'organization', body: '系统角色已调整为管理员。', read: true, createdAt: '2026-11-22T01:00:00Z' },
  ]
  let firstLoad = true
  await page.route('**/api/v1/**', async route => {
    const url = new URL(route.request().url())
    if (url.pathname.endsWith('/auth/refresh')) return route.fulfill({ json: { accessToken: 'fixture' } })
    if (url.pathname.endsWith('/auth/me')) return route.fulfill({ json: { id: 'demo', name: '演示老师', status: 'ACTIVE', systemRole: 'MEMBER' } })
    if (url.pathname.endsWith('/notifications/read')) {
      const input = route.request().postDataJSON()
      for (const item of items) if (input.all || item.id === input.id) item.read = true
      return route.fulfill({ json: { ok: true, changed: 1 } })
    }
    if (url.pathname.endsWith('/notifications')) {
      if (firstLoad) {
        firstLoad = false
        await new Promise(resolve => setTimeout(resolve, 1800))
      }
      const filtered = items.filter(item =>
        (url.searchParams.get('unread') !== 'true' || !item.read) &&
        (!url.searchParams.get('category') || item.category === url.searchParams.get('category')))
      return route.fulfill({
        json: {
          items: filtered,
          total: filtered.length,
          unread: items.filter(item => !item.read).length,
          through: items[0].id,
        },
      })
    }
    return route.fulfill({ status: 403, json: { code: 'FORBIDDEN' } })
  })
  await page.goto('/notifications')
  const pause = () => page.waitForTimeout(3000)

  await expect(page.getByText('共 3 条', { exact: true })).toHaveCount(0)
  await expect(page.getByText('未读 0 条')).toHaveCount(0)
  await page.screenshot({ path: `${shots}/protable-forward-01-loading.png` })
  await recordToReport('正向：加载中提示未读数而不是 0')

  await expect(records(page)).toHaveCount(3)
  await expect(page.getByText('未读 2 条')).toBeVisible()
  await expect(page.getByText('共 3 条', { exact: true })).toBeVisible()
  await expect(page.locator('.inbox-cell-time[dateTime="2026-11-22T02:00:00Z"]').first()).toBeVisible()
  await expect(page.getByText('授权生效', { exact: true })).toBeVisible()
  await pause()
  await page.screenshot({ path: `${shots}/protable-forward-02-initial.png` })
  await recordToReport('正向：消息生成时间与授权生效日期分别标注')

  await setCategory(page, '个人授权')
  await expect(records(page)).toHaveCount(1)
  await expect(records(page).first()).toContainText('个人授权变更')
  await pause()
  await recordToReport('正向：按个人授权类别原生查询后仅保留一条消息')

  await setCategory(page, '全部类别')
  await expect(records(page)).toHaveCount(3)
  await setReadStatus(page, '仅未读')
  await expect(records(page)).toHaveCount(2)
  await pause()
  await recordToReport('正向：阅读状态仅未读筛选隐藏已读消息')

  await page.getByRole('button', { name: '标为已读', exact: true }).first().click()
  await expect(page.getByRole('button', { name: '消息中心，未读 1' })).toBeVisible()
  await expect(records(page)).toHaveCount(1)
  await pause()
  await page.screenshot({ path: `${shots}/protable-forward-03-one-read.png` })
  await recordToReport('正向：单条标为已读后未读计数与列表同步更新')

  await page.getByRole('button', { name: '全部标为已读' }).click()
  const dialog = page.getByRole('dialog')
  await expect(dialog).toContainText('将本人全部消息标为已读？')
  await expect(dialog).toContainText('包含其他类别，不受当前筛选影响')
  await pause()
  await page.screenshot({ path: `${shots}/protable-forward-04-confirm-scope.png` })
  await recordToReport('正向：全部标为已读前明确本人全局范围')
  await dialog.getByRole('button', { name: '确认标为已读' }).click()

  await expect(page.getByRole('button', { name: '消息中心，未读 0' })).toBeVisible()
  await expect(page.getByText('当前仅看未读，切换到全部消息可查看已读记录。')).toBeVisible()
  await pause()
  await page.screenshot({ path: `${shots}/protable-forward-05-empty.png` })
  await recordToReport('正向：全部标为已读后的未读空态与筛选说明')

  await resetSearch(page)
  await expect(records(page)).toHaveCount(3)
  await expect(records(page).filter({ hasText: '已读' })).toHaveCount(3)
  await expect(page.getByText('未读 0 条')).toBeVisible()
  await expect(page.getByText('共 3 条', { exact: true })).toBeVisible()
  await pause()
  await page.screenshot({ path: `${shots}/protable-forward-06-restored.png` })
  await recordToReport('正向：重置恢复完整列表且计数为 0 仅出现在数据已加载后')
  if (process.env.INBOX_DOM_ONLY !== '1') await aiAssert('消息列表显示全部三条消息，均为已读')
})
