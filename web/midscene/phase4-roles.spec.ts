import { expect } from '@playwright/test'
import { test } from './fixture'

test.use({ actionTimeout: 12000, video: 'on', launchOptions: { channel: 'chrome', slowMo: 350 } })
test.skip(!process.env.PHASE4_TEST_PASSWORD, '使用 phase4-roles 隔离数据库')
test('班级内外模板老师和个人授权学生的收件与审批', async ({ browser, baseURL, request }) => {
  let caseId = ''
  for (const login of ['discipline-member-revoke-in', 'discipline-teacher-in', 'discipline-teacher-out', 'discipline-member-out', 'discipline-member-in']) {
    const context = await browser.newContext({ baseURL, viewport: { width: 1440, height: 900 },
      recordVideo: { dir: test.info().outputDir, size: { width: 1440, height: 900 } } })
    const page = await context.newPage()
    const video = page.video()
    try {
      const auth = await context.request.post('/api/v1/auth/login', { data: { loginName: login, password: process.env.PHASE4_TEST_PASSWORD } })
      expect(auth.status()).toBe(200)
      const headers = { Authorization: 'Bearer ' + (await auth.json()).accessToken }
      const authorized = login.endsWith('-in')
      const messages = await context.request.get('/api/v1/notifications?category=discipline', { headers })
      expect(messages.status()).toBe(200)
      const inbox = await messages.json()
      expect(inbox.total > 0).toBe(authorized)
      await page.goto('/notifications')
      if (authorized) await expect(page.getByRole('button', { name: '处分事项', exact: true }).first()).toBeVisible()
      else await expect(page.getByRole('button', { name: '处分事项', exact: true })).toHaveCount(0)
      await page.waitForTimeout(1000)
      await page.goto('/discipline/review')
      const result = await context.request.post('/api/v1/discipline/cases/search', { headers, data: { page: 1, pageSize: 20 } })
      expect(result.status()).toBe(200)
      const queue = await result.json()
      if (authorized) {
        expect(queue.total).toBe(1)
        caseId = queue.items[0].id
        const notice = inbox.items.find((item: { caseId: string }) => item.caseId === caseId)
        expect(notice).toBeDefined()
        await page.goto('/notifications')
        await page.locator('tr[data-row-key="' + notice.id + '"]').getByRole('button', { name: '处分事项' }).click()
        await page.getByRole('link', { name: '查看对应事项' }).click()
        await expect(page.getByRole('heading', { name: '待处分' })).toBeVisible()
        const action = login.includes('teacher') ? '延迟处理' : '确认处分'
        await page.getByRole('button', { name: action }).click()
        const dialog = page.getByRole('dialog')
        if (action === '延迟处理') await dialog.getByLabel('延迟原因').fill('等待核实原始检查记录')
        if (login === 'discipline-member-revoke-in') {
          const me = await context.request.get('/api/v1/auth/me', { headers })
          const path = '/api/v1/admin/users/' + (await me.json()).id + '/permission-rules'
          const admin = await request.post('/api/v1/auth/login', { data: {loginName:'notify-admin',password:process.env.PHASE4_TEST_PASSWORD} })
          const adminHeaders = {Authorization:'Bearer ' + (await admin.json()).accessToken}
          const rules = await request.get(path,{headers:adminHeaders})
          const rule = (await rules.json()).find((entry: { action: string }) => entry.action === 'DISCIPLINE_CASE_CONFIRM')
          expect((await request.delete(path + '/' + rule.id,{headers:adminHeaders})).status()).toBe(204)
          const submission = page.waitForResponse(r => r.url().endsWith('/review') && r.request().method() === 'POST')
          await dialog.getByRole('button', {name:/确\s*定/}).click()
          expect([401,403]).toContain((await submission).status())
          continue
        }
        await page.waitForTimeout(1000)
        await dialog.getByRole('button', { name: /确\s*定/ }).click()
        await expect(dialog).toHaveCount(0)
        if (action === '延迟处理') await expect(page.getByRole('cell', { name: '等待核实原始检查记录' })).toBeVisible()
        else await expect(page.getByText('当前范围暂无待处分事项')).toBeVisible()
      } else {
        expect(queue.total).toBe(0)
        await expect(page.getByText('当前范围暂无待处分事项')).toBeVisible()
        const denied = await context.request.post('/api/v1/discipline/cases/' + caseId + '/review', {
          headers, data: { decision: 'APPROVED', version: 2, reason: '' },
        })
        expect(denied.status()).toBe(403)
      }
      await page.screenshot({ path: test.info().outputPath(login + '.png') })
    } finally {
      await context.close()
      if (video) {
        await video.saveAs(test.info().outputPath(login + '.webm'))
        await video.delete()
      }
    }
  }
})

test('超管核对归档作废依据并导出', async ({ page }) => {
  const login = await page.request.post('/api/v1/auth/login', {
    data: { loginName: 'notify-admin', password: process.env.PHASE4_TEST_PASSWORD },
  })
  expect(login.status()).toBe(200)
  await page.goto('/discipline/ledger')
  await page.getByRole('combobox', { name: '学年与学期' }).click()
  await page.locator('.ant-select-item-option').filter({ hasText: 'Fixture semester' }).first().click()
  await page.getByRole('button', { name: '处分事项' }).first().click()
  await expect(page.getByRole('dialog').getByText('已作废').first()).toBeVisible()
  await page.waitForTimeout(1200)
  await page.keyboard.press('Escape')
  await page.goto('/discipline/archive')
  const row = page.getByRole('row').filter({ hasText: '已作废' }).first()
  await row.getByRole('button', { name: '详情' }).click()
  await expect(page.getByText('作废原因', { exact: true })).toBeVisible()
  await expect(page.getByText(/规则更新|定稿更正后未达到该档位/).first()).toBeVisible()
  await page.waitForTimeout(1200)
  await page.screenshot({ path: test.info().outputPath('archive-detail.png') })
  await page.keyboard.press('Escape')
  const download = page.waitForEvent('download')
  await page.getByRole('button', { name: '导出当前结果' }).click()
  expect((await download).suggestedFilename()).toBe('处分归档.xlsx')
})
