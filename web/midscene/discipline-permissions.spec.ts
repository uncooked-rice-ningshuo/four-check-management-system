import { expect } from '@playwright/test'
import { test } from './fixture'

test.use({ video: 'on', launchOptions: { channel: 'chrome', slowMo: 350 } })

test('处分管理沿用部门功能与角色权限抽屉', async ({ page }) => {
  const department = { id: 'office', code: 'OFFICE', name: '学生会办公室', enabled: true }
  const policies = ['MEMBER', 'MINISTER', 'VICE_MINISTER', 'PRESIDIUM', 'TEACHER'].map(roleCode =>
    ({ roleCode, validityYears: 1, operationRanges: [], disciplineActions: [] }))
  await page.route('**/api/v1/**', route => {
    const path = new URL(route.request().url()).pathname
    const reply = (json: unknown) => route.fulfill({ json })
    if (path.endsWith('/auth/refresh')) return reply({ accessToken: 'fixture' })
    if (path.endsWith('/auth/me')) return reply({ id:'fixture', name:'演示管理员', systemRole:'SUPER_ADMIN', isSuperAdmin:true, status:'ACTIVE' })
    if (path.endsWith('/catalogs')) return reply({ departments:[department], inspectionTypes:[], organizationRoles:[] })
    if (path.endsWith('/default-permissions')) return reply({ departmentId:'office', version:1, scopes:[], inspectionRoles:[], reportRoles:[],
      reportMaintenance:{state:'unassigned'}, reportScope:{enabled:false}, disciplineScope:[], disciplineEnabled:false, rolePolicies:policies })
    if (path.endsWith('/effective')) return reply({ permissions:[] })
    if (path.endsWith('/users')) return reply([])
    return route.fulfill({ status:403, json:{code:'FORBIDDEN'} })
  })
  await page.goto('/admin/organization')
  const section = page.getByRole('region', { name:'处分管理权限表' })
  await expect(section.getByRole('button', {name:'角色权限'})).toBeDisabled()
  await section.getByRole('switch').click()
  await section.getByRole('button', {name:'部门功能'}).click()
  const drawer = page.getByRole('dialog')
  await drawer.getByRole('switch', {name:'学生会办公室 处分管理 查看处分归档'}).click()
  await page.waitForTimeout(1200)
  await drawer.getByRole('button', {name:'Close'}).click()
  await section.getByRole('button', {name:'角色权限'}).click()
  await expect(drawer.getByRole('switch', {name:'委员默认导出处分归档'})).toBeDisabled()
  await drawer.getByRole('switch', {name:'委员默认查看处分归档'}).click()
  await page.waitForTimeout(1500)
  await drawer.getByRole('button', {name:'Close'}).click()
  await expect(section.getByText('查看处分归档', {exact:true})).toHaveCount(2)
})

test('真实服务保存部门功能与角色权限后重载', async ({ page, request }) => {
  const password = process.env.MATRIX_TEST_PASSWORD
  test.skip(!password, '需要一次性数据库')
  const loginName = 'browser-admin'
  const login = await request.post('/api/v1/auth/login', {data:{loginName,password}})
  expect(login.status()).toBe(200)
  const headers = {Authorization:'Bearer ' + (await login.json()).accessToken}
  const newPassword = password + 'A'
  expect((await request.post('/api/v1/auth/change-password',{headers,data:{currentPassword:password,newPassword}})).status()).toBe(204)
  expect((await page.request.post('/api/v1/auth/login',{data:{loginName,password:newPassword}})).status()).toBe(200)
  await page.goto('/admin/organization')
  const section = page.getByRole('region',{name:'处分管理权限表'})
  const titleStyle = (element: Element) => {
    const style = getComputedStyle(element)
    return [style.fontSize,style.lineHeight,style.marginTop,style.marginBottom,style.color]
  }
  expect(await page.locator('#department-discipline-title').evaluate(titleStyle)).toEqual(await page.locator('#department-report-title').evaluate(titleStyle))
  await expect(page.getByText('按部门设置处分管理职责和各角色的默认权限')).toBeVisible()
  await page.getByRole('combobox',{name:'选择配置部门'}).click()
  await page.getByText('学习部',{exact:true}).last().click()
  await expect(section.getByRole('switch',{name:'学习部 承担处分管理'})).not.toBeChecked()
  await expect(section.getByRole('button',{name:'角色权限'})).toBeDisabled()
  await section.getByRole('switch').click()
  await section.getByRole('button',{name:'部门功能'}).click()
  const drawer = page.getByRole('dialog')
  await drawer.getByRole('switch',{name:'学习部 处分管理 查看处分归档'}).click()
  await drawer.getByRole('switch',{name:'学习部 处分管理 查看待处分'}).click()
  await drawer.getByRole('button',{name:'Close'}).click()
  await section.getByRole('button',{name:'角色权限'}).click()
  await expect(drawer.getByRole('switch',{name:'委员默认导出处分归档'})).toBeDisabled()
  await drawer.getByRole('switch',{name:'委员默认查看处分归档'}).click()
  await expect(drawer.getByRole('columnheader')).toHaveText(['业务动作','部门','委员','部长','副部长','主席团','老师'])
  await expect(drawer.locator('.department-defaults-footnote')).toBeVisible()
  await drawer.getByRole('switch',{name:'委员默认查看待处分'}).click()
  await drawer.getByRole('button',{name:'委员查看待处分范围'}).click()
  const range = page.getByRole('dialog',{name:/查看待处分 · 查看范围/})
  await expect(range.getByText('可查看：不限')).toBeVisible()
  await range.getByRole('button',{name:'添加一组范围'}).click()
  await range.getByRole('combobox',{name:'第1组年级'}).click()
  await page.getByText('2026级',{exact:true}).last().click()
  await expect(range.getByText('可查看：2026级')).toBeVisible()
  await range.getByRole('button',{name:'应用范围'}).click()

  await page.waitForTimeout(1500)
  await drawer.getByRole('button',{name:'Close'}).click()
  await page.getByRole('button',{name:/保存修改/}).click()
  const saved = page.waitForResponse(r=>r.url().endsWith('/default-permissions') && r.request().method()==='PUT')
  await page.getByRole('button',{name:'保存全部修改'}).click()
  expect((await saved).status()).toBe(200)
  await page.reload()
  await page.getByRole('combobox',{name:'选择配置部门'}).click()
  await page.getByText('学习部',{exact:true}).last().click()
  await expect(section.getByText('查看待处分、查看处分归档',{exact:true})).toHaveCount(2)
  const departmentId = '10000000-0000-0000-0000-000000000002'
  const account = '/api/v1/admin/users/' + process.env.MATRIX_MEMBER_ID
  const adminLogin = await request.post('/api/v1/auth/login',{data:{loginName,password:newPassword}})
  const adminHeaders = {Authorization: 'Bearer ' + (await adminLogin.json()).accessToken}
  const assigned = await request.put(account + '/assignment',{headers:adminHeaders,data:{version:1,departmentId,departmentIds:[departmentId],organizationRoleId:'20000000-0000-0000-0000-000000000004'}})
  expect(assigned.status()).toBe(204)
  async function memberPermissions() {
    const login = await request.post('/api/v1/auth/login',{data:{loginName:'matrix-member',password}})
    expect(login.status()).toBe(200)
    const token = (await login.json()).accessToken
    const effective = await request.get('/api/v1/permissions/effective',{headers:{Authorization:'Bearer ' + token}})
    expect(effective.status()).toBe(200)
    return (await effective.json()).permissions as {action:string;ranges?:{gradeId?:string}[]}[]
  }
  const grants = await memberPermissions()
  expect(grants.find(p=>p.action==='DISCIPLINE_CASE_VIEW')?.ranges).toEqual([{gradeId:process.env.MATRIX_GRADE_ID}])
  const override = await request.post(account + '/permission-rules',{headers:adminHeaders,data:{
    action:'DISCIPLINE_CASE_VIEW',effect:'ALLOW',mode:'OVERRIDE',sourceDepartmentId:departmentId,scope:{},ranges:[],
    startsAt:'2026-01-01T00:00:00Z',note:'验证个人查看不限范围',enabled:true,
  }})
  expect(override.status()).toBe(201)
  const personal = (await memberPermissions()).find(p=>p.action==='DISCIPLINE_CASE_VIEW')
  expect(personal).toBeDefined()
  expect(personal?.ranges ?? []).toEqual([])
  await expect(page.getByRole('heading',{name:'周通报'})).toBeVisible()
  for (const [path, title] of [['config','处分配置'],['ledger','学期违纪台账'],['review','待处分'],['archive','处分归档']]) {
    await page.goto('/discipline/' + path)
    await expect(page.getByRole('heading',{name:title,exact:true,level:2})).toBeVisible()
    await page.screenshot({path:test.info().outputPath(path+'-desktop.png')})
    await page.setViewportSize({width:390,height:844})
    await expect.poll(() => page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(390)
    await page.screenshot({path:test.info().outputPath(path+'-mobile.png')})
    await page.setViewportSize({width:1440,height:900})
  }
  expect((await request.post('/api/v1/discipline/archive/export',{data:{page:1,pageSize:20}})).status()).toBe(401)
})
