import { StrictMode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ConfigProvider } from 'antd'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { ApiError } from '../../shared/api/httpClient'
import { updateAcademicYear } from '../../shared/api/adminApi'
import { AcademicCalendarPage } from './AcademicCalendarPage'

vi.mock('../../shared/api/adminApi', () => ({
  getAcademicCalendar: vi.fn().mockResolvedValue({
    academicYears: [{ id: 'y1', name: '2025-2026学年', startDate: '2025-09-02T00:00:00Z', endDate: '2026-08-30T00:00:00Z', enabled: true }],
    semesters: [{ id: 's1', academicYearId: 'y1', code: 'FIRST', name: '2025-2026学年上学期', startDate: '2025-09-03T00:00:00Z', endDate: '2026-01-30T00:00:00Z', enabled: true }],
    weeks: [{ id: 'w1', semesterId: 's1', weekNo: 2, startDate: '2025-09-08T00:00:00Z', endDate: '2025-09-14T00:00:00Z', note: '校历备注', enabled: true, attendancePenaltyMultiplier: 2 }],
  }),
  createAcademicYear: vi.fn(), createSemester: vi.fn(), createWeek: vi.fn(),
  updateAcademicYear: vi.fn(), updateSemester: vi.fn(), updateWeek: vi.fn(),
}))

function setup() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(<QueryClientProvider client={client}><StrictMode><AcademicCalendarPage /></StrictMode></QueryClientProvider>)
  return client
}

describe('学期周次交互', () => {
  it('创建弹窗关闭后首次编辑不会操作未挂载的校历表单', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    try {
      setup()
      await screen.findByRole('tab', { name: '学年' })
      fireEvent.click(screen.getByRole('button', { name: '新增学年' }))
      const createDialog = await screen.findByRole('dialog', { name: '新增学年' })
      fireEvent.click(within(createDialog).getByRole('button', { name: /取\s*消/ }))
      await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
      fireEvent.click(screen.getByRole('button', { name: '编辑' }))
      const editDialog = await screen.findByRole('dialog', { name: '编辑学年' })
      await waitFor(() => expect(within(editDialog).getByPlaceholderText('开始日期')).toHaveValue('2025-09-02'))
      await new Promise(resolve => setTimeout(resolve, 0))
      const formWarnings = consoleError.mock.calls.map(call => call.map(String).join(' '))
        .filter(text => text.includes('Instance created by `useForm` is not connected'))
      expect(formWarnings).toEqual([])
    } finally {
      consoleError.mockRestore()
    }
  }, 30_000)

  it('校历表单挂载前后及保存提示不产生 Ant Design 警告', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const warningMessages = () => consoleError.mock.calls.map(call => call.map(String).join(' '))
    try {
      render(<ConfigProvider theme={{ token: { colorPrimary: '#0b5cad' } }}><QueryClientProvider client={client}><StrictMode><AcademicCalendarPage /></StrictMode></QueryClientProvider></ConfigProvider>)
      await screen.findByRole('tab', { name: '学年' })
      await new Promise(resolve => setTimeout(resolve, 0))
      expect(warningMessages().filter(text => text.includes('Instance created by `useForm` is not connected'))).toEqual([])

      fireEvent.click(screen.getByRole('button', { name: '编辑' }))
      const dialog = await screen.findByRole('dialog', { name: '编辑学年' })
      await waitFor(() => expect(within(dialog).getByPlaceholderText('开始日期')).toHaveValue('2025-09-02'))
      fireEvent.click(within(dialog).getByRole('button', { name: /确\s*定/ }))
      await waitFor(() => expect(updateAcademicYear).toHaveBeenCalledWith(expect.objectContaining({ id: 'y1' })))
      await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
      expect(warningMessages().filter(text => text.includes('Static function can not consume context'))).toEqual([])
    } finally {
      consoleError.mockRestore()
    }
  }, 30_000)

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
        expect(within(dialog).getByText('教学礼拜周（2 倍）')).toBeVisible()
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
    const start = screen.getAllByPlaceholderText('开始日期').find(input => !input.closest('.ant-modal'))
    const end = screen.getAllByPlaceholderText('结束日期').find(input => !input.closest('.ant-modal'))
    if (!start || !end) throw new Error('未找到校历筛选范围输入框')
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

  it('保存失败提示原因并保留输入，重试成功刷新所有校历使用方', async () => {
    vi.mocked(updateAcademicYear).mockRejectedValueOnce(new ApiError(409, 'ACADEMIC_DATE_OVERLAP', '日期范围重叠，请核对校历'))
    const client = setup()
    const dependentKeys = [['work-context'], ['academic', 'current-week'], ['academic', 'calendar', 'override'], ['dashboard', 'workbench'], ['attendance', 'filter-options']]
    dependentKeys.forEach(key => client.setQueryData(key, { previous: true }))
    await screen.findByRole('tab', { name: '学年' })
    fireEvent.click(screen.getByRole('button', { name: '编辑' }))
    const dialog = await screen.findByRole('dialog', { name: '编辑学年' })
    await waitFor(() => expect(within(dialog).getByPlaceholderText('开始日期')).toHaveValue('2025-09-02'))
    fireEvent.click(within(dialog).getByRole('button', { name: /确\s*定/ }))
    expect(await screen.findByText('日期范围重叠，请核对校历')).toBeVisible()
    expect(within(dialog).getByPlaceholderText('开始日期')).toHaveValue('2025-09-02')
    dependentKeys.forEach(key => expect(client.getQueryState(key)?.isInvalidated).toBe(false))
    await waitFor(() => expect(within(dialog).getByRole('button', { name: /确\s*定/ })).not.toHaveClass('ant-btn-loading'))
    fireEvent.click(within(dialog).getByRole('button', { name: /确\s*定/ }))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    dependentKeys.forEach(key => expect(client.getQueryState(key)?.isInvalidated).toBe(true))
  }, 30_000)

})
