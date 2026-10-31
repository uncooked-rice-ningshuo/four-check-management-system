async (page) => {
  const fixture = __FIXTURE__
  const base = page.url().split('/').slice(0, 3).join('/')
  const cases = []
  const browserErrors = []
  page.on('pageerror', error => browserErrors.push(error.message))
  const check = (ok, message) => { if (!ok) throw new Error(message) }
  const select = async (scope, label, text) => {
    const input = scope.getByRole('combobox', { name: new RegExp(label + '\\s*[:：]?$') })
    await input.click()
    await page.locator('.ant-select-dropdown:visible .ant-select-item-option').filter({ hasText: text }).first().click()
  }
  const logout = async () => {
    await page.getByRole('button', { name: /账户菜单/ }).click()
    await page.getByRole('menuitem', { name: /退出登录/ }).click()
    await page.waitForURL('**/login')
  }
  const login = async loginName => {
    await page.getByRole('textbox', { name: '账号', exact: true }).fill(loginName)
    await page.getByLabel('密码', { exact: true }).fill(fixture.authPassword)
    await page.getByRole('button', { name: /登\s*录/ }).click()
    await page.getByRole('group', { name: '当前工作上下文' }).waitFor()
    const response = await page.request.post(`${base}/api/v1/auth/login`, {
      data: { loginName, password: fixture.authPassword },
    })
    check(response.status() === 200, `Synthetic account token request failed for ${loginName}`)
    return (await response.json()).accessToken
  }
  const api = async (token, path, method = 'GET', data, expected = 200, code) => {
    const response = await page.request.fetch(`${base}/api/v1${path}`, {
      method,
      data,
      headers: token ? { Authorization: `Bearer ${token}` } : {},
    })
    check(response.status() === expected, `${method} ${path.split('?')[0]} expected ${expected}, got ${response.status()}`)
    if (code) check((await response.json()).code === code, `Unexpected error code from ${path.split('?')[0]}`)
    return response
  }

  await logout()
  const entryToken = await login(fixture.eveningActors.entry)
  await page.goto(`${base}/inspections/evening-self-study`)
  await page.getByRole('heading', { name: '晚自习检查', exact: true }).waitFor()
  await page.getByText('只记录缺勤学生；节次固定为第 9–10 节、2 学时，不关联课程。', { exact: true }).waitFor()
  await page.getByRole('button', { name: '新增缺勤记录', exact: true }).click()
  const create = page.getByRole('dialog', { name: '新增晚自习缺勤记录' })
  await select(create, '录入学期', '浏览器回归学期')
  await select(create, '录入周次', fixture.weekStart)
  await create.getByLabel('录入归属部门').waitFor()
  await select(create, '学生班级', 'E2E class')
  await select(create, '缺勤学生', 'E2E student A')
  await select(create, '晚自习教室', '101')
  await create.getByLabel('缺勤日期').fill(fixture.date)
  await create.getByLabel('缺勤日期').press('Enter')
  const time = create.getByLabel('检查时间')
  await time.fill('20:15:00')
  await time.press('Enter')
  await create.getByLabel('备注').fill('晚自习专属浏览器验收')
  check(await create.getByLabel('课程').count() === 0, 'Evening record drawer must not include a course field')
  await create.getByText('第 9–10 节', { exact: true }).waitFor()
  await create.getByText('2 学时（由系统固定）', { exact: true }).waitFor()
  const createResponsePromise = page.waitForResponse(response =>
    response.url().includes('/api/v1/inspections') && response.request().method() === 'POST')
  await create.getByRole('button', { name: '保存草稿' }).click()
  const createResponse = await createResponsePromise
  check(createResponse.status() === 201, `Evening record creation returned ${createResponse.status()}`)
  const created = await createResponse.json()
  check(created.details.period === '9-10' && created.details.absenceHours === 2,
    `Server did not derive the fixed evening period and absence hours: ${JSON.stringify(created.details)}`)
  check(created.departmentId === fixture.eveningActors.departmentId, 'Record did not inherit the entry account department')
  await create.waitFor({ state: 'hidden' })
  const row = page.getByRole('row').filter({ has: page.getByRole('cell', { name: 'E2E student A', exact: true }) })
  await row.waitFor()
  await page.getByRole('button', { name: '提交记录' }).click()
  const submission = page.getByRole('dialog', { name: '提交检查记录' })
  await select(submission, '提交学期', '浏览器回归学期')
  await select(submission, '提交周次', fixture.weekStart)
  await select(submission, '提交归属', '学习部')
  await submission.getByRole('button', { name: '下一步：添加记录' }).click()
  await submission.getByRole('row').filter({ hasText: 'E2E student A' }).getByRole('checkbox').check()
  await submission.getByRole('button', { name: '下一步：核对清单' }).click()
  await submission.getByRole('button', { name: '提交审核' }).click()
  const confirmation = page.getByRole('dialog', { name: '最终确认本次提交' })
  const batchResponsePromise = page.waitForResponse(response =>
    response.url().endsWith('/api/v1/department-submissions') && response.request().method() === 'POST')
  await confirmation.getByRole('button', { name: '确认提交' }).click()
  const batchResponse = await batchResponsePromise
  check(batchResponse.status() === 201, `Evening submission returned ${batchResponse.status()}`)
  const batch = await batchResponse.json()
  await confirmation.waitFor({ state: 'hidden' })
  await row.getByText('待审核').waitFor()
  await api(entryToken, `/department-submissions/${batch.id}/confirm`, 'POST', { version: batch.rowVersion }, 403, 'FORBIDDEN_SCOPE')
  cases.push('有权部门成员通过晚自习页面录入；服务端固定 9–10 节、2 学时且不关联课程；真实提交批次进入待审核')

  await logout()
  const reviewerToken = await login(fixture.eveningActors.reviewer)
  await page.goto(`${base}/inspections/evening-self-study`)
  await page.getByRole('heading', { name: '晚自习检查', exact: true }).waitFor()
  check(await page.getByRole('button', { name: '新增缺勤记录', exact: true }).count() === 0,
    'Review-only account must not see the evening entry action')
  await page.getByRole('button', { name: '检查提交批次' }).click()
  const reviewDrawer = page.getByRole('dialog', { name: '检查提交批次' })
  const reviewBatchRow = reviewDrawer.getByRole('row').filter({ hasText: '浏览器回归学期' })
  await reviewBatchRow.waitFor()
  const confirmResponsePromise = page.waitForResponse(response =>
    response.url().includes(`/api/v1/department-submissions/${batch.id}/confirm`) && response.request().method() === 'POST')
  await reviewBatchRow.getByRole('button', { name: '确认', exact: true }).click()
  const confirmBatch = page.getByRole('dialog', { name: '确认该批次？' })
  await confirmBatch.getByRole('button', { name: '确认批次' }).click()
  const confirmResponse = await confirmResponsePromise
  check(confirmResponse.status() === 200, `Department review returned ${confirmResponse.status()}`)
  const approvedBatch = await confirmResponse.json()
  await confirmBatch.waitFor({ state: 'hidden' })
  await reviewDrawer.getByText('部门已确认').waitFor()
  await api(reviewerToken, `/department-submissions/${batch.id}/confirm`, 'POST', { version: approvedBatch.rowVersion }, 409, 'INVALID_SUBMISSION_STATE')
  await reviewDrawer.getByRole('button', { name: '关闭' }).click()
  await reviewDrawer.waitFor({ state: 'hidden' })
  cases.push('部门审核角色在真实页面确认晚自习批次；成员账号不能替代审核人确认，审核完成后批次不可回退')

  await api(reviewerToken, '/inspections', 'POST', {
    semesterId: fixture.semesterId,
    weekId: fixture.weekId,
    departmentId: fixture.eveningActors.departmentId,
    inspectionTypeId: fixture.eveningActors.inspectionTypeId,
    details: { classId: fixture.class, studentId: fixture.studentB, classroomId: fixture.room, date: fixture.date, time: '20:30:00' },
  }, 403, 'FORBIDDEN_SCOPE')
  await logout()
  const outsiderToken = await login(fixture.eveningActors.outsider)
  const outsiderList = await page.request.get(`${base}/api/v1/inspections?semesterId=${fixture.semesterId}&weekId=${fixture.weekId}&inspectionTypeId=${fixture.eveningActors.inspectionTypeId}&page=1&pageSize=20`, {
    headers: { Authorization: `Bearer ${outsiderToken}` },
  })
  check((outsiderList.status() === 403 && (await outsiderList.json()).code === 'FORBIDDEN_SCOPE') ||
    (outsiderList.status() === 200 && (await outsiderList.json()).total === 0),
  `Out-of-scope list returned visible data or an unexpected response: ${outsiderList.status()}`)
  await api(outsiderToken, '/inspections', 'POST', {
    semesterId: fixture.semesterId,
    weekId: fixture.weekId,
    departmentId: fixture.eveningActors.departmentId,
    inspectionTypeId: fixture.eveningActors.inspectionTypeId,
    details: { classId: fixture.class, studentId: fixture.studentB, classroomId: fixture.room, date: fixture.date, time: '20:30:00' },
  }, 403, 'FORBIDDEN_SCOPE')
  cases.push('无映射部门看不到已确认数据且不能伪造部门写入；审核角色没有录入权限')

  await logout()
  const adminToken = await login('browser-admin')
  const summaryResponse = await api(adminToken,
    `/reports/summary?semesterId=${fixture.semesterId}&weekId=${fixture.weekId}`)
  const summary = await summaryResponse.json()
  const eveningRow = summary.rows.find(item => item.departmentId === fixture.eveningActors.departmentId &&
    item.inspectionTypeId === fixture.eveningActors.inspectionTypeId)
  check(eveningRow && eveningRow.confirmedRecordCount === 1, 'Confirmed evening record is absent from the report count')
  const recordsResponse = await api(adminToken,
    `/reports/records?semesterId=${fixture.semesterId}&weekId=${fixture.weekId}&departmentId=${fixture.eveningActors.departmentId}&inspectionTypeId=${fixture.eveningActors.inspectionTypeId}&page=1&pageSize=20`)
  const records = await recordsResponse.json()
  check(records.total === 1 && records.items[0].studentName === 'E2E student A' &&
    records.items[0].typeCode === 'EVENING_SELF_STUDY' && records.items[0].details.period === '9-10' &&
    records.items[0].details.absenceHours === 2 && records.items[0].currentCourseName === '',
  `Report detail did not use the confirmed evening snapshot: ${JSON.stringify(records)}`)
  await page.goto(`${base}/reports/weekly-summary`)
  await page.getByRole('heading', { name: '通报工作台', exact: true }).waitFor()
  await page.getByRole('row').filter({ hasText: '晚自习' }).waitFor()
  cases.push('已确认晚自习记录进入周通报真实统计和明细 API，并在通报页面显示类型行')

  check(browserErrors.length === 0, `Uncaught browser errors: ${browserErrors.join('; ')}`)
  return { cases, browserErrors }
}
