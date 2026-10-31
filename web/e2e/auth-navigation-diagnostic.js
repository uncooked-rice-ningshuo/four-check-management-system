// Diagnostic only: real refresh requests and full navigations, no mocked API.
// Credentials are substituted into a temporary runner file, never the report.
async (page) => {
  const fixture = __FIXTURE__
  const base = page.url().split('/').slice(0,3).join('/')
  const rounds = []
  const login = async () => {
    await page.goto(base + '/login')
    await page.getByRole('textbox', {name:'账号',exact:true}).fill('browser-admin')
    await page.getByLabel('密码', {exact:true}).fill(fixture.authPassword)
    await page.getByRole('button', {name:/登\s*录/}).click()
    await page.getByRole('group', {name:'当前工作上下文'}).waitFor()
  }
  for (let index = 0; index < 12; index++) {
    const events = []
    const response = r => { if (r.url().endsWith('/auth/refresh')) events.push({status:r.status()}) }
    const failed = r => { if (r.url().endsWith('/auth/refresh')) events.push({failure:r.failure()?.errorText}) }
    page.on('response', response)
    page.on('requestfailed', failed)
    const refreshing = page.waitForRequest(r => r.url().endsWith('/auth/refresh'))
    await page.goto(base + '/')
    await refreshing
    await page.goto(base + '/inspections/attendance')
    await page.getByRole('button', {name:'批量导入到课率',exact:true})
      .or(page.getByRole('textbox', {name:'账号',exact:true})).waitFor()
    const loggedOut = page.url().split('?')[0].endsWith('/login')
    page.off('response', response)
    page.off('requestfailed', failed)
    rounds.push({index, loggedOut, events})
    if (loggedOut) await login()
  }
  // A separate API context receives the rotated cookie, so the page never does.
  // This isolates lost delivery from backend rejection without printing tokens.
  const upstreamContext = await page.context().browser().newContext()
  const before = (await page.context().cookies()).find(c => c.name === 'refresh_token')
  let upstreamStatus = 0
  const loseResponse = async route => {
    const response = await upstreamContext.request.fetch(route.request())
    upstreamStatus = response.status()
    await route.abort('failed')
  }
  try {
    await page.route('**/api/v1/auth/refresh', loseResponse)
    await page.goto(base + '/')
    await page.getByRole('textbox', {name:'账号',exact:true}).waitFor()
    await page.unroute('**/api/v1/auth/refresh', loseResponse)
    const after = (await page.context().cookies()).find(c => c.name === 'refresh_token')
    const responsePromise = page.waitForResponse(r => r.url().endsWith('/auth/refresh'))
    await page.goto(base + '/')
    const retried = await responsePromise
    const body = await retried.json()
    await page.getByRole('textbox', {name:'账号',exact:true}).waitFor()
    const controlledLoss = {upstreamStatus, cookieUnchanged: Boolean(before && after && before.value === after.value), retryStatus:retried.status(), retryCode:body.code}
    if (upstreamStatus !== 200 || !controlledLoss.cookieUnchanged || retried.status() !== 401 || body.code !== 'INVALID_SESSION') throw new Error('Controlled response-loss hypothesis was not confirmed')
    return {rounds, controlledLoss, browserVersion:page.context().browser().version(), cases:['连续整页跳转认证诊断（12 轮，未注入响应）','真实刷新成功但响应丢失，旧 Cookie 重试得到 401 INVALID_SESSION'], faultInjection:['独立请求上下文接收真实刷新响应，浏览器请求中止，不交付轮换 Cookie']}
  } finally {
    await page.unroute('**/api/v1/auth/refresh', loseResponse)
    await upstreamContext.close()
  }
}
