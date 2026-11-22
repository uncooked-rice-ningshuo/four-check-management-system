import { apiRequest } from './httpClient'

export type InboxSort = 'newest' | 'oldest' | 'unread-first' | 'read-first'

export interface InboxQuery {
  page: number
  pageSize: number
  unread: boolean
  category: string
  from: string
  to: string
  sort: InboxSort
}

export interface InboxItem {
  id: string
  category: string
  caseId?: string | null
  eventStatus?: string | null
  relatedDeferredIds?: string[] | null
  action?: string
  body: string
  createdAt: string
  read: boolean
}

export interface InboxPage {
  items: InboxItem[]
  total: number
  unread: number
  through: string
  page: number
  pageSize: number
  sort: InboxSort
}

export interface InboxMarkReadResult {
  ok: boolean
  changed: number
}

export const defaultInboxQuery: InboxQuery = {
  page: 1,
  pageSize: 20,
  unread: false,
  category: '',
  from: '',
  to: '',
  sort: 'newest',
}

export function inboxQueryKey(query: InboxQuery) {
  return ['inbox', query] as const
}

export function getInbox(query: InboxQuery) {
  const params = new URLSearchParams({
    page: String(query.page),
    pageSize: String(query.pageSize),
    unread: String(query.unread),
    category: query.category,
    from: query.from,
    to: query.to,
    sort: query.sort,
  })
  return apiRequest<InboxPage>(`/notifications?${params.toString()}`)
}

// all 使用 { id: through, all: true } 兼容既有冻结边界协议；批量使用 ids。
export function markInboxRead(input: { id?: string; ids?: string[]; all?: boolean }) {
  return apiRequest<InboxMarkReadResult>('/notifications/read', {
    method: 'POST',
    body: JSON.stringify(input),
  })
}
