async (page) => {
  const fixture = __FIXTURE__
  const base = page.url().split('/').slice(0, 3).join('/')
  const cases = []
  const errors = []
  page.on('pageerror', error => errors.push(error.message))

  await page.goto(base + '/admin/master-data')
  await page.getByRole('heading', { name: '基础数据', exact: true }).waitFor()
  await page.getByRole('menuitem', { name: '院系', exact: true }).click()
  await page.getByRole('button', { name: '导入Excel', exact: true }).click()
  let dialog = page.getByRole('dialog', { name: '导入院系', exact: true })
  await dialog.waitFor()
  if (!(await dialog.evaluate(element => Boolean(element.closest('.ant-drawer-content-wrapper'))))) {
    throw new Error('Master-data Excel import did not open in a Drawer')
  }
  await dialog.getByText('院系名称（必填）', { exact: false }).waitFor()

  const templateResponse = page.waitForResponse(response => response.url().includes('/api/v1/admin/master-data/faculties/import-template'))
  await dialog.getByRole('button', { name: '下载当前分类模板', exact: true }).click()
  const template = await templateResponse
  const templateContentType = await template.headerValue('content-type')
  if (template.status() !== 200 || !templateContentType?.includes('spreadsheetml.sheet')) {
    throw new Error('Faculty import template download did not return an XLSX workbook')
  }
  cases.push('真实页面以抽屉展示院系字段要求并下载当前分类 XLSX 模板')

  await dialog.getByRole('button', { name: /取\s*消/ }).click()
  await dialog.waitFor({ state: 'hidden' })
  await page.getByRole('button', { name: '导入Excel', exact: true }).click()
  dialog = page.getByRole('dialog', { name: '导入院系', exact: true })
  await dialog.getByText('未选择文件', { exact: true }).waitFor()
  if (await dialog.getByText('文件校验失败', { exact: true }).count()) throw new Error('Closed import dialog retained the previous error')

  await dialog.locator('input[aria-label="选择基础数据导入文件"]').setInputFiles(fixture.invalidMasterDataImportFile)
  const uploadResponse = page.waitForResponse(response => response.url().includes('/api/v1/admin/master-data/faculties/imports') && response.request().method() === 'POST')
  await dialog.getByRole('button', { name: '上传并预览', exact: true }).click()
  const response = await uploadResponse
  const body = await response.json()
  if (response.status() !== 422 || body.code !== 'INVALID_XLSX_SIGNATURE') {
    throw new Error(`Malformed workbook response was ${response.status()} ${body.code ?? 'unknown'}`)
  }
  await dialog.getByText('文件校验失败', { exact: true }).waitFor()
  if (await dialog.getByRole('button', { name: '确认新增', exact: true }).count()) throw new Error('Malformed workbook exposed confirmation')
  cases.push('无效 XLSX 在真实页面展示安全校验错误，且不可确认写入')

  await dialog.getByRole('button', { name: /取\s*消/ }).click()
  await dialog.waitFor({ state: 'hidden' })
  await page.getByRole('button', { name: '导入Excel', exact: true }).click()
  dialog = page.getByRole('dialog', { name: '导入院系', exact: true })
  await dialog.getByText('未选择文件', { exact: true }).waitFor()
  if (await dialog.getByText('文件校验失败', { exact: true }).count()) throw new Error('Reopened import dialog retained the previous upload error')
  cases.push('取消后重开导入抽屉清空文件与失败状态')

  await dialog.getByRole('button', { name: /取\s*消/ }).click()
  await dialog.waitFor({ state: 'hidden' })
  await page.getByRole('menuitem', { name: '学生', exact: true }).click()
  await page.getByRole('button', { name: '导入Excel', exact: true }).click()
  const studentDialog = page.getByRole('dialog', { name: '导入学生', exact: true })
  await studentDialog.locator('input[aria-label="选择基础数据导入文件"]').setInputFiles(fixture.studentImportFile)
  const studentPreviewPromise = page.waitForResponse(response => response.url().includes('/api/v1/admin/master-data/students/imports') && response.request().method() === 'POST')
  await studentDialog.getByRole('button', { name: '上传并预览', exact: true }).click()
  const studentPreviewResponse = await studentPreviewPromise
  const studentPreview = await studentPreviewResponse.json()
  if (studentPreviewResponse.status() !== 201 || studentPreview.validRows !== 1 || studentPreview.invalidRows !== 0) {
    throw new Error(`Student preview was ${studentPreviewResponse.status()} with ${studentPreview.validRows} valid and ${studentPreview.invalidRows} invalid rows`)
  }
  await studentDialog.getByText('预览通过，请核对后确认新增', { exact: true }).waitFor()

  // The deployed trial is plain HTTP on an IP address, where randomUUID is absent.
  await page.evaluate(() => Object.defineProperty(window.crypto, 'randomUUID', { configurable: true, value: undefined }))
  const confirmURL = `**/api/v1/admin/master-data/students/imports/${studentPreview.id}/confirm`
  let releaseConfirmRequest
  const confirmRequestGate = new Promise(resolve => { releaseConfirmRequest = resolve })
  await page.route(confirmURL, async route => {
    await confirmRequestGate
    await route.continue()
  })
  const confirmRequestPromise = page.waitForRequest(request => request.url().includes(`/students/imports/${studentPreview.id}/confirm`) && request.method() === 'POST', { timeout: 5000 })
  const confirmResponsePromise = page.waitForResponse(response => response.url().includes(`/students/imports/${studentPreview.id}/confirm`) && response.request().method() === 'POST')
  await studentDialog.getByRole('button', { name: '确认新增', exact: true }).click()
  await page.getByRole('button', { name: '确认导入', exact: true }).click()
  await confirmRequestPromise
  await studentDialog.locator('button.ant-btn-loading').waitFor({ timeout: 3000 })
  releaseConfirmRequest()
  const confirmResponse = await confirmResponsePromise
  await page.unroute(confirmURL)
  const confirmation = await confirmResponse.json()
  if (confirmResponse.status() !== 200 || confirmation.importedRows !== 1) {
    throw new Error(`Student confirmation response was ${confirmResponse.status()} with ${confirmation.importedRows} imported rows`)
  }
  await page.getByText('成功新增 1 条学生数据', { exact: true }).waitFor()
  cases.push('HTTP 环境 crypto.randomUUID 缺失时，确认按钮显示加载并完成学生新增')

  if (errors.length) throw new Error('Uncaught browser errors: ' + errors.join('; '))
  return {
    cases,
    browserErrors: errors,
    browserVersion: await page.context().browser().version(),
    apiError: { status: response.status(), code: body.code },
  }
}
