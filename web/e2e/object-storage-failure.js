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
  const failed = page.waitForResponse(r => r.url().includes('/imports/attendance/template?'))
  await drawer.getByRole('combobox',{name:'导入目标部门'}).click()
  await page.locator('.ant-select-dropdown:visible').getByText('学习部',{exact:true}).click()
  const response = await failed
  if (response.status() !== 500 || (await response.json()).code !== 'IMPORT_SERVICE_UNAVAILABLE') throw new Error('Storage timeout did not return the expected safe error')
  await drawer.getByText('无法获取当前导入模板，请稍后重试',{exact:true}).waitFor()
  if (await drawer.getByRole('button',{name:'下载标准模板'}).isEnabled()) throw new Error('Download enabled during real storage outage')
  if (await drawer.getByRole('button',{name:'上传并生成预览'}).isEnabled()) throw new Error('Upload enabled without template')
  return {cases:['真实暂停隔离 MinIO：ready=503，模板接口 500 IMPORT_SERVICE_UNAVAILABLE，页面失败提示且下载/上传禁用'],faultInjection:['SIGSTOP 暂停本轮一次性 MinIO，触发真实对象存储操作超时']}
}
