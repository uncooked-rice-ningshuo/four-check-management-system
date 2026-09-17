import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { beforeEach, expect, it, vi } from 'vitest'
import { WorkContextBar } from './WorkContextBar'
import { getWorkContext, setWorkContext } from '../../shared/api/workContextClient'
vi.mock('../../shared/api/workContextClient', () => ({ getWorkContext: vi.fn(), setWorkContext: vi.fn() }))
vi.mock('../../shared/api/adminApi', () => ({ getAcademicCalendar: vi.fn().mockResolvedValue({ academicYears: [], semesters: [], weeks: [] }) }))
function show(manage = false) { return render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}><WorkContextBar canManage={manage} /></QueryClientProvider>) }
beforeEach(() => vi.clearAllMocks())
it('shows loading and an empty teaching week without a management button', async () => {
  vi.mocked(getWorkContext).mockResolvedValue({ week: null, source: 'NONE', version: 1 })
  show()
  expect(screen.getByRole('status')).toBeInTheDocument()
  expect(await screen.findByText('当前无有效教学周，请选择工作范围')).toBeInTheDocument()
  expect(screen.queryByText('设置临时工作周')).not.toBeInTheDocument()
})
it('shows an actionable read failure', async () => {
  vi.mocked(getWorkContext).mockRejectedValue(new Error('failure'))
  show()
  expect(await screen.findByText('读取工作周次失败')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: '重试' })).toBeInTheDocument()
})
it('displays override status and sends the captured version when restoring', async () => {
  vi.mocked(getWorkContext).mockResolvedValue({ week: null, source: 'OVERRIDE', expiresAt: '2026-09-07T10:00:00Z', version: 4 })
  vi.mocked(setWorkContext).mockResolvedValue({ week: null, source: 'NONE', version: 5 })
  show(true)
  await screen.findByText(/临时覆盖/)
  fireEvent.click(screen.getByRole('button', { name: '设置临时工作周' }))
  fireEvent.change(screen.getByLabelText('覆盖原因'), { target: { value: '恢复正常工作' } })
  fireEvent.click(screen.getByRole('button', { name: '恢复校历' }))
  await waitFor(() => expect(setWorkContext).toHaveBeenCalledWith({ version: 4, weekId: '', expiresAt: null, reason: '恢复正常工作' }, expect.anything()))
})
