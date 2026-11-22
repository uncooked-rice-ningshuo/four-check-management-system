import { expect } from '@playwright/test'
import { test } from './fixture'

test.skip(!process.env.TEMPLATE_GUI_PASSWORD,'使用 document-templates 一次性数据库')
test.use({ video:'on', actionTimeout:10000, launchOptions:{channel:'chrome',slowMo:300} })
test('逐档模板在真实服务中预览、修改并保存',async({page})=>{
  const login = await page.request.post('/api/v1/auth/login',{data:{loginName:'browser-admin',password:process.env.TEMPLATE_GUI_PASSWORD}})
  expect(login.status()).toBe(200)
  await page.goto('/discipline/documents')
  const row = page.getByRole('row').filter({has:page.getByRole('rowheader',{name:'FIRST',exact:true})})
  await row.getByRole('button',{name:'预览',exact:true}).click()
  const sample = page.getByLabel('处分文件示例预览')
  await expect(sample.getByLabel('模板正文预览')).toContainText('陈小禾')
  await sample.getByRole('button',{name:'放大预览'}).click()
  await sample.getByRole('button',{name:'适应宽度'}).click()
  await page.screenshot({path:test.info().outputPath('active-template-example.png')})
  await page.getByRole('dialog').getByRole('button',{name:/close|关闭/i}).first().click()
  await row.getByRole('button',{name:'编辑模板'}).click()
  const body = page.getByLabel('文件模板正文')
  await body.locator('p').click()
  await page.keyboard.press('End')
  await page.keyboard.insertText('归档核对')
  await page.getByRole('button',{name:'保存并启用'}).click()
  await page.getByRole('button',{name:/^启\s*用$/}).click()
  await expect(row).toContainText('第 2 版')
  await page.reload()
  await row.getByRole('button',{name:'预览',exact:true}).click()
  await expect(sample).toContainText('归档核对')
  await expect(sample).toContainText('陈小禾')
})

test('归档按真实记录预览并下载选定版本',async({page})=>{
  const login = await page.request.post('/api/v1/auth/login',{data:{loginName:'browser-admin',password:process.env.TEMPLATE_GUI_PASSWORD}})
  expect(login.status()).toBe(200)
  await page.goto('/discipline/archive')
  await page.getByRole('button',{name:'生成文书'}).first().click()
  const drawer=page.getByRole('dialog')
  await expect(drawer.getByLabel('模板正文预览')).toContainText('虚构学生')
  const download=drawer.getByRole('button',{name:'确认并下载 Word'})
  await drawer.getByLabel('发文字号').fill('示例院发〔2026〕1号')
  await expect(download).toBeDisabled()
  await drawer.getByRole('button',{name:'更新预览'}).click()
  await expect(download).toBeEnabled()
  const file=page.waitForEvent('download')
  await download.click()
  await (await file).saveAs(test.info().outputPath('归档决定书.docx'))
})

test('模板编辑、归档清单导出、处分文书出文分别鉴权',async({request})=>{
  const login=async(loginName:string)=>{
    const response=await request.post('/api/v1/auth/login',{data:{loginName,password:process.env.TEMPLATE_GUI_PASSWORD}})
    expect(response.status()).toBe(200)
    return {Authorization:'Bearer '+(await response.json()).accessToken}
  }
  const admin=await login('browser-admin')
  const list=await request.post('/api/v1/discipline/archive/search',{headers:admin,data:{status:'APPROVED',page:1,pageSize:1}})
  const id=(await list.json()).items[0].id
  for(const kind of ['edit','export','excel']){
    const headers=await login('doc-'+kind)
    expect((await request.get('/api/v1/discipline/document-templates',{headers})).status()).toBe(kind==='edit'?200:403)
    for(const suffix of ['/preview','']){
      const result=await request.post('/api/v1/discipline/archive/'+id+'/document'+suffix,{headers,data:{}})
      expect(result.status()).toBe(kind==='export'?200:403)
    }
  }
  const wrongEdition=await request.post('/api/v1/discipline/archive/'+id+'/document',{headers:admin,data:{templateId:'f418f483-2502-4d81-b766-9595964e9767'}})
  expect(wrongEdition.status()).toBe(404)
})
