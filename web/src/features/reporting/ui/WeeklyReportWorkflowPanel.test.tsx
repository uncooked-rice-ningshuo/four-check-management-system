import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { StrictMode } from 'react'
import { describe, expect, it, vi } from 'vitest'
import type { ReportWorkflow, ReportingPeriod, ReportingRecord, ReportingSummaryRow } from '../../../entities/reporting/model/reporting'
import { WeeklyReportWorkflowPanel } from './WeeklyReportWorkflowPanel'

const mocks = vi.hoisted(() => ({ workflow: vi.fn(), records: vi.fn(), correct: vi.fn() }))
vi.mock('../model/queries', () => ({
  useReportWorkflow: mocks.workflow,
  useReportVersionRecords: mocks.records,
  useGenerateReportDraft: () => ({ mutate: vi.fn(), isPending: false }),
  useReportTransition: () => ({ mutate: vi.fn(), isPending: false }),
  useAddReportAdjustment: () => ({ mutate: vi.fn(), isPending: false }),
  useAddReportCorrection: () => ({ mutate: mocks.correct, isPending: false }),
}))

const period: ReportingPeriod = { academicYearId: 'year-1', academicYearName: '学年', semesterId: 'semester-1', semesterName: '学期', weekId: 'week-1', weekNo: 1, startDate: '2026-03-01', endDate: '2026-03-07' }
const summaryRow: ReportingSummaryRow = { departmentId: 'department-1', departmentName: '宿管部', collegeLevel: false, inspectionTypeId: 'type-1', inspectionTypeName: '宿舍卫生', isRequired: false, submissionCount: 1, draftBatchCount: 0, pendingReviewBatchCount: 0, returnedBatchCount: 0, confirmedBatchCount: 1, confirmedRecordCount: 1, pendingReviewOrReturnedRecordCount: 0, zeroRecordPendingReviewBatchCount: 0, zeroRecordReturnedBatchCount: 0, zeroRecordConfirmedBatchCount: 0 }
const record: ReportingRecord = { id: 'record-1', studentNumber: '', studentName: '测试学生', className: '测试班级', grade: '', typeCode: 'DORMITORY_HYGIENE', details: { result: 'UNQUALIFIED' }, currentDormitoryName: '', currentCourseName: '', currentClassroomName: '' }
const workflow: ReportWorkflow = {
  report: { id: 'report-1', semesterId: period.semesterId, weekId: period.weekId, status: 'DRAFT', versionNo: 1, rowVersion: 1, updatedAt: '2026-03-01' },
  versions: [{ id: 'version-1', versionNo: 1, createdBy: 'actor', createdAt: '2026-03-01', content: {
    period, scopeLimited: false, statisticsRuleVersion: 'v1', confirmedRecordCount: 1,
    rows: [{ departmentId: 'department-1', departmentName: '宿管部', collegeLevel: false, inspectionTypeId: 'type-1', inspectionType: '宿舍卫生', recordCount: 1, confirmedZeroReceiptCount: 0, dataStatus: 'CONFIRMED_RECORDS' }],
    incompleteCells: [], pendingDepartmentReconfirmationCount: 0, corrections: [], generatedAt: '2026-03-01',
  } }], reviews: [], corrections: [],
}

describe('通报记录修正', () => {
  it('未生成草稿时只在底栏提供生成入口', () => {
    mocks.workflow.mockReturnValue({ data: { report: null, versions: [], reviews: [], corrections: [] }, isError: false, isPending: false })
    mocks.records.mockReturnValue({ isError: false, isPending: false, isFetching: false })
    render(<WeeklyReportWorkflowPanel period={period} summaryRows={[summaryRow]} permissions={[{ action: 'REPORT_GENERATE', effect: 'ALLOW', scope: {}, sources: [] }]} onAccessDenied={vi.fn()} stage="source" />)

    const actions = screen.getByRole('region', { name: '当前阶段操作' })
    expect(within(actions).getByRole('button', { name: '确认数据并生成草稿' })).toBeVisible()
    expect(screen.getAllByRole('button', { name: '确认数据并生成草稿' })).toHaveLength(1)
  })

  it('来源阶段只在底栏提供继续编制入口', () => {
    mocks.workflow.mockReturnValue({ data: workflow, isError: false, isPending: false })
    mocks.records.mockReturnValue({ isError: false, isPending: false, isFetching: false })
    const onStageChange = vi.fn()
    render(<WeeklyReportWorkflowPanel period={period} summaryRows={[summaryRow]} permissions={[]} onAccessDenied={vi.fn()} stage="source" onStageChange={onStageChange} />)

    const actions = screen.getByRole('region', { name: '当前阶段操作' })
    expect(screen.getAllByRole('button', { name: '继续编制草稿' })).toHaveLength(1)
    fireEvent.click(within(actions).getByRole('button', { name: '继续编制草稿' }))
    expect(onStageChange).toHaveBeenCalledWith(1)
  })

  it('草稿阶段只显示一次当前版本，版本历史按需展开', () => {
    mocks.workflow.mockReturnValue({ data: workflow, isError: false, isPending: false })
    mocks.records.mockReturnValue({ isError: false, isPending: false, isFetching: false })
    render(<WeeklyReportWorkflowPanel period={period} summaryRows={[summaryRow]} permissions={[]} onAccessDenied={vi.fn()} stage="draft" />)

    expect(screen.getByText('草稿 · V1')).toBeVisible()
    expect(screen.getByText('版本与操作记录').closest('details')).not.toHaveAttribute('open')
    expect(screen.getByRole('region', { name: '按部门和检查类型分类' })).toBeVisible()
    fireEvent.click(screen.getByText('版本与操作记录'))
    expect(screen.getByText('版本与操作记录').closest('details')).toHaveAttribute('open')
    expect(screen.getByRole('columnheader', { name: '生成时间' })).toBeVisible()
    expect(screen.getByText('V1')).toBeVisible()
  })

  it('定稿核对保留分类明细，审核和退回集中在底栏', () => {
    mocks.workflow.mockReturnValue({ data: { ...workflow, report: { ...workflow.report, status: 'PENDING_REVIEW' } }, isError: false, isPending: false })
    mocks.records.mockReturnValue({ isError: false, isPending: false, isFetching: false })
    render(<WeeklyReportWorkflowPanel period={period} summaryRows={[summaryRow]} permissions={['REPORT_REVIEW', 'REPORT_RETURN'].map(action => ({ action, effect: 'ALLOW', scope: {}, sources: [] }))} onAccessDenied={vi.fn()} stage="final" />)

    expect(screen.getByRole('region', { name: '按部门和检查类型分类' })).toBeVisible()
    const actions = screen.getByRole('region', { name: '当前阶段操作' })
    expect(within(actions).getByRole('button', { name: '审核通过' })).toBeVisible()
    expect(within(actions).getByRole('button', { name: '退回草稿' })).toBeVisible()
    expect(screen.getAllByRole('button', { name: '审核通过' })).toHaveLength(1)
  })

  it('审核通过后只在底栏提供确认定稿入口', () => {
    mocks.workflow.mockReturnValue({ data: { ...workflow, report: { ...workflow.report, status: 'FINALIZING' } }, isError: false, isPending: false })
    mocks.records.mockReturnValue({ isError: false, isPending: false, isFetching: false })
    render(<WeeklyReportWorkflowPanel period={period} summaryRows={[summaryRow]} permissions={[{ action: 'REPORT_FINALIZE', effect: 'ALLOW', scope: {}, sources: [] }]} onAccessDenied={vi.fn()} stage="final" />)

    const actions = screen.getByRole('region', { name: '当前阶段操作' })
    expect(within(actions).getByRole('button', { name: '确认定稿数据' })).toBeVisible()
    expect(screen.getAllByRole('button', { name: '确认定稿数据' })).toHaveLength(1)
  })

  it('已定稿的状态只显示一次，底栏保留输出入口', () => {
    mocks.workflow.mockReturnValue({ data: { ...workflow, report: { ...workflow.report, status: 'FINALIZED' } }, isError: false, isPending: false })
    mocks.records.mockReturnValue({ isError: false, isPending: false, isFetching: false })
    render(<WeeklyReportWorkflowPanel period={period} summaryRows={[summaryRow]} permissions={[]} onAccessDenied={vi.fn()} stage="final" />)

    expect(screen.getByText('已定稿 · V1')).toBeVisible()
    const actions = screen.getByRole('region', { name: '当前阶段操作' })
    expect(within(actions).queryByText('已定稿 · 只读')).not.toBeInTheDocument()
    expect(within(actions).getByRole('button', { name: /Word/ })).toBeDisabled()
    expect(within(actions).getByRole('button', { name: /Excel/ })).toBeDisabled()
    expect(screen.getByRole('region', { name: '按部门和检查类型分类' })).toBeVisible()
  })

  it('退回草稿时直接显示最近退回原因，保留展开的历史记录', () => {
    mocks.workflow.mockReturnValue({ data: { ...workflow, report: { ...workflow.report, status: 'RETURNED' }, reviews: [
      { id: 'review-1', versionNo: 1, action: 'RETURNED', comment: '请核对旧数据', actorId: 'actor', createdAt: '2026-03-01' },
      { id: 'review-2', versionNo: 1, action: 'RETURNED', comment: '请补充本周记录', actorId: 'actor', createdAt: '2026-03-02' },
    ] }, isError: false, isPending: false })
    mocks.records.mockReturnValue({ isError: false, isPending: false, isFetching: false })
    render(<WeeklyReportWorkflowPanel period={period} summaryRows={[summaryRow]} permissions={[]} onAccessDenied={vi.fn()} stage="draft" />)

    expect(screen.getByText('退回原因：请补充本周记录')).toBeVisible()
    expect(screen.getByText('版本与操作记录').closest('details')).not.toHaveAttribute('open')
    fireEvent.click(screen.getByText('版本与操作记录'))
    expect(screen.getByText(/请核对旧数据/)).toBeVisible()
  })

  it('用中文选择检查结果，并提交对应的结果值', async () => {
    mocks.workflow.mockReturnValue({ data: workflow, isError: false, isPending: false })
    mocks.records.mockReturnValue({ data: { items: [record], total: 1 }, isError: false, isPending: false, isFetching: false })
    mocks.correct.mockReset()
    render(<StrictMode><WeeklyReportWorkflowPanel period={period} summaryRows={[summaryRow]} permissions={[{ action: 'REPORT_CORRECT', effect: 'ALLOW', scope: {}, sources: [] }]} onAccessDenied={vi.fn()} /></StrictMode>)

    fireEvent.click(screen.getByRole('button', { name: '查看明细' }))
    fireEvent.click(await screen.findByRole('button', { name: '修正' }))
    fireEvent.mouseDown(await screen.findByRole('combobox', { name: '修正字段' }))
    fireEvent.click(screen.getByText('检查结果', { selector: '.ant-select-item-option-content' }))
    expect(screen.getByText('不合格')).toBeInTheDocument()
    fireEvent.mouseDown(screen.getByRole('combobox', { name: '修正后的值' }))
    fireEvent.click(screen.getByText('合格', { selector: '.ant-select-item-option-content' }))
    fireEvent.change(screen.getByRole('textbox', { name: '修正原因' }), { target: { value: '核对检查结果后修正' } })
    fireEvent.click(screen.getByRole('button', { name: '保存为新版本' }))

    await waitFor(() => expect(mocks.correct).toHaveBeenCalledWith(expect.objectContaining({
      reportId: 'report-1', body: expect.objectContaining({ field: 'details.result', afterValue: 'QUALIFIED', factChanged: true }),
    }), expect.any(Object)))
  })
})
