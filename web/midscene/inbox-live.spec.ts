import { expect } from '@playwright/test'
import { test } from './fixture'
import { records } from './inbox-protable-helpers'

test.use({ video: { mode: 'on', size: { width: 1440, height: 900 } } })
test('账号角色调整后，收件人可阅读自己的消息', async ({ page, request, aiAssert, recordToReport }) => {
  test.skip(!process.env.INBOX_ACCOUNT, '需要隔离服务与虚构账号')
  const password = process.env.INBOX_PASSWORD
  expect((await request.get('/api/v1/notifications')).status()).toBe(401)
  const login = await request.post('/api/v1/auth/login', { data: { loginName: 'browser-admin', password } })
  expect(login.status()).toBe(200)
  const headers = { Authorization: 'Bearer ' + (await login.json()).accessToken }
  const changed = await request.post('/api/v1/auth/change-password', { headers, data: { currentPassword: password, newPassword: password + 'A' } })
  expect(changed.status()).toBe(204)
  const active = await request.post('/api/v1/auth/login', { data: { loginName: 'browser-admin', password: password + 'A' } })
  expect(active.status()).toBe(200)
  headers.Authorization = 'Bearer ' + (await active.json()).accessToken
  const endpoint = '/api/v1/admin/users/' + process.env.INBOX_ACCOUNT
  const change = { headers, data: { studentNo: 'demo-recipient', name: '演示成员', systemRole: 'ADMIN', version: 1 } }
  const updated = await request.put(endpoint, change)
  expect(updated.status(), updated.ok() ? '' : (await updated.json()).code).toBe(200)
  expect((await request.put(endpoint, change)).status()).toBe(409)
  const departmentId = '10000000-0000-0000-0000-000000000002'
  const assigned = await request.put(endpoint + '/assignment', { headers, data: {
    version: 2, departmentId, departmentIds: [departmentId], organizationRoleId: '20000000-0000-0000-0000-000000000004',
  } })
  expect(assigned.status()).toBe(204)
  const rules = endpoint + '/permission-rules'
  const grant = { action: 'DISCIPLINE_LEDGER_VIEW', effect: 'ALLOW', mode: 'ADD', startsAt: '2026-11-23T00:00:00+08:00', enabled: true }
  const created = await request.post(rules, { headers, data: grant })
  expect(created.status(), created.ok() ? '' : (await created.json()).code).toBe(201)
  const rule = rules + '/' + (await created.json()).id
  const adjustment = { headers, data: { ...grant, startsAt: '2026-11-24T00:00:00+08:00' } }
  expect((await request.put(rule, adjustment)).status()).toBe(200)
  expect((await request.put(rule, adjustment)).status()).toBe(200)
  expect((await request.delete(rule, { headers })).status()).toBe(204)
  const recipient = await page.request.post('/api/v1/auth/login', { data: { loginName: 'demo-recipient', password } })
  expect(recipient.status()).toBe(200)
  const recipientHeaders = { Authorization: 'Bearer ' + (await recipient.json()).accessToken }
  const inbox = await request.get('/api/v1/notifications?category=permission', { headers: recipientHeaders })
  const filtered = await inbox.json()
  expect(filtered.total).toBe(3)
  const foreign = await request.post('/api/v1/notifications/read', { headers, data: { id: filtered.items[0].id } })
  expect(foreign.status()).toBe(404)
  expect((await foreign.json()).code).toBe('NOT_FOUND')
  const invalid = await request.get('/api/v1/notifications?unread=invalid', { headers })
  expect(invalid.status()).toBe(400)
  await page.goto('/notifications')
  const record = records(page).filter({ hasText: '系统角色已调整' })
  await expect(records(page)).toHaveCount(5)
  await recordToReport('部门、角色与授权的五条变更消息')
  await records(page).filter({ hasText: '学习部' }).getByRole('button', { name: '部门与角色变更' }).click()
  await expect(page.getByRole('region', { name: '消息详情' })).toContainText('部门委员')
  await page.keyboard.press('Escape')
  await expect(records(page).filter({ hasText: '已撤销' })).toContainText('学期违纪台账查看')
  await expect(records(page).filter({ hasText: '个人授权已修改' })).toContainText('2026-11-23 00:00:00 → 2026-11-24 00:00:00')
  await expect(record).toContainText('系统角色已调整为管理员')
  const own = await request.get('/api/v1/notifications', { headers })
  expect((await own.json()).total).toBe(0)
  await page.waitForTimeout(3000)
  await record.getByRole('button', { name: '标为已读' }).click()
  await expect(page.getByRole('button', { name: '消息中心，未读 4' })).toBeVisible()
  await page.waitForTimeout(3000)
  await page.reload()
  await expect(record).toContainText('已读')
  await recordToReport('重新登录会话内刷新后保留已读状态')
  await page.getByRole('button', { name: '全部标为已读' }).click()
  await expect(page.getByRole('dialog')).toContainText('不受当前筛选影响')
  await page.getByRole('button', { name: '确认标为已读' }).click()
  await expect(page.getByRole('button', { name: '消息中心，未读 0' })).toBeVisible()
  await page.waitForTimeout(3000)
  if (process.env.INBOX_DOM_ONLY !== '1') await aiAssert('系统角色调整消息显示为已读')
})
