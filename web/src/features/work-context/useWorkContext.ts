import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'
import { getWorkContext, setWorkContext } from '../../shared/api/workContextClient'
export function useWorkContext() {
  const client = useQueryClient()
  const query = useQuery({ queryKey: ['work-context'], queryFn: getWorkContext, refetchInterval: 30_000 })
  const contextKey = query.data ? `${query.data.version}:${query.data.source}:${query.data.week?.id ?? ''}` : ''
  useEffect(() => {
    if (!contextKey) return
    void client.invalidateQueries({ queryKey: ['academic', 'current-week'] })
    void client.invalidateQueries({ queryKey: ['dashboard', 'workbench'] })
  }, [client, contextKey])
  return query
}
export function useSetWorkContext() {
  const client = useQueryClient()
  return useMutation({ mutationFn: setWorkContext, onSuccess: async () => {
    await Promise.all([
      client.invalidateQueries({ queryKey: ['work-context'] }),
      client.invalidateQueries({ queryKey: ['academic', 'current-week'] }),
      client.invalidateQueries({ queryKey: ['dashboard', 'workbench'] }),
    ])
  } })
}
