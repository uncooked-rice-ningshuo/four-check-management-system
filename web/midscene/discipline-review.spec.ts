import { expect } from '@playwright/test'
import { test } from './fixture'

test.use({ video:'on', launchOptions:{channel:'chrome',slowMo:350} })
test('老师延迟后再次确认处分',async({page})=>{
  let item={id:'sample',canProcess:true,studentNumber:'演示001',studentName:'演示学生',gradeName:'2024级',majorName:'计算机科学与技术',groupName:'课堂缺勤',unit:'学时',className:'演示班',tier:'警告',amount:4,status:'PENDING',version:1,reason:'',dueAt:'2026-11-24T10:00:00Z'}
  const decisions:string[]=[]
  await page.route('**/api/v1/**',async route=>{
    const path=new URL(route.request().url()).pathname
    const json=(body:unknown)=>route.fulfill({json:body})
    if(path.endsWith('/auth/refresh'))return json({accessToken:'fixture'})
    if(path.endsWith('/auth/me'))return json({id:'demo',name:'演示老师',systemRole:'MEMBER',status:'ACTIVE'})
    if(path.endsWith('/permissions/effective'))return json({permissions:[{action:'DISCIPLINE_CASE_CONFIRM',effect:'ALLOW',scope:{}}]})
    if(path.endsWith('/cases/search'))return json({items:item.status==='APPROVED'?[]:[item],total:item.status==='APPROVED'?0:1})
    if(path.endsWith('/sample/review')){
      const input=route.request().postDataJSON()
      expect(input.version).toBe(item.version)
      decisions.push(input.decision)
      item={...item,status:input.decision,reason:input.reason,version:item.version+1}
      return route.fulfill({status:204})
    }
    return route.fulfill({status:403,json:{code:'FORBIDDEN'}})
  })
  await page.goto('/')
  await page.getByText('处分管理',{exact:true}).click()
  await page.getByRole('link',{name:'待处分',exact:true}).click()
  await expect(page).toHaveURL(/\/discipline\/review$/)
  await expect(page.getByRole('heading',{name:'待处分',exact:true})).toBeVisible()
  const row = page.getByRole('row').filter({hasText:'演示001'})
  for (const text of ['演示学生','2024级','计算机科学与技术','演示班','课堂缺勤','4 学时','警告']) {
    await expect(row).toContainText(text)
  }
  await page.screenshot({path:test.info().outputPath('review-queue.png')})
  await page.getByRole('button',{name:'延迟处理'}).click()
  const dialog=page.getByRole('dialog')
  const ok=dialog.getByRole('button',{name:/确\s*定/})
  await ok.click()
  await expect(dialog.getByText('请填写延迟原因')).toBeVisible()
  expect(decisions).toEqual([])
  await dialog.getByLabel('延迟原因').fill('需要核对本周记录')
  await page.waitForTimeout(1400)
  await ok.click()
  await expect(page.getByRole('row').filter({hasText:'演示001'})).toContainText('已延迟')
  await page.getByRole('button',{name:'确认处分'}).click()
  await page.waitForTimeout(1400)
  await ok.click()
  await expect(page.getByText('演示001',{exact:true})).toHaveCount(0)
  expect(decisions).toEqual(['DEFERRED','APPROVED'])
})

test('超管从处分管理进入待处分并确认事项', async ({ page }) => {
  await page.route('**/api/v1/**', route => {
    const path = new URL(route.request().url()).pathname
    if (path.endsWith('/auth/refresh')) return route.fulfill({json:{accessToken:'fixture'}})
    if (path.endsWith('/auth/me')) return route.fulfill({json:{id:'admin',name:'演示管理员',isSuperAdmin:true,systemRole:'SUPER_ADMIN',status:'ACTIVE'}})
    if (path.endsWith('/permissions/effective')) return route.fulfill({json:{permissions:[]}})
    if (path.endsWith('/demo/review')) { expect(route.request().postDataJSON().decision).toBe('APPROVED'); return route.fulfill({status:204}) }
    if (path.endsWith('/cases/search')) return route.fulfill({json:{items:[{id:'demo',canProcess:true,studentNumber:'只读示例',className:'演示班',tier:'警告',amount:4,status:'PENDING',version:1,reason:null,dueAt:'2026-11-24T08:00:00Z'}],total:1}})
    return route.fulfill({status:403,json:{code:'FORBIDDEN'}})
  })
  await page.goto('/')
  await page.getByText('处分管理',{exact:true}).click()
  await page.getByRole('link',{name:'待处分',exact:true}).click()
  await expect(page.getByText('只读示例')).toBeVisible()
  await expect(page.getByRole('button',{name:'延迟处理'})).toBeVisible()
  await page.getByRole('button',{name:'确认处分'}).click()
  await page.getByRole('dialog').getByRole('button',{name:/确\s*定/}).click()
  await expect(page.getByRole('dialog')).toHaveCount(0)
})

test('同一待处分列表区分可处理和只读记录', async ({page}) => {
  const rows = [true,false].map((canProcess,i) => ({
    id:String(i),studentNumber:'演示00'+i,className:i?'2024级演示班':'2025级演示班',
    canProcess,tier:'警告',amount:4,status:'PENDING',version:1,dueAt:'2026-11-24T08:00:00Z',
  }))
  await page.route('**/api/v1/**',route=>{
    const path=new URL(route.request().url()).pathname
    if(path.endsWith('/auth/refresh'))return route.fulfill({json:{accessToken:'fixture'}})
    if(path.endsWith('/auth/me'))return route.fulfill({json:{id:'demo',name:'演示老师',systemRole:'MEMBER',status:'ACTIVE'}})
    if(path.endsWith('/permissions/effective'))return route.fulfill({json:{permissions:['DISCIPLINE_CASE_VIEW','DISCIPLINE_CASE_CONFIRM'].map(action=>({action,effect:'ALLOW',scope:{}}))}})
    if(path.endsWith('/cases/search'))return route.fulfill({json:{items:rows,total:2}})
    return route.fulfill({status:403,json:{code:'FORBIDDEN'}})
  })
  await page.goto('/discipline/review')
  const actionable=page.getByRole('row').filter({hasText:'2025级演示班'})
  const readonly=page.getByRole('row').filter({hasText:'2024级演示班'})
  await expect(actionable.getByRole('button',{name:'确认处分'})).toBeVisible()
  await expect(readonly).toContainText('只读')
  await expect(readonly.getByRole('button')).toHaveCount(0)
  const color=(cell:HTMLElement|SVGElement)=>getComputedStyle(cell).backgroundColor
  expect(await actionable.getByRole('cell').first().evaluate(color)).not.toBe(await readonly.getByRole('cell').first().evaluate(color))
  await page.waitForTimeout(1800)
  await actionable.getByRole('button',{name:'延迟处理'}).click()
  await expect(page.getByRole('dialog')).toBeVisible()
  await page.waitForTimeout(1500)
})
