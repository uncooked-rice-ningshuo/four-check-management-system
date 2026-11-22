import { expect } from '@playwright/test'
import { test } from './fixture'
import { batchAlert, records, selectAllRows } from './inbox-protable-helpers'

// 真实 API + 一次性 PostgreSQL 的慢速录像回归。DOM 断言不调用远端视觉服务，避免额度与超时依赖。
// hasTouch 让 locator.tap() 走真实触屏事件；显式停留让录像里每一步状态可读。
test.use({
  video: { mode: 'on', size: { width: 1440, height: 900 } },
  hasTouch: true,
  launchOptions: { channel: 'chrome', slowMo: 200 },
})

test('消息中心增强：铃铛悬停/Escape/触屏、筛选排序、批量已读与越权原子失败', async ({ page, request }) => {
  test.skip(!process.env.INBOX_ACCOUNT, '需要隔离服务与虚构账号')
  test.setTimeout(180_000)
  const password = process.env.INBOX_PASSWORD!
  await page.goto('/login')
  await page.getByLabel('账号').fill('demo-recipient')
  await page.getByLabel('密码').fill(password)
  await page.locator('button[type="submit"]').click()
  const bell = page.getByRole('button', { name: '消息中心，未读 3' })
  await expect(bell).toBeVisible()
  await page.waitForTimeout(700) // 录像停留：顶栏未读 3

  const panel = page.getByRole('dialog', { name: '近期消息' })

  // 桌面语义：悬停打开、鼠标可移入、Escape 关闭并把焦点还给铃铛。
  await bell.hover()
  await expect(panel).toBeVisible()
  await expect(panel.getByText('当天夜间的部门变更')).toBeVisible()
  await panel.hover()
  await expect(panel).toBeVisible()
  await page.waitForTimeout(700) // 录像停留：悬停面板
  await page.keyboard.press('Escape')
  await expect(panel).toBeHidden()
  await expect(bell).toBeFocused()
  await page.waitForTimeout(500)

  // 触屏语义：从页面其他焦点开始，首次 tap 必须“打开且保持打开”。
  // onFocusCapture 会 show，随后 onClick 不能把同一次 tap 当成 toggle 关闭。
  await page.getByRole('button', { name: /账户菜单/ }).focus()
  await expect(bell).not.toBeFocused()
  await bell.tap()
  await expect(panel).toBeVisible()
  await expect(bell).toHaveAttribute('aria-expanded', 'true')
  await page.waitForTimeout(900) // 录像停留：首次 tap 打开

  // 再次 tap 切换关闭；第三次 tap 重新打开后外点关闭。
  await bell.tap()
  await expect(panel).toBeHidden()
  await page.waitForTimeout(500)
  await bell.tap()
  await expect(panel).toBeVisible()
  await page.waitForTimeout(500)
  await page.locator('#main-content').tap({ position: { x: 20, y: 20 } })
  await expect(panel).toBeHidden()
  await page.waitForTimeout(500)

  // 触屏打开后键盘焦点进入面板：鼠标移开超过关闭延时仍应可操作。
  await bell.tap()
  await expect(panel).toBeVisible()
  await page.keyboard.press('Tab')
  const viewAll = panel.getByRole('link', { name: '查看全部' })
  await expect(viewAll).toBeFocused()
  await page.mouse.move(2, 2)
  await page.waitForTimeout(500) // 超过 closeDelayMs，验证不因鼠标离开而关闭
  await expect(panel).toBeVisible()
  await expect(viewAll).toBeEnabled()
  await page.waitForTimeout(700) // 录像停留：鼠标离开但键盘焦点在面板

  await viewAll.tap()
  await expect(page).toHaveURL(/\/notifications/)
  await page.waitForTimeout(700)

  // 完整中心：服务端筛选排序分页。
  await expect(records(page)).toHaveCount(4)
  const login = await request.post('/api/v1/auth/login', { data: { loginName: 'demo-recipient', password } })
  expect(login.status()).toBe(200)
  const headers = { Authorization: 'Bearer ' + (await login.json()).accessToken }

  const ranged = await request.get('/api/v1/notifications?from=2026-11-23&to=2026-11-23', { headers })
  const rangedBody = await ranged.json()
  expect(rangedBody.total).toBe(2)
  expect(rangedBody.items).toHaveLength(2)
  const oldest = await request.get('/api/v1/notifications?sort=oldest', { headers })
  expect((await oldest.json()).items[0].body).toBe('较早的部门变更')
  const unreadFirst = await request.get('/api/v1/notifications?sort=unread-first', { headers })
  expect((await unreadFirst.json()).items.slice(0, 3).every((item: { read: boolean }) => !item.read)).toBe(true)
  const paged = await request.get('/api/v1/notifications?page=2&pageSize=2&sort=oldest', { headers })
  expect((await paged.json()).items).toHaveLength(2)

  // UI 日期范围筛选（北京时间，含止日）与页面全选批量已读。
  await page.getByPlaceholder('开始日期').fill('2026-11-23')
  await page.getByPlaceholder('开始日期').press('Enter')
  await page.getByPlaceholder('结束日期').fill('2026-11-23')
  await page.getByPlaceholder('结束日期').press('Enter')
  await page.locator('form.ant-pro-query-filter').getByRole('button', { name: /查\s*询/ }).click()
  await expect(page.getByText('共 2 条', { exact: true })).toBeVisible()
  await expect(records(page)).toHaveCount(2)
  await page.waitForTimeout(700) // 录像停留：筛选后 2 条

  await selectAllRows(page)
  await expect(batchAlert(page)).toContainText('已选择 2 条消息')
  await page.waitForTimeout(700) // 录像停留：已选择 2 条
  await batchAlert(page).getByRole('button', { name: '标为已读' }).click()
  const confirm = page.getByRole('dialog')
  await expect(confirm).toContainText('仅处理当前账号选中的 2 条消息')
  await page.waitForTimeout(1200) // 录像停留：批量确认弹窗内容可读
  await page.getByRole('button', { name: '确认标为已读' }).click()
  await expect(page.getByText(/已将选中的/)).toBeVisible()
  await page.waitForTimeout(900) // 录像停留：批量已读结果

  // 批量接口越权原子失败与重复请求幂等。
  const own = await request.get('/api/v1/notifications', { headers })
  const ownItems = (await own.json()).items as { id: string }[]
  const foreignLogin = await request.post('/api/v1/auth/login', { data: { loginName: 'demo-foreign', password } })
  expect(foreignLogin.status()).toBe(200)
  const foreignHeaders = { Authorization: 'Bearer ' + (await foreignLogin.json()).accessToken }
  const foreignList = await request.get('/api/v1/notifications', { headers: foreignHeaders })
  const foreignId = (await foreignList.json()).items[0].id

  const before = await request.get('/api/v1/notifications', { headers })
  const beforeUnread = (await before.json()).unread
  const mixed = await request.post('/api/v1/notifications/read', {
    headers,
    data: { ids: [ownItems[0].id, foreignId] },
  })
  expect(mixed.status()).toBe(404)
  expect((await mixed.json()).code).toBe('NOT_FOUND')
  const after = await request.get('/api/v1/notifications', { headers })
  expect((await after.json()).unread).toBe(beforeUnread)

  const ids = ownItems.slice(0, 2).map(item => item.id)
  const first = await request.post('/api/v1/notifications/read', { headers, data: { ids } })
  expect(first.status()).toBe(200)
  const changed = (await first.json()).changed
  const repeated = await request.post('/api/v1/notifications/read', { headers, data: { ids } })
  expect(repeated.status()).toBe(200)
  expect((await repeated.json()).changed).toBe(0)
  expect(changed).toBeGreaterThanOrEqual(0)
})
