async (page) => {
  const fixture = __FIXTURE__
  const output = __OUTPUT__
  const base = page.url().split('/').slice(0, 3).join('/')
  const cases = []
  const browserErrors = []
  page.on('pageerror', error => browserErrors.push(error.message))
  const check = (value, message) => { if (!value) throw new Error(message) }
  const select = async (scope, label, option) => {
    await scope.getByRole('combobox', { name: new RegExp(label + '$') }).click()
    await page.locator('.ant-select-dropdown:visible').getByText(option, { exact:true }).click()
  }
  const response = async (method, path, action, status) => {
    const [result] = await Promise.all([page.waitForResponse(r => r.request().method() === method && r.url().split('?')[0].split('/').slice(3).join('/').replace(/^/, '/') === path), action()])
    check(result.status() === status, `${method} ${path}: expected ${status}, got ${result.status()}`)
    return result
  }
  const openImport = async (prepare = false) => {
    await page.getByRole('button', { name: '批量导入教室卫生', exact: true }).click()
    if (prepare) {
      const scope = page.getByRole('dialog', { name: '教室卫生导入范围', exact: true })
      await scope.getByRole('combobox', { name: '导入学期' }).click()
      await page.locator('.ant-select-dropdown:visible').getByText(/浏览器回归学期/).click()
      await scope.getByRole('combobox', { name: '导入周次' }).click()
      await page.locator('.ant-select-dropdown:visible').getByText(new RegExp(fixture.weekStart)).click()
      await scope.getByRole('button', { name: '准备导入', exact: true }).click()
    }
    return page.getByRole('dialog', { name: '批量导入教室卫生', exact: true })
  }
  const fresh = async () => {
    // Full navigation intentionally starts a new independent batch, retaining old
    // batch audit history. Resume is tested separately before confirming.
    await page.goto(base + '/inspections/classroom-hygiene')
    await page.getByRole('heading', { name: '教室卫生检查', exact: true }).waitFor()
  }
  await fresh()
  await page.getByText('当前范围暂无教室卫生记录', { exact: true }).waitFor()
  let drawer = await openImport(true)
  await select(drawer, '导入目标部门', '学习部')
  const [download] = await Promise.all([page.waitForEvent('download'), drawer.getByRole('button', { name: '下载标准模板' }).click()])
  await download.saveAs(`${output}/downloaded-template.xlsx`)
  check(download.suggestedFilename().endsWith('.xlsx'), 'Wrong download filename')
  await drawer.locator('input[type=file]').setInputFiles(fixture.badFile)
  await drawer.getByText('仅支持无宏的 .xlsx 文件', { exact: true }).waitFor()
  check(!await drawer.getByRole('button', { name: '上传并生成预览' }).isEnabled(), 'Invalid extension enabled upload')
  cases.push('已发布模板真实下载、非 XLSX 文件前端拦截')
  await drawer.locator('input[type=file]').setInputFiles(fixture.wrongTemplateFile)
  const wrong = await response('POST', '/api/v1/imports/inspections', () => drawer.getByRole('button', { name: '上传并生成预览' }).click(), 422)
  check((await wrong.json()).code === 'INVALID_WORKBOOK', 'Wrong template rejection code')
  cases.push('到课率模板不能导入教室卫生；失败后保留目标并允许重新选文件')
  await drawer.locator('input[type=file]').setInputFiles(fixture.validFile)
  const upload = await response('POST', '/api/v1/imports/inspections', () => drawer.getByRole('button', { name: '上传并生成预览' }).click(), 201)
  const batch = await upload.json()
  await drawer.getByText('校验通过', { exact: true }).waitFor()
  await drawer.getByRole('cell', { name: '000001', exact: true }).waitFor()
  check(await drawer.getByRole('columnheader', {name: '时间/轮次', exact: true}).count() === 1, 'Missing object occurrence preview')
  check(await drawer.getByRole('columnheader', {name: '课程', exact: true}).count() === 0, 'Course columns leaked into classroom-hygiene preview')
  await drawer.getByRole('button', { name: 'Close', exact: true }).click()
  await page.getByRole('button', { name: '保留并关闭', exact: true }).click()
  await drawer.waitFor({ state: 'hidden' })
  drawer = await openImport()
  await drawer.getByText('校验通过', { exact: true }).waitFor()
  check(await drawer.getByRole('cell',{name:'优秀',exact:true}).count()===1,'Missing excellent area result')
  check(await drawer.getByRole('columnheader',{name:'姓名',exact:true}).count()===0,'Individual student columns leaked')
  cases.push('真实上传和两行对象预览、优秀空责任学生、检查轮次、关闭后恢复未确认批次')
  const referenceContext = await page.context().browser().newContext()
  try {
    const login = await referenceContext.request.post(base + '/api/v1/auth/login', { data: { loginName: 'browser-admin', password: fixture.authPassword } })
    check(login.status() === 200, 'Reference admin login failed')
    const headers = { Authorization: 'Bearer ' + (await login.json()).accessToken }
    const students = await (await referenceContext.request.get(base + '/api/v1/admin/master-data/students?pageSize=200', { headers })).json()
    const student = students.items.find(item => item.name === 'E2E student A')
    check(!!student, 'Missing isolated student')
    const path = base + '/api/v1/admin/master-data/students/' + student.id
    const disable = await referenceContext.request.put(path, { headers, data: { ...student, enabled: false } })
    check(disable.status() === 200, 'Could not disable preview student')
    await drawer.getByRole('button', { name: '确认导入 2 条记录' }).click()
    const stale = await response('POST', `/api/v1/imports/${batch.id}/confirm`, () => page.getByRole('button', { name: '确认写入草稿记录' }).click(), 409)
    check((await stale.json()).code === 'IMPORT_PREVIEW_STALE', 'Wrong stale-preview error')
    await drawer.getByText('基础数据或检查范围已变化，请重新上传并校验后确认', { exact: true }).waitFor()
    const records = await (await referenceContext.request.get(base + '/api/v1/inspections?inspectionTypeId=' + batch.inspectionTypeId, { headers })).json()
    check(records.total === 0, 'Stale preview wrote partial records')
    check(await drawer.getByRole('cell', { name: '000001', exact: true }).count() === 1, 'Conflict lost preview')
    const restore = await referenceContext.request.put(path, { headers, data: { ...student, enabled: true } })
    check(restore.status() === 200, 'Could not restore preview student')
    cases.push('真实停用预览学生后确认409、整批零写入、预览保留；恢复基础数据可重试')
    const previewForRoom = await (await referenceContext.request.get(base + '/api/v1/imports/' + batch.id + '/rows?page=1&pageSize=20', { headers })).json()
    const areaID = previewForRoom.items[0].normalizedInspection.input.details.areaId
    const areaRoot=base+'/api/v1/admin/hygiene-areas/classroom-hygiene-areas'
    const areas = await (await referenceContext.request.get(areaRoot+'?departmentId='+batch.departmentId+'&pageSize=200',{headers})).json()
    const area=areas.items.find(item=>item.id===areaID)
    check(!!area,'Missing inspected area')
    const areaPath=areaRoot+'/'+areaID+'?departmentId='+batch.departmentId
    check((await referenceContext.request.put(areaPath,{headers,data:{...area,enabled:false}})).status()===200,'Could not disable area')
    await drawer.getByRole('button',{name:'确认导入 2 条记录'}).click()
    const changedArea=await response('POST',`/api/v1/imports/${batch.id}/confirm`,()=>page.getByRole('button',{name:'确认写入草稿记录'}).click(),409)
    check((await changedArea.json()).code==='IMPORT_PREVIEW_STALE','Disabled area accepted')
    check((await referenceContext.request.put(areaPath,{headers,data:area})).status()===200,'Could not restore area')
    cases.push('真实停用检查区域后确认409；恢复区域可重试')

    const preview = await (await referenceContext.request.get(base + '/api/v1/imports/' + batch.id + '/rows?page=1&pageSize=20', { headers })).json()
    const record = preview.items[1].normalizedInspection.input.details
    const duplicate = await referenceContext.request.post(base + '/api/v1/inspections', { headers, data: { semesterId: batch.semesterId, weekId: batch.weekId, inspectionTypeId: batch.inspectionTypeId, departmentId: batch.departmentId, details: { ...record } } })
    check(duplicate.status() === 201, 'Could not create concurrent duplicate')
    const manual = await duplicate.json()
    await drawer.getByRole('button', { name: '确认导入 2 条记录' }).click()
    const conflict = await response('POST', `/api/v1/imports/${batch.id}/confirm`, () => page.getByRole('button', { name: '确认写入草稿记录' }).click(), 409)
    check((await conflict.json()).code === 'RECORD_ALREADY_EXISTS', 'Duplicate leaked a database failure')
    await drawer.getByText('已有相同检查记录，请修正文件并重新上传校验', { exact: true }).waitFor()
    const afterConflict = await (await referenceContext.request.get(base + '/api/v1/inspections?inspectionTypeId=' + batch.inspectionTypeId, { headers })).json()
    check(afterConflict.total === 1 && afterConflict.items[0].id === manual.id, 'Duplicate conflict wrote partial import')
    const remove = await referenceContext.request.post(base + '/api/v1/inspections/batch', { headers, data: { action: 'DELETE', items: [{ id: manual.id, version: manual.rowVersion }], reason: 'isolated duplicate recovery' } })
    check(remove.status() === 204, 'Could not retire isolated duplicate')
    cases.push('真实手工记录与预览并发重复：409明确提示、无部分入库、撤销冲突草稿后可重试')

  } finally { await referenceContext.close() }
  await drawer.getByRole('button', { name: '确认导入 2 条记录' }).click()
  await response('POST', `/api/v1/imports/${batch.id}/confirm`, () => page.getByRole('button', { name: '确认写入草稿记录' }).click(), 200)
  await drawer.getByText('已成功导入 2 条草稿记录', { exact: true }).waitFor()

  await drawer.getByRole('button', { name: 'Close', exact: true }).click()
  await drawer.waitFor({ state: 'hidden' })
  await page.getByRole('button', { name: '导入批次', exact: true }).first().click()
  drawer = page.getByRole('dialog', { name: '批量导入教室卫生', exact: true })
  await drawer.getByText('已成功导入 2 条草稿记录', { exact: true }).waitFor()
  await drawer.getByRole('button', { name: '整批回滚', exact: true }).click()
  let modal = page.getByRole('dialog', { name: '确认整批回滚', exact: true })
  await modal.getByRole('button', { name: '确认回滚全部记录', exact: true }).click()
  await modal.getByText('请填写回滚原因', { exact: true }).waitFor()
  await modal.getByLabel('回滚原因').fill('隔离浏览器整批误导入回归')
  const fail = route => route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify({ code: 'IMPORT_SERVICE_UNAVAILABLE', message: '导入服务暂时不可用' }) })
  await page.route('**/api/v1/imports/*/rollback', fail)
  await modal.getByRole('button', { name: '确认回滚全部记录', exact: true }).click()
  await modal.getByText('回滚失败，请稍后重试。', { exact: true }).waitFor()
  check(await modal.getByLabel('回滚原因').inputValue() === '隔离浏览器整批误导入回归', 'Failure lost reason')
  await page.unroute('**/api/v1/imports/*/rollback', fail)
  await response('POST', `/api/v1/imports/${batch.id}/rollback`, () => modal.getByRole('button', { name: '确认回滚全部记录', exact: true }).click(), 200)
  await modal.waitFor({ state: 'hidden' })
  await drawer.getByText('已整批回滚 2 条记录', { exact: true }).waitFor()
  check(await drawer.getByText('已成功导入 2 条草稿记录', { exact: true }).count() === 0, 'Stale import success after rollback')
  await drawer.getByRole('button', { name: 'Close', exact: true }).click()
  await drawer.waitFor({ state: 'hidden' })
  await page.getByText('当前范围暂无教室卫生记录', { exact: true }).waitFor()
  cases.push('列表批次入口、整批二次确认、必填原因、500保留重试、成功后批次与列表缓存同步')
  drawer = await openImport(true)
  await select(drawer, '导入目标部门', '学习部')
  await drawer.locator('input[type=file]').setInputFiles(fixture.validFile)
  const uploaded = await response('POST', '/api/v1/imports/inspections', () => drawer.getByRole('button', { name: '上传并生成预览' }).click(), 201)
  const second = await uploaded.json()
  await drawer.getByRole('button', { name: '确认导入 2 条记录' }).click()
  await response('POST', `/api/v1/imports/${second.id}/confirm`, () => page.getByRole('button', { name: '确认写入草稿记录' }).click(), 200)
  await drawer.getByText('已成功导入 2 条草稿记录', { exact: true }).waitFor()
  const context = await page.context().browser().newContext()
  try {
    const login = await context.request.post(base + '/api/v1/auth/login', { data: { loginName: 'browser-admin', password: fixture.authPassword } })
    check(login.status() === 200, 'Isolated admin login failed')
    const headers = { Authorization: 'Bearer ' + (await login.json()).accessToken }
    const list = await (await context.request.get(base + '/api/v1/inspections?inspectionTypeId=' + second.inspectionTypeId, { headers })).json()
    const original = list.items[0]
    const patch = await context.request.patch(base + '/api/v1/inspections/' + original.id, { headers, data: { version: original.rowVersion, reason: '阻止回滚的真实编辑', record: { semesterId: original.semesterId, weekId: original.weekId, inspectionTypeId: original.inspectionTypeId, departmentId: original.departmentId, details: { ...original.details, remark: '已核对修改' } } } })
    check(patch.status() === 200, 'Could not prepare edited record')
    const unauth = await context.request.post(base + '/api/v1/imports/' + second.id + '/rollback', { data: { version: second.version, reason: '未授权' } })
    check(unauth.status() === 401, 'Unauthenticated rollback accepted')
    await drawer.getByRole('button', { name: '整批回滚', exact: true }).click()
    modal = page.getByRole('dialog', { name: '确认整批回滚', exact: true })
    await modal.getByLabel('回滚原因').fill('应安全拒绝')
    const rejected = await response('POST', `/api/v1/imports/${second.id}/rollback`, () => modal.getByRole('button', { name: '确认回滚全部记录', exact: true }).click(), 409)
    check((await rejected.json()).code === 'IMPORT_ROLLBACK_UNSAFE', 'Wrong unsafe rollback code')
    await modal.getByText('批次含已修改、已提交、已锁定或缺失记录，不能整批回滚', { exact: true }).waitFor()
    check(await modal.getByLabel('回滚原因').inputValue() === '应安全拒绝', '409 lost reason')
    const after = await (await context.request.get(base + '/api/v1/inspections?inspectionTypeId=' + second.inspectionTypeId, { headers })).json()
    check(after.total === 2, 'Rejected rollback removed part of the batch')
  } finally { await context.close() }
  await modal.getByRole('button', { name: /取\s*消/ }).click()
  await modal.waitFor({ state: 'hidden' })
  await drawer.getByRole('button', { name: '整批回滚', exact: true }).click()
  modal = page.getByRole('dialog', { name: '确认整批回滚', exact: true })
  check(await modal.getByLabel('回滚原因').inputValue() === '', 'Reopen leaked previous reason')
  cases.push('回滚后可重新导入；真实编辑后整批409零改动、401保护、关闭重开清空输入')
  check(browserErrors.length === 0, 'Browser runtime errors')
  return { cases, browserErrors, batchId: batch.id, secondBatchId: second.id, browserVersion: page.context().browser().version(), faultInjection: ['回滚POST 500'] }
}
