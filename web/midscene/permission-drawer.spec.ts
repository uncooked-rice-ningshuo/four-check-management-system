import { expect, test } from '@playwright/test'

for (const width of [1440, 390]) {
  test.describe(`权限详情 · ${width}px`, () => {
    test.use({ viewport: { width, height: 900 } })
    test('角色与个人权限独立翻页，全屏和还原保留当前页', async ({ page }) => {
      const user = { accountId: 'fixture', loginName: 'fixture', name: '分页验收', studentNo: '', status: 'ACTIVE', departmentId: '', organizationRoleId: '', systemRole: 'MEMBER', version: 1, departmentIds: [], isSuperAdmin: false, mustChangePassword: false }
      const rows = Array.from({ length: 12 }, (_, i) => ({ id: String(i), action: 'REPORT_VIEW', scope: {}, sourceDepartmentId: String(i), status: '有效', effect: 'ALLOW', mode: 'ADD', enabled: true }))
      await page.route('**/api/v1/**', route => {
        const path = new URL(route.request().url()).pathname
        const json = (body: unknown) => route.fulfill({ json: body })
        if (path.endsWith('/auth/refresh')) return json({ accessToken: 'fixture' })
        if (path.endsWith('/auth/me')) return json({ ...user, id: 'admin', systemRole: 'SUPER_ADMIN', isSuperAdmin: true })
        if (path.endsWith('/admin/users')) return json([user])
        if (path.endsWith('/catalogs')) return json({ departments: [], organizationRoles: [], inspectionTypes: [] })
        if (path.endsWith('/permission-rules')) return json(rows)
        if (path.endsWith('/permissions')) return json({ permissions: rows, defaultPermissions: rows })
        return json({ permissions: [], items: [], total: 0, page: 1, pageSize: 100 })
      })
      await page.goto('/admin/users')
      const open = page.getByRole('button', { name: '权限详情' })
      await open.click()
      const drawer = page.getByRole('dialog')
      const pagers = drawer.locator('.ant-pagination')
      await expect(pagers).toHaveCount(2)
      const initialWidth = await drawer.evaluate(el => Math.round(el.getBoundingClientRect().width))
      await pagers.first().getByTitle('3', { exact: true }).click()
      await expect(drawer.locator('tbody').first().locator('tr.ant-table-row')).toHaveCount(2)
      await expect(drawer.locator('tbody').last().locator('tr.ant-table-row')).toHaveCount(5)
      await pagers.last().getByTitle('2', { exact: true }).click()
      await drawer.getByRole('button', { name: '全屏查看' }).click()
      await expect.poll(() => drawer.evaluate(el => Math.round(el.getBoundingClientRect().width))).toBe(page.viewportSize()!.width)
      await expect(pagers.first().locator('.ant-pagination-item-active')).toHaveText('3')
      await expect(pagers.last().locator('.ant-pagination-item-active')).toHaveText('2')
      await page.screenshot({ path: test.info().outputPath('全屏权限详情.png') })
      await drawer.getByRole('button', { name: '退出全屏' }).click()
      await expect.poll(() => drawer.evaluate(el => Math.round(el.getBoundingClientRect().width))).toBe(initialWidth)
      await drawer.getByRole('button', { name: '全屏查看' }).click()
      await page.keyboard.press('Escape')
      await open.click()
      await expect(drawer.getByRole('button', { name: '全屏查看' })).toBeVisible()
      await expect(pagers.first().locator('.ant-pagination-item-active')).toHaveText('1')
    })
  })
}

test('处分归档详情全屏时保留来源表页码，关闭后回到列表', async ({ page }) => {
  const sources = Array.from({ length: 9 }, (_, i) => ({ record: String(i), week: i + 1, version: 1, amount: 2 }))
  await page.route('**/api/v1/**', route => {
    const path = new URL(route.request().url()).pathname
    const json = (body: unknown) => route.fulfill({ json: body })
    if (path.endsWith('/auth/refresh')) return json({ accessToken: 'fixture' })
    if (path.endsWith('/auth/me')) return json({ id: 'admin', name: '演示管理员', isSuperAdmin: true, systemRole: 'SUPER_ADMIN', status: 'ACTIVE' })
    if (path.endsWith('/archive/search')) return json({ items: [{ id: 'fixture', studentNumber: '演示001', status: 'APPROVED', tier: '警告', sources }], total: 1 })
    return json({ permissions: [], items: [] })
  })
  await page.goto('/discipline/archive')
  await page.getByRole('button', { name: '详情' }).click()
  const drawer = page.getByRole('dialog')
  await drawer.locator('.ant-pagination').getByTitle('2', { exact: true }).click()
  await drawer.getByRole('button', { name: '全屏查看' }).click()
  await expect(drawer.getByRole('cell', { name: '9', exact: true })).toBeVisible()
  await drawer.getByRole('button', { name: '退出全屏' }).click()
  await expect(drawer.locator('.ant-pagination-item-active')).toHaveText('2')
  await page.keyboard.press('Escape')
  await expect(drawer).toBeHidden()
  await expect(page.getByRole('cell', { name: '演示001' })).toBeVisible()
})
