import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import type { ReportArchivePage, ReportWorkflow, ReportingPeriod } from '../../entities/reporting/model/reporting'
import { WeeklyReportArchiveDetailPage, WeeklyReportArchivePage } from './WeeklyReportArchivePage'

const mocks = vi.hoisted(() => ({ useReportArchive: vi.fn(), useReportArchiveWorkflow: vi.fn(), records: vi.fn(), startRevision: vi.fn(), permissionResult: { data: { permissions: [] as Array<{action: string; effect: 'ALLOW'}>, canReviseFinalizedReport: false } } }))
vi.mock('../../features/reporting/model/queries', () => ({
  useReportArchive: mocks.useReportArchive,
  useReportArchiveWorkflow: mocks.useReportArchiveWorkflow,
  useReportVersionRecords: mocks.records,
  useStartReportRevision: () => ({ mutateAsync: mocks.startRevision, isPending: false }),
}))
vi.mock('../../features/attendance/hooks/useAttendancePermissions', () => ({
  useAttendancePermissions: () => mocks.permissionResult,
}))

const period: ReportingPeriod = {
  academicYearId: 'year-1', academicYearName: '2025-2026学年', semesterId: 'semester-1', semesterName: '第二学期',
  weekId: 'week-2', weekNo: 2, startDate: '2026-03-09T00:00:00Z', endDate: '2026-03-15T00:00:00Z',
}
const archivePage: ReportArchivePage = {
  items: [{ id: 'report-1', period, status: 'FINALIZED', versionNo: 1, versionCount: 1,
    confirmedRecordCount: 7, confirmedZeroReceiptCount: 0, cellCount: 1, scopeLimited: false, finalizedAt: '2026-03-16T08:00:00Z', updatedAt: '2026-03-16T08:00:00Z' }],
  total: 1, page: 1, pageSize: 20,
}
const archiveWorkflow: ReportWorkflow = {
  report: { id: 'report-1', semesterId: period.semesterId, weekId: period.weekId, status: 'FINALIZED', versionNo: 1, rowVersion: 3, updatedAt: '2026-03-16T08:00:00Z' },
  versions: [{ id: 'version-1', versionNo: 1, createdBy: 'actor-1', createdAt: '2026-03-16T07:00:00Z', content: {
    period, scopeLimited: false, statisticsRuleVersion: 'COUNT_DETAILS_V1', confirmedRecordCount: 7,
    rows: [{ departmentId: 'department-1', departmentName: '宿管部', collegeLevel: false,
      inspectionTypeId: 'type-1', inspectionType: '宿舍卫生', recordCount: 7, confirmedZeroReceiptCount: 0, dataStatus: 'CONFIRMED_RECORDS' }],
    incompleteCells: [], corrections: [], generatedAt: '2026-03-16T07:00:00Z',
  } }],
  reviews: [], corrections: [],
}

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<MemoryRouter initialEntries={['/reports/archive']}><QueryClientProvider client={client}><Routes><Route path='/reports/archive' element={<WeeklyReportArchivePage />} /><Route path='/reports/archive/:semesterId/:weekId' element={<WeeklyReportArchiveDetailPage />} /></Routes></QueryClientProvider></MemoryRouter>)
}

describe('WeeklyReportArchivePage', () => {
  beforeEach(() => {
    mocks.startRevision.mockReset().mockResolvedValue({ id: 'intent-1', reportId: 'report-1', baseVersion: 1 })
    mocks.permissionResult.data = { permissions: [], canReviseFinalizedReport: false }
    mocks.useReportArchive.mockReturnValue({ data: archivePage, isPending: false, isFetching: false, isError: false, refetch: vi.fn() })
    mocks.useReportArchiveWorkflow.mockReturnValue({ data: archiveWorkflow, isPending: false, isError: false, refetch: vi.fn() })
    mocks.records.mockReturnValue({ data: { items: [], total: 0 }, isPending: false, isFetching: false, isError: false, refetch: vi.fn() })
  })
  it('opens a week in a dedicated read-only detail page', async () => {
    renderPage()
    fireEvent.click(screen.getByRole('button', { name: '查看详情' }))
    expect(await screen.findByRole('region', { name: '周通报归档详情' })).toBeVisible()
    expect(screen.getByRole('navigation', { name: '通报版本' })).toBeVisible()
    expect(screen.getByRole('heading', { name: 'V1 · 通报快照' })).toBeVisible()
    expect(screen.getByRole('button', { name: '上一版本' })).toBeDisabled()
    expect(screen.getByRole('button', { name: '下一版本' })).toBeDisabled()
    expect(screen.queryByRole('region', { name: 'V1 周指标' })).not.toBeInTheDocument()
    expect(screen.queryByRole('tab', { name: '版本数据' })).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '周指标' }))
    expect(await screen.findByRole('dialog', { name: 'V1 · 周指标' })).toBeVisible()
    expect(screen.queryByRole('dialog', { name: '归档版本明细' })).not.toBeInTheDocument()
    expect(mocks.useReportArchiveWorkflow).toHaveBeenCalledWith(period.semesterId, period.weekId, true, expect.any(Function))
  })
  it('keeps archive correction unavailable to office members', async () => {
    renderPage()
    expect(screen.queryByRole('button', { name: '基于定稿修正' })).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '查看详情' }))
    expect(await screen.findByRole('region', { name: '周通报归档详情' })).toBeVisible()
    expect(screen.queryByRole('button', { name: '基于定稿修正' })).not.toBeInTheDocument()
  })
  it('requires confirmation before an office director opens finalized correction', async () => {
    mocks.permissionResult.data = { permissions: [], canReviseFinalizedReport: true }
    renderPage()
    fireEvent.click(screen.getByRole('button', { name: '基于定稿修正' }))
    expect(await screen.findByText(/当前定稿 V1 将保留/)).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: '通报归档' })).toBeVisible()
    expect(mocks.startRevision).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: '确认发起' }))
    await waitFor(() => expect(mocks.startRevision).toHaveBeenCalledWith({ reportId: 'report-1', semesterId: 'semester-1', weekId: 'week-2', versionNo: 1 }))
  })
  it('keeps the confirmation open when starting a revision fails', async () => {
    mocks.permissionResult.data = { permissions: [], canReviseFinalizedReport: true }
    mocks.startRevision.mockRejectedValue(new Error('配置版本已变化'))
    renderPage()
    fireEvent.click(screen.getByRole('button', { name: '基于定稿修正' }))
    fireEvent.click(await screen.findByRole('button', { name: '确认发起' }))
    expect(await screen.findByText('配置版本已变化')).toBeVisible()
    expect(screen.getByRole('button', { name: '确认发起' })).toBeVisible()
  })
  it('shows correction history and returns to the snapshot', async () => {
    mocks.useReportArchiveWorkflow.mockReturnValue({ data: { ...archiveWorkflow, corrections: [{ id: 'correction-1', versionNo: 1, recordId: 'record-1', field: 'studentName', reason: '核对姓名', beforeValue: '原姓名', afterValue: '修正姓名', correctedBy: 'office-user', correctedAt: '2026-03-16T08:00:00Z' }] }, isPending: false, isError: false, refetch: vi.fn() })
    renderPage()
    fireEvent.click(screen.getByRole('button', { name: '查看详情' }))
    fireEvent.click(await screen.findByRole('button', { name: /流程与修正记录/ }))
    expect(screen.getByRole('dialog', { name: '流程与修正记录' })).toBeVisible()
    expect(screen.getByText('审核通过后仍需确认定稿')).toBeVisible()
    expect(screen.getByRole('tab', { name: '审核与状态轨迹 (0)' })).toBeVisible()
    fireEvent.click(screen.getByRole('tab', { name: '追加修正记录 (1)' }))
    expect(screen.getByRole('columnheader', { name: '原值' })).toBeVisible()
    expect(screen.getByText('修正姓名')).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: 'V1 · 查看快照' }))
    await waitFor(() => expect(screen.queryByRole('dialog', { name: '流程与修正记录' })).not.toBeInTheDocument())
  })
  it('keeps the list status filter', () => {
    renderPage()
    expect(screen.getByRole('combobox', { name: /通报状态/ })).toBeVisible()
    expect(screen.getByRole('button', { name: /查\s*询/ })).toBeVisible()
  })
  it('moves between timeline versions', async () => {
    const report = archiveWorkflow.report
    if (!report) throw new Error('测试缺少通报')
    const secondVersion = { ...archiveWorkflow.versions[0], id: 'version-2', versionNo: 2, createdAt: '2026-03-17T07:00:00Z' }
    mocks.useReportArchiveWorkflow.mockReturnValue({ data: {
      ...archiveWorkflow, report: { ...report, versionNo: 2 }, versions: [...archiveWorkflow.versions, secondVersion],
    }, isPending: false, isError: false, refetch: vi.fn() })
    renderPage()
    fireEvent.click(screen.getByRole('button', { name: '查看详情' }))
    expect(await screen.findByRole('heading', { name: 'V2 · 通报快照' })).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: '上一版本' }))
    expect(screen.getByRole('heading', { name: 'V1 · 通报快照' })).toBeVisible()
    expect(screen.getByRole('button', { name: /V1/ })).toHaveAttribute('aria-current', 'step')
  })
})
