async (page) => {
  const fixture = __FIXTURE__
  const base = page.url().split('/').slice(0, 3).join('/')
  const cases = []
  const reportRequests = []
  const browserErrors = []
  const messageContextWarnings = []
  page.on('pageerror', error => browserErrors.push(error.message))
  page.on('console', message => {
    if (message.type() === 'error' && message.text().includes('Static function can not consume context')) messageContextWarnings.push(message.text())
  })
  page.on('request', request => {
    if (request.url().includes('/api/v1/reports/')) reportRequests.push(request.url())
  })
  const check = (condition, message) => { if (!condition) throw new Error(message) }
  const logout = async () => {
    await page.getByRole('button', { name: /账户菜单/ }).click()
    await page.getByRole('menuitem', { name: /退出登录/ }).click()
    await page.waitForURL('**/login')
  }
  const login = async loginName => {
    const responsePromise = page.waitForResponse(response => response.url().includes('/api/v1/auth/login') && response.request().method() === 'POST')
    await page.getByRole('textbox', { name: '账号', exact: true }).fill(loginName)
    await page.getByLabel('密码', { exact: true }).fill(fixture.authPassword)
    await page.getByRole('button', { name: /登\s*录/ }).click()
    const response = await responsePromise
    check(response.status() === 200, `Synthetic account login failed for ${loginName}: ${response.status()}`)
    await page.getByRole('region', { name: '当前工作上下文' }).waitFor()
    return (await response.json()).accessToken
  }
  const openReportFromNavigation = async () => {
    await page.getByRole('menuitem', { name: /通报管理/ }).click()
    const reportLink = page.getByRole('link', { name: '数据汇总与统计', exact: true })
    await reportLink.waitFor()
    await reportLink.click()
    await page.getByRole('heading', { name: '数据汇总与统计', exact: true }).waitFor()
  }
  const reportPath = `${base}/reports/weekly-summary`

  await page.goto(`${base}/admin/users`)
  await page.getByRole('heading', { name: '用户与角色', exact: true }).waitFor()
  await page.getByPlaceholder('输入账号、学号或姓名…').fill(fixture.reportingRoleUsers.scopedAdmin)
  await page.getByRole('button', { name: /查.*询/ }).click()
  const scopedAdminUserRow = page.getByRole('row').filter({ hasText: fixture.reportingRoleUsers.scopedAdmin })
  await scopedAdminUserRow.waitFor()
  await scopedAdminUserRow.getByRole('button', { name: '权限详情' }).click()
  await page.getByText('浏览器指定通报管理员 · 生效权限', { exact: true }).waitFor()
  await page.getByRole('button', { name: '新增权限规则', exact: true }).click()

  const chooseOption = async (label, optionName) => {
    const combobox = page.getByRole('combobox', { name: label })
    await combobox.click()
    if (label === '权限动作') {
      await combobox.fill(optionName)
      await combobox.press('Enter')
      return
    }
    for (let attempt = 0; attempt < 40; attempt += 1) {
      await combobox.press('ArrowDown')
      const activeId = await combobox.getAttribute('aria-activedescendant')
      if (!activeId) continue
      const activeOption = page.locator(`#${activeId}`)
      const activeLabel = await activeOption.getAttribute('aria-label')
      const activeText = await activeOption.innerText()
      if (activeLabel === optionName || activeText === optionName) {
        await combobox.press('Enter')
        return
      }
    }
    throw new Error(`Could not select ${optionName} from ${label}`)
  }
  await chooseOption('权限动作', '查看通报汇总')
  await chooseOption('学年', '2026-2027 学年')
  await chooseOption('学期', '浏览器回归学期')
  await chooseOption('周次', '第 1 周')
  await chooseOption('检查类型', '到课率')
  await page.getByText('高级数据范围（部门、班级、年级）', { exact: true }).click()
  await chooseOption('部门范围', '学习部')
  await page.getByRole('textbox', { name: '授权说明' }).fill('隔离浏览器验收：配置本周到课率通报范围')

  const createdPermissionRuleResponse = page.waitForResponse(response => {
    return response.url().includes(`/api/v1/admin/users/`) && response.url().includes('/permission-rules') &&
      response.request().method() === 'POST'
  })
  await page.getByRole('dialog').getByRole('button', { name: /确.*定/ }).click()
  const createdPermissionRule = await createdPermissionRuleResponse
  check(createdPermissionRule.status() === 201, `Creating REPORT_VIEW through permission UI returned ${createdPermissionRule.status()}`)
  const createdPermission = await createdPermissionRule.json()
  check(createdPermission.action === 'REPORT_VIEW' && createdPermission.scope.weekId === 'b0000000-0000-0000-0000-000000000003' &&
    createdPermission.scope.inspectionTypeId === '30000000-0000-0000-0000-000000000001' &&
    createdPermission.scope.departmentId === '10000000-0000-0000-0000-000000000002',
  `Permission UI saved an unexpected report scope: ${JSON.stringify(createdPermission.scope)}`)
  await page.getByText(/部门：学习部；检查类型：到课率/).waitFor()
  cases.push('系统管理员通过用户权限详情页面新增 REPORT_VIEW，并保存学年、学期、周次、类型和部门范围')
  await page.getByText('权限规则已保存', { exact: true }).waitFor({ state: 'hidden' })

  await page.getByRole('button', { name: '新增权限规则', exact: true }).click()
  await chooseOption('权限动作', '强制生成未完成通报')
  await page.getByRole('textbox', { name: '授权说明' }).fill('隔离浏览器验收：错误目标账号不得获授强制生成权限')
  const rejectedForcePermissionResponse = page.waitForResponse(response => {
    return response.url().includes(`/api/v1/admin/users/`) && response.url().includes('/permission-rules') &&
      response.request().method() === 'POST'
  })
  await page.getByRole('dialog').getByRole('button', { name: /确.*定/ }).click()
  const rejectedForcePermission = await rejectedForcePermissionResponse
  const rejectedForcePermissionBody = await rejectedForcePermission.json()
  check(rejectedForcePermission.status() === 400 && rejectedForcePermissionBody.code === 'REPORT_FORCE_GENERATE_ROLE_REQUIRED',
    `Ineligible account received force generation permission: ${rejectedForcePermission.status()} ${rejectedForcePermissionBody.code}`)
  await page.getByText('该权限仅可单独授予学生会办公室部长或副部长', { exact: true }).waitFor()
  cases.push('权限详情页面可选择强制生成权限，但服务端拒绝将其授予非办公室负责人账号')
  await page.getByText('该权限仅可单独授予学生会办公室部长或副部长', { exact: true }).waitFor({ state: 'hidden' })
  await page.getByRole('dialog', { name: '新增权限规则' }).getByRole('button', { name: '取 消' }).click()
  await page.getByRole('dialog', { name: '新增权限规则' }).waitFor({ state: 'hidden' })

  await page.getByRole('dialog', { name: '浏览器指定通报管理员 · 生效权限' })
    .getByRole('button', { name: 'Close', exact: true }).click()
  await page.getByText('浏览器指定通报管理员 · 生效权限', { exact: true }).waitFor({ state: 'hidden' })

  await page.getByPlaceholder('输入账号、学号或姓名…').fill(fixture.reportingRoleUsers.viceMinister)
  await page.getByRole('button', { name: /查.*询/ }).click()
  const officeViceMinisterRow = page.getByRole('row').filter({ hasText: fixture.reportingRoleUsers.viceMinister })
  await officeViceMinisterRow.waitFor()
  await officeViceMinisterRow.getByRole('button', { name: '权限详情' }).click()
  await page.getByText('浏览器办公室副部长 · 生效权限', { exact: true }).waitFor()
  await page.getByRole('button', { name: '新增权限规则', exact: true }).click()
  await chooseOption('权限动作', '强制生成未完成通报')
  await page.getByRole('textbox', { name: '授权说明' }).fill('隔离浏览器验收：指定通报副部长的单独强制生成权限')
  const grantedForcePermissionResponse = page.waitForResponse(response => {
    return response.url().includes(`/api/v1/admin/users/`) && response.url().includes('/permission-rules') &&
      response.request().method() === 'POST'
  })
  await page.getByRole('dialog').getByRole('button', { name: /确.*定/ }).click()
  const grantedForcePermission = await grantedForcePermissionResponse
  check(grantedForcePermission.status() === 201, `Eligible office deputy permission grant returned ${grantedForcePermission.status()}`)
  const grantedForcePermissionBody = await grantedForcePermission.json()
  check(grantedForcePermissionBody.action === 'REPORT_FORCE_GENERATE' && grantedForcePermissionBody.effect === 'ALLOW',
    `Office deputy received an unexpected permission: ${JSON.stringify(grantedForcePermissionBody)}`)
  const forceScope = grantedForcePermissionBody.scope ?? {}
  check(!forceScope.departmentId && !forceScope.classId && !forceScope.grade,
    `Force generation permission must apply to the weekly report instead of a department subset: ${JSON.stringify(forceScope)}`)
  await page.getByRole('table').getByText('强制生成未完成通报', { exact: true }).waitFor()
  cases.push('系统管理员可在权限详情中单独授予学生会办公室副部长强制生成权限')
  await page.getByText('权限规则已保存', { exact: true }).waitFor({ state: 'hidden' })
  await page.getByRole('dialog', { name: '浏览器办公室副部长 · 生效权限' })
    .getByRole('button', { name: 'Close', exact: true }).click()
  await page.getByText('浏览器办公室副部长 · 生效权限', { exact: true }).waitFor({ state: 'hidden' })

  await logout()
  for (const loginName of [fixture.reportingRoleUsers.minister, fixture.reportingRoleUsers.viceMinister]) {
    const accessToken = await login(loginName)
    const periodResponse = await page.request.get(`${base}/api/v1/reports/periods`, {
      headers: { Authorization: `Bearer ${accessToken}` },
    })
    check(periodResponse.status() === 200, `${loginName} should read report periods`)
    await openReportFromNavigation()
    await page.getByText(/第 1 周 ·/).first().waitFor()
    const departmentRow = page.locator('tr[data-row-key="department:10000000-0000-0000-0000-000000000002:type:30000000-0000-0000-0000-000000000001"]')
    await departmentRow.waitFor()
    const departmentCells = await departmentRow.getByRole('cell').allInnerTexts()
    const collegeRow = page.locator('tr[data-row-key="college:type:30000000-0000-0000-0000-000000000001"]')
    await collegeRow.waitFor()
    const collegeCells = await collegeRow.getByRole('cell').allInnerTexts()
    check(departmentCells[2] === '2' && departmentCells[6] === '2' && departmentCells[7] === '3', `${loginName} saw an unexpected department summary: ${departmentCells.join('|')}`)
    check(collegeCells[2] === '1' && collegeCells[6] === '1' && collegeCells[7] === '2', `${loginName} saw an unexpected college summary: ${collegeCells.join('|')}`)
    await logout()
  }
  cases.push('办公室部长和副部长均能从页面入口查看部门与校级周汇总')

  const scopedToken = await login(fixture.reportingRoleUsers.scopedAdmin)
  const scopedPeriods = await page.request.get(`${base}/api/v1/reports/periods`, {
    headers: { Authorization: `Bearer ${scopedToken}` },
  })
  const scopedPeriodPage = await scopedPeriods.json()
  check(scopedPeriods.status() === 200 && scopedPeriodPage.items.length === 1, 'Scoped report admin should see only the granted week')
  await openReportFromNavigation()
  await page.getByText(/第 1 周 ·/).first().waitFor()
  const scopedRows = page.locator('tr[data-row-key="department:10000000-0000-0000-0000-000000000002:type:30000000-0000-0000-0000-000000000001"]')
  await scopedRows.waitFor()
  const scopedCells = await scopedRows.getByRole('cell').allInnerTexts()
  check(scopedCells[2] === '2' && scopedCells[6] === '2' && scopedCells[7] === '3', `Scoped report admin saw an unexpected department summary: ${scopedCells.join('|')}`)
  check(await page.getByRole('row').filter({ hasText: '校级独立批次' }).count() === 0, 'Scoped report admin saw a college-level batch outside the department grant')
  await logout()
  cases.push('指定通报管理员页面仅展示其授权周次、类型与部门范围')

  const ordinaryToken = await login(fixture.reportingRoleUsers.member)
  const requestCountBeforeDeniedNavigation = reportRequests.length
  check(await page.getByRole('link', { name: '数据汇总与统计', exact: true }).count() === 0, 'Ordinary member saw the reporting navigation item')
  await page.goto(reportPath)
  await page.getByText('403', { exact: true }).waitFor()
  await page.getByText('无权访问该业务', { exact: true }).waitFor()
  check(reportRequests.length === requestCountBeforeDeniedNavigation, 'Unauthorized route requested report data before denying access')
  for (const path of [
    '/api/v1/reports/periods',
    '/api/v1/reports/summary?semesterId=b0000000-0000-0000-0000-000000000002&weekId=b0000000-0000-0000-0000-000000000003',
  ]) {
    const response = await page.request.get(`${base}${path}`, { headers: { Authorization: `Bearer ${ordinaryToken}` } })
    check(response.status() === 403, `Unauthorized API request ${path} returned ${response.status()}`)
    check((await response.json()).code === 'FORBIDDEN_SCOPE', `Unauthorized API request ${path} did not return FORBIDDEN_SCOPE`)
  }
  cases.push('普通成员侧栏不显示通报入口，直达路由与两个 API 端点均返回拒绝')
  await logout()

  const presidiumToken = await login(fixture.reportingRoleUsers.presidium)
  const requestCountBeforePresidiumDeniedNavigation = reportRequests.length
  check(await page.getByRole('link', { name: '数据汇总与统计', exact: true }).count() === 0,
    'Presidium membership alone must not show the reporting navigation item')
  await page.goto(reportPath)
  await page.getByText('403', { exact: true }).waitFor()
  await page.getByText('无权访问该业务', { exact: true }).waitFor()
  check(reportRequests.length === requestCountBeforePresidiumDeniedNavigation,
    'Presidium route requested report data before denying access')
  for (const path of [
    '/api/v1/reports/periods',
    '/api/v1/reports/summary?semesterId=b0000000-0000-0000-0000-000000000002&weekId=b0000000-0000-0000-0000-000000000003',
  ]) {
    const response = await page.request.get(`${base}${path}`, { headers: { Authorization: `Bearer ${presidiumToken}` } })
    check(response.status() === 403, `Presidium request ${path} returned ${response.status()}`)
    check((await response.json()).code === 'FORBIDDEN_SCOPE', `Presidium request ${path} did not return FORBIDDEN_SCOPE`)
  }
  cases.push('主席团身份本身不显示通报入口，直达页面与两个 API 均拒绝')

  check(browserErrors.length === 0, `Uncaught browser errors: ${browserErrors.join('; ')}`)
  check(messageContextWarnings.length === 0, `Ant Design static message context warnings: ${messageContextWarnings.length}`)
  return { cases, browserErrors }
}
