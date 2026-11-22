import { useEffect, useRef, useState } from 'react'
import type { KeyboardEvent as ReactKeyboardEvent, ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { Badge, Button, Spin } from 'antd'
import { ApartmentOutlined, BellOutlined, SafetyCertificateOutlined } from '@ant-design/icons'
import { useInboxBell } from './useInbox'
import './NotificationBell.css'

type BellTab = 'all' | 'unread'

const categoryMeta: Record<string, { label: string; icon: ReactNode }> = {
  organization: { label: '部门与角色变更', icon: <ApartmentOutlined /> },
  permission: { label: '个人授权变更', icon: <SafetyCertificateOutlined /> },
}

const fallbackMeta = { label: '消息变更', icon: <BellOutlined /> }

const openDelayMs = 120
const closeDelayMs = 180

function timeParts(iso: string) {
  return new Intl.DateTimeFormat('zh-CN', {
    timeZone: 'Asia/Shanghai',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).formatToParts(new Date(iso))
}

function shortTime(iso: string) {
  const parts = timeParts(iso)
  const part = (type: string) => parts.find(item => item.type === type)?.value ?? ''
  return `${part('month')}-${part('day')} ${part('hour')}:${part('minute')}`
}

function fullTime(iso: string) {
  return `${new Date(iso).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai', hour12: false })} 北京时间`
}

export function NotificationBell() {
  const [open, setOpen] = useState(false)
  const [tab, setTab] = useState<BellTab>('all')
  const containerRef = useRef<HTMLDivElement>(null)
  const triggerRef = useRef<HTMLButtonElement>(null)
  const openTimer = useRef<number | undefined>(undefined)
  const closeTimer = useRef<number | undefined>(undefined)
  const restoringFocus = useRef(false)
  // 触屏/鼠标按下取得的焦点不应由 onFocusCapture 弹出面板：紧接着的 click 会负责打开，
  // 否则同一 tap 会先 show 再 toggle 关闭，首次点击看起来毫无反应。
  const pointerDown = useRef(false)

  // 两个缓存桶常驻：全部用于角标与「全部」页签，未读桶直接用服务端 unread 过滤。
  // 悬停/切换只读，绝不触发已读写入。
  const allInbox = useInboxBell(false)
  const unreadInbox = useInboxBell(true)
  const inbox = tab === 'unread' ? unreadInbox : allInbox
  const globalUnread = allInbox.data?.unread ?? unreadInbox.data?.unread

  const cancelTimers = () => {
    window.clearTimeout(openTimer.current)
    window.clearTimeout(closeTimer.current)
  }
  const show = () => {
    cancelTimers()
    setOpen(true)
  }
  const hide = () => {
    cancelTimers()
    setOpen(false)
  }
  const scheduleShow = () => {
    window.clearTimeout(closeTimer.current)
    window.clearTimeout(openTimer.current)
    openTimer.current = window.setTimeout(() => setOpen(true), openDelayMs)
  }
  const scheduleHide = () => {
    window.clearTimeout(openTimer.current)
    window.clearTimeout(closeTimer.current)
    closeTimer.current = window.setTimeout(() => {
      // 鼠标移开但键盘焦点仍在面板内时不能关闭，否则键盘用户一移动鼠标就失去操作目标。
      if (containerRef.current?.contains(document.activeElement)) return
      setOpen(false)
    }, closeDelayMs)
  }

  useEffect(() => {
    if (!open) return
    const onPointerDown = (event: PointerEvent) => {
      if (!containerRef.current?.contains(event.target as Node)) {
        window.clearTimeout(openTimer.current)
        window.clearTimeout(closeTimer.current)
        setOpen(false)
      }
    }
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      event.preventDefault()
      restoringFocus.current = true
      setOpen(false)
      triggerRef.current?.focus()
      window.setTimeout(() => { restoringFocus.current = false }, 0)
    }
    document.addEventListener('pointerdown', onPointerDown)
    document.addEventListener('keydown', onKeyDown)
    return () => {
      document.removeEventListener('pointerdown', onPointerDown)
      document.removeEventListener('keydown', onKeyDown)
    }
  }, [open])

  useEffect(() => () => cancelTimers(), [])

  const onTabKeyDown = (event: ReactKeyboardEvent<HTMLButtonElement>) => {
    if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') return
    event.preventDefault()
    const next: BellTab = tab === 'all' ? 'unread' : 'all'
    setTab(next)
    const tabs = event.currentTarget.parentElement?.querySelectorAll<HTMLButtonElement>('[role="tab"]')
    tabs?.[next === 'all' ? 0 : 1]?.focus()
  }

  const badgeCount = allInbox.isError ? '!' : globalUnread
  const label = allInbox.isError
    ? '消息中心，读取失败'
    : globalUnread === undefined ? '消息中心，未读加载中' : `消息中心，未读 ${globalUnread}`

  return <div
    ref={containerRef}
    className="bell-root"
    onMouseEnter={scheduleShow}
    onMouseLeave={scheduleHide}
    onFocusCapture={() => {
      // 键盘 Tab 进入时自动弹出；指针取得的焦点交给后续 click 打开，避免同一次 tap 打开又关闭。
      if (restoringFocus.current || pointerDown.current) return
      show()
    }}
    onBlur={event => {
      if (!event.currentTarget.contains(event.relatedTarget as Node | null)) hide()
    }}
  >
    <button
      ref={triggerRef}
      type="button"
      className="bell-trigger"
      aria-label={label}
      aria-expanded={open}
      aria-haspopup="dialog"
      aria-controls="bell-panel"
      onPointerDown={() => { pointerDown.current = true }}
      onClick={() => {
        pointerDown.current = false
        if (open) hide(); else show()
      }}
    >
      <Badge count={badgeCount} size="small" offset={[2, -2]}>
        <BellOutlined style={{ fontSize: 22 }} />
      </Badge>
    </button>
    {open && <div id="bell-panel" className="bell-panel" role="dialog" aria-label="近期消息">
      <header className="bell-panel__head">
        <strong>近期消息</strong>
        <Link to="/notifications" onClick={hide}>查看全部</Link>
      </header>
      <div className="bell-panel__tabs" role="tablist" aria-label="消息筛选">
        <button
          type="button"
          role="tab"
          aria-selected={tab === 'all'}
          className="bell-panel__tab"
          onClick={() => setTab('all')}
          onKeyDown={onTabKeyDown}
        >全部</button>
        <button
          type="button"
          role="tab"
          aria-selected={tab === 'unread'}
          className="bell-panel__tab"
          onClick={() => setTab('unread')}
          onKeyDown={onTabKeyDown}
        >
          未读
          {globalUnread ? <span className="bell-panel__tab-count" aria-hidden="true">{globalUnread}</span> : null}
          <span className="bell-sr-only">{globalUnread ? `，${globalUnread} 条未读` : '，无未读'}</span>
        </button>
      </div>
      {inbox.isError ? <div className="bell-panel__state" role="alert">
        读取失败，<Button type="link" size="small" onClick={() => void inbox.refetch()}>重试</Button>
      </div> : inbox.isPending ? <div className="bell-panel__state" aria-busy="true">
        <Spin size="small" /> 正在读取…
      </div> : inbox.data?.items.length ? <ul
        className="bell-panel__list"
        role="tabpanel"
        aria-label={tab === 'all' ? '全部消息' : '未读消息'}
      >
        {inbox.data.items.map(item => {
          const meta = categoryMeta[item.category] ?? fallbackMeta
          return <li key={item.id} className={`bell-item${item.read ? '' : ' bell-item--unread'}`}>
            <span className="bell-item__icon" aria-hidden="true">{meta.icon}</span>
            <div className="bell-item__content">
              <div className="bell-item__meta">
                <span className="bell-item__category">{meta.label}</span>
                <time className="bell-item__time" dateTime={item.createdAt} title={fullTime(item.createdAt)}>
                  {shortTime(item.createdAt)}
                </time>
              </div>
              <p className="bell-item__body">{item.body}</p>
            </div>
            {!item.read && <span className="bell-item__dot" role="img" aria-label="未读" />}
          </li>
        })}
      </ul> : <div className="bell-panel__state">
        {tab === 'unread' ? '暂无未读消息' : '暂无消息'}
      </div>}
    </div>}
  </div>
}
