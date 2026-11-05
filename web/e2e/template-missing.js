async (page) => {
  const base = page.url().split('/').slice(0, 3).join('/')
  await page.goto(base + '/inspections/attendance')
  await page.getByRole('heading', { name: '到课率检查', exact: true }).waitFor()
  await page.getByRole('button', { name: '批量导入到课率', exact: true }).click()
  const drawer = page.getByRole('dialog', { name: '批量导入到课率', exact: true })
  await drawer.getByRole('combobox', { name: '导入目标部门' }).click()
  await page.locator('.ant-select-dropdown:visible').getByText('学习部', { exact:true }).click()
  await drawer.getByText('当前检查类型尚未发布导入模板，请联系管理员发布后再导入', { exact: true }).waitFor()
  if (await drawer.getByRole('button', { name: '下载标准模板' }).isEnabled()) throw new Error('Missing template was downloadable')
  if (await drawer.getByRole('button', { name: '上传并生成预览' }).isEnabled()) throw new Error('Missing template accepted upload')
  return { cases: ['真实未发布模板：错误反馈、下载和上传禁用'] }
}
