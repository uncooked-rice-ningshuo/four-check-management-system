import { expect } from '@playwright/test'
import { test } from './fixture'
import { records, setCategory } from './inbox-protable-helpers'

test.use({ video: 'on', launchOptions: { channel: 'chrome', slowMo: 350 } })

test('处分消息可筛选和阅读，打开详情不提交审批', async ({ page }) => {
  const body = '有新的待处分事项，请在待处分页面核对。该学生另有已延迟事项。'
  const writes: string[] = []
  const categories: string[] = []
  await page.route('**/api/v1/**', async route => {
    const request = route.request()
    const url = new URL(request.url())
    if (url.pathname.endsWith('/auth/refresh')) return route.fulfill({ json: { accessToken: 'fixture' } })
    if (url.pathname.endsWith('/auth/me')) return route.fulfill({ json: { id: 'reviewer', name: '测试经办人', status: 'ACTIVE', systemRole: 'MEMBER' } })
    if (request.method() !== 'GET') writes.push(url.pathname)
    if (url.pathname.endsWith('/notifications')) {
      categories.push(url.searchParams.get('category') ?? '')
      return route.fulfill({ json: { items: [{ id: '1', body, category: 'discipline', read: false, createdAt: '2026-11-22T02:00:00Z' }], total: 1, unread: 1, through: '1', page: 1, pageSize: 20, sort: 'newest' } })
    }
    return route.fulfill({ status: 403, json: { code: 'FORBIDDEN' } })
  })
  await page.goto('/notifications')
  await setCategory(page, '处分事项')
  await expect.poll(() => categories.includes('discipline')).toBe(true)
  await records(page).first().getByRole('button', { name: '处分事项' }).click()
  const detail = page.getByRole('region', { name: '消息详情' })
  await expect(detail.getByText(body, { exact: true })).toBeVisible()
  await expect(detail.getByRole('button', { name: '标为已读' })).toBeEnabled()
  await page.waitForTimeout(2000)
  await page.screenshot({ path: '../output/midscene/discipline-inbox-detail.png' })
  expect(writes).toEqual([])
})
