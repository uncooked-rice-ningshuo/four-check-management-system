async (page) => {
  const fixture = __FIXTURE__
  const base = page.url().split('/').slice(0, 3).join('/')
  const cases = []
  const browserErrors = []
  page.on('pageerror', error => browserErrors.push(error.message))
  const check = (condition, message) => { if (!condition) throw new Error(message) }
  const login = async loginName => {
    const responsePromise = page.waitForResponse(response => response.url().includes('/api/v1/auth/login') && response.request().method() === 'POST')
    await page.getByRole('textbox', { name: '账号', exact: true }).fill(loginName)
    await page.getByLabel('密码', { exact: true }).fill(fixture.authPassword)
    await page.getByRole('button', { name: /登\s*录/ }).click()
    const response = await responsePromise
    check(response.status() === 200, `Synthetic account login failed for ${loginName}: ${response.status()}`)
    await page.getByRole('group', { name: '当前工作上下文' }).waitFor()
    return (await response.json()).accessToken
  }
  const logout = async () => {
    await page.getByRole('button', { name: /账户菜单/ }).click()
    await page.getByRole('menuitem', { name: /退出登录/ }).click()
    await page.waitForURL('**/login')
  }
  const chooseOption = async (label, optionName) => {
    const combobox = page.getByRole('combobox', { name: label })
    await combobox.click()
    if (label === '权限动作') {
      await combobox.fill(optionName)
      await combobox.press('Enter')
      return
    }
    const option = page.locator('.ant-select-dropdown:visible .ant-select-item-option')
      .filter({ hasText: optionName })
    await option.waitFor()
    check(await option.count() === 1, `Expected one ${optionName} option in ${label}`)
    await option.click()
  }
  const reportPath = `${base}/reports/weekly-summary`
  const attendancePath = `${base}/inspections/attendance?semesterId=${fixture.weeklyReport.semesterId}` +
    `&weekId=${fixture.weeklyReport.weekId}&departmentId=${fixture.weeklyReport.departmentId}`

  // The browser harness starts logged in as its disposable system administrator.
  await page.goto(`${base}/admin/users`)
  await page.getByRole('heading', { name: '用户与角色', exact: true }).waitFor()
  await page.getByPlaceholder('输入账号、学号或姓名…').fill(fixture.weeklyReport.manager)
  await page.getByRole('button', { name: /查.*询/ }).click()
  const managerRow = page.getByRole('row').filter({ hasText: fixture.weeklyReport.manager })
  await managerRow.waitFor()
  await managerRow.getByRole('button', { name: '权限详情' }).click()
  await page.getByText('浏览器逐周应交管理员 · 生效权限', { exact: true }).waitFor()
  await page.getByRole('button', { name: '新增权限规则', exact: true }).click()
  await chooseOption('权限动作', '管理逐周应交范围')
  await page.getByRole('textbox', { name: '授权说明' }).fill('隔离浏览器验收：配置本周应交范围')
  const permissionResponsePromise = page.waitForResponse(response =>
    response.url().includes('/permission-rules') && response.request().method() === 'POST')
  await page.getByRole('dialog', { name: '新增权限规则' }).getByRole('button', { name: /确.*定/ }).click()
  const permissionResponse = await permissionResponsePromise
  const permissionBody = await permissionResponse.json()
  check(permissionResponse.status() === 201 && permissionBody.action === 'REPORT_OBLIGATION_MANAGE' &&
    permissionBody.effect === 'ALLOW', `Office minister obligation permission grant failed: ${permissionResponse.status()} ${JSON.stringify(permissionBody)}`)
  await page.getByRole('row').filter({ hasText: '管理逐周应交范围' }).waitFor()
  await page.getByRole('dialog', { name: '浏览器逐周应交管理员 · 生效权限' })
    .getByRole('button', { name: 'Close', exact: true }).click()
  cases.push('管理员通过权限详情单独授予办公室部长逐周应交管理权限')

  await logout()
  const managerToken = await login(fixture.weeklyReport.manager)
  await page.goto(reportPath)
  await page.getByRole('heading', { name: '通报工作台', exact: true }).waitFor()
  await page.getByText('该周应交范围配置', { exact: true }).waitFor()
  await chooseOption('应交检查类型', '到课率')
  await chooseOption('应交部门或校级范围', '学习部')
  const obligationResponsePromise = page.waitForResponse(response =>
    response.url().endsWith('/api/v1/reports/obligations') && response.request().method() === 'POST')
  await page.getByRole('button', { name: '添加应交项', exact: true }).click()
  const obligationResponse = await obligationResponsePromise
  const obligationBody = await obligationResponse.json()
  check(obligationResponse.status() === 201 && obligationBody.departmentId === fixture.weeklyReport.departmentId &&
    obligationBody.inspectionTypeId === fixture.weeklyReport.inspectionTypeId && obligationBody.eligible,
  `Weekly obligation creation failed: ${obligationResponse.status()} ${JSON.stringify(obligationBody)}`)
  await page.getByText('已添加本周应交项', { exact: true }).waitFor()
  await page.getByText('学习部', { exact: true }).last().waitFor()
  cases.push('办公室负责人从通报页为当前周配置学习部到课率应交项')

  await logout()
  const submitterToken = await login(fixture.weeklyReport.submitter)
  await page.goto(attendancePath)
  const zeroButton = page.getByRole('button', { name: '提交零记录回执', exact: true })
  await zeroButton.waitFor()
  await zeroButton.click()
  const submitDialog = page.getByRole('dialog', { name: '确认提交零记录回执？' })
  await submitDialog.waitFor()
  await submitDialog.getByText(/已检查、确认零条/).waitFor()
  const submitAction = submitDialog.getByRole('button', { name: '确认提交', exact: true })
  const submitResponsePromiseAfterDialog = page.waitForResponse(response =>
    response.url().endsWith('/api/v1/department-submissions') && response.request().method() === 'POST')
  await submitAction.click()
  const submitResponse = await submitResponsePromiseAfterDialog
  const submissionBody = await submitResponse.json()
  check(submitResponse.status() === 201 && submissionBody.kind === 'ZERO_RECORD' && submissionBody.status === 'PENDING_REVIEW' &&
    submissionBody.scope.weekId === fixture.weeklyReport.weekId && submissionBody.scope.departmentId === fixture.weeklyReport.departmentId &&
    submissionBody.recordIds.length === 0,
  `Browser zero receipt submission failed: ${submitResponse.status()} ${JSON.stringify(submissionBody)}`)
  await page.getByText('批次状态已更新', { exact: true }).waitFor()
  check(submissionBody.createdBy === (await (await page.request.get(`${base}/api/v1/auth/me`, { headers: { Authorization: `Bearer ${submitterToken}` } })).json()).id,
    'Zero receipt was not submitted by the logged-in member')
  cases.push('部门成员从到课率页显式提交空记录零回执，并进入待审核状态')

  await logout()
  const reviewerToken = await login(fixture.weeklyReport.reviewer)
  await page.goto(attendancePath)
  await page.getByRole('button', { name: '检查提交批次', exact: true }).click()
  const batchDrawer = page.getByRole('dialog', { name: '检查提交批次' })
  await batchDrawer.waitFor()
  const receiptRow = batchDrawer.getByRole('row').filter({ hasText: '确认零条' })
  await receiptRow.waitFor()
  const returnButton = receiptRow.getByRole('button', { name: '退回', exact: true })
  await returnButton.waitFor()
  await returnButton.click()
  const returnDialog = page.getByRole('dialog', { name: '退回提交批次' })
  await returnDialog.waitFor()
  await returnDialog.getByRole('textbox', { name: '退回原因' }).fill('请复核本周检查范围后重提')
  const returnResponsePromise = page.waitForResponse(response =>
    response.url().endsWith(`/api/v1/department-submissions/${submissionBody.id}/return`) && response.request().method() === 'POST')
  await returnDialog.getByRole('button', { name: '确认退回', exact: true }).click()
  const returnResponse = await returnResponsePromise
  const returnedBody = await returnResponse.json()
  check(returnResponse.status() === 200 && returnedBody.id === submissionBody.id && returnedBody.status === 'RETURNED' &&
    returnedBody.kind === 'ZERO_RECORD' && returnedBody.revisionNo === 1 && returnedBody.returnReason === '请复核本周检查范围后重提' &&
    returnedBody.recordIds.length === 0,
  `Reviewer return failed: ${returnResponse.status()} ${JSON.stringify(returnedBody)}`)
  await page.getByText('批次状态已更新', { exact: true }).waitFor()
  await batchDrawer.getByRole('button', { name: '关闭', exact: true }).click()
  await batchDrawer.waitFor({ state: 'hidden' })
  cases.push('部门负责人填写原因退回零回执，保留原批次和修订')

  await logout()
  await login(fixture.weeklyReport.submitter)
  await page.goto(attendancePath)
  await page.getByRole('button', { name: '检查提交批次', exact: true }).click()
  const returnedDrawer = page.getByRole('dialog', { name: '检查提交批次' })
  await returnedDrawer.waitFor()
  const returnedRow = returnedDrawer.getByRole('row').filter({ hasText: '确认零条' })
  await returnedRow.waitFor()
  const resubmitButton = returnedRow.getByRole('button', { name: '重提', exact: true })
  await resubmitButton.waitFor()
  await resubmitButton.click()
  const resubmitDialog = page.getByRole('dialog', { name: '重新提交该批次？' })
  await resubmitDialog.waitFor()
  const resubmitResponsePromise = page.waitForResponse(response =>
    response.url().endsWith(`/api/v1/department-submissions/${submissionBody.id}/resubmit`) && response.request().method() === 'POST')
  await resubmitDialog.getByRole('button', { name: '重新提交', exact: true }).click()
  const resubmitResponse = await resubmitResponsePromise
  const resubmittedBody = await resubmitResponse.json()
  const resubmitRequest = resubmitResponse.request().postDataJSON()
  check(resubmitResponse.status() === 200 && resubmittedBody.id === submissionBody.id && resubmittedBody.kind === 'ZERO_RECORD' &&
    resubmittedBody.status === 'PENDING_REVIEW' && resubmittedBody.revisionNo === 2 && resubmittedBody.recordIds.length === 0 &&
    resubmitRequest.kind === 'ZERO_RECORD' && Array.isArray(resubmitRequest.records) && resubmitRequest.records.length === 0,
  `Member zero receipt resubmission failed: ${resubmitResponse.status()} ${JSON.stringify(resubmittedBody)}`)
  await page.getByText('批次状态已更新', { exact: true }).waitFor()
  await returnedDrawer.getByRole('button', { name: '关闭', exact: true }).click()
  await returnedDrawer.waitFor({ state: 'hidden' })
  cases.push('部门成员原批次零记录重提，修订递增并重新进入审核')

  await logout()
  await login(fixture.weeklyReport.reviewer)
  await page.goto(attendancePath)
  await page.getByRole('button', { name: '检查提交批次', exact: true }).click()
  const confirmationDrawer = page.getByRole('dialog', { name: '检查提交批次' })
  await confirmationDrawer.waitFor()
  const pendingReceiptRow = confirmationDrawer.getByRole('row').filter({ hasText: '确认零条' })
  await pendingReceiptRow.waitFor()
  const confirmButton = pendingReceiptRow.getByRole('button', { name: '确认', exact: true })
  await confirmButton.waitFor()
  const confirmResponsePromise = page.waitForResponse(response =>
    response.url().endsWith(`/api/v1/department-submissions/${submissionBody.id}/confirm`) && response.request().method() === 'POST')
  await confirmButton.click()
  const confirmDialog = page.getByRole('dialog', { name: '确认该批次？' })
  await confirmDialog.waitFor()
  await confirmDialog.getByRole('button', { name: '确认批次', exact: true }).click()
  const confirmResponse = await confirmResponsePromise
  const confirmedBody = await confirmResponse.json()
  check(confirmResponse.status() === 200 && confirmedBody.id === submissionBody.id && confirmedBody.status === 'CONFIRMED' &&
    confirmedBody.kind === 'ZERO_RECORD' && confirmedBody.revisionNo === 2 && confirmedBody.recordIds.length === 0,
  `Reviewer confirmation after resubmission failed: ${confirmResponse.status()} ${JSON.stringify(confirmedBody)}`)
  await page.getByText('批次状态已更新', { exact: true }).waitFor()
  await confirmationDrawer.getByRole('button', { name: '关闭', exact: true }).click()
  await confirmationDrawer.waitFor({ state: 'hidden' })
  cases.push('部门负责人审核重提版本并确认；零回执仍不包含记录')

  await logout()
  await login(fixture.weeklyReport.manager)
  await page.goto(reportPath)
  await page.getByRole('heading', { name: '通报工作台', exact: true }).waitFor()
  await page.getByText('零回执已确认 1', { exact: true }).waitFor()
  await page.getByText('应交', { exact: true }).first().waitFor()
  cases.push('通报汇总页显示已确认零回执，不把它计成实际记录')

  check(browserErrors.length === 0, `Uncaught browser errors: ${browserErrors.join('; ')}`)
  return { cases, browserErrors, browserVersion: await page.context().browser().version() }
}
