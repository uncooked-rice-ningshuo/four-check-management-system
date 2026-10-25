import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { App as AntDesignApp } from 'antd'
import { StrictMode } from 'react'
import { beforeEach, expect, it, vi } from 'vitest'
import type { AcademicCalendar } from '../../../entities/academic/model/academic'
import { academicWeekImportApi } from '../../../shared/api/academicWeekImportApi'
import { AcademicWeekImportDrawer } from './AcademicWeekImportDrawer'

const validPreview = {
  batchId: 'batch-1', status: 'PREVIEW_READY' as const, version: 1, templateVersion: 2,
  semesterId: 'semester-1', semesterName: '2026-2027 学年上学期', academicYearName: '2026-2027 学年',
  totalRows: 1, validRows: 1, invalidRows: 0, affectsToday: false,
  rows: [{ rowNo: 6, values: { week_no: '1', start_date: '2026-09-07', end_date: '2026-09-13', attendance_penalty_multiplier: '1' }, status: 'VALID' as const, errors: [] }],
  createdAt: '2026-10-24T00:00:00Z',
}

vi.mock('../../../shared/api/academicWeekImportApi', () => ({
  academicWeekImportApi: {
    spec: vi.fn().mockResolvedValue({ templateCode: 'ACADEMIC_WEEKS', templateVersion: 2, sheetName: '周次导入', maxRows: 60, fields: [] }),
    template: vi.fn().mockResolvedValue({ blob: new Blob(['template']), headers: new Headers() }),
    preview: vi.fn(), batch: vi.fn(), confirm: vi.fn(),
  },
}))

const calendar: AcademicCalendar = {
  academicYears: [{ id: 'year-1', name: '2026-2027 学年', startDate: '2026-09-01T00:00:00Z', endDate: '2027-08-31T00:00:00Z', enabled: false }],
  semesters: [{ id: 'semester-1', academicYearId: 'year-1', code: 'FIRST', name: '2026-2027 学年上学期', startDate: '2026-09-01T00:00:00Z', endDate: '2027-01-31T00:00:00Z', enabled: true }],
  weeks: [],
}

beforeEach(() => vi.clearAllMocks())

it('requires an explicit target semester and warns when its parent year is disabled', async () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(<AntDesignApp><QueryClientProvider client={client}><StrictMode><AcademicWeekImportDrawer open calendar={calendar} onClose={vi.fn()} /></StrictMode></QueryClientProvider></AntDesignApp>)

  const target = await screen.findByRole('combobox', { name: '导入目标学期' })
  expect(target).toHaveValue('')
  expect(screen.getByRole('button', { name: /上传并生成预览/ })).toBeDisabled()
  expect(screen.queryByText('目标学期或所属学年已停用')).not.toBeInTheDocument()
  expect(academicWeekImportApi.preview).not.toHaveBeenCalled()
})

it('clears an explicit semester selection when the drawer closes and reopens', async () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const onClose = vi.fn()
  const view = render(<AntDesignApp><QueryClientProvider client={client}><AcademicWeekImportDrawer open calendar={calendar} onClose={onClose} /></QueryClientProvider></AntDesignApp>)
  const target = await screen.findByRole('combobox', { name: '导入目标学期' })
  fireEvent.mouseDown(target)
  fireEvent.click(await screen.findByText(/2026-2027 学年上学期（停用）/))
  expect(await screen.findByText('目标学期或所属学年已停用')).toBeInTheDocument()

  fireEvent.click(screen.getByRole('button', { name: 'Close' }))
  expect(onClose).toHaveBeenCalledOnce()
  view.rerender(<AntDesignApp><QueryClientProvider client={client}><AcademicWeekImportDrawer open={false} calendar={calendar} onClose={onClose} /></QueryClientProvider></AntDesignApp>)
  view.rerender(<AntDesignApp><QueryClientProvider client={client}><AcademicWeekImportDrawer open={true} calendar={calendar} onClose={onClose} /></QueryClientProvider></AntDesignApp>)
  expect(await screen.findByRole('combobox', { name: '导入目标学期' })).toHaveValue('')
})

it('clears the previous workbook when replacing a preview', async () => {
  vi.mocked(academicWeekImportApi.preview).mockResolvedValue(validPreview)
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(<AntDesignApp><QueryClientProvider client={client}><AcademicWeekImportDrawer open calendar={calendar} onClose={vi.fn()} /></QueryClientProvider></AntDesignApp>)

  const target = await screen.findByRole('combobox', { name: '导入目标学期' })
  fireEvent.mouseDown(target)
  fireEvent.click(await screen.findByText(/2026-2027 学年上学期（停用）/))
  const file = new File(['workbook'], 'academic-week.xlsx', { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' })
  fireEvent.change(document.querySelector('input[type="file"]') as HTMLInputElement, { target: { files: [file] } })
  fireEvent.click(screen.getByRole('button', { name: /上传并生成预览/ }))

  await screen.findByRole('button', { name: '更换文件' })
  fireEvent.click(screen.getByRole('button', { name: '更换文件' }))
  await waitFor(() => expect(screen.getByRole('button', { name: /上传并生成预览/ })).toBeDisabled())
})
