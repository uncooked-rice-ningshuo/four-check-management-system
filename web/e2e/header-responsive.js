async (page) => {
  const base = page.url().split('/').slice(0, 3).join('/')
  const cases = []
  await page.goto(base + '/notifications')
  await page.getByRole('button', { name: '进入系统全屏' }).click()
  await page.getByRole('button', { name: '退出系统全屏' }).waitFor()
  await page.getByRole('link', { name: '工作台', exact: true }).first().click()
  if (!await page.evaluate(() => document.fullscreenElement === document.documentElement)) throw new Error('Navigation exited system fullscreen')
  await page.evaluate(() => document.exitFullscreen())
  await page.getByRole('button', { name: '进入系统全屏' }).waitFor()
  cases.push('根文档进入全屏、站内导航保持、浏览器退出后按钮同步')
  const initialViewport = page.viewportSize()
  for (const width of [1440, 857, 390]) {
    await page.setViewportSize({ width, height: 831 })
    const manage = page.getByRole('button', { name: '设置临时工作周' })
    await manage.locator('svg').waitFor({ state: 'visible' })
    await page.locator('.bell-trigger svg').waitFor({ state: 'visible' })
    if (await manage.evaluate(button => button.getBoundingClientRect().right > document.querySelector('.header-fullscreen').getBoundingClientRect().left)) {
      throw new Error('Work week action overlaps notification button at width ' + width)
    }
    await manage.click()
    await page.getByRole('dialog', { name: '设置临时工作周' }).getByRole('button', { name: /取\s*消/ }).click()
  }
  if (initialViewport) await page.setViewportSize(initialViewport)
  cases.push('1440、857、390px 页头图标可见，工作周与通知操作不重叠，设置可打开关闭')
  return { cases, browserVersion: page.context().browser().version() }
}
