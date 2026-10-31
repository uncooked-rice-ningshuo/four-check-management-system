// Uses a signed, expired credential from the disposable API only. Never retain it in reports.
async (page) => {
  const fixture = __FIXTURE__
  const base = page.url().split('/').slice(0, 3).join('/')
  const errors = [], cases = [], httpFailures = []
  const check = (condition, message) => { if (!condition) throw new Error(message) }
  page.on('pageerror', error => errors.push(error.message))
  let expireRequests = false, failRefresh = false, renewals = 0, rejected = 0
  let releaseConcurrent
  let concurrentGate = null
  await page.route('**/api/v1/**', async route => {
    const request = route.request()
    const path = '/' + request.url().split('/').slice(3).join('/').split('?')[0]
    if (path === '/api/v1/auth/refresh') {
      renewals++
      if (concurrentGate) await concurrentGate
      if (failRefresh) {
        await route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify({ code: 'AUTH_SERVICE_UNAVAILABLE', message: '认证服务暂时不可用，请稍后重试' }) })
        return
      }
      const response = await route.fetch()
      if (response.status() === 200) expireRequests = false
      await route.fulfill({ response })
      return
    }
    if (expireRequests && request.headers().authorization && !path.startsWith('/api/v1/auth/')) {
      const response = await route.fetch({ headers: { ...request.headers(), authorization: 'Bearer ' + fixture.expiredAccessToken } })
      check(response.status() === 401, 'Expired signed credential must be rejected by real API')
      check((await response.json()).code === 'UNAUTHORIZED', 'Expired credential error code mismatch')
      rejected++
      httpFailures.push({ path, status: response.status(), code: 'UNAUTHORIZED' })
      if (releaseConcurrent && rejected >= 2) releaseConcurrent()
      await route.fulfill({ response })
      return
    }
    await route.continue()
  })
  await page.clock.install()
  await page.goto(base + '/admin/academic')
  await page.getByRole('group', { name: '当前工作上下文' }).waitFor()
  await page.getByRole('tab', { name: '周次', exact: true }).click()
  await page.getByRole('button', { name: '编辑', exact: true }).first().click()
  const dialog = page.getByRole('dialog', { name: '编辑周次', exact: true })
  await dialog.waitFor()
  const note = dialog.getByLabel('备注', { exact: true })
  await note.fill('静默续期期间未保存的输入')
  const before = renewals
  expireRequests = true
  const renewed = page.waitForResponse(response => response.url().endsWith('/auth/refresh') && response.status() === 200)
  const weekLoaded = page.waitForResponse(response => response.url().endsWith('/work-context') && response.status() === 200)
  await page.clock.runFor(31_000)
  await renewed
  await weekLoaded
  check(renewals === before + 1, 'Work week polling should rotate exactly once')
  check(await note.inputValue() === '静默续期期间未保存的输入', 'Silent renewal erased the open form')
  check(!await page.getByText('读取工作周次失败', { exact: true }).isVisible(), 'Work week stayed failed after renewal')
  cases.push('真实过期签名被API拒绝401；30秒工作周轮询自动续期和重试200，无需重载且保留弹窗输入')

  rejected = 0
  concurrentGate = new Promise(resolve => { releaseConcurrent = resolve })
  const beforeConcurrent = renewals
  expireRequests = true
  const concurrent = await page.evaluate(async () => {
    const { apiRequest } = await import('/src/shared/api/httpClient.ts')
    const results = await Promise.all([apiRequest('/work-context'), apiRequest('/admin/academic/calendar')])
    return results.length
  })
  concurrentGate = null
  releaseConcurrent = null
  check(concurrent === 2 && rejected >= 2 && renewals === beforeConcurrent + 1, 'Concurrent requests must share one real rotation')
  cases.push('两个并发真实业务401共享一次Cookie轮换并各自重试成功')

  expireRequests = true
  failRefresh = true
  const failed = page.waitForResponse(response => response.url().endsWith('/auth/refresh') && response.status() === 500)
  await page.clock.runFor(31_000)
  await failed
  // Allow TanStack Query's one configured retry to settle before using the visible retry button.
  const failedAgain = page.waitForResponse(response => response.url().endsWith('/auth/refresh') && response.status() === 500)
  await page.clock.runFor(2_000)
  await failedAgain
  await page.getByText('读取工作周次失败', { exact: true }).waitFor()
  check(await note.inputValue() === '静默续期期间未保存的输入', 'Temporary refresh failure erased form')
  failRefresh = false
  const recovered = page.waitForResponse(response => response.url().endsWith('/work-context') && response.status() === 200)
  // The work-context action is behind the open modal; close it as a user would before retrying.
  await dialog.getByRole('button', { name: /取\s*消/ }).click()
  await page.getByRole('button', { name: '重试', exact: true }).click()
  await recovered
  await page.getByRole('group', { name: '当前工作上下文' }).waitFor()
  cases.push('续期500保留页面和表单；故障解除后工作周重试成功，不要求重新登录')

  await page.context().clearCookies({ name: 'refresh_token' })
  expireRequests = true
  const invalidSession = page.waitForResponse(response => response.url().endsWith('/auth/refresh') && response.status() === 401)
  await page.clock.runFor(31_000)
  await invalidSession
  await page.waitForURL('**/login')
  await page.getByText('登录状态已失效，请重新登录。', { exact: true }).waitFor()
  await page.getByLabel('账号', { exact: true }).fill('browser-admin')
  await page.getByLabel('密码', { exact: true }).fill(fixture.authPassword)
  expireRequests = false
  await page.getByRole('button', { name: /^登\s*录$/ }).click()
  await page.getByRole('heading', { name: '学期与周次', exact: true }).waitFor()
  await page.getByRole('group', { name: '当前工作上下文' }).waitFor()
  cases.push('Refresh Cookie失效时真实401转登录并明确提示；重新登录返回原业务页')
  check(errors.length === 0, 'Unexpected browser runtime error')
  await page.unrouteAll({ behavior: 'wait' })
  return { cases, browserVersion: page.context().browser().version(), browserErrors: errors, httpFailures, faultInjection: ['Disposable API signed expired JWT replaces outgoing bearer only', 'Refresh HTTP 500 for temporary outage', 'Clear disposable refresh Cookie for real invalid-session response'] }
}
