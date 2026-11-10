async (page) => {
  const fixture = __FIXTURE__
  const origin = page.url().split('/').slice(0,3).join('/')
  const browser = page.context().browser()
  const apiContext = await browser.newContext()
  const api = async (path, token, method = 'GET', data, expected = 200) => {
    const res = await apiContext.request.fetch(origin + '/api/v1' + path, {
      method, data, headers: token ? { Authorization: 'Bearer ' + token } : {},
    })
    const body = await res.json()
    if (res.status() !== expected) throw new Error(method + ' ' + path.split('?')[0] + ' got ' + res.status() + ' ' + body.code)
    return body
  }
  const check = (value, message) => { if (!value) throw new Error(message) }
  const admin = (await api('/auth/login', '', 'POST', { loginName: 'browser-admin', password: fixture.authPassword })).accessToken
  const catalogs = await api('/public/registration-options')
  const study = catalogs.departments.find(d => d.code === 'STUDY')
  const role = catalogs.organizationRoles.find(r => r.code === 'MEMBER')
  async function register(name, department, organizationRole = role) {
    const account = await api('/auth/register', '', 'POST', { studentNo: name, name, password: fixture.matrixPassword, requestedDepartmentId: department.id, requestedOrgRoleId: organizationRole.id }, 201)
    await api('/admin/registration-requests/' + account.id + '/review', admin, 'POST', { decision: 'APPROVE', reason: 'isolated submission regression' })
    if (department.code === 'OFFICE') {
      for (const action of ['INSPECTION_VIEW', 'INSPECTION_ENTER', 'INSPECTION_MANAGE', 'INSPECTION_REVIEW']) {
        await api('/admin/users/' + account.id + '/permission-rules', admin, 'POST', { action, effect: 'ALLOW', scope: {}, enabled: true, note: 'isolated office role test' }, 201)
      }
    }
    return (await api('/auth/login', '', 'POST', { loginName: name, password: fixture.matrixPassword })).accessToken
  }
  const member = await register('routing-member', study)
  const officeDepartment = catalogs.departments.find(d => d.code === 'OFFICE')
  const offices = []
  for (const code of ['MEMBER', 'MINISTER', 'VICE_MINISTER', 'PRESIDIUM']) {
    const name = 'routing-office-' + code.toLowerCase().replace('_', '-')
    offices.push({ name, token: await register(name, officeDepartment, catalogs.organizationRoles.find(r => r.code === code)) })
  }
  const office = offices[0].token
  const scope = { semesterId: 'b0000000-0000-0000-0000-000000000002', weekId: 'b0000000-0000-0000-0000-000000000003', inspectionTypeId: '30000000-0000-0000-0000-000000000001' }
  const adminChannel = '00000000-0000-0000-0000-000000000000'
  const details = studentId => ({ studentId, classId: fixture.class, courseId: fixture.course, classroomId: fixture.room, date: fixture.date, period: '1-2', absenceHours: 2 })
  const draft = await api('/inspections', member, 'POST', { ...scope, details: details(fixture.studentA) }, 201)
  const adminDraft = await api('/inspections', admin, 'POST', { ...scope, details: details(fixture.studentB) }, 201)
  const candidates = (actor, departmentId, expected = 200) => api('/inspections/submission-candidates?' + Object.entries({ ...scope, departmentId, page: '1', pageSize: '10' }).map(([k,v]) => k + '=' + encodeURIComponent(v)).join('&'), actor, 'GET', undefined, expected)
  check((await candidates(member, adminChannel, 403)).code === 'FORBIDDEN_SCOPE', 'wrong scope response')
  const rows = (await candidates(member, study.id)).items
  check(rows.some(r => r.id === draft.id && r.classId === fixture.class && r.className === 'E2E class'), 'member candidate class mismatch')
  check(!rows.some(r => r.id === adminDraft.id), 'member can submit administrator draft')
  for (const target of [study.id, adminChannel]) check((await candidates(admin, target)).items.some(r => r.id === adminDraft.id), 'missing administrator draft')
  await api('/inspections/' + adminDraft.id, member)
  await api('/inspections/' + adminDraft.id, office, 'GET', undefined, 404)
  await page.goto(origin + '/inspections/attendance')
  await page.getByRole('button', { name: '提交记录', exact: true }).click()
  const cart = page.getByRole('dialog', { name: '提交检查记录' })
  async function select(label, option) {
    await cart.getByRole('combobox', { name: label }).click()
    await page.locator('.ant-select-dropdown:visible .ant-select-item-option').filter({ hasText: option }).first().click()
  }
  await select('提交学期', '浏览器回归学期')
  await select('提交周次', fixture.weekStart)
  for (const name of ['管理员', '学习部']) {
    await select('提交归属', name)
    await cart.getByRole('button', { name: '下一步：添加记录' }).click()
    const row = cart.getByRole('row').filter({ hasText: 'E2E student B' })
    await row.waitFor()
    if (name === '管理员') await cart.getByRole('button', { name: '上一步' }).click()
    else await row.getByRole('checkbox').check()
  }
  await cart.getByRole('button', { name: '下一步：核对清单' }).click()
  await cart.getByRole('button', { name: '提交审核' }).click()
  const confirmation = page.getByRole('dialog', { name: '最终确认本次提交' })
  const confirmButton = confirmation.getByRole('button', { name: '确认提交' })
  await confirmButton.waitFor()
  const isAboveDrawer = () => confirmButton.evaluate(button => {
    const rect = button.getBoundingClientRect()
    const top = document.elementFromPoint(rect.x + rect.width / 2, rect.y + rect.height / 2)
    const drawer = [...document.querySelectorAll('.ant-drawer')].find(node => node.querySelector('.submission-steps'))
    const wrap = button.closest('.ant-modal-wrap')
    return !!drawer && !!wrap && Number(getComputedStyle(wrap).zIndex) > Number(getComputedStyle(drawer).zIndex) && button.contains(top)
  })
  await page.setViewportSize({ width: 390, height: 844 })
  await confirmButton.click({ trial: true })
  check(await isAboveDrawer(), 'confirmation must receive pointer input above the drawer')
  await confirmation.getByRole('button', { name: /取\s*消/ }).click()
  await confirmation.waitFor({ state: 'hidden' })
  await cart.getByRole('button', { name: '提交审核' }).click()
  await page.setViewportSize({ width: 1280, height: 800 })
  await confirmButton.click({ trial: true })
  check(await isAboveDrawer(), 'reopened confirmation is covered')
  const posted = page.waitForResponse(r => r.url().endsWith('/department-submissions') && r.request().method() === 'POST')
  await confirmButton.click()
  const result = await posted
  check(result.status() === 201, 'administrator UI submission failed')
  const batch = await result.json()
  await page.getByRole('button',{name:'检查提交批次'}).click()
  const batches = page.getByRole('dialog',{name:'到课率提交批次'})
  await batches.getByRole('button',{name:'查看明细'}).first().click()
  await batches.getByText('E2E student B',{exact:true}).waitFor()
  await batches.getByText('E2E course',{exact:true}).waitFor()
  await batches.getByRole('button',{name:'返回批次列表'}).click()
  await batches.getByRole('button',{name:'查看明细'}).first().waitFor()
  await page.keyboard.press('Escape')

  check(batch.scope.departmentId === study.id, 'wrong submission department')
  for (const viewer of offices) {
    await api('/inspections/' + adminDraft.id, viewer.token, 'GET', undefined, 404)
    await api('/department-submissions/' + batch.id, viewer.token, 'GET', undefined, 404)
    await api('/department-submissions/' + batch.id + '/confirm', viewer.token, 'POST', { version: batch.rowVersion }, 403)
  }
  await api('/department-submissions/' + batch.id + '/confirm', admin, 'POST', { version: batch.rowVersion })
  await api('/inspections/' + adminDraft.id, office)
  const cases = ['管理员核对弹窗在窄屏和桌面均位于抽屉上方，取消后保留清单，重开后鼠标确认提交返回 201', '到课率委员主部门 200、管理员通道 403；双通道共享本人候选；学办只见已确认记录']
  const browserErrors = []
  for (const name of ['routing-member', ...offices.map(o => o.name)]) {
    const context = await browser.newContext()
    try {
      const tab = await context.newPage()
      tab.on('pageerror', error => browserErrors.push(error.message))
      await tab.goto(origin + '/login')
      await tab.getByRole('textbox', { name: '账号', exact: true }).fill(name)
      await tab.locator('input[type=password]').fill(fixture.matrixPassword)
      await tab.getByRole('button', { name: /登\s*录/ }).click()
      await tab.waitForURL(url => url.pathname === '/')
      await tab.goto(origin + '/inspections/attendance')
      await tab.getByText('E2E student B', { exact: true }).first().waitFor()
      const submit = tab.getByRole('button', { name: '提交记录', exact: true })
      if (name === 'routing-member') {
        await submit.click()
        const target = tab.getByRole('combobox', { name: '提交归属' })
        check(await target.isDisabled(), 'member can change department')
        await tab.getByRole('dialog').getByText('学习部', { exact: true }).first().waitFor()
      } else {
        for (const path of ['attendance', 'unattended-classroom', 'morning-attendance', 'late-return', 'evening-self-study', 'dormitory-hygiene', 'classroom-hygiene', 'public-dormitory-hygiene']) {
          if (path !== 'attendance') await tab.goto(origin + '/inspections/' + path)
          await tab.locator('.ant-pro-table').first().waitFor()
          check(await submit.count() === 0, name + ' can submit on ' + path)
          check(await tab.getByRole('button', { name: /新增记录|检查提交批次|批量导入|编辑|删除|提交零记录/ }).count() === 0, name + ' has mutation controls on ' + path)
        }
      }
    } finally { await context.close() }
  }
  check(!browserErrors.length, 'browser page errors')
  cases.push('委员归属只读显示学习部；学办四种职务在八类日常检查页面均无写入与审核按钮')
  await apiContext.close()
  return { cases, browserVersion: browser.version(), browserErrors }
}
