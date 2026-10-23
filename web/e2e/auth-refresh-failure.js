async(page)=>{
 const base=page.url().split('/').slice(0,3).join('/'),errors=[]
 page.on('pageerror',e=>errors.push(e.message))
 const check=(ok,message)=>{if(!ok)throw new Error(message)}
 const before=(await page.context().cookies()).find(c=>c.name==='refresh_token')
 check(!!before,'Expected an authenticated browser session')
 const response=await page.context().request.post(base+'/api/v1/auth/refresh')
 check(response.status()===500,'Database failure must return 500')
 const body=await response.json()
 check(body.code==='AUTH_SERVICE_UNAVAILABLE','Wrong refresh error contract')
 check(!response.headers()['set-cookie'],'Infrastructure failure must preserve cookie')
 check((await page.context().cookies()).find(c=>c.name==='refresh_token')?.value===before.value,'Failed rotation changed browser cookie')
 await page.goto(base+'/inspections/attendance')
 await page.getByRole('heading',{name:'欢迎回来',exact:true}).waitFor()
 await page.getByText('暂时无法恢复登录状态，请重试或使用账号密码登录。',{exact:true}).waitFor()
 await page.getByLabel('账号',{exact:true}).fill('retained-input')
 const [retry]=await Promise.all([page.waitForResponse(r=>r.url().endsWith('/auth/refresh')),page.getByRole('button',{name:'重试恢复登录',exact:true}).click()])
 check(retry.status()===500,'Failure retry changed error contract')
 check(await page.getByLabel('账号',{exact:true}).inputValue()==='retained-input','Retry failure erased login input')
 check((await page.context().cookies()).find(c=>c.name==='refresh_token')?.value===before.value,'Startup failure erased retryable cookie')
 await page.setViewportSize({width:375,height:812})
 const retryBounds=await page.getByRole('button',{name:'重试恢复登录',exact:true}).boundingBox()
 check(!!retryBounds&&retryBounds.x>=0&&retryBounds.x+retryBounds.width<=375,'Recovery button outside narrow viewport')
 check(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth),'Recovery notice causes page overflow')
 check(errors.length===0,'Browser runtime failure')
 return {cases:['真实 PostgreSQL 保存故障返回500、稳定错误码、不写Set-Cookie','浏览器初始化故障显示明确提示和重试入口，失败重试保留输入与Cookie','375px登录提示和恢复按钮完整可见，无页面横向溢出'],faultInjection:['PostgreSQL refresh_sessions INSERT trigger failure'],browserErrors:errors}
}
