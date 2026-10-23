import { act, renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider, useQuery } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { useConfirmImport } from './useImportBatch'
import { ApiError } from '../../../shared/api/httpClient'
import { importJobApi } from '../api/importJobApi'

vi.mock('../api/importJobApi', () => ({ importJobApi: { confirm: vi.fn() } }))

describe('import confirmation cache refresh', () => {
  it('reloads the authoritative batch after a version conflict', async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
    let version = 3
    vi.mocked(importJobApi.confirm).mockImplementation(async () => {
      version = 4
      throw new ApiError(409, 'VERSION_CONFLICT', 'conflict')
    })
    const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={client}>{children}</QueryClientProvider>
    const { result } = renderHook(() => ({
      batch: useQuery({ queryKey: ['import-jobs', 'batch', 'batch'], queryFn: async () => version }),
      confirm: useConfirmImport('batch'),
    }), { wrapper })
    await waitFor(() => expect(result.current.batch.data).toBe(3))
    await act(async () => { await expect(result.current.confirm.mutateAsync({ version: 3, idempotencyKey: 'conflict' })).rejects.toMatchObject({ code: 'VERSION_CONFLICT' }) })
    await waitFor(() => expect(result.current.batch.data).toBe(4))
    client.clear()
  })

  it('refreshes an all-departments list and the workbench after importing into one department', async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
    const context = { semesterId: 'semester', weekId: 'week', inspectionTypeId: 'attendance', departmentId: 'study' }
    let imported = false
    vi.mocked(importJobApi.confirm).mockImplementation(async () => {
      imported = true
      return { batchId: 'batch', importedRows: 2, status: 'CONFIRMED', version: 2 }
    })
    const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={client}>{children}</QueryClientProvider>
    const { result } = renderHook(() => ({
      list: useQuery({ queryKey: ['attendance', 'list', { ...context, departmentId: '' }], queryFn: async () => imported ? 2 : 0 }),
      classroomHygiene: useQuery({ queryKey: ['classroom-hygienes','list',{}], queryFn: async () => imported ? 2 : 0 }),
      hygiene: useQuery({ queryKey: ['dormitory-hygienes','list',{}], queryFn: async () => imported ? 2 : 0 }),
      dashboard: useQuery({ queryKey: ['dashboard', 'workbench'], queryFn: async () => imported ? 2 : 0 }),
      confirm: useConfirmImport('batch'),
    }), { wrapper })
    await waitFor(() => expect(result.current.list.data).toBe(0))
    await act(() => result.current.confirm.mutateAsync({ version: 1, idempotencyKey: 'test-key' }))
    await waitFor(() => expect(result.current.list.data).toBe(2))
    expect(result.current.dashboard.data).toBe(2)
    expect(result.current.hygiene.data).toBe(2)
    expect(result.current.classroomHygiene.data).toBe(2)
    client.clear()
  })
})
