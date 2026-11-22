import { expect } from '@playwright/test'
import { test } from './fixture'

test.use({ video: { mode: 'on', size: { width: 1440, height: 900 } } })

test('共同负责到课率的三个部门分别提交和审核', async ({ page, request, aiAssert }) => {
  test.skip(!process.env.CROSS_DEPT_FIXTURE, '由隔离浏览器测试脚本准备三个部门')
  test.setTimeout(300_000)
  const f = JSON.parse(process.env.CROSS_DEPT_FIXTURE!)
  const password = process.env.CROSS_DEPT_PASSWORD!
  const departments = ['STUDY', 'DORMITORY', 'SPORTS']
  const drafts: { id: string, version: number }[] = []
  for (const [i, department] of departments.entries()) {
    const login = await request.post('/api/v1/auth/login', { data: { loginName: 'gui-' + department, password } })
    expect(login.status()).toBe(200)
    const draft = await request.post('/api/v1/inspections', {
      headers: { Authorization: 'Bearer ' + (await login.json()).accessToken },
      data: { departmentId: '10000000-0000-0000-0000-00000000000' + (i + 2), semesterId: 'b0000000-0000-0000-0000-000000000002', weekId: 'b0000000-0000-0000-0000-000000000003',
        inspectionTypeId: '30000000-0000-0000-0000-000000000001',
        details: { studentId: i === 0 ? f.studentA : f.studentB, classId: f.class, courseId: f.course,
          classroomId: f.room, date: f.date, period: i === 2 ? '3-4' : '1-2', absenceHours: 2 } },
    })
    expect(draft.status(), department + ' ' + (await draft.text())).toBe(201)
    const saved = await draft.json()
    drafts.push({ id: saved.id, version: saved.rowVersion })
  }

  async function authorization(loginName: string) {
    const response = await request.post('/api/v1/auth/login', { data: { loginName, password } })
    expect(response.status()).toBe(200)
    return { Authorization: 'Bearer ' + (await response.json()).accessToken }
  }
  const member = await authorization('gui-DORMITORY')
  const study = await authorization('gui-STUDY')
  const senior = await request.post('/api/v1/inspections', { headers: study, data: {
    semesterId: 'b0000000-0000-0000-0000-000000000002', weekId: 'b0000000-0000-0000-0000-000000000003',
    inspectionTypeId: '30000000-0000-0000-0000-000000000001', details: {
      studentId: f.student24, classId: f.class24, courseId: f.course, classroomId: f.room,
      date: f.date, period: '1-2', absenceHours: 2 },
  } })
  expect(senior.status()).toBe(201)
  for (const id of [(await senior.json()).id, drafts[0].id, drafts[1].id]) {
    const detail = await request.get('/api/v1/inspections/' + id, { headers: member })
    expect(detail.status()).toBe(200)
    expect(await detail.json()).toMatchObject({ canEdit: id === drafts[1].id, canDelete: id === drafts[1].id })
  }
  for (const endpoint of ['/api/v1/inspections', '/api/v1/inspections/attendance']) {
    const denied = await request.post(endpoint + '/batch', { headers: member,
      data: { action: 'DELETE', items: [drafts[1], drafts[0]], reason: '隔离批删回归' } })
    expect(denied.status()).toBe(403)
    expect((await denied.json()).code).toBe('FORBIDDEN_SCOPE')
    for (const draft of drafts.slice(0, 2)) {
      const detail = await request.get('/api/v1/inspections/' + draft.id, { headers: member })
      expect(detail.status()).toBe(200)
      expect((await detail.json()).rowVersion).toBe(draft.version)
    }
  }
  const creator = await authorization('audit-admin-a')
  const manager = await authorization('audit-admin-b')
  const supervisor = await authorization('audit-super')
  const adminInput = {
    semesterId: 'b0000000-0000-0000-0000-000000000002', weekId: 'b0000000-0000-0000-0000-000000000003',
    inspectionTypeId: '30000000-0000-0000-0000-000000000001', details: {
      studentId: f.studentA, classId: f.class, courseId: f.course, classroomId: f.room,
      date: f.date, period: '5-6', absenceHours: 2 },
  }
  const created = await request.post('/api/v1/inspections', { headers: creator, data: adminInput })
  expect(created.status()).toBe(201)
  const adminDraft = await created.json()
  expect(adminDraft.attributionMode).toBe('ADMINISTRATOR')
  for (const [headers, allowed] of [[creator, true], [manager, false], [supervisor, true]] as const) {
    const detail = await request.get('/api/v1/inspections/' + adminDraft.id, { headers })
    expect(detail.status()).toBe(200)
    expect(await detail.json()).toMatchObject({ canEdit: allowed, canDelete: allowed })
  }
  const deletion = { version: adminDraft.rowVersion, reason: '管理员来源隔离回归' }
  const foreignDelete = await request.delete('/api/v1/inspections/' + adminDraft.id, { headers: manager, data: deletion })
  expect(foreignDelete.status()).toBe(403)
  const superDelete = await request.delete('/api/v1/inspections/' + adminDraft.id, { headers: supervisor, data: deletion })
  expect(superDelete.status()).toBe(204)

  async function signIn(department: string) {
    await page.context().clearCookies()
    await page.goto('/login')
    await page.getByRole('textbox', { name: '账号', exact: true }).fill('gui-' + department)
    await page.locator('input[type=password]').fill(password)
    await page.getByRole('button', { name: /登\s*录/ }).click()
    await page.waitForURL(url => url.pathname === '/')
    await page.goto('/inspections/attendance')
  }
  async function openCandidates() {
    await page.getByRole('button', { name: '提交记录', exact: true }).click()
    const dialog = page.getByRole('dialog', { name: '提交检查记录' })
    for (const [label, value] of [['提交学期', '浏览器回归学期'], ['提交周次', f.weekStart]]) {
      await dialog.getByRole('combobox', { name: label }).click()
      await page.locator('.ant-select-dropdown:visible .ant-select-item-option').filter({ hasText: value }).click()
    }
    await dialog.getByRole('button', { name: '下一步：添加记录' }).click()
    return dialog
  }
  for (const department of departments.slice(1)) {
    await signIn(department)
    const foreignDraft = page.getByRole('row').filter({ hasText: 'E2E student A' })
    await expect(foreignDraft).toBeVisible()
    await expect.soft(foreignDraft.getByRole('button', { name: '编辑', exact: true }), department + ' 外部门草稿编辑入口').toHaveCount(0, { timeout: 1000 })
    await expect.soft(foreignDraft.getByRole('button', { name: '删除', exact: true }), department + ' 外部门草稿删除入口').toHaveCount(0, { timeout: 1000 })
    const dialog = await openCandidates()
    await expect(dialog.getByText('E2E student B', { exact: true })).toBeVisible()
    await expect(dialog.getByText('E2E student A', { exact: true })).toHaveCount(0)
    await expect(dialog.getByText('当前范围有 1 条可提交记录')).toBeVisible()
    await page.screenshot({ path: test.info().outputPath(department + '-candidates.png') })
  }
  await signIn('STUDY')
  const dialog = await openCandidates()
  await dialog.getByRole('row').filter({ hasText: 'E2E student A' }).getByRole('checkbox').check()
  await dialog.getByRole('button', { name: '下一步：核对清单' }).click()
  await dialog.getByRole('button', { name: '提交审核' }).click()
  const posted = page.waitForResponse(r => r.request().method() === 'POST' && r.url().endsWith('/department-submissions'))
  await page.getByRole('button', { name: '确认提交' }).click()
  expect((await posted).status()).toBe(201)
  await page.getByRole('button', { name: '检查提交批次' }).click()
  await expect(page.getByRole('dialog', { name: '到课率提交批次' }).getByRole('row').filter({ hasText: 'GUI-STUDY' })).toContainText('待部门审核')
  for (const department of departments.slice(1)) {
    await signIn(department)
    await expect(page.getByRole('button', { name: '检查提交批次' })).toBeVisible()
    const list = page.waitForResponse(r => new URL(r.url()).pathname.endsWith('/department-submissions') && r.request().method() === 'GET')
    await page.getByRole('button', { name: '检查提交批次' }).click()
    expect((await list).status()).toBe(200)
    const emptyList = page.getByText('当前范围暂无提交批次')
    await emptyList.scrollIntoViewIfNeeded()
    await expect(emptyList).toBeInViewport()
    await expect(emptyList).toBeVisible()
    await page.screenshot({ path: test.info().outputPath(department + '-batches.png') })
  }
  if (process.env.CROSS_DEPT_DOM_ONLY !== '1') {
    await aiAssert('提交批次列表显示当前范围暂无提交批次')
  }
  const routed = await request.post('/api/v1/inspections', { headers: creator, data: adminInput })
  expect(routed.status()).toBe(201)
  const original = await routed.json()
  const scope = { semesterId: adminInput.semesterId, weekId: adminInput.weekId,
    inspectionTypeId: adminInput.inspectionTypeId, departmentId: '10000000-0000-0000-0000-000000000002' }
  const submission = await request.post('/api/v1/department-submissions', { headers: creator,
    data: { scope, records: [{ id: original.id, version: original.rowVersion }], idempotencyKey: 'admin-routing-regression' } })
  expect(submission.status()).toBe(201)
  const batch = await submission.json()
  const batchURL = '/api/v1/department-submissions/' + batch.id
  const returned = await request.post(batchURL + '/return', { headers: study,
    data: { version: batch.rowVersion, reason: '请管理员修正' } })
  expect(returned.status()).toBe(200)
  const revision = await returned.json()
  const patch = { version: revision.records[0].rowVersion, reason: '更正备注',
    record: { ...adminInput, details: { ...adminInput.details, remark: '修正后' } } }
  const recordURL = '/api/v1/inspections/' + original.id
  expect((await request.patch(recordURL, { headers: study, data: patch })).status()).toBe(403)
  const corrected = await request.patch(recordURL, { headers: creator, data: patch })
  expect(corrected.status()).toBe(200)
  const resubmitted = await request.post(batchURL + '/resubmit', { headers: creator,
    data: { version: revision.rowVersion, records: [{ id: original.id, version: (await corrected.json()).rowVersion }] } })
  expect(resubmitted.status()).toBe(200)
  const confirmed = await request.post(batchURL + '/confirm', { headers: study,
    data: { version: (await resubmitted.json()).rowVersion } })
  expect(confirmed.status()).toBe(200)
  const final = await request.get(recordURL, { headers: supervisor })
  expect(await final.json()).toMatchObject({ attributionMode: 'ADMINISTRATOR', createdBy: original.createdBy,
    status: 'CONFIRMED', canEdit: false, canDelete: false })

})
