import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, expect, it, vi } from 'vitest'
import { MorningScheduleDialog } from './MorningScheduleDialog'

const scheduleApi = vi.hoisted(() => ({ get: vi.fn(), save: vi.fn() }))
vi.mock('../../../shared/api/morningScheduleApi', () => ({ morningScheduleApi: scheduleApi }))

const schedule = {
  semesterId: 'semester', weekId: 'week', startDate: '2026-11-02', endDate: '2026-11-06',
  plannedDates: ['2026-11-02', '2026-11-03', '2026-11-04', '2026-11-05', '2026-11-06'],
  version: 2, isDefault: true, canEdit: true,
}

function renderDialog() {
  return render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    <MorningScheduleDialog weekId="week" onClose={vi.fn()} />
  </QueryClientProvider>)
}

beforeEach(() => {
  scheduleApi.get.mockReset().mockResolvedValue(schedule)
  scheduleApi.save.mockReset().mockResolvedValue({ ...schedule, plannedDates: ['2026-11-02'], isDefault: false })
})

it('loads this week’s planned days, allows editing, and saves the explicit date set', async () => {
  renderDialog()
  const monday = await screen.findByRole('checkbox', { name: /2026-11-02.*周一/ })
  expect(monday).toBeChecked()
  expect(screen.getByText(/手工录入校验，Excel 导入始终按本周默认工作日校验/)).toBeInTheDocument()
  fireEvent.click(screen.getByRole('checkbox', { name: /2026-11-03.*周二/ }))
  fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }))
  await waitFor(() => expect(scheduleApi.save).toHaveBeenCalledWith('week', 2, ['2026-11-02', '2026-11-04', '2026-11-05', '2026-11-06']))
})

it('shows the plan read-only when the user has no schedule management permission', async () => {
  scheduleApi.get.mockResolvedValue({ ...schedule, canEdit: false })
  renderDialog()
  const monday = await screen.findByRole('checkbox', { name: /2026-11-02.*周一/ })
  expect(monday).toBeDisabled()
  expect(screen.getByRole('button', { name: /保\s*存/ })).toBeDisabled()
  expect(screen.getByText('当前账号仅可查看此计划。')).toBeInTheDocument()
})
