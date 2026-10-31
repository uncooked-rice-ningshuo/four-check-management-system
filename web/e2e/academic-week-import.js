async (page) => {
  const fixture = __FIXTURE__
  const base = page.url().split('/').slice(0, 3).join('/')
  const cases = []
  const errors = []
  let confirmationAttempts = 0
  let originalIdempotencyKey = ''
  page.on('pageerror', error => errors.push(error.message))
  const chooseWorkbook = async (drawer, filePath, filename) => {
    await drawer.locator('input[type="file"]').last().setInputFiles(filePath)
    await drawer.getByText(filename, { exact: true }).waitFor({ state: 'visible' })
    const uploadButton = drawer.getByRole('button', { name: /上传并生成预览/ })
    if (!(await uploadButton.isEnabled())) {
      const fileInputs = await drawer.locator('input[type="file"]').evaluateAll(inputs => inputs.map(input => ({
        fileName: input.files?.[0]?.name ?? '',
        accept: input.getAttribute('accept'),
      })))
      throw new Error(`Workbook ${filename} was selected but the preview button stayed disabled: ${JSON.stringify(fileInputs)}`)
    }
  }
  const postPreviewOrDiagnose = async (drawer, filename) => {
    const requestPromise = page.waitForRequest(
      request => request.url().includes('/api/v1/admin/academic/weeks/imports') && request.method() === 'POST',
      { timeout: 5000 },
    )
    await drawer.getByRole('button', { name: /上传并生成预览/ }).click()
    try {
      const request = await requestPromise
      const response = await request.response()
      if (!response) throw new Error('preview request ended without a response')
      return response
    } catch (error) {
      const details = {
        drawerVisible: await drawer.isVisible(),
        alerts: await drawer.locator('.ant-alert').allTextContents(),
        buttons: await drawer.getByRole('button').evaluateAll(buttons => buttons.map(button => ({ text: button.innerText, disabled: button.disabled, ariaLabel: button.getAttribute('aria-label') }))),
        fileInputs: await drawer.locator('input[type="file"]').evaluateAll(inputs => inputs.map(input => ({ fileName: input.files?.[0]?.name ?? '', accept: input.getAttribute('accept') }))),
      }
      throw new Error(`No preview request after selecting ${filename}: ${JSON.stringify(details)}; ${String(error)}`)
    }
  }

  await page.goto(base + '/admin/academic')
  await page.getByRole('heading', { name: '学期与周次', exact: true }).waitFor()
  const templateResponsePromise = page.waitForResponse(response => response.url().includes('/api/v1/admin/academic/weeks/import-template'))
  await page.getByRole('button', { name: /导入周次/ }).click()
  let drawer = page.locator('.ant-drawer-open [role="dialog"]').last()
  await drawer.waitFor()
  let target = drawer.getByRole('combobox', { name: '导入目标学期' })
  if (await target.inputValue() !== '') throw new Error('Import drawer selected a semester without user input')
  await target.click()
  await target.press('ArrowDown')
  await target.press('Enter')
  await drawer.getByTitle(/浏览器回归学期/).waitFor()

  const templateDownloadPromise = page.waitForEvent('download')
  await drawer.getByRole('button', { name: /下载周次导入模板/ }).click()
  const templateResponse = await templateResponsePromise
  const templateDownload = await templateDownloadPromise
  if (templateResponse.status() !== 200 || !(await templateResponse.headerValue('content-type'))?.includes('spreadsheetml.sheet')) {
    throw new Error('Academic week template download did not return an XLSX workbook')
  }
  if (templateDownload.suggestedFilename() !== '周次导入模板.xlsx') throw new Error('Academic week template download used an unexpected filename')
  cases.push('真实校历抽屉要求明确选择学期并下载当前版本 XLSX 模板')

  await chooseWorkbook(drawer, fixture.invalidAcademicWeekImportFile, 'academic-week-invalid.xlsx')
  const invalidPreviewResponse = await postPreviewOrDiagnose(drawer, 'academic-week-invalid.xlsx')
  const invalidPreview = await invalidPreviewResponse.json()
  if (invalidPreviewResponse.status() !== 201 || invalidPreview.invalidRows !== 1 || invalidPreview.status !== 'PREVIEW_HAS_ERRORS') {
    throw new Error(`Invalid preview was ${invalidPreviewResponse.status()} with ${invalidPreview.invalidRows} invalid rows`)
  }
  await drawer.getByText('存在错误行，整批不能确认。请修正 Excel 后重新上传。', { exact: true }).waitFor()
  const blockedConfirm = drawer.getByRole('button', { name: /确认新增/ })
  if (!(await blockedConfirm.isDisabled())) throw new Error('Invalid academic-week preview allowed confirmation')
  cases.push('无效周次行显示逐行校验结果且阻止整批确认')

  await drawer.getByRole('button', { name: '更换文件', exact: true }).click()
  await chooseWorkbook(drawer, fixture.malformedAcademicWeekImportFile, 'academic-week-corrupt.xlsx')
  const malformedResponse = await postPreviewOrDiagnose(drawer, 'academic-week-corrupt.xlsx')
  if (malformedResponse.status() !== 422) throw new Error(`Malformed workbook returned ${malformedResponse.status()} instead of 422`)
  await drawer.getByRole('alert').last().waitFor()
  cases.push('模板代码不符时安全拒绝，页面显示错误并执行未绑定源文件补偿清理')

  await chooseWorkbook(drawer, fixture.validAcademicWeekImportFile, 'academic-week-valid.xlsx')
  const validPreviewResponse = await postPreviewOrDiagnose(drawer, 'academic-week-valid.xlsx')
  const validPreview = await validPreviewResponse.json()
  if (validPreviewResponse.status() !== 201 || validPreview.invalidRows !== 0 || validPreview.validRows !== 1 || !validPreview.affectsToday) {
    throw new Error(`Valid preview did not recognize today's week: ${JSON.stringify(validPreview)}`)
  }
  await drawer.getByText('导入日期覆盖今天，确认后可能改变系统自动识别的当前工作周。', { exact: true }).waitFor()

  const staleConfirmURL = `**/api/v1/admin/academic/weeks/imports/${validPreview.batchId}/confirm`
  await page.route(staleConfirmURL, route => route.fulfill({
    status: 409,
    contentType: 'application/json',
    body: JSON.stringify({ code: 'IMPORT_PREVIEW_STALE', message: '预览后校历已变化或日期发生冲突，请重新上传并校验' }),
  }))
  await drawer.getByRole('button', { name: /确认新增 1 个周次/ }).click()
  const staleResponsePromise = page.waitForResponse(response => response.url().includes(`/academic/weeks/imports/${validPreview.batchId}/confirm`) && response.request().method() === 'POST')
  await page.getByRole('dialog').last().getByRole('button', { name: '确认导入', exact: true }).click()
  const staleResponse = await staleResponsePromise
  await page.unroute(staleConfirmURL)
  if (staleResponse.status() !== 409) throw new Error(`Stale preview response was ${staleResponse.status()}`)
  await drawer.getByText('预览后校历已变化或日期发生冲突，请重新上传并校验', { exact: true }).waitFor()
  await drawer.getByRole('button', { name: '更换文件', exact: true }).click()
  await chooseWorkbook(drawer, fixture.validAcademicWeekImportFile, 'academic-week-valid.xlsx')
  const refreshedPreviewResponse = await postPreviewOrDiagnose(drawer, 'academic-week-valid.xlsx')
  const refreshedPreview = await refreshedPreviewResponse.json()
  if (refreshedPreviewResponse.status() !== 201 || refreshedPreview.invalidRows !== 0 || refreshedPreview.validRows !== 1 || !refreshedPreview.affectsToday) {
    throw new Error('Re-upload after stale preview did not produce a valid new preview')
  }
  await drawer.getByText('导入日期覆盖今天，确认后可能改变系统自动识别的当前工作周。', { exact: true }).waitFor()
  cases.push('预览后冲突返回 409 并要求重新上传；重新预览后继续确认')

  const confirmURL = `**/api/v1/admin/academic/weeks/imports/${refreshedPreview.batchId}/confirm`
  await page.route(confirmURL, async route => {
    const idempotencyKey = route.request().headers()['idempotency-key'] ?? ''
    if (!idempotencyKey) throw new Error('Academic-week confirmation omitted its idempotency key')
    if (confirmationAttempts === 0) originalIdempotencyKey = idempotencyKey
    else if (idempotencyKey !== originalIdempotencyKey) throw new Error('Retry used a new idempotency key')
    confirmationAttempts += 1
    if (confirmationAttempts === 1) {
      const committedResponse = await route.fetch()
      if (committedResponse.status() !== 200) throw new Error(`First confirmation did not commit: ${committedResponse.status()}`)
      await route.fulfill({ status: 502, contentType: 'application/json', body: JSON.stringify({ code: 'SIMULATED_RESPONSE_LOSS', message: '模拟确认响应丢失' }) })
      return
    }
    await route.continue()
  })

  await drawer.getByRole('button', { name: /确认新增 1 个周次/ }).click()
  await page.getByRole('dialog').last().getByRole('button', { name: '确认导入', exact: true }).click()
  await drawer.getByText('模拟确认响应丢失', { exact: true }).waitFor()
  await drawer.getByRole('button', { name: /确认新增 1 个周次/ }).click()
  const retryResponsePromise = page.waitForResponse(response => response.url().includes(`/academic/weeks/imports/${refreshedPreview.batchId}/confirm`) && response.request().method() === 'POST')
  await page.getByRole('dialog').last().getByRole('button', { name: '确认导入', exact: true }).click()
  const retryResponse = await retryResponsePromise
  const confirmation = await retryResponse.json()
  await page.unroute(confirmURL)
  if (retryResponse.status() !== 200 || confirmation.importedRows !== 1 || confirmationAttempts !== 2 || !originalIdempotencyKey) {
    throw new Error(`Idempotent confirmation retry failed: ${retryResponse.status()} ${JSON.stringify(confirmation)}`)
  }
  await drawer.getByText('成功新增 1 个周次。', { exact: true }).waitFor()
  await page.locator('.work-context-week').filter({ hasText: '第 1 周' }).waitFor()
  cases.push('真实确认后刷新当前工作周；模拟响应丢失后用同一幂等键安全重试')

  await drawer.getByRole('button', { name: 'Close' }).click()
  await drawer.waitFor({ state: 'hidden' })
  await page.getByRole('button', { name: /导入周次/ }).click()
  drawer = page.locator('.ant-drawer-open [role="dialog"]').last()
  await drawer.waitFor()
  target = drawer.getByRole('combobox', { name: '导入目标学期' })
  if (await target.inputValue() !== '' || await drawer.getByText('academic-week-valid.xlsx', { exact: true }).count() !== 0) {
    throw new Error('Closing and reopening the drawer retained the target semester or selected file')
  }
  cases.push('确认后关闭并重开抽屉，目标学期和文件选择均已清空')

  if (errors.length) throw new Error('Uncaught browser errors: ' + errors.join('; '))
  return { cases, browserErrors: errors, browserVersion: await page.context().browser().version(), confirmationAttempts }
}
