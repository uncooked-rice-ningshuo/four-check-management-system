import { expect } from '@playwright/test'
import { test } from './fixture'

test('通过视觉操作提交空登录表单并看到必填提示', async ({ page, aiAct, aiAssert }) => {
  // Keep this public-page smoke test independent from any API bound to localhost:8080.
  await page.route('**/api/v1/auth/refresh', (route) => route.fulfill({
    status: 401,
    contentType: 'application/json',
    body: JSON.stringify({ code: 'UNAUTHORIZED', message: '未登录' }),
  }))

  await page.goto('/login')
  await expect(page.getByRole('heading', { name: '欢迎回来', exact: true })).toBeVisible()

  await aiAct('点击蓝色的“登录”主按钮，不填写账号和密码。')

  await expect(page.getByText('请输入账号', { exact: true })).toBeVisible()
  await expect(page.getByText('请输入密码', { exact: true })).toBeVisible()
  await aiAssert('账号和密码输入框旁都显示了对应的必填提示，登录页面仍然可见。')
})
