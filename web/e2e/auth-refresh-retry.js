async(page)=>{
 const base=page.url().split('/').slice(0,3).join('/'),errors=[]
 page.on('pageerror',e=>errors.push(e.message))
 const check=(ok,message)=>{if(!ok)throw new Error(message)}
 const before=(await page.context().cookies()).find(c=>c.name==='refresh_token')
 await page.getByRole('button',{name:'重试恢复登录',exact:true}).click()
 await page.getByRole('heading',{name:'到课率检查',exact:true}).waitFor()
 const after=(await page.context().cookies()).find(c=>c.name==='refresh_token')
 check(!!after&&after.value!==before?.value,'Recovery did not rotate session')
 await page.getByRole('button',{name:/账户菜单$/}).click()
 await page.getByText('退出登录',{exact:true}).click()
 await page.getByRole('heading',{name:'欢迎回来',exact:true}).waitFor()
 check(!(await page.context().cookies()).some(c=>c.name==='refresh_token'),'Logout failed to clear cookie')
 const response=await page.context().request.post(base+'/api/v1/auth/refresh')
 check(response.status()===401,'Logged-out browser recovered a session')
 check(errors.length===0,'Browser runtime failure')
 return {cases:['数据库故障解除后原位点击重试，无需密码恢复原业务页并轮换Cookie','恢复后退出登录清除Cookie，后续刷新401'],browserErrors:errors}
}
