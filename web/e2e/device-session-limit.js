async (page) => {
  const fixture = __FIXTURE__
  const base = page.url().split('/').slice(0, 3).join('/')
  const failures = []
  page.on('pageerror', error => failures.push(error.message))
  const login = async () => {
    const response = await page.context().request.post(base + '/api/v1/auth/login', {
      data: { loginName: 'browser-admin', password: fixture.authPassword },
    })
    if (response.status() !== 200) throw new Error('Additional device login failed: ' + response.status())
  }
  // Initial UI login is device one. Each subsequent login creates another
  // logical session; the page keeps the first device's in-memory Access Token.
  await login()
  await login()
  await login()
  let refreshes = 0
  await page.route('**/api/v1/auth/refresh', async route => { refreshes++; await route.continue() })
  const result = await page.evaluate(async () => {
    const { apiRequest } = await import('/src/shared/api/httpClient.ts')
    try { await apiRequest('/auth/me'); return 'accepted' }
    catch (error) { return error?.code ?? 'unknown' }
  })
  if (result !== 'SESSION_EVICTED') throw new Error('Old device was not rejected: ' + result)
  if (refreshes !== 0) throw new Error('Evicted device attempted Refresh Token rotation')
  await page.waitForURL('**/login')
  await page.getByText('账号已在其他设备登录，当前设备已退出。', { exact: true }).waitFor()
  if (failures.length) throw new Error('Unexpected browser error: ' + failures.join(', '))
  await page.unrouteAll({ behavior: 'wait' })
  return { cases: ['第四次真实登录挤出最早会话；旧 Access Token 返回 SESSION_EVICTED，前端不续期并显示原因'], browserVersion: page.context().browser().version(), browserErrors: failures }
}
