import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import type { ReportArchivePage, ReportWorkflow, ReportingPeriod, ReportingRecord } from '../../entities/reporting/model/reporting'
import { WeeklyReportArchivePage } from './WeeklyReportArchivePage'

const mocks = vi.hoisted(() => ({ useReportArchive: vi.fn(), useReportArchiveWorkflow: vi.fn(), records: vi.fn() }))
vi.mock('../../features/reporting/model/queries', () => ({
  useReportArchive: mocks.useReportArchive,
  useReportArchiveWorkflow: mocks.useReportArchiveWorkflow,
  useReportVersionRecords: mocks.records,
}))
vi.mock('../../features/attendance/hooks/useAttendancePermissions', () => ({
  useAttendancePermissions: () => ({ data: { permissions: [] } }),
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
  return render(<MemoryRouter><QueryClientProvider client={client}><WeeklyReportArchivePage /></QueryClientProvider></MemoryRouter>)
}

describe('WeeklyReportArchivePage', () => {
  beforeEach(() => {
    mocks.useReportArchive.mockReturnValue({ data: archivePage, isPending: false, isFetching: false, isError: false, refetch: vi.fn() })
    mocks.useReportArchiveWorkflow.mockReturnValue({ data: archiveWorkflow, isPending: false, isError: false, refetch: vi.fn() })
    mocks.records.mockReturnValue({ data: { items: [], total: 0 }, isPending: false, isFetching: false, isError: false, refetch: vi.fn() })
  })

  it('lists finalized snapshots separately and opens read-only version history', async () => {
    renderPage()

    expect(screen.getByRole('heading', { name: '通报归档' })).toBeVisible()
    expect(screen.getByRole('heading', { name: '通报归档' }).closest('.attendance-page-heading')).toBeInTheDocument()
    expect(screen.getByText('2025-2026学年 · 第二学期 · 第 2 周')).toBeVisible()
    expect(screen.getByText('2025-2026学年 · 第二学期 · 第 2 周').closest('.report-archive-period')).toBeInTheDocument()
    expect(screen.getByText('已定稿')).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: '查看版本' }))

    await waitFor(() => expect(screen.getByText(/历史版本/)).toBeVisible())
    expect(await screen.findByText('归档快照为只读记录')).toBeVisible()
    const versionRow = screen.getByRole('row', { name: /V1 当前.*查看明细/ })
    expect(within(versionRow).getByRole('button', { name: '查看 Word 文件' })).toBeVisible()
    expect(within(versionRow).getByRole('button', { name: '查看 Excel 文件' })).toBeVisible()
    expect(screen.queryByRole('columnheader', { name: '文件输出' })).not.toBeInTheDocument()
    await waitFor(() => expect(mocks.useReportArchiveWorkflow).toHaveBeenCalledWith(period.semesterId, period.weekId, true, expect.any(Function)))
  })

  it('归档周期中学期名称已含学年时只展示一次学年', async () => {
    const namedPeriod = { ...period, academicYearName: '2026-2027 学年', semesterName: '2026-2027 学年上学期', weekNo: 9 }
    mocks.useReportArchive.mockReturnValue({
      data: { ...archivePage, items: [{ ...archivePage.items[0], period: namedPeriod }] },
      isPending: false, isFetching: false, isError: false, refetch: vi.fn(),
    })
    renderPage()

    expect(screen.getByText('2026-2027 学年 · 上学期 · 第 9 周')).toBeVisible()
    expect(screen.queryByText(/2026-2027 学年 · 2026-2027 学年上学期/)).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '查看版本' }))
    expect(await screen.findByText('2026-2027 学年 · 上学期 · 第 9 周 · 历史版本')).toBeVisible()
  })

  it('把归档状态筛选放在 ProTable 表单中并将周通报列表置于其下方', () => {
    renderPage()

    expect(screen.getByRole('combobox', { name: /通报状态/ })).toBeVisible()
    expect(screen.getByRole('button', { name: /查\s*询/ })).toBeVisible()
    expect(screen.getByRole('button', { name: /重\s*置/ })).toBeVisible()
    expect(screen.getByText('周通报列表')).toBeVisible()
    expect(document.querySelector('.anticon-reload')).toBeVisible()
  })

  it('shows before and after values in correction history without a department confirmation column', async () => {
    mocks.useReportArchiveWorkflow.mockReturnValue({
      data: { ...archiveWorkflow, reviews: [
        { id: 'review-submit', versionNo: 1, action: 'SUBMITTED', comment: '', actorId: 'office-user', createdAt: '2026-03-16T07:10:00Z' },
        { id: 'review-return', versionNo: 1, action: 'RETURNED', comment: '请补齐检查说明', actorId: 'reviewer', createdAt: '2026-03-16T07:30:00Z' },
      ], corrections: [{
        id: 'correction-1', versionNo: 1, recordId: 'record-1', field: 'studentName', reason: '依据证据核对姓名',
        beforeValue: '原姓名', afterValue: '修正姓名', correctedBy: 'office-user', correctedAt: '2026-03-16T08:00:00Z',
      }] },
      isPending: false, isError: false, refetch: vi.fn(),
    })
    renderPage()
    fireEvent.click(screen.getByRole('button', { name: '查看版本' }))

    expect(await screen.findByRole('columnheader', { name: '原值' })).toBeVisible()
    expect(screen.getByRole('columnheader', { name: '修正后' })).toBeVisible()
    expect(screen.getByText('原姓名')).toBeVisible()
    expect(screen.getByText('修正姓名')).toBeVisible()
    expect(screen.queryByRole('columnheader', { name: '部门重确认' })).not.toBeInTheDocument()
    const reviewHistory = screen.getByText('审核与状态轨迹').closest('.report-archive-review-card')
    expect(reviewHistory).toHaveTextContent('提交审核')
    expect(reviewHistory).toHaveTextContent('请补齐检查说明')
    expect(screen.getByRole('heading', { name: '版本快照' })).toBeVisible()
  })

  it('从归档操作记录可追溯到对应版本的实际记录明细', async () => {
    const historicalRecord: ReportingRecord = {
      id: 'record-archive-1', studentNumber: '20260001', studentName: '归档快照学生', className: '计算机 1 班', grade: '2026级',
      typeCode: 'DORMITORY_HYGIENE', details: { date: '2026-03-11', round: '第二轮', result: 'UNQUALIFIED', description: '归档版本中的原始情况说明', remark: '归档版本中的原始备注' },
      currentDormitoryName: '3舍 204', currentCourseName: '', currentClassroomName: '', currentAreaName: '公共区域',
    }
    mocks.useReportArchiveWorkflow.mockReturnValue({
      data: { ...archiveWorkflow, corrections: [{
        id: 'correction-archive-1', versionNo: 1, recordId: historicalRecord.id, field: 'details.description',
        beforeValue: '原说明', afterValue: '修正说明', reason: '核对归档来源记录', correctedBy: 'office-user', correctedAt: '2026-03-16T08:00:00Z',
      }] },
      isPending: false, isError: false, refetch: vi.fn(),
    })
    mocks.records.mockReturnValue({ data: { items: [historicalRecord], total: 7,
      filterOptions: { studentNumbers: ['20260001'], studentNames: ['归档快照学生'], classNames: ['计算机 1 班'], grades: ['2026级'] } },
      isPending: false, isFetching: false, isError: false, refetch: vi.fn() })
    renderPage()
    fireEvent.click(screen.getByRole('button', { name: '查看版本' }))

    fireEvent.click(await screen.findByRole('button', { name: '查看 V1 数据明细' }))
    const versionContext = await screen.findByRole('group', { name: '归档版本上下文' })
    expect(within(versionContext).getByRole('heading', { name: 'V1 · 数据明细' })).toBeVisible()
    expect(within(versionContext).getByText('2025-2026学年 · 第二学期 · 第 2 周')).toBeVisible()
    expect(within(versionContext).queryByRole('button', { name: '生成 Word' })).not.toBeInTheDocument()
    expect(screen.getByRole('columnheader', { name: '生成文件' })).toBeVisible()
    expect(screen.queryByRole('button', { name: '返回历史版本' })).not.toBeInTheDocument()
    expect(screen.queryByText(/此处展示版本生成时保存的只读内容/)).not.toBeInTheDocument()
    const closeVersion = screen.getAllByRole('button', { name: 'Close' }).at(-1)
    if (!closeVersion) throw new Error('归档版本明细缺少关闭按钮')
    fireEvent.click(closeVersion)
    await waitFor(() => expect(screen.queryByRole('group', { name: '归档版本上下文' })).not.toBeInTheDocument())
    expect(screen.getByRole('button', { name: '查看 V1 数据明细' })).toBeVisible()

    fireEvent.click(screen.getByRole('button', { name: '查看 V1 数据明细' }))
    const snapshotRegion = await screen.findByRole('region', { name: 'V1 数据明细' })
    expect(within(snapshotRegion).getByRole('tab', { name: /宿舍卫生/ })).toBeVisible()
    const snapshotRow = within(snapshotRegion).getAllByRole('row').find(row => row.textContent?.includes('3舍 204'))
    expect(snapshotRow).toBeDefined()
    if (!snapshotRow) throw new Error('V1 归档快照中没有宿舍卫生记录')
    expect(within(snapshotRow).getByText('2026-03-11')).toBeVisible()
    expect(within(snapshotRegion).getByRole('columnheader', { name: '检查对象' })).toBeVisible()
    expect(within(snapshotRegion).getByRole('columnheader', { name: '寝室' })).toBeVisible()
    for (const value of ['3舍 204', '轮次', '第二轮', '不合格']) {
      expect(within(snapshotRow).getByText(value, { exact: true })).toBeVisible()
    }
    for (const irrelevantValue of ['课程', '教室', '公共区域']) {
      expect(within(snapshotRow).queryByText(irrelevantValue, { exact: true })).not.toBeInTheDocument()
    }
    expect(within(snapshotRegion).queryByRole('button', { name: 'Expand row' })).not.toBeInTheDocument()
    expect(within(snapshotRegion).getByText(/归档版本中的原始情况说明/)).toBeVisible()
    expect(within(snapshotRegion).getByText('归档版本中的原始备注')).toBeVisible()
    expect(within(snapshotRegion).queryByText('记录方式')).not.toBeInTheDocument()
  })
})
