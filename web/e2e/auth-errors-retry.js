async(page)=>{
 const fixture=__FIXTURE__,base=page.url().split('/').slice(0,3).join('/'),errors=[]
 page.on('pageerror',e=>errors.push(e.message))
 const check=(ok,message)=>{if(!ok)throw new Error(message)}
 await page.getByRole('button',{name:'重试退出',exact:true}).click()
 await page.getByRole('heading',{name:'欢迎回来',exact:true}).waitFor()
 check(!(await page.context().cookies()).some(c=>c.name==='refresh_token'),'Logout retry did not clear cookie')
 const login=async(password)=>{
  await page.getByLabel('账号',{exact:true}).fill('browser-admin')
  await page.getByLabel('密码',{exact:true}).fill(password)
  await page.getByRole('button',{name:/登\s*录/}).click()
  await page.getByRole('button',{name:/账户菜单$/}).waitFor()
 }
 await login(fixture.authPassword)
 await page.getByRole('button',{name:/账户菜单$/}).click()
 await page.getByText('账户安全',{exact:true}).click()
 await page.getByLabel('当前密码',{exact:true}).fill(fixture.authPassword)
 await page.getByLabel('新密码',{exact:true}).fill(fixture.nextPassword)
 await page.getByLabel('确认新密码',{exact:true}).fill(fixture.nextPassword)
 let logoutRequests=0
 page.on('request',r=>{if(r.url().endsWith('/auth/logout'))logoutRequests++})
 await page.getByRole('button',{name:'更新密码并重新登录',exact:true}).click()
 await page.getByRole('heading',{name:'欢迎回来',exact:true}).waitFor()
 check(logoutRequests===0,'Successful password change issued a second logout mutation')
 check(!(await page.context().cookies()).some(c=>c.name==='refresh_token'),'Password change retained cookie')
 await login(fixture.nextPassword)
 const rejected=await page.context().request.post(base+'/api/v1/auth/login',{data:{loginName:'browser-admin',password:fixture.authPassword}})
 check(rejected.status()===401&&(await rejected.json()).code==='INVALID_CREDENTIALS','Old password remains usable')
 check(errors.length===0,'Unhandled browser error')
 return {cases:['故障解除后重试退出成功，原密码仍可登录证明失败改密已回滚','再次改密成功仅一次副作用请求，Cookie清除，新密码登录成功、旧密码401'],browserErrors:errors}
}
