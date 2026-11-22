import { expect, test } from '@playwright/test'
import { env } from 'node:process'

test.use({ video: 'on' })
test.skip(!env.RULE_TEST_PASSWORD, '使用一次性数据库的 discipline-rule-access 套件')

for (const action of ['NONE', 'VIEW', 'EDIT', 'PUBLISH']) {
  for (const role of ['MEMBER', 'MINISTER', 'VICE_MINISTER']) {
    test(`办公室 ${role} · ${action}`, async ({ page }) => {
      const session = await page.request.post('/api/v1/auth/login', {
        data: { loginName: `rule-${role}-${action}`, password: env.RULE_TEST_PASSWORD },
      })
      expect(session.status()).toBe(200)
      const headers = { Authorization: 'Bearer ' + (await session.json()).accessToken }
      const catalog = await page.request.get('/api/v1/admin/catalogs', { headers })
      expect(catalog.status()).toBe(403)
      const rules = await page.request.get('/api/v1/admin/discipline/rule-versions', { headers })
      expect(rules.status()).toBe(action === 'NONE' ? 403 : 200)
      await page.goto('/discipline/config')
      const history = page.getByRole('button', { name: '版本历史' })
      if (action === 'NONE') {
        await expect(page.getByText(/无权|没有权限|暂无权限/).first()).toBeVisible()
        await expect(history).toHaveCount(0)
        return
      }
      await expect(history).toBeVisible()
      const edit = page.getByRole('button', { name: /继续修改草稿/ })
      const publish = page.getByRole('button', { name: /发布草稿/ })
      await expect(edit).toHaveCount(action === 'EDIT' ? 1 : 0)
      await expect(publish).toHaveCount(action === 'PUBLISH' ? 1 : 0)
      if (action === 'VIEW') {
        await history.click()
        await expect(page.getByText('编辑中草稿')).toBeVisible()
      } else if (action === 'EDIT') {
        await edit.click()
        await expect(page.getByRole('button', { name: '保存草稿' })).toBeVisible()
        const groupName = page.getByRole('textbox', { name: /累计项名称/ })
        const changedName = '测试累计项-' + role
        await groupName.fill(changedName)
        await page.getByRole('button', { name: '保存草稿' }).click()
        await expect(page.getByText('规则草稿已保存')).toBeVisible()
        await page.reload()
        await expect(groupName).toHaveValue(changedName)
      } else {
        await publish.click()
        const dialog = page.getByRole('dialog')
        await expect(dialog).toBeVisible()
        if (role === 'VICE_MINISTER') {
          await dialog.getByRole('button', { name: '确认发布' }).click()
          await expect(page.getByText('v1 · 当前生效', { exact: true })).toBeVisible()
        } else {
          await dialog.getByRole('button', { name: '返回检查' }).click()
        }
      }
    })
  }
}
