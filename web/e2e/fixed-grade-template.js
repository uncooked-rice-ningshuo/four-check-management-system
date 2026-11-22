async page => {
  const base = page.url().split('/').slice(0, 3).join('/')
  const choose = async (root, field, label) => {
    await root.getByRole('combobox', { name: field, exact: true }).click()
    await page.locator('.ant-select-dropdown:visible').getByText(label, { exact: true }).click()
  }
  const openMatrix = async () => {
    await page.goto(base + '/admin/organization')
    await choose(page, '选择配置部门', '大一年级部')
    await page.getByRole('button', { name: '配置到课率角色权限', exact: true }).click()
    return page.getByRole('dialog', { name: '到课率 · 角色权限', exact: true })
  }
  for (const withMajor of [false, true]) {
    const matrix = await openMatrix()
    const toggle = matrix.getByRole('switch', { name: '大一年级部 到课率 委员 录入与提交', exact: true })
    if (!withMajor) {
      if (await toggle.getAttribute('aria-checked') !== 'false') throw Error('Legacy entry still enabled')
      await toggle.click()
    }
    await matrix.getByRole('button', { name: '委员录入与提交范围' }).click()
    const form = page.getByRole('dialog', { name: '到课率 · 委员 · 录入与提交 · 操作范围' })
    if (!withMajor) {
      await form.getByRole('button', { name: '添加一组范围' }).click()
      await choose(form, '第1组年级', '2026级')
    } else {
      await form.getByText('2026级', { exact: true }).first().waitFor()
      await choose(form, '第1组专业', 'E2E faculty／E2E major')
    }
    await form.getByRole('button', { name: '应用范围' }).click()
    await matrix.getByRole('button', { name: 'Close', exact: true }).click()
    await page.getByRole('button', { name: /保存修改/ }).click()
    await page.getByRole('button', { name: '保存全部修改' }).click()
    await page.getByText('部门默认权限已保存').waitFor()
  }
  const matrix = await openMatrix()
  await matrix.getByRole('button', { name: '委员录入与提交范围' }).click()
  const form = page.getByRole('dialog', { name: '到课率 · 委员 · 录入与提交 · 操作范围' })
  await form.getByText('E2E faculty／E2E major', { exact: true }).first().waitFor()
  const fixture = __FIXTURE__
  const api = async (path, method, data, token, expected) => {
    const response = await page.request.fetch(base + '/api/v1' + path, { method, data, headers: token ? { Authorization: 'Bearer ' + token } : {} })
    const body = await response.json()
    if (response.status() !== expected) throw Error(path + ': ' + response.status() + ' ' + (body.code ?? '') + ' ' + (body.message ?? ''))
    return body
  }
  const login = async loginName => (await api('/auth/login', 'POST', { loginName, password: fixture.authPassword }, '', 200)).accessToken
  const admin = await login('browser-admin')
  const catalog = await api('/admin/catalogs','GET',undefined,admin,200)
  const departmentId = catalog.departments.find(d => d.code === 'GRADE_ONE').id
  const inspectionTypeId = catalog.inspectionTypes.find(t => t.code === 'ATTENDANCE_RATE').id
  const record = { departmentId, inspectionTypeId, semesterId:'b0000000-0000-0000-0000-000000000002',weekId:'b0000000-0000-0000-0000-000000000003',
    classId:fixture.class,grade:'2026级',className:'E2E class',studentNo:'000001',studentName:'E2E student A',courseId:fixture.course,
    classroom:'E2E building 101', date:fixture.weekStart,time:'08:00',period:'1-2',absenceHours:2 }
  for (const roleCode of ['MEMBER','VICE_MINISTER']) {
    const name = 'range-' + roleCode.toLowerCase()
    const requestedOrgRoleId = catalog.organizationRoles.find(r => r.code === roleCode).id
    const account = await api('/auth/register','POST',{studentNo:name,name,password:fixture.authPassword,requestedDepartmentId:departmentId,requestedOrgRoleId},'',201)
    await api('/admin/registration-requests/'+account.id+'/review','POST',{decision:'APPROVE',reason:'isolated regression'},admin,200)
    const access = await login(name)
    await api('/inspections/attendance','POST',record,access,roleCode === 'MEMBER' ? 201 : 403)
    if (roleCode === 'MEMBER') await api('/inspections/attendance','POST',{...record,classId:fixture.outsideClass,grade:'2025级'},access,403)
  }
  return { browserVersion: page.context().browser().version(), cases: ['委员仅年级保存重开', '委员年级和专业保存重开','委员范围内写入及跨年级拒绝','副部长未获录入权限'] }
}
