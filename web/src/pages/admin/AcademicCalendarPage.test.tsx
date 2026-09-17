import { StrictMode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { updateAcademicYear } from '../../shared/api/adminApi'
import { AcademicCalendarPage } from './AcademicCalendarPage'

vi.mock('../../shared/api/adminApi', () => ({
  getAcademicCalendar: vi.fn().mockResolvedValue({
    academicYears: [{ id: 'y1', name: '2025-2026学年', startDate: '2025-09-02T00:00:00Z', endDate: '2026-08-30T00:00:00Z', enabled: true }],
    semesters: [{ id: 's1', academicYearId: 'y1', code: 'FIRST', name: '2025-2026学年上学期', startDate: '2025-09-03T00:00:00Z', endDate: '2026-01-30T00:00:00Z', enabled: true }],
    weeks: [{ id: 'w1', semesterId: 's1', weekNo: 2, startDate: '2025-09-08T00:00:00Z', endDate: '2025-09-14T00:00:00Z', note: '校历备注', enabled: true }],
  }),
  createAcademicYear: vi.fn(), createSemester: vi.fn(), createWeek: vi.fn(),
  updateAcademicYear: vi.fn(), updateSemester: vi.fn(), updateWeek: vi.fn(),
}))

function setup() {
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><StrictMode><AcademicCalendarPage /></StrictMode></QueryClientProvider>)
}

describe('学期周次交互', () => {
  it.each([
    ['学年', '2025-09-02', '2026-08-30'],
    ['学期', '2025-09-03', '2026-01-30'],
    ['周次', '2025-09-08', '2025-09-14'],
  ])('%s 编辑首次打开和重新打开均回显原数据', async (tab, start, end) => {
    setup()
    await screen.findByRole('tab', { name: tab })
    fireEvent.click(screen.getByRole('button', { name: '新增学年' }))
    const createDialog = await screen.findByRole('dialog')
    fireEvent.click(within(createDialog).getByRole('button', { name: /取\s*消/ }))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    fireEvent.click(screen.getByRole('tab', { name: tab }))
    for (let attempt = 0; attempt < 2; attempt++) {
      const editButton = screen.getAllByRole('button', { name: '编辑' }).at(-1)
      if (!editButton) throw new Error('未找到编辑按钮')
      fireEvent.click(editButton)
      const dialog = await screen.findByRole('dialog', { name: `编辑${tab}` })
      await waitFor(() => expect(within(dialog).getByPlaceholderText('开始日期')).toHaveValue(start))
      expect(within(dialog).getByPlaceholderText('结束日期')).toHaveValue(end)
      expect(within(dialog).getByPlaceholderText('开始日期').closest('.ant-picker-range')).not.toBeNull()
      if (tab === '周次') {
        expect(within(dialog).getByLabelText('周次')).toHaveValue('2')
        expect(within(dialog).getByLabelText('备注')).toHaveValue('校历备注')
      }
      fireEvent.change(within(dialog).getByPlaceholderText('开始日期'), { target: { value: '2025-10-01' } })
      fireEvent.click(within(dialog).getByRole('button', { name: /取\s*消/ }))
      await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    }
  }, 30_000)
  it('周次展示学年学期，并按时间范围筛选和重置', async () => {
    setup()
    fireEvent.click(await screen.findByRole('tab', { name: '周次' }))
    const row = (await screen.findByRole('cell', { name: '校历备注' })).closest('tr')
    if (!row) throw new Error('未找到周次行')
    expect(within(row).getByText('2025-2026学年')).toBeVisible()
    expect(within(row).getByText('2025-2026学年上学期')).toBeVisible()
    expect(screen.getByRole('combobox', { name: '学年' })).toBeVisible()
    expect(screen.getByRole('combobox', { name: '学期' })).toBeVisible()
    const start = screen.getByPlaceholderText('开始日期')
    const end = screen.getByPlaceholderText('结束日期')
    fireEvent.focus(start)
    fireEvent.change(start, { target: { value: '2025-10-01' } })
    fireEvent.keyDown(start, { key: 'Enter', code: 'Enter' })
    fireEvent.blur(start)
    fireEvent.focus(end)
    fireEvent.change(end, { target: { value: '2025-10-07' } })
    fireEvent.keyDown(end, { key: 'Enter', code: 'Enter' })
    fireEvent.blur(end)
    fireEvent.click(screen.getByRole('button', { name: /查\s*询/ }))
    await waitFor(() => expect(screen.queryByRole('cell', { name: '校历备注' })).not.toBeInTheDocument())
    fireEvent.click(screen.getByRole('button', { name: /重\s*置/ }))
    expect(await screen.findByRole('cell', { name: '校历备注' })).toBeVisible()
  }, 30_000)

  it('日期范围回显后保存仍提交 UTC 日期字符串', async () => {
    setup()
    await screen.findByRole('tab', { name: '学年' })
    fireEvent.click(screen.getByRole('button', { name: '编辑' }))
    const dialog = await screen.findByRole('dialog', { name: '编辑学年' })
    await waitFor(() => expect(within(dialog).getByPlaceholderText('开始日期')).toHaveValue('2025-09-02'))
    fireEvent.click(within(dialog).getByRole('button', { name: /确\s*定/ }))
    await waitFor(() => expect(updateAcademicYear).toHaveBeenCalledWith(expect.objectContaining({
      id: 'y1', startDate: '2025-09-02T00:00:00Z', endDate: '2026-08-30T00:00:00Z',
    })))
  }, 30_000)

  it.each(['学年', '学期', '周次'])('新增%s 使用日期范围选择器', async kind => {
    setup()
    await screen.findByRole('tab', { name: '学年' })
    fireEvent.click(screen.getByRole('button', { name: `新增${kind}` }))
    const dialog = await screen.findByRole('dialog', { name: `新增${kind}` })
    expect(within(dialog).getByPlaceholderText('开始日期').closest('.ant-picker-range')).not.toBeNull()
    expect(within(dialog).getByPlaceholderText('结束日期').closest('.ant-picker-range')).not.toBeNull()
  }, 30_000)

})
