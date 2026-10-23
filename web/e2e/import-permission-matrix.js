// Real API role matrix. Only disposable accounts and batches from the runner.
async (page) => {
  const fixture = __FIXTURE__
  const base = page.url().split('/').slice(0,3).join('/') + '/api/v1'
  const contexts = []
  const cases = []
  const check = (ok, message) => { if (!ok) throw new Error(message) }
  const request = async (context, token, path, method = 'GET', data, expected = 200, code) => {
    const response = await context.request.fetch(base + path, {method, data, headers: token ? {Authorization:'Bearer ' + token} : {}})
    check(response.status() === expected, `${method} ${path.split('?')[0]} expected ${expected}, got ${response.status()}`)
    if (code) check((await response.json()).code === code, 'Wrong stable error code for ' + path.split('?')[0])
    return response
  }
  const context = async () => {
    const result = await page.context().browser().newContext()
    contexts.push(result)
    return result
  }
  try {
    const admin = await context()
    const adminToken = (await (await request(admin, '', '/auth/login', 'POST', {loginName:'browser-admin',password:fixture.authPassword})).json()).accessToken
    const catalogs = await (await request(admin, '', '/public/registration-options')).json()
    const study = catalogs.departments.find(d => d.code === 'STUDY')
    const sports = catalogs.departments.find(d => d.code === 'SPORTS')
    const memberRole = catalogs.organizationRoles.find(r => r.code === 'MEMBER')
    check(study && sports && memberRole, 'Required catalog fixture is missing')
    const batch = await (await request(admin, adminToken, '/imports/' + fixture.permissionBatchId)).json()
    const scope = {semesterId:batch.semesterId, weekId:batch.weekId, inspectionTypeId:batch.inspectionTypeId, departmentId:study.id}
    const query = value => Object.entries(value).map(([k,v]) => encodeURIComponent(k) + '=' + encodeURIComponent(v)).join('&')
    const createMember = async (name, department) => {
      const client = await context()
      const account = await (await request(client, '', '/auth/register', 'POST', {studentNo:name,name, password:fixture.matrixPassword,requestedDepartmentId:department.id,requestedOrgRoleId:memberRole.id}, 201)).json()
      await request(admin, adminToken, '/admin/registration-requests/' + account.id + '/review', 'POST', {decision:'APPROVE',reason:'isolated role matrix'})
      const token = (await (await request(client, '', '/auth/login','POST',{loginName:name,password:fixture.matrixPassword})).json()).accessToken
      return {client,token,id:account.id}
    }
    const owner = await createMember('matrix-study', study)
    const outsider = await createMember('matrix-sports', sports)
    const rule = async (member, action, effect, departmentId) => (await (await request(admin, adminToken, '/admin/users/' + member.id + '/permission-rules', 'POST', {action,effect,scope:effect === 'DENY' ? {departmentId} : {departmentId,inspectionTypeId:scope.inspectionTypeId},enabled:true,note:'isolated role matrix'},201)).json())
    await rule(outsider, 'INSPECTION_VIEW','ALLOW',sports.id)
    await rule(outsider, 'INSPECTION_ENTER','ALLOW',sports.id)
    outsider.token = (await (await request(outsider.client, '', '/auth/login','POST',{loginName:'matrix-sports',password:fixture.matrixPassword})).json()).accessToken
    const listPath = '/inspections/attendance?' + query({...scope,departmentId:''})
    const ownRecords = await (await request(owner.client, owner.token, listPath)).json()
    const otherRecords = await (await request(outsider.client, outsider.token, listPath)).json()
    check(ownRecords.total === 4 && ownRecords.items.length === 4, 'Own department records missing')
    check(otherRecords.total === 0 && otherRecords.items.length === 0, 'Unfiltered list leaked another department')
    await request(outsider.client, outsider.token, '/inspections/attendance?' + query(scope), 'GET', undefined, 403, 'FORBIDDEN_SCOPE')
    cases.push('部门留空由服务端限制：学习部四条，体育部零条；显式越权部门返回 403')
    const templatePath = '/imports/attendance/template?' + query(scope)
    const download = await request(owner.client, owner.token, templatePath)
    check(download.headers()['content-type'].includes('spreadsheetml'), 'Template is not an XLSX response')
    check((await download.body()).length > 0, 'Template download is empty')
    await request(outsider.client, outsider.token, templatePath, 'GET', undefined, 403,'FORBIDDEN_SCOPE')
    await request(outsider.client, outsider.token, '/imports/' + batch.id, 'GET', undefined, 404,'IMPORT_BATCH_NOT_FOUND')
    await request(outsider.client, outsider.token, '/imports/' + batch.id + '/rows', 'GET', undefined, 404,'IMPORT_BATCH_NOT_FOUND')
    await request(outsider.client, outsider.token, '/imports/' + batch.id + '/confirm', 'POST',{version:batch.version},404,'IMPORT_BATCH_NOT_FOUND')
    cases.push('跨部门模板下载拒绝；批次详情、预览行和确认隐藏资源存在性')
    await request(owner.client, owner.token, '/imports/' + batch.id)
    await rule(owner, 'INSPECTION_VIEW','DENY',study.id)
    await rule(owner, 'INSPECTION_ENTER','DENY',study.id)
    await request(owner.client, owner.token, templatePath, 'GET', undefined,401,'UNAUTHORIZED')
    owner.token = (await (await request(owner.client, '', '/auth/login','POST',{loginName:'matrix-study',password:fixture.matrixPassword})).json()).accessToken
    await request(owner.client, owner.token, templatePath, 'GET', undefined,403,'FORBIDDEN_SCOPE')
    await request(owner.client, owner.token, listPath, 'GET', undefined,403,'FORBIDDEN_SCOPE')
    await request(owner.client, owner.token, '/imports/' + batch.id + '/confirm', 'POST',{version:batch.version},404,'IMPORT_BATCH_NOT_FOUND')
    cases.push('权限变更使旧 Access Token 失效；重新登录后个人 DENY 覆盖部门默认授权并拒绝列表、下载和确认')
    const memberPage = await owner.client.newPage()
    await memberPage.goto(base.replace('/api/v1','') + '/')
    await memberPage.getByRole('heading',{name:'工作台',exact:true}).waitFor()
    check(await memberPage.getByRole('link',{name:'到课率检查',exact:true}).count() === 0, 'Denied navigation is visible')
    await memberPage.goto(base.replace('/api/v1','') + '/inspections/attendance')
    await memberPage.getByText('无权访问该业务',{exact:true}).waitFor()
    cases.push('单独 DENY 的真实浏览器导航隐藏且直达路由受限')
    await request(admin, '', templatePath, 'GET',undefined,401,'UNAUTHORIZED')
    cases.push('模板下载未携带 Access Token 返回 401')
    return {cases, batchId:batch.id, browserVersion:page.context().browser().version()}
  } finally {
    for (const client of contexts.reverse()) await client.close()
  }
}
