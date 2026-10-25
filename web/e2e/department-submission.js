async (page) => {
  const fixture = __FIXTURE__
  const cases = [], browserErrors = [], reportRequests = []
  page.on('pageerror', error => browserErrors.push(error.message))
  page.on('request', request => { if (/\/api\/v1\/(reports|report-templates)(\/|\?|$)/.test(request.url())) reportRequests.push(request.url().split('?')[0]) })
  const check = (ok, message) => { if (!ok) throw new Error(message) }
  const select = async (scope, label, text) => {
    await scope.getByRole('combobox', { name: new RegExp(label + '\\s*[:：]?$') }).click()
    await page.locator('.ant-select-dropdown:visible .ant-select-item-option').filter({ hasText: text }).first().click()
  }
  check(await page.getByText(/待生成通报|待通报审核|待定稿|全院通报进度/).count() === 0, 'stage-three placeholder appeared on phase-two workbench')
  const menu = page.locator('#admin-navigation .ant-menu-submenu:not(.ant-menu-submenu-open) > .ant-menu-submenu-title').filter({ hasText: '日常检查' })
  if (await menu.count()) await menu.click()
  await page.getByRole('link', { name: '早锻炼/早签', exact: true }).click()
  await page.getByRole('heading', { name: '早锻炼/早签检查', exact: true }).waitFor()
  await page.getByRole('button', { name: '新增记录', exact: true }).click()
  const create = page.getByRole('dialog', { name: '新增早锻炼/早签记录' })
  await select(create, '录入学期', '浏览器回归学期')
  await select(create, '录入周次', fixture.weekStart)
  await select(create, '目标部门', '体育部')
  await select(create, '学生班级', 'E2E class')
  await select(create, '学生', 'E2E student A')
  await create.getByLabel('检查日期').fill(fixture.date)
  await create.getByLabel('检查日期').press('Enter')
  await create.getByLabel('备注').fill('批次原值')
  await create.getByRole('button', { name: '保存草稿' }).click()
  await create.waitFor({ state: 'hidden' })
  const row = () => page.getByRole('row').filter({ has: page.getByRole('cell', { name: 'E2E student A', exact: true }) })
  await row().waitFor()
  await row().getByRole('checkbox').check()
  let firstKey = ''
  const lostResponse = async route => {
    if (route.request().method() !== 'POST') return route.continue()
    firstKey = route.request().postDataJSON().idempotencyKey
    const committed = await route.fetch()
    check(committed.status() === 201, 'isolated submit did not commit before response loss')
    await route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ code: 'SUBMISSION_FAILED', message: '模拟响应丢失' }) })
  }
  await page.route('**/api/v1/department-submissions', lostResponse)
  await page.getByRole('button', { name: '准备提交（已选 1）' }).click()
  const cart = page.getByRole('dialog', { name: '准备提交' })
  await cart.getByRole('button', { name: '核对并提交' }).click()
  const submit = page.getByRole('dialog', { name: '最终确认本次提交' })
  await submit.getByRole('button', { name: '确认提交' }).click()
  await page.getByText('模拟响应丢失').waitFor()
  await page.unroute('**/api/v1/department-submissions', lostResponse)
  const retryRequest = page.waitForRequest(request => request.method() === 'POST' && request.url().endsWith('/api/v1/department-submissions'))
  await submit.getByRole('button', { name: '确认提交' }).click()
  check((await retryRequest).postDataJSON().idempotencyKey === firstKey, 'response-loss retry did not reuse idempotency key')
  await submit.waitFor({ state: 'hidden' })
  await row().getByText('待审核').waitFor()
  cases.push('真实草稿提交；模拟服务端已提交但响应丢失后复用幂等键重试，列表状态同步')

  await page.getByRole('button', { name: '检查提交批次' }).click()
  let drawer = page.getByRole('dialog', { name: '检查提交批次' })
  await drawer.getByText('浏览器回归学期').waitFor()
  await drawer.getByRole('button', { name: '退回', exact: true }).click()
  const returnDialog = page.getByRole('dialog', { name: '退回提交批次' })
  await returnDialog.getByLabel('退回原因').fill('请核对备注')
  await returnDialog.getByRole('button', { name: '确认退回' }).click()
  await returnDialog.waitFor({ state: 'hidden' })
  await drawer.getByText('已退回').waitFor()
  await drawer.locator('.ant-table-row-expand-icon').first().click()
  await drawer.getByRole('button', { name: /更正第\s*1\s*条记录/ }).click()
  await drawer.waitFor({ state: 'hidden' })
  const edit = page.getByRole('dialog', { name: '编辑早锻炼/早签记录' })
  await edit.getByLabel('备注').fill('批次更正值')
  await edit.getByLabel('修改原因').fill('核对后更正')
  await edit.getByRole('button', { name: '保存修改' }).click()
  await edit.waitFor({ state: 'hidden' })
  cases.push('部门退回后原记录可更正，原因保留')

  await page.getByRole('button', { name: '检查提交批次' }).click()
  drawer = page.getByRole('dialog', { name: '检查提交批次' })
  await drawer.getByRole('button', { name: '重提', exact: true }).click()
  const resubmit = page.getByRole('dialog', { name: '重新提交该批次？' })
  await resubmit.getByRole('button', { name: '重新提交' }).click()
  await resubmit.waitFor({ state: 'hidden' })
  await drawer.getByText('待部门审核').waitFor()
  await drawer.getByText('2', { exact: true }).waitFor()
  await drawer.getByRole('button', { name: '确认', exact: true }).click()
  const confirm = page.getByRole('dialog', { name: '确认该批次？' })
  await confirm.getByRole('button', { name: '确认批次' }).click()
  await confirm.waitFor({ state: 'hidden' })
  await drawer.getByText('部门已确认').waitFor()
  cases.push('原批次修订递增后确认；真实 API 和 PostgreSQL 持久化')

  const origin = page.url().split('/').slice(0, 3).join('/')
  const contexts = []
  const context = async () => { const value = await page.context().browser().newContext(); contexts.push(value); return value }
  const request = async (client, token, path, method = 'GET', data, expected = 200, code) => {
    const response = await client.request.fetch(origin + '/api/v1' + path, { method, data, headers: token ? { Authorization: 'Bearer ' + token } : {} })
    check(response.status() === expected, `${method} ${path.split('?')[0]} expected ${expected}, got ${response.status()}`)
    if (code) check((await response.json()).code === code, `wrong error code for ${path}`)
    return response
  }
  try {
    const admin = await context()
    const adminToken = (await (await request(admin, '', '/auth/login', 'POST', { loginName: 'browser-admin', password: fixture.authPassword })).json()).accessToken
    const catalogs = await (await request(admin, '', '/public/registration-options')).json()
    const sports = catalogs.departments.find(item => item.code === 'SPORTS')
    const study = catalogs.departments.find(item => item.code === 'STUDY')
    const role = catalogs.organizationRoles.find(item => item.code === 'MEMBER')
    check(sports && study && role, 'role matrix catalogs missing')
    const options = await (await request(admin, adminToken, '/inspections/context-options?typeCode=MORNING_ATTENDANCE&purpose=view&kind=semesters')).json()
    const scope = { semesterId: 'b0000000-0000-0000-0000-000000000002', weekId: 'b0000000-0000-0000-0000-000000000003', departmentId: sports.id, inspectionTypeId: options.inspectionTypeId }
    const member = async (name, department) => {
      const client = await context()
      const account = await (await request(client, '', '/auth/register', 'POST', { studentNo: name, name, password: fixture.matrixPassword, requestedDepartmentId: department.id, requestedOrgRoleId: role.id }, 201)).json()
      await request(admin, adminToken, '/admin/registration-requests/' + account.id + '/review', 'POST', { decision: 'APPROVE', reason: 'isolated workflow role matrix' })
      return { client, id: account.id, name, token: '' }
    }
    const owner = await member('workflow-owner', sports)
    const reviewer = await member('workflow-reviewer', sports)
    const outsider = await member('workflow-outsider', study)
    await request(admin, adminToken, '/admin/users/' + reviewer.id + '/permission-rules', 'POST', { action: 'INSPECTION_REVIEW', effect: 'ALLOW', scope: { inspectionTypeId: scope.inspectionTypeId }, enabled: true, note: 'isolated reviewer' }, 201)
    await request(admin, adminToken, '/admin/users/' + reviewer.id + '/permission-rules', 'POST', { action: 'INSPECTION_ENTER', effect: 'DENY', scope: { inspectionTypeId: scope.inspectionTypeId }, enabled: true, note: 'review only' }, 201)
    for (const person of [owner, reviewer, outsider]) person.token = (await (await request(person.client, '', '/auth/login', 'POST', { loginName: person.name, password: fixture.matrixPassword })).json()).accessToken
    const input = { ...scope, details: { classId: fixture.class, studentId: fixture.studentB, date: fixture.date, remark: '成员初稿' } }
    await request(owner.client, owner.token, '/inspections', 'POST', {...input,departmentId:study.id}, 403, 'FORBIDDEN_SCOPE')
    const created = await (await request(owner.client, owner.token, '/inspections', 'POST', {...input,departmentId:''}, 201)).json()
    check(created.departmentId===sports.id,'member record did not inherit its department')
    const batch = await (await request(owner.client, owner.token, '/department-submissions', 'POST', { scope, records: [{ id: created.id, version: created.rowVersion }], idempotencyKey: 'workflow-matrix-owner-1' }, 201)).json()
    check(batch.records.length === 1 && batch.records[0].status === 'PENDING_REVIEW', 'member batch response incomplete')
    await request(owner.client, owner.token, '/department-submissions/' + batch.id + '/confirm', 'POST', { version: batch.rowVersion }, 403, 'FORBIDDEN_SCOPE')
    const hidden = await (await request(outsider.client, outsider.token, '/department-submissions?inspectionTypeId=' + scope.inspectionTypeId)).json()
    check(hidden.total === 0, 'cross-department submission list leaked')
    await request(outsider.client, outsider.token, '/department-submissions/' + batch.id, 'GET', undefined, 404, 'SUBMISSION_NOT_FOUND')
    await request(outsider.client, outsider.token, '/department-submissions/' + batch.id + '/return', 'POST', { version: batch.rowVersion, reason: 'denied' }, 403, 'FORBIDDEN_SCOPE')
    cases.push('录入成员可提交但不能审核；跨部门列表/详情/退回不泄漏批次')

    await request(reviewer.client, reviewer.token, '/inspections', 'POST', input, 403, 'FORBIDDEN_SCOPE')
    const returned = await (await request(reviewer.client, reviewer.token, '/department-submissions/' + batch.id + '/return', 'POST', { version: batch.rowVersion, reason: '请核对成员备注' })).json()
    const corrected = await (await request(owner.client, owner.token, '/inspections/' + created.id, 'PATCH', { version: returned.records[0].rowVersion, reason: '退回更正', record: { ...input, details: { ...input.details, remark: '成员更正稿' } } })).json()
    const resubmitted = await (await request(owner.client, owner.token, '/department-submissions/' + batch.id + '/resubmit', 'POST', { version: returned.rowVersion, records: [{ id: created.id, version: corrected.rowVersion }] })).json()
    check(resubmitted.revisionNo === 2, 'member resubmission did not preserve batch revision')
    await request(owner.client, owner.token, '/department-submissions/' + batch.id + '/confirm', 'POST', { version: resubmitted.rowVersion }, 403, 'FORBIDDEN_SCOPE')
    const approved = await (await request(reviewer.client, reviewer.token, '/department-submissions/' + batch.id + '/confirm', 'POST', { version: resubmitted.rowVersion })).json()
    check(approved.status === 'CONFIRMED', 'reviewer confirmation failed')
    await request(reviewer.client, reviewer.token, '/department-submissions/' + batch.id + '/return', 'POST', { version: approved.rowVersion, reason: 'invalid' }, 409, 'INVALID_SUBMISSION_STATE')
    await request(owner.client,owner.token,'/inspections/'+created.id,'DELETE',{version:approved.records[0].rowVersion,reason:'非法删除已确认记录'},409,'RECORD_NOT_EDITABLE')
    const workbench = await (await request(owner.client, owner.token, '/dashboard/workbench')).json()
    const weekly = workbench.departmentStatuses.find(item => item.departmentId === sports.id && item.inspectionTypeId === scope.inspectionTypeId)
    check(weekly && weekly.batchTotal >= 2 && weekly.batchConfirmed >= 2, 'weekly dashboard did not aggregate both member batches')
    cases.push('审核成员无录入权仍可退回/确认；成员更正和原批次重提；确认后非法回退被拒绝')
    cases.push('当前周工作台按部门和类型汇总两个已确认批次，与记录数分别展示')

    const dormitory = catalogs.departments.find(item => item.code === 'DORMITORY')
    check(dormitory, 'dormitory department missing')
    const studyOwner = await member('workflow-study-owner', study)
    const studyReviewer = await member('workflow-study-reviewer', study)
    const dormOwner = await member('workflow-dorm-owner', dormitory)
    const dormReviewer = await member('workflow-dorm-reviewer', dormitory)
    const additionalTypes = [
      { code: 'ATTENDANCE_RATE', department: study, owner: studyOwner, reviewer: studyReviewer, outsider: owner, details: { classId: fixture.class, studentId: fixture.studentA, courseId: fixture.course, classroomId: fixture.room, date: fixture.date, time: '08:00', period: '1-2', absenceHours: 2, remark: '角色矩阵' } },
      { code: 'UNATTENDED_CLASSROOM', department: study, owner: studyOwner, reviewer: studyReviewer, outsider: owner, details: { classId: fixture.class, studentId: fixture.studentB, courseId: fixture.course, classroomId: fixture.room, date: fixture.date, time: '09:00', period: '1-2', absenceHours: 2, remark: '角色矩阵' } },
      { code: 'LATE_RETURN', department: dormitory, owner: dormOwner, reviewer: dormReviewer, outsider: studyOwner, details: { classId: fixture.class, studentId: fixture.studentA, dormitoryId: fixture.dormA, date: fixture.date, time: '23:30', remark: '角色矩阵' } },
      { code: 'DORMITORY_HYGIENE', department: dormitory, owner: dormOwner, reviewer: dormReviewer, outsider: studyOwner, details: { dormitoryId: fixture.dormA, date: fixture.date, time: '09:00', result: 'QUALIFIED', responsibleClassIds: [fixture.class], responsibleStudentIds: [], remark: '角色矩阵' } },
    ]
    let confirmedLateRecord
    for (const type of additionalTypes) {
      const typeOptions = await (await request(admin, adminToken, '/inspections/context-options?typeCode=' + type.code + '&purpose=view&kind=semesters')).json()
      type.scope = { semesterId: scope.semesterId, weekId: scope.weekId, departmentId: type.department.id, inspectionTypeId: typeOptions.inspectionTypeId }
      await request(admin, adminToken, '/admin/users/' + type.reviewer.id + '/permission-rules', 'POST', { action: 'INSPECTION_REVIEW', effect: 'ALLOW', scope: { inspectionTypeId: type.scope.inspectionTypeId }, enabled: true, note: 'isolated reviewer' }, 201)
      await request(admin, adminToken, '/admin/users/' + type.reviewer.id + '/permission-rules', 'POST', { action: 'INSPECTION_ENTER', effect: 'DENY', scope: { inspectionTypeId: type.scope.inspectionTypeId }, enabled: true, note: 'review only' }, 201)
    }
    for (const person of [studyOwner, studyReviewer, dormOwner, dormReviewer]) person.token = (await (await request(person.client, '', '/auth/login', 'POST', { loginName: person.name, password: fixture.matrixPassword })).json()).accessToken
    for (const type of additionalTypes) {
      const recordInput = { ...type.scope, details: type.details }
      const forgedDepartment = type.department.id===study.id?dormitory.id:study.id
      await request(type.owner.client, type.owner.token, '/inspections', 'POST', {...recordInput,departmentId:forgedDepartment}, 403, 'FORBIDDEN_SCOPE')
      const record = await (await request(type.owner.client, type.owner.token, '/inspections', 'POST', {...recordInput,departmentId:''}, 201)).json()
      check(record.departmentId===type.department.id,type.code+' member department was not inherited')
      const submission = await (await request(type.owner.client, type.owner.token, '/department-submissions', 'POST', { scope: type.scope, records: [{ id: record.id, version: record.rowVersion }], idempotencyKey: 'workflow-matrix-' + type.code }, 201)).json()
      await request(type.owner.client, type.owner.token, '/department-submissions/' + submission.id + '/confirm', 'POST', { version: submission.rowVersion }, 403, 'FORBIDDEN_SCOPE')
      const outsiderList = await (await request(type.outsider.client, type.outsider.token, '/department-submissions?inspectionTypeId=' + type.scope.inspectionTypeId)).json()
      check(outsiderList.total === 0, type.code + ' cross-department list leaked')
      await request(type.outsider.client, type.outsider.token, '/department-submissions/' + submission.id, 'GET', undefined, 404, 'SUBMISSION_NOT_FOUND')
      await request(type.outsider.client, type.outsider.token, '/department-submissions/' + submission.id + '/return', 'POST', { version: submission.rowVersion, reason: 'denied' }, 403, 'FORBIDDEN_SCOPE')
      await request(type.reviewer.client, type.reviewer.token, '/inspections', 'POST', recordInput, 403, 'FORBIDDEN_SCOPE')
      const returnedBatch = await (await request(type.reviewer.client, type.reviewer.token, '/department-submissions/' + submission.id + '/return', 'POST', { version: submission.rowVersion, reason: '核对后可原样重提' })).json()
      const resubmittedBatch = await (await request(type.owner.client, type.owner.token, '/department-submissions/' + submission.id + '/resubmit', 'POST', { version: returnedBatch.rowVersion, records: [{ id: record.id, version: returnedBatch.records[0].rowVersion }] })).json()
      check(resubmittedBatch.revisionNo === 2, type.code + ' resubmit did not increment revision')
      const confirmedBatch = await (await request(type.reviewer.client, type.reviewer.token, '/department-submissions/' + submission.id + '/confirm', 'POST', { version: resubmittedBatch.rowVersion })).json()
      check(confirmedBatch.status === 'CONFIRMED', type.code + ' reviewer confirmation failed')
      await request(type.reviewer.client, type.reviewer.token, '/department-submissions/' + submission.id + '/return', 'POST', { version: confirmedBatch.rowVersion, reason: 'invalid' }, 409, 'INVALID_SUBMISSION_STATE')
      await request(type.owner.client,type.owner.token,'/inspections/'+record.id,'DELETE',{version:confirmedBatch.records[0].rowVersion,reason:'非法删除已确认记录'},409,'RECORD_NOT_EDITABLE')
      if (type.code==='LATE_RETURN') confirmedLateRecord={id:record.id,version:confirmedBatch.records[0].rowVersion}
      cases.push(type.code + ' 录入/审核分权、跨部门隐藏与拒绝、原样重提递增修订、确认后不可退回')
    }
    const lateType=additionalTypes.find(type=>type.code==='LATE_RETURN')
    check(lateType&&confirmedLateRecord,'late return confirmed record missing')
    const draftLate=await (await request(dormOwner.client,dormOwner.token,'/inspections','POST',{...lateType.scope,departmentId:'',details:{...lateType.details,time:'23:45',remark:'混合批量删除草稿'}},201)).json()
    await request(dormOwner.client,dormOwner.token,'/inspections/batch','POST',{action:'DELETE',items:[{id:draftLate.id,version:draftLate.rowVersion},confirmedLateRecord],reason:'混合状态不可删除'},409,'RECORD_NOT_EDITABLE')
    const retainedLate=await (await request(dormOwner.client,dormOwner.token,'/inspections/'+draftLate.id)).json()
    check(retainedLate.status==='DRAFT'&&retainedLate.rowVersion===draftLate.rowVersion,'mixed-state batch delete partially removed draft')
    cases.push('混合草稿和已确认晚归记录的批量删除整体拒绝，草稿仍保持原版本')
    const memberPage = await studyOwner.client.newPage()
    await memberPage.goto(origin+'/login')
    await memberPage.getByRole('textbox',{name:'账号',exact:true}).fill(studyOwner.name)
    await memberPage.getByLabel('密码',{exact:true}).fill(fixture.matrixPassword)
    await memberPage.getByRole('button',{name:/登\s*录/}).click()
    await memberPage.goto(origin+'/inspections/attendance')
    await memberPage.getByRole('heading',{name:'到课率检查',exact:true}).waitFor()
    await memberPage.getByRole('button',{name:'新增记录',exact:true}).click()
    const memberCreate=memberPage.getByRole('dialog',{name:'新增到课率记录'})
    await memberCreate.getByRole('combobox',{name:'录入归属部门'}).waitFor()
    check(await memberCreate.getByRole('combobox',{name:'录入归属部门'}).isDisabled(),'member can change the create department')
    await memberCreate.getByRole('button',{name:/取\s*消/}).click()
    await memberPage.getByRole('button',{name:'批量导入到课率'}).click()
    const memberImport=memberPage.getByRole('dialog',{name:'批量导入到课率'})
    const importDepartment=memberImport.getByRole('combobox',{name:'导入目标部门'})
    await importDepartment.waitFor()
    check(await importDepartment.isDisabled(),'member can change the import department')
    await memberImport.getByRole('button',{name:'Close',exact:true}).click()
    await memberImport.waitFor({state:'hidden'})
    const memberForm = async (memberBrowserPage,path) => {
      await memberBrowserPage.goto(origin+path)
      await memberBrowserPage.getByRole('button',{name:'新增记录',exact:true}).click()
      const form=memberBrowserPage.getByRole('dialog',{name:/新增.*记录/})
      const department=form.getByRole('combobox',{name:'录入归属部门'})
      await department.waitFor()
      check(await department.isDisabled(),path+' member can change the create department')
      await form.getByRole('button',{name:/取\s*消/}).click()
    }
    await memberForm(memberPage,'/inspections/unattended-classroom')
    const loginMember = async person => {
      const p=await person.client.newPage()
      await p.goto(origin+'/login')
      await p.getByRole('textbox',{name:'账号',exact:true}).fill(person.name)
      await p.getByLabel('密码',{exact:true}).fill(fixture.matrixPassword)
      await p.getByRole('button',{name:/登\s*录/}).click()
      return p
    }
    await memberForm(await loginMember(owner),'/inspections/morning-attendance')
    const dormPage=await loginMember(dormOwner)
    await memberForm(dormPage,'/inspections/late-return')
    await memberForm(dormPage,'/inspections/dormitory-hygiene')
    cases.push('五类真实成员新增页面的归属部门均只读')
    const reviewPage = async (person,path) => {
      const p=await loginMember(person)
      await p.goto(origin+path)
      await p.getByRole('button',{name:'检查提交批次'}).waitFor()
      check(await p.getByRole('button',{name:'新增记录',exact:true}).count()===0,path+' reviewer can create records')
      await p.getByRole('button',{name:'检查提交批次'}).click()
      const list=p.getByRole('dialog',{name:'检查提交批次'})
      await list.getByText('部门已确认').first().waitFor()
      await p.keyboard.press('Escape')
      await list.waitFor({state:'hidden'})
    }
    await reviewPage(reviewer,'/inspections/morning-attendance')
    await reviewPage(studyReviewer,'/inspections/attendance')
    await reviewPage(studyReviewer,'/inspections/unattended-classroom')
    await reviewPage(dormReviewer,'/inspections/late-return')
    await reviewPage(dormReviewer,'/inspections/dormitory-hygiene')
    cases.push('五类真实审核成员可查看已确认批次且无新增入口')
    const crossRolePages = [
      { type: additionalTypes[0], path: '/inspections/attendance', name: '到课率' },
      { type: additionalTypes[1], path: '/inspections/unattended-classroom', name: '无机课堂' },
      { type: additionalTypes[2], path: '/inspections/late-return', name: '晚归' },
      { type: additionalTypes[3], path: '/inspections/dormitory-hygiene', name: '宿舍卫生' },
    ]
    for (const { type, path, name } of crossRolePages) {
      const pendingDetails = {...type.details,remark:'页面审核链'}
      if (type.code==='ATTENDANCE_RATE'||type.code==='UNATTENDED_CLASSROOM') pendingDetails.time='10:00'
      if (type.code==='LATE_RETURN') pendingDetails.time='23:31'
      if (type.code==='DORMITORY_HYGIENE') pendingDetails.time='10:00'
      const pendingResponse = await type.owner.client.request.fetch(origin+'/api/v1/inspections',{method:'POST',data:{...type.scope,departmentId:'',details:pendingDetails},headers:{Authorization:'Bearer '+type.owner.token}})
      check(pendingResponse.status()===201,type.code+' pending UI record expected 201, got '+pendingResponse.status())
      const pendingRecord = await pendingResponse.json()
      const pendingBatch = await (await request(type.owner.client,type.owner.token,'/department-submissions','POST',{scope:type.scope,records:[{id:pendingRecord.id,version:pendingRecord.rowVersion}],idempotencyKey:type.code+'-reviewer-ui-chain'},201)).json()
      const reviewUI = await loginMember(type.reviewer)
      await reviewUI.goto(origin+path)
      await reviewUI.getByRole('button',{name:'检查提交批次'}).click()
      let reviewDrawer = reviewUI.getByRole('dialog',{name:'检查提交批次'})
      await reviewDrawer.getByRole('button',{name:'退回',exact:true}).click()
      let decision = reviewUI.getByRole('dialog',{name:'退回提交批次'})
      await decision.getByLabel('退回原因').fill('页面核对')
      await decision.getByRole('button',{name:'确认退回'}).click()
      await decision.waitFor({state:'hidden'})
      await reviewUI.keyboard.press('Escape')
      await reviewDrawer.waitFor({state:'hidden'})
      const ownerUI = type.owner===studyOwner?memberPage:dormPage
      await ownerUI.goto(origin+path)
      await ownerUI.getByRole('button',{name:'检查提交批次'}).click()
      const ownerDrawer = ownerUI.getByRole('dialog',{name:'检查提交批次'})
      await ownerDrawer.getByRole('button',{name:'重提',exact:true}).click()
      decision = ownerUI.getByRole('dialog',{name:'重新提交该批次？'})
      await decision.getByRole('button',{name:'重新提交'}).click()
      await decision.waitFor({state:'hidden'})
      await ownerUI.keyboard.press('Escape')
      await ownerDrawer.waitFor({state:'hidden'})
      await reviewUI.goto(origin+path)
      await reviewUI.getByRole('button',{name:'检查提交批次'}).click()
      reviewDrawer = reviewUI.getByRole('dialog',{name:'检查提交批次'})
      await reviewDrawer.getByRole('button',{name:'确认',exact:true}).click()
      decision = reviewUI.getByRole('dialog',{name:'确认该批次？'})
      await decision.getByRole('button',{name:'确认批次'}).click()
      await decision.waitFor({state:'hidden'})
      const finalBatch = await (await request(type.owner.client,type.owner.token,'/department-submissions/'+pendingBatch.id)).json()
      check(finalBatch.status==='CONFIRMED'&&finalBatch.revisionNo===2,name+' cross-role UI chain did not persist')
      cases.push(name+'真实成员与审核成员页面退回、原批次重提、确认和持久化')
    }
    const office = catalogs.departments.find(item => item.code==='OFFICE')
    check(office,'office department missing')
    const collegeViewer = await member('workflow-college-viewer',office)
    for (const type of [additionalTypes[0],additionalTypes[1],{scope},additionalTypes[2],additionalTypes[3]]) {
      const targetScope=type.scope
      await request(admin,adminToken,'/admin/users/'+collegeViewer.id+'/permission-rules','POST',{action:'INSPECTION_VIEW',effect:'ALLOW',scope:{inspectionTypeId:targetScope.inspectionTypeId},enabled:true,note:'isolated college viewer'},201)
    }
    collegeViewer.token=(await (await request(collegeViewer.client,'','/auth/login','POST',{loginName:collegeViewer.name,password:fixture.matrixPassword})).json()).accessToken
    const viewerUI=await loginMember(collegeViewer)
    for (const { type, path, name } of [{type:{scope},path:'/inspections/morning-attendance',name:'早锻炼'},...crossRolePages]) {
      const list=await (await request(collegeViewer.client,collegeViewer.token,'/inspections?inspectionTypeId='+type.scope.inspectionTypeId+'&departmentId='+type.scope.departmentId)).json()
      check(list.total>=1,name+' college viewer cannot see authorized records')
      await request(collegeViewer.client,collegeViewer.token,'/inspections','POST',{...type.scope,details:{}},403,'FORBIDDEN_SCOPE')
      await viewerUI.goto(origin+path)
      await viewerUI.getByRole('button',{name:'检查提交批次'}).waitFor()
      check(await viewerUI.getByRole('button',{name:'新增记录',exact:true}).count()===0,name+' college viewer can create records')
    }
    cases.push('学院查看者经五类明确授权可查看数据，不能录入，页面无新增入口')
    const systemAdmin = await member('workflow-system-admin',office)
    const assignments = await (await request(admin,adminToken,'/admin/users')).json()
    const systemAdminVersion = assignments.find(item => item.accountId === systemAdmin.id)?.version
    check(Number.isInteger(systemAdminVersion), 'system admin account version missing')
    await request(admin,adminToken,'/admin/users/'+systemAdmin.id,'PUT',{studentNo:systemAdmin.name,name:'Isolated system admin',systemRole:'ADMIN',version:systemAdminVersion})
    systemAdmin.token=(await (await request(systemAdmin.client,'','/auth/login','POST',{loginName:systemAdmin.name,password:fixture.matrixPassword})).json()).accessToken
    for (const { type, name } of [{type:{scope},name:'早锻炼'},...crossRolePages]) {
      const query='/inspections?inspectionTypeId='+type.scope.inspectionTypeId+'&departmentId='+type.scope.departmentId
      const superList=await (await request(admin,adminToken,query)).json()
      check(superList.total>=1,name+' super admin cannot view records')
      const adminList=await (await request(systemAdmin.client,systemAdmin.token,query)).json()
      check(adminList.total===superList.total,name+' office type mapping did not grant cross-department view')
      await request(systemAdmin.client,systemAdmin.token,'/inspections/'+superList.items[0].id,'GET',undefined,200)
    }
    cases.push('办公室主部门按类型跨部门查看五类记录；系统管理员查看权仍不授予录入权')
    const officeReviewer=await member('workflow-office-reviewer',office)
    const officeScope=additionalTypes[0].scope
    for(const action of ['INSPECTION_VIEW','INSPECTION_REVIEW'])await request(admin,adminToken,'/admin/users/'+officeReviewer.id+'/permission-rules','POST',{action,effect:'ALLOW',scope:{inspectionTypeId:officeScope.inspectionTypeId},enabled:true,note:'isolated office assigned scope'},201)
    officeReviewer.token=(await (await request(officeReviewer.client,'','/auth/login','POST',{loginName:officeReviewer.name,password:fixture.matrixPassword})).json()).accessToken
    const officeRecord=await (await request(studyOwner.client,studyOwner.token,'/inspections','POST',{...officeScope,departmentId:'',details:{...additionalTypes[0].details,time:'11:00',remark:'办公室授权审核'}},201)).json()
    const officeBatch=await (await request(studyOwner.client,studyOwner.token,'/department-submissions','POST',{scope:officeScope,records:[{id:officeRecord.id,version:officeRecord.rowVersion}],idempotencyKey:'office-assigned-review'},201)).json()
    const officeWorkbench=await (await request(officeReviewer.client,officeReviewer.token,'/dashboard/workbench')).json()
    check(officeWorkbench.departmentStatuses.some(item=>item.departmentId===study.id&&item.inspectionTypeId===officeScope.inspectionTypeId&&item.batchPending>=1),'assigned office reviewer cannot see authorized weekly status')
    check(!officeWorkbench.departmentStatuses.some(item=>item.departmentId===sports.id&&item.inspectionTypeId===additionalTypes[0].scope.inspectionTypeId),'workbench exposed an unmapped department/type combination')
    await request(officeReviewer.client,officeReviewer.token,'/inspections','POST',{...officeScope,details:{}},403,'FORBIDDEN_SCOPE')
    const officeUI=await loginMember(officeReviewer)
    await officeUI.goto(origin+'/inspections/attendance')
    await officeUI.getByRole('button',{name:'检查提交批次'}).click()
    const officeDrawer=officeUI.getByRole('dialog',{name:'检查提交批次'})
    await officeDrawer.getByRole('button',{name:'确认',exact:true}).click()
    const officeDecision=officeUI.getByRole('dialog',{name:'确认该批次？'})
    await officeDecision.waitFor()
    check(await officeDecision.evaluate(node=>node.contains(document.activeElement)),'confirmation dialog did not receive keyboard focus')
    await officeDecision.getByRole('button',{name:'确认批次'}).click()
    await officeDecision.waitFor({state:'hidden'})
    const officeFinal=await (await request(officeReviewer.client,officeReviewer.token,'/department-submissions/'+officeBatch.id)).json()
    check(officeFinal.status==='CONFIRMED','assigned office reviewer did not confirm scoped batch')
    cases.push('办公室指定授权仅显示学习部到课率周状态，可审核该批次且无录入权；确认弹窗接收键盘焦点')

    const collegeDepartmentId='00000000-0000-0000-0000-000000000000'
    const collegeScope={...officeScope,departmentId:collegeDepartmentId}
    await request(systemAdmin.client,systemAdmin.token,'/inspections','POST',{...collegeScope,details:{...additionalTypes[0].details,time:'12:00',remark:'校级独立批次'}},403,'FORBIDDEN_SCOPE')
    await request(admin,adminToken,'/admin/users/'+systemAdmin.id+'/permission-rules','POST',{action:'INSPECTION_ENTER',effect:'ALLOW',scope:{inspectionTypeId:officeScope.inspectionTypeId},enabled:true,note:'isolated college entry'},201)
    await request(admin,adminToken,'/admin/users/'+systemAdmin.id+'/permission-rules','POST',{action:'INSPECTION_VIEW',effect:'ALLOW',scope:{inspectionTypeId:officeScope.inspectionTypeId},enabled:true,note:'isolated college view'},201)
    systemAdmin.token=(await (await request(systemAdmin.client,'','/auth/login','POST',{loginName:systemAdmin.name,password:fixture.matrixPassword})).json()).accessToken
    const collegeRecord=await (await request(systemAdmin.client,systemAdmin.token,'/inspections','POST',{...collegeScope,details:{...additionalTypes[0].details,time:'12:00',remark:'校级独立批次'}},201)).json()
    check(collegeRecord.departmentId==='','college record did not persist null department')
    const collegeList=await (await request(systemAdmin.client,systemAdmin.token,'/inspections?inspectionTypeId='+officeScope.inspectionTypeId+'&departmentId='+collegeDepartmentId)).json()
    check(collegeList.items.some(item=>item.id===collegeRecord.id),'college-scoped list omitted record')
    await request(officeReviewer.client,officeReviewer.token,'/inspections/'+collegeRecord.id,'GET',undefined,200)
    const collegeBatch=await (await request(systemAdmin.client,systemAdmin.token,'/department-submissions','POST',{scope:collegeScope,records:[{id:collegeRecord.id,version:collegeRecord.rowVersion}],idempotencyKey:'college-independent-batch'},201)).json()
    check(collegeBatch.scope.departmentId===''&&collegeBatch.departmentName==='校级','college submission inherited a department')
    await request(systemAdmin.client,systemAdmin.token,'/department-submissions/'+collegeBatch.id+'/confirm','POST',{version:collegeBatch.rowVersion},403,'FORBIDDEN_SCOPE')
    await request(admin,adminToken,'/admin/users/'+systemAdmin.id+'/permission-rules','POST',{action:'INSPECTION_REVIEW',effect:'ALLOW',scope:{inspectionTypeId:officeScope.inspectionTypeId},enabled:true,note:'isolated college review'},201)
    systemAdmin.token=(await (await request(systemAdmin.client,'','/auth/login','POST',{loginName:systemAdmin.name,password:fixture.matrixPassword})).json()).accessToken
    const collegeConfirmed=await (await request(systemAdmin.client,systemAdmin.token,'/department-submissions/'+collegeBatch.id+'/confirm','POST',{version:collegeBatch.rowVersion})).json()
    check(collegeConfirmed.status==='CONFIRMED','explicitly authorized admin could not confirm college batch')
    const collegeWorkbench=await (await request(systemAdmin.client,systemAdmin.token,'/dashboard/workbench')).json()
    check(collegeWorkbench.departmentStatuses.some(item=>item.departmentId===''&&item.inspectionTypeId===officeScope.inspectionTypeId&&item.batchConfirmed>=1),'college weekly summary omitted confirmed batch')
    cases.push('普通管理员校级录入与审核分权；用户类型授权跨部门及校级生效，周汇总独立列示')
    await page.goto(origin+'/inspections/morning-attendance')
    await page.getByRole('button',{name:'新增记录',exact:true}).click()
    const collegeForm=page.getByRole('dialog',{name:'新增早锻炼/早签记录'})
    await select(collegeForm,'录入学期','浏览器回归学期')
    await select(collegeForm,'录入周次',fixture.weekStart)
    await select(collegeForm,'目标部门','校级')
    await select(collegeForm,'学生班级','E2E class')
    cases.push('真实页面手工新增可选择校级归属并加载班级选项')
  } finally { for (const client of contexts) await client.close() }
  check(reportRequests.length === 0, 'phase-two page requested stage-three report API')
  cases.push('阶段二工作台无通报待办/伪进度占位，页面未请求阶段三通报接口')
  check(browserErrors.length === 0, 'Unexpected browser page error: ' + browserErrors.join('; '))
  return { cases, browserErrors, faultInjection: ['提交已落库但客户端收到 503，保留原版本和幂等键重试'], browserVersion: await page.evaluate(() => navigator.userAgent) }
}
