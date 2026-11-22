import type { Page } from '@playwright/test'

// ProTable 原生查询区的共享定位器；旧自定义卡片筛选的选择器已不再适用。
export const records = (page: Page) => page.locator('.ant-table-tbody tr[data-row-key]')

export const searchForm = (page: Page) => page.locator('form.ant-pro-query-filter')

export async function chooseOption(page: Page, field: string, option: string) {
  await page.getByRole('combobox', { name: field }).click()
  await page.getByText(option, { exact: true }).last().click()
}

export async function applySearch(page: Page) {
  await searchForm(page).getByRole('button', { name: /查\s*询/ }).click()
}

export async function resetSearch(page: Page) {
  await searchForm(page).getByRole('button', { name: /重\s*置/ }).click()
}

export async function setReadStatus(page: Page, status: '全部' | '仅未读') {
  await chooseOption(page, '阅读状态', status)
  await applySearch(page)
}

export async function setCategory(page: Page, category: '全部类别' | '部门与角色' | '个人授权') {
  await chooseOption(page, '消息类别', category)
  await applySearch(page)
}

// 批量操作的原生提示条只在选择行后出现。
export const batchAlert = (page: Page) => page.locator('.ant-pro-table-alert')

export async function selectAllRows(page: Page) {
  await page.locator('.ant-table-thead input[type="checkbox"]').check()
}
