import { act, renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider, useQuery } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { expect, it, vi } from 'vitest'
import { rollbackImportBatch } from '../../../shared/api/importRollbackApi'
import { useImportRollback } from './useImportRollback'
vi.mock('../../../shared/api/importRollbackApi', () => ({ rollbackImportBatch: vi.fn() }))
it('refreshes every affected inspection list and workbench after a whole batch rollback', async () => {
  let count = 2
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={client}>{children}</QueryClientProvider>
  vi.mocked(rollbackImportBatch).mockImplementation(async () => { count = 0; return { batchId: 'batch', status: 'ROLLED_BACK', rolledBackRows: 2, version: 5 } })
  const { result } = renderHook(() => ({
    records: useQuery({ queryKey: ['attendance', 'list', { departmentId: '' }], queryFn: async () => count }),
    dashboard: useQuery({ queryKey: ['dashboard', 'workbench'], queryFn: async () => count }),
    rollback: useImportRollback('batch'),
  }), { wrapper })
  await waitFor(() => expect(result.current.records.data).toBe(2))
  await act(() => result.current.rollback.mutateAsync({ version: 4, reason: '重复导入' }))
  await waitFor(() => expect(result.current.records.data).toBe(0))
  expect(result.current.dashboard.data).toBe(0)
  client.clear()
})
