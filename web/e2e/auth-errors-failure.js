async(page)=>{
 const fixture=__FIXTURE__,base=page.url().split('/').slice(0,3).join('/'),errors=[],messageContextWarnings=[]
 page.on('pageerror',e=>errors.push(e.message))
 page.on('console',m=>{if(m.type()==='error'&&m.text().includes('Static function can not consume context'))messageContextWarnings.push(m.text())})
 const check=(ok,message)=>{if(!ok)throw new Error(message)}
 const before=(await page.context().cookies()).find(c=>c.name==='refresh_token')
 const failedLogin=await page.context().request.post(base+'/api/v1/auth/login',{data:{loginName:'browser-admin',password:fixture.authPassword}})
 check(failedLogin.status()===500&&(await failedLogin.json()).code==='AUTH_SERVICE_UNAVAILABLE','Login persistence error misclassified')
 check(!failedLogin.headers()['set-cookie'],'Failed login changed cookie')
 await page.getByRole('button',{name:/账户菜单$/}).click()
 await page.getByText('账户安全',{exact:true}).click()
 await page.getByRole('heading',{name:'账户安全',exact:true}).waitFor()
 await page.getByLabel('当前密码',{exact:true}).fill(fixture.authPassword)
 await page.getByLabel('新密码',{exact:true}).fill(fixture.nextPassword)
 await page.getByLabel('确认新密码',{exact:true}).fill(fixture.nextPassword)
 const [changed]=await Promise.all([page.waitForResponse(r=>r.url().endsWith('/auth/change-password')),page.getByRole('button',{name:/更新密码并重新登录$/}).click()])
 check(changed.status()===500,'Password revocation failure was not rejected')
 await page.getByText('认证服务暂时不可用，请稍后重试',{exact:true}).waitFor()
 check(await page.getByLabel('当前密码',{exact:true}).inputValue()===fixture.authPassword,'Password failure erased form')
 let release=()=>{},opened=()=>{},conflictRequests=0
 const gate=new Promise(resolve=>{release=resolve}),started=new Promise(resolve=>{opened=resolve})
 const conflictRoute='**/api/v1/auth/change-password'
 await page.route(conflictRoute,async route=>{
  conflictRequests++;opened();await gate
  await route.fulfill({status:409,contentType:'application/json',body:JSON.stringify({code:'VERSION_CONFLICT',message:'账号状态已变化，请重新登录后重试'})})
 })
 try {
  await page.getByRole('button',{name:/更新密码并重新登录$/}).click()
  await started
  check(await page.getByRole('button',{name:/更新密码并重新登录$/}).isDisabled(),'Pending password mutation permits duplicate submission')
  await page.getByLabel('当前密码',{exact:true}).press('Enter')
  check(conflictRequests===1,'Duplicate password request while pending')
 } finally { release() }
 await page.getByText('账号状态已变化，请重新登录后重试',{exact:true}).waitFor()
 await page.unroute(conflictRoute)
 check(await page.getByLabel('当前密码',{exact:true}).inputValue()===fixture.authPassword,'Version conflict erased form input')
 await page.getByRole('button',{name:/账户菜单$/}).click()
 const [logout]=await Promise.all([page.waitForResponse(r=>r.url().endsWith('/auth/logout')),page.getByText('退出登录',{exact:true}).click()])
 check(logout.status()===500,'Failed revocation reported successful logout')
 await page.getByText('暂时无法确认退出，请重试。',{exact:true}).waitFor()
 check((await page.context().cookies()).find(c=>c.name==='refresh_token')?.value===before?.value,'Failed account operation erased cookie')
 await page.setViewportSize({width:375,height:812})
 await page.waitForFunction(()=>{
  const button=[...document.querySelectorAll('button')].find(b=>b.textContent?.trim()==='重试退出')
  const rect=button?.getBoundingClientRect()
  return !!rect&&rect.x>=0&&rect.right<=window.innerWidth
 },undefined,{timeout:5000})
 const bounds=await page.getByRole('button',{name:'重试退出',exact:true}).boundingBox()
 check(!!bounds&&bounds.x>=0&&bounds.x+bounds.width<=375,'Logout retry is inaccessible on narrow screen')
 check(errors.length===0,'Unhandled browser error')
 check(messageContextWarnings.length===0,`Ant Design static message context warnings: ${messageContextWarnings.length}`)
 return {cases:['真实会话保存故障：登录500且不清既有Cookie','真实撤销故障：改密整体失败保留输入和原Cookie','延迟409冲突：改密按钮禁用、键盘不重复提交、失败保留输入','真实退出撤销故障：显示重试提示、保留当前页面，375px按钮可达'],faultInjection:['PostgreSQL refresh_sessions INSERT/UPDATE trigger failure','HTTP 409 VERSION_CONFLICT with delayed response'],browserErrors:errors}
}
