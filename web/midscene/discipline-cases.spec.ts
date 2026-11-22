import { expect } from '@playwright/test'
import { test } from './fixture'

test.use({ video: 'on', launchOptions: { channel: 'chrome', slowMo: 350 } })

test('台账明细显示两档独立事项，换人后显示空态', async ({ page }) => {
  const cases = ['通报批评', '警告'].map((tier, i) => ({
    id: String(i + 1), tier, amount: 4, status: 'PENDING', week: 3, version: 2, dueAt: '2026-11-24T08:00:00Z',
  }))
  await page.route('**/api/v1/**', route => {
    const url = new URL(route.request().url())
    const json = (value: unknown) => route.fulfill({ json: value })
    if (url.pathname.endsWith('/auth/refresh')) return json({ accessToken: 'fixture' })
    if (url.pathname.endsWith('/auth/me')) return json({ id: 'demo', name: '演示老师', status: 'ACTIVE', systemRole: 'MEMBER' })
    if (url.pathname.endsWith('/permissions/effective')) return json({ permissions: [{ action: 'DISCIPLINE_LEDGER_VIEW', effect: 'ALLOW', scope: {} }] })
    if (url.pathname.endsWith('/ledger/detail')) {
      const { kind, studentNumber, page, pageSize, status } = route.request().postDataJSON()
      const rows = kind === 'cases' && studentNumber === '1' && (!status || status === 'PENDING')
        ? [...cases, ...Array.from({length: 10}, (_, i) => ({...cases[0], id: 'older-' + i, tier: '历史档位 ' + i}))] : []
      return json({tiers: cases.map(item => item.tier), items: rows.slice((page-1)*pageSize,page*pageSize), total: rows.length, page,pageSize})
    }
    if (url.pathname.endsWith('/ledger/search')) return json({
      semesters: [{ id: 'semester', name: '演示学期' }], weeks: [3], columns: [], organizations: [], ruleVersion: 1,
      items: ['跨档示例', '未达档示例'].map((name, i) => ({ number: String(i + 1), name, grade: '2026级',
        className: '演示班', sources: [], values: {}, cases: i ? [] : cases })),
      total: 2, unassigned: 0, page: 1, pageSize: 50,
    })
    return route.fulfill({ status: 403, json: { code: 'FORBIDDEN' } })
  })
  await page.goto('/discipline/ledger')
  await page.getByLabel('学年与学期').click()
  await page.getByText('演示学期', { exact: true }).last().click()
  const student = page.getByRole('row').filter({ has: page.getByRole('button', { name: '跨档示例', exact: true }) })
  await expect(student).toContainText('警告 · 待处理')
  await student.getByRole('button', { name: '检查记录', exact: true }).click()
  await expect(page.getByRole('dialog')).toContainText('跨档示例 · 检查记录')
  await page.keyboard.press('Escape')
  await student.getByRole('button', { name: '处分事项', exact: true }).click()
  const drawer = page.getByRole('dialog')
  for (const tier of ['通报批评', '警告']) {
    const row = drawer.getByRole('row').filter({ hasText: tier })
    await expect(row).toContainText('第3周 · V2')
    await expect(row).toContainText('待处理')
    await expect(row).toContainText('2026')
  }
  await drawer.locator('.ant-pagination-item-2').click()
  await expect(drawer.getByText('历史档位 9')).toBeVisible()
  await drawer.getByRole('combobox', { name: '状态' }).click()
  await page.locator('.ant-select-item-option').filter({hasText:'已延迟'}).click()
  await drawer.getByRole('button', {name: /查\s*询/}).click()
  await expect(drawer.getByText('本学期暂无处分事项')).toBeVisible()
  await page.waitForTimeout(2200)
  await page.keyboard.press('Escape')
  await page.getByRole('row').filter({ hasText: '未达档示例' }).getByRole('button', { name: '处分事项', exact: true }).click()
  await expect(drawer.getByText('本学期暂无处分事项')).toBeVisible()
  await expect(drawer.getByText('通报批评')).toHaveCount(0)
  await page.waitForTimeout(1800)
})
