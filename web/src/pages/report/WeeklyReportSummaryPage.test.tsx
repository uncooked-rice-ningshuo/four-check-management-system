import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { reportingApi } from '../../shared/api/reportingApi'
import { ApiError } from '../../shared/api/httpClient'
import type { ReportWorkflow, ReportingPeriod, ReportingSummary } from '../../entities/reporting/model/reporting'
import { WeeklyReportSummaryPage } from './WeeklyReportSummaryPage'

vi.mock('../../shared/api/reportingApi', () => ({
  reportingApi: { periods: vi.fn(), summary: vi.fn(), workflow: vi.fn() },
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
  vi.mocked(reportingApi.workflow).mockReset().mockResolvedValue({ report: null, versions: [], reviews: [], corrections: [] })
})

describe('周通报汇总缓存状态', () => {
  it('在草稿中用中文展示来源状态、审核动作和修正字段，未知值不暴露内部代码', async () => {
    const statuses = ['NO_CONFIRMED_DATA', 'CONFIRMED_RECORDS', 'CONFIRMED_ZERO_RECORD_RECEIPT', 'CONFIRMED_RECORDS_AND_ZERO_RECEIPT', 'FUTURE_STATUS']
    const workflow: ReportWorkflow = {
      report: { id: 'report-1', semesterId: period.semesterId, weekId: period.weekId, status: 'DRAFT', versionNo: 1, rowVersion: 1, updatedAt: '2026-03-16T08:00:00Z' },
      versions: [{ id: 'version-1', versionNo: 1, createdBy: 'actor-1', createdAt: '2026-03-16T08:00:00Z', content: {
        period, scopeLimited: false, statisticsRuleVersion: 'COUNT_DETAILS_V1', confirmedRecordCount: 42,
        rows: statuses.map((dataStatus, index) => ({ departmentId: `department-${index}`, departmentName: `部门${index}`, collegeLevel: false,
          inspectionTypeId: `type-${index}`, inspectionType: '到课率', recordCount: 0, confirmedZeroReceiptCount: 0, dataStatus })),
        incompleteCells: [], pendingDepartmentReconfirmationCount: 0, corrections: [], generatedAt: '2026-03-16T08:00:00Z',
      } }],
      reviews: [{ id: 'review-1', versionNo: 1, action: 'FINALIZED', comment: '', actorId: 'actor-1', createdAt: '2026-03-16T08:00:00Z' }],
      corrections: [{ id: 'correction-1', versionNo: 1, recordId: 'record-1', field: 'details.result', reason: '核对结果', factChanged: true,
        reconfirmationStatus: 'REQUIRED', correctedBy: 'actor-1', correctedAt: '2026-03-16T08:00:00Z' }],
    }
    vi.mocked(reportingApi.summary).mockReset().mockResolvedValue(summary)
    vi.mocked(reportingApi.workflow).mockResolvedValue(workflow)
    const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: Infinity } } })
    render(<MemoryRouter initialEntries={['/reports/workbench?semesterId=semester-1&weekId=week-2&stage=1']}><QueryClientProvider client={client}><WeeklyReportSummaryPage /></QueryClientProvider></MemoryRouter>)

    expect(await screen.findByText('暂无已确认数据')).toBeVisible()
    expect(screen.getByText('已确认记录')).toBeVisible()
    expect(screen.getByText('已确认零记录')).toBeVisible()
    expect(screen.getByText('已确认记录及零记录回执')).toBeVisible()
    expect(screen.getByText('未知来源状态')).toBeVisible()
    expect(screen.getByText('草稿 · V1')).toBeVisible()
    expect(screen.getByRole('combobox', { name: '选择通报学期和周次' }).closest('.ant-select')).toHaveTextContent('2025-2026下学期 · 第 2 周')
    fireEvent.click(screen.getByText('版本与操作记录'))
    expect(screen.getByText('检查结果')).toBeVisible()
    expect(screen.getByText(/确认定稿 · V1/)).toBeVisible()
    expect(screen.getByRole('cell', { name: '待部门重确认' })).toBeVisible()
    for (const code of [...statuses, 'details.result', 'FINALIZED', 'REQUIRED']) expect(screen.queryByText(code)).not.toBeInTheDocument()
  })

  it('刷新失败时标明页面保留的是最近一次成功的汇总', async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: Infinity } } })
    const { container } = render(<MemoryRouter><QueryClientProvider client={client}><WeeklyReportSummaryPage /></QueryClientProvider></MemoryRouter>)

    const selector = await screen.findByRole('combobox', { name: '选择通报学期和周次' })
    await waitFor(() => expect(selector).toBeEnabled())
    expect(screen.getByRole('heading', { name: '通报工作台' })).toBeVisible()
    expect(selector.closest('.attendance-page-heading')).not.toBeNull()
    fireEvent.mouseDown(selector)
    fireEvent.click(screen.getByText(/第 2 周/, { selector: '.ant-select-item-option-content' }))
    await screen.findByText('部门与校级批次汇总', { exact: true })
    expect(screen.queryByText('周通报版本与状态')).not.toBeInTheDocument()
    const refreshButton = screen.getByRole('button', { name: '刷新汇总' })
    expect(refreshButton.closest('.weekly-report-summary-table-toolbar')).not.toBeNull()
    expect(container.querySelector('.weekly-report-summary-table')?.closest('.ant-card')).toBeNull()
    expect(container.querySelector('.weekly-report-kpi-card')).toBeNull()
    expect(screen.getAllByText(/2026年3月9日—2026年3月15日/).length).toBeGreaterThan(0)
    expect(screen.getByRole('columnheader', { name: '待审核/退回记录' })).toBeVisible()
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
    render(<MemoryRouter><QueryClientProvider client={client}><WeeklyReportSummaryPage /></QueryClientProvider></MemoryRouter>)

    const selector = await screen.findByRole('combobox', { name: '选择通报学期和周次' })
    await waitFor(() => expect(selector).toBeEnabled())
    fireEvent.mouseDown(selector)
    fireEvent.click(screen.getByText(/第 2 周/, { selector: '.ant-select-item-option-content' }))
    await screen.findByText('部门与校级批次汇总', { exact: true })
    expect(screen.getAllByText('42').length).toBeGreaterThan(0)

    fireEvent.click(screen.getByRole('button', { name: '刷新汇总' }))

    await screen.findByText('你的通报查看权限已变化，系统已清除本地汇总数据。请重新选择当前可查看的周期。')
    expect(screen.queryByText('部门与校级批次汇总', { exact: true })).not.toBeInTheDocument()
    await waitFor(() => expect(client.getQueryData(['reports', 'summary', 'semester-1', 'week-2'])).toBeUndefined())
    expect(screen.queryByText('部门与校级批次汇总', { exact: true })).not.toBeInTheDocument()
    expect(screen.queryByText('42')).not.toBeInTheDocument()
    expect(reportingApi.summary).toHaveBeenCalledTimes(2)
  })

  it('仅有部分范围汇总权限时保留来源汇总，不因完整版本被拒绝而清空周期', async () => {
    vi.mocked(reportingApi.summary).mockReset().mockResolvedValue(summary)
    vi.mocked(reportingApi.workflow).mockReset()
      .mockRejectedValue(new ApiError(403, 'FORBIDDEN_SCOPE', '当前无权查看该范围'))
    const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: Infinity } } })
    render(<MemoryRouter><QueryClientProvider client={client}><WeeklyReportSummaryPage /></QueryClientProvider></MemoryRouter>)

    const selector = await screen.findByRole('combobox', { name: '选择通报学期和周次' })
    await waitFor(() => expect(selector).toBeEnabled())
    fireEvent.mouseDown(selector)
    fireEvent.click(screen.getByText(/第 2 周/, { selector: '.ant-select-item-option-content' }))

    expect(await screen.findByRole('heading', { name: '部门与校级批次汇总' })).toBeVisible()
    expect(screen.getAllByText('42').length).toBeGreaterThan(0)
    expect(screen.getByText('当前账号无权查看完整通报版本')).toBeVisible()
    expect(screen.getByText(/2026年3月9日—2026年3月15日/)).toBeVisible()
    expect(screen.queryByText('你的通报查看权限已变化，系统已清除本地汇总数据。请重新选择当前可查看的周期。')).not.toBeInTheDocument()
    expect(client.getQueryData(['reports', 'summary', 'semester-1', 'week-2'])).toEqual(summary)
  })

  it('版本状态读取失败时展示安全的服务端提示并保留重试入口', async () => {
    vi.mocked(reportingApi.summary).mockReset().mockResolvedValue(summary)
    vi.mocked(reportingApi.workflow).mockReset()
      .mockRejectedValue(new ApiError(500, 'REPORT_SUMMARY_FAILED', '通报汇总暂时不可用，请稍后重试'))
    const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: Infinity } } })
    render(<MemoryRouter><QueryClientProvider client={client}><WeeklyReportSummaryPage /></QueryClientProvider></MemoryRouter>)

    const selector = await screen.findByRole('combobox', { name: '选择通报学期和周次' })
    await waitFor(() => expect(selector).toBeEnabled())
    fireEvent.mouseDown(selector)
    fireEvent.click(screen.getByText(/第 2 周/, { selector: '.ant-select-item-option-content' }))

    expect(await screen.findByText('通报版本状态加载失败')).toBeVisible()
    expect(screen.getByText('通报汇总暂时不可用，请稍后重试')).toBeVisible()
    expect(screen.getByRole('button', { name: /^重\s*试$/ })).toBeVisible()
  })
})
