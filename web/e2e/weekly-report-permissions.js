async (page) => {
  const fixture = __FIXTURE__
  const outputDir = __OUTPUT__
  const base = page.url().split('/').slice(0, 3).join('/')
  const cases = []
  const browserErrors = []
  page.on('pageerror', error => browserErrors.push(error.message))
  const check = (condition, message) => { if (!condition) throw new Error(message) }
  const semesterId = 'b0000000-0000-0000-0000-000000000002'
  const weekId = 'b0000000-0000-0000-0000-000000000003'
  const archivePath = `${base}/reports/archive`
  const login = async loginName => {
    const responsePromise = page.waitForResponse(response => response.url().endsWith('/api/v1/auth/login') && response.request().method() === 'POST')
    await page.getByRole('textbox', { name: '账号', exact: true }).fill(loginName)
    await page.getByLabel('密码', { exact: true }).fill(fixture.authPassword)
    await page.getByRole('button', { name: /登\s*录/ }).click()
    const response = await responsePromise
    check(response.status() === 200, `${loginName} login returned ${response.status()}`)
    await page.getByRole('group', { name: '当前工作上下文' }).waitFor()
    return (await response.json()).accessToken
  }
  const logout = async () => {
    await page.goto(base)
    await page.getByRole('button', { name: /账户菜单/ }).click()
    await page.getByRole('menuitem', { name: /退出登录/ }).click()
    await page.waitForURL('**/login')
  }
  await logout()
  let adminToken = await login('browser-admin')
  await page.goto(`${base}/admin/organization`)
  await page.getByRole('heading', { name: '部门与默认权限' }).waitFor()
  await page.screenshot({ path: `${outputDir}/department-defaults-desktop.png`, fullPage: true })
  await page.setViewportSize({ width: 614, height: 749 })
  await page.screenshot({ path: `${outputDir}/department-defaults-narrow.png`, fullPage: true })
  await page.getByRole('button', { name: '配置无机课堂角色权限' }).click()
  const secondTypeView = page.getByRole('switch', { name: '宿管部 无机课堂 委员 查看记录' })
  const secondTypeBefore = await secondTypeView.isChecked()
  check(await secondTypeView.isDisabled(), 'Unassigned inspection role action is editable')
  await page.getByRole('button', { name: 'Close', exact: true }).click()
  await page.getByRole('switch', { name: '宿管部 到课率 承担此检查' }).click()
  await page.getByRole('button', { name: '配置到课率部门功能' }).click()
  await page.getByRole('switch', { name: '宿管部 到课率 查看记录' }).click()
  await page.getByRole('button', { name: 'Close', exact: true }).click()
  await page.getByRole('button', { name: '配置到课率角色权限' }).click()
  const firstTypeView = page.getByRole('switch', { name: '宿管部 到课率 委员 查看记录' })
  check(await firstTypeView.isEnabled(), 'Assigned inspection action did not unlock its role switch')
  await firstTypeView.click()
  await page.getByRole('button', { name: 'Close', exact: true }).click()
  await page.getByRole('button', { name: '配置无机课堂角色权限' }).click()
  check(await secondTypeView.isChecked() === secondTypeBefore, 'Inspection role edit changed another inspection item')
  await page.getByRole('button', { name: 'Close', exact: true }).click()
  await page.getByRole('button', { name: '配置到课率角色权限' }).click()
  await firstTypeView.click()
  await page.getByRole('button', { name: 'Close', exact: true }).click()
  await page.getByRole('button', { name: '配置到课率部门功能' }).click()
  await page.getByRole('switch', { name: '宿管部 到课率 查看记录' }).click()
  await page.getByRole('button', { name: 'Close', exact: true }).click()
  await page.getByRole('switch', { name: '宿管部 到课率 承担此检查' }).click()
  check(await page.getByRole('switch', { name: '宿管部 承担周通报' }).isDisabled(), 'Second maintainer switch is enabled')
  check(await page.getByRole('button', { name: '角色权限', exact: true }).last().isDisabled(), 'Non-maintainer role editor is enabled')
  await page.getByText('已由学生会办公室维护，不能重复开启', { exact: true }).waitFor()
  const catalogsResponse = await page.request.get(`${base}/api/v1/admin/catalogs`, { headers: { Authorization: `Bearer ${adminToken}` } })
  const catalogs = await catalogsResponse.json()
  const dorm = catalogs.departments.find(item => item.code === 'DORMITORY')
  const path = `${base}/api/v1/admin/departments/${dorm.id}/default-permissions`
  const defaults = await (await page.request.get(path, { headers: { Authorization: `Bearer ${adminToken}` } })).json()
  const blank = { canView: false, canEnter: false, canManage: false, canReview: false, canExport: false }
  defaults.scopes = catalogs.inspectionTypes.map(type => defaults.scopes.find(item => item.inspectionTypeId === type.id) ?? { ...blank, departmentId: dorm.id, inspectionTypeId: type.id, enabled: false })
  defaults.inspectionRoles = catalogs.inspectionTypes.flatMap(type => ['MEMBER', 'LEADERSHIP', 'PRESIDIUM'].map(roleCode => defaults.inspectionRoles.find(item => item.inspectionTypeId === type.id && item.roleCode === roleCode) ?? { ...blank, inspectionTypeId: type.id, roleCode }))
  defaults.reportRoles = ['MEMBER', 'MINISTER', 'VICE_MINISTER'].map(roleCode => defaults.reportRoles.find(item => item.roleCode === roleCode) ?? { ...defaults.reportScope, roleCode })
  const blocked = await page.request.put(path, { headers: { Authorization: `Bearer ${adminToken}` }, data: { ...defaults, reportScope: { ...defaults.reportScope, enabled: true } } })
  const blockedBody = await blocked.json()
  check(blocked.status() === 409 && blockedBody.code === 'REPORT_MAINTENANCE_ALREADY_ASSIGNED', `Second maintainer rejection: ${blocked.status()} ${blockedBody.code}`)
  cases.push('exclusive maintainer UI and API rejection')
  const headers = { Authorization: `Bearer ${adminToken}` }
  const users = await (await page.request.get(`${base}/api/v1/admin/users`, { headers })).json()
  const plain = users.find(user => user.loginName === fixture.reportingRoleUsers.member)
  const delegated = users.find(user => user.loginName === fixture.reportingRoleUsers.scopedAdmin)
  const grant = { action: 'REPORT_CORRECT', effect: 'ALLOW', scope: {}, note: '隔离回归授权资格' }
  const rejected = await page.request.post(`${base}/api/v1/admin/users/${plain.accountId}/permission-rules`, { headers, data: grant })
  check(rejected.status() === 400 && (await rejected.json()).code === 'REPORT_MAINTENANCE_DEPARTMENT_REQUIRED', 'Ordinary account grant was not rejected at creation')
  const created = await page.request.post(`${base}/api/v1/admin/users/${delegated.accountId}/permission-rules`, { headers, data: grant })
  check(created.status() === 201, `Explicit administrator grant failed: ${created.status()}`)
  const createdRule = await created.json()
  const delegatedPermissions = await (await page.request.get(`${base}/api/v1/admin/users/${delegated.accountId}/permissions`, { headers })).json()
  check(delegatedPermissions.permissions.some(item => item.action === 'REPORT_CORRECT' && item.effect === 'ALLOW'), 'Administrator explicit grant not effective')
  const deleted = await page.request.delete(`${base}/api/v1/admin/users/${delegated.accountId}/permission-rules/${createdRule.id}`, { headers })
  check(deleted.status() === 204, 'Isolated administrator grant cleanup failed')
  cases.push('非维护部门普通账号新增授权被拒绝；系统管理员单独授权生效并可撤销')

  await page.screenshot({ path: `${outputDir}/department-report-narrow.png` })
  await page.getByRole('combobox', { name: '选择配置部门' }).click()
  await page.locator('.ant-select-dropdown:not(.ant-select-dropdown-hidden) .ant-select-item-option').filter({ hasText: '学生会办公室' }).click()
  await page.getByRole('heading', { name: '日常检查' }).waitFor()
  await page.screenshot({ path: `${outputDir}/office-report-narrow.png` })
  await page.setViewportSize({ width: 1280, height: 900 })
  await page.getByRole('button', { name: '角色权限', exact: true }).last().click()
  check(await page.getByRole('columnheader', { name: '部长', exact: true }).count() === 1, 'Office minister column label is incorrect')
  check(await page.getByRole('columnheader', { name: '副部长', exact: true }).count() === 1, 'Office vice minister column label is incorrect')
  await page.getByRole('switch', { name: '学生会办公室 委员 生成草稿' }).waitFor()
  check(await page.getByRole('switch', { name: '学生会办公室 委员 审核', exact: true }).isDisabled(), 'Office member review switch is editable')
  const archiveDefault = page.getByRole('switch', { name: '学生会办公室 委员 查看归档' })
  const saveDefaults = async () => {
    await page.getByRole('button', { name: /^保存修改 \(/ }).click()
    const dialog = page.getByRole('dialog', { name: '保存“学生会办公室”默认权限？' })
    await dialog.waitFor()
    const responsePromise = page.waitForResponse(response => response.url().endsWith('/api/v1/admin/departments/10000000-0000-0000-0000-000000000001/default-permissions') && response.request().method() === 'PUT')
    await dialog.getByRole('button', { name: '保存全部修改' }).click()
    const response = await responsePromise
    check(response.status() === 200, `Default permission save returned ${response.status()}: ${await response.text()}`)
    return response.json()
  }
  check(await archiveDefault.isChecked(), 'Office member archive default did not load')
  await archiveDefault.click()
  await page.getByRole('button', { name: 'Close', exact: true }).click()
  const savedDefaults = await saveDefaults()
  const staleSave = await page.request.put(`${base}/api/v1/admin/departments/${savedDefaults.departmentId}/default-permissions`, {
    headers: { Authorization: `Bearer ${adminToken}` }, data: { ...savedDefaults, version: savedDefaults.version - 1 },
  })
  check(staleSave.status() === 409, `Stale department default save returned ${staleSave.status()}`)
  await page.reload()
  await page.getByRole('heading', { name: '部门与默认权限' }).waitFor()
  await page.getByRole('combobox', { name: '选择配置部门' }).click()
  await page.locator('.ant-select-dropdown:not(.ant-select-dropdown-hidden) .ant-select-item-option').filter({ hasText: '学生会办公室' }).click()
  await page.getByRole('button', { name: '角色权限', exact: true }).last().click()
  check(!await archiveDefault.isChecked(), 'Saved archive default did not persist')
  await archiveDefault.click()
  await page.getByRole('button', { name: 'Close', exact: true }).click()
  await saveDefaults()
  cases.push('办公室岗位动作两次保存及重载验证持久化，委员审核不可配置，旧版本提交被拒绝')
  await page.setViewportSize({ width: 614, height: 749 })
  await page.getByRole('tab', { name: '部门', exact: true }).click()
  await page.getByRole('button', { name: '新增部门' }).waitFor()
  await page.screenshot({ path: `${outputDir}/department-list-narrow.png`, fullPage: true })
  await page.setViewportSize({ width: 1280, height: 900 })
  const request = async (path, data, token = adminToken) => {
    const response = await page.request.post(`${base}/api/v1${path}`, {
      headers: { Authorization: `Bearer ${token}` }, data,
    })
    check(response.ok(), `${path} returned ${response.status()}: ${await response.text()}`)
    return response.json()
  }
  const weekDate = await page.request.put(`${base}/api/v1/reports/output-settings/week-dates/${semesterId}/${weekId}`, {
    headers: { Authorization: `Bearer ${adminToken}` },
    data: { issueDate: fixture.weekStart, version: 0 },
  })
  check(weekDate.ok(), `Report issue date returned ${weekDate.status()}: ${await weekDate.text()}`)
  await logout()
  const draftMemberToken = await login(fixture.reportingRoleUsers.officeMember)
  let report = await request('/reports/drafts', { semesterId, weekId, expectedRowVersion: 0 }, draftMemberToken)
  check(report.status === 'DRAFT', 'Draft creation did not enter DRAFT state')
  const settingsResponse = await page.request.get(`${base}/api/v1/reports/output-settings`, {
    headers: { Authorization: `Bearer ${draftMemberToken}` },
  })
  check(settingsResponse.ok(), `Report output settings returned ${settingsResponse.status()}`)
  const settings = await settingsResponse.json()
  for (const format of ['WORD', 'EXCEL']) {
    const output = await page.request.post(`${base}/api/v1/reports/workflow/${semesterId}/${weekId}/versions/1/outputs`, {
      headers: { Authorization: `Bearer ${draftMemberToken}` },
      data: { format, typeCodes: ['ATTENDANCE_RATE'], combined: false, settingsVersion: settings.version },
    })
    const bytes = await output.body()
    check(output.status() === 200 && bytes.length > 500 && bytes[0] === 80 && bytes[1] === 75,
      `Office member ${format} draft output returned ${output.status()}: ${output.status() === 200 ? `${bytes.length} bytes` : await output.text()}`)
  }
  cases.push('办公室委员使用默认权限生成草稿和当前草稿 Word、Excel 文件')
  await logout()
  adminToken = await login('browser-admin')
  report = await request(`/reports/${report.id}/submit`, { rowVersion: report.rowVersion })
  report = await request(`/reports/${report.id}/approve`, { rowVersion: report.rowVersion })
  report = await request(`/reports/${report.id}/finalize`, { rowVersion: report.rowVersion })
  check(report.status === 'FINALIZED', 'Report did not reach FINALIZED state')
  const prematureRevision = await page.request.post(`${base}/api/v1/reports/drafts`, {
    headers: { Authorization: `Bearer ${adminToken}` },
    data: { semesterId, weekId, expectedRowVersion: report.rowVersion },
  })
  check(prematureRevision.status() === 403, `Finalized revision without confirmation returned ${prematureRevision.status()}`)
  await page.goto(archivePath)
  await page.getByRole('heading', { name: '通报归档', exact: true }).waitFor()
  const revise = page.getByRole('button', { name: '基于定稿修正', exact: true })
  await revise.waitFor()
  await revise.click()
  const confirmation = page.getByRole('dialog', { name: '基于定稿发起更正？' })
  await confirmation.waitFor()
  await confirmation.getByText(/当前定稿 V1 将保留/).waitFor()
  await confirmation.getByRole('button', { name: /取\s*消/ }).click()
  check(page.url().split('?')[0].endsWith('/reports/archive'), 'Cancel unexpectedly left archive')
  await revise.click()
  const revisionResponse = page.waitForResponse(response => response.url().endsWith(`/api/v1/reports/${report.id}/revision-intents`) && response.request().method() === 'POST')
  await confirmation.getByRole('button', { name: '确认发起' }).click()
  const revisionResult = await revisionResponse
  check(revisionResult.status() === 201, `Finalized revision intent returned ${revisionResult.status()}`)
  await page.waitForURL('**/reports/workbench?**')
  cases.push('定稿归档的更正入口显示影响范围；取消停留归档，确认写入服务端后进入现有工作台')

  await logout()
  const memberToken = await login(fixture.reportingRoleUsers.officeMember)
  await page.goto(archivePath)
  await page.getByRole('heading', { name: '通报归档', exact: true }).waitFor()
  await page.getByText('浏览器回归学期').first().waitFor()
  check(await page.getByRole('button', { name: '基于定稿修正', exact: true }).count() === 0,
    'Office member can launch finalized revision from archive')
  const forbidden = await page.request.post(`${base}/api/v1/reports/drafts`, {
    headers: { Authorization: `Bearer ${memberToken}` },
    data: { semesterId, weekId, expectedRowVersion: report.rowVersion },
  })
  check(forbidden.status() === 403, `Office member finalized revision returned ${forbidden.status()}`)
  cases.push('办公室委员可查看归档，但无定稿更正入口且直接发起请求被服务端拒绝')
  await logout()
  const presidiumToken = await login(fixture.reportingRoleUsers.studyPresidium)
  const presidiumHeaders = { Authorization: `Bearer ${presidiumToken}` }
  const inspections = await page.request.get(`${base}/api/v1/inspections?inspectionTypeId=30000000-0000-0000-0000-000000000001&page=1&pageSize=100`, {
    headers: presidiumHeaders,
  })
  check(inspections.status() === 200, `Presidium inspections returned ${inspections.status()}`)
  const inspectionPage = await inspections.json()
  check(inspectionPage.items.length > 0 && inspectionPage.items.every(item => item.departmentId === '10000000-0000-0000-0000-000000000002'),
    'Presidium inspection list crossed its primary department')
  const outsideRecord = await page.request.get(`${base}/api/v1/inspections/c0000000-0000-0000-0000-000000000006`, {
    headers: presidiumHeaders,
  })
  check(outsideRecord.status() === 404, `Presidium cross-department detail returned ${outsideRecord.status()}`)
  cases.push('学习部主席团仅能读取主部门检查记录，其他来源详情被拒绝')
  check(browserErrors.length === 0, `Browser errors: ${browserErrors.join('; ')}`)
  return { cases, browserErrors, browserVersion: await page.evaluate(() => navigator.userAgent) }
}
