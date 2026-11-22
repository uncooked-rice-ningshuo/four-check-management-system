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
  const close = async (dialog, discard = false) => {
    await dialog.getByRole('button', { name: /取\s*消/ }).click()
    if (discard) await page.getByRole('button', { name: '放弃修改' }).click()
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

  lifecycleStage = 'academic year roster'
  await page.getByRole('tab', { name: '学年', exact: true }).click()
  const openRoster = page.getByRole('button', { name: '配置学年名单' }).first()
  await openRoster.click()
  const rosterDialog = page.getByRole('dialog', { name: /学年名单/ })
  const gradeSelect = rosterDialog.getByRole('combobox', { name: '学年添加年级' })
  await gradeSelect.click()
  await gradeSelect.fill('2026级')
  await page.locator('.ant-select-dropdown:visible .ant-select-item-option-active').filter({ hasText: '2026级' }).waitFor()
  await gradeSelect.press('Enter')
  await page.locator('.ant-select-dropdown:visible').last().waitFor({ state: 'hidden' })
  await rosterDialog.getByRole('checkbox', { name: 'E2E major' }).check()
  const majorNode = rosterDialog.getByRole('treeitem', { name: /E2E major/ })
  await majorNode.locator('.ant-tree-switcher').click()
  await rosterDialog.getByRole('checkbox', { name: 'E2E class', checked: true }).waitFor()
  const classCheck = rosterDialog.getByRole('checkbox', { name: 'E2E class' })
  for (const row of [majorNode, rosterDialog.getByRole('treeitem', { name: /E2E class/ })]) {
    const offset = await row.evaluate(node => {
      const box = node.querySelector('[role="checkbox"]').getBoundingClientRect()
      const label = node.querySelector('.roster-major-label > span, .ant-tree-title:not(:has(.roster-major-label))').getBoundingClientRect()
      return Math.abs(box.y + box.height / 2 - label.y - label.height / 2)
    })
    check(offset <= 1, `Checkbox and label centers differ by ${offset}px`)
  }
  await classCheck.uncheck()
  await majorNode.getByText('1 个班', { exact: true }).waitFor()
  await classCheck.check()
  await rosterDialog.screenshot({ animations: 'disabled', path: `${output}/academic-year-membership.png` })
  await rosterDialog.getByRole('button', { name: '下一步：调整展示顺序' }).click()
  await rosterDialog.getByRole('button', { name: '上一步' }).click()
  await rosterDialog.getByRole('checkbox', { name: 'E2E major', checked: true }).waitFor()
  await rosterDialog.getByRole('button', { name: '下一步：调整展示顺序' }).click()
  const [rosterSave] = await Promise.all([page.waitForResponse(r => r.url().includes('/academic/years/') && r.request().method() === 'PUT'), rosterDialog.getByRole('button', { name: '保存配置' }).click()])
  check(rosterSave.status() === 200, 'Roster save failed')
  const savedYear = await rosterSave.json()
  check(savedYear.roster.grades[0].majors[0].classIds[0] === fixture.class, 'Unexpected roster class')
  await rosterDialog.waitFor({ state: 'hidden' })
  await page.reload()
  await page.getByRole('tab', { name: '学年', exact: true }).click()
  await page.getByRole('button', { name: '查看顺序' }).first().click()
  const orderView = page.getByRole('dialog', { name: /查看顺序/ })
  const gradeHeading = orderView.locator('summary').filter({ hasText: '2026级' })
  const majorHeading = orderView.locator('summary').filter({ hasText: 'E2E major' })
  const savedClass = orderView.getByText('E2E class', { exact: true })
  await majorHeading.waitFor()
  check(!await savedClass.isVisible(), 'Classes should start collapsed')
  await majorHeading.click()
  await savedClass.waitFor()
  await gradeHeading.click()
  await savedClass.waitFor({ state: 'hidden' })
  await gradeHeading.click()
  await savedClass.waitFor()
  await majorHeading.click()
  await savedClass.waitFor({ state: 'hidden' })
  check(await orderView.getByRole('checkbox').count() === 0 && await orderView.getByRole('button', { name: /保存|调序|上移|下移/ }).count() === 0, 'Read-only view contains editing controls')
  await orderView.getByRole('button', { name: /关\s*闭/ }).click()
  await openRoster.click()
  await rosterDialog.getByRole('checkbox', { name: 'E2E major', checked: true }).waitFor()
  await rosterDialog.getByRole('checkbox', { name: 'E2E class', checked: true }).waitFor()
  await rosterDialog.getByRole('button', { name: '下一步：调整展示顺序' }).click()
  await rosterDialog.getByText('E2E major', { exact: true }).click()
  await rosterDialog.getByText('E2E class', { exact: true }).first().waitFor()
  await page.screenshot({ path: `${output}/academic-year-roster.png` })
  await close(rosterDialog)

  // Real browser lifecycle: create -> close -> first edit -> close -> reopen.
  for (const kind of ['学年', '学期', '周次']) {
    lifecycleStage = `create-close-edit ${kind}`
    await page.getByRole('button', { name: `新增${kind}`, exact: true }).click()
    const createDialog = page.getByRole('dialog', { name: `新增${kind}`, exact: true })
    if (kind === '学年') await waitValue(createDialog.getByPlaceholder('开始日期'), fixture.yearStart)
    if (kind === '学期') await waitValue(createDialog.getByPlaceholder('开始日期'), fixture.yearEnd.slice(0, 4) + '-02-01')
    await close(createDialog)
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
    await close(dialog, true)
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
  await page.goto(await page.evaluate(() => new URL('/admin/users', location.origin).href))
  await page.getByRole('button', { name: '新增用户' }).waitFor()
  check(await page.getByRole('button', { name: '旧阶段授权清单' }).count() === 0, 'Retired stage workflow still visible')
  return { marker: 'CALENDAR_REGRESSION_PASSED', browserVersion: page.context().browser().version(), browserErrors: errors, knownWarnings: [...new Set(knownWarnings)], unexpectedConsoleErrors, faultInjection: ['HTTP 500', 'null error body'] }
}
