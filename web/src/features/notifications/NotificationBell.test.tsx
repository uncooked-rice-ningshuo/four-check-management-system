import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useInboxBell } from './useInbox'
import { NotificationBell } from './NotificationBell'

vi.mock('./useInbox', () => ({ useInboxBell: vi.fn() }))

const readItem = { id: '1', category: 'permission', body: '个人授权规则已启用', createdAt: '2026-11-22T01:00:00Z', read: true }
const unreadItem = { id: '2', category: 'organization', body: '部门与角色已更新', createdAt: '2026-11-22T02:00:00Z', read: false }
const page = (items: typeof readItem[], unread: number) => ({
  items,
  unread,
  through: '2',
  total: items.length,
  page: 1,
  pageSize: 5,
  sort: 'newest' as const,
})

const allPage = page([unreadItem, readItem], 3)
const unreadPage = page([unreadItem], 3)

function renderBell() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={client}><MemoryRouter><NotificationBell /></MemoryRouter></QueryClientProvider>)
}

describe('NotificationBell', () => {
  beforeEach(() => {
    vi.mocked(useInboxBell).mockImplementation((unread = false) => ({
      data: unread ? unreadPage : allPage,
      isError: false,
      isPending: false,
      refetch: vi.fn(),
    }) as never)
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('exposes the unread count on an accessible trigger', () => {
    renderBell()
    const trigger = screen.getByRole('button', { name: '消息中心，未读 3' })
    expect(trigger).toHaveAttribute('aria-expanded', 'false')
    expect(trigger).toHaveAttribute('aria-haspopup', 'dialog')
  })

  it('opens on hover, is hoverable and never marks a message read', async () => {
    const { container } = renderBell()
    fireEvent.mouseEnter(container.querySelector('.bell-root')!)
    const panel = await screen.findByRole('dialog', { name: '近期消息' })
    expect(panel).toHaveTextContent('近期消息')
    expect(panel).toHaveTextContent('部门与角色已更新')
    expect(panel).toHaveTextContent('11-22 10:00')
    expect(within(panel).getByRole('link', { name: '查看全部' })).toBeVisible()
    expect(within(panel).queryByRole('button', { name: '标为已读' })).not.toBeInTheDocument()
    fireEvent.mouseEnter(panel)
    expect(screen.getByRole('dialog', { name: '近期消息' })).toBeVisible()
    expect(useInboxBell).toHaveBeenCalled()
  })

  it('closes on Escape and returns focus to the bell', async () => {
    const { container } = renderBell()
    const trigger = screen.getByRole('button', { name: '消息中心，未读 3' })
    fireEvent.mouseEnter(container.querySelector('.bell-root')!)
    await screen.findByRole('dialog', { name: '近期消息' })
    fireEvent.keyDown(trigger, { key: 'Escape' })
    await waitFor(() => expect(screen.queryByRole('dialog', { name: '近期消息' })).not.toBeInTheDocument())
    expect(trigger).toHaveFocus()
  })

  it('toggles on click so touch users can open and close it', async () => {
    renderBell()
    const trigger = screen.getByRole('button', { name: '消息中心，未读 3' })
    fireEvent.click(trigger)
    expect(await screen.findByRole('dialog', { name: '近期消息' })).toBeVisible()
    expect(trigger).toHaveAttribute('aria-expanded', 'true')
    fireEvent.click(trigger)
    await waitFor(() => expect(screen.queryByRole('dialog', { name: '近期消息' })).not.toBeInTheDocument())
  })

  it('keeps the panel open on the first touch tap when focus was elsewhere', async () => {
    renderBell()
    const trigger = screen.getByRole('button', { name: '消息中心，未读 3' })
    const root = trigger.parentElement!

    // 触屏把焦点从别处移到铃铛：pointerdown → focusin → pointerup → click。
    // onFocusCapture 不能先 show 再让 click toggle 关闭。
    fireEvent.pointerDown(trigger)
    fireEvent.focusIn(root)
    fireEvent.pointerUp(trigger)
    fireEvent.click(trigger)
    expect(await screen.findByRole('dialog', { name: '近期消息' })).toBeVisible()
    expect(trigger).toHaveAttribute('aria-expanded', 'true')

    // 再次 tap 才关闭。
    fireEvent.pointerDown(trigger)
    fireEvent.pointerUp(trigger)
    fireEvent.click(trigger)
    await waitFor(() => expect(screen.queryByRole('dialog', { name: '近期消息' })).not.toBeInTheDocument())
    expect(trigger).toHaveAttribute('aria-expanded', 'false')
  })

  it('keeps the panel usable when the mouse leaves while keyboard focus is inside', async () => {
    const { container } = renderBell()
    const root = container.querySelector('.bell-root')!
    fireEvent.mouseEnter(root)
    const panel = await screen.findByRole('dialog', { name: '近期消息' })
    const viewAll = within(panel).getByRole('link', { name: '查看全部' })
    viewAll.focus()
    expect(viewAll).toHaveFocus()

    fireEvent.mouseLeave(root)
    await new Promise(resolve => setTimeout(resolve, 250)) // 超过 closeDelayMs
    expect(screen.getByRole('dialog', { name: '近期消息' })).toBeVisible()
    expect(viewAll).toBeEnabled()

    // 焦点真正离开面板后才关闭。
    fireEvent.focusOut(viewAll, { relatedTarget: document.body })
    await waitFor(() => expect(screen.queryByRole('dialog', { name: '近期消息' })).not.toBeInTheDocument())
  })

  it('switches tabs both ways using the server unread filter, never a client-side slice', async () => {
    renderBell()
    fireEvent.click(screen.getByRole('button', { name: '消息中心，未读 3' }))
    const panel = await screen.findByRole('dialog', { name: '近期消息' })

    const all = within(panel).getByRole('tab', { name: /全部/ })
    const unread = within(panel).getByRole('tab', { name: /未读/ })
    expect(all).toHaveAttribute('aria-selected', 'true')
    expect(within(panel).getByText('个人授权规则已启用')).toBeInTheDocument()

    fireEvent.click(unread)
    expect(unread).toHaveAttribute('aria-selected', 'true')
    expect(all).toHaveAttribute('aria-selected', 'false')
    // 未读桶由 useInboxBell(true) 从服务端过滤，最近五条之外的未读也会返回。
    expect(useInboxBell).toHaveBeenCalledWith(true)
    expect(within(panel).queryByText('个人授权规则已启用')).not.toBeInTheDocument()
    expect(within(panel).getByText('部门与角色已更新')).toBeInTheDocument()

    fireEvent.click(all)
    expect(all).toHaveAttribute('aria-selected', 'true')
    expect(within(panel).getByText('个人授权规则已启用')).toBeInTheDocument()
  })

  it('shows the shared global unread count on the unread tab', async () => {
    renderBell()
    fireEvent.click(screen.getByRole('button', { name: '消息中心，未读 3' }))
    const panel = await screen.findByRole('dialog', { name: '近期消息' })
    const unread = within(panel).getByRole('tab', { name: /未读/ })
    expect(unread).toHaveTextContent('3')
    expect(unread).toHaveTextContent('3 条未读')
  })

  it('uses a correct single-sentence empty state for each tab', async () => {
    vi.mocked(useInboxBell).mockImplementation(() => ({
      data: page([], 0),
      isError: false,
      isPending: false,
      refetch: vi.fn(),
    }) as never)
    renderBell()
    fireEvent.click(screen.getByRole('button', { name: '消息中心，未读 0' }))
    const panel = await screen.findByRole('dialog', { name: '近期消息' })
    expect(within(panel).getByText('暂无消息')).toBeInTheDocument()
    fireEvent.click(within(panel).getByRole('tab', { name: /未读/ }))
    expect(within(panel).getByText('暂无未读消息')).toBeInTheDocument()
    expect(within(panel).queryByText('暂无消息')).not.toBeInTheDocument()
  })

  it('never writes read state when switching tabs repeatedly', async () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('{}', { status: 200 }))
    renderBell()
    fireEvent.click(screen.getByRole('button', { name: '消息中心，未读 3' }))
    const panel = await screen.findByRole('dialog', { name: '近期消息' })
    const all = within(panel).getByRole('tab', { name: /全部/ })
    const unread = within(panel).getByRole('tab', { name: /未读/ })
    for (let i = 0; i < 3; i++) {
      fireEvent.click(unread)
      fireEvent.click(all)
    }
    const writes = fetchSpy.mock.calls.filter(([input]) => {
      const url = typeof input === 'string' ? input : (input as Request).url
      return url.includes('/notifications/read')
    })
    expect(writes).toHaveLength(0)
    expect(screen.getByRole('button', { name: '消息中心，未读 3' })).toBeInTheDocument()
  })

  it('supports arrow keys between tabs and Escape from a focused tab', async () => {
    const { container } = renderBell()
    fireEvent.mouseEnter(container.querySelector('.bell-root')!)
    const panel = await screen.findByRole('dialog', { name: '近期消息' })
    const all = within(panel).getByRole('tab', { name: /全部/ })
    const unread = within(panel).getByRole('tab', { name: /未读/ })
    all.focus()
    fireEvent.keyDown(all, { key: 'ArrowRight' })
    expect(unread).toHaveAttribute('aria-selected', 'true')
    expect(unread).toHaveFocus()
    fireEvent.keyDown(unread, { key: 'ArrowLeft' })
    expect(all).toHaveAttribute('aria-selected', 'true')
    expect(all).toHaveFocus()
    fireEvent.keyDown(all, { key: 'Escape' })
    await waitFor(() => expect(screen.queryByRole('dialog', { name: '近期消息' })).not.toBeInTheDocument())
    expect(screen.getByRole('button', { name: '消息中心，未读 3' })).toHaveFocus()
  })
})
