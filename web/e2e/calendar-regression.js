// Playwright CLI run-code function. verify-browser.py supplies only synthetic dates
// and an artifact directory; login credentials never enter this retained script.
async (page) => {
  const fixture = __FIXTURE__
  const output = __OUTPUT__
  const errors = []
  const knownWarnings = []
  const unexpectedConsoleErrors = []
  let lifecycleStage = 'setup'
  page.on('pageerror', error => errors.push(error.message))
  page.on('console', message => {
    if (message.type() !== 'error' && message.type() !== 'warning') return
    const text = message.text()
    if (text.includes('Static function can not consume context') || text.includes('useForm` is not connected')) {
      const location = message.location()
      knownWarnings.push(`${text} (${location.url}:${location.lineNumber}, stage=${lifecycleStage})`)
      return
    }
    const url = message.location().url
    const expectedHTTP = (url.includes('/auth/refresh') && text.includes('401'))
      || (url.endsWith('/favicon.ico') && text.includes('404'))
      || (url.includes('/academic/weeks/') && (text.includes('409') || text.includes('500')))
    if (!expectedHTTP) unexpectedConsoleErrors.push(text)
  })
  const check = (condition, message) => { if (!condition) throw new Error(message) }
  const waitValue = async (locator, expected) => {
    await locator.waitFor({ state: 'visible' })
    const deadline = Date.now() + 5000
    while (await locator.inputValue() !== expected && Date.now() < deadline) await page.waitForTimeout(25)
    check(await locator.inputValue() === expected, `Expected input ${expected}, got ${await locator.inputValue()}`)
  }
  const close = async dialog => {
    await dialog.getByRole('button', { name: /取\s*消/ }).click()
    await dialog.waitFor({ state: 'hidden' })
  }
  const openWeek = async note => {
    await page.getByRole('tab', { name: '周次', exact: true }).click()
    await page.getByRole('row').filter({ hasText: note }).getByRole('button', { name: '编辑', exact: true }).click()
    const dialog = page.getByRole('dialog', { name: '编辑周次', exact: true })
    await dialog.waitFor()
    return dialog
  }
  const submit = async (dialog, path, expectedStatus) => {
    lifecycleStage = `save ${path} ${expectedStatus}`
    const [response] = await Promise.all([
      page.waitForResponse(r => r.url().includes(path) && r.request().method() === 'PUT'),
      dialog.getByRole('button', { name: /确\s*定/ }).click(),
    ])
    check(response.status() === expectedStatus, `Unexpected save status ${response.status()}`)
    return response
  }
  const setDates = async (dialog, start, end) => {
    for (const [placeholder, value] of [['开始日期', start], ['结束日期', end]]) {
      const input = dialog.getByPlaceholder(placeholder)
      await input.fill(value)
      await input.press('Enter')
    }
    await dialog.getByText('备注', { exact: true }).click()
  }
  lifecycleStage = 'initial calendar page render'
  await page.goto(page.url().split('/').slice(0, 3).join('/') + '/admin/academic')
  await page.getByRole('heading', { name: '学期与周次', exact: true }).waitFor()
  await page.locator('.work-context-week').filter({ hasText: '第 1 周' }).waitFor()

  // Real browser lifecycle: create -> close -> first edit -> close -> reopen.
  for (const kind of ['学年', '学期', '周次']) {
    lifecycleStage = `create-close-edit ${kind}`
    await page.getByRole('button', { name: `新增${kind}`, exact: true }).click()
    await close(page.getByRole('dialog', { name: `新增${kind}`, exact: true }))
    await page.getByRole('tab', { name: kind, exact: true }).click()
    for (let attempt = 0; attempt < 2; attempt++) {
      const row = kind === '周次' ? page.getByRole('row').filter({ hasText: '浏览器回归 A' }) : page
      await row.getByRole('button', { name: '编辑', exact: true }).first().click()
      const dialog = page.getByRole('dialog', { name: `编辑${kind}`, exact: true })
      await waitValue(dialog.getByPlaceholder('开始日期'), kind === '周次' ? fixture.weekStart : fixture.yearStart)
      await waitValue(dialog.getByPlaceholder('结束日期'), kind === '周次' ? fixture.weekEnd : fixture.yearEnd)
      await close(dialog)
    }
  }
  for (const [note, start] of [['浏览器回归 B', fixture.otherStart], ['浏览器回归 A', fixture.weekStart]]) {
    lifecycleStage = `reopen and edit ${note}`
    const dialog = await openWeek(note)
    await waitValue(dialog.getByPlaceholder('开始日期'), start)
    await waitValue(dialog.getByLabel('备注'), note)
    await close(dialog)
  }

  // An actual backend conflict (no mock): B's dates overlap A. Keep the dialog
  // and values after 409, then correct them and save without reopening.
  let dialog = await openWeek('浏览器回归 B')
  await waitValue(dialog.getByPlaceholder('开始日期'), fixture.otherStart)
  await setDates(dialog, fixture.weekStart, fixture.weekEnd)
  const conflict = await submit(dialog, '/academic/weeks/', 409)
  check((await conflict.json()).code === 'ACADEMIC_DATE_OVERLAP', 'Missing stable conflict code')
  await page.getByText('日期范围重叠，请核对校历', { exact: true }).waitFor()
  await waitValue(dialog.getByPlaceholder('开始日期'), fixture.weekStart)
  await page.screenshot({ path: `${output}/calendar-conflict.png`, fullPage: true })
  await setDates(dialog, fixture.otherStart, fixture.otherEnd)
  await dialog.getByLabel('备注').fill('浏览器回归 B 已修正')
  await submit(dialog, '/academic/weeks/', 200)
  await dialog.waitFor({ state: 'hidden' })
  await page.getByRole('cell', { name: '浏览器回归 B 已修正', exact: true }).waitFor()

  // Change the current week number in the real DB. The bar and already-loaded
  // override options must update immediately rather than waiting for polling.
  dialog = await openWeek('浏览器回归 A')
  await waitValue(dialog.getByLabel('周次', { exact: true }), '1')
  await dialog.getByLabel('周次', { exact: true }).fill('3')
  await dialog.getByRole('combobox', { name: '到课率违纪计入倍数' }).click()
  await page.locator('.ant-select-dropdown:visible .ant-select-item-option').filter({ hasText: '教学礼拜周（2 倍）' }).click()
  await submit(dialog, '/academic/weeks/', 200)
  await dialog.waitFor({ state: 'hidden' })
  await page.locator('.work-context-week').filter({ hasText: '第 3 周' }).waitFor({ timeout: 5000 })
  await page.getByRole('row').filter({ hasText: '浏览器回归 A' }).getByText('2 倍').waitFor()
  lifecycleStage = 'open global work context editor'
  await page.getByRole('button', { name: '设置临时工作周', exact: true }).click()
  await page.getByRole('combobox', { name: '目标周次' }).click()
  await page.locator('.ant-select-dropdown:visible .ant-select-item-option').filter({ hasText: '第 3 周' }).waitFor({ timeout: 5000 })
  await page.getByRole('button', { name: /取\s*消/ }).click()

  // Explicit fault injection: only these PUT responses are mocked. All login,
  // reads, conflict and successful persistence above use real services.
  for (const [body, expected] of [
    [{ code: 'INTERNAL_ERROR', message: '服务暂时不可用，请稍后重试' }, '服务暂时不可用，请稍后重试'],
    [null, '请求失败'],
  ]) {
    dialog = await openWeek('浏览器回归 A')
    await waitValue(dialog.getByLabel('周次', { exact: true }), '3')
    await dialog.getByLabel('备注').fill('失败时保留的输入')
    const pattern = '**/api/v1/admin/academic/weeks/*'
    const handler = route => route.request().method() === 'PUT'
      ? route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify(body) })
      : route.continue()
    await page.route(pattern, handler)
    try {
      await submit(dialog, '/academic/weeks/', 500)
      await page.getByText(expected, { exact: true }).last().waitFor()
      await waitValue(dialog.getByLabel('备注'), '失败时保留的输入')
      const deadline = Date.now() + 1000
      const button = dialog.getByRole('button', { name: /确\s*定/ })
      while ((await button.getAttribute('class')).includes('ant-btn-loading') && Date.now() < deadline) await page.waitForTimeout(25)
      check(!(await button.getAttribute('class')).includes('ant-btn-loading'), 'Retry waits for message dismissal')
    } finally {
      await page.unroute(pattern, handler)
    }
    await close(dialog)
  }
  await page.reload()
  await page.locator('.work-context-week').filter({ hasText: '第 3 周' }).waitFor()
  dialog = await openWeek('浏览器回归 A')
  await waitValue(dialog.getByLabel('周次', { exact: true }), '3')
  await waitValue(dialog.getByLabel('备注'), '浏览器回归 A')
  await close(dialog)
  await page.getByRole('cell', { name: '浏览器回归 B 已修正', exact: true }).waitFor()
  check(knownWarnings.length === 0, `Unexpected Ant Design warnings: ${[...new Set(knownWarnings)].join('; ')}`)
  check(unexpectedConsoleErrors.length === 0, `Unexpected console errors: ${unexpectedConsoleErrors.join('; ')}`)
  check(errors.length === 0, `Uncaught browser errors: ${errors.join('; ')}`)
  await page.screenshot({ path: `${output}/calendar-passed.png`, fullPage: true })
  return { marker: 'CALENDAR_REGRESSION_PASSED', browserVersion: page.context().browser().version(), browserErrors: errors, knownWarnings: [...new Set(knownWarnings)], unexpectedConsoleErrors, faultInjection: ['HTTP 500', 'null error body'] }
}
