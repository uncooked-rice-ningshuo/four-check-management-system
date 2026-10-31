import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import type { ReportArchivePage, ReportWorkflow, ReportingPeriod } from '../../entities/reporting/model/reporting'
import { WeeklyReportArchivePage } from './WeeklyReportArchivePage'

const mocks = vi.hoisted(() => ({ useReportArchive: vi.fn(), useReportArchiveWorkflow: vi.fn() }))
vi.mock('../../features/reporting/model/queries', () => ({
  useReportArchive: mocks.useReportArchive,
  useReportArchiveWorkflow: mocks.useReportArchiveWorkflow,
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
    rows: [{ departmentId: 'department-1', departmentName: '学习部', collegeLevel: false,
      inspectionTypeId: 'type-1', inspectionType: '到课率', recordCount: 7, confirmedZeroReceiptCount: 0, dataStatus: 'CONFIRMED_RECORDS' }],
    incompleteCells: [], pendingDepartmentReconfirmationCount: 0, corrections: [], generatedAt: '2026-03-16T07:00:00Z',
  } }],
  reviews: [], corrections: [],
}

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<MemoryRouter><QueryClientProvider client={client}><WeeklyReportArchivePage /></QueryClientProvider></MemoryRouter>)
}

describe('WeeklyReportArchivePage', () => {
  beforeEach(() => {
    mocks.useReportArchive.mockReturnValue({ data: archivePage, isPending: false, isFetching: false, isError: false, refetch: vi.fn() })
    mocks.useReportArchiveWorkflow.mockReturnValue({ data: archiveWorkflow, isPending: false, isError: false, refetch: vi.fn() })
  })

  it('lists finalized snapshots separately and opens read-only version history', async () => {
    renderPage()

    expect(screen.getByRole('heading', { name: '通报归档' })).toBeVisible()
    expect(screen.getByText('2025-2026学年 · 第二学期 · 第 2 周')).toBeVisible()
    expect(screen.getByText('已定稿')).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: '查看版本' }))

    await waitFor(() => expect(screen.getByText(/历史版本/)).toBeVisible())
    expect(await screen.findByText('归档快照为只读记录')).toBeVisible()
    await waitFor(() => expect(mocks.useReportArchiveWorkflow).toHaveBeenCalledWith(period.semesterId, period.weekId, true, expect.any(Function)))
  })
})
