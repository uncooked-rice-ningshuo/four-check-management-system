import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { reportingApi } from '../../shared/api/reportingApi'
import { ApiError } from '../../shared/api/httpClient'
import type { ReportingPeriod, ReportingSummary } from '../../entities/reporting/model/reporting'
import { WeeklyReportSummaryPage } from './WeeklyReportSummaryPage'

vi.mock('../../shared/api/reportingApi', () => ({
  reportingApi: { periods: vi.fn(), summary: vi.fn() },
}))

const period: ReportingPeriod = {
  academicYearId: 'year-1',
  academicYearName: '2025-2026学年',
  semesterId: 'semester-1',
  semesterName: '2025-2026下学期',
  weekId: 'week-2',
  weekNo: 2,
  startDate: '2026-03-09T00:00:00Z',
  endDate: '2026-03-15T00:00:00Z',
}

const summary: ReportingSummary = {
  period,
  rows: [{
    departmentId: 'department-1',
    departmentName: '学习部',
    collegeLevel: false,
    inspectionTypeId: 'type-1',
    inspectionTypeName: '到课率',
    isRequired: false,
    submissionCount: 1,
    draftBatchCount: 0,
    pendingReviewBatchCount: 0,
    returnedBatchCount: 0,
    confirmedBatchCount: 1,
    confirmedRecordCount: 42,
    pendingReviewOrReturnedRecordCount: 0,
    zeroRecordPendingReviewBatchCount: 0,
    zeroRecordReturnedBatchCount: 0,
    zeroRecordConfirmedBatchCount: 0,
  }],
  totals: {
    submissionCount: 1,
    draftBatchCount: 0,
    pendingReviewBatchCount: 0,
    returnedBatchCount: 0,
    confirmedBatchCount: 1,
    confirmedRecordCount: 42,
    pendingReviewOrReturnedRecordCount: 0,
    zeroRecordPendingReviewBatchCount: 0,
    zeroRecordReturnedBatchCount: 0,
    zeroRecordConfirmedBatchCount: 0,
  },
}

beforeEach(() => {
  vi.mocked(reportingApi.periods).mockReset().mockResolvedValue({ items: [period] })
  vi.mocked(reportingApi.summary).mockReset()
    .mockResolvedValueOnce(summary)
    .mockRejectedValueOnce(new Error('synthetic refresh failure'))
})

describe('周通报汇总缓存状态', () => {
  it('刷新失败时标明页面保留的是最近一次成功的汇总', async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: Infinity } } })
    render(<QueryClientProvider client={client}><WeeklyReportSummaryPage /></QueryClientProvider>)

    const selector = await screen.findByRole('combobox', { name: '选择通报学期和周次' })
    await waitFor(() => expect(selector).toBeEnabled())
    expect(screen.getByText(/若授权范围只覆盖部分部门或检查类型/)).toBeVisible()
    fireEvent.mouseDown(selector)
    fireEvent.click(screen.getByText(/第 2 周/, { selector: '.ant-select-item-option-content' }))
    await screen.findByText('部门与校级批次汇总', { exact: true })
    expect(screen.getAllByText(/2026年3月9日—2026年3月15日/).length).toBeGreaterThan(0)
    expect(screen.getByRole('columnheader', { name: '待办记录' })).toBeVisible()
    expect(screen.queryByRole('columnheader', { name: '未确认记录' })).not.toBeInTheDocument()
    expect(screen.getAllByText('42').length).toBeGreaterThan(0)

    fireEvent.click(screen.getByRole('button', { name: '刷新汇总' }))
    await screen.findByText('本次刷新失败，以下为最近一次成功加载的数据。请重试确认最新状态。')

    expect(screen.getAllByText('42').length).toBeGreaterThan(0)
    expect(reportingApi.summary).toHaveBeenCalledTimes(2)
  })

  it('汇总权限被撤销后清除旧缓存并隐藏先前数据', async () => {
    vi.mocked(reportingApi.summary).mockReset()
      .mockResolvedValueOnce(summary)
      .mockRejectedValue(new ApiError(403, 'FORBIDDEN_SCOPE', '当前无权查看该范围'))
    const client = new QueryClient({ defaultOptions: { queries: { retry: 1, gcTime: Infinity } } })
    render(<QueryClientProvider client={client}><WeeklyReportSummaryPage /></QueryClientProvider>)

    const selector = await screen.findByRole('combobox', { name: '选择通报学期和周次' })
    await waitFor(() => expect(selector).toBeEnabled())
    fireEvent.mouseDown(selector)
    fireEvent.click(screen.getByText(/第 2 周/, { selector: '.ant-select-item-option-content' }))
    await screen.findByText('部门与校级批次汇总', { exact: true })
    expect(screen.getAllByText('42').length).toBeGreaterThan(0)

    fireEvent.click(screen.getByRole('button', { name: '刷新汇总' }))

    await screen.findByText('你的通报查看权限已变化，系统已清除本地汇总数据。请重新选择当前可查看的周期。')
    await waitFor(() => expect(client.getQueryData(['reports', 'summary', 'semester-1', 'week-2'])).toBeUndefined())
    expect(screen.queryByText('部门与校级批次汇总', { exact: true })).not.toBeInTheDocument()
    expect(screen.queryByText('42')).not.toBeInTheDocument()
    expect(reportingApi.summary).toHaveBeenCalledTimes(2)
  })
})
