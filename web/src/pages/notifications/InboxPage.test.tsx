import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { getInbox, markInboxRead, type InboxQuery } from '../../shared/api/inboxApi'
import { InboxPage } from './InboxPage'

vi.mock('../../shared/api/inboxApi', async importOriginal => {
  const actual = await importOriginal<typeof import('../../shared/api/inboxApi')>()
  return { ...actual, getInbox: vi.fn(), markInboxRead: vi.fn() }
})

interface RecordFixture {
  id: string
  category: string
  action?: string
  body: string
  createdAt: string
  read: boolean
}

const records: RecordFixture[] = [
  { id: '2', category: 'organization', body: '部门与角色已更新，即时生效。当前主部门：学习部；学习部（老师）', read: false, createdAt: '2026-11-22T02:00:00Z' },
  { id: '1', category: 'permission', action: 'DISCIPLINE_LEDGER_VIEW', body: '个人授权规则已启用。动作 检查查看，开始日期 2026-11-23。', read: false, createdAt: '2026-11-22T01:00:00Z' },
]

function response(query: InboxQuery) {
  const visible = records.filter(item => (!query.unread || !item.read) && (!query.category || item.category === query.category))
  return {
    items: visible.slice((query.page - 1) * query.pageSize, query.page * query.pageSize),
    total: visible.length,
    unread: records.filter(item => !item.read).length,
    through: records[0].id,
    page: query.page,
    pageSize: query.pageSize,
    sort: query.sort,
  }
}

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity }, mutations: { retry: false } } })
  const view = render(<QueryClientProvider client={client}><InboxPage /></QueryClientProvider>)
  return { client, ...view }
}

const query = (patch: Partial<InboxQuery>) => expect.objectContaining({ page: 1, unread: false, category: '', from: '', to: '', sort: 'newest', ...patch })

const filterForm = () => {
  const form = screen.getByRole('combobox', { name: '消息类别' }).closest('form')
  if (!form) throw new Error('未找到原生查询表单')
  return within(form)
}

async function submitSearch(patch: { category?: string; read?: string }) {
  const form = filterForm()
  if (patch.category) {
    fireEvent.mouseDown(form.getByRole('combobox', { name: '消息类别' }))
    fireEvent.click(await screen.findByText(patch.category, { selector: '.ant-select-item-option-content' }))
  }
  if (patch.read) {
    fireEvent.mouseDown(form.getByRole('combobox', { name: '阅读状态' }))
    fireEvent.click(await screen.findByText(patch.read, { selector: '.ant-select-item-option-content' }))
  }
  fireEvent.click(form.getByRole('button', { name: /查\s*询/ }))
}

function batchBar() {
  const alert = document.querySelector('.ant-pro-table-alert')
  if (!alert) throw new Error('未找到批量操作提示条')
  return within(alert as HTMLElement)
}

describe('InboxPage', () => {
  beforeEach(() => {
    records.forEach(item => { item.read = false })
    vi.mocked(getInbox).mockReset()
    vi.mocked(markInboxRead).mockReset()
    vi.mocked(markInboxRead).mockResolvedValue({ ok: true, changed: 1 })
  })

  it('renders a retryable load failure in the content area and recovers', async () => {
    vi.mocked(getInbox)
      .mockRejectedValueOnce(new Error('offline'))
      .mockImplementation(filters => Promise.resolve(response(filters)))

    renderPage()

    expect(await screen.findByText('读取消息失败')).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: /重\s*试/ }))
    expect(await screen.findByRole('cell', { name: /部门与角色变更/ })).toBeVisible()
    expect(screen.getByText('共 2 条')).toBeVisible()
  })

  it('applies native search fields and toolbar sort from the first page', async () => {
    vi.mocked(getInbox).mockImplementation(filters => Promise.resolve(response(filters)))

    renderPage()
    await screen.findByRole('cell', { name: /部门与角色变更/ })
    expect(getInbox).toHaveBeenCalledWith(query({}))

    await submitSearch({ read: '仅未读' })
    await waitFor(() => expect(getInbox).toHaveBeenCalledWith(query({ unread: true })))

    await submitSearch({ category: '个人授权' })
    await waitFor(() => expect(getInbox).toHaveBeenCalledWith(query({ unread: true, category: 'permission' })))
    expect(screen.queryByRole('cell', { name: /部门与角色变更/ })).not.toBeInTheDocument()

    fireEvent.mouseDown(screen.getByRole('combobox', { name: '消息排序' }))
    fireEvent.click(await screen.findByText('未读优先', { selector: '.ant-select-item-option-content' }))
    await waitFor(() => expect(getInbox).toHaveBeenCalledWith(query({ unread: true, category: 'permission', sort: 'unread-first' })))
  })

  it('keeps unread messages visible when marking read fails, then succeeds on retry', async () => {
    vi.mocked(getInbox).mockImplementation(filters => Promise.resolve(response(filters)))
    vi.mocked(markInboxRead)
      .mockRejectedValueOnce(new Error('failed'))
      .mockImplementation(async ({ id }) => {
        records.forEach(item => { if (item.id === id) item.read = true })
        return { ok: true, changed: 1 }
      })

    renderPage()
    await screen.findByRole('cell', { name: /部门与角色变更/ })
    expect(screen.getAllByRole('button', { name: '标为已读' })).toHaveLength(2)

    fireEvent.click(screen.getAllByRole('button', { name: '标为已读' })[0])
    expect(await screen.findByText('更新失败，请重试')).toBeVisible()
    expect(screen.getAllByRole('button', { name: '标为已读' })).toHaveLength(2)

    fireEvent.click(screen.getAllByRole('button', { name: '标为已读' })[0])
    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('已将 1 条消息标为已读'))
    expect(screen.getAllByRole('button', { name: '标为已读' })).toHaveLength(1)
  })

  it('explains a filtered empty result and restores the list with native reset', async () => {
    vi.mocked(getInbox).mockImplementation(filters => {
      const payload = response(filters)
      return Promise.resolve({ ...payload, items: filters.unread ? [] : payload.items })
    })

    renderPage()
    await screen.findByRole('cell', { name: /部门与角色变更/ })

    await submitSearch({ read: '仅未读' })
    expect(await screen.findByText('当前仅看未读，切换到全部消息可查看已读记录。')).toBeVisible()

    fireEvent.click(filterForm().getByRole('button', { name: /重\s*置/ }))
    await waitFor(() => expect(getInbox).toHaveBeenCalledWith(query({})))
    expect(await screen.findByRole('cell', { name: /部门与角色变更/ })).toBeVisible()
  })

  it('shows loading placeholders instead of zero counts before data arrives', () => {
    vi.mocked(getInbox).mockImplementation(() => new Promise(() => {}))
    renderPage()
    expect(screen.getByText('未读数加载中…')).toBeVisible()
    expect(screen.queryByText('共 0 条')).not.toBeInTheDocument()
    expect(screen.queryByText('未读 0 条')).not.toBeInTheDocument()
  })

  it('keeps counts unavailable after a failed load', async () => {
    vi.mocked(getInbox).mockRejectedValue(new Error('offline'))
    renderPage()
    expect(await screen.findByText('读取消息失败')).toBeVisible()
    expect(screen.getByText('未读数读取失败')).toBeVisible()
    expect(screen.queryByText('共 0 条')).not.toBeInTheDocument()
    expect(screen.queryByText('未读 0 条')).not.toBeInTheDocument()
  })

  it('returns to the last valid page after reading the final unread item', async () => {
    const many = Array.from({ length: 21 }, (_, index) => ({
      id: String(21 - index),
      category: 'organization',
      body: `部门变更 ${21 - index}`,
      read: false,
      createdAt: '2026-11-22T02:00:00Z',
    }))
    const list = (filters: InboxQuery) => {
      const visible = many.filter(item => !filters.unread || !item.read)
      return {
        items: visible.slice((filters.page - 1) * filters.pageSize, filters.page * filters.pageSize),
        total: visible.length,
        unread: many.filter(item => !item.read).length,
        through: '21',
        page: filters.page,
        pageSize: filters.pageSize,
        sort: filters.sort,
      }
    }
    vi.mocked(getInbox).mockImplementation(filters => Promise.resolve(list(filters)))
    vi.mocked(markInboxRead).mockImplementation(async ({ id }) => {
      const target = many.find(item => item.id === id)
      if (target) target.read = true
      return { ok: true, changed: 1 }
    })

    renderPage()
    await screen.findByRole('cell', { name: /部门变更 21/ })
    await submitSearch({ read: '仅未读' })
    await waitFor(() => expect(getInbox).toHaveBeenCalledWith(query({ unread: true })))
    fireEvent.click(await screen.findByTitle('2'))
    await waitFor(() => expect(getInbox).toHaveBeenCalledWith(query({ page: 2, unread: true })))
    await screen.findByRole('cell', { name: /部门变更 1/ })

    fireEvent.click(screen.getAllByRole('button', { name: '标为已读' })[0])
    await waitFor(() => expect(getInbox).toHaveBeenCalledWith(query({ unread: true })))
    await waitFor(() => expect(screen.getAllByRole('cell', { name: /部门变更 \d+/ })).toHaveLength(20))
  })

  it('confirms the account-wide scope before marking all messages read', async () => {
    vi.mocked(getInbox).mockImplementation(filters => Promise.resolve(response(filters)))
    vi.mocked(markInboxRead).mockImplementation(async ({ all }) => {
      if (all) records.forEach(item => { item.read = true })
      return { ok: true, changed: 2 }
    })

    renderPage()
    await screen.findByRole('cell', { name: /部门与角色变更/ })
    await submitSearch({ category: '个人授权' })
    await waitFor(() => expect(getInbox).toHaveBeenCalledWith(query({ category: 'permission' })))
    await screen.findByRole('cell', { name: /个人授权变更/ })

    fireEvent.click(screen.getByRole('button', { name: '全部标为已读' }))
    const dialog = await screen.findByRole('dialog')
    expect(dialog).toHaveTextContent('将本人全部消息标为已读？')
    expect(dialog).toHaveTextContent('覆盖本人全部 2 条未读消息')
    fireEvent.click(within(dialog).getByRole('button', { name: /取\s*消/ }))
    expect(markInboxRead).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: '全部标为已读' }))
    fireEvent.click(await screen.findByRole('button', { name: '确认标为已读' }))
    await waitFor(() => expect(markInboxRead).toHaveBeenCalledWith({ id: '2', all: true }, expect.anything()))
    expect(await screen.findByText(/已将本人全部 2 条未读消息标为已读/)).toBeVisible()
  })

  it('freezes the unread count and submit boundary while the confirm dialog is open', async () => {
    let request = 0
    const arrived = { id: '3', category: 'permission', body: '新到的授权变更', read: false, createdAt: '2026-11-22T03:00:00Z' }
    vi.mocked(getInbox).mockImplementation(filters => {
      const base = response(filters)
      if (request++ === 0) return Promise.resolve(base)
      return Promise.resolve({
        ...base,
        items: [arrived, ...base.items],
        total: base.total + 1,
        unread: base.unread + 1,
        through: arrived.id,
      })
    })
    vi.mocked(markInboxRead).mockResolvedValue({ ok: true, changed: 2 })

    const { client } = renderPage()
    await screen.findByRole('cell', { name: /部门与角色变更/ })
    fireEvent.click(screen.getByRole('button', { name: '全部标为已读' }))
    const dialog = await screen.findByRole('dialog')
    expect(dialog).toHaveTextContent('覆盖本人全部 2 条未读消息')

    await act(async () => { await client.refetchQueries({ queryKey: ['inbox'] }) })
    expect(await screen.findByRole('cell', { name: /新到的授权变更/ })).toBeVisible()
    expect(document.querySelector('.inbox-heading-count')).toHaveTextContent('未读 3 条')
    expect(screen.getByRole('dialog')).toHaveTextContent('覆盖本人全部 2 条未读消息')

    fireEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: '确认标为已读' }))
    await waitFor(() => expect(markInboxRead).toHaveBeenCalledWith({ id: '2', all: true }, expect.anything()))
    expect(markInboxRead).not.toHaveBeenCalledWith({ id: '3', all: true }, expect.anything())
    expect(await screen.findByText(/已将本人全部 2 条未读消息标为已读/)).toBeVisible()
  })

  it('distinguishes the message time from the authorization effective date', async () => {
    vi.mocked(getInbox).mockImplementation(filters => Promise.resolve(response(filters)))
    renderPage()
    await screen.findByRole('cell', { name: /部门与角色变更/ })

    const messageTime = document.querySelector('time[dateTime="2026-11-22T02:00:00Z"]')
    expect(messageTime).toHaveAttribute('dateTime', '2026-11-22T02:00:00Z')
    const effective = screen.getByText('授权生效')
    const chip = effective.parentElement
    expect(chip).toHaveTextContent('2026-11-23')
    expect(chip?.querySelector('time')).toHaveAttribute('dateTime', '2026-11-23')
  })

  it('selects rows and marks only the selected messages read', async () => {
    vi.mocked(getInbox).mockImplementation(filters => Promise.resolve(response(filters)))
    vi.mocked(markInboxRead).mockImplementation(async ({ ids }) => {
      records.forEach(item => { if (ids?.includes(item.id)) item.read = true })
      return { ok: true, changed: ids?.length ?? 0 }
    })

    renderPage()
    await screen.findByRole('cell', { name: /部门与角色变更/ })
    fireEvent.click(screen.getAllByRole('checkbox')[0])
    expect(batchBar().getByText(/已选择/)).toHaveTextContent('已选择 2 条消息')

    fireEvent.click(batchBar().getByRole('button', { name: '标为已读' }))
    const dialog = await screen.findByRole('dialog')
    expect(dialog).toHaveTextContent('仅处理当前账号选中的 2 条消息')
    fireEvent.click(within(dialog).getByRole('button', { name: '确认标为已读' }))
    await waitFor(() => expect(markInboxRead).toHaveBeenCalledWith({ ids: ['2', '1'] }, expect.anything()))
    expect(await screen.findByText(/已将选中的 2 条消息标为已读/)).toBeVisible()
    expect(document.querySelector('.ant-pro-table-alert')).toBeNull()
  })

  it('uses the native date range field without an extra annotation', async () => {
    vi.mocked(getInbox).mockImplementation(filters => Promise.resolve(response(filters)))
    renderPage()
    await screen.findByRole('cell', { name: /部门与角色变更/ })

    const form = filterForm()
    expect(form.getByPlaceholderText('开始日期')).toBeVisible()
    expect(form.getByPlaceholderText('结束日期')).toBeVisible()
    expect(document.body).not.toHaveTextContent(/北京时间 UTC\+8/)
  })

  it('keeps the unfiltered empty state to one centered message without a placeholder row', async () => {
    vi.mocked(getInbox).mockImplementation(filters => Promise.resolve({ ...response(filters), items: [], total: 0, unread: 0 }))
    renderPage()

    expect(await screen.findByText('有新的部门、角色或授权变更时会显示在这里。')).toBeVisible()
  })

  it('shows the result count in the table heading alongside selection controls', async () => {
    vi.mocked(getInbox).mockImplementation(filters => Promise.resolve(response(filters)))
    renderPage()
    await screen.findByRole('cell', { name: /部门与角色变更/ })

    expect(screen.getByText('消息列表')).toBeVisible()
    expect(screen.getByText('共 2 条')).toBeVisible()
    expect(screen.getAllByRole('checkbox').length).toBeGreaterThan(1)
  })

  it('clears the selection when filters change so batch actions never hit stale rows', async () => {
    vi.mocked(getInbox).mockImplementation(filters => Promise.resolve(response(filters)))

    renderPage()
    await screen.findByRole('cell', { name: /部门与角色变更/ })
    fireEvent.click(screen.getAllByRole('checkbox')[0])
    expect(batchBar().getByText(/已选择/)).toBeInTheDocument()

    await submitSearch({ read: '仅未读' })
    await waitFor(() => expect(getInbox).toHaveBeenCalledWith(query({ unread: true })))
    expect(document.querySelector('.ant-pro-table-alert')).toBeNull()
  })

  it('opens a non-modal detail that stays view-only until explicitly marked read', async () => {
    vi.mocked(getInbox).mockImplementation(filters => Promise.resolve(response(filters)))
    renderPage()
    await screen.findByRole('cell', { name: /部门与角色变更/ })

    expect(screen.queryByRole('region', { name: '消息详情' })).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '部门与角色变更' }))
    const detail = await screen.findByRole('region', { name: '消息详情' })
    expect(detail.querySelector('time')).toHaveAttribute('dateTime', '2026-11-22T02:00:00Z')
    expect(detail).toHaveTextContent('部门与角色已更新')
    expect(within(detail).getByRole('button', { name: '标为已读' })).toBeVisible()
    expect(markInboxRead).not.toHaveBeenCalled()

    fireEvent.click(within(detail).getByRole('button', { name: '标为已读' }))
    await waitFor(() => expect(markInboxRead).toHaveBeenCalledWith({ id: '2' }, expect.anything()))
  })

  it('switches between records A/B, closes on Escape and restores focus to the opener', async () => {
    vi.mocked(getInbox).mockImplementation(filters => Promise.resolve(response(filters)))
    renderPage()
    await screen.findByRole('cell', { name: /部门与角色变更/ })

    fireEvent.click(screen.getByRole('button', { name: '部门与角色变更' }))
    let detail = await screen.findByRole('region', { name: '消息详情' })
    expect(detail).toHaveTextContent('部门与角色已更新')

    const secondTitle = screen.getByRole('button', { name: '个人授权变更' })
    fireEvent.click(secondTitle)
    detail = await screen.findByRole('region', { name: '消息详情' })
    expect(detail).toHaveTextContent('个人授权规则已启用')
    expect(detail).not.toHaveTextContent('部门与角色已更新')

    fireEvent.keyDown(document, { key: 'Escape' })
    await waitFor(() => expect(screen.queryByRole('region', { name: '消息详情' })).not.toBeInTheDocument())
    await waitFor(() => expect(document.activeElement).toBe(secondTitle))
  })

  it('opens detail from the focusable title and keeps unread styling separate from selection', async () => {
    vi.mocked(getInbox).mockImplementation(filters => Promise.resolve(response(filters)))
    renderPage()
    await screen.findByRole('cell', { name: /部门与角色变更/ })

    const firstRow = screen.getAllByRole('row')[1]
    expect(firstRow).toHaveClass('inbox-row--unread')
    expect(firstRow).not.toHaveClass('ant-table-row-selected')
    fireEvent.click(screen.getAllByRole('checkbox')[0])
    expect(firstRow).toHaveClass('ant-table-row-selected')
    expect(screen.queryByRole('region', { name: '消息详情' })).not.toBeInTheDocument()

    const title = within(firstRow).getByRole('button', { name: '部门与角色变更' })
    title.focus()
    expect(document.activeElement).toBe(title)
    fireEvent.click(title)
    expect(await screen.findByRole('region', { name: '消息详情' })).toBeVisible()
  })

  it('keeps checkbox, batch and row actions from opening the detail pane', async () => {
    vi.mocked(getInbox).mockImplementation(filters => Promise.resolve(response(filters)))
    renderPage()
    await screen.findByRole('cell', { name: /部门与角色变更/ })

    fireEvent.click(screen.getAllByRole('checkbox')[0])
    fireEvent.click(batchBar().getByRole('button', { name: '标为已读' }))
    expect(screen.queryByRole('region', { name: '消息详情' })).not.toBeInTheDocument()
    fireEvent.click(within(await screen.findByRole('dialog')).getByRole('button', { name: /取\s*消/ }))

    const firstRow = screen.getAllByRole('row')[1]
    fireEvent.click(within(firstRow).getByRole('button', { name: '标为已读' }))
    expect(screen.queryByRole('region', { name: '消息详情' })).not.toBeInTheDocument()
  })

  it('closes detail when filters change so a previous record cannot linger', async () => {
    vi.mocked(getInbox).mockImplementation(filters => Promise.resolve(response(filters)))
    renderPage()
    await screen.findByRole('cell', { name: /部门与角色变更/ })

    fireEvent.click(screen.getByRole('button', { name: '部门与角色变更' }))
    expect(await screen.findByRole('region', { name: '消息详情' })).toHaveTextContent('部门与角色已更新')

    await submitSearch({ category: '个人授权' })
    await waitFor(() => expect(screen.queryByRole('region', { name: '消息详情' })).not.toBeInTheDocument())
    expect(screen.queryByRole('cell', { name: /部门与角色变更/ })).not.toBeInTheDocument()
  })

  it('closes detail on pagination without carrying the previous record', async () => {
    const many = Array.from({ length: 25 }, (_, index) => ({
      id: String(25 - index),
      category: 'organization',
      body: `分页消息 ${25 - index}`,
      read: false,
      createdAt: '2026-11-22T02:00:00Z',
    }))
    vi.mocked(getInbox).mockImplementation(filters => Promise.resolve({
      items: many.slice((filters.page - 1) * filters.pageSize, filters.page * filters.pageSize),
      total: many.length,
      unread: many.length,
      through: '25',
      page: filters.page,
      pageSize: filters.pageSize,
      sort: filters.sort,
    }))

    renderPage()
    await screen.findByRole('cell', { name: /分页消息 25/ })
    fireEvent.click(screen.getAllByRole('button', { name: '部门与角色变更' })[0])
    expect(await screen.findByRole('region', { name: '消息详情' })).toHaveTextContent('分页消息 25')

    fireEvent.click(await screen.findByTitle('2'))
    await waitFor(() => expect(screen.queryByRole('region', { name: '消息详情' })).not.toBeInTheDocument())
    expect(await screen.findByRole('cell', { name: /分页消息 5/ })).toBeVisible()
  })
})
