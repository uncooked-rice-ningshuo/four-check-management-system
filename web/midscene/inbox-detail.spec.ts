import { expect } from '@playwright/test'
import { mkdirSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { test } from './fixture'
import { records, setCategory, batchAlert } from './inbox-protable-helpers'

// 隔离真实 API + 一次性 PostgreSQL 的慢速录像。断言只读 DOM/网络，不调用远端视觉服务。
// 截图固定写到独立目录，避免被 playwright --output 在下次运行时清空。
const shotDir = fileURLToPath(new URL('../../output/midscene/inbox-detail-shots/', import.meta.url))
mkdirSync(shotDir, { recursive: true })

test.use({
  video: { mode: 'on', size: { width: 1280, height: 900 } },
  launchOptions: { channel: 'chrome', slowMo: 200 },
})

test('消息中心分栏详情：808/390px、读写分离、键盘与批量互不干扰', async ({ page, request }) => {
  test.skip(!process.env.INBOX_ACCOUNT, '需要隔离服务与虚构账号')
  test.setTimeout(300_000)
  page.setDefaultTimeout(12_000)
  const password = process.env.INBOX_PASSWORD!

  // 侧栏 256px + 内容区左右各 30px：1124px 视口恰好得到 808px 管理内容区。
  await page.setViewportSize({ width: 1124, height: 900 })
  await page.goto('/login')
  await page.getByLabel('账号').fill('demo-recipient')
  await page.getByLabel('密码').fill(password)
  await page.locator('button[type="submit"]').click()
  await page.goto('/notifications')
  await expect(records(page).first()).toBeVisible()
  await expect(page.getByText('共 29 条', { exact: true })).toBeVisible()

  const pageBox = await page.locator('.inbox-page').boundingBox()
  expect(Math.abs((pageBox?.width ?? 0) - 808)).toBeLessThan(40)

  // 批注 2：查询区与列表卡片恢复约 16px 间距，查询表单横跨整个容器。
  const search = page.locator('.inbox-table .ant-pro-table-search')
  expect(await search.evaluate(el => getComputedStyle(el).marginBottom)).toBe('16px')
  const searchBox = await search.boundingBox()
  const tableBox = await page.locator('.inbox-table').boundingBox()
  expect(Math.abs((searchBox?.width ?? 0) - (tableBox?.width ?? 0))).toBeLessThan(2)

  // 批注 3：已读白底、未读浅蓝整行背景，同时保留已读/未读文字。
  const unreadRow = records(page).filter({ hasText: '当天夜间的部门变更' })
  const readRow = records(page).filter({ hasText: '已读的授权变更' })
  const unreadBg = await unreadRow.locator('td').nth(1).evaluate(el => getComputedStyle(el).backgroundColor)
  const readBg = await readRow.locator('td').nth(1).evaluate(el => getComputedStyle(el).backgroundColor)
  expect(unreadBg).toBe('rgb(246, 250, 255)')
  expect(readBg).toBe('rgb(255, 255, 255)')
  await expect(unreadRow).toContainText('未读')
  await expect(readRow).toContainText('已读')
  await page.waitForTimeout(600)
  await page.screenshot({ path: `${shotDir}/808-list.png`, fullPage: true })

  let readPosts = 0
  const countRead = (url: URL, method: string) => {
    if (url.pathname.includes('/notifications/read') && method === 'POST') readPosts += 1
  }
  page.on('request', req => countRead(new URL(req.url()), req.method()))

  // 打开详情：默认仅查看，右侧按时间、类别/来源、标题、完整正文排版。
  const openerA = unreadRow.getByRole('button', { name: '部门与角色变更' })
  await openerA.click()
  const detail = page.getByRole('region', { name: '消息详情' })
  await expect(detail).toBeVisible()
  await expect(detail).toContainText('当天夜间的部门变更')
  await expect(detail.locator('time').first()).toHaveAttribute('datetime', /2026-11-23/)
  await expect(detail).not.toContainText('发件人')
  await expect(detail.locator('[aria-label*="删除"], [aria-label*="置顶"]')).toHaveCount(0)

  // 查询区仍全宽，列表与详情各占约一半，均未裁掉主表。
  const workspace = await page.locator('.inbox-workspace').boundingBox()
  const listPane = page.locator('.inbox-list-pane')
  const listCard = await listPane.boundingBox()
  const detailBox = await detail.boundingBox()
  expect((listCard!.width / workspace!.width)).toBeLessThan(0.55)
  expect((detailBox!.width / workspace!.width)).toBeLessThan(0.55)
  expect(Math.abs((await search.boundingBox())!.width - (await page.locator('.inbox-table').boundingBox())!.width)).toBeLessThan(2)
  await page.screenshot({ path: `${shotDir}/808-detail.png`, fullPage: true })

  await page.waitForTimeout(600)
  expect(readPosts).toBe(0) // 仅打开详情不写库
  await detail.getByRole('button', { name: '标为已读' }).click()
  await expect.poll(() => readPosts).toBeGreaterThan(0)
  await expect(detail).toContainText('已读')

  // A/B 切换：换一条消息后详情内容跟着换，不残留上一条正文。
  const rowB = records(page).filter({ hasText: '当天早间的授权变更' })
  const openerB = rowB.getByRole('button', { name: '个人授权变更' })
  await openerB.click()
  await expect(detail).toContainText('当天早间的授权变更')
  await expect(detail).not.toContainText('当天夜间的部门变更')

  // Escape 关闭并把焦点还给刚才的标题按钮；键盘 Enter 也能打开。
  await page.keyboard.press('Escape')
  await expect(detail).toBeHidden()
  await expect(openerB).toBeFocused()
  await openerB.focus()
  await page.keyboard.press('Enter')
  await expect(detail).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(detail).toBeHidden()

  // 可聚焦的阅读状态同样打开详情。
  await rowB.locator('.inbox-status-open').click()
  await expect(detail).toBeVisible()
  await expect(detail).toContainText('当天早间的授权变更')
  await page.keyboard.press('Escape')

  // 复选、批量与行操作只做自己的事，不误开详情；选中态与未读态分别表现。
  await page.locator('.ant-table-thead input[type="checkbox"]').check()
  await expect(batchAlert(page)).toContainText('已选择')
  await expect(unreadRow).toHaveClass(/ant-table-row-selected/)
  await expect(detail).toBeHidden()
  await page.screenshot({ path: `${shotDir}/808-selected.png`, fullPage: true })
  await batchAlert(page).getByRole('button', { name: '标为已读' }).click()
  await expect(page.getByRole('dialog')).toContainText('仅处理当前账号选中的')
  await expect(detail).toBeHidden()
  await page.getByRole('dialog').getByRole('button', { name: /取\s*消/ }).click()
  await records(page).filter({ hasText: '分页消息 25' }).getByRole('button', { name: '标为已读' }).click()
  await expect(detail).toBeHidden()

  // 筛选与翻页都收起详情，不串旧记录。
  await records(page).filter({ hasText: '分页消息 25' }).getByRole('button', { name: '部门与角色变更' }).click()
  await expect(detail).toBeVisible()
  await setCategory(page, '个人授权')
  await expect(detail).toBeHidden()
  await expect(records(page).filter({ hasText: '分页消息 25' })).toHaveCount(0)
  await setCategory(page, '全部类别')
  await expect(records(page)).toHaveCount(20)
  await records(page).first().locator('.inbox-cell-open').click()
  await expect(detail).toBeVisible()
  await page.getByRole('listitem', { name: '2', exact: true }).click()
  await expect(detail).toBeHidden()
  await expect(records(page).first()).toContainText('分页消息')

  // 反向：越权批量 404；失败注入后错误可见且可重试恢复。
  const login = await request.post('/api/v1/auth/login', { data: { loginName: 'demo-recipient', password } })
  expect(login.status()).toBe(200)
  const headers = { Authorization: 'Bearer ' + (await login.json()).accessToken }
  const foreignLogin = await request.post('/api/v1/auth/login', { data: { loginName: 'demo-foreign', password } })
  expect(foreignLogin.status()).toBe(200)
  const foreignHeaders = { Authorization: 'Bearer ' + (await foreignLogin.json()).accessToken }
  const foreignList = await request.get('/api/v1/notifications', { headers: foreignHeaders })
  const foreignId = (await foreignList.json()).items[0].id
  const denied = await request.post('/api/v1/notifications/read', { headers, data: { ids: [foreignId] } })
  expect(denied.status()).toBe(404)

  const failure = /api\/v1\/notifications(?:\?|$)/
  await page.route(failure, route => route.abort())
  await setCategory(page, '部门与角色')
  await expect(page.getByText('读取消息失败', { exact: true })).toBeVisible()
  await page.unroute(failure)
  await page.getByRole('button', { name: /重\s*试/ }).click()
  await expect(records(page).first()).toBeVisible()

  // 390px 单列：详情占满且列表收起，返回后恢复列表与全宽。
  await page.setViewportSize({ width: 390, height: 844 })
  await expect(page.locator('.admin-sider.ant-layout-sider-collapsed')).toBeVisible()
  await page.waitForTimeout(700)
  await expect(records(page).first()).toBeVisible()
  await page.screenshot({ path: `${shotDir}/390-list.png`, fullPage: true })
  await records(page).first().locator('.inbox-cell-open').click()
  await expect(detail).toBeVisible()
  await expect(page.locator('.inbox-list-pane')).toBeHidden({ timeout: 10_000 })
  await page.screenshot({ path: `${shotDir}/390-detail.png`, fullPage: true })
  expect(await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)).toBeLessThanOrEqual(1)
  await page.getByRole('button', { name: '返回列表' }).click()
  await expect(detail).toBeHidden()
  await expect(records(page).first()).toBeVisible()
})
