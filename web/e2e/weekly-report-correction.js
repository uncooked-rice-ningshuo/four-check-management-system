async (page) => {
  const fixture = __FIXTURE__
  const base = page.url().split('/').slice(0, 3).join('/')
  const cases = []
  const browserErrors = []
  page.on('pageerror', error => browserErrors.push(error.message))
  const check = (condition, message) => { if (!condition) throw new Error(message) }
  const closeVersionDrawer = async (drawer, versionNo) => {
    await drawer.getByRole('button', { name: 'Close', exact: true }).click()
    await page.getByRole('dialog', { name: `V${versionNo} · 数据明细`, exact: true }).waitFor({ state: 'hidden' })
    await drawer.locator('.ant-drawer-mask').waitFor({ state: 'hidden' })
  }
  const closeArchiveDrawer = async drawer => {
    await drawer.getByRole('button', { name: 'Close', exact: true }).click()
    await drawer.getByRole('dialog').waitFor({ state: 'hidden' })
    await drawer.locator('.ant-drawer-mask').waitFor({ state: 'hidden' })
  }
  const selectSnapshotFilter = async (filters, label, searchText, optionName) => {
    const select = filters.getByRole('combobox', { name: label, exact: true })
    await select.click()
    await select.fill(searchText)
    await select.press('Enter')
    await filters.getByTitle(optionName, { exact: true }).waitFor()
  }
  const inspectVersionSnapshot = async (drawer, versionNo, studentName, expectedDate) => {
    const versionSnapshot = drawer.getByRole('region', { name: `V${versionNo} 数据明细`, exact: true })
    const category = versionSnapshot.getByRole('tab', { name: /到课率/ })
    await category.click()
    await versionSnapshot.getByRole('row').filter({ hasText: studentName })
      .filter({ hasText: `${expectedDate} 08:00` }).waitFor()
    if (versionNo === 1) {
      const filters = versionSnapshot.getByRole('form', { name: '筛选本版记录' })
      await selectSnapshotFilter(filters, '学号', '0000', '000001')
      await selectSnapshotFilter(filters, '姓名', studentName.slice(0, 8), studentName)
      await selectSnapshotFilter(filters, '班级', 'E2E', 'E2E class')
      await selectSnapshotFilter(filters, '年级', '2026', '2026级')
      const selectedFilters = await filters.locator('.ant-select').allTextContents()
      check(['000001', studentName, 'E2E class', '2026级'].every(value => selectedFilters.some(item => item.includes(value))),
        `Searchable select values were not retained before submit: ${selectedFilters.join(' | ')}`)
      const recordsResponsePromise = page.waitForResponse(response => response.request().method() === 'GET' &&
        response.url().includes(`/versions/${versionNo}/records?`))
      await filters.locator('.report-version-record-filter-actions .ant-btn-primary').click()
      let recordsResponse
      try {
        recordsResponse = await recordsResponsePromise
      } catch (error) {
        const requests = await page.evaluate(() => performance.getEntriesByType('resource')
          .map(entry => entry.name).filter(url => url.includes('/versions/') && url.includes('/records?')))
        throw new Error(`No version-record response after applying filters; observed requests: ${requests.join(' | ')}; ${error}`)
      }
      const filterParams = await page.evaluate(url => Object.fromEntries(new URL(url).searchParams.entries()), recordsResponse.url())
      check(recordsResponse.status() === 200 && filterParams.studentNumber === '000001' &&
        filterParams.studentName === studentName && filterParams.className === 'E2E class' &&
        filterParams.grade === '2026级',
      `Snapshot filter request did not carry all selected values: ${recordsResponse.status()} ${recordsResponse.url()}`)
    }
    const recordRow = versionSnapshot.getByRole('row').filter({ hasText: studentName })
      .filter({ hasText: `${expectedDate} 08:00` })
    await recordRow.waitFor()
    const summary = await recordRow.innerText()
    check(await versionSnapshot.getByRole('columnheader', { name: '基本信息', exact: true }).count() === 1 &&
      await versionSnapshot.getByRole('columnheader', { name: '课程与地点', exact: true }).count() === 1 &&
      await versionSnapshot.getByRole('columnheader', { name: '到课情况', exact: true }).count() === 1,
    `V${versionNo} snapshot table should use attendance-specific headings`)
    for (const value of [studentName, '000001', 'E2E class', '2026级', `${expectedDate} 08:00`,
      '课程', 'E2E course', '教室', 'E2E building 101', '发生节次', '1', '缺课节次数', '1 节次', '违纪计入次数', '1 次']) {
      check(summary.includes(value), `V${versionNo} table cell did not show ${value}: ${summary}`)
    }
    check(!summary.includes('记录方式') && !summary.includes('情况说明'),
      `V${versionNo} table should omit the removed detailed-information section: ${summary}`)
    check(await versionSnapshot.getByRole('button', { name: 'Expand row', exact: true }).count() === 0,
      `V${versionNo} table should not expose an expanded details row`)
  }

  await page.setViewportSize({ width: 1280, height: 900 })
  await page.goto(`${base}/login`)
  const loginResponsePromise = page.waitForResponse(response =>
    response.url().includes('/api/v1/auth/login') && response.request().method() === 'POST')
  await page.getByRole('textbox', { name: '账号', exact: true }).fill('browser-admin')
  await page.getByLabel('密码', { exact: true }).fill(fixture.authPassword)
  await page.getByRole('button', { name: /登\s*录/ }).click()
  const loginResponse = await loginResponsePromise
  check(loginResponse.status() === 200, `Synthetic office admin login failed: ${loginResponse.status()}`)
  await page.getByRole('group', { name: '当前工作上下文' }).waitFor()

  await page.goto(`${base}/reports/archive`)
  await page.getByRole('heading', { name: '通报归档', exact: true }).waitFor()
  const archiveHeadingOffset = await page.locator('.weekly-report-archive-page .attendance-page-heading').evaluate(element => {
    const content = element.closest('.admin-content')
    return content ? element.getBoundingClientRect().top - content.getBoundingClientRect().top : -1
  })
  const archiveRow = page.getByRole('row').filter({ hasText: '浏览器回归学期' })
  await archiveRow.waitFor()
  await archiveRow.getByRole('button', { name: '查看版本', exact: true }).click()
  await page.getByText('归档快照为只读记录', { exact: true }).waitFor()
  const initialArchiveDrawer = page.locator('.weekly-report-archive-drawer')
  await initialArchiveDrawer.getByRole('button', { name: '查看明细', exact: true }).click()
  const initialVersionDrawer = page.locator('.weekly-report-version-detail-drawer')
  await inspectVersionSnapshot(initialVersionDrawer, 1, 'E2E student A', fixture.date)
  await closeVersionDrawer(initialVersionDrawer, 1)
  await closeArchiveDrawer(initialArchiveDrawer)
  await archiveRow.getByRole('button', { name: '基于定稿修正', exact: true }).click()
  await page.waitForURL('**/reports/workbench?*stage=1')
  await page.getByRole('heading', { name: '通报工作台', exact: true }).waitFor()
  await page.getByText('已定稿 · V1', { exact: true }).waitFor()
  const workbenchHeadingOffset = await page.locator('.weekly-report-summary-page .attendance-page-heading').evaluate(element => {
    const content = element.closest('.admin-content')
    return content ? element.getBoundingClientRect().top - content.getBoundingClientRect().top : -1
  })
  check(archiveHeadingOffset >= 0 && Math.abs(archiveHeadingOffset - workbenchHeadingOffset) <= 2,
    `Archive and workbench page headings have different top spacing: ${archiveHeadingOffset}px vs ${workbenchHeadingOffset}px`)
  cases.push('通报归档与工作台复用相同页面标题容器和顶部节奏')

  const versionHistory = page.locator('.weekly-report-history')
  const v1HistoryRow = versionHistory.getByRole('row').filter({ hasText: 'V1' }).last()
  await v1HistoryRow.waitFor()
  check((await v1HistoryRow.innerText()).includes('全周可见数据') &&
    await versionHistory.getByRole('link', { name: '打开通报归档' }).count() === 1,
  'Readonly version history did not show the saved version and archive entry')
  cases.push('工作台展示只读版本记录，并通过归档查看冻结明细')

  const recordsResponsePromise = page.waitForResponse(response =>
    response.status() === 200 && response.url().includes('/api/v1/reports/workflow/') &&
    response.url().includes('/versions/1/records?'))
  await page.getByRole('button', { name: '查看明细', exact: true }).first().click()
  const recordsResponse = await recordsResponsePromise
  const records = await recordsResponse.json()
  check(records.items.some(item => item.studentName === 'E2E student A'),
    `The attendance detail snapshot did not contain the expected record: ${JSON.stringify(records)}`)

  const workflowDetailDrawer = page.locator('.weekly-report-record-drawer')
  const workflowFilters = workflowDetailDrawer.getByRole('form', { name: '筛选本版记录' })
  await selectSnapshotFilter(workflowFilters, '学号', '0000', '000001')
  await selectSnapshotFilter(workflowFilters, '姓名', 'E2E student', 'E2E student A')
  await selectSnapshotFilter(workflowFilters, '班级', 'E2E', 'E2E class')
  await selectSnapshotFilter(workflowFilters, '年级', '2026', '2026级')
  const workflowFilterResponsePromise = page.waitForResponse(response => response.request().method() === 'GET' &&
    response.url().includes('/versions/1/records?'))
  await workflowFilters.locator('.report-version-record-filter-actions .ant-btn-primary').click()
  const workflowFilterResponse = await workflowFilterResponsePromise
  const workflowFilterParams = await page.evaluate(url => Object.fromEntries(new URL(url).searchParams.entries()), workflowFilterResponse.url())
  const workflowFilteredRecords = await workflowFilterResponse.json()
  check(workflowFilterResponse.status() === 200 && workflowFilterParams.studentNumber === '000001' &&
    workflowFilterParams.studentName === 'E2E student A' && workflowFilterParams.className === 'E2E class' &&
    workflowFilterParams.grade === '2026级' && workflowFilteredRecords.total === 2 &&
    workflowFilteredRecords.items.length === 2 && workflowFilteredRecords.items.every(item => item.studentName === 'E2E student A'),
  `Workbench filter request did not carry all values or return only the matching record: ${workflowFilterResponse.status()} ${workflowFilterResponse.url()} ${JSON.stringify(workflowFilteredRecords)}`)

  const recordRow = workflowDetailDrawer.getByRole('row').filter({ hasText: 'E2E student A' })
    .filter({ hasText: `${fixture.date} 08:00` })
  const recordSummary = await recordRow.innerText()
  check(['E2E student A', '000001', 'E2E class', '2026级', `${fixture.date} 08:00`,
    '课程', 'E2E course', '教室', 'E2E building 101', '发生节次', '1', '缺课节次数', '1 节次', '违纪计入次数', '1 次']
    .every(value => recordSummary.includes(value)) && !recordSummary.includes('记录方式') && !recordSummary.includes('情况说明'),
  `The filtered workbench row did not show grouped snapshot columns: ${recordSummary}`)
  check(await workflowDetailDrawer.getByRole('button', { name: 'Expand row', exact: true }).count() === 0,
    'The workbench record table should not expose a detailed-information expansion')
  await recordRow.getByRole('button', { name: '修正', exact: true }).click()
  let correctionDialog = page.locator('.report-correction-modal')
  await correctionDialog.waitFor()
  for (const field of ['学号', '年级', '检查日期', '发生时间', '节次', '缺课节次数', '课程', '教室']) {
    await correctionDialog.getByText(field, { exact: true }).first().waitFor()
  }
  const studentNameInput = correctionDialog.getByLabel('姓名', { exact: true })
  check(await studentNameInput.inputValue() === 'E2E student A', 'Correction editor did not prefill the selected record name')
  check(await correctionDialog.getByRole('spinbutton', { name: '缺课节次数', exact: true }).count() === 1,
    'Attendance penalty input did not expose its displayed type-specific label to assistive technology')
  const unwrappedNumberInputs = await correctionDialog.locator('input[type="number"]').evaluateAll(inputs =>
    inputs.filter(input => !input.closest('.ant-input-number')).length)
  check(await correctionDialog.locator('.ant-input-number').count() > 0 &&
    await correctionDialog.locator('.ant-input').count() > 0 &&
    await correctionDialog.locator('input[type="date"], input[type="time"], select').count() === 0 &&
    unwrappedNumberInputs === 0,
  `Correction fields should use Ant Design inputs (unwrapped number inputs: ${unwrappedNumberInputs})`)
  const originalViewport = page.viewportSize()
  const desktopGridColumns = await correctionDialog.locator('.report-correction-field-grid').evaluate(element =>
    getComputedStyle(element).gridTemplateColumns.trim().split(/\s+/).length)
  const desktopDialogWidth = await correctionDialog.evaluate(element => parseFloat(getComputedStyle(element).width))
  check(desktopGridColumns >= 2 && desktopDialogWidth >= 900,
    `Correction form should use a wide desktop layout: ${desktopGridColumns} columns, ${desktopDialogWidth}px`)
  await page.setViewportSize({ width: 390, height: 844 })
  const mobileGridColumns = await correctionDialog.locator('.report-correction-field-grid').evaluate(element =>
    getComputedStyle(element).gridTemplateColumns.trim().split(/\s+/).length)
  const mobileDialogWidth = await correctionDialog.evaluate(element => parseFloat(getComputedStyle(element).width))
  check(mobileGridColumns === 1 && mobileDialogWidth <= 390,
    `Correction form should fit a single column on mobile: ${mobileGridColumns} columns, ${mobileDialogWidth}px`)
  if (originalViewport) await page.setViewportSize(originalViewport)
  cases.push('修正表单桌面双栏，390px 窄屏单栏且弹窗不超出视口')
  await studentNameInput.fill('未保存的姓名')
  await correctionDialog.getByRole('button', { name: /取\s*消/ }).click()

  await recordRow.getByRole('button', { name: '修正', exact: true }).click()
  correctionDialog = page.locator('.report-correction-modal')
  await correctionDialog.waitFor()
  check(await correctionDialog.getByLabel('姓名', { exact: true }).inputValue() === 'E2E student A',
    'Cancel and reopen retained an unsaved name from the previous correction session')
  check(await correctionDialog.getByRole('button', { name: '保存修改并生成新版本', exact: true }).isDisabled(),
    'Correction save should stay disabled until a field changes and an explanation is entered')

  const date = new Date(`${fixture.date}T00:00:00Z`)
  date.setUTCDate(date.getUTCDate() + 1)
  const correctedDate = date.toISOString().slice(0, 10)
  await correctionDialog.getByLabel('姓名', { exact: true }).fill('E2E corrected student')
  await correctionDialog.getByLabel('检查日期', { exact: true }).fill(correctedDate)
  await correctionDialog.getByLabel('处理说明', { exact: true }).fill('办公室依据临时凭据核对姓名和检查日期')
  const correctionRequestPromise = page.waitForRequest(request =>
    request.url().includes('/api/v1/reports/') && request.url().endsWith('/corrections') &&
    request.method() === 'POST')
  const correctionResponsePromise = page.waitForResponse(response =>
    response.url().includes('/api/v1/reports/') && response.url().endsWith('/corrections') &&
    response.request().method() === 'POST')
  await correctionDialog.getByRole('button', { name: '保存修改并生成新版本', exact: true }).click()
  const [correctionRequest, correctionResponse] = await Promise.all([correctionRequestPromise, correctionResponsePromise])
  const correctionRequestBody = correctionRequest.postDataJSON()
  const correctionResult = await correctionResponse.json()
  check(correctionResponse.status() === 201 && correctionResult.report.status === 'DRAFT' &&
    correctionResult.report.versionNo === 2,
  `Correction did not create a fresh draft version: ${correctionResponse.status()} ${JSON.stringify(correctionResult)}`)
  check(correctionRequestBody.changes.length === 2 && correctionRequestBody.changes.some(change =>
    change.field === 'studentName' && change.afterValue === 'E2E corrected student') && correctionRequestBody.changes.some(change =>
    change.field === 'details.date' && change.afterValue === correctedDate) &&
    correctionRequestBody.reason === '办公室依据临时凭据核对姓名和检查日期',
  `The form did not submit the edited fields and explanation together: ${JSON.stringify(correctionRequestBody)}`)
  await page.getByText('草稿 · V2', { exact: true }).waitFor()
  const submitButton = page.getByRole('button', { name: '草稿无误，进入定稿确认', exact: true })
  await submitButton.waitFor()
  check(await submitButton.isEnabled(), 'Office correction unexpectedly blocked draft submission')
  check(await page.getByText(/待部门重确认/).count() === 0, 'The new workflow still shows a department reconfirmation state')

  const correctedRecordRow = page.getByRole('row').filter({ hasText: 'E2E corrected student' }).last()
  await correctedRecordRow.waitFor()
  const correctedRecordText = await correctedRecordRow.innerText()
  check(correctedRecordText.includes(correctedDate),
    `The next report version did not display its corrected date: ${correctedRecordText}`)
  cases.push('定稿记录按流程返回工作台修正；多字段同次提交生成新草稿版本，无部门重确认门禁')
  cases.push('修正弹窗显示完整到课率字段；取消重开重置旧输入，当前版本明细加载新值')

  const correctedDetailDrawer = page.locator('.weekly-report-record-drawer')
  await correctedDetailDrawer.getByRole('button', { name: 'Close', exact: true }).click()
  await correctedDetailDrawer.waitFor({ state: 'hidden' })
  await page.getByRole('menuitem').filter({ hasText: '通报管理' }).click()
  await page.getByRole('link', { name: '通报归档', exact: true }).click()
  await page.getByRole('heading', { name: '通报归档', exact: true }).waitFor()
  await page.getByRole('button', { name: '查看版本', exact: true }).click()
  await page.getByText('追加修正记录', { exact: true }).waitFor()
  const correctionAudit = page.getByRole('table').last()
  const auditRows = await correctionAudit.locator('.ant-table-tbody tr').allInnerTexts()
  const auditText = auditRows.join('\n')
  const reason = '办公室依据临时凭据核对姓名和检查日期'
  const dateAuditRow = auditRows.find(row => row.includes('检查日期') && row.includes(fixture.date) && row.includes(correctedDate) && row.includes(reason))
  const nameAuditRow = auditRows.find(row => row.includes('姓名') && row.includes('E2E student A') && row.includes('E2E corrected student') && row.includes(reason))
  check(Boolean(dateAuditRow && nameAuditRow),
    `Archive field-level audit is incomplete: ${auditRows.join(' | ')}`)
  check(await page.getByRole('columnheader', { name: '部门重确认' }).count() === 0,
    'Archive still exposes the removed department confirmation column')
  const archiveDrawer = page.locator('.weekly-report-archive-drawer')
  const archivedVersionTable = archiveDrawer.locator('.ant-table').first()
  const archivedV1Row = archivedVersionTable.getByRole('row').filter({ hasText: 'V1' }).last()
  await archivedV1Row.getByRole('button', { name: '查看明细', exact: true }).click()
  const archivedV1Drawer = page.locator('.weekly-report-version-detail-drawer')
  await inspectVersionSnapshot(archivedV1Drawer, 1, 'E2E student A', fixture.date)
  await closeVersionDrawer(archivedV1Drawer, 1)
  const nameAuditRowLocator = archiveDrawer.locator('table').filter({ hasText: '原值' }).last()
    .locator('tbody tr').filter({ hasText: 'E2E corrected student' })
  await nameAuditRowLocator.getByRole('button', { name: '查看 V2 数据明细', exact: true }).click()
  const archivedV2Drawer = page.locator('.weekly-report-version-detail-drawer')
  await inspectVersionSnapshot(archivedV2Drawer, 2, 'E2E corrected student', correctedDate)
  cases.push('通报归档抽屉和修正审计可直达关联版本，并展示各自冻结的数据明细')

  const archiveViewport = page.viewportSize()
  await page.setViewportSize({ width: 390, height: 844 })
  await page.waitForFunction(() => {
    const wrapper = document.querySelector('.weekly-report-archive-drawer-wrapper')
    return wrapper instanceof HTMLElement && wrapper.getBoundingClientRect().width <= window.innerWidth
  })
  const mobileArchive = page.locator('.weekly-report-archive-page')
  const archiveHeaderDirection = await mobileArchive.locator('.attendance-page-heading').evaluate(element => getComputedStyle(element).flexDirection)
  const archiveTableOverflow = await mobileArchive.locator('.ant-table-content').first().evaluate(element => getComputedStyle(element).overflowX)
  const archiveContentFits = await page.locator('.admin-content').evaluate(element => element.scrollWidth <= element.clientWidth)
  const archiveDrawerWidth = await archiveDrawer.locator('.weekly-report-archive-drawer-wrapper').evaluate(element => element.getBoundingClientRect().width)
  const versionDrawerWidth = await page.locator('.weekly-report-version-detail-drawer-wrapper').evaluate(element => element.getBoundingClientRect().width)
  check(archiveHeaderDirection === 'column' && archiveTableOverflow === 'auto' && archiveContentFits && archiveDrawerWidth <= 390 && versionDrawerWidth <= 390,
    `Archive mobile layout overflowed or lost shared spacing: direction=${archiveHeaderDirection}, table=${archiveTableOverflow}, contentFits=${archiveContentFits}, drawer=${archiveDrawerWidth}px, version=${versionDrawerWidth}px`)
  if (archiveViewport) await page.setViewportSize(archiveViewport)
  cases.push('通报归档窄屏沿用统一标题布局，横向滚动收在表格内，版本抽屉适配视口')

  check(browserErrors.length === 0, `Uncaught browser errors: ${browserErrors.join('; ')}`)
  return { cases, browserErrors }
}
