import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { beforeEach, expect, it, vi } from 'vitest'
import { WorkContextBar } from './WorkContextBar'
import { getWorkContext, setWorkContext } from '../../shared/api/workContextClient'
import { getAcademicCalendar } from '../../shared/api/adminApi'
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
it('uses the Ant Design date time picker and sends the chosen Beijing time', async () => {
  vi.mocked(getWorkContext).mockResolvedValue({ week: null, source: 'NONE', version: 4 })
  vi.mocked(getAcademicCalendar).mockResolvedValue({
    academicYears: [{ id: 'year', enabled: true }],
    semesters: [{ id: 'semester', academicYearId: 'year', name: '第一学期', enabled: true }],
    weeks: [{ id: 'week', semesterId: 'semester', weekNo: 1, enabled: true }],
  } as Awaited<ReturnType<typeof getAcademicCalendar>>)
  vi.mocked(setWorkContext).mockResolvedValue({ week: null, source: 'NONE', version: 5 })
  show(true)
  fireEvent.click(await screen.findByRole('button', { name: '设置临时工作周' }))
  fireEvent.mouseDown(screen.getByRole('combobox', { name: '目标周次' }))
  fireEvent.click(await screen.findByText('第一学期 第 1 周'))
  const expiry = screen.getByLabelText('到期时间（北京时间）')
  fireEvent.change(expiry, { target: { value: '2026-10-10 12:00' } })
  fireEvent.keyDown(expiry, { key: 'Enter' })
  await waitFor(() => expect(expiry).toHaveValue('2026-10-10 12:00'))
  fireEvent.change(screen.getByLabelText('覆盖原因'), { target: { value: '调整周次' } })
  const confirm = screen.getByRole('button', { name: '确认全局覆盖' })
  await waitFor(() => expect(confirm).toBeEnabled())
  fireEvent.click(confirm)
  await waitFor(() => expect(setWorkContext).toHaveBeenCalledWith({ version: 4, weekId: 'week', expiresAt: '2026-10-10T04:00:00.000Z', reason: '调整周次' }, expect.anything()))
})
