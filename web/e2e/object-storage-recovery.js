async (page) => {
  const revealInspectionMenu = async () => {
    const group = page.locator('#admin-navigation .ant-menu-submenu:not(.ant-menu-submenu-open) > .ant-menu-submenu-title').filter({ hasText: '日常检查' })
    if (await group.count()) await group.click()
  }

  const base = page.url().split('/').slice(0,3).join('/')
  await page.goto(base + '/')
  await page.getByRole('heading',{name:'工作台',exact:true}).waitFor()
  await revealInspectionMenu()
  await page.getByRole('link',{name:'到课率检查',exact:true}).click()
  await page.getByRole('button',{name:'批量导入到课率',exact:true}).click()
  const drawer = page.getByRole('dialog',{name:'批量导入到课率',exact:true})
  const restored = page.waitForResponse(r => r.url().includes('/imports/attendance/template?'))
  await drawer.getByRole('combobox',{name:'导入目标部门'}).click()
  await page.locator('.ant-select-dropdown:visible').getByText('学习部',{exact:true}).click()
  const response = await restored
  if (response.status() !== 200) throw new Error('Template did not recover after MinIO resumed')
  const [download] = await Promise.all([page.waitForEvent('download'),drawer.getByRole('button',{name:'下载标准模板'}).click()])
  if (!download.suggestedFilename().endsWith('.xlsx')) throw new Error('Recovered download has wrong filename')
  return {cases:['恢复同一隔离 MinIO 后 ready=200，重新打开页面可再次真实下载模板']}
}
