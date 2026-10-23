// The runner supplies only synthetic fixtures. Failures below alter responses or
// expected versions; successful confirmation still commits through the real API.
async (page) => {
  const revealInspectionMenu = async () => {
    const group = page.locator('#admin-navigation .ant-menu-submenu:not(.ant-menu-submenu-open) > .ant-menu-submenu-title').filter({ hasText: '日常检查' })
    if (await group.count()) await group.click()
  }

  const fixture = __FIXTURE__
  const output = __OUTPUT__
  const base = page.url().split('/').slice(0,3).join('/')
  const cases = []
  const check = (condition, message) => { if (!condition) throw new Error(message) }
  // Start an independent scenario via the workbench; reloading the same URL
  // intentionally resumes the previous suite's unfinished batch.
  await page.goto(base + '/')
  await page.getByRole('heading', {name:'工作台',exact:true}).waitFor()
  await revealInspectionMenu()
  await page.getByRole('link', {name:'到课率检查',exact:true}).click()
  await page.getByRole('button', { name:'批量导入到课率',exact:true }).click()
  const drawer = page.getByRole('dialog', { name:'批量导入到课率',exact:true })
  await drawer.getByRole('combobox', { name:'导入目标部门' }).click()
  await page.locator('.ant-select-dropdown:visible').getByText('学习部', {exact:true}).click()
  const upload = async file => {
    await drawer.locator('input[type=file]').setInputFiles(file)
    const [result] = await Promise.all([
      page.waitForResponse(r => r.url().endsWith('/api/v1/imports/attendance') && r.request().method() === 'POST'),
      drawer.getByRole('button',{name:'上传并生成预览'}).click(),
    ])
    check(result.status() === 201, 'Upload did not create a preview')
    return result.json()
  }
  const invalid = await upload(fixture.invalidFile)
  await drawer.getByText('存在错误行，当前批次不能确认', {exact:true}).waitFor()
  await drawer.getByText('仅错误',{exact:true}).click()
  await drawer.getByRole('cell',{name:'UNKNOWN',exact:true}).waitFor()
  await drawer.getByRole('button',{name:'重新上传文件',exact:true}).click()
  let restart = page.getByRole('dialog',{name:'重新上传到课率文件？'})
  await restart.getByRole('button',{name:'返回预览'}).click()
  await drawer.getByRole('cell',{name:'UNKNOWN',exact:true}).waitFor()
  await drawer.getByRole('button',{name:'重新上传文件',exact:true}).click()
  restart = page.getByRole('dialog',{name:'重新上传到课率文件？'})
  await restart.getByText(/原批次和审计记录会保留/).waitFor()
  await restart.getByRole('button',{name:'保留原批次并重新上传'}).click()
  await drawer.getByRole('button',{name:'上传并生成预览'}).waitFor()
  check(!await drawer.getByRole('button',{name:'上传并生成预览'}).isEnabled(), 'Old file remained selected after restart')
  const corrected = await upload(fixture.correctedFile)
  check(invalid.id !== corrected.id, 'Restart overwrote the original batch')
  await drawer.getByText('校验通过',{exact:true}).waitFor()
  await drawer.getByRole('cell',{name:'000001',exact:true}).waitFor()
  await drawer.getByRole('cell',{name:'000002',exact:true}).waitFor()
  cases.push('错误批次重传取消/确认、新批次预览、旧文件与错误筛选状态清空')

  const openConfirmation = async () => {
    await drawer.getByRole('button',{name:'确认导入 2 条记录'}).click()
    const dialog = page.getByRole('dialog',{name:'确认写入到课率草稿'})
    await dialog.waitFor()
    const text = await dialog.innerText()
    for (const label of ['到课率','浏览器回归学期','第 3 周','学习部','2 条草稿记录']) check(text.includes(label), 'Missing scope label: ' + label)
    check(!/[0-9a-f]{8}-[0-9a-f]{4}-/i.test(text), 'Confirmation exposes internal IDs')
    return dialog
  }
  const path = `**/api/v1/imports/${corrected.id}/confirm`
  const confirmResponse = action => Promise.all([
    page.waitForResponse(r => r.url().endsWith(`/imports/${corrected.id}/confirm`) && r.request().method() === 'POST'), action(),
  ])
  let dialog = await openConfirmation()
  await page.screenshot({path:`${output}/import-confirmation-scope.png`,fullPage:true})
  const conflict = async route => {
    const upstream = await route.fetch({postData:JSON.stringify({version:999999})})
    check(upstream.status() === 409, 'Backend did not reject stale version')
    await route.fulfill({response:upstream})
  }
  await page.route(path, conflict)
  const [rejected] = await confirmResponse(() => dialog.getByRole('button',{name:'确认写入草稿记录'}).click())
  check(rejected.status() === 409 && (await rejected.json()).code === 'VERSION_CONFLICT', 'Missing conflict contract')
  await drawer.getByText('数据已被其他人更新，请刷新后重试',{exact:true}).waitFor()
  await page.unroute(path, conflict)
  await dialog.waitFor({state:'hidden'})
  cases.push('确认弹窗显示真实业务范围、过期版本由真实 API 返回 409 后可重试')

  // Commit once upstream, but replace only the browser-facing response. A retry
  // must reuse the key and recover the result without inserting another record.
  let committedKey = ''
  const lostResponse = async route => {
    committedKey = route.request().headers()['idempotency-key']
    const upstream = await route.fetch()
    check(upstream.status() === 200 && (await upstream.json()).importedRows === 2, 'Real confirmation did not commit')
    await route.fulfill({status:500,contentType:'application/json',body:JSON.stringify({code:'INTERNAL_ERROR',message:'测试响应丢失'})})
  }
  await page.route(path, lostResponse)
  dialog = await openConfirmation()
  const [failed] = await confirmResponse(() => dialog.getByRole('button',{name:'确认写入草稿记录'}).click())
  check(failed.status() === 500, 'Response loss was not injected')
  await drawer.getByText('操作失败，请稍后重试',{exact:true}).waitFor()
  await page.unroute(path, lostResponse)
  await dialog.waitFor({state:'hidden'})
  dialog = await openConfirmation()
  const [retried] = await confirmResponse(() => dialog.getByRole('button',{name:'确认写入草稿记录'}).click())
  check(retried.status() === 200, 'Idempotent retry did not recover confirmation')
  check(Boolean(committedKey) && retried.request().headers()['idempotency-key'] === committedKey, 'Retry changed idempotency key')
  await drawer.getByText('已成功导入 2 条草稿记录',{exact:true}).waitFor()
  await drawer.getByRole('button',{name:'Close',exact:true}).click()
  await drawer.waitFor({state:'hidden'})
  await page.getByText('共 4 条',{exact:true}).first().waitFor()
  cases.push('确认已真实提交但浏览器收到 500，同键重试恢复成功且列表只增加两条')
  return {cases, invalidBatchId:invalid.id, correctedBatchId:corrected.id, faultInjection:['修改确认版本为 999999，真实 API 返回 409','真实 API 已提交，将浏览器响应替换为 500']}
}
