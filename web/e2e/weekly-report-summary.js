async (page) => {
  const fixture = __FIXTURE__
  const base = page.url().split('/').slice(0, 3).join('/')
  const cases = []
  const browserErrors = []
  let summaryRequests = 0
  page.on('pageerror', error => browserErrors.push(error.message))
  page.on('request', request => {
    if (request.url().includes('/api/v1/reports/summary?')) summaryRequests += 1
  })
  const check = (value, message) => { if (!value) throw new Error(message) }
  const waitUntilEnabled = async locator => {
    const deadline = Date.now() + 15000
    while (!(await locator.isEnabled()) && Date.now() < deadline) await new Promise(resolve => setTimeout(resolve, 100))
    check(await locator.isEnabled(), 'Expected selector to become available after permissions refreshed')
  }
  const waitForSummaryIdle = () => page.waitForFunction(() => {
    const button = Array.from(document.querySelectorAll('button')).find(item => item.textContent?.includes('刷新汇总'))
    return button instanceof HTMLButtonElement && !button.disabled && !button.classList.contains('ant-btn-loading')
  })
  const selectWeek = async weekNo => {
    const combobox = page.getByRole('combobox', { name: '选择通报学期和周次' })
    await combobox.click()
    const option = page.locator('.ant-select-dropdown:visible .ant-select-item-option')
      .filter({ hasText: `第 ${weekNo} 周` })
    check(await option.count() === 1, `Expected exactly one available week ${weekNo} option`)
    await option.click()
  }

  const periodFailure = route => route.fulfill({
    status: 503,
    contentType: 'application/json',
    body: JSON.stringify({ code: 'SERVICE_UNAVAILABLE', message: 'synthetic unavailable' }),
  })
  await page.route('**/api/v1/reports/periods', periodFailure)
  await page.goto(base + '/reports/weekly-summary')
  await page.getByRole('heading', { name: '数据汇总与统计', exact: true }).waitFor()
  await page.getByText('可查看的学期周次加载失败。', { exact: true }).waitFor({ timeout: 15000 })
  check(summaryRequests === 0, 'Summary was requested before the user selected a reporting period')
  await page.unroute('**/api/v1/reports/periods', periodFailure)
  const firstPeriodSelector = page.getByRole('combobox', { name: '选择通报学期和周次' })
  const recoveredPeriodsResponse = page.waitForResponse(response =>
    response.url().endsWith('/api/v1/reports/periods'))
  const firstDefaultSummary = page.waitForResponse(response => response.status() === 200 &&
    response.url().includes('/api/v1/reports/summary?') && response.url().includes('weekId=b0000000-0000-0000-0000-000000000003'))
  await page.getByRole('alert').filter({ hasText: '可查看的学期周次加载失败。' })
    .getByRole('button').click()
  const recoveredPeriods = await recoveredPeriodsResponse
  check(recoveredPeriods.status() === 200,
    `Retrying report periods returned ${recoveredPeriods.status()}: ${JSON.stringify(await recoveredPeriods.json().catch(() => ({})))}`)
  const periodSelector = firstPeriodSelector
  await firstDefaultSummary
  await page.getByText('可查看的学期周次加载失败。', { exact: true }).waitFor({ state: 'hidden' })
  await waitUntilEnabled(periodSelector)
  await page.getByText(/第 1 周 ·/).first().waitFor()
  cases.push('周期列表 503 可重试；恢复后仅在可查看周期内自动选中系统当前工作周并加载汇总')

  const malformedPeriodDate = async route => {
    const response = await route.fetch()
    const payload = await response.json()
    check(payload.items.length > 0, 'Malformed-date scenario needs an available reporting period')
    payload.items[0].startDate = 'not-a-date'
    await route.fulfill({ response, body: JSON.stringify(payload) })
  }
  await page.route('**/api/v1/reports/periods', malformedPeriodDate)
  await page.reload()
  await page.getByText('可查看的学期周次加载失败。', { exact: true }).waitFor({ timeout: 15000 })
  check(await periodSelector.isDisabled(), 'Malformed period response should keep the period selector unavailable')
  await page.unroute('**/api/v1/reports/periods', malformedPeriodDate)
  const recoveredDefaultSummary = page.waitForResponse(response => response.status() === 200 &&
    response.url().includes('/api/v1/reports/summary?') && response.url().includes('weekId=b0000000-0000-0000-0000-000000000003'))
  await page.getByRole('alert').filter({ hasText: '可查看的学期周次加载失败。' })
    .getByRole('button').click()
  await recoveredDefaultSummary
  await page.getByText('可查看的学期周次加载失败。', { exact: true }).waitFor({ state: 'hidden' })
  await waitUntilEnabled(periodSelector)
  cases.push('周期接口返回不可解析日期时显示失败与重试状态；重试成功后恢复')

  await page.getByText('当前为提交数据预览').waitFor()
  await page.getByText(/不计算到课率/).waitFor()
  await page.getByText(/第 1 周 ·/).first().waitFor()
  await page.getByText('部门与校级批次汇总', { exact: true }).waitFor()
  const attendanceRow = page.locator('tr[data-row-key="department:10000000-0000-0000-0000-000000000002:type:30000000-0000-0000-0000-000000000001"]')
  await attendanceRow.waitFor()
  const cells = await attendanceRow.getByRole('cell').allInnerTexts()
  check(cells[2] === '1' && cells[6] === '1' && cells[7] === '2', `Summary did not use only the current confirmed revision: ${cells.join('|')}`)
  check(await page.getByRole('cell', { name: '教室卫生', exact: true }).count() === 0, 'Paused inspection type appeared in reporting summary')
  const collegeRow = page.locator('tr[data-row-key="college:type:30000000-0000-0000-0000-000000000001"]')
  const collegeCells = await collegeRow.getByRole('cell').allInnerTexts()
  check(collegeCells[2] === '1' && collegeCells[6] === '1' && collegeCells[7] === '2', `Unexpected college summary row: ${collegeCells.join('|')}`)
  const collegeRows = page.locator('tr[data-row-key="college"]')
  check(await collegeRows.count() === 1, `Summary showed college-level cells without a college submission: ${await collegeRows.allInnerTexts()}`)
  cases.push('自动选中当前工作周并加载真实汇总；只统计确认批次当前第 2 版修订快照，核对校级分列和暂停类型排除')

  const recordResponsePromise = page.waitForResponse(response =>
    response.url().includes('/api/v1/reports/records?') && response.url().includes('weekId=b0000000-0000-0000-0000-000000000003'))
  await attendanceRow.getByRole('button', { name: '查看已确认记录' }).click()
  const recordResponse = await recordResponsePromise
  check(recordResponse.status() === 200,
    `Records endpoint returned ${recordResponse.status()}: ${JSON.stringify(await recordResponse.json().catch(() => ({})))}`)
  const recordPage = await recordResponse.json()
  check(recordPage.total === 2 && recordPage.items.some(item => item.studentName === 'E2E student A') &&
    recordPage.items.some(item => item.studentName === 'E2E student B'),
  `Records endpoint did not return the two actual confirmed snapshot records: ${JSON.stringify(recordPage)}`)
  await page.getByText('以下为当前确认修订中的实际提交记录', { exact: true }).waitFor()
  await page.getByRole('row').filter({ hasText: 'E2E student A' }).waitFor()
  await page.getByRole('button', { name: 'Close', exact: true }).click()

  const collegeRecordsResponsePromise = page.waitForResponse(response =>
    response.url().includes('/api/v1/reports/records?') && response.url().includes('collegeLevel=true') &&
    response.url().includes('weekId=b0000000-0000-0000-0000-000000000003'))
  await collegeRow.getByRole('button', { name: '查看已确认记录' }).click()
  const collegeRecordsResponse = await collegeRecordsResponsePromise
  check(collegeRecordsResponse.status() === 200,
    `College-level records endpoint returned ${collegeRecordsResponse.status()}: ${JSON.stringify(await collegeRecordsResponse.json().catch(() => ({})))}`)
  const collegeRecordPage = await collegeRecordsResponse.json()
  check(collegeRecordPage.total === 2 && collegeRecordPage.items.some(item => item.studentName === 'E2E student A') &&
    collegeRecordPage.items.some(item => item.studentName === 'E2E student B'),
  `College-level records endpoint returned unexpected records: ${JSON.stringify(collegeRecordPage)}`)
  await page.getByRole('dialog', { name: '校级 · 到课率 · 已确认记录' }).waitFor()
  await page.getByRole('row').filter({ hasText: 'E2E student B' }).waitFor()
  await page.getByRole('button', { name: 'Close', exact: true }).click()
  cases.push('部门与校级类型行均打开真实记录抽屉，并展示当前确认修订中的记录')

  const cachedSummaryFailure = async route => {
    if (route.request().url().includes('weekId=b0000000-0000-0000-0000-000000000003')) {
      return route.fulfill({
        status: 503,
        contentType: 'application/json',
        body: JSON.stringify({ code: 'SERVICE_UNAVAILABLE', message: 'synthetic unavailable' }),
      })
    }
    return route.continue()
  }
  await page.route('**/api/v1/reports/summary*', cachedSummaryFailure)
  const failedRefreshResponse = page.waitForResponse(response => {
    return response.status() === 503 && response.url().includes('/api/v1/reports/summary?') &&
      response.url().includes('weekId=b0000000-0000-0000-0000-000000000003')
  })
  await page.getByRole('button', { name: '刷新汇总', exact: true }).click()
  await failedRefreshResponse
  await page.getByText('周汇总刷新失败', { exact: true }).waitFor()
  await page.getByText('本次刷新失败，以下为最近一次成功加载的数据。请重试确认最新状态。', { exact: true }).waitFor()
  check(await attendanceRow.count() === 1, 'Last successful summary disappeared after a refresh failure')
  await page.unroute('**/api/v1/reports/summary*', cachedSummaryFailure)
  const recoveredRefreshResponse = page.waitForResponse(response => {
    return response.status() === 200 && response.url().includes('/api/v1/reports/summary?') &&
      response.url().includes('weekId=b0000000-0000-0000-0000-000000000003')
  })
  await page.getByRole('button', { name: '重试周汇总', exact: true }).click()
  await recoveredRefreshResponse
  await page.getByText('周汇总刷新失败', { exact: true }).waitFor({ state: 'hidden' })
  await waitForSummaryIdle()
  cases.push('同周刷新 503 时标注并保留最近成功数据；重试成功后清除陈旧数据告警')

  let forbiddenRequests = 0
  const revokedSummary = route => {
    forbiddenRequests += 1
    return route.fulfill({
      status: 403,
      contentType: 'application/json',
      body: JSON.stringify({ code: 'FORBIDDEN_SCOPE', message: '当前无权查看该范围' }),
    })
  }
  await page.route('**/api/v1/reports/summary*', revokedSummary)
  const forbiddenResponse = page.waitForResponse(response => {
    return response.status() === 403 && response.url().includes('/api/v1/reports/summary?') &&
      response.url().includes('weekId=b0000000-0000-0000-0000-000000000003')
  })
  await page.getByRole('button', { name: '刷新汇总', exact: true }).click()
  await forbiddenResponse
  await page.getByText('你的通报查看权限已变化，系统已清除本地汇总数据。请重新选择当前可查看的周期。', { exact: true }).waitFor()
  check(await page.getByText('部门与校级批次汇总', { exact: true }).count() === 0, 'Previously authorized summary remained visible after a 403')
  check(await attendanceRow.count() === 0, 'Previously authorized department row remained visible after a 403')
  check(forbiddenRequests === 1, `A 403 summary request was retried ${forbiddenRequests} times`)
  await page.unroute('**/api/v1/reports/summary*', revokedSummary)
  await waitUntilEnabled(periodSelector)
  const restoredAfterPermissionRefresh = page.waitForResponse(response => {
    return response.status() === 200 && response.url().includes('/api/v1/reports/summary?') &&
      response.url().includes('weekId=b0000000-0000-0000-0000-000000000003')
  })
  await selectWeek(1)
  await restoredAfterPermissionRefresh
  await page.getByText('部门与校级批次汇总', { exact: true }).waitFor()
  cases.push('汇总刷新遇到 403 时立即隐藏并清除旧数据、取消重试；重新选择当前可查看周期后恢复')

  let summaryFailures = 0
  const summaryFailure = route => {
    summaryFailures += 1
    return route.fulfill({
      status: 503,
      contentType: 'application/json',
      body: JSON.stringify({ code: 'SERVICE_UNAVAILABLE', message: 'synthetic unavailable' }),
    })
  }
  await page.route('**/api/v1/reports/summary*', summaryFailure)
  const secondSummaryResponse = page.waitForResponse(response => {
    return response.url().includes('/api/v1/reports/summary?') && response.url().includes('weekId=b0000000-0000-0000-0000-000000000004')
  })
  await Promise.all([secondSummaryResponse, selectWeek(2)])
  await page.getByText('周汇总加载失败', { exact: true }).waitFor({ timeout: 15000 })
  check(summaryFailures > 0, 'Summary failure route was not exercised')
  check(await attendanceRow.count() === 0, 'Summary from the previous week remained visible after period change')
  await page.unroute('**/api/v1/reports/summary*', summaryFailure)
  const recoveredSummaryResponse = page.waitForResponse(response => {
    return response.status() === 200 && response.url().includes('/api/v1/reports/summary?') &&
      response.url().includes('weekId=b0000000-0000-0000-0000-000000000004')
  })
  await page.getByRole('button', { name: '重试周汇总', exact: true }).click()
  await recoveredSummaryResponse
  await page.getByText('周汇总加载失败', { exact: true }).waitFor({ state: 'hidden' })
  check(summaryFailures >= 2, 'Summary error state did not retry after automatic request retries')
  cases.push('汇总 503 显示失败状态；手动重试后真实 API 成功并清除错误')

  let releaseDelayedFirstWeek
  let resolveDelayedFirstWeekRequest
  const delayedFirstWeekRequestSeen = new Promise(resolve => {
    resolveDelayedFirstWeekRequest = resolve
  })
  const delayedFirstWeekResponse = new Promise(resolve => {
    releaseDelayedFirstWeek = resolve
  })
  const delayedFirstWeekRoute = async route => {
    if (route.request().url().includes('weekId=b0000000-0000-0000-0000-000000000003')) {
      resolveDelayedFirstWeekRequest()
      await delayedFirstWeekResponse
    }
    await route.continue()
  }
  await page.route('**/api/v1/reports/summary*', delayedFirstWeekRoute)
  await selectWeek(1)
  await delayedFirstWeekRequestSeen
  const secondWeekResponse = page.waitForResponse(response => {
    return response.status() === 200 && response.url().includes('/api/v1/reports/summary?') &&
      response.url().includes('weekId=b0000000-0000-0000-0000-000000000004')
  })
  await selectWeek(2)
  await secondWeekResponse
  const secondWeekAttendanceRow = page.locator('tr[data-row-key="department:10000000-0000-0000-0000-000000000002:type:30000000-0000-0000-0000-000000000001"]')
  await secondWeekAttendanceRow.waitFor()
  let secondWeekCells = await secondWeekAttendanceRow.getByRole('cell').allInnerTexts()
  check(secondWeekCells[2] === '0' && secondWeekCells[3] === '0' && secondWeekCells[4] === '0',
    `The second week's own summary was not shown: ${secondWeekCells.join('|')}`)

  const delayedFirstWeekResponseReceived = page.waitForResponse(response => {
    return response.status() === 200 && response.url().includes('/api/v1/reports/summary?') &&
      response.url().includes('weekId=b0000000-0000-0000-0000-000000000003')
  })
  releaseDelayedFirstWeek()
  await delayedFirstWeekResponseReceived
  await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))))
  secondWeekCells = await secondWeekAttendanceRow.getByRole('cell').allInnerTexts()
  check(secondWeekCells[2] === '0' && secondWeekCells[3] === '0' && secondWeekCells[4] === '0',
    `Late first-week response replaced the second week's summary: ${secondWeekCells.join('|')}`)
  await page.unroute('**/api/v1/reports/summary*', delayedFirstWeekRoute)
  cases.push('第一周汇总响应延迟时切换到第二周；旧响应晚到不覆盖当前周汇总')

  await page.getByRole('button', { name: /账户菜单/ }).click()
  await page.getByRole('menuitem', { name: /退出登录/ }).click()
  await page.waitForURL('**/login')
  const loginResponsePromise = page.waitForResponse(response => response.url().includes('/api/v1/auth/login') && response.request().method() === 'POST')
  await page.getByRole('textbox', { name: '账号', exact: true }).fill('browser-admin')
  await page.getByLabel('密码', { exact: true }).fill(fixture.authPassword)
  await page.getByRole('button', { name: /登\s*录/ }).click()
  const loginResponse = await loginResponsePromise
  check(loginResponse.status() === 200, `Browser admin login failed before the submission flow: ${loginResponse.status()}`)
  const { accessToken } = await loginResponse.json()
  await page.getByRole('region', { name: '当前工作上下文' }).waitFor()

  const submissionResponse = await page.request.post(`${base}/api/v1/department-submissions`, {
    headers: { Authorization: `Bearer ${accessToken}` },
    data: {
      scope: {
        semesterId: 'b0000000-0000-0000-0000-000000000002',
        weekId: 'b0000000-0000-0000-0000-000000000003',
        departmentId: '10000000-0000-0000-0000-000000000002',
        inspectionTypeId: '30000000-0000-0000-0000-000000000001',
      },
      records: [{ id: fixture.reportingSubmissionRecordId, version: 1 }],
      idempotencyKey: 'weekly-report-summary-browser-flow',
    },
  })
  check(submissionResponse.status() === 201, `Synthetic department submission failed: ${submissionResponse.status()} ${await submissionResponse.text()}`)
  const submittedBatch = await submissionResponse.json()
  const confirmationResponse = await page.request.post(`${base}/api/v1/department-submissions/${submittedBatch.id}/confirm`, {
    headers: { Authorization: `Bearer ${accessToken}` },
    data: { version: submittedBatch.rowVersion },
  })
  check(confirmationResponse.status() === 200, `Synthetic department confirmation failed: ${confirmationResponse.status()} ${await confirmationResponse.text()}`)

  await page.goto(base + '/reports/weekly-summary')
  await page.getByRole('heading', { name: '数据汇总与统计', exact: true }).waitFor()
  const submittedWeekSummary = page.waitForResponse(response => {
    return response.status() === 200 && response.url().includes('/api/v1/reports/summary?') &&
      response.url().includes('weekId=b0000000-0000-0000-0000-000000000003')
  })
  await submittedWeekSummary
  const updatedAttendanceRow = page.locator('tr[data-row-key="department:10000000-0000-0000-0000-000000000002:type:30000000-0000-0000-0000-000000000001"]')
  await updatedAttendanceRow.waitFor()
  const updatedCells = await updatedAttendanceRow.getByRole('cell').allInnerTexts()
  check(updatedCells[2] === '2' && updatedCells[3] === '2' && updatedCells[4] === '3',
    `The summary page did not include the newly submitted and confirmed record: ${updatedCells.join('|')}`)
  cases.push('真实提交并确认一个部门批次后重载汇总页；页面显示新增确认批次和当前修订记录数')

  await page.screenshot({ path: '__OUTPUT__/weekly-report-summary.png', fullPage: true })
  check(browserErrors.length === 0, `Uncaught browser errors: ${browserErrors.join('; ')}`)
  return { cases, browserErrors }
}
