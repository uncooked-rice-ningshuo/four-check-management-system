// 隔离证据用：替换 features/notifications/useInbox，避免连接真实账号或写入数据库。
// mode 来自 URL 参数：empty 无数据、data 混合已读/未读、unread 仅未读（模仿服务端 unread filter）。

type StubItem = {
  id: string
  category: string
  action?: string
  body: string
  createdAt: string
  read: boolean
}

const dataset: StubItem[] = [
  { id: '5', category: 'organization', body: '部门归属已调整为计算机学院学生会文艺部', createdAt: '2026-11-22T09:40:00Z', read: false },
  { id: '4', category: 'permission', action: 'INSPECTION_VIEW', body: '个人授权新增：可查看教室卫生检查', createdAt: '2026-11-22T08:20:00Z', read: false },
  { id: '3', category: 'organization', body: '组织角色已变更为部长', createdAt: '2026-11-21T15:10:00Z', read: false },
  { id: '2', category: 'permission', action: 'REPORT_VIEW', body: '个人授权调整：通报查看范围已更新', createdAt: '2026-11-21T11:05:00Z', read: true },
  { id: '1', category: 'organization', body: '部门与角色配置已同步', createdAt: '2026-11-20T10:00:00Z', read: true },
]

function currentMode(): string {
  return new URLSearchParams(window.location.search).get('mode') ?? 'data'
}

export function useInboxBell(unread = false) {
  const mode = currentMode()
  const allItems = mode === 'empty' ? [] : dataset
  const items = unread ? allItems.filter(item => !item.read) : allItems
  const unreadCount = allItems.filter(item => !item.read).length
  return {
    data: {
      items,
      total: items.length,
      unread: unreadCount,
      through: '5',
      page: 1,
      pageSize: 5,
      sort: 'newest' as const,
    },
    isPending: false,
    isError: false,
    refetch: () => undefined,
  }
}

// 兼容可能仍被导入的其他符号。
export function useInbox() {
  return useInboxBell()
}
