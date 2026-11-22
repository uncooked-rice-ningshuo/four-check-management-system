import { Link } from 'react-router-dom'
import { permissionActionName } from '../../entities/permission/model/actions'
import { useCallback, useEffect, useRef, useState } from 'react'
import { ProTable, type ProColumns } from '@ant-design/pro-components'
import { Alert, Button, Modal, Select, Space, Tag, Typography } from 'antd'
import { ArrowLeftOutlined, CheckOutlined, FileTextOutlined, ApartmentOutlined, SafetyCertificateOutlined } from '@ant-design/icons'
import { useInbox, useMarkInboxRead } from '../../features/notifications/useInbox'
import type { InboxItem, InboxSort } from '../../shared/api/inboxApi'
import './InboxPage.css'

const pageSize = 20

// 后端只支持“全部/仅未读”，阅读状态下拉不得伪造“仅已读”。
type ReadStatus = 'all' | 'unread'

interface InboxFilters {
  category: string
  from: string
  to: string
  unread: boolean
}

interface InboxSearchValues {
  category?: 'all' | 'organization' | 'permission' | 'discipline'
  createdAt?: [string, string]
  read?: ReadStatus
}

const emptyFilters: InboxFilters = { category: '', from: '', to: '', unread: false }

const sortOptions: { value: InboxSort; label: string }[] = [
  { value: 'newest', label: '最新优先' },
  { value: 'oldest', label: '最早优先' },
  { value: 'unread-first', label: '未读优先' },
  { value: 'read-first', label: '已读优先' },
]

const categoryTitle: Record<string, string> = {
  organization: '部门与角色变更',
  permission: '个人授权变更',
  discipline: '处分事项',
}

function categoryIcon(category: string) {
  if (category === 'discipline') return <FileTextOutlined aria-hidden="true" />
  return category === 'permission' ? <SafetyCertificateOutlined aria-hidden="true" />
    : <ApartmentOutlined aria-hidden="true" />
}

function noticeSubject(item: InboxItem) {
  const action = item.action ? permissionActionName(item.action) : ''
  return action ? `${action} · ${item.body.split('。', 1)[0]}` : categoryTitle[item.category] ?? '账号变更'
}

function NoticeLetter({ item, recipientName }: { item: InboxItem; recipientName?: string }) {
  if (item.category === 'discipline') return <p>{item.body}</p>
  const [opening, ...paragraphs] = item.body.split('。').filter(Boolean)
  const fields = paragraphs.join('。').split('；').filter(Boolean)
  return <>
    <p>{recipientName ? `${recipientName}，您好！` : '您好！'}</p>
    <p>{item.action ? `您账号中「${permissionActionName(item.action)}」的` : '您的'}{opening}。</p>
    {fields.length > 0 && <dl className="inbox-detail__fields">{fields.map((field, index) => {
      const split = field.indexOf('：')
      return split < 0 ? <div key={index}><dd>{field}</dd></div> : <div key={index} className="inbox-detail__field">
        <dt>{field.slice(0, split)}</dt><dd>{field.slice(split + 1)}</dd>
      </div>
    })}</dl>}
    <p className="inbox-letter-signature">学生日常管理系统</p>
  </>
}

// 全部标为已读的提交边界在弹窗打开时冻结；轮询期间新到的消息不属于用户确认时看到的未读集合。
interface ReadAllBoundary {
  through: string
  unread: number
}

function messageTime(iso: string): string {
  return new Date(iso).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai' })
}

// 分栏后时间列较窄，只保留月日与时分；完整时间仍由 time 属性和详情区承载。
function shortMessageTime(iso: string): string {
  return new Date(iso).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false })
}

export function InboxPage({ recipientName }: { recipientName?: string } = {}) {
  const [page, setPage] = useState(1)
  const [filters, setFilters] = useState<InboxFilters>(emptyFilters)
  const [sort, setSort] = useState<InboxSort>('newest')
  const [selected, setSelected] = useState<string[]>([])
  const [notice, setNotice] = useState('')
  const [confirmBoundary, setConfirmBoundary] = useState<ReadAllBoundary | null>(null)
  const [confirmBatch, setConfirmBatch] = useState(false)
  // 详情只存编号：记录随筛选/缓存变化重新解析，翻页或读掉当前条目后不会留下旧记录。
  const [detailId, setDetailId] = useState<string | null>(null)
  const detailRef = useRef<HTMLElement | null>(null)
  const detailOpenerRef = useRef<HTMLElement | null>(null)
  const inbox = useInbox({
    page,
    pageSize,
    unread: filters.unread,
    category: filters.category,
    from: filters.from,
    to: filters.to,
    sort,
  })
  const read = useMarkInboxRead()
  const items = inbox.data?.items ?? []
  const unreadTotal = inbox.data?.unread ?? 0
  const maxPage = inbox.data ? Math.max(1, Math.ceil(inbox.data.total / pageSize)) : 1
  const currentPage = Math.min(page, maxPage)

  // 仅看未读时读掉末页最后一项会让当前页越界；渲染期校正到最后一个有效页并重新取数。
  if (inbox.data && !inbox.isError && page > maxPage) setPage(maxPage)

  const detailItem = items.find(item => item.id === detailId) ?? null
  const compact = detailItem !== null

  const resetDetail = useCallback(() => {
    setDetailId(null)
    detailOpenerRef.current = null
  }, [])

  const closeDetail = useCallback(() => {
    const opener = detailOpenerRef.current
    resetDetail()
    // 焦点还给打开详情的标题/状态按钮；筛选后该行已卸载时直接放弃。
    if (opener?.isConnected) opener.focus()
  }, [resetDetail])

  const openDetail = useCallback((id: string, opener?: HTMLElement | null) => {
    detailOpenerRef.current = opener ?? null
    setDetailId(id)
  }, [])

  // 详情是非模态侧栏，打开后把焦点移入阅读区；Escape 在文档级关闭并把焦点还给触发控件。
  useEffect(() => {
    if (detailId) detailRef.current?.focus()
  }, [detailId])

  useEffect(() => {
    if (!detailId || confirmBoundary || confirmBatch) return
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      event.preventDefault()
      closeDetail()
    }
    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  }, [detailId, confirmBoundary, confirmBatch, closeDetail])

  function applyFilters(next: InboxFilters) {
    setFilters(next)
    setPage(1)
    setSelected([])
    setNotice('')
    resetDetail()
  }

  function changeSort(value: InboxSort) {
    setSort(value)
    setPage(1)
    setSelected([])
    setNotice('')
    resetDetail()
  }

  function onSearch(values: InboxSearchValues) {
    const [from = '', to = ''] = values.createdAt ?? []
    applyFilters({ category: values.category === 'all' ? '' : values.category ?? '', from, to, unread: values.read === 'unread' })
  }

  async function markOne(id: string) {
    setNotice('')
    try {
      await read.mutateAsync({ id })
      setNotice('已将 1 条消息标为已读')
    } catch {
      // 失败状态由 read.isError 在页面内展示，输入与未读行保持不变。
    }
  }

  async function markSelected() {
    setNotice('')
    const ids = selected
    try {
      const result = await read.mutateAsync({ ids })
      setSelected([])
      setConfirmBatch(false)
      setNotice(`已将选中的 ${result.changed || ids.length} 条消息标为已读`)
    } catch {
      // 同上，保留选择便于重试。
    }
  }

  function openConfirmAll() {
    if (!inbox.data || !inbox.data.unread) return
    setConfirmBoundary({ through: inbox.data.through, unread: inbox.data.unread })
  }

  async function markAll(boundary: ReadAllBoundary) {
    setNotice('')
    try {
      await read.mutateAsync({ id: boundary.through, all: true })
      setNotice(`已将本人全部 ${boundary.unread} 条未读消息标为已读（不限当前筛选）`)
    } catch {
      // 同上，失败提示由页面统一展示。
    }
  }

  const columns: ProColumns<InboxItem>[] = [
    {
      title: '消息类别',
      dataIndex: 'category',
      width: 120,
      valueType: 'select',
      valueEnum: { all: { text: '全部类别' }, organization: { text: '部门与角色' }, permission: { text: '个人授权' }, discipline: { text: '处分事项' } },
      fieldProps: { placeholder: '全部类别', allowClear: true },
      // 分栏时列表只保留摘要列，类别仍在查询表单与详情中完整呈现。
      hideInTable: compact,
      render: (_, record) => <Tag variant="filled" color={record.read ? undefined : 'blue'}>
        {noticeSubject(record)}
      </Tag>,
      responsive: ['sm'],
    },
    {
      title: '消息内容',
      dataIndex: 'body',
      search: false,
      render: (_, record) => <div className="inbox-cell-body">
        <button
          type="button"
          className="inbox-cell-title inbox-cell-open"
          onClick={event => { event.stopPropagation(); openDetail(record.id, event.currentTarget) }}
        >
          {noticeSubject(record)}
        </button>

        {record.category === 'discipline' ? record.body : record.body.split('。').filter(Boolean)[1]?.split('；')[0] ?? record.body}
        <span className="inbox-cell-status-mobile" aria-hidden="true">
          <Tag color={record.read ? 'default' : 'blue'}>{record.read ? '已读' : '未读'}</Tag>
        </span>
        <time className="inbox-cell-time-mobile" dateTime={record.createdAt}>{messageTime(record.createdAt)}</time>
      </div>,
    },
    {
      title: '消息时间',
      dataIndex: 'createdAt',
      width: compact ? 104 : 170,
      responsive: ['md'],
      valueType: 'dateRange',
      fieldProps: { placeholder: ['开始日期', '结束日期'] },
      render: (_, record) => <time className="inbox-cell-time" dateTime={record.createdAt}>
        {compact ? shortMessageTime(record.createdAt) : messageTime(record.createdAt)}
      </time>,
    },
    {
      title: '阅读状态',
      dataIndex: 'read',
      width: compact ? 72 : 96,
      valueType: 'select',
      valueEnum: { all: { text: '全部' }, unread: { text: '仅未读' } },
      fieldProps: { placeholder: '全部', allowClear: false },
      render: (_, record) => <button
        type="button"
        className="inbox-status-open"
        onClick={event => { event.stopPropagation(); openDetail(record.id, event.currentTarget) }}
      >
        <Tag color={record.read ? 'default' : 'blue'}>{record.read ? '已读' : '未读'}</Tag>
      </button>,
      responsive: ['sm'],
    },
    {
      title: '操作',
      valueType: 'option',
      width: compact ? 76 : 88,
      render: (_, record) => record.read ? <span className="inbox-cell-done">—</span>
        : <Button
          type="link"
          size="small"
          disabled={read.isPending}
          onClick={event => { event.stopPropagation(); void markOne(record.id) }}
        >标为已读</Button>,
    },
  ]

  return <section className="admin-page inbox-page">
    <header className="page-heading">
      <div>
        <Typography.Title level={2}>消息中心</Typography.Title>
        <Typography.Paragraph>查看发给本人的组织变更、个人授权和处分事项通知。</Typography.Paragraph>
      </div>
      <p className="inbox-heading-count" aria-live="polite">
        {inbox.isError ? '未读数读取失败' : !inbox.data ? '未读数加载中…' : <>未读 <strong>{inbox.data.unread}</strong> 条</>}
      </p>
    </header>
    {notice && <p className="inbox-status" role="status">{notice}</p>}
    {read.isError && <Alert role="alert" type="error" showIcon title="更新失败，请重试" className="inbox-alert" />}
    {inbox.isError && <Alert
      role="alert"
      type="error"
      showIcon
      title="读取消息失败"
      className="inbox-alert"
      action={<Button onClick={() => void inbox.refetch()}>重试</Button>}
    />}
    <ProTable<InboxItem, InboxSearchValues>
      className="inbox-table"
      rowKey="id"
      cardBordered
      columns={columns}
      dataSource={items}
      loading={inbox.isFetching}
      rowClassName={record => record.read ? '' : 'inbox-row--unread'}
      onRow={record => ({
        onClick: event => {
          const target = event.target as HTMLElement
          // 复选框、单元格按钮与工具栏只执行自身动作，不误开详情。
          if (target.closest('button, a, input, label, .ant-select, .ant-pagination, .ant-pro-table-alert')) return
          const row = event.currentTarget as HTMLElement
          openDetail(record.id, row.querySelector<HTMLElement>('.inbox-cell-open'))
        },
      })}
      tableRender={(_props, tableArea) => <div className={`inbox-workspace${compact ? ' inbox-workspace--open' : ''}`}>
        <div className="inbox-list-pane">{tableArea}</div>
        <aside
          ref={detailRef}
          className={`inbox-detail${detailItem ? ' inbox-detail--open' : ''}`}
          role="region"
          aria-label="消息详情"
          tabIndex={-1}
          aria-hidden={!detailItem}
          inert={!detailItem}
        >
          <div className="inbox-detail__head">
            <span className="inbox-detail__caption">阅读消息</span>
            <Button type="link" icon={<ArrowLeftOutlined aria-hidden />} onClick={closeDetail}>返回列表</Button>
          </div>
          <article className="inbox-detail__content">
            {detailItem && <div key={detailItem.id} className="inbox-detail__message">
            <h3 className="inbox-detail__title">{noticeSubject(detailItem)}</h3>
            <div className="inbox-detail__meta">
              <span className="inbox-detail__category-icon">{categoryIcon(detailItem.category)}<span>{detailItem.category === 'discipline' ? '处分事项' : detailItem.category === 'permission' ? '个人授权' : '组织变更'}</span></span>
              {detailItem.action && <span>{permissionActionName(detailItem.action)}</span>}
              <time dateTime={detailItem.createdAt}>{messageTime(detailItem.createdAt)}</time>
              <span className={`inbox-detail__read-state${detailItem.read ? '' : ' inbox-detail__read-state--unread'}`}>{detailItem.read ? '已读' : '未读'}</span>
            </div>
            <div className="inbox-detail__body"><NoticeLetter item={detailItem} recipientName={recipientName} /></div>
            <div className="inbox-detail__foot">
              {detailItem.caseId && detailItem.eventStatus && <Link
                to={['PENDING', 'DEFERRED'].includes(detailItem.eventStatus) ? '/discipline/review' : '/discipline/archive'}
                state={{ caseId: detailItem.caseId }}>查看对应事项</Link>}
              {detailItem.relatedDeferredIds?.map((id, index, ids) => <Link key={id} to="/discipline/review"
                state={{ caseId: id }}>查看已延迟事项{ids.length > 1 ? index + 1 : ''}</Link>)}
              {!detailItem.read && <Button type="primary" icon={<CheckOutlined aria-hidden />} loading={read.isPending} onClick={() => void markOne(detailItem.id)}>标为已读</Button>}
            </div>
            </div>}
          </article>
        </aside>
      </div>}
      scroll={{ x: compact ? 480 : 'max-content' }}
      search={{ labelWidth: 'auto', defaultCollapsed: false, collapseRender: false, span: { xs: 24, sm: 12, md: 8, lg: 6, xl: 6, xxl: 6 } }}
      onSubmit={onSearch}
      onReset={() => applyFilters(emptyFilters)}
      options={{ reload: () => void inbox.refetch(), density: true, setting: true }}
      toolBarRender={() => [
        <Select<InboxSort>
          key="sort"
          className="inbox-sort-select"
          aria-label="消息排序"
          value={sort}
          options={sortOptions}
          onChange={changeSort}
        />,
        <Button key="read-all" disabled={!unreadTotal || read.isPending} onClick={openConfirmAll}>全部标为已读</Button>,
      ]}
      rowSelection={{
        selectedRowKeys: selected,
        onChange: keys => setSelected(keys.map(String)),
      }}
      tableAlertRender={({ selectedRowKeys }) => <span>
        已选择 <strong>{selectedRowKeys.length}</strong> 条消息
      </span>}
      tableAlertOptionRender={({ onCleanSelected }) => [
        <Button key="mark" type="primary" size="small" disabled={read.isPending} onClick={() => setConfirmBatch(true)}>标为已读</Button>,
        <Button key="clear" type="text" size="small" onClick={onCleanSelected}>取消选择</Button>,
      ]}
      headerTitle={<Space size={10}>
        <span>消息列表</span>
        {inbox.data && !inbox.isError && <Tag variant="filled" color="blue">共 {inbox.data.total} 条</Tag>}
      </Space>}
      pagination={{
        current: currentPage,
        pageSize,
        total: inbox.data?.total ?? 0,
        showSizeChanger: false,
        onChange: next => { setPage(next); setSelected([]); setNotice(''); resetDetail() },
      }}
      locale={{
        emptyText: inbox.isError ? '读取消息失败，请点击上方重试'
          : inbox.isPending ? '正在读取消息…'
            : filters.unread ? '当前仅看未读，切换到全部消息可查看已读记录。'
              : filters.category || filters.from || filters.to ? '暂无符合条件的消息'
                : '收到的通知会显示在这里。',
      }}
    />
    <Modal
      open={confirmBoundary !== null}
      title="将本人全部消息标为已读？"
      okText="确认标为已读"
      cancelText="取消"
      onCancel={() => setConfirmBoundary(null)}
      onOk={() => {
        const boundary = confirmBoundary
        setConfirmBoundary(null)
        if (boundary) void markAll(boundary)
      }}
    >
      <p>此操作覆盖本人全部 {confirmBoundary?.unread ?? 0} 条未读消息，包含其他类别，不受当前筛选影响。</p>
    </Modal>
    <Modal
      open={confirmBatch}
      title="将选中消息标为已读？"
      okText="确认标为已读"
      cancelText="取消"
      onCancel={() => setConfirmBatch(false)}
      onOk={() => void markSelected()}
    >
      <p>仅处理当前账号选中的 {selected.length} 条消息，不影响其他消息或其他账号；重复提交不会产生额外变化。</p>
    </Modal>
  </section>
}
