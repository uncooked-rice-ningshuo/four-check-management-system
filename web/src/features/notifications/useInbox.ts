import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  defaultInboxQuery,
  getInbox,
  inboxQueryKey,
  markInboxRead,
  type InboxItem,
  type InboxPage,
  type InboxQuery,
} from '../../shared/api/inboxApi'

export function useInbox(query: InboxQuery = defaultInboxQuery) {
  return useQuery({ queryKey: inboxQueryKey(query), queryFn: () => getInbox(query), refetchInterval: 60000 })
}

// 顶栏铃铛复用同一套 ['inbox'] 缓存，鼠标悬停只读取，不触发任何已读写入。
// unread 下推到服务端筛选：未读页签显示的是本人全部未读消息，而不是最近五条里再过滤。
export function useInboxBell(unread = false) {
  return useInbox({ ...defaultInboxQuery, pageSize: 5, unread })
}

type ReadInput = { id?: string; ids?: string[]; all?: boolean }

function isAffected(item: InboxItem, input: ReadInput): boolean {
  if (input.all) return Number(item.id) <= Number(input.id ?? 0)
  if (input.ids?.length) return input.ids.includes(item.id)
  if (input.id) return item.id === input.id
  return false
}

// 每次已读写入后，同一事务性地修正所有 inbox 缓存桶：未读数用服务端返回的 changed
// 统一扣减，当前页条目立即翻转；随后失效让服务端排序/筛选/分页成为最终事实来源。
// 这避免 Novu #10817 中「某一筛选桶更新、计数桶未更新」造成的角标漂移。
export function useMarkInboxRead() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: markInboxRead,
    onSuccess: (result, input) => {
      const changed = Math.max(0, result?.changed ?? 0)
      // setQueriesData 的 updater 只拿到旧值；逐个 query 设置才能按各自筛选条件更新。
      client.getQueryCache().findAll({ queryKey: ['inbox'] }).forEach(query => {
        const filter = (query.queryKey[1] ?? {}) as InboxQuery
        client.setQueryData<InboxPage>(query.queryKey, old => {
          if (!old) return old
          const newlyRead = new Set(old.items.filter(item => !item.read && isAffected(item, input)).map(item => item.id))
          const items = old.items.map(item => newlyRead.has(item.id) ? { ...item, read: true } : item)
          const visible = filter.unread ? items.filter(item => !newlyRead.has(item.id)) : items
          return {
            ...old,
            unread: Math.max(0, old.unread - changed),
            items: visible,
            total: filter.unread ? Math.max(0, old.total - newlyRead.size) : old.total,
          }
        })
      })
      void client.invalidateQueries({ queryKey: ['inbox'] })
    },
  })
}
