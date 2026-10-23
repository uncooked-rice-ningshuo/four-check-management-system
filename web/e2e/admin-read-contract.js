async(page)=>{
 const base=page.url().split('/').slice(0,3).join('/')
 const errors=[]
 page.on('pageerror',error=>errors.push(error.message))
 const check=(ok,message)=>{if(!ok)throw new Error(message)}
 for(const [endpoint,body,title,retry] of [
  ['users',null,'用户列表加载失败','重试用户列表'],
  ['catalogs',{departments:[],organizationRoles:[]},'组织目录加载失败','重试组织目录'],
 ]){
  const path='**/api/v1/admin/'+endpoint
  const malformed=route=>route.fulfill({status:200,contentType:'application/json',body:JSON.stringify(body)})
  await page.route(path,malformed)
  await page.goto(base+'/admin/users')
  await page.getByText(title,{exact:true}).waitFor()
  check(await page.getByText('暂无用户',{exact:true}).count()===0,'Malformed response displayed normal empty state')
  check(await page.getByText('部门已停用',{exact:true}).count()===0,'Malformed catalog displayed disabled department')
  await page.unroute(path,malformed)
  await page.getByRole('button',{name:retry,exact:true}).click()
  await page.getByText(title,{exact:true}).waitFor({state:'hidden'})
  await page.getByRole('cell',{name:'浏览器回归管理员',exact:true}).waitFor()
 }
 const path='**/api/v1/admin/users'
 const empty=route=>route.fulfill({status:200,contentType:'application/json',body:'[]'})
 await page.route(path,empty)
 await page.reload()
 await page.getByText('暂无用户',{exact:true}).waitFor()
 check(await page.getByText('用户列表加载失败',{exact:true}).count()===0,'Valid empty response rejected')
 await page.unroute(path,empty)
 check(errors.length===0,'Page raised an uncaught response error')
 return {cases:['用户200/null明确失败并真实API重试恢复','组织目录200缺字段明确失败并真实API重试恢复','有效空用户数组显示正常空态'],faultInjection:['HTTP200 JSON null、缺失inspectionTypes、有效空数组'],browserErrors:errors}
}
