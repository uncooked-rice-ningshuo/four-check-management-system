import { test, expect } from '@playwright/test'

test('处分模板独立标记两行，保存后切换留白预览',async({page})=>{
  const paragraph=(text:string)=>({type:'paragraph',attrs:{style:'body',textAlign:'center'},content:[{type:'text',text}]})
  const item={ruleId:'demo',groupKey:'demo',tierCode:'warning',name:'示例',revision:1,document:{type:'doc',schemaVersion:2,content:['示例学院','示例文号','红线','决定'].map(paragraph)}}
  await page.route('**/api/v1/**',async route=>{
    const path=new URL(route.request().url()).pathname
    if(path.endsWith('/auth/refresh'))return route.fulfill({json:{accessToken:'fixture'}})
    if(path.endsWith('/auth/me'))return route.fulfill({json:{id:'demo',name:'演示管理员',status:'ACTIVE',systemRole:'SUPER_ADMIN',isSuperAdmin:true}})
    if(path.endsWith('/discipline/document-templates')){
      if(route.request().method()==='PUT')Object.assign(item,route.request().postDataJSON(),{revision:2})
      return route.fulfill({json:{items:[item]}})
    }
    return route.fulfill({json:{items:[],permissions:[]}})
  })
  await page.goto('/discipline/documents')
  await page.getByRole('row').filter({hasText:'示例'}).getByRole('button',{name:/编辑/}).click()
  const editor=page.getByLabel('文件模板正文')
  for(const text of ['示例学院','红线']){
    await editor.getByText(text,{exact:true}).click()
    await page.getByRole('button',{name:'红头行',exact:true}).click()
  }
  await expect(editor.locator('[data-letterhead]')).toHaveCount(2)
  await page.getByRole('button',{name:'保存并启用'}).click()
  await page.getByRole('button',{name:/^启\s*用$/}).click()
  await page.getByRole('button',{name:'预览',exact:true}).click()
  const sample=page.getByRole('region',{name:'处分文件示例预览'})
  const stage=sample.getByLabel('模板纸张工作区')
  await expect.poll(()=>stage.evaluate(el=>{el.scrollTop=100;return el.scrollTop})).toBe(100)
  const zoom=sample.getByRole('group',{name:'预览缩放'})
  const scale=await zoom.innerText()
  await expect(zoom).toBeInViewport()
  await zoom.getByLabel('放大预览').click()
  await expect(zoom).not.toHaveText(scale)
  await zoom.getByLabel('适应宽度').click()
  await expect(zoom).toHaveText(scale)
  const number=sample.getByText('示例文号',{exact:true})
  const topWithinPaper=()=>number.evaluate(el=>el.getBoundingClientRect().top-el.closest('.word-template-paper')!.getBoundingClientRect().top)
  const pos=await topWithinPaper()
  await sample.getByText('无红头版',{exact:true}).click()
  await expect(sample.getByText('示例学院',{exact:true})).toBeHidden()
  await expect(number).toBeVisible()
  expect(await topWithinPaper()).toBeCloseTo(pos,1)
})

test('归档文书选择单版、双版并重试失败下载',async({page})=>{
  const modes:string[]=[]
  let fail=true
  await page.route('**/api/v1/**',async route=>{
    const path=new URL(route.request().url()).pathname
    if(path.endsWith('/auth/refresh'))return route.fulfill({json:{accessToken:'fixture'}})
    if(path.endsWith('/auth/me'))return route.fulfill({json:{id:'demo',name:'演示管理员',status:'ACTIVE',systemRole:'SUPER_ADMIN',isSuperAdmin:true}})
    if(path.endsWith('/archive/search'))return route.fulfill({json:{total:1,items:[{id:'demo',studentNumber:'DEMO',status:'APPROVED'}]}})
    if(path.endsWith('/document/preview'))return route.fulfill({json:{templateId:'edition',editions:[{id:'edition',revision:1}],fields:{},missing:[],document:{type:'doc',schemaVersion:2,content:[{type:'paragraph',attrs:{letterhead:true,style:'title',textAlign:'center'},content:[{type:'text',text:'示例学院'}]}]}}})
    if(path.endsWith('/document')){
      modes.push(route.request().postDataJSON().variant)
      if(fail){fail=false;return route.fulfill({status:503,json:{code:'UNAVAILABLE',message:'请重试'}})}
      return route.fulfill({body:'synthetic download'})
    }
    return route.fulfill({json:{items:[],permissions:[{action:'DISCIPLINE_DOCUMENT_EXPORT',scope:{},effect:'ALLOW'}]}})
  })
  await page.goto('/discipline/archive')
  await page.getByRole('button',{name:'生成文书'}).click()
  await page.getByRole('button',{name:'确认并下载 Word'}).click()
  await expect(page.getByText('文书下载失败')).toBeVisible()
  for(const [option,suffix] of [['仅带红头版','（带红头版）.docx'],['仅无红头版','（无红头版）.docx'],['两种版本','（两种版本）.zip']]){
    await page.getByRole('combobox',{name:'生成版本'}).click()
    await page.locator('.ant-select-item-option-content:visible').filter({hasText:option}).click()
    const downloaded=page.waitForEvent('download')
    await page.getByRole('button',{name:/确认并下载 Word|下载两份 Word/}).click()
    expect((await downloaded).suggestedFilename()).toBe('处分决定书'+suffix)
  }
  expect(modes).toEqual(['letterhead','letterhead','plain','both'])
})

test('缩放红头的文字选区不侵入前后空段',async({page})=>{
  const heading = '湖南科技大学计算机科学与工程学院'
  const paragraphs = ['',heading,'','发文字号'].map(text => ({
    type:'paragraph',
    attrs:{style:'body',letterhead:text===heading,lineSpacingMode:'single',lineSpacing:0,textAlign:'center'},
    content:text ? [{
      type:'text',text,marks:text===heading ? [
        {type:'textStyle',attrs:{scale:35,fontSize:'80pt',fontFamily:'宋体',spacing:40}},
        {type:'bold'},
      ] : [],
    }] : [],
  }))
  const item={ruleId:'demo',groupKey:'demo',tierCode:'warning',name:'示例',revision:1,
    document:{type:'doc',schemaVersion:2,content:paragraphs}}
  await page.route('**/api/v1/**',async route=>{
    const path=new URL(route.request().url()).pathname
    if(path.endsWith('/auth/refresh'))return route.fulfill({json:{accessToken:'fixture'}})
    if(path.endsWith('/auth/me'))return route.fulfill({json:{id:'demo',name:'演示管理员',status:'ACTIVE',systemRole:'SUPER_ADMIN',isSuperAdmin:true}})
    if(path.endsWith('/discipline/document-templates')){
      if(route.request().method()==='PUT')Object.assign(item,route.request().postDataJSON(),{revision:2})
      return route.fulfill({json:{items:[item]}})
    }
    return route.fulfill({json:{items:[],permissions:[]}})
  })
  await page.goto('/discipline/documents')
  await page.getByRole('row').filter({hasText:'示例'}).getByRole('button',{name:/编辑/}).click()
  const editor=page.getByLabel('文件模板正文')
  const title=editor.locator('p').nth(1)
  await expect(title.locator('.word-scaled-glyph')).toHaveCount(16)
  await page.evaluate(()=>document.fonts.ready.then(()=>true))
  await expect.poll(()=>title.evaluate(el=>{
    const range=document.createRange()
    range.selectNodeContents(el)
    const text=range.getBoundingClientRect()
    return Math.max(el.previousElementSibling!.getBoundingClientRect().bottom-text.top,
      text.bottom-el.nextElementSibling!.getBoundingClientRect().top)
  })).toBeLessThanOrEqual(1)
})
